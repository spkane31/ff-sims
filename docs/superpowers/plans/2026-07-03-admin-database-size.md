# Admin Database Size Monitoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a third `/admin` monitoring signal — total Postgres database size and a per-table size breakdown — matching the existing backlog/segments endpoints in shape and style.

**Architecture:** One new Go handler (`GetAdminDatabaseSize`) runs two raw SQL queries against Postgres (`pg_database_size`, `pg_stat_user_tables`) and returns them as JSON. One new frontend service method + hook + page section fetch and render that JSON, following the exact patterns already used for `AdminBacklog`/`AdminSegments`.

**Tech Stack:** Go (Gin, GORM raw SQL), Next.js/React (TypeScript, Tailwind CSS).

## Global Constraints

- Postgres-only SQL (`pg_database_size`, `pg_catalog.pg_stat_user_tables`) — this app has exactly one deployed database type, always Postgres. Do not add SQLite/other-dialect branching.
- No auth — matches the existing app-wide posture (no auth anywhere today).
- No historical/trend storage — point-in-time snapshot only, same as `GetAdminBacklog`/`GetAdminSegments`.
- Backend errors return `500` with `gin.H{"error": err.Error()}` — the only error-handling style used in `internal/api/handlers/`.
- Frontend loading/error states are rendered inline in the page component (spinner div + red text paragraph) — no toast/global error UI exists in this app.

---

### Task 1: Backend handler — `GetAdminDatabaseSize`

**Files:**
- Modify: `backend/internal/api/handlers/admin.go` (append to end of file)
- Modify: `backend/internal/api/routes.go:48-51` (add route)
- Test: `backend/internal/api/handlers/admin_test.go` (append to end of file)

**Interfaces:**
- Consumes: `database.DB` (package var, `*gorm.DB`, already imported in `admin.go`); `newAdminTestDB(t)` and `withAdminTestDB(t, db)` test helpers already defined in `admin_test.go`.
- Produces: `AdminTableSizeRow{ TableName string; SizeBytes int64; RowEstimate int64 }`, `AdminDatabaseSizeResponse{ TotalBytes int64; Tables []AdminTableSizeRow }`, `GetAdminDatabaseSize(c *gin.Context)` — all consumed by Task 3 (frontend types must match these JSON field names exactly: `table_name`, `size_bytes`, `row_estimate`, `total_bytes`, `tables`).

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/api/handlers/admin_test.go`:

```go
func TestGetAdminDatabaseSize_RequiresPostgres(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin/database-size", GetAdminDatabaseSize)

	req := httptest.NewRequest(http.MethodGet, "/admin/database-size", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on non-Postgres backend, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if body["error"] == "" {
		t.Error("expected non-empty error message")
	}
}
```

This is the only test possible here: the repo's Go tests run against an in-memory SQLite fake (see `newAdminTestDB`), and CI (`.github/workflows/ci.yml`) has no real Postgres, so `pg_database_size`/`pg_stat_user_tables` will not resolve. This test confirms the handler fails cleanly (500 + error body) rather than panicking — it does not, and cannot, verify real byte counts.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/api/handlers/... -run TestGetAdminDatabaseSize_RequiresPostgres -v`
Expected: FAIL — `undefined: GetAdminDatabaseSize`

- [ ] **Step 3: Write the handler**

Append to `backend/internal/api/handlers/admin.go`:

```go
// AdminTableSizeRow is one table's on-disk size (including its indexes) and
// estimated row count.
type AdminTableSizeRow struct {
	TableName   string `json:"table_name"`
	SizeBytes   int64  `json:"size_bytes"`
	RowEstimate int64  `json:"row_estimate"`
}

// AdminDatabaseSizeResponse reports the total Postgres database size and a
// per-table breakdown, used to spot which tables are driving storage growth.
type AdminDatabaseSizeResponse struct {
	TotalBytes int64               `json:"total_bytes"`
	Tables     []AdminTableSizeRow `json:"tables"`
}

// GetAdminDatabaseSize reports the total on-disk size of the current
// Postgres database and a per-table breakdown (table + index bytes, sorted
// largest first) for the public schema.
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

- [ ] **Step 4: Register the route**

In `backend/internal/api/routes.go`, change:

```go
	admin := v1.Group("/admin")
	admin.GET("/backlog", handlers.GetAdminBacklog)
	admin.GET("/segments", handlers.GetAdminSegments)
```

to:

```go
	admin := v1.Group("/admin")
	admin.GET("/backlog", handlers.GetAdminBacklog)
	admin.GET("/segments", handlers.GetAdminSegments)
	admin.GET("/database-size", handlers.GetAdminDatabaseSize)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd backend && go test ./internal/api/handlers/... -run TestGetAdminDatabaseSize_RequiresPostgres -v`
Expected: PASS

- [ ] **Step 6: Run the full backend test suite and build**

Run: `cd backend && go build ./... && go test ./...`
Expected: all packages build and all tests PASS (no regressions in `admin_test.go`'s existing tests).

- [ ] **Step 7: Commit**

```bash
git add backend/internal/api/handlers/admin.go backend/internal/api/handlers/admin_test.go backend/internal/api/routes.go
git commit -m "feat(admin): add database size endpoint"
```

---

### Task 2: Manually verify the endpoint against a real Postgres instance

**Files:** none (verification only — no files changed in this task)

**Interfaces:**
- Consumes: `GetAdminDatabaseSize` route from Task 1, running backend server, local Postgres instance with the app's schema migrated.

Since Task 1's automated test cannot exercise real Postgres SQL (SQLite doesn't support `pg_database_size`/`pg_stat_user_tables`), this task is the only check that the actual SQL is valid and returns sane data before the frontend is built against it.

- [ ] **Step 1: Start the backend against a real Postgres database**

Run (from `backend/`): `make run` (or `go run ./cmd/server` if `DATABASE_URL` is already exported in your shell). Confirm it logs a successful DB connection and listens on the configured port (default `8080`).

- [ ] **Step 2: Call the new endpoint directly**

Run: `curl -s http://localhost:8080/api/v1/admin/database-size | python3 -m json.tool`

Expected: HTTP 200, JSON body shaped like:

```json
{
  "total_bytes": 184320000,
  "tables": [
    { "table_name": "sleeper_transactions", "size_bytes": 94371840, "row_estimate": 812345 },
    { "table_name": "sleeper_leagues", "size_bytes": 41943040, "row_estimate": 3021 }
  ]
}
```

Confirm `total_bytes` is a large positive integer, `tables` is sorted largest-`size_bytes`-first, and every table you know exists in the schema (e.g. `sleeper_leagues`, `sleeper_transactions`, `players`, `teams`) appears somewhere in the list.

- [ ] **Step 3: Note any discrepancy**

If the query errors (e.g. permissions issue on `pg_stat_user_tables` for a restricted DB role) or a schema is not `public` in your local setup, stop and resolve it before continuing to Task 3 — the frontend work assumes this endpoint already returns real data correctly.

---

### Task 3: Frontend service + hook

**Files:**
- Modify: `frontend/src/services/adminService.ts`
- Create: `frontend/src/hooks/useAdminDatabaseSize.ts`

**Interfaces:**
- Consumes: `apiClient.get<T>(endpoint)` from `frontend/src/services/apiClient.ts`; JSON shape from Task 1 (`total_bytes`, `tables[].table_name/size_bytes/row_estimate`).
- Produces: `AdminTableSizeRow`, `AdminDatabaseSizeResponse` TypeScript interfaces and `adminService.getDatabaseSize()` (consumed by the hook below); `useAdminDatabaseSize()` returning `{ databaseSize, isLoading, error }` where `databaseSize: AdminDatabaseSizeResponse | null` (consumed by Task 4).

- [ ] **Step 1: Add types and service method**

In `frontend/src/services/adminService.ts`, add after the existing `AdminSegmentsResponse` interface (before `export const adminService = {`):

```typescript
export interface AdminTableSizeRow {
  table_name: string;
  size_bytes: number;
  row_estimate: number;
}

export interface AdminDatabaseSizeResponse {
  total_bytes: number;
  tables: AdminTableSizeRow[];
}
```

And add a method inside the `adminService` object, after `getSegments`:

```typescript
  getDatabaseSize: async (): Promise<AdminDatabaseSizeResponse> => {
    return apiClient.get<AdminDatabaseSizeResponse>('/admin/database-size');
  },
```

The full object should now read:

```typescript
export const adminService = {
  getBacklog: async (): Promise<AdminBacklogResponse> => {
    return apiClient.get<AdminBacklogResponse>('/admin/backlog');
  },

  getSegments: async (): Promise<AdminSegmentsResponse> => {
    return apiClient.get<AdminSegmentsResponse>('/admin/segments');
  },

  getDatabaseSize: async (): Promise<AdminDatabaseSizeResponse> => {
    return apiClient.get<AdminDatabaseSizeResponse>('/admin/database-size');
  },
};
```

- [ ] **Step 2: Create the hook**

Create `frontend/src/hooks/useAdminDatabaseSize.ts`:

```typescript
import { useState, useEffect, useCallback } from "react";
import { adminService, AdminDatabaseSizeResponse } from "../services/adminService";

export function useAdminDatabaseSize() {
  const [databaseSize, setDatabaseSize] = useState<AdminDatabaseSizeResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchDatabaseSize = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await adminService.getDatabaseSize();
      setDatabaseSize(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to fetch admin database size"));
      setDatabaseSize(null);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchDatabaseSize();
  }, [fetchDatabaseSize]);

  return { databaseSize, isLoading, error };
}
```

- [ ] **Step 3: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: no new type errors (no test runner exists for hooks/services in this repo, matching `useAdminBacklog`/`useAdminSegments` precedent — typecheck is the available automated check).

- [ ] **Step 4: Commit**

```bash
git add frontend/src/services/adminService.ts frontend/src/hooks/useAdminDatabaseSize.ts
git commit -m "feat(admin): add database size service and hook"
```

---

### Task 4: Frontend page section

**Files:**
- Modify: `frontend/src/pages/admin/index.tsx`

**Interfaces:**
- Consumes: `useAdminDatabaseSize()` from Task 3, returning `{ databaseSize: AdminDatabaseSizeResponse | null, isLoading: boolean, error: Error | null }`; `AdminDatabaseSizeResponse.total_bytes: number`, `.tables: AdminTableSizeRow[]` where each row has `.table_name: string`, `.size_bytes: number`, `.row_estimate: number`.
- Produces: nothing consumed elsewhere — this is the final rendering task.

- [ ] **Step 1: Add the import and a `formatBytes` helper**

In `frontend/src/pages/admin/index.tsx`, change the import line:

```typescript
import Layout from "../../components/Layout";
import { useAdminBacklog } from "../../hooks/useAdminBacklog";
import { useAdminSegments } from "../../hooks/useAdminSegments";
```

to:

```typescript
import Layout from "../../components/Layout";
import { useAdminBacklog } from "../../hooks/useAdminBacklog";
import { useAdminSegments } from "../../hooks/useAdminSegments";
import { useAdminDatabaseSize } from "../../hooks/useAdminDatabaseSize";
```

Add this helper function right after `formatRelativeTime` (after its closing `}` on line 19, before `function SegmentDistribution()`):

```typescript
function formatBytes(bytes: number): string {
  if (bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / Math.pow(1024, exponent);
  return `${value.toFixed(exponent === 0 ? 0 : 1)} ${units[exponent]}`;
}
```

- [ ] **Step 2: Add the `DatabaseSize` section component**

Insert this new component after the `SegmentDistribution` function's closing `}` (after line 129, before `export default function AdminBacklog()`):

```typescript
function DatabaseSize() {
  const { databaseSize, isLoading, error } = useAdminDatabaseSize();

  return (
    <section>
      <h2 className="text-2xl font-bold text-blue-600 mb-2">Database Size</h2>
      <p className="text-gray-600 dark:text-gray-300 mb-4">
        Total Postgres database size and a per-table breakdown, used to spot which tables are
        driving storage growth. Per-table sizes include their indexes and won&apos;t sum exactly
        to the total (which also covers system catalogs and free space).
      </p>

      {isLoading && (
        <div className="flex items-center space-x-2">
          <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin" />
          <p>Loading database size...</p>
        </div>
      )}

      {error && <p className="text-red-600">Failed to load database size.</p>}

      {!isLoading && !error && databaseSize && (
        <>
          <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md border border-gray-100 dark:border-gray-600 mb-4 max-w-xs">
            <div className="text-3xl font-bold text-blue-600 mb-1">
              {formatBytes(databaseSize.total_bytes)}
            </div>
            <div className="text-lg font-medium text-gray-800 dark:text-gray-100">
              Total database size
            </div>
          </div>

          <div className="bg-white dark:bg-gray-700 rounded-lg shadow-md border border-gray-100 dark:border-gray-600 overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
              <thead className="bg-gray-50 dark:bg-gray-800">
                <tr>
                  <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Table
                  </th>
                  <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Size
                  </th>
                  <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    % of Total
                  </th>
                  <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Rows (est.)
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
                {databaseSize.tables.map((row) => (
                  <tr key={row.table_name}>
                    <td className="py-2 px-4 text-gray-800 dark:text-gray-100">{row.table_name}</td>
                    <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                      {formatBytes(row.size_bytes)}
                    </td>
                    <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                      {databaseSize.total_bytes > 0
                        ? `${((row.size_bytes / databaseSize.total_bytes) * 100).toFixed(1)}%`
                        : "—"}
                    </td>
                    <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                      {row.row_estimate.toLocaleString()}
                    </td>
                  </tr>
                ))}
                {databaseSize.tables.length === 0 && (
                  <tr>
                    <td colSpan={4} className="py-4 px-4 text-center text-gray-500 dark:text-gray-400">
                      No tables found.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </>
      )}
    </section>
  );
}
```

- [ ] **Step 3: Render the new section on the page**

In the same file, change the end of the default export:

```typescript
        <SegmentDistribution />
      </div>
    </Layout>
  );
}
```

to:

```typescript
        <SegmentDistribution />

        <DatabaseSize />
      </div>
    </Layout>
  );
}
```

- [ ] **Step 4: Typecheck and lint**

Run: `cd frontend && npx tsc --noEmit && npm run lint`
Expected: no errors.

- [ ] **Step 5: Manually verify in the browser**

Run: `cd frontend && npm run dev` (with the Task 1 backend running against real Postgres in another terminal).

Open `http://localhost:3000/admin` and confirm:
- The existing "Sync Backlog" and "Segment Distribution" sections still render as before (no regression).
- A new "Database Size" section appears below them with a total-size stat card and a per-table breakdown table, sorted largest-first, with sensible non-zero values.
- Toggle browser dark mode and confirm the new section is legible in both themes (matches the existing `dark:` classes already used elsewhere on the page).

- [ ] **Step 6: Commit**

```bash
git add frontend/src/pages/admin/index.tsx
git commit -m "feat(admin): render database size section on admin page"
```

## Self-Review Notes

- **Spec coverage:** Total DB size stat card (Task 4 Step 2), per-table breakdown with name/size/%/rows (Task 4 Step 2), backend endpoint + route (Task 1), service/hook (Task 3), Postgres-only SQL with no SQLite branching (Task 1 Step 3), error-path test given CI constraints (Task 1 Steps 1-2), manual verification since no real-Postgres CI exists (Task 2, Task 4 Step 5) — all spec sections have a corresponding task.
- **Placeholder scan:** no TBD/TODO; every step has literal code or an exact command.
- **Type consistency:** `AdminTableSizeRow`/`AdminDatabaseSizeResponse` field names (`table_name`, `size_bytes`, `row_estimate`, `total_bytes`, `tables`) are identical across the Go struct tags (Task 1), the TypeScript interfaces (Task 3), and the JSX usage (Task 4). `useAdminDatabaseSize()`'s returned shape (`databaseSize`, `isLoading`, `error`) matches its usage in Task 4 Step 2 exactly.
