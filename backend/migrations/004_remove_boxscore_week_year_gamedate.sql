-- +goose Up
ALTER TABLE box_scores DROP COLUMN IF EXISTS week;
ALTER TABLE box_scores DROP COLUMN IF EXISTS year;
ALTER TABLE box_scores DROP COLUMN IF EXISTS game_date;

-- +goose Down
ALTER TABLE box_scores ADD COLUMN IF NOT EXISTS week bigint NOT NULL DEFAULT 0;
ALTER TABLE box_scores ADD COLUMN IF NOT EXISTS year bigint NOT NULL DEFAULT 0;
ALTER TABLE box_scores ADD COLUMN IF NOT EXISTS game_date timestamptz;
