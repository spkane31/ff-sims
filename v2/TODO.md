# TODO Items

* [ ] Update Github Actions cron job

* [ ] Transactions page

* [ ] Draft data on team detail pagec

* [ ] Strength-of-schedule measurement and important games coming up

* [ ] Expected wins on a seasonal basis - **SCHEDULE GENERATOR COMPLETED**

## Expected Wins on a Seasonal Basis - Proposal

### Overview
The "Expected Wins" feature will calculate how many wins a team "should have" based on their actual scoring performance across randomized season schedules. This provides a more accurate measure of team strength than traditional wins/losses by removing schedule luck.

### Core Concept
For each team, we'll:
1. Take their actual weekly scores from the season
2. Generate thousands of randomized schedules (different opponent matchings)
3. Calculate wins/losses for each randomized schedule
4. Average the results to determine "expected wins"

### Technical Implementation

#### 1. New Database Models

```go
// Expected wins calculation and storage
type ExpectedWinsAnalysis struct {
    ID        uint           `json:"id" gorm:"primarykey"`
    CreatedAt time.Time      `json:"createdAt"`
    UpdatedAt time.Time      `json:"updatedAt"`
    DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

    LeagueID       uint    `json:"league_id"`
    Season         int     `json:"season"`
    WeekCalculated int     `json:"week_calculated"` // Through which week calculated
    NumSimulations int     `json:"num_simulations" gorm:"default:10000"`

    // Relationships
    League      *League                  `json:"-"`
    TeamResults []ExpectedWinsTeamResult `json:"team_results,omitempty"`
}

type ExpectedWinsTeamResult struct {
    ID        uint           `json:"id" gorm:"primarykey"`
    CreatedAt time.Time      `json:"createdAt"`
    UpdatedAt time.Time      `json:"updatedAt"`
    DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

    AnalysisID     uint    `json:"analysis_id"`
    TeamID         uint    `json:"team_id"`
    ActualWins     int     `json:"actual_wins"`
    ExpectedWins   float64 `json:"expected_wins"`
    WinLuck        float64 `json:"win_luck"` // ActualWins - ExpectedWins
    ExpectedRecord string  `json:"expected_record"` // "7.2-6.8" format

    // Week-by-week expected wins progression
    WeeklyExpectedWins []float64 `json:"weekly_expected_wins" gorm:"type:json"`

    // Relationships
    Analysis *ExpectedWinsAnalysis `json:"-"`
    Team     *Team                 `json:"-"`
}
```

#### 2. Calculation Algorithm

```go
func CalculateExpectedWins(leagueID uint, season int, throughWeek int, numSims int) (*ExpectedWinsAnalysis, error) {
    // 1. Get all team scores for each completed week
    teamWeeklyScores := getTeamWeeklyScores(leagueID, season, throughWeek)

    // 2. Generate random schedules and calculate wins
    teamExpectedWins := make(map[uint][]float64) // teamID -> wins per simulation

    for sim := 0; sim < numSims; sim++ {
        // Generate random schedule for this simulation
        randomSchedule := generateRandomSchedule(teamWeeklyScores, throughWeek)

        // Calculate wins for each team in this random schedule
        simWins := calculateWinsForSchedule(randomSchedule, teamWeeklyScores)

        for teamID, wins := range simWins {
            teamExpectedWins[teamID] = append(teamExpectedWins[teamID], wins)
        }
    }

    // 3. Calculate averages and create results
    analysis := &ExpectedWinsAnalysis{
        LeagueID:       leagueID,
        Season:         season,
        WeekCalculated: throughWeek,
        NumSimulations: numSims,
    }

    for teamID, winsArray := range teamExpectedWins {
        avgWins := average(winsArray)
        actualWins := getActualWins(teamID, season, throughWeek)

        result := ExpectedWinsTeamResult{
            TeamID:         teamID,
            ActualWins:     actualWins,
            ExpectedWins:   avgWins,
            WinLuck:        float64(actualWins) - avgWins,
            ExpectedRecord: formatRecord(avgWins, float64(throughWeek)-avgWins),
        }

        analysis.TeamResults = append(analysis.TeamResults, result)
    }

    return analysis, nil
}
```

#### 3. Schedule Generation Strategy

```go
func generateRandomSchedule(teamScores map[uint][]float64, numWeeks int) [][]Matchup {
    teams := getTeamIDs(teamScores)
    schedule := make([][]Matchup, numWeeks)

    for week := 0; week < numWeeks; week++ {
        // Shuffle teams randomly for this week
        shuffledTeams := shuffle(teams)

        // Create pairings (assuming even number of teams)
        for i := 0; i < len(shuffledTeams); i += 2 {
            matchup := Matchup{
                HomeTeamID: shuffledTeams[i],
                AwayTeamID: shuffledTeams[i+1],
                Week:       week + 1,
            }
            schedule[week] = append(schedule[week], matchup)
        }
    }

    return schedule
}

func calculateWinsForSchedule(schedule [][]Matchup, teamScores map[uint][]float64) map[uint]float64 {
    wins := make(map[uint]float64)

    for week, matchups := range schedule {
        for _, matchup := range matchups {
            homeScore := teamScores[matchup.HomeTeamID][week]
            awayScore := teamScores[matchup.AwayTeamID][week]

            if homeScore > awayScore {
                wins[matchup.HomeTeamID]++
            } else {
                wins[matchup.AwayTeamID]++
            }
        }
    }

    return wins
}
```

#### 4. API Endpoints

```go
// GET /api/v1/leagues/{id}/expected-wins/{season}
func GetExpectedWins(c *gin.Context) {
    leagueID := parseUintParam(c, "id")
    season := parseIntParam(c, "season")
    throughWeek := parseIntQuery(c, "through_week", getCurrentWeek())

    analysis, err := GetLatestExpectedWinsAnalysis(leagueID, season, throughWeek)
    if err != nil || analysis == nil {
        // Calculate if not exists or outdated
        analysis, err = CalculateExpectedWins(leagueID, season, throughWeek, 10000)
        if err != nil {
            c.JSON(500, gin.H{"error": err.Error()})
            return
        }

        // Save results
        db.Create(analysis)
    }

    c.JSON(200, gin.H{"data": analysis})
}

// POST /api/v1/leagues/{id}/expected-wins/{season}/calculate
func RecalculateExpectedWins(c *gin.Context) {
    // Force recalculation with custom parameters
}
```

#### 5. Frontend Integration

```typescript
// Add to models.ts
export interface ExpectedWinsAnalysis extends BaseModel {
  league_id: number;
  season: number;
  week_calculated: number;
  num_simulations: number;
  team_results: ExpectedWinsTeamResult[];
}

export interface ExpectedWinsTeamResult extends BaseModel {
  analysis_id: number;
  team_id: number;
  actual_wins: number;
  expected_wins: number;
  win_luck: number;
  expected_record: string;
  weekly_expected_wins: number[];
}
```

```typescript
// Add to services
export const expectedWinsService = {
  getExpectedWins: async (leagueId: number, season: number, throughWeek?: number): Promise<ExpectedWinsAnalysis> => {
    const params = throughWeek ? `?through_week=${throughWeek}` : '';
    const response = await apiClient.get(`/leagues/${leagueId}/expected-wins/${season}${params}`);
    return response.data;
  },

  recalculateExpectedWins: async (leagueId: number, season: number, options?: { numSimulations?: number }): Promise<ExpectedWinsAnalysis> => {
    const response = await apiClient.post(`/leagues/${leagueId}/expected-wins/${season}/calculate`, options);
    return response.data;
  }
};
```

#### 6. User Interface Components

**Expected Wins Dashboard:**
- Table showing each team's actual vs expected wins
- "Luck" indicator (green for over-performing, red for under-performing)
- Expected record in "W.d-L.d" format
- Week-by-week progression chart

**Integration Points:**
- Add Expected Wins column to standings table
- Show on team detail pages
- Include in season summary/analytics page
- Add to playoff predictions context

### Benefits

1. **Fair Team Assessment**: Removes schedule luck from team evaluation
2. **Playoff Predictions**: More accurate seeding predictions based on true team strength
3. **Trade Analysis**: Better evaluation of team performance for trade decisions
4. **Historical Analysis**: Compare "should have been" records across seasons
5. **Entertainment Value**: Fun way to discuss team performance and luck

### Performance Considerations

- **Caching**: Cache results per week, only recalculate when new games complete
- **Background Processing**: Run calculations as background jobs for large simulations
- **Progressive Enhancement**: Start with fewer simulations, increase for more accuracy
- **Database Indexing**: Index on league_id, season, week_calculated for fast lookups

### Implementation Timeline

1. **Phase 1**: Backend models and calculation engine
2. **Phase 2**: API endpoints and basic caching
3. **Phase 3**: Frontend components and dashboard
4. **Phase 4**: Integration with existing pages and features
5. **Phase 5**: Advanced features (weekly progression, historical comparisons)

This feature would provide valuable insights into team performance beyond traditional win-loss records, helping users understand the difference between skill and luck in fantasy football.

## Weekly Expected Wins Tracking Implementation - COMPLETED ✅

### Overview
**IMPLEMENTED**: A comprehensive weekly expected wins tracking system that:
1. Tracks expected wins progression throughout the season
2. Calculates season totals using the final regular season week
3. Provides week-by-week analysis and cumulative statistics
4. Supports idempotent processing (no duplicate entries on re-runs)

### Implementation Details

#### Database Schema
The system uses two main models:

**`WeeklyExpectedWins`**: Stores week-by-week data with both cumulative and weekly values:
- `ExpectedWins`: Cumulative expected wins through this week (can be > 1)
- `WeeklyExpectedWins`: Expected wins for just this week (≤ 1)
- `ActualWins`/`ActualLosses`: Cumulative actual performance
- `WinLuck`: Difference between actual and expected wins
- Includes opponent data, scores, and context

**`SeasonExpectedWins`**: Final season totals calculated from the last regular season week:
- Uses cumulative data from the final regular season week
- Includes season aggregates (points for/against, averages)
- Playoff and standings information

#### Core Processing Logic

**Weekly Processing** (`ProcessWeeklyExpectedWins`):
- Processes each week sequentially (1 → N) during ETL
- Calculates cumulative expected wins using all games through current week
- Derives weekly expected wins by subtracting previous week's cumulative total
- Uses logistic probability function based on point differentials
- Idempotent via `SaveWeeklyExpectedWins` upsert function

**Season Finalization** (`FinalizeSeasonExpectedWins`):
- Triggered after regular season completion
- Uses each team's final regular season week data (handles missing weeks gracefully)
- Calculates season aggregates and contextual metrics

#### ETL Integration
- Modified ETL to process weeks 1 through final week sequentially
- Added `--calculate-expected-wins` flag for recalculation
- Clears existing data when forced recalculation is requested
- Processes all years or specific year ranges

#### Expected Wins Calculation
**Regular Season Games Only**: Excludes playoff games using `IsPlayoff = false AND GameType = "NONE"` filters to ensure consistent game counts across all teams.

**Logistic Probability Function**: Uses point differential to calculate win probability:
```
P(win) = 1 / (1 + e^(-0.09 * point_differential))
```

#### Idempotency
Implements robust upsert logic in `SaveWeeklyExpectedWins`:
- Searches for existing record by (team_id, year, week)
- Updates existing records preserving ID and created timestamp
- Creates new records only when none exist
- Prevents duplicate entries on multiple runs

#### Performance Features
- Comprehensive test coverage including idempotency tests
- Efficient database queries with proper indexing
- Sequential processing to maintain data consistency
- Error handling that continues processing other teams/weeks on failures

### Data Structure Validation
Extensive testing validates:
- Weekly expected wins are ≤ 1.0 (single week maximum)
- Cumulative values increase monotonically through the season
- Actual wins/losses match expected game counts
- Idempotent behavior preserves data integrity
- Playoff games are properly excluded from calculations

### API Endpoints
Provides REST endpoints for:
- Weekly expected wins data (specific week or full season progression)
- Season totals and rankings
- Recalculation triggers

### Frontend Integration
TypeScript interfaces mirror Go models for type safety across the full stack. Components can display:
- Week-by-week expected wins progression
- Season totals and luck analysis
- Team performance trends and comparisons

This implementation successfully provides fair team assessment by removing schedule luck from performance evaluation, enabling more accurate playoff predictions and historical analysis.

### Random Schedule Generation Configuration - UPDATED ✅
**IMPLEMENTED**: Replaced logistic probability approach with true simulation-based expected wins calculation.

**Current Implementation**:
- **10,000 random schedule simulations** per calculation (configurable via `EXPECTED_WINS_SIMULATIONS` environment variable)
- **True schedule luck removal**: Generates random opponent matchings using actual weekly scores
- **Performance optimized**: Uses Go's built-in random shuffle for schedule generation
- **Script-only access**: Removed POST API endpoints for recalculation to prevent server stress

**How It Works**:
1. **Extract Weekly Scores**: Collects actual scores for each team by week from completed games
2. **Generate Random Schedules**: Creates thousands of hypothetical schedules with random opponent pairings
3. **Simulate Matchups**: For each simulation, determines winners using actual scores in random matchups
4. **Average Results**: Expected wins = average wins across all simulated schedules

**Configuration**:
```bash
# Set simulation count (default: 10,000)
export EXPECTED_WINS_SIMULATIONS=5000
```

**Key Benefits**:
- **True schedule luck analysis**: Shows how teams would perform across all possible schedules
- **Accounts for strength of schedule**: Weak teams benefit when randomly matched against easier opponents
- **More comprehensive than logistic approach**: Reveals schedule-dependent performance variations
- **Performance balanced**: 10,000 simulations provide accurate results while remaining computationally feasible

**Technical Implementation**:
- Removed `logisticWinProbability()` function and all logistic-based calculations
- Added `runScheduleSimulations()` with proper random number generation
- Updated tests to account for simulation variance with appropriate tolerance
- Maintained idempotent ETL processing and script-only recalculation access

* [ ] Show the most important games based on simulation data and which ones change the outcome the most

* [ ] Transactions data

* [ ] Draft picks on the teams page

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

* [X] Store and display rosters for each game
