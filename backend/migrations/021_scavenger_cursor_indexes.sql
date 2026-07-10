-- +goose Up
-- +goose NO TRANSACTION

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_transactions_created_at
    ON sleeper_transactions (created_at);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_drafts_last_fetched_at
    ON sleeper_drafts (last_fetched_at);

-- +goose Down
-- +goose NO TRANSACTION

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_drafts_last_fetched_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_transactions_created_at;
