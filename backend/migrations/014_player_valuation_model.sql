-- +goose Up

-- Rebuild player_valuations: the old columns (raw_trade_value, recency_factor,
-- age_curve_factor, adjusted_value) were from an earlier valuation design that
-- was never implemented; the table is empty and has no writers.
DROP TABLE player_valuations;

CREATE TABLE player_valuations (
    segment            TEXT NOT NULL,
    sleeper_player_id  TEXT NOT NULL REFERENCES sleeper_players(sleeper_player_id),
    valuation_date     DATE NOT NULL,
    rank               INT,
    value              FLOAT,
    vorp               FLOAT,
    sd                 FLOAT,
    games              FLOAT,
    position           TEXT,
    PRIMARY KEY (segment, sleeper_player_id, valuation_date)
);

CREATE INDEX idx_player_valuations_segment_date
    ON player_valuations (segment, valuation_date);

CREATE TABLE valuation_state (
    segment            TEXT NOT NULL,
    sleeper_player_id  TEXT NOT NULL,
    guess              FLOAT NOT NULL,
    var                FLOAT NOT NULL,
    games              FLOAT NOT NULL DEFAULT 0,
    cum_par            FLOAT NOT NULL DEFAULT 0,
    position           TEXT,
    name               TEXT,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (segment, sleeper_player_id)
);

CREATE TABLE valuation_runs (
    segment                   TEXT NOT NULL,
    season                    TEXT NOT NULL,
    last_event_ts             TIMESTAMPTZ,
    last_transaction_created  BIGINT NOT NULL DEFAULT 0,
    last_week_processed       INT NOT NULL DEFAULT 0,
    last_run_at               TIMESTAMPTZ,
    PRIMARY KEY (segment, season)
);

-- +goose Down

DROP TABLE IF EXISTS valuation_runs;
DROP TABLE IF EXISTS valuation_state;
DROP TABLE IF EXISTS player_valuations;

CREATE TABLE player_valuations (
    sleeper_player_id  TEXT  REFERENCES sleeper_players(sleeper_player_id),
    valuation_date     DATE,
    raw_trade_value    FLOAT,
    recency_factor     FLOAT,
    age_curve_factor   FLOAT,
    adjusted_value     FLOAT,
    PRIMARY KEY (sleeper_player_id, valuation_date)
);
