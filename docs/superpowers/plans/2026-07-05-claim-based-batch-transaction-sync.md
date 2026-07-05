# Claim-Based Batch Transaction Sync Implementation Plan (#140)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace per-league child workflows with Postgres-claimed batch activities so transaction sync scales from ~43k to 600k+ league-syncs/day at ~1/300th the Temporal action cost.

**Architecture:** A claim activity atomically marks up to N stale leagues as claimed (`UPDATE … FOR UPDATE SKIP LOCKED … RETURNING`); a batch activity processes the claimed leagues with bounded goroutine concurrency, heartbeating, stamping each league done in the DB as it goes; the `TransactionSyncDispatcher` workflow loops claim→batch with K parallel batches until the backlog drains. A proactive client-side rate limiter keeps each worker fleet under its per-IP Sleeper budget.

**Tech Stack:** Go, Temporal Go SDK (workflows/activities/testsuite), GORM (Postgres prod, SQLite unit tests), goose migrations, `golang.org/x/time/rate`.

**Issue:** https://github.com/spkane31/ff-sims/issues/140 · **Spec:** `docs/superpowers/specs/2026-07-05-sleeper-sync-throughput-design.md`

## Global Constraints

- All Go work in `backend/`; verify with `cd backend && go build ./... && go test ./...`.
- `SLEEPER_RPM` default **2000** (start high, tune down later).
- Claim TTL: **20 minutes** (SQL literal `interval '20 minutes'`).
- Batch defaults: batch size **250**, parallel batches **4**, in-batch league concurrency **12** — all env-overridable via `TXN_SYNC_BATCH_SIZE`, `TXN_SYNC_PARALLEL_BATCHES`, `TXN_SYNC_LEAGUE_CONCURRENCY`.
- Claim-query tests require Postgres (`FOR UPDATE SKIP LOCKED`); they must skip (not fail) when `TEST_DATABASE_URL` is unset. Never port them to SQLite.
- Per-league failures must never fail a batch activity.
- Do NOT run psql against the prod DB; running dotenv-loading Go programs (e.g. `go run ./cmd/migrate`) is fine (user preference).
- Commit after every task; commit messages end with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.

## Existing code you'll touch (orientation)

- `backend/internal/workflows/transaction_sync.go` — dispatcher + per-league child workflow (child workflow gets deleted).
- `backend/internal/workflows/helpers.go` — task queues, `SyncBatchSize`, `defaultActivityOptions`.
- `backend/internal/activities/data_fetch.go` — `FetchLeagueTransactions` (leg loop, gets absorbed), `GetStaleLeaguesForTransactions` (replaced by claim), `MarkLeagueTransactionsFetched` (absorbed into per-league stamping).
- `backend/internal/activities/params.go` — param structs.
- `backend/internal/models/sleeper.go` — `SleeperLeague` model.
- `backend/internal/sleeper/client.go` — HTTP client (retry loop already merged via #138; you add the rate limiter).
- `backend/cmd/worker/main.go` — worker registration (versioning already merged via #139).
- Migrations: goose SQL files in `backend/migrations/`, latest is `017_draft_adp_ci.sql`; embedded via `backend/migrations/fs.go`, run by `backend/cmd/migrate`.
- Test helpers: `newTestDB(t)` (SQLite in-memory, `backend/internal/activities/discovery_test.go:49`); fake Sleeper API via `httptest` + `sleeper.NewWithBaseURL` (`data_fetch_test.go`); workflow tests via `testsuite.WorkflowTestSuite` (`backend/internal/workflows/workflows_test.go`); activity heartbeat precedent in `backend/internal/activities/player_sync.go:50`.

---

### Task 1: Migration + model field for `claimed_at`

**Files:**
- Create: `backend/migrations/018_league_claims.sql`
- Modify: `backend/internal/models/sleeper.go` (SleeperLeague struct, after `LastTransactionLegFetched`)

**Interfaces:**
- Produces: `sleeper_leagues.claimed_at timestamptz NULL` column; `models.SleeperLeague.ClaimedAt *time.Time` field; index `idx_sleeper_leagues_txn_stale`.

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
-- +goose NO TRANSACTION

ALTER TABLE sleeper_leagues ADD COLUMN IF NOT EXISTS claimed_at timestamptz;

-- Serves the claim query in ClaimLeaguesForTransactions: filter on the stale-
-- transactions predicate, order never-fetched first then oldest. NULLS FIRST
-- matches the query's ORDER BY exactly so the sort is an index walk.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_leagues_txn_stale
    ON sleeper_leagues (last_transactions_fetched_at ASC NULLS FIRST)
    WHERE skipped_at IS NULL AND last_fetched_at IS NOT NULL AND season >= '2025';

-- +goose Down
-- +goose NO TRANSACTION

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_leagues_txn_stale;
ALTER TABLE sleeper_leagues DROP COLUMN IF EXISTS claimed_at;
```

- [ ] **Step 2: Add the model field**

In `backend/internal/models/sleeper.go`, inside `SleeperLeague`, directly after the `LastTransactionLegFetched` line:

```go
	ClaimedAt                 *time.Time `gorm:"column:claimed_at"`
```

- [ ] **Step 3: Verify build and tests**

Run: `cd backend && go build ./... && go test ./internal/...`
Expected: PASS (SQLite `AutoMigrate` in tests picks up the new column automatically).

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/018_league_claims.sql backend/internal/models/sleeper.go
git commit -m "feat: add sleeper_leagues.claimed_at and stale-transactions index (#140)"
```

Note: do NOT apply the migration to the prod DB in this task; that happens at rollout (see final verification section).

---

### Task 2: Proactive rate limiter in the Sleeper client

**Files:**
- Modify: `backend/internal/sleeper/client.go`
- Test: `backend/internal/sleeper/client_test.go` (append)
- Modify: `backend/go.mod` (via `go get golang.org/x/time`)

**Interfaces:**
- Consumes: existing `Client.get` retry loop (merged in #138).
- Produces: every request through `(*Client).get` waits on a shared `*rate.Limiter` sized from `SLEEPER_RPM` (default 2000). `NewWithBaseURL` keeps its exact signature (many tests + worker main depend on it); a new `newLimiter()` helper reads the env.

- [ ] **Step 1: Write the failing tests** (append to `client_test.go`)

```go
func TestGet_RateLimiterSpacesRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewWithBaseURL(srv.URL)
	// 120 RPM = one token every 500ms; burst 1 so the second call must wait.
	c.limiter = rate.NewLimiter(rate.Limit(120.0/60.0), 1)

	start := time.Now()
	var out map[string]any
	for i := 0; i < 2; i++ {
		if err := c.get(context.Background(), "/v1/state/nfl", &out); err != nil {
			t.Fatalf("get %d: %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed < 400*time.Millisecond {
		t.Errorf("expected second request to be rate-limited (>=400ms), took %v", elapsed)
	}
}

func TestNewLimiter_DefaultsAndEnvOverride(t *testing.T) {
	t.Setenv("SLEEPER_RPM", "")
	if l := newLimiter(); l.Limit() != rate.Limit(2000.0/60.0) {
		t.Errorf("default limiter = %v, want %v", l.Limit(), rate.Limit(2000.0/60.0))
	}
	t.Setenv("SLEEPER_RPM", "600")
	if l := newLimiter(); l.Limit() != rate.Limit(600.0/60.0) {
		t.Errorf("env limiter = %v, want %v", l.Limit(), rate.Limit(600.0/60.0))
	}
}
```

(This test file is `package sleeper` — internal fields are accessible. Add `"golang.org/x/time/rate"` to the test imports. If the existing file is `package sleeper_test`, put these in a new `client_internal_test.go` with `package sleeper` instead — check the first line before writing.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go get golang.org/x/time && go test ./internal/sleeper/ -run 'RateLimiter|NewLimiter' -v`
Expected: FAIL — `c.limiter undefined` / `newLimiter undefined` (compile error).

- [ ] **Step 3: Implement**

In `client.go`, add to imports: `"os"`, `"strconv"` (already there), `"golang.org/x/time/rate"`.

```go
// defaultRPM is the SLEEPER_RPM fallback: requests/minute budget per process.
// Each fleet (DigitalOcean, Raspberry Pi) has its own IP, so per-process is
// per-IP. Start high, tune down.
const defaultRPM = 2000
```

Extend the struct and constructors:

```go
type Client struct {
	http    *http.Client
	baseURL string
	limiter *rate.Limiter
}

func NewWithBaseURL(baseURL string) *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		baseURL: baseURL,
		limiter: newLimiter(),
	}
}

// newLimiter builds the client-wide request limiter from SLEEPER_RPM
// (requests/minute, default 2000). Burst of one second's worth of tokens keeps
// short spikes smooth without letting the minute budget be spent all at once.
func newLimiter() *rate.Limiter {
	rpm := defaultRPM
	if v := os.Getenv("SLEEPER_RPM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			rpm = n
		}
	}
	perSecond := float64(rpm) / 60.0
	burst := int(perSecond)
	if burst < 1 {
		burst = 1
	}
	return rate.NewLimiter(rate.Limit(perSecond), burst)
}
```

At the top of the retry loop in `get` (immediately after the `if ctx.Err() != nil` check, so every attempt including retries consumes a token):

```go
		if err := c.limiter.Wait(ctx); err != nil {
			return err
		}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/sleeper/ -v`
Expected: PASS, including all pre-existing retry tests (they use `NewWithBaseURL`, which now has a 2000 RPM limiter — 33 tokens of burst is far more than any test makes, so no slowdown).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/sleeper/ backend/go.mod backend/go.sum
git commit -m "feat: proactive SLEEPER_RPM rate limiter in sleeper client (#140)"
```

---

### Task 3: Claim activity (`ClaimLeaguesForTransactions`)

**Files:**
- Modify: `backend/internal/activities/params.go`
- Modify: `backend/internal/activities/data_fetch.go` (add activity; leave `GetStaleLeaguesForTransactions` in place — deleted in Task 5)
- Create: `backend/internal/activities/claim_pg_test.go`

**Interfaces:**
- Produces:
  - `ClaimLeaguesForTransactionsParams{BatchSize int}`
  - `LeagueTransactionState{LeagueID string; Season string; LastLegFetched *int}` — **adds `Season`** to the existing struct (needed for leg capping in Task 4; the struct's only current consumer is the code this plan replaces).
  - `func (a *DataFetchActivities) ClaimLeaguesForTransactions(ctx context.Context, params ClaimLeaguesForTransactionsParams) ([]LeagueTransactionState, error)`

- [ ] **Step 1: Update params.go**

Add, and modify `LeagueTransactionState`:

```go
type ClaimLeaguesForTransactionsParams struct {
	BatchSize int
}

// LeagueTransactionState carries the league ID, season, and leg cursor for one
// claimed league, as returned by ClaimLeaguesForTransactions.
type LeagueTransactionState struct {
	LeagueID       string
	Season         string
	LastLegFetched *int
}
```

`GetStaleLeaguesForTransactions` (still present until Task 5) must compile: add `Season: l.Season,` to the struct literal it builds.

- [ ] **Step 2: Write the failing Postgres-gated tests**

Create `backend/internal/activities/claim_pg_test.go`. These tests need real Postgres for `FOR UPDATE SKIP LOCKED` and `interval` arithmetic; they skip without `TEST_DATABASE_URL`. Each test runs in a throwaway schema so parallel/local runs can't collide.

```go
package activities_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/activities"
	"backend/internal/models"
)

// newPGTestDB opens TEST_DATABASE_URL inside a fresh throwaway schema and
// migrates SleeperLeague into it. Skips the test when the env var is unset —
// claim queries use FOR UPDATE SKIP LOCKED, which SQLite cannot express.
func newPGTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; claim tests need Postgres (FOR UPDATE SKIP LOCKED)")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	schema := fmt.Sprintf("claim_test_%d", rand.Int63())
	if err := db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DROP SCHEMA " + schema + " CASCADE")
		sqlDB, _ := db.DB()
		sqlDB.Close()
	})
	if err := db.Exec("SET search_path TO " + schema).Error; err != nil {
		t.Fatalf("set search_path: %v", err)
	}
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

func TestClaimLeagues_OrderingLimitAndStamp(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour)
	recent := now.Add(-1 * time.Hour)
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "never", LastFetchedAt: &now})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "oldest", LastFetchedAt: &now, LastTransactionsFetchedAt: &old})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "recent", LastFetchedAt: &now, LastTransactionsFetchedAt: &recent})

	a := &activities.DataFetchActivities{DB: db}
	got, err := a.ClaimLeaguesForTransactions(context.Background(), activities.ClaimLeaguesForTransactionsParams{BatchSize: 2})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(got) != 2 || got[0].LeagueID != "never" || got[1].LeagueID != "oldest" {
		t.Fatalf("expected [never oldest], got %+v", got)
	}
	if got[0].Season != "2026" {
		t.Errorf("expected Season populated, got %+v", got[0])
	}
	var claimed int64
	db.Model(&models.SleeperLeague{}).Where("claimed_at IS NOT NULL").Count(&claimed)
	if claimed != 2 {
		t.Errorf("expected 2 rows stamped claimed_at, got %d", claimed)
	}
}

func TestClaimLeagues_ExcludesIneligible(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "skipped", LastFetchedAt: &now, SkippedAt: &now})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "unfetched"}) // last_fetched_at NULL
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "old-season", Season: "2024", LastFetchedAt: &now})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "done-complete", Status: "complete", LastFetchedAt: &now, LastTransactionsFetchedAt: &now})
	// complete but never transaction-synced: still eligible
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "complete-unsynced", Status: "complete", LastFetchedAt: &now})

	a := &activities.DataFetchActivities{DB: db}
	got, err := a.ClaimLeaguesForTransactions(context.Background(), activities.ClaimLeaguesForTransactionsParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(got) != 1 || got[0].LeagueID != "complete-unsynced" {
		t.Fatalf("expected only complete-unsynced, got %+v", got)
	}
}

func TestClaimLeagues_RespectsAndExpiresClaims(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	fresh := now.Add(-1 * time.Minute)
	stale := now.Add(-30 * time.Minute)
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "fresh-claim", LastFetchedAt: &now, ClaimedAt: &fresh})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "expired-claim", LastFetchedAt: &now, ClaimedAt: &stale})

	a := &activities.DataFetchActivities{DB: db}
	got, err := a.ClaimLeaguesForTransactions(context.Background(), activities.ClaimLeaguesForTransactionsParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(got) != 1 || got[0].LeagueID != "expired-claim" {
		t.Fatalf("expected only expired-claim to be re-claimable, got %+v", got)
	}
}

func TestClaimLeagues_ConcurrentClaimsAreDisjoint(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	for i := 0; i < 20; i++ {
		seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: fmt.Sprintf("lg%02d", i), LastFetchedAt: &now})
	}

	a := &activities.DataFetchActivities{DB: db}
	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := a.ClaimLeaguesForTransactions(context.Background(), activities.ClaimLeaguesForTransactionsParams{BatchSize: 10})
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, s := range got {
				seen[s.LeagueID]++
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

- [ ] **Step 3: Run tests to verify they fail (and skip without Postgres)**

Run: `cd backend && go test ./internal/activities/ -run ClaimLeagues -v`
Expected without `TEST_DATABASE_URL`: compile error first (`ClaimLeaguesForTransactions` undefined) — that's the failing state.
To exercise for real, start a disposable Postgres: `docker run --rm -d --name claimtest -e POSTGRES_PASSWORD=t -p 5499:5432 postgres:16` then `TEST_DATABASE_URL="postgres://postgres:t@localhost:5499/postgres?sslmode=disable" go test ./internal/activities/ -run ClaimLeagues -v`. After implementation these must PASS; without the env they must SKIP.

- [ ] **Step 4: Implement the claim activity** (in `data_fetch.go`)

```go
// claimLeaguesForTransactionsSQL atomically claims up to @batch_size stale
// leagues for transaction syncing. FOR UPDATE SKIP LOCKED lets concurrent
// claimers (two fleets, K parallel pipelines) partition the backlog without
// blocking or double-claiming; the 20-minute expiry window re-queues leagues
// claimed by a worker that died mid-batch. Ordering matches the partial index
// idx_sleeper_leagues_txn_stale (never-fetched first, then oldest).
const claimLeaguesForTransactionsSQL = `
UPDATE sleeper_leagues SET claimed_at = now()
WHERE sleeper_league_id IN (
    SELECT sleeper_league_id FROM sleeper_leagues
    WHERE skipped_at IS NULL AND last_fetched_at IS NOT NULL AND season >= '2025'
      AND NOT (status = 'complete' AND last_transactions_fetched_at IS NOT NULL)
      AND (claimed_at IS NULL OR claimed_at < now() - interval '20 minutes')
    ORDER BY last_transactions_fetched_at ASC NULLS FIRST
    LIMIT ?
    FOR UPDATE SKIP LOCKED
)
RETURNING sleeper_league_id, season, last_transaction_leg_fetched`

// ClaimLeaguesForTransactions claims up to BatchSize leagues with stale
// transaction data and returns their sync state. Postgres-only (SKIP LOCKED).
func (a *DataFetchActivities) ClaimLeaguesForTransactions(ctx context.Context, params ClaimLeaguesForTransactionsParams) ([]LeagueTransactionState, error) {
	var rows []struct {
		SleeperLeagueID           string
		Season                    string
		LastTransactionLegFetched *int
	}
	if err := a.DB.WithContext(ctx).Raw(claimLeaguesForTransactionsSQL, params.BatchSize).Scan(&rows).Error; err != nil {
		return nil, err
	}
	states := make([]LeagueTransactionState, len(rows))
	for i, r := range rows {
		states[i] = LeagueTransactionState{
			LeagueID:       r.SleeperLeagueID,
			Season:         r.Season,
			LastLegFetched: r.LastTransactionLegFetched,
		}
	}
	return states, nil
}
```

Note: `RETURNING` from an `UPDATE` does not guarantee order; the outer `UPDATE` may return rows in any order. The ordering test (`never` before `oldest`) depends on the subquery's LIMIT choosing *which* rows, and Postgres in practice returns them in update order — if the ordering assertion proves flaky, relax the test to set-membership (`never` and `oldest` both present, `recent` absent). Which leagues get claimed matters; the order within a batch does not.

- [ ] **Step 5: Run tests to verify pass/skip both ways**

Run (no env): `cd backend && go test ./internal/activities/ -v -run ClaimLeagues` → SKIP messages.
Run (with the docker Postgres from Step 3): PASS all four.
Also run the full suite: `go test ./internal/...` → PASS (the `Season:` addition in Step 1 keeps `GetStaleLeaguesForTransactions` compiling).

- [ ] **Step 6: Commit**

```bash
git add backend/internal/activities/
git commit -m "feat: atomic league claiming for transaction sync (SKIP LOCKED) (#140)"
```

---

### Task 4: Batch sync activity (`SyncLeagueTransactionsBatch`)

**Files:**
- Modify: `backend/internal/activities/params.go`
- Modify: `backend/internal/activities/data_fetch.go`
- Test: `backend/internal/activities/data_fetch_test.go` (append)

**Interfaces:**
- Consumes: `LeagueTransactionState` (Task 3), `sleeper.Client.GetNFLState/GetTransactions`, `models.SleeperLeague.ClaimedAt` (Task 1).
- Produces:
  - `SyncLeagueTransactionsBatchParams{Leagues []LeagueTransactionState; Concurrency int}`
  - `SyncBatchResult{Processed int; Failed int}`
  - `func (a *DataFetchActivities) SyncLeagueTransactionsBatch(ctx context.Context, params SyncLeagueTransactionsBatchParams) (SyncBatchResult, error)`
  - unexported helpers `maxLegForLeague(season string, state *sleeper.NFLState) int` and `(a *DataFetchActivities) syncOneLeague(ctx, lg LeagueTransactionState, maxLeg int) error`

- [ ] **Step 1: Add params/result structs** (params.go)

```go
type SyncLeagueTransactionsBatchParams struct {
	Leagues     []LeagueTransactionState
	Concurrency int
}

// SyncBatchResult summarizes one batch activity execution. Failed leagues keep
// their claim and re-enter the queue when it expires.
type SyncBatchResult struct {
	Processed int
	Failed    int
}
```

- [ ] **Step 2: Write the failing tests** (append to `data_fetch_test.go`; add imports `"sync/atomic"`, `"go.temporal.io/sdk/testsuite"`, `"gorm.io/gorm"`)

The fake Sleeper server needs `/v1/state/nfl`. Batch tests run the activity through `testsuite` so `activity.RecordHeartbeat` has a valid context (same reason `player_sync` tests do).

```go
// batchTestServer fakes /v1/state/nfl plus per-league transaction legs.
// legs maps "leagueID/leg" -> transactions; missing keys 404 (empty leg).
func batchTestServer(t *testing.T, week int, legs map[string][]sleeper.Transaction, calls *atomic.Int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			calls.Add(1)
		}
		if strings.HasSuffix(r.URL.Path, "/state/nfl") {
			json.NewEncoder(w).Encode(sleeper.NFLState{Season: "2026", SeasonType: "regular", Week: week})
			return
		}
		// path: /v1/league/{id}/transactions/{leg}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		key := parts[2] + "/" + parts[4]
		txns, ok := legs[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(txns)
	}))
}

func runBatch(t *testing.T, dfa *activities.DataFetchActivities, params activities.SyncLeagueTransactionsBatchParams) activities.SyncBatchResult {
	t.Helper()
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestActivityEnvironment()
	env.RegisterActivity(dfa.SyncLeagueTransactionsBatch)
	val, err := env.ExecuteActivity(dfa.SyncLeagueTransactionsBatch, params)
	if err != nil {
		t.Fatalf("batch activity: %v", err)
	}
	var res activities.SyncBatchResult
	if err := val.Get(&res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return res
}

func claimedLeague(t *testing.T, db *gorm.DB, id string) models.SleeperLeague {
	t.Helper()
	now := time.Now().UTC()
	l := models.SleeperLeague{SleeperLeagueID: id, Season: "2026", LastFetchedAt: &now, ClaimedAt: &now}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return l
}

func TestSyncBatch_StampsAndClearsClaims(t *testing.T) {
	db := newTestDB(t)
	claimedLeague(t, db, "lg1")
	claimedLeague(t, db, "lg2")

	srv := batchTestServer(t, 3, map[string][]sleeper.Transaction{
		"lg1/2": {{TransactionID: "tx1", Type: "waiver", Status: "complete", Leg: 2}},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runBatch(t, dfa, activities.SyncLeagueTransactionsBatchParams{
		Leagues: []activities.LeagueTransactionState{
			{LeagueID: "lg1", Season: "2026"},
			{LeagueID: "lg2", Season: "2026"},
		},
		Concurrency: 2,
	})
	if res.Processed != 2 || res.Failed != 0 {
		t.Fatalf("expected 2 processed / 0 failed, got %+v", res)
	}

	var lg1 models.SleeperLeague
	db.First(&lg1, "sleeper_league_id = ?", "lg1")
	if lg1.LastTransactionsFetchedAt == nil || lg1.ClaimedAt != nil {
		t.Errorf("lg1 not stamped/unclaimed: %+v", lg1)
	}
	if lg1.LastTransactionLegFetched == nil || *lg1.LastTransactionLegFetched != 2 {
		t.Errorf("lg1 leg cursor = %v, want 2", lg1.LastTransactionLegFetched)
	}
	var txCount int64
	db.Model(&models.SleeperTransaction{}).Count(&txCount)
	if txCount != 1 {
		t.Errorf("expected 1 transaction row, got %d", txCount)
	}
}

func TestSyncBatch_LegLoopCappedAtCurrentWeek(t *testing.T) {
	db := newTestDB(t)
	claimedLeague(t, db, "lg1")

	var calls atomic.Int64
	srv := batchTestServer(t, 3, nil, &calls) // all legs 404
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	runBatch(t, dfa, activities.SyncLeagueTransactionsBatchParams{
		Leagues:     []activities.LeagueTransactionState{{LeagueID: "lg1", Season: "2026"}},
		Concurrency: 1,
	})
	// 1 state call + legs 1..3 = 4 total; the old code would have made 19.
	if got := calls.Load(); got != 4 {
		t.Errorf("expected 4 HTTP calls (state + 3 legs), got %d", got)
	}
}

func TestSyncBatch_PastSeasonFetchesAllLegs(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().UTC()
	l := models.SleeperLeague{SleeperLeagueID: "lg1", Season: "2025", LastFetchedAt: &now, ClaimedAt: &now}
	db.Create(&l)

	var calls atomic.Int64
	srv := batchTestServer(t, 3, nil, &calls)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	runBatch(t, dfa, activities.SyncLeagueTransactionsBatchParams{
		Leagues:     []activities.LeagueTransactionState{{LeagueID: "lg1", Season: "2025"}},
		Concurrency: 1,
	})
	// 2025 < state season 2026: full historical sweep, legs 1..18 + state = 19.
	if got := calls.Load(); got != 19 {
		t.Errorf("expected 19 HTTP calls, got %d", got)
	}
}

func TestSyncBatch_PerLeagueFailureDoesNotFailBatch(t *testing.T) {
	db := newTestDB(t)
	claimedLeague(t, db, "bad")
	claimedLeague(t, db, "good")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/state/nfl") {
			json.NewEncoder(w).Encode(sleeper.NFLState{Season: "2026", Week: 1})
			return
		}
		if strings.Contains(r.URL.Path, "/league/bad/") {
			w.WriteHeader(http.StatusBadRequest) // non-retryable, non-404
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runBatch(t, dfa, activities.SyncLeagueTransactionsBatchParams{
		Leagues: []activities.LeagueTransactionState{
			{LeagueID: "bad", Season: "2026"},
			{LeagueID: "good", Season: "2026"},
		},
		Concurrency: 2,
	})
	if res.Processed != 1 || res.Failed != 1 {
		t.Fatalf("expected 1/1, got %+v", res)
	}
	var bad, good models.SleeperLeague
	db.First(&bad, "sleeper_league_id = ?", "bad")
	db.First(&good, "sleeper_league_id = ?", "good")
	if bad.ClaimedAt == nil || bad.LastTransactionsFetchedAt != nil {
		t.Errorf("failed league must stay claimed and unstamped: %+v", bad)
	}
	if good.ClaimedAt != nil || good.LastTransactionsFetchedAt == nil {
		t.Errorf("good league must be stamped and unclaimed: %+v", good)
	}
}

func TestSyncBatch_RetrySkipsAlreadyStampedLeagues(t *testing.T) {
	db := newTestDB(t)
	// lg1 was stamped by a previous attempt (claim cleared); lg2 still claimed.
	now := time.Now().UTC()
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1", Season: "2026", LastFetchedAt: &now, LastTransactionsFetchedAt: &now})
	claimedLeague(t, db, "lg2")

	var calls atomic.Int64
	srv := batchTestServer(t, 1, nil, &calls)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runBatch(t, dfa, activities.SyncLeagueTransactionsBatchParams{
		Leagues: []activities.LeagueTransactionState{
			{LeagueID: "lg1", Season: "2026"},
			{LeagueID: "lg2", Season: "2026"},
		},
		Concurrency: 1,
	})
	if res.Processed != 1 {
		t.Fatalf("expected only still-claimed lg2 processed, got %+v", res)
	}
	// state + leg 1 for lg2 only
	if got := calls.Load(); got != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", got)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd backend && go test ./internal/activities/ -run SyncBatch -v`
Expected: compile error (`SyncLeagueTransactionsBatch` undefined).

- [ ] **Step 4: Implement** (data_fetch.go; add imports `"sync"`, `"go.temporal.io/sdk/activity"`)

```go
// maxLegForLeague returns the highest transaction leg worth fetching. Past
// seasons get the full 1..18 sweep; the current season is capped at the
// current NFL week (offseason week 0 still fetches leg 1, where offseason
// moves land). A nil state (state endpoint down) falls back to 18 rather than
// stalling the batch.
func maxLegForLeague(season string, state *sleeper.NFLState) int {
	if state == nil || season < state.Season {
		return 18
	}
	if state.Week < 1 {
		return 1
	}
	return min(state.Week, 18)
}

// SyncLeagueTransactionsBatch syncs transactions for a claimed batch of
// leagues with bounded concurrency, stamping each league done as it completes.
// Per-league failures are counted, not propagated: a failed league keeps its
// claim and re-enters the queue when the claim expires. The activity heartbeats
// as leagues complete so a dead worker is detected via HeartbeatTimeout.
func (a *DataFetchActivities) SyncLeagueTransactionsBatch(ctx context.Context, params SyncLeagueTransactionsBatchParams) (SyncBatchResult, error) {
	logger := activity.GetLogger(ctx)
	res := SyncBatchResult{}

	// Re-scope to leagues still claimed: on an activity retry, leagues stamped
	// by the previous attempt have claimed_at cleared and must not re-sync.
	ids := make([]string, len(params.Leagues))
	byID := make(map[string]LeagueTransactionState, len(params.Leagues))
	for i, lg := range params.Leagues {
		ids[i] = lg.LeagueID
		byID[lg.LeagueID] = lg
	}
	var stillClaimed []string
	if err := a.DB.WithContext(ctx).Model(&models.SleeperLeague{}).
		Where("sleeper_league_id IN ? AND claimed_at IS NOT NULL", ids).
		Pluck("sleeper_league_id", &stillClaimed).Error; err != nil {
		return res, err
	}
	if len(stillClaimed) == 0 {
		return res, nil
	}

	state, err := a.Sleeper.GetNFLState(ctx)
	if err != nil {
		logger.Warn("GetNFLState failed; falling back to full 18-leg sweep", "error", err)
		state = nil
	}

	concurrency := params.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	type leagueResult struct {
		leagueID string
		err      error
	}
	sem := make(chan struct{}, concurrency)
	results := make(chan leagueResult, len(stillClaimed))
	var wg sync.WaitGroup
	for _, id := range stillClaimed {
		lg := byID[id]
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results <- leagueResult{leagueID: lg.LeagueID, err: a.syncOneLeague(ctx, lg, maxLegForLeague(lg.Season, state))}
		}()
	}
	go func() { wg.Wait(); close(results) }()

	done := 0
	for r := range results {
		done++
		if r.err != nil {
			res.Failed++
			logger.Warn("league transaction sync failed", "leagueID", r.leagueID, "error", r.err)
		} else {
			res.Processed++
		}
		if done%10 == 0 {
			activity.RecordHeartbeat(ctx, done)
		}
	}
	return res, nil
}

// syncOneLeague fetches transactions for one league from its leg cursor up to
// maxLeg, upserts them, and stamps completion (clearing the claim) in a single
// update. Per-leg 404s mean "no transactions for that leg" and are skipped.
func (a *DataFetchActivities) syncOneLeague(ctx context.Context, lg LeagueTransactionState, maxLeg int) error {
	startLeg := 1
	if lg.LastLegFetched != nil && *lg.LastLegFetched > 1 {
		startLeg = *lg.LastLegFetched - 1
	}

	maxSeen := 0
	for leg := startLeg; leg <= maxLeg; leg++ {
		txns, err := a.Sleeper.GetTransactions(ctx, lg.LeagueID, leg)
		if err != nil {
			var nfe *sleeper.NotFoundError
			if errors.As(err, &nfe) {
				continue
			}
			return fmt.Errorf("leg %d: %w", leg, err)
		}
		if len(txns) == 0 {
			continue
		}
		rows := make([]models.SleeperTransaction, len(txns))
		for i, t := range txns {
			addsJSON, _ := json.Marshal(t.Adds)
			dropsJSON, _ := json.Marshal(t.Drops)
			picksJSON, _ := json.Marshal(t.DraftPicks)
			waiverJSON, _ := json.Marshal(t.WaiverBudget)
			rows[i] = models.SleeperTransaction{
				SleeperTransactionID: t.TransactionID,
				SleeperLeagueID:      lg.LeagueID,
				Type:                 t.Type,
				Status:               t.Status,
				CreatedAtSleeper:     t.Created,
				Leg:                  t.Leg,
				Adds:                 addsJSON,
				Drops:                dropsJSON,
				DraftPicks:           picksJSON,
				WaiverBudget:         waiverJSON,
			}
		}
		if err := a.DB.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(rows, 500).Error; err != nil {
			return fmt.Errorf("leg %d upsert: %w", leg, err)
		}
		if leg > maxSeen {
			maxSeen = leg
		}
	}

	updates := map[string]interface{}{
		"last_transactions_fetched_at": time.Now().UTC(),
		"claimed_at":                   nil,
	}
	if maxSeen > 0 {
		updates["last_transaction_leg_fetched"] = maxSeen
	}
	return a.DB.WithContext(ctx).
		Model(&models.SleeperLeague{}).
		Where("sleeper_league_id = ?", lg.LeagueID).
		Updates(updates).Error
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd backend && go test ./internal/activities/ -v`
Expected: PASS (new SyncBatch tests plus all existing ones).

- [ ] **Step 6: Commit**

```bash
git add backend/internal/activities/
git commit -m "feat: batched league transaction sync with per-league stamping (#140)"
```

---

### Task 5: Dispatcher rewrite + config activity + delete the per-league path

**Files:**
- Modify: `backend/internal/workflows/transaction_sync.go` (full rewrite)
- Modify: `backend/internal/workflows/helpers.go` (batch activity options + iteration bound)
- Modify: `backend/internal/activities/data_fetch.go` (add `GetTransactionSyncConfig`; delete `GetStaleLeaguesForTransactions`, `FetchLeagueTransactions`, `MarkLeagueTransactionsFetched`)
- Modify: `backend/internal/activities/params.go` (add `TransactionSyncConfig`; delete `FetchLeagueTransactionsParams`, `MarkLeagueTransactionsFetchedParams`)
- Modify: `backend/cmd/worker/main.go:96-97` (remove `LeagueTransactionSyncWorkflow` registration)
- Modify: `backend/internal/workflows/workflows_test.go` (replace TransactionSync tests)
- Modify: `backend/internal/activities/data_fetch_test.go` (delete tests of removed activities: `TestFetchLeagueTransactions_*`, `TestGetStaleLeaguesForTransactions_*`, `TestMarkLeagueTransactionsFetched_*` — whichever exist)

**Interfaces:**
- Consumes: `ClaimLeaguesForTransactions` (Task 3), `SyncLeagueTransactionsBatch` (Task 4).
- Produces:
  - `TransactionSyncConfig{ParallelBatches int; BatchSize int; Concurrency int}` + `func (a *DataFetchActivities) GetTransactionSyncConfig(ctx context.Context) (TransactionSyncConfig, error)`
  - Rewritten `func TransactionSyncDispatcher(ctx workflow.Context) error`
  - `workflows.TxnMaxDispatchIterations = 25`, `batchActivityOptions` in helpers.go
  - **Deletes:** `LeagueTransactionSyncWorkflow`, `LeagueSyncParams` stays (drafts still use it — check: `draft_sync.go` uses `LeagueSyncParams`; keep it).

- [ ] **Step 1: Add the config activity** (params.go + data_fetch.go)

params.go:

```go
// TransactionSyncConfig is read from env by GetTransactionSyncConfig so the
// dispatcher workflow (which cannot read env deterministically) can be tuned
// without a redeploy of workflow code.
type TransactionSyncConfig struct {
	ParallelBatches int // TXN_SYNC_PARALLEL_BATCHES, default 4
	BatchSize       int // TXN_SYNC_BATCH_SIZE, default 250
	Concurrency     int // TXN_SYNC_LEAGUE_CONCURRENCY, default 12
}
```

data_fetch.go (add import `"os"`, `"strconv"`):

```go
// GetTransactionSyncConfig returns the dispatcher tuning knobs from env.
func (a *DataFetchActivities) GetTransactionSyncConfig(ctx context.Context) (TransactionSyncConfig, error) {
	return TransactionSyncConfig{
		ParallelBatches: envInt("TXN_SYNC_PARALLEL_BATCHES", 4),
		BatchSize:       envInt("TXN_SYNC_BATCH_SIZE", 250),
		Concurrency:     envInt("TXN_SYNC_LEAGUE_CONCURRENCY", 12),
	}, nil
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}
```

- [ ] **Step 2: Add batch activity options and iteration bound** (helpers.go)

```go
const (
	// TxnMaxDispatchIterations bounds the dispatcher's claim loop so one run's
	// event history stays small; the 5-minute schedule picks up any remainder.
	// 25 iterations × 4 batches × 250 leagues = 25k leagues per run.
	TxnMaxDispatchIterations = 25
)

// batchActivityOptions suit long-running batch activities that heartbeat:
// generous StartToClose for a 250-league batch under rate limiting, tight
// HeartbeatTimeout so a dead worker is detected in minutes and the retry
// re-processes only unstamped leagues.
var batchActivityOptions = workflow.ActivityOptions{
	StartToCloseTimeout: 30 * time.Minute,
	HeartbeatTimeout:    2 * time.Minute,
	RetryPolicy: &temporal.RetryPolicy{
		InitialInterval:    5 * time.Second,
		BackoffCoefficient: 2.0,
		MaximumAttempts:    3,
	},
}
```

- [ ] **Step 3: Rewrite the failing workflow tests** (workflows_test.go — replace all existing `TransactionSync*` tests)

```go
func TestTransactionSyncDispatcher_DrainsUntilShortClaim(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	cfg := activities.TransactionSyncConfig{ParallelBatches: 2, BatchSize: 2, Concurrency: 4}
	env.OnActivity(dfa.GetTransactionSyncConfig, mock.Anything).Return(cfg, nil)

	full := []activities.LeagueTransactionState{{LeagueID: "a", Season: "2026"}, {LeagueID: "b", Season: "2026"}}
	short := []activities.LeagueTransactionState{{LeagueID: "c", Season: "2026"}}
	// First claim full, second claim short -> dispatcher must stop claiming after the short one.
	env.OnActivity(dfa.ClaimLeaguesForTransactions, mock.Anything, activities.ClaimLeaguesForTransactionsParams{BatchSize: 2}).
		Return(full, nil).Once()
	env.OnActivity(dfa.ClaimLeaguesForTransactions, mock.Anything, activities.ClaimLeaguesForTransactionsParams{BatchSize: 2}).
		Return(short, nil).Once()

	env.OnActivity(dfa.SyncLeagueTransactionsBatch, mock.Anything, activities.SyncLeagueTransactionsBatchParams{Leagues: full, Concurrency: 4}).
		Return(activities.SyncBatchResult{Processed: 2}, nil).Once()
	env.OnActivity(dfa.SyncLeagueTransactionsBatch, mock.Anything, activities.SyncLeagueTransactionsBatchParams{Leagues: short, Concurrency: 4}).
		Return(activities.SyncBatchResult{Processed: 1}, nil).Once()

	env.ExecuteWorkflow(workflows.TransactionSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestTransactionSyncDispatcher_EmptyClaimStopsImmediately(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.GetTransactionSyncConfig, mock.Anything).
		Return(activities.TransactionSyncConfig{ParallelBatches: 4, BatchSize: 250, Concurrency: 12}, nil)
	env.OnActivity(dfa.ClaimLeaguesForTransactions, mock.Anything, activities.ClaimLeaguesForTransactionsParams{BatchSize: 250}).
		Return([]activities.LeagueTransactionState{}, nil).Once()

	env.ExecuteWorkflow(workflows.TransactionSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestTransactionSyncDispatcher_BatchFailureDoesNotFailRun(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.GetTransactionSyncConfig, mock.Anything).
		Return(activities.TransactionSyncConfig{ParallelBatches: 1, BatchSize: 2, Concurrency: 4}, nil)
	short := []activities.LeagueTransactionState{{LeagueID: "a", Season: "2026"}}
	env.OnActivity(dfa.ClaimLeaguesForTransactions, mock.Anything, mock.Anything).Return(short, nil).Once()
	// Non-retryable so the mock's .Once() isn't consumed by activity retries
	// (batchActivityOptions allows 3 attempts).
	env.OnActivity(dfa.SyncLeagueTransactionsBatch, mock.Anything, mock.Anything).
		Return(activities.SyncBatchResult{}, temporal.NewNonRetryableApplicationError("boom", "test", nil)).Once()

	env.ExecuteWorkflow(workflows.TransactionSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	// Failed batches are logged; the leagues' claims expire and re-queue.
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `cd backend && go test ./internal/workflows/ -run TransactionSync -v`
Expected: compile error (`GetTransactionSyncConfig` mock target exists but dispatcher signature/flow mismatch) or assertion failures against the old dispatcher.

- [ ] **Step 5: Rewrite `transaction_sync.go`**

Replace the entire file body (both functions) with:

```go
package workflows

import (
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// TransactionSyncDispatcher drains the stale-transactions backlog by fanning
// out claim→batch pipelines. Each iteration claims up to ParallelBatches
// batches of leagues (atomically, via FOR UPDATE SKIP LOCKED in Postgres) and
// runs a SyncLeagueTransactionsBatch activity per claim in parallel. A short
// or empty claim means the backlog is drained for now, so the run exits and
// the 5-minute schedule takes over. Failed batch activities are logged, not
// propagated: their leagues' claims expire after 20 minutes and re-queue.
func TransactionSyncDispatcher(ctx workflow.Context) error {
	dfa := &activities.DataFetchActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	batchCtx := workflow.WithActivityOptions(ctx, batchActivityOptions)
	logger := workflow.GetLogger(ctx)

	var cfg activities.TransactionSyncConfig
	if err := workflow.ExecuteActivity(actCtx, dfa.GetTransactionSyncConfig).Get(ctx, &cfg); err != nil {
		return err
	}

	for iter := 0; iter < TxnMaxDispatchIterations; iter++ {
		var futures []workflow.Future
		drained := false
		for k := 0; k < cfg.ParallelBatches; k++ {
			var leagues []activities.LeagueTransactionState
			err := workflow.ExecuteActivity(actCtx, dfa.ClaimLeaguesForTransactions, activities.ClaimLeaguesForTransactionsParams{
				BatchSize: cfg.BatchSize,
			}).Get(ctx, &leagues)
			if err != nil {
				logger.Error("claim failed; stopping dispatch for this run", "error", err)
				drained = true
				break
			}
			if len(leagues) == 0 {
				drained = true
				break
			}
			futures = append(futures, workflow.ExecuteActivity(batchCtx, dfa.SyncLeagueTransactionsBatch, activities.SyncLeagueTransactionsBatchParams{
				Leagues:     leagues,
				Concurrency: cfg.Concurrency,
			}))
			if len(leagues) < cfg.BatchSize {
				drained = true
				break
			}
		}
		for _, f := range futures {
			var res activities.SyncBatchResult
			if err := f.Get(ctx, &res); err != nil {
				logger.Error("transaction batch failed; claims will expire and re-queue", "error", err)
				continue
			}
			logger.Info("transaction batch done", "processed", res.Processed, "failed", res.Failed)
		}
		if drained {
			break
		}
	}
	return nil
}
```

- [ ] **Step 6: Delete the per-league path**

- `transaction_sync.go`: `LeagueTransactionSyncWorkflow` is gone (covered by the rewrite above).
- `data_fetch.go`: delete `GetStaleLeaguesForTransactions`, `FetchLeagueTransactions`, `MarkLeagueTransactionsFetched`.
- `params.go`: delete `FetchLeagueTransactionsParams`, `MarkLeagueTransactionsFetchedParams`. Keep `LeagueSyncParams` (drafts use it) and `GetStaleLeaguesParams` (drafts use it).
- `cmd/worker/main.go`: delete the line `transactionsw.RegisterWorkflow(workflows.LeagueTransactionSyncWorkflow)` and update the comment above it.
- Delete now-orphaned tests in `data_fetch_test.go` and `workflows_test.go` that exercise the removed functions (`grep -n "FetchLeagueTransactions\|GetStaleLeaguesForTransactions\|MarkLeagueTransactionsFetched\|LeagueTransactionSyncWorkflow" backend/internal/... ` to find them all).

- [ ] **Step 7: Run the full suite and build**

Run: `cd backend && go build ./... && go test ./...`
Expected: PASS, no references to deleted symbols (`grep -rn "LeagueTransactionSyncWorkflow\|GetStaleLeaguesForTransactions" backend/` returns nothing).

- [ ] **Step 8: Commit**

```bash
git add backend/
git commit -m "feat: claim-drain dispatcher for transaction sync; drop per-league workflows (#140)"
```

---

### Task 6: Rollout documentation

**Files:**
- Modify: `docs/superpowers/specs/2026-07-05-sleeper-sync-throughput-design.md` (no changes needed — reference only)
- Create: `docs/transaction-sync-operations.md`

**Interfaces:** none — operator documentation.

- [ ] **Step 1: Write the ops doc**

```markdown
# Transaction Sync Operations

## Tuning knobs (env, per worker process)

| Var | Default | Meaning |
|-----|---------|---------|
| `SLEEPER_RPM` | 2000 | Sleeper API requests/minute budget for this process (per fleet IP). Start high, tune down if 429s appear in logs. |
| `TXN_SYNC_PARALLEL_BATCHES` | 4 | Claim→batch pipelines per dispatcher iteration. |
| `TXN_SYNC_BATCH_SIZE` | 250 | Leagues claimed per batch activity. |
| `TXN_SYNC_LEAGUE_CONCURRENCY` | 12 | Goroutines syncing leagues inside one batch activity. |

Changing dispatcher knobs needs only a worker restart (they're read by the
`GetTransactionSyncConfig` activity each run, not baked into workflow code).

## How it works

Every 5 minutes `TransactionSyncDispatcher` claims batches of stale leagues
(`claimed_at` + `FOR UPDATE SKIP LOCKED`, 20-minute claim TTL) and runs
`SyncLeagueTransactionsBatch` activities that stamp each league done as they
go. Both fleets (DigitalOcean + Raspberry Pi) poll the same queue and
partition work naturally via the claims.

## Rollout / verification

1. Apply migration 018: `cd backend && go run ./cmd/migrate up` (adds
   `claimed_at` + partial index; `CREATE INDEX CONCURRENTLY`, safe live).
2. Deploy workers (DO promotes the new deployment version on start; Pi
   self-updates within minutes — see worker versioning docs).
3. Watch `/admin` fetch-age buckets: "Never fetched" and "24h+" should shrink
   visibly within hours at default settings (~4 × 250 leagues per claim wave).
4. Watch worker logs for `rate limited (429)` — if present, lower `SLEEPER_RPM`.

## Failure modes

- Worker dies mid-batch: its leagues stay claimed for 20 minutes, then
  re-queue. Heartbeat timeout (2m) retries the activity sooner; the retry
  re-processes only leagues that weren't already stamped.
- Sleeper state endpoint down: batches fall back to the full 18-leg sweep
  (slower, still correct).
- Claim query errors: dispatcher logs and exits; next scheduled run retries.
```

- [ ] **Step 2: Commit**

```bash
git add docs/transaction-sync-operations.md
git commit -m "docs: transaction sync tuning and rollout notes (#140)"
```

---

## Final verification (before PR)

- [ ] `cd backend && go build ./... && go vet ./... && go test ./...` — all green.
- [ ] Postgres claim tests pass against a disposable Postgres (docker command in Task 3 Step 3).
- [ ] `grep -rn "LeagueTransactionSyncWorkflow\|GetStaleLeaguesForTransactions\|MarkLeagueTransactionsFetched\|FetchLeagueTransactionsParams" backend/` → no hits.
- [ ] Migration `018_league_claims.sql` runs cleanly up and down against the disposable Postgres (`DATABASE_URL=... go run ./cmd/migrate up` / `down`).
- [ ] Branch + PR per repo convention (issue branch → PR into main, reference #140). Do not merge without the user seeing test results.

## Deviations from issue #140 text (intentional, flag in PR)

1. **League-404 → `skipped_at`:** not implemented in the batch activity. `GET /v1/league/{id}/transactions/{leg}` 404s are indistinguishable from "empty leg" (existing semantics), so league liveness detection stays with the league-details fetch path. Behavior unchanged from today.
2. **K from env:** achieved via the `GetTransactionSyncConfig` activity rather than reading env in workflow code (workflows can't read env deterministically).
3. **`GetNFLState` failure:** falls back to an 18-leg sweep per the issue, logged as a warning.
