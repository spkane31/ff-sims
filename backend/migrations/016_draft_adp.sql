-- +goose Up

CREATE TABLE draft_adp (
    segment           TEXT NOT NULL,
    season            TEXT NOT NULL,
    sleeper_player_id TEXT NOT NULL REFERENCES sleeper_players(sleeper_player_id),
    avg_pick_no       NUMERIC NOT NULL,
    pick_count        INTEGER NOT NULL,
    min_pick_no       INTEGER NOT NULL,
    max_pick_no       INTEGER NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (segment, season, sleeper_player_id)
);

CREATE INDEX idx_draft_adp_segment_season_avg_pick
    ON draft_adp (segment, season, avg_pick_no);

-- +goose Down

DROP TABLE IF EXISTS draft_adp;
