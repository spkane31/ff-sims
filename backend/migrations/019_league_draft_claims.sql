-- +goose Up
-- +goose NO TRANSACTION

-- Separate claim column from transactions' claimed_at so draft and
-- transaction claims never contend for the same rows.
ALTER TABLE sleeper_leagues ADD COLUMN IF NOT EXISTS drafts_claimed_at timestamptz;

-- Serves the claim query in ClaimLeaguesForDrafts: filter on the stale-drafts
-- predicate, order never-fetched first then oldest. NULLS FIRST matches the
-- query's ORDER BY exactly so the sort is an index walk.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_leagues_draft_stale
    ON sleeper_leagues (last_drafts_fetched_at ASC NULLS FIRST)
    WHERE skipped_at IS NULL AND last_fetched_at IS NOT NULL AND season >= '2025';

-- +goose Down
-- +goose NO TRANSACTION

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_leagues_draft_stale;
ALTER TABLE sleeper_leagues DROP COLUMN IF EXISTS drafts_claimed_at;
