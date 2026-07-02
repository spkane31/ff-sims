# Admin Backlog Endpoint Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a read-only `GET /api/v1/admin/backlog` endpoint and a matching `/admin` frontend
page reporting how many current-season leagues have never had Sleeper transactions fetched, and
the oldest `last_transactions_fetched_at` among the ones that have — sized for eyeballing Temporal
worker backlog without a manual SQL session.

**Architecture:** One new Go handler (`internal/api/handlers/admin.go`) queries
`models.SleeperLeague` directly via the package-level `database.DB`, following the exact pattern
of every other handler in that package (no service layer, no new package). One new route group in
`internal/api/routes.go`. On the frontend, one service function + one hook + one page, following
the existing `leaguesService.ts` / `useLeagues.ts` / `pages/index.tsx` pattern exactly.

**Tech Stack:** Go, Gin, GORM (Postgres in prod, sqlite in-memory for tests), Next.js/React/TypeScript.

## Global Constraints

- No auth/access control on the new route or page (matches existing app-wide posture).
- Read-only — no mutation/trigger actions (see spec's `expected_wins.go` precedent).
- Single hardcoded endpoint, not a generic query framework.
- "Current season" = `MAX(season)` over `sleeper_leagues`, not a live Sleeper API call.
- No nav link added to `Header.tsx` — page is reached via direct URL `/admin`.

Full design context: `docs/superpowers/specs/2026-07-02-admin-backlog-endpoint-design.md`.

---

### Task 1: Backend — `GET /api/v1/admin/backlog` handler, route, tests

**Files:**
- Create: `backend/internal/api/handlers/admin.go`
- Create: `backend/internal/api/handlers/admin_test.go`
- Modify: `backend/internal/api/routes.go`

**Interfaces:**
- Produces: `handlers.AdminBacklogResponse` struct (`Season string`, `TotalLeagues int64`,
  `NeverFetchedCount int64`, `OldestTransactionsFetchedAt *time.Time`, all with matching `json`
  tags), and `handlers.GetAdminBacklog(c *gin.Context)` — a `gin.HandlerFunc`. Task 2 (frontend)
  consumes the JSON shape this produces, not the Go types directly.

- [x] **Step 1: Write the failing tests**

Create `backend/internal/api/handlers/admin_test.go`:

```go
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"backend/internal/database"
	"backend/internal/models"
)

func newAdminTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.SleeperLeague{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func withAdminTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })
}

func performGetAdminBacklog(t *testing.T) AdminBacklogResponse {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin/backlog", GetAdminBacklog)

	req := httptest.NewRequest(http.MethodGet, "/admin/backlog", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp AdminBacklogResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return resp
}

func TestGetAdminBacklog_MixedFetchState(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	now := time.Now().UTC().Truncate(time.Second)
	older := now.Add(-48 * time.Hour)

	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-never", Season: "2026"})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-recent", Season: "2026", LastTransactionsFetchedAt: &now})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-old", Season: "2026", LastTransactionsFetchedAt: &older})
	// different (older) season — must not be counted in the 2026 totals
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-2025", Season: "2025", LastTransactionsFetchedAt: &now})

	resp := performGetAdminBacklog(t)

	if resp.Season != "2026" {
		t.Errorf("expected season 2026, got %q", resp.Season)
	}
	if resp.TotalLeagues != 3 {
		t.Errorf("expected 3 leagues in 2026, got %d", resp.TotalLeagues)
	}
	if resp.NeverFetchedCount != 1 {
		t.Errorf("expected 1 never-fetched, got %d", resp.NeverFetchedCount)
	}
	if resp.OldestTransactionsFetchedAt == nil {
		t.Fatal("expected non-nil oldest fetch timestamp")
	}
	if !resp.OldestTransactionsFetchedAt.Equal(older) {
		t.Errorf("expected oldest fetch %v, got %v", older, *resp.OldestTransactionsFetchedAt)
	}
}

func TestGetAdminBacklog_AllNeverFetched(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-a", Season: "2026"})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-b", Season: "2026"})

	resp := performGetAdminBacklog(t)

	if resp.TotalLeagues != 2 || resp.NeverFetchedCount != 2 {
		t.Errorf("expected 2/2 never fetched, got total=%d never=%d", resp.TotalLeagues, resp.NeverFetchedCount)
	}
	if resp.OldestTransactionsFetchedAt != nil {
		t.Errorf("expected nil oldest fetch timestamp, got %v", *resp.OldestTransactionsFetchedAt)
	}
}

func TestGetAdminBacklog_ExcludesSkipped(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	skippedAt := time.Now().UTC()
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-skipped", Season: "2026", SkippedAt: &skippedAt})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-active", Season: "2026"})

	resp := performGetAdminBacklog(t)

	if resp.TotalLeagues != 1 {
		t.Errorf("expected 1 non-skipped league, got %d", resp.TotalLeagues)
	}
	if resp.NeverFetchedCount != 1 {
		t.Errorf("expected 1 never-fetched (excluding skipped), got %d", resp.NeverFetchedCount)
	}
}

func TestGetAdminBacklog_EmptyTable(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	resp := performGetAdminBacklog(t)

	if resp.Season != "" {
		t.Errorf("expected empty season, got %q", resp.Season)
	}
	if resp.TotalLeagues != 0 || resp.NeverFetchedCount != 0 {
		t.Errorf("expected 0/0, got total=%d never=%d", resp.TotalLeagues, resp.NeverFetchedCount)
	}
	if resp.OldestTransactionsFetchedAt != nil {
		t.Error("expected nil oldest fetch timestamp for empty table")
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/api/handlers/... -run TestGetAdminBacklog -v`
Expected: FAIL — compile error, `AdminBacklogResponse` and `GetAdminBacklog` undefined.

- [x] **Step 3: Write the handler implementation**

Create `backend/internal/api/handlers/admin.go`:

```go
package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend/internal/database"
	"backend/internal/models"
)

// AdminBacklogResponse reports the Sleeper transaction-sync backlog for the
// current season, used to size Temporal worker throughput.
type AdminBacklogResponse struct {
	Season                      string     `json:"season"`
	TotalLeagues                int64      `json:"total_leagues"`
	NeverFetchedCount           int64      `json:"never_fetched_count"`
	OldestTransactionsFetchedAt *time.Time `json:"oldest_transactions_fetched_at"`
}

// GetAdminBacklog returns how many leagues in the current season (the max
// value of sleeper_leagues.season) have never had transactions fetched, and
// the oldest last_transactions_fetched_at among the ones that have.
func GetAdminBacklog(c *gin.Context) {
	var season string
	if err := database.DB.Model(&models.SleeperLeague{}).
		Select("COALESCE(MAX(season), '')").
		Scan(&season).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp AdminBacklogResponse
	resp.Season = season

	if err := database.DB.Model(&models.SleeperLeague{}).
		Where("season = ? AND skipped_at IS NULL", season).
		Count(&resp.TotalLeagues).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := database.DB.Model(&models.SleeperLeague{}).
		Where("season = ? AND skipped_at IS NULL AND last_transactions_fetched_at IS NULL", season).
		Count(&resp.NeverFetchedCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var oldestLeague models.SleeperLeague
	err := database.DB.
		Where("season = ? AND skipped_at IS NULL AND last_transactions_fetched_at IS NOT NULL", season).
		Order("last_transactions_fetched_at ASC").
		Limit(1).
		Take(&oldestLeague).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err == nil {
		resp.OldestTransactionsFetchedAt = oldestLeague.LastTransactionsFetchedAt
	}

	c.JSON(http.StatusOK, resp)
}
```

(Note: an earlier draft of this used `Select("MIN(...)").Scan(&sql.NullTime{})`, but sqlite's
driver returns aggregate results as a raw string rather than a recognized time column, so
`sql.NullTime.Scan` fails with "unsupported Scan, storing driver.Value type string into type
*time.Time". Using `Order(...).Limit(1).Take(...)` into the actual model goes through GORM's
normal column scanning instead, which handles the type correctly — this surfaced during Step 2's
test run and was fixed before commit.)

- [x] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/api/handlers/... -run TestGetAdminBacklog -v`
Expected: PASS (all 4 tests).

- [x] **Step 5: Wire the route**

Modify `backend/internal/api/routes.go` — the file currently ends with:

```go
	sleeper := v1.Group("/sleeper")
	sleeper.GET("/stats", handlers.GetSleeperStats)
	sleeper.GET("/trades", handlers.GetSleeperTrades)
	sleeper.GET("/transactions", handlers.GetSleeperTransactions)
	sleeper.GET("/drafts", handlers.GetSleeperDrafts)
}
```

Change it to:

```go
	sleeper := v1.Group("/sleeper")
	sleeper.GET("/stats", handlers.GetSleeperStats)
	sleeper.GET("/trades", handlers.GetSleeperTrades)
	sleeper.GET("/transactions", handlers.GetSleeperTransactions)
	sleeper.GET("/drafts", handlers.GetSleeperDrafts)

	admin := v1.Group("/admin")
	admin.GET("/backlog", handlers.GetAdminBacklog)
}
```

- [x] **Step 6: Run the full backend test suite**

Run: `cd backend && go test ./...`
Expected: PASS, all packages, no regressions.

- [x] **Step 7: Commit**

```bash
git add backend/internal/api/handlers/admin.go backend/internal/api/handlers/admin_test.go backend/internal/api/routes.go
git commit -m "feat(api): add admin backlog endpoint for Sleeper transaction sync"
```

---

### Task 2: Frontend — `/admin` backlog page

**Files:**
- Create: `frontend/src/services/adminService.ts`
- Create: `frontend/src/hooks/useAdminBacklog.ts`
- Create: `frontend/src/pages/admin/index.tsx`

**Interfaces:**
- Consumes: `GET /admin/backlog` → JSON `{ season: string, total_leagues: number,
  never_fetched_count: number, oldest_transactions_fetched_at: string | null }` (produced by
  Task 1's `handlers.GetAdminBacklog`).
- Consumes: `apiClient.get<T>(endpoint: string)` from `frontend/src/services/apiClient.ts`
  (existing, unmodified).
- Consumes: `Layout` default export from `frontend/src/components/Layout.tsx` (existing).
- Produces: `adminService.getBacklog(): Promise<AdminBacklogResponse>`, and
  `useAdminBacklog(): { backlog: AdminBacklogResponse | null, isLoading: boolean, error: Error | null }`.

- [ ] **Step 1: Create the service**

Create `frontend/src/services/adminService.ts`:

```typescript
import { apiClient } from './apiClient';

export interface AdminBacklogResponse {
  season: string;
  total_leagues: number;
  never_fetched_count: number;
  oldest_transactions_fetched_at: string | null;
}

export const adminService = {
  getBacklog: async (): Promise<AdminBacklogResponse> => {
    return apiClient.get<AdminBacklogResponse>('/admin/backlog');
  },
};
```

- [ ] **Step 2: Create the hook**

Create `frontend/src/hooks/useAdminBacklog.ts`:

```typescript
import { useState, useEffect, useCallback } from "react";
import { adminService, AdminBacklogResponse } from "../services/adminService";

export function useAdminBacklog() {
  const [backlog, setBacklog] = useState<AdminBacklogResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchBacklog = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await adminService.getBacklog();
      setBacklog(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to fetch admin backlog"));
      setBacklog(null);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchBacklog();
  }, [fetchBacklog]);

  return { backlog, isLoading, error };
}
```

- [ ] **Step 3: Create the page**

Create `frontend/src/pages/admin/index.tsx`:

```tsx
import Layout from "../../components/Layout";
import { useAdminBacklog } from "../../hooks/useAdminBacklog";

function formatRelativeTime(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime();
  const diffHours = diffMs / (1000 * 60 * 60);
  if (diffHours < 1) return "less than an hour ago";
  if (diffHours < 24) return `${Math.floor(diffHours)} hour${Math.floor(diffHours) === 1 ? "" : "s"} ago`;
  const diffDays = Math.floor(diffHours / 24);
  return `${diffDays} day${diffDays === 1 ? "" : "s"} ago`;
}

export default function AdminBacklog() {
  const { backlog, isLoading, error } = useAdminBacklog();

  return (
    <Layout>
      <div className="space-y-8">
        <section>
          <h1 className="text-3xl font-bold text-blue-600 mb-2">Admin: Sync Backlog</h1>
          <p className="text-gray-600 dark:text-gray-300">
            Sleeper transaction sync backlog for the current season, used to gauge how much to
            scale the Temporal workers.
          </p>
        </section>

        {isLoading && (
          <div className="flex items-center space-x-2">
            <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin" />
            <p>Loading backlog...</p>
          </div>
        )}

        {error && <p className="text-red-600">Failed to load backlog.</p>}

        {!isLoading && !error && backlog && (
          <section className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md border border-gray-100 dark:border-gray-600">
              <div className="text-3xl font-bold text-blue-600 mb-1">
                {backlog.never_fetched_count.toLocaleString()} / {backlog.total_leagues.toLocaleString()}
              </div>
              <div className="text-lg font-medium text-gray-800 dark:text-gray-100">
                Leagues never fetched (season {backlog.season || "—"})
              </div>
            </div>

            <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md border border-gray-100 dark:border-gray-600">
              <div className="text-3xl font-bold text-blue-600 mb-1">
                {backlog.oldest_transactions_fetched_at
                  ? formatRelativeTime(backlog.oldest_transactions_fetched_at)
                  : backlog.total_leagues === 0
                    ? "No leagues"
                    : "None fetched yet"}
              </div>
              <div className="text-lg font-medium text-gray-800 dark:text-gray-100">
                Oldest transactions fetch
              </div>
            </div>
          </section>
        )}
      </div>
    </Layout>
  );
}
```

- [ ] **Step 4: Manual verification**

Run the backend and frontend dev servers, then confirm the page renders real data:

```bash
cd backend && make run &
cd frontend && npm run dev &
```

- Open `http://localhost:3000/admin` in a browser.
- Confirm it shows a season, a "never fetched / total" count, and either a relative-time oldest
  fetch, "None fetched yet", or "No leagues" (matching your local DB's actual `sleeper_leagues`
  state).
- Confirm no console errors and the loading spinner briefly appears before data loads.
- Stop both dev servers when done.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/services/adminService.ts frontend/src/hooks/useAdminBacklog.ts frontend/src/pages/admin/index.tsx
git commit -m "feat(admin): add /admin backlog page for Sleeper sync monitoring"
```
