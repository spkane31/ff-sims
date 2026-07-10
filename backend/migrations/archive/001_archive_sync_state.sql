-- +goose Up

CREATE TABLE archive_sync_state (
    stream text PRIMARY KEY,
    cursor_state jsonb NOT NULL DEFAULT '{}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down

DROP TABLE archive_sync_state;
