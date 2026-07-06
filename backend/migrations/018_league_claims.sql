-- +goose Up
-- +goose NO TRANSACTION

ALTER TABLE sleeper_leagues ADD COLUMN IF NOT EXISTS claimed_at timestamptz;

-- Serves the claim query in ClaimLeaguesForTransactions: filter on the stale-
-- transactions predicate, order never-fetched first then oldest. NULLS FIRST
-- matches the query's ORDER BY exactly so the sort is an index walk.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_leagues_txn_stale
    ON sleeper_leagues (last_transactions_fetched_at ASC NULLS FIRST)
    WHERE skipped_at IS NULL AND last_fetched_at IS NOT NULL AND season >= '2025';

-- +goose Down
-- +goose NO TRANSACTION

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_leagues_txn_stale;
ALTER TABLE sleeper_leagues DROP COLUMN IF EXISTS claimed_at;
