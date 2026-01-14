# Database Migrations

This project uses **GORM AutoMigrate** for database schema management.

## How to Reset the Database

To drop and recreate the database with fresh schema from models:

```bash
# From the backend directory
./scripts/reset_db.sh
```

Or manually:

```bash
# Drop and recreate database
psql -U postgres -c "DROP DATABASE IF EXISTS ffsims;"
psql -U postgres -c "CREATE DATABASE ffsims;"

# Run AutoMigrate and load data
DB_MIGRATE=true make etl
```

## Model Definitions

All database schema is defined in `internal/models/`:
- `league.go` - League model with source field (espn/sleeper)
- `team.go` - Team model (ESPNID is nullable for multi-platform support)
- `matchup.go` - Matchup model (uses Season, not Year)
- `player.go` - Player model
- `transaction.go` - Transaction and DraftSelection models
- And more...

## Notes

- The schema is defined entirely in Go structs with GORM tags
- No SQL migration files needed
- AutoMigrate runs when `DB_MIGRATE=true` environment variable is set
- Foreign keys are managed by GORM
