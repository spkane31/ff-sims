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
// LIMIT.
func (a *ScavengerActivities) GetScavengerConfig(ctx context.Context) (ScavengerConfig, error) {
	return ScavengerConfig{
		LeagueBatchSize:  max(helpers.GetEnv("SCAVENGER_LEAGUE_BATCH_SIZE", 500), 1),
		TxnBatchSize:     max(helpers.GetEnv("SCAVENGER_TXN_BATCH_SIZE", 5000), 1),
		DraftBatchSize:   max(helpers.GetEnv("SCAVENGER_DRAFT_BATCH_SIZE", 200), 1),
		MaxBatchesPerRun: max(helpers.GetEnv("SCAVENGER_MAX_BATCHES_PER_RUN", 50), 1),
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
SELECT sleeper_draft_id, sleeper_league_id, type, status, season, last_fetched_at, created_at, updated_at
FROM sleeper_drafts
WHERE (created_at, sleeper_draft_id) > (?, ?)
  AND created_at <= ?
ORDER BY created_at, sleeper_draft_id
LIMIT ?`

// ReplicateDraftHeadersBatch copies up to BatchSize draft rows from cloud to
// archive, ordered by (created_at, sleeper_draft_id) — this catches new
// drafts as they're first created. It does not catch later status changes on
// an existing draft (sleeper_drafts.updated_at is dead — never assigned by
// the upsert in data_fetch.go); those are caught separately, once picks
// land, by ReplicateDraftPicksBatch's last_fetched_at watermark.
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

	archiveRows := make([]models.ArchiveSleeperTransaction, len(rows))
	for i, r := range rows {
		archiveRows[i] = models.ArchiveSleeperTransaction{
			SleeperTransactionID: r.SleeperTransactionID, SleeperLeagueID: r.SleeperLeagueID,
			Type: r.Type, Status: r.Status, CreatedAtSleeper: r.CreatedAtSleeper, Leg: r.Leg,
			Adds: r.Adds, Drops: r.Drops, DraftPicks: r.DraftPicks, WaiverBudget: r.WaiverBudget,
			CreatedAt: r.CreatedAt,
		}
	}
	last := rows[len(rows)-1]
	newCursor := cursor{Time: last.CreatedAt, ID: last.SleeperTransactionID}

	err = a.Archive.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(archiveRows, 500).Error; err != nil {
			return err
		}
		return writeCursor(tx, streamTransactions, newCursor)
	})
	if err != nil {
		return ReplicateBatchResult{}, err
	}
	return ReplicateBatchResult{Replicated: len(rows), Drained: len(rows) < params.BatchSize}, nil
}
