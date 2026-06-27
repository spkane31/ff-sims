-- +goose Up

ALTER TABLE sleeper_leagues
    ADD COLUMN IF NOT EXISTS last_transaction_leg_fetched INT;

-- +goose Down

ALTER TABLE sleeper_leagues
    DROP COLUMN IF EXISTS last_transaction_leg_fetched;
