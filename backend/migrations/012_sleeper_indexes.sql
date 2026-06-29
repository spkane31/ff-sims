-- +goose Up
-- +goose NO TRANSACTION

-- Partial index covering the common trade query pattern. Stores rows pre-sorted
-- by created_at_sleeper DESC so ORDER BY + LIMIT can skip the 10M+ other rows.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_transactions_trade_complete
    ON sleeper_transactions (created_at_sleeper DESC)
    WHERE type = 'trade' AND status = 'complete';

-- FK-style index so the JOIN to sleeper_leagues is a cheap lookup, not a seq scan.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_transactions_league_id
    ON sleeper_transactions (sleeper_league_id);

-- General (type, status) index covers GetSleeperTransactions with type filters
-- and any future queries filtering on these columns.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_transactions_type_status
    ON sleeper_transactions (type, status);

-- Partial index for completed-draft counting and filtering.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_drafts_status_complete
    ON sleeper_drafts (sleeper_draft_id)
    WHERE status = 'complete';

-- Index so the LEFT JOIN in GetSleeperDrafts doesn't seq-scan draft_picks.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_draft_picks_draft_id
    ON sleeper_draft_picks (sleeper_draft_id);

-- Partial index for the leagues COUNT in GetSleeperStats.
-- Allows an index-only scan instead of touching 134k heap rows.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_leagues_fetched_nn
    ON sleeper_leagues (sleeper_league_id)
    WHERE last_fetched_at IS NOT NULL;

-- +goose Down

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_leagues_fetched_nn;
DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_draft_picks_draft_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_drafts_status_complete;
DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_transactions_type_status;
DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_transactions_league_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_transactions_trade_complete;
