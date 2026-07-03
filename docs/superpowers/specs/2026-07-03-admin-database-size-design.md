# Spec: Admin Database Size Monitoring

**Date:** 2026-07-03
**Status:** Draft

## Context

The app runs a single Postgres database (`ffsims`, see `DATABASE_URL` default in
`backend/internal/config/config.go`). The `/admin` page already surfaces operational signals for
capacity planning: sync backlog (`GetAdminBacklog`) and league-format distribution
(`GetAdminSegments`), both added per
`docs/superpowers/specs/2026-07-02-admin-backlog-endpoint-design.md`. This spec adds a third
signal to the same page: how big the database is, and which tables are driving that size — useful
for spotting runaway growth (e.g. `sleeper_transactions`) before it becomes a storage problem.

## Non-goals

- No historical/trend view (point-in-time snapshot only, matching the existing admin endpoints).
- No alerting/thresholds — this is a dashboard, not a monitor with notifications.
- No auth (matches existing app-wide posture — no auth exists anywhere in this app today).
- No support for non-Postgres backends. The query set is Postgres-specific
  (`pg_database_size`, `pg_stat_user_tables`); there is exactly one database in this app and it's
  always Postgres in every deployed environment.

## Design

### Backend

**Added to** `backend/internal/api/handlers/admin.go`:

```go
type AdminTableSizeRow struct {
    TableName   string `json:"table_name"`
    SizeBytes   int64  `json:"size_bytes"`
    RowEstimate int64  `json:"row_estimate"`
}

type AdminDatabaseSizeResponse struct {
    TotalBytes int64               `json:"total_bytes"`
    Tables     []AdminTableSizeRow `json:"tables"`
}

func GetAdminDatabaseSize(c *gin.Context) {
    var totalBytes int64
    if err := database.DB.Raw(`SELECT pg_database_size(current_database())`).
        Scan(&totalBytes).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    const q = `
        SELECT
            relname AS table_name,
            pg_total_relation_size(relid) AS size_bytes,
            n_live_tup AS row_estimate
        FROM pg_catalog.pg_stat_user_tables
        WHERE schemaname = 'public'
        ORDER BY size_bytes DESC`

    tables := []AdminTableSizeRow{}
    if err := database.DB.Raw(q).Scan(&tables).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, AdminDatabaseSizeResponse{TotalBytes: totalBytes, Tables: tables})
}
```

`pg_total_relation_size` includes the table's indexes (i.e. "what would I reclaim if I dropped this
table"), which is the number that matters for capacity planning. `total_bytes` (from
`pg_database_size`) is the authoritative whole-database size and is **not** expected to exactly
equal the sum of `tables[].size_bytes` — it also covers system catalogs, TOAST overhead already
folded per-table, and free space map / visibility map pages. The frontend should not try to
reconcile the two; both are shown as independently correct numbers.

**Route** — add to `backend/internal/api/routes.go`:

```go
admin.GET("/database-size", handlers.GetAdminDatabaseSize)
```

**Response shape:**

```json
{
  "total_bytes": 184320000,
  "tables": [
    { "table_name": "sleeper_transactions", "size_bytes": 94371840, "row_estimate": 812345 },
    { "table_name": "sleeper_leagues", "size_bytes": 41943040, "row_estimate": 3021 }
  ]
}
```

No row limit — all `public` schema tables are returned; the frontend table scrolls if long, same
as `SegmentDistribution`.

### Frontend

- `frontend/src/services/adminService.ts` — add `AdminTableSizeRow` / `AdminDatabaseSizeResponse`
  interfaces and `adminService.getDatabaseSize()`, calling
  `apiClient.get<AdminDatabaseSizeResponse>("/admin/database-size")`.
- `frontend/src/hooks/useAdminDatabaseSize.ts` — same `useState`/`useCallback`/`useEffect` shape as
  `useAdminBacklog`.
- `frontend/src/pages/admin/index.tsx` — new `DatabaseSize` section component, added to the page
  below `SegmentDistribution`:
  - A stat card showing total DB size, human-formatted (bytes → KB/MB/GB via a small
    `formatBytes` helper alongside the existing `formatRelativeTime` helper).
  - A table listing every entry in `tables`: table name, size (human-formatted), row estimate
    (`toLocaleString()`), and % of `total_bytes` — same table/thead/tbody Tailwind classes as
    `SegmentDistribution`'s table, sorted largest-first (already sorted server-side).
  - Loading/error states rendered inline, matching the other two sections exactly.

### Error handling

- Backend: DB errors return `500` with `gin.H{"error": ...}`, matching every other admin handler.
- Frontend: loading/error states rendered inline, matching the other two sections.

### Testing

- Backend: `backend/internal/api/handlers/admin_test.go`. The existing tests in this file run
  against an in-memory SQLite fake (`newAdminTestDB`), and CI (`ci.yml`) runs `go test ./...` with
  no real Postgres available anywhere. `pg_database_size` / `pg_stat_user_tables` don't exist in
  SQLite, so a real happy-path test (asserting actual byte counts) isn't possible in this harness.
  Add one test that confirms the handler fails cleanly instead of panicking when run against a
  non-Postgres backend:
  - `TestGetAdminDatabaseSize_RequiresPostgres`: swap in the SQLite fake, call the handler, assert
    it returns `500` with a non-empty `error` body (not a panic/5xx-with-empty-body).
- Frontend: no automated test (no page-level test infra in the repo, matching the backlog/segments
  precedent); verified manually in-browser — start the dev server, hit `/admin` against a real
  Postgres backend, confirm the total size and per-table rows render sensible numbers.
