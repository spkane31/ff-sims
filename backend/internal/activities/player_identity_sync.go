package activities

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"go.temporal.io/sdk/activity"
	"gorm.io/gorm"

	"backend/internal/models"
)

// playerIdentitySyncBatchSize matches FetchAndUpsertAllPlayers' batch order
// of magnitude; sleeper_players tops out around 12k rows, so this is cheap
// either way.
const playerIdentitySyncBatchSize = 500

// playerIdentityHeartbeatInterval controls how often SyncPlayerIdentities
// heartbeats *within* a batch, not just once per completed batch. Each row
// is its own DB round-trip (no bulk upsert — see linkOrCreateSkillPlayer),
// so a full 500-row batch against the real production DB can take well
// longer than a single heartbeat timeout even though the activity is still
// actively working; heartbeating every 25 rows keeps well inside that
// window regardless of batch size or per-row latency.
const playerIdentityHeartbeatInterval = 25

// sleeperDefPosition/espnDefPositions are the position labels Sleeper
// ("DEF") and ESPN ("DEF" or legacy "D/ST") use for team defenses, kept
// distinct from real players throughout this file because team-defense
// "player IDs" are known to be inconsistent across platforms — see
// identityConflict reasons below.
const sleeperDefPosition = "DEF"

var espnDefPositions = []string{"DEF", "D/ST"}

// identityConflict is a case SyncPlayerIdentities declined to resolve
// automatically — surfaced in the returned error rather than silently
// skipped, so a human can look at the specific players involved and fix it
// by hand.
type identityConflict struct {
	SleeperPlayerID string
	Reason          string
}

// SyncPlayerIdentities mirrors sleeper_players into players' sleeper_id
// column so /players/:id can be resolved from a Sleeper player ID, not just
// an ESPN one. It matches skill positions by espn_id (players is otherwise
// scoped to whichever ESPN league(s) this app has ETL'd, so most Sleeper
// players won't have an existing row and get a new identity-only one) and
// team defenses by team abbreviation instead, since defense "player IDs"
// aren't reliably comparable across Sleeper and ESPN. Existing ESPN-sourced
// Name/Position/Team are never overwritten — ESPN stays authoritative there.
//
// Benign non-matches (no espn_id, or a valid espn_id with no existing
// players row) are handled inline by creating a new row. Genuine conflicts
// (an espn_id match whose players row already carries a *different*
// sleeper_id, or a DEF row that resolves to zero or multiple players rows by
// team) are collected instead of silently skipped; if any are found, the
// activity returns an itemized error after all other rows in the run have
// already committed, so it shows up as a clear, actionable Temporal failure
// for manual follow-up rather than an easy-to-miss counter.
func (a *PlayerSyncActivities) SyncPlayerIdentities(ctx context.Context) (PlayerIdentitySyncResult, error) {
	var result PlayerIdentitySyncResult
	var conflicts []identityConflict

	lastID := ""
	for {
		var batch []models.SleeperPlayer
		if err := a.DB.WithContext(ctx).
			Where("sleeper_player_id > ?", lastID).
			Order("sleeper_player_id").
			Limit(playerIdentitySyncBatchSize).
			Find(&batch).Error; err != nil {
			return result, fmt.Errorf("player identity sync: fetch sleeper_players after %q: %w", lastID, err)
		}
		if len(batch) == 0 {
			break
		}

		batchConflicts, err := a.syncIdentityBatch(ctx, batch, &result)
		if err != nil {
			return result, fmt.Errorf("player identity sync: %w", err)
		}
		conflicts = append(conflicts, batchConflicts...)

		lastID = batch[len(batch)-1].SleeperPlayerID
		result.Scanned += len(batch)
		activity.RecordHeartbeat(ctx, result.Scanned)

		if len(batch) < playerIdentitySyncBatchSize {
			break
		}
	}

	if len(conflicts) > 0 {
		result.Conflicts = len(conflicts)
		lines := make([]string, len(conflicts))
		for i, c := range conflicts {
			lines[i] = fmt.Sprintf("  sleeper_player_id=%s: %s", c.SleeperPlayerID, c.Reason)
		}
		return result, fmt.Errorf(
			"player identity sync: %d conflict(s) need manual review:\n%s",
			len(conflicts), strings.Join(lines, "\n"),
		)
	}
	return result, nil
}

func (a *PlayerSyncActivities) syncIdentityBatch(ctx context.Context, batch []models.SleeperPlayer, result *PlayerIdentitySyncResult) ([]identityConflict, error) {
	db := a.DB.WithContext(ctx)
	var conflicts []identityConflict

	processed := 0
	heartbeat := func() {
		processed++
		if processed%playerIdentityHeartbeatInterval == 0 {
			activity.RecordHeartbeat(ctx, result.Scanned+processed)
		}
	}

	var defRows, skillRows []models.SleeperPlayer
	for _, sp := range batch {
		if sp.Position == sleeperDefPosition {
			defRows = append(defRows, sp)
		} else {
			skillRows = append(skillRows, sp)
		}
	}

	// Already-linked sleeper players are a no-op — check the whole batch up
	// front so a re-run doesn't re-touch rows a previous run already synced.
	sleeperIDs := make([]string, 0, len(batch))
	for _, sp := range batch {
		sleeperIDs = append(sleeperIDs, sp.SleeperPlayerID)
	}
	alreadyLinked, err := models.GetPlayersBySleeperIDs(db, sleeperIDs)
	if err != nil {
		return nil, fmt.Errorf("look up already-linked players: %w", err)
	}

	if len(skillRows) > 0 {
		espnIDs := make([]int64, 0, len(skillRows))
		for _, sp := range skillRows {
			if id, err := strconv.ParseInt(sp.EspnID, 10, 64); err == nil && id > 0 {
				espnIDs = append(espnIDs, id)
			}
		}
		espnMatches, err := models.GetPlayersByESPNIDs(db, espnIDs)
		if err != nil {
			return nil, fmt.Errorf("look up players by espn_id: %w", err)
		}
		for _, sp := range skillRows {
			heartbeat()
			if _, ok := alreadyLinked[sp.SleeperPlayerID]; ok {
				continue
			}
			espnID, _ := strconv.ParseInt(sp.EspnID, 10, 64)
			outcome, c, err := a.linkOrCreateSkillPlayer(db, sp, espnID, espnMatches)
			if err != nil {
				return nil, err
			}
			if c != nil {
				conflicts = append(conflicts, *c)
				continue
			}
			if outcome == outcomeLinked {
				result.Linked++
			} else {
				result.Created++
			}
		}
	}

	if len(defRows) > 0 {
		teams := make([]string, 0, len(defRows))
		for _, sp := range defRows {
			teams = append(teams, sp.NflTeam)
		}
		var existingDefs []models.Player
		if err := db.Where("position IN ? AND team IN ?", espnDefPositions, teams).Find(&existingDefs).Error; err != nil {
			return nil, fmt.Errorf("look up defenses by team: %w", err)
		}
		defsByTeam := map[string][]models.Player{}
		for _, p := range existingDefs {
			defsByTeam[p.Team] = append(defsByTeam[p.Team], p)
		}
		for _, sp := range defRows {
			heartbeat()
			if _, ok := alreadyLinked[sp.SleeperPlayerID]; ok {
				continue
			}
			matches := defsByTeam[sp.NflTeam]
			if len(matches) != 1 {
				conflicts = append(conflicts, identityConflict{
					SleeperPlayerID: sp.SleeperPlayerID,
					Reason: fmt.Sprintf(
						"DEF team=%q resolved to %d players rows (expected exactly 1); team-abbreviation mismatch between Sleeper and ESPN, or a duplicate defense row",
						sp.NflTeam, len(matches),
					),
				})
				continue
			}
			existing := matches[0]
			if existing.SleeperID != "" && existing.SleeperID != sp.SleeperPlayerID {
				conflicts = append(conflicts, identityConflict{
					SleeperPlayerID: sp.SleeperPlayerID,
					Reason: fmt.Sprintf(
						"DEF team=%q players.id=%d already linked to a different sleeper_id=%q",
						sp.NflTeam, existing.ID, existing.SleeperID,
					),
				})
				continue
			}
			if err := db.Model(&models.Player{}).Where("id = ?", existing.ID).
				Update("sleeper_id", sp.SleeperPlayerID).Error; err != nil {
				return nil, fmt.Errorf("link defense %s to player %d: %w", sp.SleeperPlayerID, existing.ID, err)
			}
			result.Linked++
		}
	}

	return conflicts, nil
}

// skillOutcome distinguishes linking to an existing ESPN-sourced player from
// creating a brand-new identity-only row, so the caller can attribute the
// right counter without re-deriving which path was taken.
type skillOutcome int

const (
	outcomeCreated skillOutcome = iota
	outcomeLinked
)

// linkOrCreateSkillPlayer resolves one non-DEF Sleeper player against the
// (already batch-fetched) espnMatches map. It returns a non-nil conflict
// instead of writing when the matched players row is already claimed by a
// different Sleeper player; otherwise it links or creates and reports which.
func (a *PlayerSyncActivities) linkOrCreateSkillPlayer(db *gorm.DB, sp models.SleeperPlayer, espnID int64, espnMatches map[int64]models.Player) (skillOutcome, *identityConflict, error) {
	if espnID > 0 {
		if existing, ok := espnMatches[espnID]; ok {
			if existing.SleeperID != "" && existing.SleeperID != sp.SleeperPlayerID {
				return 0, &identityConflict{
					SleeperPlayerID: sp.SleeperPlayerID,
					Reason: fmt.Sprintf(
						"espn_id=%d players.id=%d already linked to a different sleeper_id=%q",
						espnID, existing.ID, existing.SleeperID,
					),
				}, nil
			}
			if err := db.Model(&models.Player{}).Where("id = ?", existing.ID).
				Update("sleeper_id", sp.SleeperPlayerID).Error; err != nil {
				return 0, nil, fmt.Errorf("link player %d to sleeper_id %s: %w", existing.ID, sp.SleeperPlayerID, err)
			}
			return outcomeLinked, nil, nil
		}
	}

	newPlayer := models.Player{
		ESPNID:    espnID, // 0 when unknown, matching espn_id's existing "unset" sentinel
		SleeperID: sp.SleeperPlayerID,
		Name:      sp.FullName,
		Position:  sp.Position,
		Team:      sp.NflTeam,
	}
	if err := db.Create(&newPlayer).Error; err != nil {
		if IsUniqueViolation(err) {
			return 0, &identityConflict{
				SleeperPlayerID: sp.SleeperPlayerID,
				Reason: fmt.Sprintf(
					"create failed on a unique constraint (likely espn_id=%d already used by a different Sleeper player, or a concurrent write): %v",
					espnID, err,
				),
			}, nil
		}
		return 0, nil, fmt.Errorf("create player for sleeper_id %s: %w", sp.SleeperPlayerID, err)
	}
	return outcomeCreated, nil, nil
}

// IsUniqueViolation reports whether err is a Postgres unique_violation
// (SQLSTATE 23505), the class of error a duplicate espn_id or sleeper_id can
// legitimately produce despite the batch-level pre-checks above (e.g. two
// Sleeper player IDs mapped to the same espn_id in Sleeper's own data).
// Exported for direct unit testing — SQLite (used elsewhere in this
// package's tests) doesn't produce pgconn.PgError, so this classification
// logic can only be exercised in isolation, not via a live duplicate-insert
// race in tests.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
