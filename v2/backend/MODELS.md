# Database Models

This document describes the clean database schema managed by GORM AutoMigrate.

## Core Models

### League (`models.League`)
Represents a fantasy football league from any platform.

**Key Fields:**
- `ID` - Auto-increment primary key (used in URLs: `/league/:id`)
- `LeagueID` - Platform-specific league ID (ESPN/Sleeper ID)
- `Source` - Platform source: "espn" or "sleeper"
- `Name`, `Description`, `ScoringType`
- Embedded: `RosterSettings`, `ScoringSettings`

**Unique Constraint:** `(league_id, source)` - Same league ID from different platforms are distinct

### Team (`models.Team`)
Represents a fantasy team within a league.

**Key Fields:**
- `ID` - Auto-increment primary key
- `LeagueID` - Foreign key to `leagues.id`
- `ESPNID` - Nullable pointer (`*uint`) for ESPN team ID (nil for Sleeper teams)
- `Name` - Current team name
- `Owners` - JSON array of owner names

**Important:** No calculated fields (`Wins`, `Losses`, `Ties`, `Points`) - these are computed from matchups

**Relationships:**
- `NameHistory` - Historical team names with timestamps
- `HomeMatchups`, `AwayMatchups` - All matchups for this team
- `DraftSelections`, `Transactions` - Team history

### Matchup (`models.Matchup`)
Represents a game between two teams.

**Key Fields:**
- `ID` - Auto-increment primary key
- `LeagueID` - Foreign key to `leagues.id`
- `Season` - Year of the season (e.g., 2025)
- `Week` - Week number (1-17)
- `HomeTeamID`, `AwayTeamID` - Foreign keys to `teams.id`
- `HomeTeamFinalScore`, `AwayTeamFinalScore` - Actual scores
- `Completed`, `IsPlayoff` - Status flags
- `GameType` - "NONE", "WINNERS_BRACKET", "WINNERS_CONSOLATION_LADDER", etc.

**Unique Constraint:** `(league_id, season, week, home_team_id, away_team_id)`

**Note:** Uses `Season` field (not `Year`) to avoid confusion

### Player (`models.Player`)
Shared player database across all leagues.

**Key Fields:**
- `ID` - Auto-increment primary key
- `ESPNID` - ESPN player ID (int64, can be negative for defenses)
- `SleeperID` - Sleeper player ID
- `Name`, `Position`, `Team` (NFL team)
- `Active`, `InjuryStatus`

### DraftSelection (`models.DraftSelection`)
Records draft picks per league per season.

**Key Fields:**
- `LeagueID` - Foreign key to `leagues.id`
- `Year` - Draft year
- `Round`, `Pick` - Draft position
- `TeamID` - Team that made the pick
- `PlayerID` - Player selected

**Unique Constraint:** `(league_id, year, round, pick)`

### Transaction (`models.Transaction`)
Records adds, drops, and trades.

**Key Fields:**
- `LeagueID` - Foreign key to `leagues.id`
- `Year`, `Week` - When transaction occurred
- `TeamID` - Team making transaction
- `TransactionType` - "ADDED", "DROPPED", "TRADED"
- `PlayerID`, `PlayerName`
- For trades: `TradePartnerTeamID`, `RelatedTransactionID`

## Historical/Analytics Models

### TeamNameHistory (`models.TeamNameHistory`)
Tracks team name changes over time with start/end dates.

### WeeklyExpectedWins (`models.WeeklyExpectedWins`)
Stores expected wins calculation per team per week.

### SeasonExpectedWins (`models.SeasonExpectedWins`)
Aggregated expected wins per team per season.

### BoxScore (`models.BoxScore`)
Player performance in specific matchups (not yet fully implemented).

## Simulation Models

### Simulation, SimResult, SimTeamResult
Monte Carlo playoff simulation results.

## Sleeper-Specific Models

### SleeperLeague, SleeperUser, SleeperTransaction, SleeperDraftPick
Raw data from Sleeper API (separate from normalized models).

## Clean Schema Principles

1. **No duplicate fields** - `Matchup` uses `Season` (not both `Year` and `Season`)
2. **No calculated fields in base models** - `Team` doesn't store `Wins`/`Losses`
3. **Nullable foreign platform IDs** - `Team.ESPNID` is `*uint` for multi-platform support
4. **Composite unique keys** - `League` uses `(league_id, source)` for multi-platform
5. **AutoMigrate-ready** - All schema managed through GORM tags

## Reset Database

```bash
make reset-db
```

This will:
1. Drop the database
2. Recreate it
3. Run AutoMigrate to create all tables with proper constraints
4. Ready for ETL data import
