-- +goose Up

CREATE TABLE sleeper_drafts (
    sleeper_draft_id text PRIMARY KEY,
    sleeper_league_id text,
    type text,
    status text,
    season text,
    last_fetched_at timestamptz,
    created_at timestamptz,
    updated_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_archive_sleeper_drafts_created_at
    ON sleeper_drafts (created_at, sleeper_draft_id);
CREATE INDEX IF NOT EXISTS idx_archive_sleeper_drafts_last_fetched_at
    ON sleeper_drafts (last_fetched_at, sleeper_draft_id);

-- +goose Down

DROP TABLE sleeper_drafts;
