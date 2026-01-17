# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Structure

This is a fantasy football simulation application with a fullstack architecture:

- **Backend (Go)**: API server using Gin framework with PostgreSQL database via GORM
- **Frontend (Next.js)**: React application with TypeScript, styled with Tailwind CSS
- **Database**: PostgreSQL for persistent storage of leagues, teams, players, matchups, and simulations

### Key Architecture Components

**Backend (`/backend/`)**:
- `cmd/server/` - Main API server entry point
- `cmd/etl/` - ETL process for data import from ESPN fantasy data
- `internal/models/` - GORM models (Player, Team, Matchup, BoxScore, etc.)
- `internal/api/handlers/` - HTTP handlers for REST endpoints
- `internal/database/` - Database connection and migrations
- `internal/simulation/` - Monte Carlo simulation engine for playoff predictions

**Frontend (`/frontend/`)**:
- `src/pages/` - Next.js pages using pages router (players, teams, schedule, simulations, transactions)
- `src/components/` - Reusable React components
- `src/hooks/` - Custom React hooks for API calls
- `src/services/` - API client services matching backend endpoints
- `src/types/models.ts` - TypeScript interfaces matching Go backend models

## Common Development Commands

### Backend Development
```bash
# From /backend directory
make build          # Build both server and ETL binaries
make run            # Build and run the API server
make clean          # Remove built binaries
make etl            # Run ETL process to import ESPN data
```

### Frontend Development
```bash
# From /frontend directory
npm run dev         # Start Next.js development server
npm run build       # Build for production
npm run lint        # Run ESLint
npm run clean       # Clean build artifacts
```

### Full Application (Docker)
```bash
# From root directory
make docker-build   # Build Docker image
make docker-run     # Build and run in Docker
make docker-dev     # Run with development logging
```

## Database Integration

The application uses GORM with PostgreSQL. Key model relationships:
- Teams have many Players (many-to-many via team_players)
- Matchups link home/away teams and contain scores
- BoxScores track individual player performance per matchup
- Simulations run Monte Carlo analysis for playoff predictions

The ETL process imports ESPN fantasy league data including historical matchups, player stats, and transactions.

## API Structure

REST API endpoints follow `/api/v1/` pattern:
- `/health` - Health check
- `/players` - Player CRUD and stats
- `/teams` - Team management and rosters
- `/schedules` - Matchup data by year/week
- `/simulations` - Playoff simulation results
- `/transactions` - Trade/waiver wire history

Frontend services in `src/services/` correspond to these backend handlers, using a centralized `apiClient.ts` for HTTP requests.

## TypeScript Integration

The frontend `src/types/models.ts` mirrors backend Go structs, ensuring type safety across the full stack. When modifying backend models, update the corresponding TypeScript interfaces.

## Simulation Engine

The application uses a Monte Carlo simulation engine for calculating expected wins and playoff predictions.

### Expected Wins Calculation

**Location**: `backend/internal/simulation/expected_wins.go`

The expected wins calculation uses a **schedule-based Monte Carlo approach** to determine how many wins each team would have earned if they had played against all possible opponents with their actual scores.

**How It Works**:
1. For each simulation (default: 10,000 iterations):
   - Generate a complete valid season schedule using `SeasonSimulator`
   - Apply actual team scores to the simulated schedule
   - Count wins for each team
2. Average results across all simulations to get expected win probabilities

**Key Components**:
- `CalculateExpectedWins()` - Main entry point for full season expected wins
- `CalculateMatchupExpectedWins()` - Calculates per-matchup win probabilities
- `CalculateWeeklyExpectedWins()` - Calculates expected wins for a specific week

**Configuration**:
- Default simulations: 10,000 (configurable via `EXPECTED_WINS_SIMULATIONS` env var)
- Only calculates for completed regular season games
- Results stored in `Matchup.HomeTeamExpectedWin` and `Matchup.AwayTeamExpectedWin` fields

### Season Simulator

**Location**: `backend/internal/simulation/season_simulator.go`

The `SeasonSimulator` generates realistic full-season schedules that respect fantasy football constraints:

**Constraints Enforced**:
- Each team plays exactly once per week
- Teams play each opponent at most 2 times (or 1 time for round-robin scenarios)
- No back-to-back games against the same opponent

**Usage**:
```go
// Create simulator for 10 teams, 14 weeks
simulator := NewSeasonSimulator(teamIDs, 14, leagueID, year)

// Generate a valid schedule
schedule, err := simulator.GenerateSchedule()
if err != nil {
    // Handle error (e.g., odd number of teams)
}

// Validate schedule meets constraints
err = simulator.ValidateSchedule(schedule)
```

**Schedule Types**:
- **Standard** (e.g., 10 teams, 14 weeks): Max 2 games per opponent
- **Round-Robin** (e.g., 14 teams, 13 weeks): Each pair plays exactly once

**Integration with ETL**:
The SeasonSimulator is automatically used during the ETL process:
1. ETL loads matchup data from YAML files
2. After scores are applied, `processExpectedWinsForLeague()` is called
3. Monte Carlo simulation runs using valid generated schedules
4. Expected wins are calculated and stored in the database

**Testing**:
Comprehensive tests in `season_simulator_test.go` validate:
- 10 teams × 14 weeks scenarios
- 14 teams × 13 weeks round-robin scenarios
- Constraint validation (no back-to-back, max games per opponent)
- Edge cases (odd teams, impossible schedules)

### Schedule Generator

**Location**: `backend/internal/simulation/schedule_generator.go`

Lower-level schedule generation with additional features:
- Playoff bracket generation (6-team playoffs with wildcards)
- Full validation suite for schedule constraints
- Configurable parameters (teams, weeks, max games vs opponent)

**Note**: The `SeasonSimulator` wraps and simplifies the `ScheduleGenerator` for Monte Carlo simulations.