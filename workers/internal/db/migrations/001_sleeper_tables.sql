-- +goose Up

CREATE TABLE sleeper_users (
    sleeper_user_id  TEXT        PRIMARY KEY,
    username         TEXT,
    display_name     TEXT,
    avatar           TEXT,
    last_fetched_at  TIMESTAMPTZ,
    skipped_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sleeper_users_last_fetched ON sleeper_users (last_fetched_at ASC NULLS FIRST);

CREATE TABLE sleeper_leagues (
    sleeper_league_id  TEXT     PRIMARY KEY,
    name               TEXT,
    season             TEXT,
    sport              TEXT,
    status             TEXT,
    total_rosters      INT,
    ppr                FLOAT,
    te_premium         FLOAT,
    is_superflex       BOOLEAN,
    scoring_settings   JSONB,
    roster_positions   JSONB,
    last_fetched_at    TIMESTAMPTZ,
    skipped_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sleeper_leagues_last_fetched ON sleeper_leagues (last_fetched_at ASC NULLS FIRST);

CREATE TABLE sleeper_league_users (
    sleeper_league_id  TEXT REFERENCES sleeper_leagues(sleeper_league_id),
    sleeper_user_id    TEXT REFERENCES sleeper_users(sleeper_user_id),
    PRIMARY KEY (sleeper_league_id, sleeper_user_id)
);

CREATE TABLE sleeper_players (
    sleeper_player_id  TEXT     PRIMARY KEY,
    espn_id            TEXT,
    yahoo_id           TEXT,
    full_name          TEXT,
    position           TEXT,
    nfl_team           TEXT,
    age                INT,
    years_exp          INT,
    last_fetched_at    TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sleeper_drafts (
    sleeper_draft_id   TEXT     PRIMARY KEY,
    sleeper_league_id  TEXT     REFERENCES sleeper_leagues(sleeper_league_id),
    type               TEXT,
    status             TEXT,
    season             TEXT,
    last_fetched_at    TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sleeper_draft_picks (
    sleeper_draft_id    TEXT  REFERENCES sleeper_drafts(sleeper_draft_id),
    round               INT,
    pick_no             INT,
    roster_id           INT,
    picked_by_user_id   TEXT,
    sleeper_player_id   TEXT,
    metadata            JSONB,
    PRIMARY KEY (sleeper_draft_id, round, pick_no)
);

CREATE TABLE sleeper_transactions (
    sleeper_transaction_id  TEXT     PRIMARY KEY,
    sleeper_league_id       TEXT     REFERENCES sleeper_leagues(sleeper_league_id),
    type                    TEXT,
    status                  TEXT,
    created_at_sleeper      BIGINT,
    leg                     INT,
    adds                    JSONB,
    drops                   JSONB,
    draft_picks             JSONB,
    waiver_budget           JSONB,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE player_valuations (
    sleeper_player_id  TEXT  REFERENCES sleeper_players(sleeper_player_id),
    valuation_date     DATE,
    raw_trade_value    FLOAT,
    recency_factor     FLOAT,
    age_curve_factor   FLOAT,
    adjusted_value     FLOAT,
    PRIMARY KEY (sleeper_player_id, valuation_date)
);

-- +goose Down

DROP TABLE IF EXISTS player_valuations;
DROP TABLE IF EXISTS sleeper_transactions;
DROP TABLE IF EXISTS sleeper_draft_picks;
DROP TABLE IF EXISTS sleeper_drafts;
DROP TABLE IF EXISTS sleeper_league_users;
DROP TABLE IF EXISTS sleeper_players;
DROP TABLE IF EXISTS sleeper_leagues;
DROP TABLE IF EXISTS sleeper_users;
