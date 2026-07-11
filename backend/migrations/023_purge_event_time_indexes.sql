-- +goose Up
-- +goose NO TRANSACTION

-- Supports PurgeTransactionsBatch's WHERE created_at_sleeper < ? ORDER BY
-- created_at_sleeper, sleeper_transaction_id. The only existing index on
-- created_at_sleeper (012_sleeper_indexes.sql) is partial — type='trade' AND
-- status='complete' only — and doesn't cover this general scan across every
-- transaction.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_transactions_created_at_sleeper
    ON sleeper_transactions (created_at_sleeper);

-- Supports PurgeDraftsBatch's WHERE season < ? ORDER BY season, sleeper_draft_id.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_drafts_season
    ON sleeper_drafts (season);

-- +goose Down
-- +goose NO TRANSACTION

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_drafts_season;
DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_transactions_created_at_sleeper;
