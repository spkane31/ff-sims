-- +goose Up

CREATE TABLE sleeper_leagues (
    sleeper_league_id text PRIMARY KEY,
    name text,
    season text,
    sport text,
    status text,
    total_rosters integer,
    ppr double precision,
    te_premium double precision,
    is_superflex boolean,
    draft_type text,
    league_type text,
    scoring_settings jsonb,
    roster_positions jsonb,
    created_at timestamptz,
    updated_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_archive_sleeper_leagues_updated_at
    ON sleeper_leagues (updated_at, sleeper_league_id);

-- +goose Down

DROP TABLE sleeper_leagues;
