-- +goose Up
-- +goose NO TRANSACTION

ALTER TABLE sleeper_users ADD COLUMN IF NOT EXISTS claimed_at timestamptz;

-- Serves the claim query in ClaimStaleUsers: filter on the stale-users
-- predicate, order never-fetched first then oldest. NULLS FIRST matches the
-- query's ORDER BY exactly so the sort is an index walk.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_users_stale
    ON sleeper_users (last_fetched_at ASC NULLS FIRST)
    WHERE skipped_at IS NULL;

-- +goose Down
-- +goose NO TRANSACTION

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_users_stale;
ALTER TABLE sleeper_users DROP COLUMN IF EXISTS claimed_at;
