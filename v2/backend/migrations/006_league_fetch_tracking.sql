-- +goose Up

ALTER TABLE sleeper_leagues
    ADD COLUMN IF NOT EXISTS last_drafts_fetched_at       TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_transactions_fetched_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_sleeper_leagues_last_drafts_fetched
    ON sleeper_leagues (last_drafts_fetched_at ASC NULLS FIRST);

CREATE INDEX IF NOT EXISTS idx_sleeper_leagues_last_transactions_fetched
    ON sleeper_leagues (last_transactions_fetched_at ASC NULLS FIRST);

-- +goose Down

DROP INDEX IF EXISTS idx_sleeper_leagues_last_transactions_fetched;
DROP INDEX IF EXISTS idx_sleeper_leagues_last_drafts_fetched;

ALTER TABLE sleeper_leagues
    DROP COLUMN IF EXISTS last_transactions_fetched_at,
    DROP COLUMN IF EXISTS last_drafts_fetched_at;
