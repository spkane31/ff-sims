package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"backend/internal/helpers"
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
