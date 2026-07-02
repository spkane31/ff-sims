-- +goose Up

CREATE TABLE sleeper_player_week_stats (
    season             TEXT NOT NULL,
    week               INT  NOT NULL,
    sleeper_player_id  TEXT NOT NULL,
    pts_ppr            FLOAT,
    pts_half_ppr       FLOAT,
    pts_std            FLOAT,
    stats              JSONB,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (season, week, sleeper_player_id)
);

CREATE TABLE sleeper_week_stat_fetches (
    season           TEXT NOT NULL,
    week             INT  NOT NULL,
    last_fetched_at  TIMESTAMPTZ,
    finalized        BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (season, week)
);

-- +goose Down

DROP TABLE IF EXISTS sleeper_week_stat_fetches;
DROP TABLE IF EXISTS sleeper_player_week_stats;
