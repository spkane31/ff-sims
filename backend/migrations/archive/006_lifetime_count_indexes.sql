-- +goose Up

-- Supports ScavengerActivities.UpdateLifetimeCounts's per-run COUNT queries
-- against the archive DB (the full-history store), mirroring cloud's
-- idx_sleeper_transactions_trade_complete / idx_sleeper_drafts_status_complete
-- (migrations/012_sleeper_indexes.sql).
CREATE INDEX IF NOT EXISTS idx_archive_sleeper_transactions_trade_complete
    ON sleeper_transactions (sleeper_transaction_id)
    WHERE type = 'trade' AND status = 'complete';

CREATE INDEX IF NOT EXISTS idx_archive_sleeper_drafts_status_complete
    ON sleeper_drafts (sleeper_draft_id)
    WHERE status = 'complete';

-- +goose Down

DROP INDEX IF EXISTS idx_archive_sleeper_drafts_status_complete;
DROP INDEX IF EXISTS idx_archive_sleeper_transactions_trade_complete;
