package activities

import (
	"context"
	"errors"
	"fmt"
	"strconv"

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
// is its own DB round-trip (no bulk upsert — see linkOrCreatePlayer), so a
// full 500-row batch against the real production DB can take well longer
// than a single heartbeat timeout even though the activity is still
// actively working; heartbeating every 25 rows keeps well inside that
// window regardless of batch size or per-row latency.
const playerIdentityHeartbeatInterval = 25

// duplicatePlayerFullName is Sleeper's own placeholder label for a stale
// duplicate player entry. A production run found several: Sleeper had
// issued a corrected/replacement sleeper_player_id for the same real
// athlete without deprecating the old one, and either ID can carry the same
// espn_id — sometimes with the placeholder-labeled one processed first
// (ascending sleeper_player_id order) and wrongly winning the espn_id claim
// ahead of the real player's row. These rows are excluded from the sync
// entirely — never matched against, never used to create a players row —
// since Sleeper itself is telling us not to trust them.
const duplicatePlayerFullName = "Duplicate Player"

// identityConflict is a case SyncPlayerIdentities declined to resolve
// automatically. Unlike duplicatePlayerFullName rows (a known, confidently
// excludable pattern), these are genuinely ambiguous — e.g. two distinct,
// real-looking sleeper_players rows with the same name and different
// age/experience both claiming the same espn_id, with no Sleeper-provided
// signal for which is current — so they're reported in the result for a
// human to look at rather than guessed at automatically.
type identityConflict struct {
	SleeperPlayerID string
	Reason          string
}

// SyncPlayerIdentities mirrors sleeper_players into players' sleeper_id
// column so /players/:id can be resolved from a Sleeper player ID, not just
// an ESPN one. Every position — including team defenses, whose ESPN IDs are
// small stable negative numbers (e.g. -16025 for the 49ers), not something
// that needs separate handling — matches by espn_id the same way. Existing
// ESPN-sourced Name/Position/Team are never overwritten on a match — ESPN
// stays authoritative there.
//
// Benign non-matches (no espn_id, or a valid espn_id with no existing
// players row) are handled inline by creating a new row. Genuine conflicts
// (an espn_id match whose players row already carries a *different*
// sleeper_id) are collected and reported via PlayerIdentitySyncResult's
// Conflicts/ConflictDetails rather than failing the activity — a handful of
// permanently-ambiguous Sleeper duplicate-ID rows would otherwise fail this
// workflow every single day forever with no code-level fix available, which
// is worse than surfacing them plainly in the result for a human to check
// periodically.
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
		result.ConflictDetails = make([]string, len(conflicts))
		for i, c := range conflicts {
			result.ConflictDetails[i] = fmt.Sprintf("sleeper_player_id=%s: %s", c.SleeperPlayerID, c.Reason)
		}
	}
	return result, nil
}

// syncIdentityBatch runs the whole batch's writes in a single DB
// transaction instead of letting each row auto-commit individually — a
// production run doing ~12k individual commits (each with its own
// round-trip and fsync) took over 30 minutes and still didn't finish; one
// commit per batch of up to playerIdentitySyncBatchSize rows is dramatically
// cheaper. The one write that can legitimately fail mid-batch (create, on a
// duplicate espn_id) is wrapped in its own savepoint so that conflict rolls
// back just that attempt instead of aborting the whole transaction and
// losing every other row's work in the batch.
func (a *PlayerSyncActivities) syncIdentityBatch(ctx context.Context, batch []models.SleeperPlayer, result *PlayerIdentitySyncResult) ([]identityConflict, error) {
	var conflicts []identityConflict
	err := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var txErr error
		conflicts, txErr = a.syncIdentityBatchTx(ctx, tx, batch, result)
		return txErr
	})
	return conflicts, err
}

func (a *PlayerSyncActivities) syncIdentityBatchTx(ctx context.Context, db *gorm.DB, batch []models.SleeperPlayer, result *PlayerIdentitySyncResult) ([]identityConflict, error) {
	var conflicts []identityConflict

	processed := 0
	heartbeat := func() {
		processed++
		if processed%playerIdentityHeartbeatInterval == 0 {
			activity.RecordHeartbeat(ctx, result.Scanned+processed)
		}
	}

	rows := make([]models.SleeperPlayer, 0, len(batch))
	for _, sp := range batch {
		if sp.FullName == duplicatePlayerFullName {
			continue
		}
		rows = append(rows, sp)
	}

	// Already-linked sleeper players are a no-op — check up front so a
	// re-run doesn't re-touch rows a previous run already synced.
	sleeperIDs := make([]string, 0, len(rows))
	for _, sp := range rows {
		sleeperIDs = append(sleeperIDs, sp.SleeperPlayerID)
	}
	alreadyLinked, err := models.GetPlayersBySleeperIDs(db, sleeperIDs)
	if err != nil {
		return nil, fmt.Errorf("look up already-linked players: %w", err)
	}

	espnIDs := make([]int64, 0, len(rows))
	for _, sp := range rows {
		if id, err := strconv.ParseInt(sp.EspnID, 10, 64); err == nil && id != 0 {
			espnIDs = append(espnIDs, id)
		}
	}
	espnMatches, err := models.GetPlayersByESPNIDs(db, espnIDs)
	if err != nil {
		return nil, fmt.Errorf("look up players by espn_id: %w", err)
	}

	for _, sp := range rows {
		heartbeat()
		if _, ok := alreadyLinked[sp.SleeperPlayerID]; ok {
			continue
		}
		espnID, _ := strconv.ParseInt(sp.EspnID, 10, 64)
		outcome, c, err := a.linkOrCreatePlayer(db, sp, espnID, espnMatches)
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

// linkOrCreatePlayer resolves one Sleeper player against the (already
// batch-fetched) espnMatches map. It returns a non-nil conflict instead of
// writing when the matched players row is already claimed by a different
// Sleeper player; otherwise it links or creates and reports which. espnID
// may be negative (team defenses) or 0 (no known ESPN mapping) — only 0 is
// treated as "no match," matching espn_id's "unset" sentinel convention.
func (a *PlayerSyncActivities) linkOrCreatePlayer(db *gorm.DB, sp models.SleeperPlayer, espnID int64, espnMatches map[int64]models.Player) (skillOutcome, *identityConflict, error) {
	if espnID != 0 {
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
	// db is a shared per-batch transaction (see syncIdentityBatch) — a
	// uniqueness conflict here would otherwise abort every other row's work
	// already done in this batch, so the create attempt is wrapped in its
	// own savepoint and rolled back to on conflict instead of aborting the
	// whole transaction.
	const createSavepoint = "player_identity_create_attempt"
	if err := db.SavePoint(createSavepoint).Error; err != nil {
		return 0, nil, fmt.Errorf("savepoint before create for sleeper_id %s: %w", sp.SleeperPlayerID, err)
	}
	if err := db.Create(&newPlayer).Error; err != nil {
		if IsUniqueViolation(err) {
			if rbErr := db.RollbackTo(createSavepoint).Error; rbErr != nil {
				return 0, nil, fmt.Errorf("rollback to savepoint after create conflict for sleeper_id %s: %w", sp.SleeperPlayerID, rbErr)
			}
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
