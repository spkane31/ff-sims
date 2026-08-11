-- +goose Up
-- +goose NO TRANSACTION

-- Persists each trade's per-side valuation total, computed and written by
-- transactioncron (internal/valuation.ComputeTradeValues) at sync time and
-- backfilled by its reconcile sweep — see
-- docs/superpowers/specs/2026-08-10-trade-valuation-totals-design.md.
-- Shape: {"<roster_id>": <float>, ...}, present only for roster IDs where
-- every player on that side has a fresh valuation. Cloud-only; the archive
-- DB copy of sleeper_transactions does not get this column.
ALTER TABLE sleeper_transactions ADD COLUMN IF NOT EXISTS trade_values JSONB;

-- Keeps the reconcile sweep's `WHERE trade_values IS NULL` cheap regardless
-- of table size — mirrors idx_sleeper_transactions_trade_complete's
-- partial-index pattern for the same table.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_transactions_trade_values_null
  ON sleeper_transactions (created_at_sleeper)
  WHERE type = 'trade' AND status = 'complete' AND trade_values IS NULL;

-- +goose Down
-- +goose NO TRANSACTION

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_transactions_trade_values_null;
ALTER TABLE sleeper_transactions DROP COLUMN IF EXISTS trade_values;
