-- +goose Up

-- The market estimator (analysis/src/market_value.py) replaces the old
-- recursive-belief model outright — the old model was never load-bearing in
-- production and lives on in git history. Same table, same key: each full
-- replay rewrites its window as before.
--
-- New columns:
--   market_score           the fitted trade-market score (the fit's scale;
--                          `value` remains the published curve-at-rank)
--   market_dispersion      robust spread of recent implied trade values
--   projected_par          rest-of-season weekly points-above-replacement proxy
--   projection_uncertainty error band for projected_par
ALTER TABLE player_valuations ADD COLUMN market_score FLOAT;
ALTER TABLE player_valuations ADD COLUMN market_dispersion FLOAT;
ALTER TABLE player_valuations ADD COLUMN projected_par FLOAT;
ALTER TABLE player_valuations ADD COLUMN projection_uncertainty FLOAT;

-- Dead columns from the old model: `vorp` subtracted one global fitted rho
-- from every position (replacement is league- and position-specific at query
-- time now), and `sd` was recursive filter confidence, not calibrated
-- uncertainty — market_dispersion and projection_uncertainty replace it.
ALTER TABLE player_valuations DROP COLUMN vorp;
ALTER TABLE player_valuations DROP COLUMN sd;

-- The old model's incremental belief state and watermarks have no writers or
-- readers left: every market snapshot is a pure function of the staged
-- inputs before it.
DROP TABLE valuation_state;
DROP TABLE valuation_runs;

-- +goose Down

ALTER TABLE player_valuations DROP COLUMN projection_uncertainty;
ALTER TABLE player_valuations DROP COLUMN projected_par;
ALTER TABLE player_valuations DROP COLUMN market_dispersion;
ALTER TABLE player_valuations DROP COLUMN market_score;
ALTER TABLE player_valuations ADD COLUMN vorp FLOAT;
ALTER TABLE player_valuations ADD COLUMN sd FLOAT;

-- As created by migrations 014 + 028.
CREATE TABLE valuation_state (
    segment            TEXT NOT NULL,
    sleeper_player_id  TEXT NOT NULL,
    guess              FLOAT NOT NULL,
    var                FLOAT NOT NULL,
    games              FLOAT NOT NULL DEFAULT 0,
    cum_par            FLOAT NOT NULL DEFAULT 0,
    position           TEXT,
    name               TEXT,
    trades             INT NOT NULL DEFAULT 0,
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
