-- +goose Up

-- The old 029_market_valuation_model migration was deployed before its
-- version was reassigned to 029_valuation_trade_counts. Goose records only
-- versions, so those databases skipped the canonical trade-count migration.
-- Version 030 reconciles the already-applied market-schema changes; add the
-- one missing player_valuations column here.
ALTER TABLE player_valuations ADD COLUMN IF NOT EXISTS trades INT NOT NULL DEFAULT 0;

-- +goose Down

-- Migration 029 owns this column in the canonical history. Keep it while
-- rolling back 031 so migration 029's Down can remove it from both valuation
-- tables after 030 recreates valuation_state.
