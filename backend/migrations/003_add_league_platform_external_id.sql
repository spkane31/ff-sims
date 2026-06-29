-- +goose Up
ALTER TABLE leagues ADD COLUMN IF NOT EXISTS platform    text NOT NULL DEFAULT '';
ALTER TABLE leagues ADD COLUMN IF NOT EXISTS external_id text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_leagues_platform_external_id
    ON leagues (platform, external_id)
    WHERE platform != '' AND external_id != '';

-- Replace global ESPN-ID uniqueness with per-league uniqueness on teams
DROP INDEX IF EXISTS idx_teams_espn_id;
CREATE UNIQUE INDEX IF NOT EXISTS idx_teams_espn_league
    ON teams (espn_id, league_id);

-- +goose Down
DROP INDEX IF EXISTS idx_teams_espn_league;
CREATE UNIQUE INDEX IF NOT EXISTS idx_teams_espn_id ON teams (espn_id);

DROP INDEX IF EXISTS idx_leagues_platform_external_id;
ALTER TABLE leagues DROP COLUMN IF EXISTS external_id;
ALTER TABLE leagues DROP COLUMN IF EXISTS platform;
