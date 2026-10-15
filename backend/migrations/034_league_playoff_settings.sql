-- +goose Up
CREATE TABLE league_playoff_settings (
    league_id bigint NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    year integer NOT NULL,
    playoff_team_count integer NOT NULL,
    round_weeks jsonb NOT NULL,
    PRIMARY KEY (league_id, year)
);

-- +goose Down
DROP TABLE league_playoff_settings;
