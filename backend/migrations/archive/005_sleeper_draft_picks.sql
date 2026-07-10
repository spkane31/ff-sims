-- +goose Up

CREATE TABLE sleeper_draft_picks (
    sleeper_draft_id text NOT NULL,
    round integer NOT NULL,
    pick_no integer NOT NULL,
    roster_id integer,
    picked_by_user_id text,
    sleeper_player_id text,
    metadata jsonb,
    PRIMARY KEY (sleeper_draft_id, round, pick_no)
);

CREATE INDEX IF NOT EXISTS idx_archive_sleeper_draft_picks_draft_id
    ON sleeper_draft_picks (sleeper_draft_id);

-- +goose Down

DROP TABLE sleeper_draft_picks;
