-- +goose Up

-- Operator-supplied NFL kickoffs, one row per team per game. verified rows are
-- confirmed from the official NFL schedule; provisional rows only record the
-- expected time and never make a capture replay-eligible.
CREATE TABLE nfl_kickoffs (
    season      TEXT        NOT NULL,
    week        INT         NOT NULL,
    team        TEXT        NOT NULL,
    opponent    TEXT        NOT NULL,
    kickoff_at  TIMESTAMPTZ NOT NULL,
    verified    BOOLEAN     NOT NULL DEFAULT FALSE,
    source      TEXT        NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (season, week, team)
);

CREATE INDEX idx_nfl_kickoffs_verified_kickoff ON nfl_kickoffs (kickoff_at) WHERE verified;

-- One published Sleeper projection pull. A capture and its rows commit in one
-- transaction; capture_key makes a retried activity in the same workflow run
-- return the existing capture instead of writing a second one.
CREATE TABLE sleeper_projection_captures (
    id                 BIGSERIAL   PRIMARY KEY,
    capture_key        TEXT        NOT NULL UNIQUE,
    season             TEXT        NOT NULL,
    target_week        INT         NOT NULL,
    captured_at        TIMESTAMPTZ NOT NULL,
    source             TEXT        NOT NULL,
    trigger            TEXT        NOT NULL,
    window_kickoff_at  TIMESTAMPTZ,
    week_phase         TEXT        NOT NULL,
    replay_eligible    BOOLEAN,
    replay_note        TEXT        NOT NULL DEFAULT '',
    row_count          INT         NOT NULL,
    company_counts     JSONB       NOT NULL,
    missing_companies  JSONB       NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sleeper_projection_captures_week ON sleeper_projection_captures (season, target_week, captured_at DESC);

-- Game IDs seen in a capture and the UTC kickoff cutoff each was judged
-- against, so a late snapshot cannot pass for a pregame one.
CREATE TABLE sleeper_projection_capture_games (
    capture_id        BIGINT      NOT NULL REFERENCES sleeper_projection_captures(id) ON DELETE CASCADE,
    game_id           TEXT        NOT NULL,
    kickoff_at        TIMESTAMPTZ,
    kickoff_verified  BOOLEAN     NOT NULL,
    replay_eligible   BOOLEAN,
    PRIMARY KEY (capture_id, game_id)
);

-- A player absent from a capture has no row. A row with row_kind
-- 'metadata_only' was present but carried no projection, and null point
-- totals mean the provider did not publish them (an explicit 0 stays 0).
CREATE TABLE sleeper_player_projections (
    capture_id              BIGINT      NOT NULL REFERENCES sleeper_projection_captures(id) ON DELETE CASCADE,
    sleeper_player_id       TEXT        NOT NULL,
    company                 TEXT        NOT NULL,
    row_kind                TEXT        NOT NULL CHECK (row_kind IN ('projection', 'metadata_only')),
    position                TEXT        NOT NULL,
    nfl_team                TEXT        NOT NULL DEFAULT '',
    opponent                TEXT        NOT NULL DEFAULT '',
    game_id                 TEXT        NOT NULL DEFAULT '',
    provider_last_modified  TIMESTAMPTZ,
    pts_ppr                 FLOAT,
    pts_half_ppr            FLOAT,
    pts_std                 FLOAT,
    stats                   JSONB,
    PRIMARY KEY (capture_id, sleeper_player_id, company)
);

CREATE INDEX idx_sleeper_player_projections_player ON sleeper_player_projections (sleeper_player_id, company);

-- Player status as seen in the same current-week projection response,
-- sharing the capture's ID and observation time. Consumed by #231.
CREATE TABLE sleeper_player_status_observations (
    capture_id         BIGINT      NOT NULL REFERENCES sleeper_projection_captures(id) ON DELETE CASCADE,
    sleeper_player_id  TEXT        NOT NULL,
    observed_at        TIMESTAMPTZ NOT NULL,
    source             TEXT        NOT NULL,
    nfl_team           TEXT        NOT NULL DEFAULT '',
    injury_status      TEXT        NOT NULL DEFAULT '',
    injury_start_date  TEXT        NOT NULL DEFAULT '',
    news_updated_at    TIMESTAMPTZ,
    PRIMARY KEY (capture_id, sleeper_player_id)
);

CREATE INDEX idx_sleeper_player_status_observations_player ON sleeper_player_status_observations (sleeper_player_id, observed_at DESC);

-- +goose Down

DROP TABLE IF EXISTS sleeper_player_status_observations;
DROP TABLE IF EXISTS sleeper_player_projections;
DROP TABLE IF EXISTS sleeper_projection_capture_games;
DROP TABLE IF EXISTS sleeper_projection_captures;
DROP TABLE IF EXISTS nfl_kickoffs;
