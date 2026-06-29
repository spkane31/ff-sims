-- +goose Up

CREATE TABLE IF NOT EXISTS leagues (
    id            bigserial    PRIMARY KEY,
    created_at    timestamptz,
    updated_at    timestamptz,
    deleted_at    timestamptz,
    name          text         NOT NULL DEFAULT '',
    description   text         NOT NULL DEFAULT '',
    scoring_type  text         NOT NULL DEFAULT '',
    season        bigint       NOT NULL DEFAULT 0,
    current_week  bigint       NOT NULL DEFAULT 0,
    total_weeks   bigint       NOT NULL DEFAULT 17,
    playoff_weeks bigint       NOT NULL DEFAULT 3,
    -- RosterSettings (embedded)
    qb            bigint       NOT NULL DEFAULT 1,
    rb            bigint       NOT NULL DEFAULT 2,
    wr            bigint       NOT NULL DEFAULT 2,
    te            bigint       NOT NULL DEFAULT 1,
    flex          bigint       NOT NULL DEFAULT 1,
    k             bigint       NOT NULL DEFAULT 1,
    dst           bigint       NOT NULL DEFAULT 1,
    bn            bigint       NOT NULL DEFAULT 6,
    ir            bigint       NOT NULL DEFAULT 1,
    -- ScoringSettings (embedded)
    passing_yards    double precision NOT NULL DEFAULT 0.04,
    passing_td       double precision NOT NULL DEFAULT 4,
    interception     double precision NOT NULL DEFAULT -2,
    rushing_yards    double precision NOT NULL DEFAULT 0.1,
    rushing_td       double precision NOT NULL DEFAULT 6,
    reception        double precision NOT NULL DEFAULT 0,
    receiving_yards  double precision NOT NULL DEFAULT 0.1,
    receiving_td     double precision NOT NULL DEFAULT 6,
    fumble           double precision NOT NULL DEFAULT -2,
    field_goal0to39  double precision NOT NULL DEFAULT 3,
    field_goal40to49 double precision NOT NULL DEFAULT 4,
    field_goal50plus double precision NOT NULL DEFAULT 5,
    extra_point      double precision NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_leagues_deleted_at ON leagues (deleted_at);

CREATE TABLE IF NOT EXISTS players (
    id              bigserial PRIMARY KEY,
    created_at      timestamptz,
    updated_at      timestamptz,
    deleted_at      timestamptz,
    espn_id         bigint           NOT NULL DEFAULT 0,
    name            text             NOT NULL DEFAULT '',
    position        text             NOT NULL DEFAULT '',
    team            text             NOT NULL DEFAULT '',
    fantasy_points  double precision NOT NULL DEFAULT 0,
    status          text             NOT NULL DEFAULT '',
    -- PlayerStats (embedded)
    passing_yards   double precision NOT NULL DEFAULT 0,
    passing_t_ds    double precision NOT NULL DEFAULT 0,
    interceptions   double precision NOT NULL DEFAULT 0,
    rushing_yards   double precision NOT NULL DEFAULT 0,
    rushing_t_ds    double precision NOT NULL DEFAULT 0,
    receptions      double precision NOT NULL DEFAULT 0,
    receiving_yards double precision NOT NULL DEFAULT 0,
    receiving_t_ds  double precision NOT NULL DEFAULT 0,
    fumbles         double precision NOT NULL DEFAULT 0,
    field_goals     double precision NOT NULL DEFAULT 0,
    extra_points    double precision NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_players_espn_id ON players (espn_id);
CREATE INDEX IF NOT EXISTS idx_players_deleted_at ON players (deleted_at);

CREATE TABLE IF NOT EXISTS teams (
    id          bigserial        PRIMARY KEY,
    created_at  timestamptz,
    updated_at  timestamptz,
    deleted_at  timestamptz,
    name        text             NOT NULL DEFAULT '',
    owner       text             NOT NULL DEFAULT '',
    espn_id     bigint           NOT NULL DEFAULT 0,
    league_id   bigint           NOT NULL DEFAULT 0,
    wins        bigint           NOT NULL DEFAULT 0,
    losses      bigint           NOT NULL DEFAULT 0,
    ties        bigint           NOT NULL DEFAULT 0,
    points      double precision NOT NULL DEFAULT 0,
    year        bigint           NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_teams_espn_id ON teams (espn_id);
CREATE INDEX IF NOT EXISTS idx_teams_deleted_at ON teams (deleted_at);

CREATE TABLE IF NOT EXISTS team_name_histories (
    id          bigserial   PRIMARY KEY,
    created_at  timestamptz,
    updated_at  timestamptz,
    deleted_at  timestamptz,
    team_id     bigint      NOT NULL DEFAULT 0,
    name        text        NOT NULL DEFAULT '',
    start_date  timestamptz NOT NULL DEFAULT now(),
    end_date    timestamptz
);
CREATE INDEX IF NOT EXISTS idx_team_name_histories_deleted_at ON team_name_histories (deleted_at);

CREATE TABLE IF NOT EXISTS team_players (
    player_id bigint NOT NULL,
    team_id   bigint NOT NULL,
    PRIMARY KEY (player_id, team_id)
);

CREATE TABLE IF NOT EXISTS matchups (
    id              bigserial        PRIMARY KEY,
    created_at      timestamptz,
    updated_at      timestamptz,
    deleted_at      timestamptz,
    league_id       bigint           NOT NULL DEFAULT 0,
    week            bigint           NOT NULL DEFAULT 0,
    year            bigint           NOT NULL DEFAULT 0,
    season          bigint           NOT NULL DEFAULT 0,
    home_team_id    bigint           NOT NULL DEFAULT 0,
    away_team_id    bigint           NOT NULL DEFAULT 0,
    game_date       timestamptz,
    game_type       text             NOT NULL DEFAULT '',
    home_team_final_score            double precision NOT NULL DEFAULT 0,
    away_team_final_score            double precision NOT NULL DEFAULT 0,
    home_team_espn_projected_score   double precision NOT NULL DEFAULT 0,
    away_team_espn_projected_score   double precision NOT NULL DEFAULT 0,
    completed       boolean          NOT NULL DEFAULT false,
    is_playoff      boolean          NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS idx_matchups_deleted_at ON matchups (deleted_at);

CREATE TABLE IF NOT EXISTS box_scores (
    id               bigserial        PRIMARY KEY,
    created_at       timestamptz,
    updated_at       timestamptz,
    deleted_at       timestamptz,
    matchup_id       bigint           NOT NULL DEFAULT 0,
    player_id        bigint           NOT NULL DEFAULT 0,
    team_id          bigint           NOT NULL DEFAULT 0,
    week             bigint           NOT NULL DEFAULT 0,
    year             bigint           NOT NULL DEFAULT 0,
    season           bigint           NOT NULL DEFAULT 0,
    game_date        timestamptz,
    started_flag     boolean          NOT NULL DEFAULT false,
    actual_points    double precision NOT NULL DEFAULT 0,
    projected_points double precision NOT NULL DEFAULT 0,
    -- PlayerStats (embedded)
    passing_yards    double precision NOT NULL DEFAULT 0,
    passing_t_ds     double precision NOT NULL DEFAULT 0,
    interceptions    double precision NOT NULL DEFAULT 0,
    rushing_yards    double precision NOT NULL DEFAULT 0,
    rushing_t_ds     double precision NOT NULL DEFAULT 0,
    receptions       double precision NOT NULL DEFAULT 0,
    receiving_yards  double precision NOT NULL DEFAULT 0,
    receiving_t_ds   double precision NOT NULL DEFAULT 0,
    fumbles          double precision NOT NULL DEFAULT 0,
    field_goals      double precision NOT NULL DEFAULT 0,
    extra_points     double precision NOT NULL DEFAULT 0,
    slot_position    text             NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_box_scores_deleted_at ON box_scores (deleted_at);

CREATE TABLE IF NOT EXISTS simulations (
    id              bigserial        PRIMARY KEY,
    created_at      timestamptz,
    updated_at      timestamptz,
    deleted_at      timestamptz,
    league_id       bigint           NOT NULL DEFAULT 0,
    name            text             NOT NULL DEFAULT '',
    description     text             NOT NULL DEFAULT '',
    season          bigint           NOT NULL DEFAULT 0,
    start_week      bigint           NOT NULL DEFAULT 0,
    end_week        bigint           NOT NULL DEFAULT 0,
    num_simulations bigint           NOT NULL DEFAULT 1000,
    completed       boolean          NOT NULL DEFAULT false,
    var_factor      double precision NOT NULL DEFAULT 1.0
);
CREATE INDEX IF NOT EXISTS idx_simulations_deleted_at ON simulations (deleted_at);

CREATE TABLE IF NOT EXISTS sim_results (
    id             bigserial        PRIMARY KEY,
    created_at     timestamptz,
    updated_at     timestamptz,
    deleted_at     timestamptz,
    simulation_id  bigint           NOT NULL DEFAULT 0,
    matchup_id     bigint           NOT NULL DEFAULT 0,
    team_id        bigint           NOT NULL DEFAULT 0,
    opponent_id    bigint           NOT NULL DEFAULT 0,
    score          double precision NOT NULL DEFAULT 0,
    opponent_score double precision NOT NULL DEFAULT 0,
    win            boolean          NOT NULL DEFAULT false,
    sim_run        bigint           NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_sim_results_deleted_at ON sim_results (deleted_at);

CREATE TABLE IF NOT EXISTS sim_team_results (
    id                bigserial        PRIMARY KEY,
    created_at        timestamptz,
    updated_at        timestamptz,
    deleted_at        timestamptz,
    simulation_id     bigint           NOT NULL DEFAULT 0,
    team_id           bigint           NOT NULL DEFAULT 0,
    wins              bigint           NOT NULL DEFAULT 0,
    losses            bigint           NOT NULL DEFAULT 0,
    playoff_odds      double precision NOT NULL DEFAULT 0,
    championship_odds double precision NOT NULL DEFAULT 0,
    avg_points        double precision NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_sim_team_results_deleted_at ON sim_team_results (deleted_at);

CREATE TABLE IF NOT EXISTS transactions (
    id                     bigserial   PRIMARY KEY,
    created_at             timestamptz,
    updated_at             timestamptz,
    deleted_at             timestamptz,
    team_id                bigint      NOT NULL DEFAULT 0,
    player_id              bigint      NOT NULL DEFAULT 0,
    transaction_type       text        NOT NULL DEFAULT '',
    player_name            text        NOT NULL DEFAULT '',
    bid_amount             bigint      NOT NULL DEFAULT 0,
    date                   timestamptz,
    year                   bigint      NOT NULL DEFAULT 0,
    week                   bigint      NOT NULL DEFAULT 0,
    league_id              bigint      NOT NULL DEFAULT 0,
    related_transaction_id bigint,
    trade_partner_team_id  bigint,
    notes                  text        NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_transactions_deleted_at ON transactions (deleted_at);

CREATE TABLE IF NOT EXISTS draft_selections (
    id              bigserial   PRIMARY KEY,
    created_at      timestamptz,
    updated_at      timestamptz,
    deleted_at      timestamptz,
    player_id       bigint      NOT NULL DEFAULT 0,
    player_name     text        NOT NULL DEFAULT '',
    player_position text        NOT NULL DEFAULT '',
    team_id         bigint      NOT NULL DEFAULT 0,
    round           bigint      NOT NULL DEFAULT 0,
    pick            bigint      NOT NULL DEFAULT 0,
    year            bigint      NOT NULL DEFAULT 0,
    league_id       bigint      NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_draft_selections_deleted_at ON draft_selections (deleted_at);

CREATE TABLE IF NOT EXISTS weekly_expected_wins (
    id                     bigserial        PRIMARY KEY,
    created_at             timestamptz,
    updated_at             timestamptz,
    deleted_at             timestamptz,
    team_id                bigint           NOT NULL DEFAULT 0,
    week                   bigint           NOT NULL DEFAULT 0,
    year                   bigint           NOT NULL DEFAULT 0,
    league_id              bigint           NOT NULL DEFAULT 0,
    expected_wins          double precision NOT NULL DEFAULT 0,
    weekly_expected_wins   double precision NOT NULL DEFAULT 0,
    expected_losses        double precision NOT NULL DEFAULT 0,
    weekly_expected_losses double precision NOT NULL DEFAULT 0,
    actual_wins            bigint           NOT NULL DEFAULT 0,
    actual_losses          bigint           NOT NULL DEFAULT 0,
    weekly_actual_win      boolean          NOT NULL DEFAULT false,
    -- legacy column: computed as actual_wins - expected_wins, not written by the app
    win_luck               double precision NOT NULL DEFAULT 0,
    strength_of_schedule   double precision NOT NULL DEFAULT 0,
    weekly_win_probability double precision NOT NULL DEFAULT 0,
    team_score             double precision NOT NULL DEFAULT 0,
    opponent_score         double precision NOT NULL DEFAULT 0,
    opponent_team_id       bigint           NOT NULL DEFAULT 0,
    point_differential     double precision NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_team_week_year ON weekly_expected_wins (team_id, week, year);
CREATE INDEX IF NOT EXISTS idx_league_week_year ON weekly_expected_wins (league_id, week, year);
CREATE INDEX IF NOT EXISTS idx_weekly_expected_wins_deleted_at ON weekly_expected_wins (deleted_at);

CREATE TABLE IF NOT EXISTS season_expected_wins (
    id                      bigserial        PRIMARY KEY,
    created_at              timestamptz,
    updated_at              timestamptz,
    deleted_at              timestamptz,
    team_id                 bigint           NOT NULL DEFAULT 0,
    year                    bigint           NOT NULL DEFAULT 0,
    league_id               bigint           NOT NULL DEFAULT 0,
    final_week              bigint           NOT NULL DEFAULT 0,
    expected_wins           double precision NOT NULL DEFAULT 0,
    expected_losses         double precision NOT NULL DEFAULT 0,
    actual_wins             bigint           NOT NULL DEFAULT 0,
    actual_losses           bigint           NOT NULL DEFAULT 0,
    -- legacy column: computed as actual_wins - expected_wins, not written by the app
    win_luck                double precision NOT NULL DEFAULT 0,
    strength_of_schedule    double precision NOT NULL DEFAULT 0,
    total_points_for        double precision NOT NULL DEFAULT 0,
    total_points_against    double precision NOT NULL DEFAULT 0,
    average_points_for      double precision NOT NULL DEFAULT 0,
    average_points_against  double precision NOT NULL DEFAULT 0,
    playoff_made            boolean          NOT NULL DEFAULT false,
    final_standing          bigint           NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_season_team ON season_expected_wins (team_id, year);
CREATE INDEX IF NOT EXISTS idx_season_expected_wins_deleted_at ON season_expected_wins (deleted_at);

-- +goose Down
DROP TABLE IF EXISTS season_expected_wins;
DROP TABLE IF EXISTS weekly_expected_wins;
DROP TABLE IF EXISTS draft_selections;
DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS sim_team_results;
DROP TABLE IF EXISTS sim_results;
DROP TABLE IF EXISTS simulations;
DROP TABLE IF EXISTS box_scores;
DROP TABLE IF EXISTS matchups;
DROP TABLE IF EXISTS team_players;
DROP TABLE IF EXISTS team_name_histories;
DROP TABLE IF EXISTS teams;
DROP TABLE IF EXISTS players;
DROP TABLE IF EXISTS leagues;
