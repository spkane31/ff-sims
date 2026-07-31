-- +goose Up

-- Hourly, current-season transaction-sync age distribution. Each hourly
-- snapshot is one row with a zero-filled count for every defined age bucket,
-- written by cmd/cron's "lifetime-counts" job.
CREATE TABLE sleeper_transaction_fetch_age_snapshots (
    snapshot_at timestamptz PRIMARY KEY,
    season text NOT NULL,
    never_fetched bigint NOT NULL DEFAULT 0,
    fetched_within_four_hours bigint NOT NULL DEFAULT 0,
    fetched_four_to_eight_hours bigint NOT NULL DEFAULT 0,
    fetched_eight_to_twelve_hours bigint NOT NULL DEFAULT 0,
    fetched_twelve_to_sixteen_hours bigint NOT NULL DEFAULT 0,
    fetched_sixteen_to_twenty_hours bigint NOT NULL DEFAULT 0,
    fetched_twenty_to_twenty_four_hours bigint NOT NULL DEFAULT 0,
    fetched_twenty_four_or_more_hours bigint NOT NULL DEFAULT 0
);

-- +goose Down

DROP TABLE sleeper_transaction_fetch_age_snapshots;
