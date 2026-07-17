-- +goose Up

-- Hourly history of data-scraping table sizes (users/leagues/transactions/
-- drafts, by discovery state), snapshotted by cmd/cron's "lifetime-counts"
-- job. See models.SleeperLifetimeCount.
CREATE TABLE sleeper_lifetime_counts (
    snapshot_at timestamptz NOT NULL,
    metric text NOT NULL,
    count bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (snapshot_at, metric)
);

-- Serves "growth of metric X over time" queries (chart one metric's history)
-- without a full-table scan.
CREATE INDEX IF NOT EXISTS idx_sleeper_lifetime_counts_metric_snapshot
    ON sleeper_lifetime_counts (metric, snapshot_at DESC);

-- +goose Down

DROP TABLE sleeper_lifetime_counts;
