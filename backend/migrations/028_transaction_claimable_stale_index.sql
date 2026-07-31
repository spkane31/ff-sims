-- +goose Up
-- +goose NO TRANSACTION

-- The original transaction-stale index also contains completed leagues that
-- have already had their transactions synced. Those terminal rows sort ahead
-- of current work, forcing each claim to scan and filter a large historical
-- cohort before it can satisfy its LIMIT. Keep this predicate aligned with
-- ClaimLeaguesForTransactions so the index begins at genuinely claimable
-- leagues (never transaction-fetched first, then oldest fetch).
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_leagues_txn_claimable_stale
    ON sleeper_leagues (last_transactions_fetched_at ASC NULLS FIRST)
    WHERE skipped_at IS NULL
      AND last_fetched_at IS NOT NULL
      AND season >= '2025'
      AND NOT (status = 'complete' AND last_transactions_fetched_at IS NOT NULL);

-- Leave the existing index available until the replacement has finished
-- building, then remove it so writes do not maintain both indexes forever.
DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_leagues_txn_stale;

-- +goose Down
-- +goose NO TRANSACTION

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_leagues_txn_stale
    ON sleeper_leagues (last_transactions_fetched_at ASC NULLS FIRST)
    WHERE skipped_at IS NULL AND last_fetched_at IS NOT NULL AND season >= '2025';

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_leagues_txn_claimable_stale;
