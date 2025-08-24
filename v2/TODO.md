# TODO Items

* [ ] Strength-of-schedule measurement and important games coming up

* [ ] Expected wins on a seasonal basis

* [ ] Store and display rosters for each game

* [ ] Show the most important games based on simulation data and which ones change the outcome the most

## Player Ranking Performance Optimization

### Problem
The current position ranking calculation in `GetPlayerByID` is inefficient as it:
1. Fetches all players of the same position
2. Queries box scores for each player to calculate total points
3. Compares totals to determine rank
4. This results in N+1 queries and poor performance

### Proposed Solution: Player Season Stats Table

#### 1. Create `player_season_stats` Table
```sql
CREATE TABLE player_season_stats (
    id SERIAL PRIMARY KEY,
    player_id INTEGER NOT NULL REFERENCES players(id),
    year INTEGER NOT NULL,
    position VARCHAR(10) NOT NULL,
    
    -- Aggregated stats
    games_played INTEGER DEFAULT 0,
    total_fantasy_points DECIMAL(10,2) DEFAULT 0,
    total_projected_points DECIMAL(10,2) DEFAULT 0,
    avg_fantasy_points DECIMAL(10,2) DEFAULT 0,
    
    -- Rankings
    overall_rank INTEGER,
    position_rank INTEGER,
    
    -- Performance metrics
    best_game_points DECIMAL(10,2) DEFAULT 0,
    worst_game_points DECIMAL(10,2) DEFAULT 0,
    consistency_score DECIMAL(5,2), -- Standard deviation or coefficient of variation
    
    -- Timestamps
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(player_id, year),
    INDEX idx_position_year_rank (position, year, position_rank),
    INDEX idx_overall_year_rank (year, overall_rank)
);
```

#### 2. Background Job for Ranking Calculation
Create a scheduled job (cron or background worker) that:
- Runs weekly or after each game week
- Calculates aggregated stats from box_scores table
- Updates position rankings using window functions
- Calculates consistency metrics

Example ranking query:
```sql
WITH player_totals AS (
    SELECT 
        p.id,
        p.position,
        bs.year,
        COUNT(bs.id) as games_played,
        SUM(bs.actual_points) as total_points,
        SUM(bs.projected_points) as total_projected,
        AVG(bs.actual_points) as avg_points,
        MAX(bs.actual_points) as best_game,
        MIN(bs.actual_points) as worst_game,
        STDDEV(bs.actual_points) as std_dev
    FROM players p
    JOIN box_scores bs ON p.id = bs.player_id
    WHERE bs.year = ?
    GROUP BY p.id, p.position, bs.year
),
ranked_players AS (
    SELECT *,
        ROW_NUMBER() OVER (ORDER BY total_points DESC) as overall_rank,
        ROW_NUMBER() OVER (PARTITION BY position ORDER BY total_points DESC) as position_rank
    FROM player_totals
)
INSERT INTO player_season_stats (...) 
SELECT ... FROM ranked_players
ON CONFLICT (player_id, year) DO UPDATE SET ...;
```

#### 3. Benefits
- Single query lookup for player rankings
- Consistent performance regardless of league size
- Enables efficient leaderboard queries
- Supports advanced metrics (consistency, best/worst games)
- Can be extended for historical season comparisons

## Completed

* [X] Data from 2017

* [X] Hall-of-fame / wall-of-shame on the front page showing the winners for each season and the losers

* [X] Create a user-friendly experience on mobile devices, fixing transactions on team detail page

* [X] On the players page, player positions arent showing up correctly

* [X] Include the 3rd place game in totals. Game is between the two losers of the PLAYOFF game of the second to last week.

* [X] Store player data from pre-2024
