package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/helpers"
	"backend/internal/models"
)

// ScavengerActivities holds dependencies for the archive scavenger's
// replicate-phase activities: Cloud is the hot 30-day store, Archive is the
// full-history store. Only the worker constructs this, and only when
// ARCHIVE_DATABASE_URL is set — see cmd/worker/main.go.
type ScavengerActivities struct {
	Cloud   *gorm.DB
	Archive *gorm.DB
}

// scavengerSafetyLag bounds every replicate query's upper timestamp edge.
// Guards against reading a row whose insert/update transaction hasn't
// become visible yet under concurrent writers — without it, a keyset cursor
// could advance past a timestamp before a concurrently-committing row at
// that same timestamp becomes visible, silently skipping it forever.
const scavengerSafetyLag = 5 * time.Minute

const (
	streamLeagues      = "sleeper_leagues"
	streamTransactions = "sleeper_transactions"
	streamDraftHeaders = "sleeper_drafts_headers"
	streamDraftPicks   = "sleeper_drafts_picks"
)

// GetScavengerConfig returns the scavenger's tuning knobs from env, clamped
// to at least 1 so a bad value can't stall replication or break a query's
// LIMIT. PurgeEnabled has no clamp — it's a bool kill-switch, not a size.
func (a *ScavengerActivities) GetScavengerConfig(ctx context.Context) (ScavengerConfig, error) {
	return ScavengerConfig{
		LeagueBatchSize:  max(helpers.GetEnv("SCAVENGER_LEAGUE_BATCH_SIZE", 500), 1),
		TxnBatchSize:     max(helpers.GetEnv("SCAVENGER_TXN_BATCH_SIZE", 5000), 1),
		DraftBatchSize:   max(helpers.GetEnv("SCAVENGER_DRAFT_BATCH_SIZE", 200), 1),
		MaxBatchesPerRun: max(helpers.GetEnv("SCAVENGER_MAX_BATCHES_PER_RUN", 50), 1),
		RetentionDays:    max(helpers.GetEnv("SCAVENGER_RETENTION_DAYS", 30), 1),
		PurgeEnabled:     helpers.GetEnv("SCAVENGER_PURGE_ENABLED", true),
	}, nil
}

// cursor is the keyset position for one replicate stream: every stream
// orders by (timestamp, id) and stores its progress as this same shape in
// archive_sync_state.cursor_state.
type cursor struct {
	Time time.Time `json:"time"`
	ID   string    `json:"id"`
}

// readCursor loads stream's cursor from archive_sync_state. A missing row
// (first run) returns the zero cursor, which naturally selects everything
// on the first batch since every real timestamp is after time.Time{}.
func readCursor(ctx context.Context, archive *gorm.DB, stream string) (cursor, error) {
	var row struct {
		CursorState json.RawMessage `gorm:"column:cursor_state"`
	}
	err := archive.WithContext(ctx).
		Table("archive_sync_state").
		Select("cursor_state").
		Where("stream = ?", stream).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return cursor{}, nil
	}
	if err != nil {
		return cursor{}, err
	}
	var c cursor
	if err := json.Unmarshal(row.CursorState, &c); err != nil {
		return cursor{}, fmt.Errorf("unmarshal cursor for stream %s: %w", stream, err)
	}
	return c, nil
}

// writeCursor upserts stream's cursor inside tx, so the cursor advance
// commits atomically with the rows it describes: a crash between the two
// would otherwise risk the cursor moving past rows that were never actually
// written. Callers must run this inside the same transaction as the batch's
// row upserts.
func writeCursor(tx *gorm.DB, stream string, c cursor) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return tx.Exec(
		`INSERT INTO archive_sync_state (stream, cursor_state, updated_at) VALUES (?, ?, now())
		 ON CONFLICT (stream) DO UPDATE SET cursor_state = excluded.cursor_state, updated_at = excluded.updated_at`,
		stream, data,
	).Error
}

const selectLeaguesBatchSQL = `
SELECT sleeper_league_id, name, season, sport, status, total_rosters, ppr, te_premium, is_superflex,
       draft_type, league_type, scoring_settings, roster_positions, created_at, updated_at
FROM sleeper_leagues
WHERE (updated_at, sleeper_league_id) > (?, ?)
  AND updated_at <= ?
ORDER BY updated_at, sleeper_league_id
LIMIT ?`

// ReplicateLeaguesBatch copies up to BatchSize leagues from cloud to archive,
// ordered by (updated_at, sleeper_league_id), and advances the leagues
// cursor. Leagues are replicated because the ADP rollup (T7) joins drafts to
// leagues for league_type/ppr/total_rosters/is_superflex — nothing else in
// the archive currently needs sleeper_leagues, but it's small and this join
// dependency is enough to justify replicating it in full.
func (a *ScavengerActivities) ReplicateLeaguesBatch(ctx context.Context, params ReplicateBatchParams) (ReplicateBatchResult, error) {
	cur, err := readCursor(ctx, a.Archive, streamLeagues)
	if err != nil {
		return ReplicateBatchResult{}, err
	}

	var rows []models.SleeperLeague
	if err := a.Cloud.WithContext(ctx).Raw(selectLeaguesBatchSQL,
		cur.Time, cur.ID, time.Now().UTC().Add(-scavengerSafetyLag), params.BatchSize,
	).Scan(&rows).Error; err != nil {
		return ReplicateBatchResult{}, err
	}
	if len(rows) == 0 {
		return ReplicateBatchResult{Drained: true}, nil
	}

	archiveRows := make([]models.ArchiveSleeperLeague, len(rows))
	for i, r := range rows {
		archiveRows[i] = models.ArchiveSleeperLeague{
			SleeperLeagueID: r.SleeperLeagueID, Name: r.Name, Season: r.Season, Sport: r.Sport,
			Status: r.Status, TotalRosters: r.TotalRosters, PPR: r.PPR, TEPremium: r.TEPremium,
			IsSuperflex: r.IsSuperflex, DraftType: r.DraftType, LeagueType: r.LeagueType,
			ScoringSettings: r.ScoringSettings, RosterPositions: r.RosterPositions,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	}
	last := rows[len(rows)-1]
	newCursor := cursor{Time: last.UpdatedAt, ID: last.SleeperLeagueID}

	err = a.Archive.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "sleeper_league_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name", "season", "sport", "status", "total_rosters", "ppr", "te_premium",
				"is_superflex", "draft_type", "league_type", "scoring_settings", "roster_positions", "updated_at",
			}),
		}).CreateInBatches(archiveRows, 500).Error; err != nil {
			return err
		}
		return writeCursor(tx, streamLeagues, newCursor)
	})
	if err != nil {
		return ReplicateBatchResult{}, err
	}
	return ReplicateBatchResult{Replicated: len(rows), Drained: len(rows) < params.BatchSize}, nil
}

const selectDraftHeadersBatchSQL = `
SELECT d.sleeper_draft_id, d.sleeper_league_id, d.type, d.status, d.season, d.last_fetched_at, d.created_at, d.updated_at
FROM sleeper_drafts d
JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id
WHERE l.league_type = 'redraft'
  AND (d.created_at, d.sleeper_draft_id) > (?, ?)
  AND d.created_at <= ?
ORDER BY d.created_at, d.sleeper_draft_id
LIMIT ?`

// ReplicateDraftHeadersBatch copies up to BatchSize draft rows from cloud to
// archive, ordered by (created_at, sleeper_draft_id) — this catches new
// drafts as they're first created. It does not catch later status changes on
// an existing draft (sleeper_drafts.updated_at is dead — never assigned by
// the upsert in data_fetch.go); those are caught separately, once picks
// land, by ReplicateDraftPicksBatch's last_fetched_at watermark. Joined to
// sleeper_leagues to exclude keeper/dynasty leagues — same redraft-only
// filter as claimLeaguesForDraftsSQL (data_fetch.go), kept here too as
// defense-in-depth for the case archive is disabled and later re-enabled
// (this path is otherwise dead in normal operation since T15 routes all
// drafts straight to archive, never through cloud).
func (a *ScavengerActivities) ReplicateDraftHeadersBatch(ctx context.Context, params ReplicateBatchParams) (ReplicateBatchResult, error) {
	cur, err := readCursor(ctx, a.Archive, streamDraftHeaders)
	if err != nil {
		return ReplicateBatchResult{}, err
	}

	var rows []models.SleeperDraft
	if err := a.Cloud.WithContext(ctx).Raw(selectDraftHeadersBatchSQL,
		cur.Time, cur.ID, time.Now().UTC().Add(-scavengerSafetyLag), params.BatchSize,
	).Scan(&rows).Error; err != nil {
		return ReplicateBatchResult{}, err
	}
	if len(rows) == 0 {
		return ReplicateBatchResult{Drained: true}, nil
	}

	archiveRows := make([]models.ArchiveSleeperDraft, len(rows))
	for i, r := range rows {
		archiveRows[i] = models.ArchiveSleeperDraft{
			SleeperDraftID: r.SleeperDraftID, SleeperLeagueID: r.SleeperLeagueID, Type: r.Type,
			Status: r.Status, Season: r.Season, LastFetchedAt: r.LastFetchedAt,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	}
	last := rows[len(rows)-1]
	newCursor := cursor{Time: last.CreatedAt, ID: last.SleeperDraftID}

	err = a.Archive.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "sleeper_draft_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"sleeper_league_id", "type", "status", "season", "last_fetched_at", "updated_at",
			}),
		}).CreateInBatches(archiveRows, 500).Error; err != nil {
			return err
		}
		return writeCursor(tx, streamDraftHeaders, newCursor)
	})
	if err != nil {
		return ReplicateBatchResult{}, err
	}
	return ReplicateBatchResult{Replicated: len(rows), Drained: len(rows) < params.BatchSize}, nil
}

const selectDraftsByPicksWatermarkSQL = `
SELECT d.sleeper_draft_id, d.sleeper_league_id, d.type, d.status, d.season, d.last_fetched_at, d.created_at, d.updated_at
FROM sleeper_drafts d
JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id
WHERE l.league_type = 'redraft'
  AND d.last_fetched_at IS NOT NULL
  AND (d.last_fetched_at, d.sleeper_draft_id) > (?, ?)
  AND d.last_fetched_at <= ?
ORDER BY d.last_fetched_at, d.sleeper_draft_id
LIMIT ?`

// ReplicateDraftPicksBatch copies up to BatchSize drafts (plus all of their
// picks) from cloud to archive, watermarked on sleeper_drafts.last_fetched_at
// — the signal that picks have landed (set once, in data_fetch.go's
// fetchDraftPicks). This also re-copies the draft row itself, so by the time
// a draft's picks are replicated its status is current too (picks are only
// fetched once a draft reaches "complete"). Joined to sleeper_leagues to
// exclude keeper/dynasty leagues — see selectDraftHeadersBatchSQL.
func (a *ScavengerActivities) ReplicateDraftPicksBatch(ctx context.Context, params ReplicateBatchParams) (ReplicateBatchResult, error) {
	cur, err := readCursor(ctx, a.Archive, streamDraftPicks)
	if err != nil {
		return ReplicateBatchResult{}, err
	}

	var drafts []models.SleeperDraft
	if err := a.Cloud.WithContext(ctx).Raw(selectDraftsByPicksWatermarkSQL,
		cur.Time, cur.ID, time.Now().UTC().Add(-scavengerSafetyLag), params.BatchSize,
	).Scan(&drafts).Error; err != nil {
		return ReplicateBatchResult{}, err
	}
	if len(drafts) == 0 {
		return ReplicateBatchResult{Drained: true}, nil
	}

	draftIDs := make([]string, len(drafts))
	archiveDrafts := make([]models.ArchiveSleeperDraft, len(drafts))
	for i, d := range drafts {
		draftIDs[i] = d.SleeperDraftID
		archiveDrafts[i] = models.ArchiveSleeperDraft{
			SleeperDraftID: d.SleeperDraftID, SleeperLeagueID: d.SleeperLeagueID, Type: d.Type,
			Status: d.Status, Season: d.Season, LastFetchedAt: d.LastFetchedAt,
			CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
		}
	}

	var picks []models.SleeperDraftPick
	if err := a.Cloud.WithContext(ctx).Where("sleeper_draft_id IN ?", draftIDs).Find(&picks).Error; err != nil {
		return ReplicateBatchResult{}, err
	}
	archivePicks := make([]models.ArchiveSleeperDraftPick, len(picks))
	for i, p := range picks {
		archivePicks[i] = models.ArchiveSleeperDraftPick{
			SleeperDraftID: p.SleeperDraftID, Round: p.Round, PickNo: p.PickNo, RosterID: p.RosterID,
			PickedByUserID: p.PickedByUserID, SleeperPlayerID: p.SleeperPlayerID, Metadata: p.Metadata,
		}
	}

	last := drafts[len(drafts)-1]
	newCursor := cursor{Time: *last.LastFetchedAt, ID: last.SleeperDraftID}

	err = a.Archive.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "sleeper_draft_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"sleeper_league_id", "type", "status", "season", "last_fetched_at", "updated_at",
			}),
		}).CreateInBatches(archiveDrafts, 500).Error; err != nil {
			return err
		}
		if len(archivePicks) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
				CreateInBatches(archivePicks, 500).Error; err != nil {
				return err
			}
		}
		return writeCursor(tx, streamDraftPicks, newCursor)
	})
	if err != nil {
		return ReplicateBatchResult{}, err
	}
	return ReplicateBatchResult{Replicated: len(drafts), Drained: len(drafts) < params.BatchSize}, nil
}

const selectTransactionsBatchSQL = `
SELECT sleeper_transaction_id, sleeper_league_id, type, status, created_at_sleeper, leg,
       adds, drops, draft_picks, waiver_budget, created_at
FROM sleeper_transactions
WHERE (created_at, sleeper_transaction_id) > (?, ?)
  AND created_at <= ?
ORDER BY created_at, sleeper_transaction_id
LIMIT ?`

// ReplicateTransactionsBatch copies up to BatchSize transactions from cloud
// to archive, ordered by (created_at, sleeper_transaction_id). Transactions
// are insert-only and immutable in cloud, so the archive upsert is
// DoNothing on conflict — a replay can never need to overwrite a row.
func (a *ScavengerActivities) ReplicateTransactionsBatch(ctx context.Context, params ReplicateBatchParams) (ReplicateBatchResult, error) {
	cur, err := readCursor(ctx, a.Archive, streamTransactions)
	if err != nil {
		return ReplicateBatchResult{}, err
	}

	var rows []models.SleeperTransaction
	if err := a.Cloud.WithContext(ctx).Raw(selectTransactionsBatchSQL,
		cur.Time, cur.ID, time.Now().UTC().Add(-scavengerSafetyLag), params.BatchSize,
	).Scan(&rows).Error; err != nil {
		return ReplicateBatchResult{}, err
	}
	if len(rows) == 0 {
		return ReplicateBatchResult{Drained: true}, nil
	}

	// Skip rows carrying draft picks or FAAB — the valuation model never reads
	// them (analysis/src/parsing.py parse_trade), so they're not worth archive
	// space. Filtered-out rows still advance the cursor below: cursor position
	// tracks how far into cloud we've scanned, not what got written.
	var archiveRows []models.ArchiveSleeperTransaction
	for _, r := range rows {
		if !isPlayerOnlyTransaction(r.DraftPicks, r.WaiverBudget) {
			continue
		}
		archiveRows = append(archiveRows, models.ArchiveSleeperTransaction{
			SleeperTransactionID: r.SleeperTransactionID, SleeperLeagueID: r.SleeperLeagueID,
			Type: r.Type, Status: r.Status, CreatedAtSleeper: r.CreatedAtSleeper, Leg: r.Leg,
			Adds: r.Adds, Drops: r.Drops, DraftPicks: r.DraftPicks, WaiverBudget: r.WaiverBudget,
			CreatedAt: r.CreatedAt,
		})
	}
	last := rows[len(rows)-1]
	newCursor := cursor{Time: last.CreatedAt, ID: last.SleeperTransactionID}

	err = a.Archive.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(archiveRows) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
				CreateInBatches(archiveRows, 500).Error; err != nil {
				return err
			}
		}
		return writeCursor(tx, streamTransactions, newCursor)
	})
	if err != nil {
		return ReplicateBatchResult{}, err
	}
	return ReplicateBatchResult{Replicated: len(rows), Drained: len(rows) < params.BatchSize}, nil
}

// purgeDeleteChunkSize caps each purge delete transaction so a single
// batch's worth of deletes (up to a few thousand rows) doesn't hold row
// locks on the hot cloud tables for one long transaction while the API is
// serving reads.
const purgeDeleteChunkSize = 500

// purgeCandidate is one row eligible for purge consideration: its ID and the
// timestamp used both to order the scan and to report the alarm age when the
// row can't be verified.
type purgeCandidate struct {
	ID        string    `gorm:"column:id"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// splitVerifiedCandidates partitions candidates (ordered oldest-first) into
// IDs safe to delete (present in verified) and a count/oldest-timestamp of
// the rest. Because candidates are ordered ascending by CreatedAt, the first
// unverified row encountered is the oldest unverified row in the whole
// candidate set, not just this batch.
func splitVerifiedCandidates(candidates []purgeCandidate, verified map[string]bool) (toDelete []string, unverifiedCount int, oldestUnverified *time.Time) {
	for _, c := range candidates {
		if verified[c.ID] {
			toDelete = append(toDelete, c.ID)
			continue
		}
		unverifiedCount++
		if oldestUnverified == nil {
			t := c.CreatedAt
			oldestUnverified = &t
		}
	}
	return toDelete, unverifiedCount, oldestUnverified
}

// deleteInChunks runs deleteFn against ids in chunks of purgeDeleteChunkSize,
// each in its own short transaction, so a purge batch never holds one long
// transaction's worth of row locks on a hot cloud table.
func deleteInChunks(ctx context.Context, db *gorm.DB, ids []string, deleteFn func(tx *gorm.DB, chunk []string) error) error {
	for i := 0; i < len(ids); i += purgeDeleteChunkSize {
		chunk := ids[i:min(i+purgeDeleteChunkSize, len(ids))]
		if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return deleteFn(tx, chunk)
		}); err != nil {
			return err
		}
	}
	return nil
}

// checkUnverifiedAlarm returns an error when oldest — the oldest unverified
// candidate's timestamp seen in a purge batch — is older than
// retentionDays+15d. That means a row has sat unpurgeable for two full
// scavenger cycles past retention: replication has stalled, not just
// lagged. Unlike a replicate stream's swallowed failure, this error is
// meant to fail the activity (and the workflow run) so Temporal shows a red
// run — the intended stalled-replication alarm.
func checkUnverifiedAlarm(stream string, oldest *time.Time, retentionDays int) error {
	if oldest == nil {
		return nil
	}
	alarmAge := time.Duration(retentionDays+15) * 24 * time.Hour
	if age := time.Since(*oldest); age > alarmAge {
		return fmt.Errorf("scavenger purge: oldest unverified %s row is %s old (retention %dd + 15d alarm threshold) — replication appears stalled",
			stream, age.Round(time.Hour), retentionDays)
	}
	return nil
}

const selectPurgeTransactionCandidatesSQL = `
SELECT sleeper_transaction_id AS id, created_at
FROM sleeper_transactions
WHERE created_at_sleeper < ?
ORDER BY created_at_sleeper, sleeper_transaction_id
LIMIT ?`

// PurgeTransactionsBatch deletes up to BatchSize of the oldest cloud
// transactions — oldest by created_at_sleeper (Sleeper's own event
// timestamp), not by created_at (when we happened to insert the row) — that
// are older than RetentionDays and verified present in the archive. Using
// event time means a freshly-backfilled old transaction (e.g. a newly
// discovered league's history) is purge-eligible as soon as it's verified,
// not 30 days after whenever it happened to be inserted. Unverified rows
// (not yet replicated) are left in place — the next batch/run naturally
// retries them since only verified rows are ever deleted. Returns an error
// (see checkUnverifiedAlarm) if the oldest unverified row's insert time is
// past retention+15d — that alarm intentionally stays on the insert-time
// clock (it's tracking replication lag, not event age).
func (a *ScavengerActivities) PurgeTransactionsBatch(ctx context.Context, params PurgeBatchParams) (PurgeBatchResult, error) {
	cutoffMs := time.Now().UTC().AddDate(0, 0, -params.RetentionDays).UnixMilli()

	var candidates []purgeCandidate
	if err := a.Cloud.WithContext(ctx).Raw(selectPurgeTransactionCandidatesSQL, cutoffMs, params.BatchSize).
		Scan(&candidates).Error; err != nil {
		return PurgeBatchResult{}, err
	}
	if len(candidates) == 0 {
		return PurgeBatchResult{Drained: true}, nil
	}

	ids := make([]string, len(candidates))
	for i, c := range candidates {
		ids[i] = c.ID
	}

	var archiveIDs []string
	if err := a.Archive.WithContext(ctx).Table("sleeper_transactions").
		Where("sleeper_transaction_id IN ?", ids).
		Pluck("sleeper_transaction_id", &archiveIDs).Error; err != nil {
		return PurgeBatchResult{}, err
	}
	verified := make(map[string]bool, len(archiveIDs))
	for _, id := range archiveIDs {
		verified[id] = true
	}

	toDelete, unverifiedCount, oldestUnverified := splitVerifiedCandidates(candidates, verified)

	if len(toDelete) > 0 {
		if err := deleteInChunks(ctx, a.Cloud, toDelete, func(tx *gorm.DB, chunk []string) error {
			return tx.Where("sleeper_transaction_id IN ?", chunk).Delete(&models.SleeperTransaction{}).Error
		}); err != nil {
			return PurgeBatchResult{}, err
		}
	}

	if err := checkUnverifiedAlarm(streamTransactions, oldestUnverified, params.RetentionDays); err != nil {
		return PurgeBatchResult{}, err
	}

	return PurgeBatchResult{
		Purged:     len(toDelete),
		Unverified: unverifiedCount,
		Drained:    len(candidates) < params.BatchSize,
	}, nil
}

const selectPurgeDraftCandidatesSQL = `
SELECT d.sleeper_draft_id AS id, d.created_at
FROM sleeper_drafts d
JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id
WHERE d.season < ?
  AND l.status IN ('in_season', 'complete')
  AND l.last_drafts_fetched_at IS NOT NULL
ORDER BY d.season, d.sleeper_draft_id
LIMIT ?`

// pickCountsByDraft returns sleeper_draft_id -> pick count for draftIDs,
// used by PurgeDraftsBatch to verify pick-count parity between cloud and
// archive before deleting. A draft absent from the result has zero picks.
func pickCountsByDraft(ctx context.Context, db *gorm.DB, draftIDs []string) (map[string]int, error) {
	var rows []struct {
		SleeperDraftID string `gorm:"column:sleeper_draft_id"`
		Count          int    `gorm:"column:count"`
	}
	if err := db.WithContext(ctx).Table("sleeper_draft_picks").
		Select("sleeper_draft_id, count(*) as count").
		Where("sleeper_draft_id IN ?", draftIDs).
		Group("sleeper_draft_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(rows))
	for _, r := range rows {
		counts[r.SleeperDraftID] = r.Count
	}
	return counts, nil
}

// PurgeDraftsBatch deletes up to BatchSize of the oldest cloud drafts (and
// their picks) whose season is before the current season (see
// currentSleeperSeason in data_fetch.go) and whose owning league satisfies
// the claim-pool-exclusion predicate — status IN ('in_season','complete')
// AND last_drafts_fetched_at IS NOT NULL, the same condition that
// permanently excludes a league from ClaimLeaguesForDrafts
// (data_fetch.go:43-54). Purging a draft whose league could still be
// re-claimed would let syncOneLeagueDrafts recreate the header with
// last_fetched_at = NULL and trigger a full pick-refetch loop.
//
// Eligibility is season-based, not insert-time-based: drafts have no
// per-row date, so "season" is the age proxy (same convention T13's ingest
// routing uses). RetentionDays no longer affects eligibility here — a
// season only ends once a year, so there's no day-granularity retention
// concept to apply — but it's still passed to checkUnverifiedAlarm for the
// stalled-replication threshold, which stays anchored to insert time.
//
// A draft is verified only when its header is present in the archive AND
// its cloud and archive pick counts match exactly. Unverified drafts are
// left in place — the next batch/run retries them. Picks are deleted before
// the draft header (FK, no ON DELETE CASCADE in the cloud schema).
func (a *ScavengerActivities) PurgeDraftsBatch(ctx context.Context, params PurgeBatchParams) (PurgeBatchResult, error) {
	cutoffSeason := currentSleeperSeason()

	var candidates []purgeCandidate
	if err := a.Cloud.WithContext(ctx).Raw(selectPurgeDraftCandidatesSQL, cutoffSeason, params.BatchSize).
		Scan(&candidates).Error; err != nil {
		return PurgeBatchResult{}, err
	}
	if len(candidates) == 0 {
		return PurgeBatchResult{Drained: true}, nil
	}

	ids := make([]string, len(candidates))
	for i, c := range candidates {
		ids[i] = c.ID
	}

	var archiveDraftIDs []string
	if err := a.Archive.WithContext(ctx).Table("sleeper_drafts").
		Where("sleeper_draft_id IN ?", ids).
		Pluck("sleeper_draft_id", &archiveDraftIDs).Error; err != nil {
		return PurgeBatchResult{}, err
	}
	headerPresent := make(map[string]bool, len(archiveDraftIDs))
	for _, id := range archiveDraftIDs {
		headerPresent[id] = true
	}

	cloudPickCounts, err := pickCountsByDraft(ctx, a.Cloud, ids)
	if err != nil {
		return PurgeBatchResult{}, err
	}
	archivePickCounts, err := pickCountsByDraft(ctx, a.Archive, ids)
	if err != nil {
		return PurgeBatchResult{}, err
	}

	verified := make(map[string]bool, len(ids))
	for _, id := range ids {
		if headerPresent[id] && cloudPickCounts[id] == archivePickCounts[id] {
			verified[id] = true
		}
	}

	toDelete, unverifiedCount, oldestUnverified := splitVerifiedCandidates(candidates, verified)

	if len(toDelete) > 0 {
		if err := deleteInChunks(ctx, a.Cloud, toDelete, func(tx *gorm.DB, chunk []string) error {
			if err := tx.Where("sleeper_draft_id IN ?", chunk).Delete(&models.SleeperDraftPick{}).Error; err != nil {
				return err
			}
			return tx.Where("sleeper_draft_id IN ?", chunk).Delete(&models.SleeperDraft{}).Error
		}); err != nil {
			return PurgeBatchResult{}, err
		}
	}

	if err := checkUnverifiedAlarm("sleeper_drafts", oldestUnverified, params.RetentionDays); err != nil {
		return PurgeBatchResult{}, err
	}

	return PurgeBatchResult{
		Purged:     len(toDelete),
		Unverified: unverifiedCount,
		Drained:    len(candidates) < params.BatchSize,
	}, nil
}
