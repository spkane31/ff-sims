-- Create weekly_expected_wins table for tracking expected wins progression by week
CREATE TABLE IF NOT EXISTS weekly_expected_wins (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    
    -- Identifiers
    team_id INTEGER NOT NULL,
    week INTEGER NOT NULL,
    year INTEGER NOT NULL,
    league_id INTEGER NOT NULL,
    
    -- Expected wins data
    expected_wins DECIMAL(10,3) DEFAULT 0,         -- Cumulative through this week
    weekly_expected_wins DECIMAL(10,3) DEFAULT 0,  -- Just this week
    expected_losses DECIMAL(10,3) DEFAULT 0,       -- Cumulative losses
    weekly_expected_losses DECIMAL(10,3) DEFAULT 0, -- Just this week losses
    
    -- Actual performance
    actual_wins INTEGER DEFAULT 0,      -- Cumulative through this week
    actual_losses INTEGER DEFAULT 0,    -- Cumulative losses
    weekly_actual_win BOOLEAN DEFAULT FALSE, -- Did they win this week
    
    -- Metrics
    win_luck DECIMAL(10,3) DEFAULT 0,              -- ActualWins - ExpectedWins
    strength_of_schedule DECIMAL(8,5) DEFAULT 0,  -- Average opponent strength
    weekly_win_probability DECIMAL(8,5) DEFAULT 0, -- Win probability for this week's matchup
    
    -- Performance context
    team_score DECIMAL(10,2) DEFAULT 0,         -- Team's score this week
    opponent_score DECIMAL(10,2) DEFAULT 0,     -- Opponent's score this week
    opponent_team_id INTEGER,                    -- Who they played
    point_differential DECIMAL(10,2) DEFAULT 0, -- TeamScore - OpponentScore
    
    -- Constraints and indexes
    UNIQUE (team_id, week, year),
    
    -- Foreign key constraints
    FOREIGN KEY (team_id) REFERENCES teams(id),
    FOREIGN KEY (opponent_team_id) REFERENCES teams(id),
    FOREIGN KEY (league_id) REFERENCES leagues(id)
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_weekly_expected_wins_team_week_year ON weekly_expected_wins (team_id, week, year);
CREATE INDEX IF NOT EXISTS idx_weekly_expected_wins_league_week_year ON weekly_expected_wins (league_id, week, year);
CREATE INDEX IF NOT EXISTS idx_weekly_expected_wins_year_week ON weekly_expected_wins (year, week);
CREATE INDEX IF NOT EXISTS idx_weekly_expected_wins_deleted_at ON weekly_expected_wins (deleted_at);

-- Add comments for documentation
COMMENT ON TABLE weekly_expected_wins IS 'Stores expected wins calculations for each team, week, and year for progression tracking';
COMMENT ON COLUMN weekly_expected_wins.expected_wins IS 'Cumulative expected wins through this week';
COMMENT ON COLUMN weekly_expected_wins.weekly_expected_wins IS 'Expected wins gained just this week';
COMMENT ON COLUMN weekly_expected_wins.win_luck IS 'Difference between actual wins and expected wins (positive = lucky)';
COMMENT ON COLUMN weekly_expected_wins.strength_of_schedule IS 'Average opponent strength faced through this week';
COMMENT ON COLUMN weekly_expected_wins.weekly_win_probability IS 'Calculated win probability for this specific week matchup';