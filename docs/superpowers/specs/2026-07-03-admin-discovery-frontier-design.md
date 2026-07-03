# Spec: Admin Discovery Frontier Insight

**Date:** 2026-07-03
**Status:** Draft

## Context

League/user discovery (`backend/internal/workflows/discovery.go`,
`backend/internal/activities/discovery.go`) is a polling BFS: `DiscoveryBatchDispatcher` wakes on a
cron schedule, pulls a batch of stale `sleeper_users` (`last_fetched_at IS NULL` ordered oldest
first), and for each one spawns a `UserDiscoveryWorkflow` that fetches the user's leagues, then
each league's members (new `sleeper_users` rows, inserted with `last_fetched_at = NULL`), then
marks the user fetched. There's no in-memory queue or explicit depth counter — the "frontier"
(discovered-but-not-yet-expanded rows) is implicit in `last_fetched_at IS NULL AND skipped_at IS
NULL` on both `sleeper_users` and `sleeper_leagues`.

There's currently no visibility into how big that frontier is. This spec adds a fourth `/admin`
section, alongside Sync Backlog / Segment Distribution / Database Size, showing how many
leagues/users are known but not yet expanded.

## Non-goals

- No true BFS depth/hop-count tracking (would require a new `discovery_depth` column stamped at
  insert time). This is out of scope — the user confirmed frontier size (pending counts) is what's
  wanted, not hop distance from the seed user.
- No historical/trend view — point-in-time snapshot only, matching every other admin endpoint.
- No per-season breakdown for users — `SleeperUser` has no `season` column, and `FetchUserLeagues`
  expands a user across all seasons (`Seasons()`) in one pass, so "pending" isn't a season-scoped
  concept for users the way it is for leagues.
- No auth (matches existing app-wide posture).

## Design

### Backend

**Added to** `backend/internal/api/handlers/admin.go`:

```go
// AdminDiscoveryCounts is a total/expanded/pending/skipped breakdown for one
// entity type (sleeper_users, or sleeper_leagues within one season).
type AdminDiscoveryCounts struct {
    Total    int64 `json:"total"`
    Expanded int64 `json:"expanded"`
    Pending  int64 `json:"pending"`
    Skipped  int64 `json:"skipped"`
}

// AdminDiscoveryLeagueSeasonRow is the league discovery breakdown for one season.
type AdminDiscoveryLeagueSeasonRow struct {
    Season string `json:"season"`
    AdminDiscoveryCounts
}

// AdminDiscoveryFrontierResponse reports how much of the league/user discovery
// graph is known but not yet expanded, used to gauge remaining discovery work.
type AdminDiscoveryFrontierResponse struct {
    Users        AdminDiscoveryCounts            `json:"users"`
    LeaguesBySeason []AdminDiscoveryLeagueSeasonRow `json:"leagues_by_season"`
}
```

`Expanded` = `last_fetched_at IS NOT NULL`. `Pending` (the frontier) = `last_fetched_at IS NULL AND
skipped_at IS NULL`. `Skipped` = `skipped_at IS NOT NULL`. `Total` = all three summed (every
discovered row, regardless of state).

```go
func GetAdminDiscoveryFrontier(c *gin.Context) {
    var users AdminDiscoveryCounts
    const userQ = `
        SELECT
            COUNT(*) AS total,
            COUNT(*) FILTER (WHERE last_fetched_at IS NOT NULL) AS expanded,
            COUNT(*) FILTER (WHERE last_fetched_at IS NULL AND skipped_at IS NULL) AS pending,
            COUNT(*) FILTER (WHERE skipped_at IS NOT NULL) AS skipped
        FROM sleeper_users`
    if err := database.DB.Raw(userQ).Scan(&users).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    const leagueQ = `
        SELECT
            season,
            COUNT(*) AS total,
            COUNT(*) FILTER (WHERE last_fetched_at IS NOT NULL) AS expanded,
            COUNT(*) FILTER (WHERE last_fetched_at IS NULL AND skipped_at IS NULL) AS pending,
            COUNT(*) FILTER (WHERE skipped_at IS NOT NULL) AS skipped
        FROM sleeper_leagues
        GROUP BY season
        ORDER BY season DESC`
    rows := []AdminDiscoveryLeagueSeasonRow{}
    if err := database.DB.Raw(leagueQ).Scan(&rows).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, AdminDiscoveryFrontierResponse{Users: users, LeaguesBySeason: rows})
}
```

Note: `FILTER (WHERE ...)` is standard SQL supported by both Postgres and SQLite (3.30+, which
`gorm.io/driver/sqlite`'s bundled `mattn/go-sqlite3` uses), so this is testable against the
in-memory SQLite fake unlike the database-size endpoint.

**Route** — add to `backend/internal/api/routes.go`:

```go
admin.GET("/discovery-frontier", handlers.GetAdminDiscoveryFrontier)
```

**Response shape:**

```json
{
  "users": { "total": 5000, "expanded": 4200, "pending": 750, "skipped": 50 },
  "leagues_by_season": [
    { "season": "2026", "total": 1200, "expanded": 900, "pending": 280, "skipped": 20 },
    { "season": "2025", "total": 3400, "expanded": 3390, "pending": 5, "skipped": 5 }
  ]
}
```

### Frontend

- `frontend/src/services/adminService.ts` — add `AdminDiscoveryCounts`,
  `AdminDiscoveryLeagueSeasonRow`, `AdminDiscoveryFrontierResponse` interfaces and
  `adminService.getDiscoveryFrontier()`, calling `apiClient.get<AdminDiscoveryFrontierResponse>
  ("/admin/discovery-frontier")`.
- `frontend/src/hooks/useAdminDiscoveryFrontier.ts` — same `useState`/`useCallback`/`useEffect`
  shape as `useAdminSegments`.
- `frontend/src/pages/admin/index.tsx` — new `DiscoveryFrontier` section component, added below
  `DatabaseSize`:
  - A stat-card row for `users` (4 cards: Total / Expanded / Pending / Skipped), matching the
    Sync Backlog card style (`grid grid-cols-1 md:grid-cols-2 gap-6` sized up to 4 columns on
    larger screens).
  - A per-season table for `leagues_by_season`: Season, Total, Expanded, Pending, Skipped, % of
    total pending — same table styling as `SegmentDistribution`/`DatabaseSize`, sorted
    season-descending (already sorted server-side).
  - Loading/error states rendered inline, matching the other three sections exactly.

### Error handling

- Backend: DB errors return `500` with `gin.H{"error": ...}`, matching every other admin handler.
- Frontend: loading/error states rendered inline, matching the other three sections.

### Testing

- Backend: `backend/internal/api/handlers/admin_test.go`, extending `newAdminTestDB` to
  `AutoMigrate(&models.SleeperUser{})` as well. Add:
  - `TestGetAdminDiscoveryFrontier_UserCounts`: mix of expanded/pending/skipped
    `sleeper_users` rows, assert counts.
  - `TestGetAdminDiscoveryFrontier_LeaguesBySeason`: leagues across 2 seasons with mixed
    expanded/pending/skipped state, assert per-season breakdown and ordering.
  - `TestGetAdminDiscoveryFrontier_EmptyTables`: no rows in either table, assert all-zero counts
    and empty (non-nil) `leagues_by_season` slice.
- Frontend: no automated test (no page-level test infra in the repo, matching existing precedent);
  verified manually in-browser against a real Postgres backend.
