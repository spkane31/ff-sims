-- +goose Up
-- +goose NO TRANSACTION

ALTER TABLE sleeper_leagues ADD COLUMN IF NOT EXISTS discovery_claimed_at timestamptz;

-- Serves the claim query in ClaimStaleLeagues (internal/discoverycron): filter
-- on the stale-leagues predicate (never-fetched leagues dominate the eligible
-- set at any time), order never-fetched first then oldest. NULLS FIRST
-- matches the query's ORDER BY exactly so the sort is an index walk.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_leagues_discovery_stale
    ON sleeper_leagues (last_fetched_at ASC NULLS FIRST)
    WHERE skipped_at IS NULL AND season >= '2025';

-- +goose Down
-- +goose NO TRANSACTION

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_leagues_discovery_stale;
ALTER TABLE sleeper_leagues DROP COLUMN IF EXISTS discovery_claimed_at;
