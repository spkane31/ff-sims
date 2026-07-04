-- +goose Up

ALTER TABLE draft_adp
    ADD COLUMN ci_low_pick_no  NUMERIC NOT NULL DEFAULT 0,
    ADD COLUMN ci_high_pick_no NUMERIC NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE draft_adp
    DROP COLUMN IF EXISTS ci_low_pick_no,
    DROP COLUMN IF EXISTS ci_high_pick_no;
