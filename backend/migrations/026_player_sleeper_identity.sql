-- +goose Up

-- Adds a Sleeper identity to the players table so /players/:id can be
-- resolved from either an ESPN ID (existing) or a Sleeper player ID.
-- espn_id already used 0 as its "unset" sentinel (NOT NULL DEFAULT 0); the
-- old plain unique index therefore allowed at most one such row. Switching
-- both id columns to partial unique indexes (excluding their sentinel/empty
-- value) lets many identity-only rows created from sleeper_players coexist
-- without an ESPN match, while still enforcing uniqueness among real IDs.
ALTER TABLE players ADD COLUMN sleeper_id text NOT NULL DEFAULT '';

DROP INDEX IF EXISTS idx_players_espn_id;
CREATE UNIQUE INDEX idx_players_espn_id ON players (espn_id) WHERE espn_id <> 0;
CREATE UNIQUE INDEX idx_players_sleeper_id ON players (sleeper_id) WHERE sleeper_id <> '';

-- +goose Down

DROP INDEX IF EXISTS idx_players_sleeper_id;
DROP INDEX IF EXISTS idx_players_espn_id;
CREATE UNIQUE INDEX idx_players_espn_id ON players (espn_id);
ALTER TABLE players DROP COLUMN sleeper_id;
