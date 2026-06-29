-- +goose Up

ALTER TABLE sleeper_leagues
    ADD COLUMN IF NOT EXISTS league_type TEXT;

-- Best-effort backfill: dynasty leagues reliably carry TAXI roster slots.
-- Workers will correct edge cases on next normal sync via FetchLeagueDetails.
UPDATE sleeper_leagues
SET league_type = CASE
    WHEN roster_positions::text LIKE '%"TAXI"%' THEN 'dynasty'
    ELSE 'redraft'
END
WHERE last_fetched_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_sleeper_leagues_league_type
    ON sleeper_leagues (league_type);

-- +goose Down

DROP INDEX IF EXISTS idx_sleeper_leagues_league_type;

ALTER TABLE sleeper_leagues
    DROP COLUMN IF EXISTS league_type;
