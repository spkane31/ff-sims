# Admin Backlog Fetch-Age Buckets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a bucketed breakdown of current-season league transaction-fetch staleness (never fetched, then six 4-hour buckets 0–24h, then a 24h+ catch-all) to `/admin`, rendered as a new table inside the Discovery Frontier section, plus an explanatory caption for that section's existing leagues-by-season table.

**Architecture:** Backend: `GetAdminBacklog` (`backend/internal/api/handlers/admin.go`) gains one more query that buckets current-season, non-skipped leagues by `last_transactions_fetched_at` age via a single SQL `GROUP BY`, using Go-computed timestamp thresholds as bind parameters (portable across Postgres and the SQLite test fake — no `NOW()`/`INTERVAL`/`julianday()`). A pure `fillBacklogBuckets` helper zero-fills any bucket label the query didn't return, in a fixed display order. Frontend: `AdminBacklogResponse` gains a `buckets` field; `/admin`'s `DiscoveryFrontier` component takes `backlog` as a prop (already fetched by the page) and renders the new table plus both captions.

**Tech Stack:** Go + Gin + GORM (raw SQL) on the backend, tested against an in-memory SQLite fake; Next.js + React + TypeScript + Tailwind on the frontend, no frontend test infra (matches existing precedent for this page — verified manually in-browser).

## Global Constraints

- Bucket labels, fixed order: `Never fetched`, `0h-3h59m`, `4h-7h59m`, `8h-11h59m`, `12h-15h59m`, `16h-19h59m`, `20h-23h59m`, `24h+`.
- Scope: current season only (`season = MAX(season) AND skipped_at IS NULL`), matching the existing Sync Backlog stat cards.
- No dialect-specific date math in SQL — bucket-boundary timestamps computed in Go, passed as bind parameters, compared with plain `>`.
- Buckets are always returned in the fixed order with zero counts included (never omitted), so the frontend table always renders all 8 rows.
- New table is placed inside the existing `DiscoveryFrontier` section/component, sourced from `backlog` data passed down as a prop — not a new fetch.
- Both the existing leagues-by-season table and the new bucket table get their own short caption paragraphs (per user confirmation).

---

### Task 1: `fillBacklogBuckets` helper + `AdminBacklogBucketRow` type

**Files:**
- Modify: `backend/internal/api/handlers/admin.go`
- Test: `backend/internal/api/handlers/admin_test.go`

**Interfaces:**
- Produces: `type AdminBacklogBucketRow struct { Label string; Leagues int64 }` (JSON tags `label`, `leagues`), `var backlogBucketLabels []string` (the fixed 8-label order), `func fillBacklogBuckets(rows []AdminBacklogBucketRow) []AdminBacklogBucketRow`.

- [ ] **Step 1: Write the failing tests**

Add to `backend/internal/api/handlers/admin_test.go` (after the existing `TestGetAdminBacklog_EmptyTable`, before `TestGetAdminDatabaseSize_RequiresPostgres`):

```go
func TestFillBacklogBuckets_ZeroFillsMissingLabels(t *testing.T) {
	rows := []AdminBacklogBucketRow{
		{Label: "24h+", Leagues: 3},
		{Label: "Never fetched", Leagues: 5},
	}

	filled := fillBacklogBuckets(rows)

	want := []AdminBacklogBucketRow{
		{Label: "Never fetched", Leagues: 5},
		{Label: "0h-3h59m", Leagues: 0},
		{Label: "4h-7h59m", Leagues: 0},
		{Label: "8h-11h59m", Leagues: 0},
		{Label: "12h-15h59m", Leagues: 0},
		{Label: "16h-19h59m", Leagues: 0},
		{Label: "20h-23h59m", Leagues: 0},
		{Label: "24h+", Leagues: 3},
	}
	if len(filled) != len(want) {
		t.Fatalf("expected %d buckets, got %d", len(want), len(filled))
	}
	for i, w := range want {
		if filled[i] != w {
			t.Errorf("index %d: expected %+v, got %+v", i, w, filled[i])
		}
	}
}

func TestFillBacklogBuckets_EmptyInput(t *testing.T) {
	filled := fillBacklogBuckets(nil)

	if len(filled) != len(backlogBucketLabels) {
		t.Fatalf("expected %d buckets, got %d", len(backlogBucketLabels), len(filled))
	}
	for i, row := range filled {
		if row.Leagues != 0 {
			t.Errorf("index %d: expected 0 leagues, got %d", i, row.Leagues)
		}
		if row.Label != backlogBucketLabels[i] {
			t.Errorf("index %d: expected label %q, got %q", i, backlogBucketLabels[i], row.Label)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/api/handlers/... -run TestFillBacklogBuckets -v`
Expected: FAIL — build error, `AdminBacklogBucketRow`, `fillBacklogBuckets`, and `backlogBucketLabels` are undefined.

- [ ] **Step 3: Implement the type, labels, and helper**

In `backend/internal/api/handlers/admin.go`, add immediately after the `AdminBacklogResponse` struct (after line 22, before the `AdminSegmentRow` comment):

```go
// AdminBacklogBucketRow is one fetch-age bucket for current-season leagues,
// ordered from "never fetched" through "24h+".
type AdminBacklogBucketRow struct {
	Label   string `json:"label"`
	Leagues int64  `json:"leagues"`
}

// backlogBucketLabels is the fixed display order for AdminBacklogBucketRow,
// from "never fetched" through "24h+".
var backlogBucketLabels = []string{
	"Never fetched", "0h-3h59m", "4h-7h59m", "8h-11h59m",
	"12h-15h59m", "16h-19h59m", "20h-23h59m", "24h+",
}

// fillBacklogBuckets reorders a sparse (possibly out-of-order) set of bucket
// rows onto the fixed backlogBucketLabels sequence, zero-filling any label
// with no matching rows.
func fillBacklogBuckets(rows []AdminBacklogBucketRow) []AdminBacklogBucketRow {
	counts := make(map[string]int64, len(rows))
	for _, r := range rows {
		counts[r.Label] = r.Leagues
	}

	filled := make([]AdminBacklogBucketRow, len(backlogBucketLabels))
	for i, label := range backlogBucketLabels {
		filled[i] = AdminBacklogBucketRow{Label: label, Leagues: counts[label]}
	}
	return filled
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/api/handlers/... -run TestFillBacklogBuckets -v`
Expected: PASS (both `TestFillBacklogBuckets_ZeroFillsMissingLabels` and `TestFillBacklogBuckets_EmptyInput`).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/api/handlers/admin.go backend/internal/api/handlers/admin_test.go
git commit -m "Add fillBacklogBuckets helper for admin backlog fetch-age buckets"
```

---

### Task 2: Wire bucket query into `GetAdminBacklog`

**Files:**
- Modify: `backend/internal/api/handlers/admin.go`
- Test: `backend/internal/api/handlers/admin_test.go`

**Interfaces:**
- Consumes: `AdminBacklogBucketRow`, `fillBacklogBuckets` (Task 1).
- Produces: `AdminBacklogResponse.Buckets []AdminBacklogBucketRow` (JSON key `buckets`), populated by `GetAdminBacklog`.

- [ ] **Step 1: Write the failing tests**

Add to `backend/internal/api/handlers/admin_test.go`, directly after `TestGetAdminBacklog_ExcludesSkipped` (before `TestGetAdminBacklog_EmptyTable`):

```go
func TestGetAdminBacklog_Buckets(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	now := time.Now().UTC()
	at := func(d time.Duration) *time.Time {
		ts := now.Add(d)
		return &ts
	}

	db.Create(&models.SleeperLeague{SleeperLeagueID: "never", Season: "2026"})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "b0", Season: "2026", LastTransactionsFetchedAt: at(-1 * time.Hour)})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "b4", Season: "2026", LastTransactionsFetchedAt: at(-5 * time.Hour)})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "b8", Season: "2026", LastTransactionsFetchedAt: at(-9 * time.Hour)})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "b12", Season: "2026", LastTransactionsFetchedAt: at(-13 * time.Hour)})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "b16", Season: "2026", LastTransactionsFetchedAt: at(-17 * time.Hour)})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "b20", Season: "2026", LastTransactionsFetchedAt: at(-21 * time.Hour)})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "b24", Season: "2026", LastTransactionsFetchedAt: at(-30 * time.Hour)})

	resp := performGetAdminBacklog(t)

	if len(resp.Buckets) != 8 {
		t.Fatalf("expected 8 buckets, got %d", len(resp.Buckets))
	}

	wantOrder := []string{
		"Never fetched", "0h-3h59m", "4h-7h59m", "8h-11h59m",
		"12h-15h59m", "16h-19h59m", "20h-23h59m", "24h+",
	}
	for i, label := range wantOrder {
		if resp.Buckets[i].Label != label {
			t.Errorf("index %d: expected label %q, got %q", i, label, resp.Buckets[i].Label)
		}
		if resp.Buckets[i].Leagues != 1 {
			t.Errorf("bucket %q: expected 1 league, got %d", label, resp.Buckets[i].Leagues)
		}
	}
}

func TestGetAdminBacklog_BucketsExcludeOtherSeasonsAndSkipped(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	now := time.Now().UTC()
	skippedAt := now
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-2026", Season: "2026", LastTransactionsFetchedAt: &now})
	db.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg-2026-skipped", Season: "2026", LastTransactionsFetchedAt: &now, SkippedAt: &skippedAt,
	})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-2025", Season: "2025", LastTransactionsFetchedAt: &now})

	resp := performGetAdminBacklog(t)

	if resp.Season != "2026" {
		t.Fatalf("expected season 2026, got %q", resp.Season)
	}

	var total int64
	for _, row := range resp.Buckets {
		total += row.Leagues
	}
	if total != 1 {
		t.Errorf("expected 1 league counted across buckets (excluding other season + skipped), got %d", total)
	}
}
```

Then modify the existing `TestGetAdminBacklog_EmptyTable` (in the same file) to also assert the buckets are zero-filled. Replace:

```go
	if resp.OldestTransactionsFetchedAt != nil {
		t.Error("expected nil oldest fetch timestamp for empty table")
	}
}
```

with:

```go
	if resp.OldestTransactionsFetchedAt != nil {
		t.Error("expected nil oldest fetch timestamp for empty table")
	}
	if len(resp.Buckets) != 8 {
		t.Fatalf("expected 8 buckets, got %d", len(resp.Buckets))
	}
	for _, row := range resp.Buckets {
		if row.Leagues != 0 {
			t.Errorf("bucket %q: expected 0 leagues, got %d", row.Label, row.Leagues)
		}
	}
}
```

(This is the end of `TestGetAdminBacklog_EmptyTable` — the closing brace shown is the function's own closing brace.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/api/handlers/... -run TestGetAdminBacklog -v`
Expected: FAIL — `resp.Buckets` is undefined on `AdminBacklogResponse` (build error).

- [ ] **Step 3: Implement**

In `backend/internal/api/handlers/admin.go`, add a `Buckets` field to `AdminBacklogResponse`:

```go
type AdminBacklogResponse struct {
	Season                      string                  `json:"season"`
	TotalLeagues                int64                   `json:"total_leagues"`
	NeverFetchedCount           int64                   `json:"never_fetched_count"`
	OldestTransactionsFetchedAt *time.Time              `json:"oldest_transactions_fetched_at"`
	Buckets                     []AdminBacklogBucketRow `json:"buckets"`
}
```

Then, in `GetAdminBacklog`, replace the final two lines (`c.JSON(http.StatusOK, resp)` and its blank line before, i.e. everything after the `oldestLeague` block) with:

```go
	now := time.Now()
	const bucketQ = `
		SELECT
			CASE
				WHEN last_transactions_fetched_at IS NULL THEN 'Never fetched'
				WHEN last_transactions_fetched_at > ? THEN '0h-3h59m'
				WHEN last_transactions_fetched_at > ? THEN '4h-7h59m'
				WHEN last_transactions_fetched_at > ? THEN '8h-11h59m'
				WHEN last_transactions_fetched_at > ? THEN '12h-15h59m'
				WHEN last_transactions_fetched_at > ? THEN '16h-19h59m'
				WHEN last_transactions_fetched_at > ? THEN '20h-23h59m'
				ELSE '24h+'
			END AS label,
			COUNT(*) AS leagues
		FROM sleeper_leagues
		WHERE season = ? AND skipped_at IS NULL
		GROUP BY label`

	bucketRows := []AdminBacklogBucketRow{}
	if err := database.DB.Raw(bucketQ,
		now.Add(-4*time.Hour), now.Add(-8*time.Hour), now.Add(-12*time.Hour),
		now.Add(-16*time.Hour), now.Add(-20*time.Hour), now.Add(-24*time.Hour),
		season,
	).Scan(&bucketRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp.Buckets = fillBacklogBuckets(bucketRows)

	c.JSON(http.StatusOK, resp)
}
```

So the full end of `GetAdminBacklog` (from the `oldestLeague` check onward) reads:

```go
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

	now := time.Now()
	const bucketQ = `
		SELECT
			CASE
				WHEN last_transactions_fetched_at IS NULL THEN 'Never fetched'
				WHEN last_transactions_fetched_at > ? THEN '0h-3h59m'
				WHEN last_transactions_fetched_at > ? THEN '4h-7h59m'
				WHEN last_transactions_fetched_at > ? THEN '8h-11h59m'
				WHEN last_transactions_fetched_at > ? THEN '12h-15h59m'
				WHEN last_transactions_fetched_at > ? THEN '16h-19h59m'
				WHEN last_transactions_fetched_at > ? THEN '20h-23h59m'
				ELSE '24h+'
			END AS label,
			COUNT(*) AS leagues
		FROM sleeper_leagues
		WHERE season = ? AND skipped_at IS NULL
		GROUP BY label`

	bucketRows := []AdminBacklogBucketRow{}
	if err := database.DB.Raw(bucketQ,
		now.Add(-4*time.Hour), now.Add(-8*time.Hour), now.Add(-12*time.Hour),
		now.Add(-16*time.Hour), now.Add(-20*time.Hour), now.Add(-24*time.Hour),
		season,
	).Scan(&bucketRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp.Buckets = fillBacklogBuckets(bucketRows)

	c.JSON(http.StatusOK, resp)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/api/handlers/... -run TestGetAdminBacklog -v`
Expected: PASS for all `TestGetAdminBacklog_*` tests, including the two new ones and the extended `TestGetAdminBacklog_EmptyTable`.

Then run the full handler package to make sure nothing else broke:

Run: `cd backend && go test ./internal/api/handlers/...`
Expected: PASS (`ok backend/internal/api/handlers ...`).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/api/handlers/admin.go backend/internal/api/handlers/admin_test.go
git commit -m "Add fetch-age bucket breakdown to GetAdminBacklog"
```

---

### Task 3: Frontend types for the bucket data

**Files:**
- Modify: `frontend/src/services/adminService.ts`

**Interfaces:**
- Consumes: JSON shape produced by Task 2 (`buckets: [{ label, leagues }, ...]` on the backlog response).
- Produces: `AdminBacklogBucketRow` TypeScript interface, `AdminBacklogResponse.buckets: AdminBacklogBucketRow[]`.

- [ ] **Step 1: Add the interface and field**

In `frontend/src/services/adminService.ts`, replace:

```ts
export interface AdminBacklogResponse {
  season: string;
  total_leagues: number;
  never_fetched_count: number;
  oldest_transactions_fetched_at: string | null;
}
```

with:

```ts
export interface AdminBacklogBucketRow {
  label: string;
  leagues: number;
}

export interface AdminBacklogResponse {
  season: string;
  total_leagues: number;
  never_fetched_count: number;
  oldest_transactions_fetched_at: string | null;
  buckets: AdminBacklogBucketRow[];
}
```

- [ ] **Step 2: Type-check**

Run: `cd frontend && npx tsc --noEmit`
Expected: no new errors (this file has no other consumers yet that would break — `useAdminBacklog.ts` just passes the type through, and `admin/index.tsx` isn't updated until Task 4).

- [ ] **Step 3: Commit**

```bash
git add frontend/src/services/adminService.ts
git commit -m "Add AdminBacklogBucketRow type to admin service"
```

---

### Task 4: Render the bucket table and captions on `/admin`

**Files:**
- Modify: `frontend/src/pages/admin/index.tsx`

**Interfaces:**
- Consumes: `AdminBacklogResponse` (with `buckets`, Task 3), `useAdminBacklog()` (unchanged, already imported).
- Produces: `DiscoveryFrontier` now takes a `{ backlog: AdminBacklogResponse | null }` prop.

- [ ] **Step 1: Import the response type**

In `frontend/src/pages/admin/index.tsx`, add to the top imports:

```tsx
import { AdminBacklogResponse } from "../../services/adminService";
```

- [ ] **Step 2: Thread `backlog` into `DiscoveryFrontier`**

Change the function signature from:

```tsx
function DiscoveryFrontier() {
  const { frontier, isLoading, error } = useAdminDiscoveryFrontier();
```

to:

```tsx
function DiscoveryFrontier({ backlog }: { backlog: AdminBacklogResponse | null }) {
  const { frontier, isLoading, error } = useAdminDiscoveryFrontier();
```

- [ ] **Step 3: Add the caption and the new bucket table**

Inside `DiscoveryFrontier`, find this exact block (the end of the `leagues_by_season` table and the fragment/section that wraps it):

```tsx
                {frontier.leagues_by_season.length === 0 && (
                  <tr>
                    <td colSpan={6} className="py-4 px-4 text-center text-gray-500 dark:text-gray-400">
                      No leagues discovered yet.
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

Replace it with:

```tsx
                {frontier.leagues_by_season.length === 0 && (
                  <tr>
                    <td colSpan={6} className="py-4 px-4 text-center text-gray-500 dark:text-gray-400">
                      No leagues discovered yet.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          <p className="text-sm text-gray-500 dark:text-gray-400 mt-2">
            Total is every league discovered that season; Expanded means the discovery workflow
            has fetched it (<code>last_fetched_at</code> set); Pending is discovered but not yet
            expanded — the frontier left to crawl; Skipped is permanently excluded and doesn&apos;t
            count toward pending.
          </p>

          <h3 className="text-xl font-semibold text-gray-800 dark:text-gray-100 mt-8 mb-2">
            Transaction Fetch Age (season {backlog?.season || "—"})
          </h3>

          <div className="bg-white dark:bg-gray-700 rounded-lg shadow-md border border-gray-100 dark:border-gray-600 overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
              <thead className="bg-gray-50 dark:bg-gray-800">
                <tr>
                  <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Bucket
                  </th>
                  <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Leagues
                  </th>
                  <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    % of Total
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
                {backlog && backlog.total_leagues > 0 ? (
                  backlog.buckets.map((row) => (
                    <tr key={row.label}>
                      <td className="py-2 px-4 text-gray-800 dark:text-gray-100">{row.label}</td>
                      <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                        {row.leagues.toLocaleString()}
                      </td>
                      <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                        {`${((row.leagues / backlog.total_leagues) * 100).toFixed(1)}%`}
                      </td>
                    </tr>
                  ))
                ) : (
                  <tr>
                    <td colSpan={3} className="py-4 px-4 text-center text-gray-500 dark:text-gray-400">
                      No leagues yet.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          <p className="text-sm text-gray-500 dark:text-gray-400 mt-2">
            How stale each current-season league&apos;s transaction sync is, bucketed in 4-hour
            increments, to help gauge how much to scale the Temporal workers.
          </p>
        </>
      )}
    </section>
  );
}
```

- [ ] **Step 4: Pass `backlog` at the call site**

In the default-exported `AdminBacklog` page component, change:

```tsx
        <DiscoveryFrontier />
```

to:

```tsx
        <DiscoveryFrontier backlog={backlog} />
```

(`backlog` is already in scope from the existing `const { backlog, isLoading, error } = useAdminBacklog();` at the top of `AdminBacklog`.)

- [ ] **Step 5: Type-check**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 6: Lint**

Run: `cd frontend && npm run lint`
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/pages/admin/index.tsx
git commit -m "Show transaction fetch-age buckets and captions on /admin"
```

---

### Task 5: Manual verification in the browser

**Files:** none (verification only).

- [ ] **Step 1: Start the backend against a local Postgres**

Run: `cd backend && make run`
Expected: server starts and logs it's listening (check `backend/cmd/server` config for the DB connection it expects — use whatever local Postgres is already configured for this repo).

- [ ] **Step 2: Start the frontend dev server**

Run: `cd frontend && npm run dev`
Expected: Next.js dev server starts on its default port.

- [ ] **Step 3: Load `/admin` and inspect the Discovery Frontier section**

Open `/admin` in a browser. Confirm:
- The existing leagues-by-season table still renders as before, now followed by the explanatory caption paragraph.
- A new "Transaction Fetch Age (season …)" heading and table appear below it, with 8 rows (`Never fetched` through `24h+`) and a caption below the table.
- If the local database has no current-season leagues, the table shows a single "No leagues yet." row instead of erroring.
- Dark mode (toggle OS/browser dark mode or however this repo's dark mode is triggered) renders both new captions and the table legibly.

- [ ] **Step 4: Confirm no regressions in the rest of the page**

Confirm Sync Backlog stat cards, Segment Distribution, Database Size, and the Discovery Frontier stat cards still render exactly as before — only the two additions (caption + new table) should be new.
