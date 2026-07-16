# Discovery Cron Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Temporal-based `DiscoveryBatchDispatcher`/`DiscoverUsersBatch` pipeline with a plain Go process (`cmd/cron -job=discovery`) triggered hourly by a systemd timer, running for at most 50 minutes, while leaving the existing Temporal discovery path fully running unchanged.

**Architecture:** Two independent worker pools (users, leagues) each run a generic claim-batch/process/refill-at-threshold loop against Postgres (`FOR UPDATE SKIP LOCKED`), under a shared deadline context. League work is split out of user work into its own independently-claimed queue (new `sleeper_leagues.discovery_claimed_at` column) so a shared league is fetched once total, not once per member. No per-item timeouts — the Sleeper client's own request/retry bounds and the job's overall deadline are sufficient.

**Tech Stack:** Go 1.25, GORM (Postgres prod, SQLite unit tests), goose migrations, systemd (`Type=oneshot` service + `OnCalendar=hourly` timer).

## Global Constraints

- Design doc: `docs/superpowers/specs/2026-07-15-discovery-cron-migration-design.md` — every task below implements a piece of it; do not deviate from its decisions (scope: discovery only; leave all Temporal discovery code running; no per-item timeouts; claim TTL 120 minutes for both the new league column and the existing `sleeper_users.claimed_at` query; pool sizes/refill batches start small, 3-5/1-2, via `CRON_DISCOVERY_*` env vars).
- Do not delete, pause, or modify `workflows/dispatcher.go`, the `sleeper-discovery-schedule` Temporal Schedule registration, or the `sleeper-discovery` task queue worker registration in `cmd/worker/main.go`.
- All new Go code goes in a new `internal/discoverycron` package and a new `cmd/cron` binary. Existing `internal/activities/discovery.go` functions (`FetchUserLeagues`, `FetchLeagueMembers`, `FetchLeagueDetails`, `ClaimStaleUsers`) are reused as-is except where a task explicitly says to modify them.
- Follow existing patterns exactly: claim-query SQL shape (`internal/activities/discovery.go`'s `claimStaleUsersSQL`, `internal/activities/data_fetch.go`'s `claimLeaguesForTransactionsSQL`), migration shape (`backend/migrations/018_league_claims.sql`, `020_user_claims.sql`), Postgres-backed claim test shape (`internal/activities/claim_pg_test.go`), and deploy pipeline shape (`deploy/worker-host/{setup,deploy}.sh`).
- Structured logging on the new path reuses `activities.DiscoveryLogTag` (`"discovery_trace"`) as a `"tag"` field on every log line, matching the existing Temporal-path convention.

---

### Task 1: Data layer — league claim column, extended claim TTL

**Files:**
- Create: `backend/migrations/024_discovery_league_claims.sql`
- Modify: `backend/internal/models/sleeper.go` (add `DiscoveryClaimedAt` to `SleeperLeague`)
- Modify: `backend/internal/activities/discovery.go:73-83` (`claimStaleUsersSQL`: 20m -> 120m)
- Modify: `backend/internal/activities/claim_pg_test.go` (`TestClaimStaleUsers_RespectsAndExpiresClaims`: update timing to match new TTL)

**Interfaces:**
- Produces: `models.SleeperLeague.DiscoveryClaimedAt *time.Time` (column `discovery_claimed_at`) — consumed by Task 2's claim query.

- [ ] **Step 1: Write the migration**

Create `backend/migrations/024_discovery_league_claims.sql`:

```sql
-- +goose Up
-- +goose NO TRANSACTION

ALTER TABLE sleeper_leagues ADD COLUMN IF NOT EXISTS discovery_claimed_at timestamptz;

-- Serves the claim query in ClaimStaleLeagues (internal/discoverycron): filter
-- on the stale-leagues predicate (never-fetched leagues dominate the eligible
-- set at any time), order never-fetched first then oldest. NULLS FIRST
-- matches the query's ORDER BY exactly so the sort is an index walk.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_leagues_discovery_stale
    ON sleeper_leagues (last_fetched_at ASC NULLS FIRST)
    WHERE skipped_at IS NULL AND season >= '2025';

-- +goose Down
-- +goose NO TRANSACTION

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_leagues_discovery_stale;
ALTER TABLE sleeper_leagues DROP COLUMN IF EXISTS discovery_claimed_at;
```

- [ ] **Step 2: Add the model field**

In `backend/internal/models/sleeper.go`, in the `SleeperLeague` struct, add `DiscoveryClaimedAt` next to the existing `ClaimedAt`/`DraftsClaimedAt` fields:

```go
	ClaimedAt                 *time.Time `gorm:"column:claimed_at"`
	DraftsClaimedAt           *time.Time `gorm:"column:drafts_claimed_at"`
	DiscoveryClaimedAt        *time.Time `gorm:"column:discovery_claimed_at"`
	SkippedAt                 *time.Time `gorm:"column:skipped_at"`
```

- [ ] **Step 3: Extend the existing user claim TTL**

In `backend/internal/activities/discovery.go`, change the `claimStaleUsersSQL` constant's comment and interval:

```go
// claimStaleUsersSQL atomically claims up to batchSize stale users for
// discovery (same pattern as the league sync paths). FOR UPDATE SKIP LOCKED
// lets concurrent claimers partition the queue without double-claiming, and
// the 120-minute expiry re-queues users claimed by a worker that died
// mid-batch. 120 minutes (not 20) because neither claimer of this column —
// the Temporal path nor the cron path (internal/discoverycron) — imposes a
// per-item timeout shorter than that; a shorter TTL risked a still-in-flight
// user being reclaimed and processed a second time concurrently. Because
// ticks claim rather than re-select, a stuck cohort can never head-of-line-
// block the queue the way the old workflow-ID-collision dedupe did.
const claimStaleUsersSQL = `
UPDATE sleeper_users SET claimed_at = now()
WHERE sleeper_user_id IN (
    SELECT sleeper_user_id FROM sleeper_users
    WHERE skipped_at IS NULL
      AND (claimed_at IS NULL OR claimed_at < now() - interval '120 minutes')
    ORDER BY last_fetched_at ASC NULLS FIRST
    LIMIT ?
    FOR UPDATE SKIP LOCKED
)
RETURNING sleeper_user_id`
```

- [ ] **Step 4: Update the existing test that hardcodes the old 20-minute TTL**

In `backend/internal/activities/claim_pg_test.go`, `TestClaimStaleUsers_RespectsAndExpiresClaims` currently uses `stale := now.Add(-30 * time.Minute)`, which was expired under the old 20-minute TTL but would NOT be expired under the new 120-minute TTL (the test would start failing silently-wrong, not erroring). Update it:

```go
func TestClaimStaleUsers_RespectsAndExpiresClaims(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	fresh := now.Add(-1 * time.Minute)
	stale := now.Add(-150 * time.Minute)
	seedUser(t, db, models.SleeperUser{SleeperUserID: "fresh-claim", ClaimedAt: &fresh})
	seedUser(t, db, models.SleeperUser{SleeperUserID: "expired-claim", ClaimedAt: &stale})

	a := &activities.DiscoveryActivities{DB: db}
	got, err := a.ClaimStaleUsers(context.Background(), activities.ClaimStaleUsersParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(got) != 1 || got[0] != "expired-claim" {
		t.Fatalf("expected only expired-claim to be re-claimable, got %v", got)
	}
}
```

- [ ] **Step 5: Run the claim tests against Postgres**

This repo's claim tests skip without a real Postgres DB. Use the local instance from `[[user_machine_constraints]]` (initdb Postgres on `:5499`) or whatever `TEST_DATABASE_URL` is already configured on this machine:

```bash
cd backend && TEST_DATABASE_URL="postgres://localhost:5499/postgres?sslmode=disable" go test ./internal/activities/... -run TestClaimStaleUsers -v -count=1
```

Expected: PASS for `TestClaimStaleUsers_OrderingAndEligibility`, `TestClaimStaleUsers_RespectsAndExpiresClaims`, `TestClaimStaleUsers_ConcurrentClaimsAreDisjoint`.

- [ ] **Step 6: Build and run the full non-Postgres suite**

```bash
cd backend && go build ./... && go test ./... -count=1
```

Expected: builds clean, all tests pass (Postgres-only tests skip if `TEST_DATABASE_URL` isn't set in this shell).

- [ ] **Step 7: Commit**

```bash
git add backend/migrations/024_discovery_league_claims.sql backend/internal/models/sleeper.go backend/internal/activities/discovery.go backend/internal/activities/claim_pg_test.go
git commit -m "Add discovery_claimed_at league claim column; extend claim TTL to 120m

Per the discovery cron migration design: a new independently-claimed
league work queue needs its own column (claimed_at and drafts_claimed_at
on sleeper_leagues already belong to transaction-sync and draft-sync
respectively). The claim TTL on sleeper_users.claimed_at moves from 20m to
120m because neither the existing Temporal path nor the new cron path
imposes a per-item timeout shorter than that."
```

---

### Task 2: League claim query

**Files:**
- Create: `backend/internal/discoverycron/claim.go`
- Create: `backend/internal/discoverycron/claim_pg_test.go`

**Interfaces:**
- Consumes: `models.SleeperLeague` (Task 1).
- Produces: `discoverycron.ClaimStaleLeagues(ctx context.Context, db *gorm.DB, batchSize int) ([]string, error)` — consumed by Task 5's `RunDiscovery`.

- [ ] **Step 1: Write the failing tests**

Create `backend/internal/discoverycron/claim_pg_test.go`, mirroring `internal/activities/claim_pg_test.go`'s shape exactly:

```go
package discoverycron_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"backend/internal/discoverycron"
	"backend/internal/models"
	"backend/internal/testutil"
)

func newPGTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; claim tests need Postgres (FOR UPDATE SKIP LOCKED)")
	}
	scopedDSN := testutil.NewPGSchema(t, dsn, "discoverycron_claim_test")
	db := testutil.OpenGORM(t, scopedDSN)
	if err := db.AutoMigrate(&models.SleeperLeague{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func seedLeague(t *testing.T, db *gorm.DB, l models.SleeperLeague) {
	t.Helper()
	if l.Season == "" {
		l.Season = "2026"
	}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed league %s: %v", l.SleeperLeagueID, err)
	}
}

func TestClaimStaleLeagues_OrderingAndEligibility(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour)
	recent := now.Add(-1 * time.Hour)
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "never"})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "oldest", LastFetchedAt: &old})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "recent", LastFetchedAt: &recent})

	got, err := discoverycron.ClaimStaleLeagues(context.Background(), db, 2)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	claimed := map[string]bool{}
	for _, id := range got {
		claimed[id] = true
	}
	if len(got) != 2 || !claimed["never"] || !claimed["oldest"] {
		t.Fatalf("expected {never, oldest}, got %v", got)
	}
	var stamped int64
	db.Model(&models.SleeperLeague{}).Where("discovery_claimed_at IS NOT NULL").Count(&stamped)
	if stamped != 2 {
		t.Errorf("expected 2 rows stamped discovery_claimed_at, got %d", stamped)
	}
}

func TestClaimStaleLeagues_ExcludesIneligible(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "skipped", SkippedAt: &now})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "old-season", Season: "2024"})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "done-complete", Status: "complete", LastFetchedAt: &now})
	// complete but never actually detail-fetched: still eligible (matches
	// leagueFullySynced's own condition: complete AND last_fetched_at set).
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "complete-unfetched", Status: "complete"})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "in-season-fetched", Status: "in_season", LastFetchedAt: &now})

	got, err := discoverycron.ClaimStaleLeagues(context.Background(), db, 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	claimed := map[string]bool{}
	for _, id := range got {
		claimed[id] = true
	}
	for _, want := range []string{"complete-unfetched", "in-season-fetched"} {
		if !claimed[want] {
			t.Errorf("expected %s to be claimed", want)
		}
	}
	for _, no := range []string{"skipped", "old-season", "done-complete"} {
		if claimed[no] {
			t.Errorf("expected %s NOT to be claimed", no)
		}
	}
}

func TestClaimStaleLeagues_RespectsAndExpiresClaims(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	fresh := now.Add(-1 * time.Minute)
	stale := now.Add(-150 * time.Minute)
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "fresh-claim", DiscoveryClaimedAt: &fresh})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "expired-claim", DiscoveryClaimedAt: &stale})
	// A transactions claim must not block a discovery claim (separate columns).
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "txn-claimed", ClaimedAt: &fresh})

	got, err := discoverycron.ClaimStaleLeagues(context.Background(), db, 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	claimed := map[string]bool{}
	for _, id := range got {
		claimed[id] = true
	}
	if len(got) != 2 || !claimed["expired-claim"] || !claimed["txn-claimed"] {
		t.Fatalf("expected {expired-claim, txn-claimed}, got %v", got)
	}
}

func TestClaimStaleLeagues_ConcurrentClaimsAreDisjoint(t *testing.T) {
	db := newPGTestDB(t)
	for i := 0; i < 20; i++ {
		seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: fmt.Sprintf("lg%02d", i)})
	}

	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := discoverycron.ClaimStaleLeagues(context.Background(), db, 10)
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range got {
				seen[id]++
			}
		}()
	}
	wg.Wait()
	if len(seen) != 20 {
		t.Errorf("expected 20 distinct leagues claimed, got %d", len(seen))
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("league %s claimed %d times", id, n)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd backend && TEST_DATABASE_URL="postgres://localhost:5499/postgres?sslmode=disable" go test ./internal/discoverycron/... -v -count=1
```

Expected: FAIL — `package discoverycron: no Go files` / `undefined: discoverycron.ClaimStaleLeagues`.

- [ ] **Step 3: Write the claim query**

Create `backend/internal/discoverycron/claim.go`:

```go
// Package discoverycron replaces the Temporal-based discovery pipeline
// (workflows.DiscoveryBatchDispatcher / activities.DiscoverUsersBatch) with
// a plain Go implementation driven by a systemd timer instead of a Temporal
// Schedule. See docs/superpowers/specs/2026-07-15-discovery-cron-migration-design.md
// for the design and docs/superpowers/plans/2026-07-15-discovery-cron-migration.md
// for how it was built. Both paths run concurrently against the same claim
// queues for now — this package does not touch the existing Temporal code.
package discoverycron

import (
	"context"

	"gorm.io/gorm"
)

// claimStaleLeaguesSQL atomically claims up to batchSize leagues needing
// discovery's member/detail fetch (mirrors activities.claimStaleUsersSQL's
// shape). Leagues already complete-and-fetched are excluded from the query
// itself — matches activities.leagueFullySynced's condition, but applied
// before claiming rather than after, so a complete league never occupies a
// pool slot at all. season >= '2025' matches
// activities.firstScannedSeason — discovery never creates older rows, but
// this table can carry historical rows from other sources.
const claimStaleLeaguesSQL = `
UPDATE sleeper_leagues SET discovery_claimed_at = now()
WHERE sleeper_league_id IN (
    SELECT sleeper_league_id FROM sleeper_leagues
    WHERE skipped_at IS NULL
      AND season >= '2025'
      AND NOT (status = 'complete' AND last_fetched_at IS NOT NULL)
      AND (discovery_claimed_at IS NULL OR discovery_claimed_at < now() - interval '120 minutes')
    ORDER BY last_fetched_at ASC NULLS FIRST
    LIMIT ?
    FOR UPDATE SKIP LOCKED
)
RETURNING sleeper_league_id`

// ClaimStaleLeagues claims up to batchSize leagues for discovery's league
// pool, never-fetched first then oldest. Postgres-only (SKIP LOCKED).
func ClaimStaleLeagues(ctx context.Context, db *gorm.DB, batchSize int) ([]string, error) {
	var ids []string
	if err := db.WithContext(ctx).Raw(claimStaleLeaguesSQL, batchSize).Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd backend && TEST_DATABASE_URL="postgres://localhost:5499/postgres?sslmode=disable" go test ./internal/discoverycron/... -v -count=1
```

Expected: PASS for all four tests.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/discoverycron/claim.go backend/internal/discoverycron/claim_pg_test.go
git commit -m "Add ClaimStaleLeagues for the discovery cron's league pool"
```

---

### Task 3: ProcessUser and ProcessLeague

**Files:**
- Modify: `backend/internal/activities/discovery.go:267-272` (export `isNotFoundAppError` -> `IsNotFoundAppError`)
- Create: `backend/internal/discoverycron/process.go`
- Create: `backend/internal/discoverycron/process_test.go`

**Interfaces:**
- Consumes: `activities.DiscoveryActivities{DB, Sleeper}`, `activities.FetchUserLeaguesParams`, `activities.FetchLeagueMembersParams`, `activities.FetchLeagueDetailsParams`, `activities.IsNotFoundAppError(err) bool` (renamed in this task).
- Produces: `discoverycron.ProcessUser(ctx context.Context, da *activities.DiscoveryActivities, userID string) error` and `discoverycron.ProcessLeague(ctx context.Context, da *activities.DiscoveryActivities, leagueID string) error` — both consumed by Task 5's `RunDiscovery`.

- [ ] **Step 1: Export the not-found error check**

In `backend/internal/activities/discovery.go`, rename `isNotFoundAppError` to `IsNotFoundAppError` (it has exactly one call site, inside `discoverOneUser`):

```go
// IsNotFoundAppError reports whether err is the NOT_FOUND application error
// produced by the fetch helpers when a Sleeper entity no longer exists.
func IsNotFoundAppError(err error) bool {
	var appErr *temporal.ApplicationError
	return errors.As(err, &appErr) && appErr.Type() == "NOT_FOUND"
}
```

And update its one call site (inside `discoverOneUser`, around line 208):

```go
		if IsNotFoundAppError(err) {
```

Run `cd backend && go build ./...` to confirm nothing else references the old lowercase name — expected: clean build.

- [ ] **Step 2: Write the failing tests**

Create `backend/internal/discoverycron/process_test.go`. This mirrors `internal/activities/discovery_test.go`'s httptest-server pattern (SQLite DB, fake Sleeper server):

```go
package discoverycron_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/activities"
	"backend/internal/discoverycron"
	"backend/internal/models"
	"backend/internal/sleeper"
)

func newSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("unwrap sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.SleeperUser{}, &models.SleeperLeague{}, &models.SleeperLeagueUser{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestProcessUser_DiscoversLeaguesAndStamps(t *testing.T) {
	db := newSQLiteDB(t)
	db.Create(&models.SleeperUser{SleeperUserID: "user1"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]sleeper.League{
			{LeagueID: "lg1", Name: "Test League", Season: "2026", Sport: "nfl", Status: "in_season"},
		})
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := discoverycron.ProcessUser(context.Background(), da, "user1"); err != nil {
		t.Fatalf("ProcessUser error: %v", err)
	}

	var u models.SleeperUser
	db.First(&u, "sleeper_user_id = ?", "user1")
	if u.LastFetchedAt == nil {
		t.Error("expected user stamped last_fetched_at")
	}

	var lg models.SleeperLeague
	db.First(&lg, "sleeper_league_id = ?", "lg1")
	if lg.LastFetchedAt != nil {
		t.Error("expected league NOT to have members/details fetched yet — that's league-pool work now")
	}
	if lg.DiscoveryClaimedAt != nil {
		t.Error("newly discovered league should not be claimed yet")
	}
}

func TestProcessUser_NotFoundMarksSkipped(t *testing.T) {
	db := newSQLiteDB(t)
	db.Create(&models.SleeperUser{SleeperUserID: "gone"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := discoverycron.ProcessUser(context.Background(), da, "gone"); err != nil {
		t.Fatalf("ProcessUser error: %v", err)
	}

	var u models.SleeperUser
	db.First(&u, "sleeper_user_id = ?", "gone")
	if u.SkippedAt == nil {
		t.Error("expected user marked skipped")
	}
	if u.LastFetchedAt != nil {
		t.Error("skipped user must not be stamped fetched")
	}
}

func TestProcessLeague_FetchesMembersAndDetailsAtomically(t *testing.T) {
	db := newSQLiteDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/users") {
			json.NewEncoder(w).Encode([]sleeper.LeagueUser{{UserID: "u1", Username: "alice"}})
			return
		}
		json.NewEncoder(w).Encode(sleeper.League{LeagueID: "lg1", Name: "L", Status: "in_season", TotalRosters: 12})
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := discoverycron.ProcessLeague(context.Background(), da, "lg1"); err != nil {
		t.Fatalf("ProcessLeague error: %v", err)
	}

	var u models.SleeperUser
	if err := db.First(&u, "sleeper_user_id = ?", "u1").Error; err != nil {
		t.Fatalf("expected member u1 to be upserted: %v", err)
	}

	var lg models.SleeperLeague
	db.First(&lg, "sleeper_league_id = ?", "lg1")
	if lg.LastFetchedAt == nil {
		t.Error("expected league details stamped")
	}
	if lg.DiscoveryClaimedAt != nil {
		t.Error("expected discovery_claimed_at cleared on success")
	}
}

func TestProcessLeague_DetailsFailureRollsBackMembers(t *testing.T) {
	db := newSQLiteDB(t)
	claimedAt := time.Now().UTC()
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1", DiscoveryClaimedAt: &claimedAt})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/users") {
			json.NewEncoder(w).Encode([]sleeper.LeagueUser{{UserID: "u1", Username: "alice"}})
			return
		}
		w.WriteHeader(http.StatusBadRequest) // details call fails
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := discoverycron.ProcessLeague(context.Background(), da, "lg1"); err == nil {
		t.Fatal("expected an error when the details fetch fails")
	}

	var count int64
	db.Model(&models.SleeperUser{}).Where("sleeper_user_id = ?", "u1").Count(&count)
	if count != 0 {
		t.Error("expected the member upsert to roll back when details fetch fails within the same transaction")
	}

	var lg models.SleeperLeague
	db.First(&lg, "sleeper_league_id = ?", "lg1")
	if lg.DiscoveryClaimedAt == nil {
		t.Error("expected discovery_claimed_at to remain set (claim stays in place) after a failed attempt")
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

```bash
cd backend && go test ./internal/discoverycron/... -run "TestProcessUser|TestProcessLeague" -v -count=1
```

Expected: FAIL — `undefined: discoverycron.ProcessUser` / `undefined: discoverycron.ProcessLeague`.

- [ ] **Step 4: Write the implementation**

Create `backend/internal/discoverycron/process.go`:

```go
package discoverycron

import (
	"context"
	"time"

	"gorm.io/gorm"

	"backend/internal/activities"
	"backend/internal/models"
)

// ProcessUser fetches userID's leagues across configured seasons and upserts
// them (activities.FetchUserLeagues already does the league-row + junction-
// row upserts), then stamps the user done. Unlike the Temporal path's
// discoverOneUser, this does not fetch league members/details inline — that
// is ProcessLeague's job now, claimed independently, so a league shared by
// many users is fetched once total instead of once per member.
func ProcessUser(ctx context.Context, da *activities.DiscoveryActivities, userID string) error {
	_, err := da.FetchUserLeagues(ctx, activities.FetchUserLeaguesParams{UserID: userID})
	if err != nil {
		if activities.IsNotFoundAppError(err) {
			return da.DB.WithContext(ctx).
				Model(&models.SleeperUser{}).
				Where("sleeper_user_id = ?", userID).
				Updates(map[string]interface{}{
					"skipped_at": time.Now().UTC(),
					"claimed_at": nil,
				}).Error
		}
		return err
	}

	return da.DB.WithContext(ctx).
		Model(&models.SleeperUser{}).
		Where("sleeper_user_id = ?", userID).
		Updates(map[string]interface{}{
			"last_fetched_at": time.Now().UTC(),
			"claimed_at":      nil,
		}).Error
}

// ProcessLeague fetches leagueID's members and details and writes both in a
// single DB transaction, then clears discovery_claimed_at. Wrapping both
// fetches in one transaction means a details-fetch failure (e.g. Sleeper
// returns an error after members already upserted successfully) leaves no
// partial state — either both land or neither does, and the claim stays in
// place for a later retry either way.
func ProcessLeague(ctx context.Context, da *activities.DiscoveryActivities, leagueID string) error {
	return da.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txDA := &activities.DiscoveryActivities{DB: tx, Sleeper: da.Sleeper}
		if err := txDA.FetchLeagueMembers(ctx, activities.FetchLeagueMembersParams{LeagueID: leagueID}); err != nil {
			return err
		}
		if err := txDA.FetchLeagueDetails(ctx, activities.FetchLeagueDetailsParams{LeagueID: leagueID}); err != nil {
			return err
		}
		return tx.Model(&models.SleeperLeague{}).
			Where("sleeper_league_id = ?", leagueID).
			Update("discovery_claimed_at", nil).Error
	})
}
```

- [ ] **Step 5: Run the tests to verify they pass**

```bash
cd backend && go test ./internal/discoverycron/... -run "TestProcessUser|TestProcessLeague" -v -count=1
```

Expected: PASS for all four tests.

- [ ] **Step 6: Run the full suite**

```bash
cd backend && go build ./... && go test ./... -count=1
```

Expected: clean build, all tests pass.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/activities/discovery.go backend/internal/discoverycron/process.go backend/internal/discoverycron/process_test.go
git commit -m "Add ProcessUser and ProcessLeague for the discovery cron pools

League work (members + details) is now claimed and processed
independently of user work, written atomically in one DB transaction.
Exports activities.IsNotFoundAppError so the new package can share the
same 404-handling convention as the Temporal path's discoverOneUser."
```

---

### Task 4: Generic pool runner

**Files:**
- Create: `backend/internal/discoverycron/pool.go`
- Create: `backend/internal/discoverycron/pool_test.go`

**Interfaces:**
- Produces: `discoverycron.PoolConfig{Size, RefillBatch, PollInterval int/time.Duration}`, `discoverycron.PoolResult{Processed, Failed int}`, `discoverycron.RunPool(ctx context.Context, cfg PoolConfig, claim func(context.Context, int) ([]string, error), process func(context.Context, string) error, onResult func(id string, err error, duration time.Duration)) PoolResult` — consumed by Task 5's `RunDiscovery`.

- [ ] **Step 1: Write the failing tests**

Create `backend/internal/discoverycron/pool_test.go`:

```go
package discoverycron_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"backend/internal/discoverycron"
)

// fakeQueue is a simple in-memory claimable queue for testing RunPool
// without a database.
type fakeQueue struct {
	mu    sync.Mutex
	ids   []string
	claim int32 // number of claim() calls, for busy-loop assertions
}

func newFakeQueue(n int) *fakeQueue {
	q := &fakeQueue{}
	for i := 0; i < n; i++ {
		q.ids = append(q.ids, fmt.Sprintf("item%d", i))
	}
	return q
}

func (q *fakeQueue) claimFn(ctx context.Context, n int) ([]string, error) {
	atomic.AddInt32(&q.claim, 1)
	q.mu.Lock()
	defer q.mu.Unlock()
	if n > len(q.ids) {
		n = len(q.ids)
	}
	got := q.ids[:n]
	q.ids = q.ids[n:]
	return got, nil
}

func TestRunPool_ProcessesAllItemsAndReportsCounts(t *testing.T) {
	q := newFakeQueue(10)
	var processed sync.Map
	process := func(ctx context.Context, id string) error {
		processed.Store(id, true)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res := discoverycron.RunPool(ctx, discoverycron.PoolConfig{Size: 3, RefillBatch: 1, PollInterval: 5 * time.Millisecond},
		q.claimFn, process, func(string, error, time.Duration) {})

	if res.Processed != 10 || res.Failed != 0 {
		t.Fatalf("expected 10 processed / 0 failed, got %+v", res)
	}
	count := 0
	processed.Range(func(k, v any) bool { count++; return true })
	if count != 10 {
		t.Errorf("expected 10 distinct items processed, got %d", count)
	}
}

func TestRunPool_RefillOnlyTriggersAtThreshold(t *testing.T) {
	q := newFakeQueue(6)
	block := make(chan struct{})
	var startedCount int32
	process := func(ctx context.Context, id string) error {
		atomic.AddInt32(&startedCount, 1)
		<-block // hold every item open until the test releases them
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan discoverycron.PoolResult, 1)
	go func() {
		done <- discoverycron.RunPool(ctx, discoverycron.PoolConfig{Size: 4, RefillBatch: 4, PollInterval: 5 * time.Millisecond},
			q.claimFn, process, func(string, error, time.Duration) {})
	}()

	// RefillBatch=4 with pool size 4: the very first claim should ask for up
	// to 4 (all slots free), then no further claim should happen until 4
	// slots free up again — never a partial refill of e.g. 1 or 2.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && atomic.LoadInt32(&startedCount) < 4 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&startedCount); got != 4 {
		t.Fatalf("expected exactly 4 items claimed before any slot freed, got %d", got)
	}

	close(block)
	cancel()
	<-done
}

func TestRunPool_EmptyClaimDoesNotBusyLoop(t *testing.T) {
	q := newFakeQueue(0)
	process := func(ctx context.Context, id string) error { return nil }

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	discoverycron.RunPool(ctx, discoverycron.PoolConfig{Size: 3, RefillBatch: 1, PollInterval: 20 * time.Millisecond},
		q.claimFn, process, func(string, error, time.Duration) {})

	// 100ms / 20ms poll interval should yield roughly 5 claim attempts, not
	// hundreds — proves the loop sleeps between empty claims instead of
	// spinning.
	if got := atomic.LoadInt32(&q.claim); got > 15 {
		t.Errorf("expected a bounded number of claim attempts on an empty queue, got %d", got)
	}
}

func TestRunPool_DrainsInFlightWorkOnDeadline(t *testing.T) {
	q := newFakeQueue(1)
	started := make(chan struct{})
	finished := make(chan struct{})
	process := func(ctx context.Context, id string) error {
		close(started)
		<-ctx.Done() // simulate work that respects the shared ctx
		close(finished)
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan discoverycron.PoolResult, 1)
	go func() {
		done <- discoverycron.RunPool(ctx, discoverycron.PoolConfig{Size: 2, RefillBatch: 1, PollInterval: 5 * time.Millisecond},
			q.claimFn, process, func(string, error, time.Duration) {})
	}()

	<-started
	cancel()

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("expected in-flight work to be allowed to finish after ctx cancellation")
	}
	res := <-done
	if res.Failed != 1 {
		t.Fatalf("expected the cancelled item to count as failed, got %+v", res)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd backend && go test ./internal/discoverycron/... -run TestRunPool -v -count=1
```

Expected: FAIL — `undefined: discoverycron.RunPool` / `discoverycron.PoolConfig`.

- [ ] **Step 3: Write the implementation**

Create `backend/internal/discoverycron/pool.go`:

```go
package discoverycron

import (
	"context"
	"time"
)

// defaultPollInterval is used when PoolConfig.PollInterval is unset. It
// bounds how often RunPool re-queries the claim function when the pool is
// below its refill threshold or the last claim came back empty — long
// enough not to hammer the database on an empty queue, short enough that a
// freshly-claimable item doesn't sit idle for long once the pool is behind.
const defaultPollInterval = 2 * time.Second

// PoolConfig sizes one RunPool call.
type PoolConfig struct {
	// Size is the maximum number of items processed concurrently.
	Size int
	// RefillBatch is how many pool slots must be free before RunPool claims
	// more work. Claiming in batches (rather than one-for-one as each slot
	// frees) keeps the number of claim queries bounded as Size scales up.
	RefillBatch int
	// PollInterval is how long RunPool waits before re-checking when it's
	// below RefillBatch free slots or the last claim was empty. Defaults to
	// defaultPollInterval if zero.
	PollInterval time.Duration
}

// PoolResult summarizes one RunPool call.
type PoolResult struct {
	Processed int
	Failed    int
}

type itemResult struct {
	id       string
	err      error
	duration time.Duration
}

// RunPool claims and processes work items until ctx is done, then waits for
// any still-in-flight items to finish before returning. claim(ctx, n) should
// return up to n item IDs (fewer, or none, if the queue is short right now).
// process(ctx, id) handles one item; a non-nil return is recorded as a
// failure but does not stop the pool or retry the item here — the caller's
// claim mechanism (a DB claim with a TTL, in production use) is what makes a
// failed item eligible again later. onResult is called once per completed
// item (success or failure) for logging.
//
// No per-item timeout is imposed here — see
// docs/superpowers/specs/2026-07-15-discovery-cron-migration-design.md's
// Concurrency model section for why. process is expected to respect ctx
// itself (the Sleeper client's calls already do).
func RunPool(
	ctx context.Context,
	cfg PoolConfig,
	claim func(ctx context.Context, n int) ([]string, error),
	process func(ctx context.Context, id string) error,
	onResult func(id string, err error, duration time.Duration),
) PoolResult {
	size := max(1, cfg.Size)
	refillBatch := max(1, cfg.RefillBatch)
	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	var res PoolResult
	results := make(chan itemResult, size)
	inFlight := 0

	record := func(r itemResult) {
		inFlight--
		if r.err != nil {
			res.Failed++
		} else {
			res.Processed++
		}
		onResult(r.id, r.err, r.duration)
	}

	drainNonBlocking := func() {
		for {
			select {
			case r := <-results:
				record(r)
			default:
				return
			}
		}
	}

	for ctx.Err() == nil {
		drainNonBlocking()
		free := size - inFlight
		if free < refillBatch {
			select {
			case r := <-results:
				record(r)
			case <-time.After(pollInterval):
			case <-ctx.Done():
			}
			continue
		}

		ids, err := claim(ctx, free)
		if err != nil || len(ids) == 0 {
			select {
			case <-time.After(pollInterval):
			case <-ctx.Done():
			}
			continue
		}

		for _, id := range ids {
			inFlight++
			go func(id string) {
				start := time.Now()
				err := process(ctx, id)
				results <- itemResult{id: id, err: err, duration: time.Since(start)}
			}(id)
		}
	}

	for inFlight > 0 {
		record(<-results)
	}
	return res
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd backend && go test ./internal/discoverycron/... -run TestRunPool -v -count=1
```

Expected: PASS for all four tests. `TestRunPool_DrainsInFlightWorkOnDeadline` should take just over 0 seconds (bounded by the cancellation propagating, not a fixed sleep). If `TestRunPool_RefillOnlyTriggersAtThreshold` is flaky, increase its 500ms polling deadline — the assertion (`startedCount == 4` before any release) should not itself be flaky since `block` isn't closed until after the loop confirms count.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/discoverycron/pool.go backend/internal/discoverycron/pool_test.go
git commit -m "Add RunPool: generic claim-batch/process/refill-at-threshold loop"
```

---

### Task 5: RunDiscovery job entrypoint + config

**Files:**
- Create: `backend/internal/discoverycron/discovery.go`
- Create: `backend/internal/discoverycron/discovery_test.go`

**Interfaces:**
- Consumes: `discoverycron.ClaimStaleLeagues`, `discoverycron.ProcessUser`, `discoverycron.ProcessLeague`, `discoverycron.RunPool`/`PoolConfig`/`PoolResult` (Tasks 2-4), `activities.DiscoveryActivities`, `activities.DiscoveryLogTag`, `activities.ClaimStaleUsersParams`, `activities.ClaimStaleUsers`.
- Produces: `discoverycron.Config{UserPoolSize, UserRefillBatch, LeaguePoolSize, LeagueRefillBatch int}`, `discoverycron.LoadConfig() Config`, `discoverycron.Report{UsersProcessed, UsersFailed, LeaguesProcessed, LeaguesFailed int}`, `discoverycron.RunDiscovery(ctx context.Context, da *activities.DiscoveryActivities, cfg Config) (Report, error)` — consumed by Task 6's `cmd/cron`.

- [ ] **Step 1: Write the failing test**

Create `backend/internal/discoverycron/discovery_test.go`:

```go
package discoverycron_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"backend/internal/activities"
	"backend/internal/discoverycron"
	"backend/internal/models"
	"backend/internal/sleeper"
)

func TestRunDiscovery_ProcessesUsersAndLeaguesToCompletion(t *testing.T) {
	db := newSQLiteDB(t)
	db.Create(&models.SleeperUser{SleeperUserID: "user1"})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-existing"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case parts[1] == "user":
			json.NewEncoder(w).Encode([]sleeper.League{{LeagueID: "lg-new", Season: "2026", Sport: "nfl"}})
		case strings.HasSuffix(r.URL.Path, "/users"):
			json.NewEncoder(w).Encode([]sleeper.LeagueUser{})
		default:
			json.NewEncoder(w).Encode(sleeper.League{LeagueID: parts[2]})
		}
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	cfg := discoverycron.Config{UserPoolSize: 2, UserRefillBatch: 1, LeaguePoolSize: 2, LeagueRefillBatch: 1}

	// The job's own deadline stands in for cmd/cron's -max-duration; give it
	// enough headroom to drain a tiny fixture, then rely on RunDiscovery
	// itself detecting an empty queue rather than the deadline firing.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	report, err := discoverycron.RunDiscovery(ctx, da, cfg)
	if err != nil {
		t.Fatalf("RunDiscovery error: %v", err)
	}
	if report.UsersProcessed != 1 || report.UsersFailed != 0 {
		t.Errorf("expected 1 user processed, got %+v", report)
	}
	if report.LeaguesProcessed < 2 {
		t.Errorf("expected both lg-existing and lg-new (discovered mid-run) processed, got %+v", report)
	}

	var newLeague models.SleeperLeague
	if err := db.First(&newLeague, "sleeper_league_id = ?", "lg-new").Error; err != nil {
		t.Fatalf("expected lg-new to have been discovered and claimed by the league pool: %v", err)
	}
	if newLeague.LastFetchedAt == nil {
		t.Error("expected lg-new's details to have been fetched")
	}
}
```

This test's queue is empty after processing the one user and two leagues, so `RunDiscovery` needs a defined way to return before its `ctx` deadline once both pools have nothing left to do — see the implementation step below (`RunDiscovery` returns once both pools' `RunPool` calls return, and `RunPool` only returns when `ctx` is done; for this test to complete before its 3s timeout, `RunDiscovery` must pass a shared ctx that both pools use, and the test's small fixture combined with a short poll interval is what keeps this fast — the pools spin on empty claims until `ctx` expires, exactly like production). Confirm this by checking the actual runtime in step 3 below is close to the poll interval you configure, not the full 3s timeout.

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd backend && go test ./internal/discoverycron/... -run TestRunDiscovery -v -count=1
```

Expected: FAIL — `undefined: discoverycron.Config` / `discoverycron.RunDiscovery`.

- [ ] **Step 3: Write the implementation**

Create `backend/internal/discoverycron/discovery.go`:

```go
package discoverycron

import (
	"context"
	"sync"
	"time"

	"backend/internal/activities"
	"backend/internal/helpers"
)

// pollInterval is deliberately short (not defaultPollInterval's 2s) so a
// production run notices newly-claimable work quickly, and so this
// package's own tests (which run against tiny in-memory fixtures) finish in
// well under a second instead of waiting out a multi-second poll cadence.
const pollInterval = 200 * time.Millisecond

// Config holds the discovery cron job's tuning knobs, read from env. Uses a
// CRON_DISCOVERY_ prefix (distinct from the Temporal path's DISCOVERY_*
// vars, which remain in effect for workflows.DiscoveryBatchDispatcher) so
// the two paths can be tuned independently while both run.
type Config struct {
	UserPoolSize      int // CRON_DISCOVERY_USER_POOL_SIZE, default 4
	UserRefillBatch   int // CRON_DISCOVERY_USER_REFILL_BATCH, default 2
	LeaguePoolSize    int // CRON_DISCOVERY_LEAGUE_POOL_SIZE, default 4
	LeagueRefillBatch int // CRON_DISCOVERY_LEAGUE_REFILL_BATCH, default 2
}

// LoadConfig reads Config from env, clamped to at least 1 so a bad value
// can't stall the pools or break a claim query's LIMIT.
func LoadConfig() Config {
	return Config{
		UserPoolSize:      max(helpers.GetEnv("CRON_DISCOVERY_USER_POOL_SIZE", 4), 1),
		UserRefillBatch:   max(helpers.GetEnv("CRON_DISCOVERY_USER_REFILL_BATCH", 2), 1),
		LeaguePoolSize:    max(helpers.GetEnv("CRON_DISCOVERY_LEAGUE_POOL_SIZE", 4), 1),
		LeagueRefillBatch: max(helpers.GetEnv("CRON_DISCOVERY_LEAGUE_REFILL_BATCH", 2), 1),
	}
}

// Report summarizes one RunDiscovery call.
type Report struct {
	UsersProcessed   int
	UsersFailed      int
	LeaguesProcessed int
	LeaguesFailed    int
}

// RunDiscovery runs the user pool and league pool concurrently until ctx is
// done (the caller — cmd/cron — sets ctx's deadline to -max-duration), then
// returns a summary. Each pool claims and processes items independently;
// see RunPool for the claim/process/refill loop shared by both.
func RunDiscovery(ctx context.Context, da *activities.DiscoveryActivities, cfg Config) (Report, error) {
	logger := newStdLogger()
	logger.Info("discovery cron starting", "tag", activities.DiscoveryLogTag,
		"userPoolSize", cfg.UserPoolSize, "userRefillBatch", cfg.UserRefillBatch,
		"leaguePoolSize", cfg.LeaguePoolSize, "leagueRefillBatch", cfg.LeagueRefillBatch)
	start := time.Now()

	var userResult, leagueResult PoolResult
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		userResult = RunPool(ctx,
			PoolConfig{Size: cfg.UserPoolSize, RefillBatch: cfg.UserRefillBatch, PollInterval: pollInterval},
			func(ctx context.Context, n int) ([]string, error) {
				return da.ClaimStaleUsers(ctx, activities.ClaimStaleUsersParams{BatchSize: n})
			},
			func(ctx context.Context, id string) error {
				return ProcessUser(ctx, da, id)
			},
			func(id string, err error, duration time.Duration) {
				logResult(logger, "user", id, err, duration)
			},
		)
	}()

	go func() {
		defer wg.Done()
		leagueResult = RunPool(ctx,
			PoolConfig{Size: cfg.LeaguePoolSize, RefillBatch: cfg.LeagueRefillBatch, PollInterval: pollInterval},
			func(ctx context.Context, n int) ([]string, error) {
				return ClaimStaleLeagues(ctx, da.DB, n)
			},
			func(ctx context.Context, id string) error {
				return ProcessLeague(ctx, da, id)
			},
			func(id string, err error, duration time.Duration) {
				logResult(logger, "league", id, err, duration)
			},
		)
	}()

	wg.Wait()

	report := Report{
		UsersProcessed:   userResult.Processed,
		UsersFailed:      userResult.Failed,
		LeaguesProcessed: leagueResult.Processed,
		LeaguesFailed:    leagueResult.Failed,
	}
	logger.Info("discovery cron finished", "tag", activities.DiscoveryLogTag,
		"duration", time.Since(start),
		"usersProcessed", report.UsersProcessed, "usersFailed", report.UsersFailed,
		"leaguesProcessed", report.LeaguesProcessed, "leaguesFailed", report.LeaguesFailed)
	return report, nil
}

func logResult(logger *stdLogger, kind, id string, err error, duration time.Duration) {
	if err != nil {
		logger.Warn(kind+" failed", "tag", activities.DiscoveryLogTag, "id", id, "error", err, "duration", duration)
		return
	}
	logger.Info(kind+" completed", "tag", activities.DiscoveryLogTag, "id", id, "duration", duration)
}
```

- [ ] **Step 4: Add the small stdlib-backed logger**

`RunDiscovery` above references `newStdLogger`/`stdLogger`, a minimal structured logger over `log.Printf` — no Temporal SDK dependency, since this package must run standalone under `cmd/cron`. Create `backend/internal/discoverycron/log.go`:

```go
package discoverycron

import (
	"fmt"
	"log"
	"strings"
)

// stdLogger is a minimal key-value logger over the standard library's log
// package, matching the shape (message + alternating key/value pairs) that
// activities.DiscoveryActivities' Temporal-based logging already uses, so
// discovery_trace-tagged log lines look the same regardless of which path
// produced them.
type stdLogger struct{}

func newStdLogger() *stdLogger { return &stdLogger{} }

func (l *stdLogger) Info(msg string, kv ...any)  { l.log("INFO", msg, kv) }
func (l *stdLogger) Warn(msg string, kv ...any)  { l.log("WARN", msg, kv) }
func (l *stdLogger) Error(msg string, kv ...any) { l.log("ERROR", msg, kv) }

func (l *stdLogger) log(level, msg string, kv []any) {
	var b strings.Builder
	b.WriteString(level)
	b.WriteString("  ")
	b.WriteString(msg)
	for i := 0; i+1 < len(kv); i += 2 {
		fmt.Fprintf(&b, " %v=%v", kv[i], kv[i+1])
	}
	log.Println(b.String())
}
```

- [ ] **Step 5: Run the test to verify it passes**

```bash
cd backend && go test ./internal/discoverycron/... -run TestRunDiscovery -v -count=1
```

Expected: PASS. If it hangs near the 3-second deadline instead of returning quickly, check that `pollInterval` (200ms) is being used — a hang usually means one pool's `RunPool` never sees `ctx.Err() != nil` because a goroutine leaked without reading from `results`; re-check `RunPool`'s `for ctx.Err() == nil` loop condition from Task 4.

- [ ] **Step 6: Run the full suite**

```bash
cd backend && go build ./... && go vet ./... && go test ./... -count=1
```

Expected: clean build, `go vet` clean, all tests pass.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/discoverycron/discovery.go backend/internal/discoverycron/log.go backend/internal/discoverycron/discovery_test.go
git commit -m "Add RunDiscovery: wires the user and league pools together

CRON_DISCOVERY_* env config, independent of the Temporal path's
DISCOVERY_* vars so both can be tuned separately while both run."
```

---

### Task 6: cmd/cron binary

**Files:**
- Create: `backend/cmd/cron/main.go`
- Create: `backend/cmd/cron/main_test.go`

**Interfaces:**
- Consumes: `discoverycron.LoadConfig()`, `discoverycron.RunDiscovery`, `config.Load()`, `database.Initialize(cfg)`, `database.DB`, `sleeper.New()`, `activities.DiscoveryActivities`.

- [ ] **Step 1: Write the failing test for job resolution**

`main()` itself isn't unit-testable directly (it calls `os.Exit`), so extract the job-lookup logic into a small testable function. Create `backend/cmd/cron/main_test.go`:

```go
package main

import (
	"context"
	"errors"
	"testing"
)

func TestResolveJob_KnownJobReturnsItsFunc(t *testing.T) {
	called := false
	registry := map[string]func(context.Context) error{
		"discovery": func(context.Context) error { called = true; return nil },
	}
	fn, err := resolveJob(registry, "discovery")
	if err != nil {
		t.Fatalf("resolveJob error: %v", err)
	}
	if err := fn(context.Background()); err != nil {
		t.Fatalf("job func error: %v", err)
	}
	if !called {
		t.Error("expected the registered job function to run")
	}
}

func TestResolveJob_UnknownJobErrorsCleanly(t *testing.T) {
	registry := map[string]func(context.Context) error{
		"discovery": func(context.Context) error { return nil },
	}
	_, err := resolveJob(registry, "does-not-exist")
	if err == nil {
		t.Fatal("expected an error for an unregistered job name")
	}
	if !errors.Is(err, errUnknownJob) {
		t.Errorf("expected errUnknownJob, got %v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd backend && go test ./cmd/cron/... -v -count=1
```

Expected: FAIL — `undefined: resolveJob` / `undefined: errUnknownJob`.

- [ ] **Step 3: Write the implementation**

Create `backend/cmd/cron/main.go`:

```go
// cmd/cron is a generic scheduled-job runner: it takes a job name and a
// max-duration, runs the matching job under a deadline context, and exits.
// It's the replacement entrypoint for pipelines migrated off Temporal — see
// docs/superpowers/specs/2026-07-15-discovery-cron-migration-design.md.
// Currently registers exactly one job ("discovery"); adding another
// (draft-sync, transaction-sync, etc., when their turn comes) is a matter of
// registering another function in the registry built in main(), not
// restructuring this file.
package main

import (
	"context"
	"errors"
	"flag"
	"log"

	"backend/internal/activities"
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/discoverycron"
	"backend/internal/sleeper"
)

// buildID identifies the commit this binary was built from. Set via
// -ldflags "-X main.buildID=<git short SHA>" in the worker host's build
// paths (deploy/worker-host/{deploy,setup}.sh), matching cmd/worker's
// convention.
var buildID = "dev"

var errUnknownJob = errors.New("unknown job")

// resolveJob looks up name in registry, returning errUnknownJob (wrapped
// with the attempted name) if it isn't registered.
func resolveJob(registry map[string]func(context.Context) error, name string) (func(context.Context) error, error) {
	fn, ok := registry[name]
	if !ok {
		return nil, errors.New(name + ": " + errUnknownJob.Error())
	}
	return fn, nil
}

func main() {
	jobName := flag.String("job", "", "job to run (see registry in main.go)")
	maxDuration := flag.Duration("max-duration", 0, "hard deadline for the job, e.g. 50m")
	flag.Parse()

	if *jobName == "" {
		log.Fatal("missing required -job flag")
	}
	if *maxDuration <= 0 {
		log.Fatal("missing required -max-duration flag (e.g. -max-duration=50m)")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := database.Initialize(cfg); err != nil {
		log.Fatalf("db connect: %v", err)
	}

	sc := sleeper.New()
	da := &activities.DiscoveryActivities{DB: database.DB, Sleeper: sc}

	registry := map[string]func(context.Context) error{
		"discovery": func(ctx context.Context) error {
			_, err := discoverycron.RunDiscovery(ctx, da, discoverycron.LoadConfig())
			return err
		},
	}

	fn, err := resolveJob(registry, *jobName)
	if err != nil {
		log.Fatalf("resolve job: %v", err)
	}

	log.Printf("cmd/cron starting: job=%s max_duration=%s build_id=%s", *jobName, *maxDuration, buildID)
	ctx, cancel := context.WithTimeout(context.Background(), *maxDuration)
	defer cancel()

	if err := fn(ctx); err != nil {
		log.Fatalf("job %s failed: %v", *jobName, err)
	}
	log.Printf("cmd/cron finished: job=%s", *jobName)
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
cd backend && go test ./cmd/cron/... -v -count=1
```

Expected: PASS for both tests.

- [ ] **Step 5: Build the binary and smoke-test the flag errors**

```bash
cd backend && go build -o /tmp/cron ./cmd/cron
/tmp/cron
echo "exit code: $?"
```

Expected: prints `missing required -job flag` and exits non-zero (via `log.Fatal`).

```bash
DATABASE_URL="postgres://localhost:5499/postgres?sslmode=disable" /tmp/cron -job=does-not-exist -max-duration=1s
echo "exit code: $?"
```

Expected: connects to the DB, then prints `resolve job: does-not-exist: unknown job` and exits non-zero. (If there's no local Postgres reachable at that URL, this step will fail earlier at `db connect` instead — that's fine, it's still confirming the flag/job-resolution path runs before hitting `RunDiscovery`; adjust the DSN to whatever `TEST_DATABASE_URL`/local Postgres is available per `[[user_machine_constraints]]`.)

- [ ] **Step 6: Commit**

```bash
git add backend/cmd/cron/main.go backend/cmd/cron/main_test.go
git commit -m "Add cmd/cron: generic job runner, discovery registered as its first job"
```

---

### Task 7: systemd units + deploy pipeline

**Files:**
- Create: `deploy/worker-host/ff-sims-discovery.service`
- Create: `deploy/worker-host/ff-sims-discovery.timer`
- Modify: `deploy/worker-host/setup.sh`
- Modify: `deploy/worker-host/deploy.sh`
- Modify: `deploy/worker-host/tests/test_deploy.sh`
- Modify: `deploy/worker-host/README.md`

- [ ] **Step 1: Write the new systemd unit files**

Create `deploy/worker-host/ff-sims-discovery.service`:

```ini
[Unit]
Description=ff-sims discovery cron job
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User={{SERVICE_USER}}
WorkingDirectory={{REPO_DIR}}/backend
EnvironmentFile=/etc/ff-sims-worker.env
ExecStart={{REPO_DIR}}/backend/cron -job=discovery -max-duration=50m
```

Create `deploy/worker-host/ff-sims-discovery.timer`:

```ini
[Unit]
Description=Run ff-sims-discovery hourly

[Timer]
OnBootSec=2min
OnCalendar=hourly
Unit=ff-sims-discovery.service

[Install]
WantedBy=timers.target
```

- [ ] **Step 2: Extend setup.sh to build and install the new binary/units**

In `deploy/worker-host/setup.sh`, modify `first_build` to also build `cmd/cron`:

```bash
first_build() {
  echo "Building worker binary"
  local sha
  sha="$(git -C "$REPO_DIR" rev-parse --short=9 HEAD)"
  (cd "$REPO_DIR/backend" && /usr/local/go/bin/go build -ldflags "-X 'main.buildID=${sha}' -X 'main.promoteOnStart=true'" -o worker ./cmd/worker)
  echo "Building cron binary"
  (cd "$REPO_DIR/backend" && /usr/local/go/bin/go build -ldflags "-X 'main.buildID=${sha}'" -o cron ./cmd/cron)
}
```

Modify `install_units` to also install the two new unit files:

```bash
install_units() {
  echo "Installing systemd units"
  for unit in ff-sims-worker.service ff-sims-deploy.service ff-sims-deploy.timer ff-sims-discovery.service ff-sims-discovery.timer; do
    sed "s#{{REPO_DIR}}#${REPO_DIR}#g; s#{{SERVICE_USER}}#${SERVICE_USER}#g" \
      "$SCRIPT_DIR/$unit" > "$SYSTEMD_DIR/$unit"
  done
  systemctl daemon-reload
}
```

Modify `main()` to also enable/start the new timer (the discovery service itself is `Type=oneshot`, triggered by its timer — it's never `systemctl start`ed directly the way `ff-sims-worker.service` is):

```bash
main() {
  ensure_go
  ensure_service_user
  disable_sleep
  first_build
  install_units

  if ensure_env_file; then
    systemctl enable ff-sims-worker.service ff-sims-deploy.timer ff-sims-discovery.timer
    systemctl start ff-sims-worker.service ff-sims-deploy.timer ff-sims-discovery.timer
  else
    echo "Skipping service start until $ENV_FILE is filled in."
  fi

  print_summary
}
```

Modify `print_summary` to mention the new logs entry point:

```bash
print_summary() {
  local ip
  ip="$(curl -4 -fsSL ifconfig.me || echo "<could not detect>")"
  cat <<EOF

Setup complete.

Worker host public IP: ${ip}
  -> Add this IP to the Postgres managed database's trusted sources
     in the DigitalOcean dashboard if you haven't already.

Logs:
  journalctl -u ff-sims-worker -f      # Temporal worker logs (drafts, transactions, etc.)
  journalctl -u ff-sims-deploy         # deploy-check history
  journalctl -u ff-sims-discovery -f   # discovery cron job logs (runs hourly)
EOF
}
```

- [ ] **Step 3: Extend deploy.sh to also build the cron binary each cycle**

In `deploy/worker-host/deploy.sh`, add a `build_cron` function alongside `build_worker`, and call it from `install_and_restart`. `cmd/cron` is a one-shot binary with no running service to restart — swapping the file in place is enough, the next timer firing picks it up:

```bash
build_worker() {
  local sha
  sha="$(git -C "$REPO_DIR" rev-parse --short=9 HEAD)"
  (cd "$REPO_DIR/backend" && "$GO_BIN" build -ldflags "-X 'main.buildID=${sha}' -X 'main.promoteOnStart=true'" -o worker.new ./cmd/worker)
}

build_cron() {
  local sha
  sha="$(git -C "$REPO_DIR" rev-parse --short=9 HEAD)"
  (cd "$REPO_DIR/backend" && "$GO_BIN" build -ldflags "-X 'main.buildID=${sha}'" -o cron.new ./cmd/cron)
}

install_and_restart() {
  local sha
  sha="$(git -C "$REPO_DIR" rev-parse HEAD)"

  if ! build_worker; then
    echo "build failed at $sha, leaving previous worker binary running" >&2
    return 1
  fi
  if ! build_cron; then
    echo "build failed at $sha, leaving previous cron binary in place" >&2
    return 1
  fi

  mv "$REPO_DIR/backend/worker.new" "$REPO_DIR/backend/worker"
  mv "$REPO_DIR/backend/cron.new" "$REPO_DIR/backend/cron"
  systemctl restart "$WORKER_SERVICE"
  echo "deployed $sha"
}
```

- [ ] **Step 4: Update test_deploy.sh's fixture to include a cmd/cron stub**

`test_deploy.sh` builds a minimal fixture repo containing only `backend/cmd/worker/main.go`. Since `deploy.sh` now also builds `./cmd/cron` on every cycle, the fixture needs that package too, or every scenario's `deploy.sh` invocation will fail at `build_cron`. In `deploy/worker-host/tests/test_deploy.sh`, add a `cmd/cron` stub next to the existing `cmd/worker` stub in the fixture setup (this block, near the top, currently ends with the `cmd/worker/main.go` heredoc):

```bash
mkdir -p "$REPO/backend/cmd/worker" "$REPO/backend/cmd/cron" "$REPO/deploy/worker-host"
cp "$SCRIPT_DIR/../deploy.sh" "$REPO/deploy/worker-host/deploy.sh"
chmod +x "$REPO/deploy/worker-host/deploy.sh"

cat > "$REPO/backend/go.mod" <<'EOF'
module backend

go 1.21
EOF
cat > "$REPO/backend/cmd/worker/main.go" <<'EOF'
package main

func main() {}
EOF
cat > "$REPO/backend/cmd/cron/main.go" <<'EOF'
package main

func main() {}
EOF
```

The `cmd/cron` stub stays constant for the whole test — only `cmd/worker/main.go` changes across scenarios (that's what each scenario is actually exercising). Add assertions that the cron binary gets built alongside the worker binary in scenario 1 (up to date, neither builds) and scenario 2 (new commit, both build):

```bash
# --- scenario 1: no new commits -> no rebuild, no restart ---
bash "$REPO/deploy/worker-host/deploy.sh"
[[ ! -f "$REPO/backend/worker" ]] || fail "should not have built a worker binary when up to date"
[[ ! -f "$REPO/backend/cron" ]] || fail "should not have built a cron binary when up to date"
[[ ! -s "$CALLS" ]] || fail "systemctl should not have been called when up to date"

# --- scenario 2: a new good commit -> rebuild + restart ---
CLONE="$WORK/clone"
git clone -q "$ORIGIN" "$CLONE"
cat > "$CLONE/backend/cmd/worker/main.go" <<'EOF'
package main

func main() { println("v2") }
EOF
git -C "$CLONE" -c user.email=test@example.com -c user.name=test commit -aqm "v2"
git -C "$CLONE" push -q origin main

bash "$REPO/deploy/worker-host/deploy.sh"
[[ -x "$REPO/backend/worker" ]] || fail "expected a worker binary to be built"
[[ -x "$REPO/backend/cron" ]] || fail "expected a cron binary to be built"
grep -q "restart ff-sims-worker.service" "$CALLS" || fail "expected systemctl restart to be called"
```

The remaining scenarios (3: build failure, 4: build_worker change takes effect same-cycle, 5: git fetch failure) don't need changes — they already assert on `backend/worker`'s state, which is unaffected by adding a parallel, always-succeeding `cmd/cron` stub build.

- [ ] **Step 5: Update the README**

In `deploy/worker-host/README.md`, near the existing logs section (around the `journalctl -u ff-sims-worker -f` line), add:

```markdown
- Discovery cron job logs (runs hourly, `Type=oneshot`): `journalctl -u ff-sims-discovery -f`
- Force an immediate discovery run without waiting for the timer: `sudo systemctl start ff-sims-discovery.service`
```

- [ ] **Step 6: Run the deploy pipeline's own test suite**

```bash
cd deploy/worker-host/tests && ./test_units.sh && ./test_setup.sh && ./test_deploy.sh
```

Expected: `PASS` printed by each script. `test_units.sh` needs no changes (it only validates `deploy/`-relative `ExecStart` targets, and both new unit files point at `{{REPO_DIR}}/backend/cron`, a built binary, not a `deploy/`-relative script — same exemption `ff-sims-worker.service` already has for `backend/worker`).

- [ ] **Step 7: Run the full Go test suite one more time**

```bash
cd backend && go build ./... && go vet ./... && go test ./... -count=1
```

Expected: clean build, `go vet` clean, all tests pass.

- [ ] **Step 8: Commit**

```bash
git add deploy/worker-host/ff-sims-discovery.service deploy/worker-host/ff-sims-discovery.timer deploy/worker-host/setup.sh deploy/worker-host/deploy.sh deploy/worker-host/tests/test_deploy.sh deploy/worker-host/README.md
git commit -m "Wire cmd/cron discovery job into the worker-host deploy pipeline

New ff-sims-discovery.service (oneshot) + .timer (hourly) alongside the
existing ff-sims-worker.service. deploy.sh now builds both cmd/worker and
cmd/cron each cycle; cmd/cron needs no service restart since it's a
one-shot process picked up fresh by its next timer firing."
```

---

## Post-plan manual step (not automatable — flag to the user, do not perform yourself)

Once this is deployed and `ff-sims-discovery.timer` is running successfully on rosebud for a while, applying the new env vars (`CRON_DISCOVERY_USER_POOL_SIZE`, `CRON_DISCOVERY_USER_REFILL_BATCH`, `CRON_DISCOVERY_LEAGUE_POOL_SIZE`, `CRON_DISCOVERY_LEAGUE_REFILL_BATCH`) to `/etc/ff-sims-worker.env` (shared with `cmd/worker` via `EnvironmentFile=`) is a live host change outside this plan's scope — the code defaults (4/2/4/2) are safe starting values and don't require any env file edit to work.
