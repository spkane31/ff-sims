-- Create season_expected_wins table for final season totals
CREATE TABLE IF NOT EXISTS season_expected_wins (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    
    -- Identifiers
    team_id INTEGER NOT NULL,
    year INTEGER NOT NULL,
    league_id INTEGER NOT NULL,
    final_week INTEGER NOT NULL, -- Last regular season week used for calculation
    
    -- Season totals
    expected_wins DECIMAL(10,3) DEFAULT 0,
    expected_losses DECIMAL(10,3) DEFAULT 0,
    actual_wins INTEGER DEFAULT 0,
    actual_losses INTEGER DEFAULT 0,
    win_luck DECIMAL(10,3) DEFAULT 0,
    strength_of_schedule DECIMAL(8,5) DEFAULT 0,
    
    -- Performance metrics
    total_points_for DECIMAL(12,2) DEFAULT 0,
    total_points_against DECIMAL(12,2) DEFAULT 0,
    average_points_for DECIMAL(10,2) DEFAULT 0,
    average_points_against DECIMAL(10,2) DEFAULT 0,
    
    -- Season context
    playoff_made BOOLEAN DEFAULT FALSE,
    final_standing INTEGER DEFAULT 0,
    
    -- Constraints
    UNIQUE (team_id, year),
    
    -- Foreign key constraints
    FOREIGN KEY (team_id) REFERENCES teams(id),
    FOREIGN KEY (league_id) REFERENCES leagues(id)
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_season_expected_wins_team_year ON season_expected_wins (team_id, year);
CREATE INDEX IF NOT EXISTS idx_season_expected_wins_league_year ON season_expected_wins (league_id, year);
CREATE INDEX IF NOT EXISTS idx_season_expected_wins_year ON season_expected_wins (year);
CREATE INDEX IF NOT EXISTS idx_season_expected_wins_expected_wins ON season_expected_wins (expected_wins DESC);
CREATE INDEX IF NOT EXISTS idx_season_expected_wins_deleted_at ON season_expected_wins (deleted_at);

-- Add comments for documentation
COMMENT ON TABLE season_expected_wins IS 'Stores final season expected wins totals calculated from the last regular season week';
COMMENT ON COLUMN season_expected_wins.expected_wins IS 'Final expected wins for the regular season';
COMMENT ON COLUMN season_expected_wins.win_luck IS 'Season total luck factor (actual wins - expected wins)';
COMMENT ON COLUMN season_expected_wins.final_week IS 'Last regular season week used in calculation (excludes playoffs)';
COMMENT ON COLUMN season_expected_wins.playoff_made IS 'Whether this team qualified for playoffs';
COMMENT ON COLUMN season_expected_wins.final_standing IS 'Final league standing position';