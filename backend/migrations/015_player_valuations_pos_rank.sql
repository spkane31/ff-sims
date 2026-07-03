-- +goose Up

ALTER TABLE player_valuations
    ADD COLUMN IF NOT EXISTS pos_rank INT;

-- +goose Down

ALTER TABLE player_valuations
    DROP COLUMN IF EXISTS pos_rank;
