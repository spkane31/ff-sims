-- +goose Up

-- Scavenger-maintained all-time totals, immune to the sleeper_transactions /
-- sleeper_drafts purge window. See models.SleeperLifetimeCount.
CREATE TABLE sleeper_lifetime_counts (
    metric text PRIMARY KEY,
    count bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down

DROP TABLE sleeper_lifetime_counts;
