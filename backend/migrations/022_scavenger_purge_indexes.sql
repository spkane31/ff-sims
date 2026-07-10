-- +goose Up
-- +goose NO TRANSACTION

-- Supports the purge candidate scan in PurgeDraftsBatch: WHERE created_at <
-- cutoff ORDER BY created_at, sleeper_draft_id. Mirrors 021's transactions
-- index — sleeper_drafts had no created_at index until now.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_drafts_created_at
    ON sleeper_drafts (created_at);

-- +goose Down
-- +goose NO TRANSACTION

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_drafts_created_at;
