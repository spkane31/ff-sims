-- +goose Up

-- How many trades each belief was built from. `games` already records decayed
-- performance evidence; this is the market-evidence counterpart, and without
-- it a consumer cannot tell a value backed by 400 trades from one backed by
-- two. Not decayed, unlike games: the question is how much evidence exists
-- over the whole run, not how recent it is.
ALTER TABLE player_valuations ADD COLUMN trades INT NOT NULL DEFAULT 0;

-- valuation_state carries it too, so Valuator.to_state/from_state stays a
-- lossless round trip for the incremental job this state exists to support.
ALTER TABLE valuation_state ADD COLUMN trades INT NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE valuation_state DROP COLUMN trades;
ALTER TABLE player_valuations DROP COLUMN trades;
