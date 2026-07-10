-- +goose Up

CREATE TABLE sleeper_transactions (
    sleeper_transaction_id text PRIMARY KEY,
    sleeper_league_id text,
    type text,
    status text,
    created_at_sleeper bigint,
    leg integer,
    adds jsonb,
    drops jsonb,
    draft_picks jsonb,
    waiver_budget jsonb,
    created_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_archive_sleeper_transactions_created_at
    ON sleeper_transactions (created_at, sleeper_transaction_id);

-- +goose Down

DROP TABLE sleeper_transactions;
