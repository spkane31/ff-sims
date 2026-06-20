# API: League-Scoped Routes Design

**Date:** 2026-06-19
**Status:** Approved
**Issue:** #53
**Parent spec:** `2026-06-17-multi-league-migration-design.md` (Phase 3)

## Overview

Restructure all backend API routes to be league-scoped under `/api/v1/leagues/:leagueId/...`. Old flat `/api/...` routes are removed with no backwards-compat shims. Players remain at a global `/api/v1/players/...` prefix since NFL players are shared across all leagues.

## Route Structure

### New routes

```
# League management
GET /api/v1/leagues                                             # list all leagues
GET /api/v1/leagues/:leagueId                                   # single league metadata

# Teams
GET /api/v1/leagues/:leagueId/teams                             # all teams in league
GET /api/v1/leagues/:leagueId/teams/:teamId                     # single team
GET /api/v1/leagues/:leagueId/teams/:teamId/expected-wins/:year # team progression

# Schedule
GET /api/v1/leagues/:leagueId/schedules                         # matchups (query: year, week)
GET /api/v1/leagues/:leagueId/schedules/:matchupId              # single matchup

# Transactions
GET /api/v1/leagues/:leagueId/transactions                      # waiver/trade history
GET /api/v1/leagues/:leagueId/transactions/draft-picks          # draft picks (query: year)

# Simulations
GET /api/v1/leagues/:leagueId/simulations/stats                 # team scoring stats

# Expected wins
GET /api/v1/leagues/:leagueId/expected-wins/weekly/:year        # weekly xwins (query: week)
GET /api/v1/leagues/:leagueId/expected-wins/season/:year        # season xwins
GET /api/v1/leagues/:leagueId/expected-wins/rankings/:year      # season rankings
GET /api/v1/leagues/:leagueId/expected-wins/luck/:year          # luck distribution

# Players (global — NFL players are shared across leagues)
GET /api/v1/players                                             # paginated player list
GET /api/v1/players/:id                                         # single player
GET /api/v1/players/stats                                       # player stats (query: year, week, season)
```

### Removed routes (no shims)

All routes under `/api/...` (non-versioned) are deleted.

## Implementation

### `routes.go`

Replace the entire `SetupRouter` body with a new group structure:

```
v1 := r.Group("/api/v1")
leagues := v1.Group("/leagues")
    leagues.GET("", GetLeagues)
    leagues.GET("/:leagueId", GetLeague)
    leagueScoped := leagues.Group("/:leagueId")
        leagueScoped.GET("/teams", ...)
        leagueScoped.GET("/teams/:teamId", ...)
        leagueScoped.GET("/teams/:teamId/expected-wins/:year", ...)
        leagueScoped.GET("/schedules", ...)
        leagueScoped.GET("/schedules/:matchupId", ...)
        leagueScoped.GET("/transactions", ...)
        leagueScoped.GET("/transactions/draft-picks", ...)
        leagueScoped.GET("/simulations/stats", ...)
        leagueScoped.GET("/expected-wins/weekly/:year", ...)
        leagueScoped.GET("/expected-wins/season/:year", ...)
        leagueScoped.GET("/expected-wins/rankings/:year", ...)
        leagueScoped.GET("/expected-wins/luck/:year", ...)
players := v1.Group("/players")
    players.GET("", ...)
    players.GET("/stats", ...)    # must be registered before /:id to avoid conflict
    players.GET("/:id", ...)
```

### Shared helper

Add `parseLeagueID(c *gin.Context) (uint, bool)` in a new file `handlers/helpers.go`. Returns the parsed leagueId and writes a 400 response + returns false on parse failure. All league-scoped handlers call this instead of repeating the parse logic.

```go
func parseLeagueID(c *gin.Context) (uint, bool) {
    val, err := strconv.ParseUint(c.Param("leagueId"), 10, 32)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid leagueId"})
        return 0, false
    }
    return uint(val), true
}
```

The existing `parseUintParam` in `expected_wins.go` is kept for parsing `:year` and `:teamId` sub-params; it is not replaced.

### Per-handler changes

| Handler | Current league source | Change |
|---|---|---|
| `GetTeams` | hardcoded `345674` | `parseLeagueID` |
| `GetTeamByID` | none | `parseLeagueID` (filter matchups by league) |
| `GetAllTimeExpectedWins` | hardcoded `345674` | `parseLeagueID` |
| `GetCurrentSeasonStandings` | hardcoded `345674` | `parseLeagueID` |
| `GetTeamProgression` | none | `parseLeagueID` + `parseUintParam(c, "teamId")` |
| `GetSchedules` | none | `parseLeagueID` |
| `GetMatchup` | none | `parseLeagueID` + add `league_id = ?` to matchup WHERE clause |
| `GetTransactions` | query param `?league_id` | `parseLeagueID` |
| `GetDraftPicks` | query param `?league_id` | `parseLeagueID` |
| `GetStats` (simulations) | none | `parseLeagueID` |
| `GetWeeklyExpectedWins` | path param `/:id` → rename to `/:leagueId` | rename param only |
| `GetSeasonExpectedWins` | path param `/:id` | rename param only |
| `GetSeasonRankings` | path param `/:id` | rename param only |
| `GetLuckDistribution` | path param `/:id` | rename param only |
| `GetLeagueYears` | query param `?league_id` | `parseLeagueID` |
| `GetPlayers` | none (global) | no change |
| `GetPlayerByID` | none (global) | no change |
| `GetPlayerStats` | none (global) | no change |

### New handlers in `leagues.go`

**`GetLeagues`** — queries all leagues ordered by ID, returns `[]League` with id, name, platform, external_id, current_week, total_weeks.

**`GetLeague`** — looks up by `:leagueId`, returns 404 if not found.

## Error handling

- Invalid `:leagueId` format → 400 `{"error": "invalid leagueId"}`
- League not found (for `GetLeague`) → 404 `{"error": "league not found"}`
- All other handlers trust that `:leagueId` exists (no existence check per request — that's the frontend's responsibility to pass a valid ID from the leagues list).

## What does NOT change

- Handler business logic (DB queries, response shapes) is unchanged except for threading `leagueID` into existing queries.
- `GET /api/health` stays at `/api/health` (not versioned — used by infra health checks).
- Player handlers have no league scoping added.
- No pagination or new response fields introduced.
