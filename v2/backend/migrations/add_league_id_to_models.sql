-- Migration: Add league_id to models for multi-league support
-- Created: 2026-01-07

-- ============================================
-- Add league_id columns to tables
-- ============================================

-- Add league_id to boxscores
ALTER TABLE boxscores ADD COLUMN league_id INTEGER;

-- Add league_id to sim_results
ALTER TABLE sim_results ADD COLUMN league_id INTEGER;

-- Add league_id to sim_team_results
ALTER TABLE sim_team_results ADD COLUMN league_id INTEGER;

-- Add league_id to team_name_histories
ALTER TABLE team_name_histories ADD COLUMN league_id INTEGER;

-- ============================================
-- Add foreign key constraints
-- ============================================

ALTER TABLE boxscores
  ADD CONSTRAINT fk_boxscores_league
  FOREIGN KEY (league_id) REFERENCES leagues(id);

ALTER TABLE sim_results
  ADD CONSTRAINT fk_sim_results_league
  FOREIGN KEY (league_id) REFERENCES leagues(id);

ALTER TABLE sim_team_results
  ADD CONSTRAINT fk_sim_team_results_league
  FOREIGN KEY (league_id) REFERENCES leagues(id);

ALTER TABLE team_name_histories
  ADD CONSTRAINT fk_team_name_histories_league
  FOREIGN KEY (league_id) REFERENCES leagues(id);

-- ============================================
-- Backfill league_id from parent relationships
-- ============================================

-- Backfill boxscores.league_id from matchups
UPDATE boxscores
SET league_id = (
  SELECT league_id
  FROM matchups
  WHERE matchups.id = boxscores.matchup_id
)
WHERE league_id IS NULL;

-- Backfill sim_results.league_id from simulations
UPDATE sim_results
SET league_id = (
  SELECT league_id
  FROM simulations
  WHERE simulations.id = sim_results.simulation_id
)
WHERE league_id IS NULL;

-- Backfill sim_team_results.league_id from simulations
UPDATE sim_team_results
SET league_id = (
  SELECT league_id
  FROM simulations
  WHERE simulations.id = sim_team_results.simulation_id
)
WHERE league_id IS NULL;

-- Backfill team_name_histories.league_id from teams
UPDATE team_name_histories
SET league_id = (
  SELECT league_id
  FROM teams
  WHERE teams.id = team_name_histories.team_id
)
WHERE league_id IS NULL;

-- ============================================
-- Create indexes for performance
-- ============================================

-- Boxscores indexes
CREATE INDEX idx_boxscores_league_id ON boxscores(league_id);
CREATE INDEX idx_boxscores_league_matchup ON boxscores(league_id, matchup_id);
CREATE INDEX idx_boxscores_league_team ON boxscores(league_id, team_id);

-- Sim results indexes
CREATE INDEX idx_sim_results_league_id ON sim_results(league_id);
CREATE INDEX idx_sim_results_league_simulation ON sim_results(league_id, simulation_id);

-- Sim team results indexes
CREATE INDEX idx_sim_team_results_league_id ON sim_team_results(league_id);
CREATE INDEX idx_sim_team_results_league_simulation ON sim_team_results(league_id, simulation_id);

-- Team name histories indexes
CREATE INDEX idx_team_name_histories_league_id ON team_name_histories(league_id);
CREATE INDEX idx_team_name_histories_league_team ON team_name_histories(league_id, team_id);

-- ============================================
-- Additional composite indexes for common queries
-- ============================================

-- Matchups: optimize year/week queries by league
CREATE INDEX IF NOT EXISTS idx_matchups_league_year_week ON matchups(league_id, year, week);
CREATE INDEX IF NOT EXISTS idx_matchups_league_completed ON matchups(league_id, completed);

-- Teams: optimize team queries by league
CREATE INDEX IF NOT EXISTS idx_teams_league_id ON teams(league_id);

-- Transactions: optimize transaction queries by league
CREATE INDEX IF NOT EXISTS idx_transactions_league_id ON transactions(league_id);
CREATE INDEX IF NOT EXISTS idx_draft_selections_league_year ON draft_selections(league_id, year);

-- Expected wins: optimize by league and year
CREATE INDEX IF NOT EXISTS idx_weekly_expected_wins_league_year ON weekly_expected_wins(league_id, year);
CREATE INDEX IF NOT EXISTS idx_season_expected_wins_league_year ON season_expected_wins(league_id, year);

-- ============================================
-- Set NOT NULL constraints after backfill
-- ============================================

ALTER TABLE boxscores ALTER COLUMN league_id SET NOT NULL;
ALTER TABLE sim_results ALTER COLUMN league_id SET NOT NULL;
ALTER TABLE sim_team_results ALTER COLUMN league_id SET NOT NULL;
ALTER TABLE team_name_histories ALTER COLUMN league_id SET NOT NULL;
