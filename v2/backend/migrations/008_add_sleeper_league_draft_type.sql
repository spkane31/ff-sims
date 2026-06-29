-- +goose Up

ALTER TABLE sleeper_leagues
    ADD COLUMN IF NOT EXISTS draft_type TEXT;

CREATE INDEX IF NOT EXISTS idx_sleeper_leagues_total_rosters
    ON sleeper_leagues (total_rosters);

CREATE INDEX IF NOT EXISTS idx_sleeper_leagues_ppr
    ON sleeper_leagues (ppr);

CREATE INDEX IF NOT EXISTS idx_sleeper_leagues_draft_type
    ON sleeper_leagues (draft_type);

-- +goose Down

DROP INDEX IF EXISTS idx_sleeper_leagues_draft_type;
DROP INDEX IF EXISTS idx_sleeper_leagues_ppr;
DROP INDEX IF EXISTS idx_sleeper_leagues_total_rosters;

ALTER TABLE sleeper_leagues
    DROP COLUMN IF EXISTS draft_type;
