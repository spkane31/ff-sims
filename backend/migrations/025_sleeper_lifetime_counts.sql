-- +goose Up

-- Hourly history of data-scraping table sizes, snapshotted by cmd/cron's
-- "lifetime-counts" job. One row per hour; see models.SleeperLifetimeCount.
-- transactions_total/trades_completed/drafts_completed are nullable — they
-- come from the archive DB and are left NULL (not 0) for any snapshot taken
-- while no archive DB is configured.
CREATE TABLE sleeper_lifetime_counts (
    snapshot_at timestamptz PRIMARY KEY,

    users_total    bigint NOT NULL DEFAULT 0,
    users_expanded bigint NOT NULL DEFAULT 0,
    users_pending  bigint NOT NULL DEFAULT 0,
    users_skipped  bigint NOT NULL DEFAULT 0,

    leagues_total    bigint NOT NULL DEFAULT 0,
    leagues_expanded bigint NOT NULL DEFAULT 0,
    leagues_pending  bigint NOT NULL DEFAULT 0,
    leagues_skipped  bigint NOT NULL DEFAULT 0,

    transactions_total bigint,
    trades_completed   bigint,
    drafts_completed   bigint
);

-- +goose Down

DROP TABLE sleeper_lifetime_counts;
