# Fantasy Football Simulations

A fun side-project for running simulations and analysis for my fantasy football league.

## Repository Structure

```
ff-sims/
├── backend/    # Go API server (Gin + PostgreSQL/GORM)
├── frontend/   # Next.js app (TypeScript + Tailwind CSS)
├── workers/    # Temporal workers + Python ESPN data ingestion
├── scripts/    # Python scripts for ESPN data fetching
└── docs/       # Documentation
```

## Getting Started

### Full Application (Docker)

```sh
make docker-build   # Build Docker image
make docker-run     # Build and run
make docker-dev     # Run with development logging
```

### Backend (Go)

```sh
cd backend
make build   # Build server and ETL binaries
make run     # Build and run the API server
make etl     # Run ETL to import ESPN data
```

### Frontend (Next.js)

```sh
cd frontend
npm run dev    # Start development server
npm run build  # Build for production
npm run lint   # Run ESLint
```

### Scripts (Python)

```sh
cd scripts
python3.10 -m venv venv
venv/bin/python -m pip install -r requirements.txt
venv/bin/python data.py
```

The scripts fetch data from ESPN using the [espn_api](https://github.com/cwendt/espn-api) library and write it to a `history.json` cache file. Delete the file to re-fetch all data.

## API

REST endpoints follow `/api/v1/`:

- `/health` - Health check
- `/players` - Player stats
- `/teams` - Team management and rosters
- `/schedules` - Matchup data by year/week
- `/simulations` - Monte Carlo playoff simulation results
- `/transactions` - Trade/waiver wire history
