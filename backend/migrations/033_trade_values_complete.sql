-- +goose Up
-- +goose NO TRANSACTION

-- Fixes a bug in 032_trade_values.sql's design: ComputeTradeValues wrote a
-- non-NULL trade_values as soon as ANY side of a trade resolved, but
-- ReconcileTradeValues only ever retried rows where trade_values IS NULL —
-- so a trade with one resolved side and one still-pending side got "stuck"
-- with a partial total forever, even after the pending side's player later
-- got a valuation. trade_values_complete tracks per-trade completeness
-- independently of whether trade_values itself is null, so the reconcile
-- sweep can keep retrying a trade until every side that has traded players
-- has a value — see docs/superpowers/specs/2026-08-10-trade-valuation-totals-design.md.
--
-- NOT NULL DEFAULT false means every pre-existing trade row (including ones
-- that already happen to be fully resolved) becomes a reconcile candidate
-- again after this migration lands — a one-time, self-healing re-verification
-- pass bounded by the existing LIMIT/timeout on ReconcileTradeValues, not a
-- separate backfill step.
ALTER TABLE sleeper_transactions ADD COLUMN IF NOT EXISTS trade_values_complete BOOLEAN NOT NULL DEFAULT false;

-- Replaces idx_sleeper_transactions_trade_values_null (032): that index's
-- predicate (trade_values IS NULL) no longer matches ReconcileTradeValues's
-- query, which now gates on trade_values_complete instead.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_transactions_trade_values_incomplete
  ON sleeper_transactions (created_at_sleeper)
  WHERE type = 'trade' AND status = 'complete' AND trade_values_complete = false;

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_transactions_trade_values_null;

-- +goose Down
-- +goose NO TRANSACTION

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_transactions_trade_values_null
  ON sleeper_transactions (created_at_sleeper)
  WHERE type = 'trade' AND status = 'complete' AND trade_values IS NULL;

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_transactions_trade_values_incomplete;

ALTER TABLE sleeper_transactions DROP COLUMN IF EXISTS trade_values_complete;
