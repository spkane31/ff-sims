# Spec: Admin Backlog Endpoint

**Date:** 2026-07-02
**Status:** Draft

## Context

The Sleeper sync pipeline (`TransactionSyncDispatcher` / `LeagueTransactionSyncWorkflow`, in
`internal/workflows/transaction_sync.go`) processes a large and growing backlog of leagues whose
transactions haven't been fetched yet (see
`docs/superpowers/specs/2026-06-28-backfill-throughput-scaling.md` for the queue-depth analysis
that motivated the current `SyncBatchSize`/interval tuning). That analysis was done via one-off
SQL run by hand. This spec adds a small, permanent `/admin` surface — starting with exactly one
query — so the same backlog signal is a page load away instead of a psql session, to help decide
when to scale Temporal worker concurrency.

This is a new surface in the codebase: today there is no `/admin` route, no auth middleware, and
no admin page anywhere in either the Go backend or the Next.js frontend (confirmed by repo-wide
search). Per discussion, no auth gating is being added now — this matches the rest of the app,
which has none, and can be revisited if this ever needs to be multi-user or externally reachable.
Also per discussion: this ships as one hardcoded endpoint + one page, not a generic ad-hoc query
framework — a registry/generic-runner abstraction is deferred until a second admin query actually
exists.

## Non-goals

- No auth/access control (matches existing app-wide posture).
- No generic query framework/registry — just this one query, hardcoded.
- No write/mutation actions (no "trigger a resync now" button) — read-only, matching the existing
  precedent in `expected_wins.go` where an admin recalculation-trigger endpoint was deliberately
  removed "to prevent server stress." Keeping this surface cheap and read-only avoids repeating
  that mistake.
- No draft-fetch backlog (only transactions) — can be added as a second stat later using the same
  pattern.

## Design

### "Current season" resolution (deviation from initial proposal)

Initially I proposed reusing `WeekStatsActivities.GetCurrentSeason`, which calls Sleeper's live
`/state/nfl` endpoint. On closer look, every existing handler in `internal/api/handlers/` only
touches `database.DB` — none make outbound HTTP calls. Adding a live external call to a monitoring
endpoint means the backlog page can fail even when the answer (which is 100% local data) is fine,
and adds latency/a new dependency for no real benefit here.

Instead, "current season" is resolved as `MAX(season)` over `sleeper_leagues`. This reflects the
season the discovery/dispatch pipeline is actually populating right now, which is the more
relevant definition for a backlog-sizing tool anyway (vs. the NFL's own calendar). If
`sleeper_leagues` is empty, `season` is `""` and all counts are naturally `0`.

### Backend

**New file** `backend/internal/api/handlers/admin.go`:

```go
package handlers

type AdminBacklogResponse struct {
    Season                     string     `json:"season"`
    TotalLeagues                int64      `json:"total_leagues"`
    NeverFetchedCount            int64      `json:"never_fetched_count"`
    OldestTransactionsFetchedAt *time.Time `json:"oldest_transactions_fetched_at"`
}

func GetAdminBacklog(c *gin.Context) {
    var season string
    database.DB.Model(&models.SleeperLeague{}).Select("COALESCE(MAX(season), '')").Scan(&season)

    var resp AdminBacklogResponse
    resp.Season = season

    base := database.DB.Model(&models.SleeperLeague{}).
        Where("season = ? AND skipped_at IS NULL", season)

    base.Count(&resp.TotalLeagues)
    // .Session(&gorm.Session{}) each reuse to avoid clause accumulation across calls
    database.DB.Model(&models.SleeperLeague{}).
        Where("season = ? AND skipped_at IS NULL AND last_transactions_fetched_at IS NULL", season).
        Count(&resp.NeverFetchedCount)
    database.DB.Model(&models.SleeperLeague{}).
        Where("season = ? AND skipped_at IS NULL AND last_transactions_fetched_at IS NOT NULL", season).
        Select("MIN(last_transactions_fetched_at)").Scan(&resp.OldestTransactionsFetchedAt)

    c.JSON(http.StatusOK, resp)
}
```

(Exact GORM chaining to be finalized during implementation — the point is three independent
queries against `sleeper_leagues`, filtered to the resolved `season` and `skipped_at IS NULL`,
matching the predicate style already used in `GetStaleLeaguesForTransactions`.)

**Route** — add to `backend/internal/api/routes.go`:

```go
admin := v1.Group("/admin")
admin.GET("/backlog", handlers.GetAdminBacklog)
```

**Response shape:**

```json
{
  "season": "2026",
  "total_leagues": 412,
  "never_fetched_count": 37,
  "oldest_transactions_fetched_at": "2026-06-20T14:03:00Z"
}
```

`oldest_transactions_fetched_at` is `null` when every league in the season is either unfetched or
there are zero leagues.

### Frontend

- `frontend/src/services/adminService.ts` — `adminService.getBacklog()`, calling
  `apiClient.get<AdminBacklogResponse>("/admin/backlog")`, matching `leaguesService.ts`'s shape.
- `frontend/src/hooks/useAdminBacklog.ts` — same `useState`/`useCallback`/`useEffect` shape as
  `useLeagues` (loading/error/data states).
- `frontend/src/pages/admin/index.tsx` — single page, wrapped in the existing `<Layout>`, showing:
  - Season being reported on
  - "`{never_fetched_count}` of `{total_leagues}` leagues never fetched this season"
  - "Oldest fetch: `{relative time}`" (or "No leagues fetched yet" / "All caught up" as
    appropriate) when `oldest_transactions_fetched_at` is present
- No shared admin nav/layout component yet — just this page file, per the "one hardcoded endpoint"
  decision. If a second admin query is added later, that's the point to extract a shared
  `AdminLayout`.

### Error handling

- Backend: DB errors return `500` with `gin.H{"error": ...}`, matching every other handler.
- Frontend: loading/error states rendered inline on the page, matching every other page (no
  special-casing for admin).

### Testing

- Backend: `backend/internal/api/handlers/admin_test.go`, following the
  `database.DB = db` / `defer func(){ database.DB = originalDB }()` swap pattern from
  `internal/simulation/*_test.go`, with a sqlite in-memory DB (`newTestDB`-style helper, or
  inline `gorm.Open(sqlite.Open(":memory:"), ...)` with `AutoMigrate(&models.SleeperLeague{})`).
  Cases:
  - Mixed seasons: only the max-season rows are counted.
  - Some leagues `last_transactions_fetched_at IS NULL`, some not: correct `never_fetched_count`
    and `oldest_transactions_fetched_at` (MIN of the non-null ones).
  - All leagues never fetched: `oldest_transactions_fetched_at` is `null`.
  - `skipped_at IS NOT NULL` rows excluded from all three numbers.
  - Empty table: `season == ""`, all counts `0`, oldest `null`.
- Frontend: no automated test (no existing page-level test infra in the repo); verified manually
  in-browser per the UI-change requirement — start the dev server, hit `/admin`, confirm it
  renders real data against a local backend.
