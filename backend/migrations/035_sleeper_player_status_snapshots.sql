-- +goose Up
-- One row per status capture. capture_id is shared with the projection batch
-- when the observations come from the pregame projection response (#229), so
-- both streams can be joined on the same fetch.
CREATE TABLE sleeper_status_captures (
    capture_id        TEXT        PRIMARY KEY,
    source            TEXT        NOT NULL,
    observed_season   TEXT,
    observed_week     INT,
    observed_at       TIMESTAMPTZ NOT NULL,
    observation_count INT         NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sleeper_status_captures_source_observed
    ON sleeper_status_captures (source, observed_at DESC);

-- What Sleeper reported for a player at observed_at. NULL injury_status means
-- Sleeper supplied no designation, never "healthy". applicability stays
-- 'unknown' unless verified evidence ties the designation to a game or week.
CREATE TABLE sleeper_player_status_observations (
    capture_id         TEXT        NOT NULL REFERENCES sleeper_status_captures (capture_id) ON DELETE CASCADE,
    sleeper_player_id  TEXT        NOT NULL,
    source             TEXT        NOT NULL,
    observed_season    TEXT,
    observed_week      INT,
    observed_at        TIMESTAMPTZ NOT NULL,
    nfl_team           TEXT,
    roster_status      TEXT,
    injury_status      TEXT,
    injury_start_date  TEXT,
    news_updated_ms    BIGINT,
    applicability      TEXT        NOT NULL DEFAULT 'unknown',
    applicable_game_id TEXT,
    applicable_week    INT,
    PRIMARY KEY (capture_id, sleeper_player_id),
    CONSTRAINT sleeper_player_status_applicability_valid
        CHECK (applicability IN ('unknown', 'game', 'week')),
    CONSTRAINT sleeper_player_status_game_applicability
        CHECK ((applicability = 'game') = (applicable_game_id IS NOT NULL)),
    CONSTRAINT sleeper_player_status_week_applicability
        CHECK ((applicability = 'week') = (applicable_week IS NOT NULL))
);

CREATE INDEX idx_sleeper_player_status_player_observed
    ON sleeper_player_status_observations (sleeper_player_id, observed_at DESC);

-- +goose Down
DROP TABLE sleeper_player_status_observations;
DROP TABLE sleeper_status_captures;
