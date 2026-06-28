-- +goose Up

CREATE TABLE IF NOT EXISTS espn_league_credentials (
    espn_league_id               TEXT PRIMARY KEY,
    espn_s2                      TEXT NOT NULL,
    swid                         TEXT NOT NULL,
    last_teams_fetched_at        TIMESTAMPTZ,
    last_schedule_fetched_at     TIMESTAMPTZ,
    last_draft_fetched_at        TIMESTAMPTZ,
    last_transactions_fetched_at TIMESTAMPTZ,
    last_players_updated_at      TIMESTAMPTZ,
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS espn_league_credentials;
