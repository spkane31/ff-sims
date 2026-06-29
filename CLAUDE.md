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