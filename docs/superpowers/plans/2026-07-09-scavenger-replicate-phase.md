# Scavenger Replicate Phase (T5) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement T5 from `docs/superpowers/plans/2026-07-07-two-database-archive.md` — the scavenger's replicate phase: copy `sleeper_leagues`, `sleeper_transactions`, `sleeper_drafts`, and `sleeper_draft_picks` from the cloud DB into the archive DB on a 6h schedule, via a new `archive-maintenance` Temporal worker. No purge (T6) — this is one-way copy only.

**Architecture:** Four new archive-side tables (no FKs, per the design doc — arrival order must not matter) mirror the cloud tables' business columns, defined as distinct Go structs (`models.ArchiveSleeper*`) so the copied field set is an explicit, auditable mapping rather than an implicit struct reuse. A new `ScavengerActivities{Cloud, Archive *gorm.DB}` holds four `Replicate*Batch` activities, each following the same shape: read a `(timestamp, id)` keyset cursor from `archive_sync_state` (archive DB), select the next batch from cloud ordered by that same keyset with a 5-minute safety lag (guards against reading a row whose insert/update transaction hasn't become visible yet), upsert into the archive table, and advance the cursor — all in one archive-DB transaction, so cursor and data can never diverge. A new `ScavengerDispatcher` workflow (matching the existing claim-drain dispatcher shape used by `DraftSyncDispatcher`/`TransactionSyncDispatcher`) drains each of the four streams in order (leagues → transactions → draft headers → draft picks) up to a per-run batch cap, on a new 6h Temporal schedule, running only on the `archive-maintenance` queue that `cmd/worker` registers when `ARCHIVE_DATABASE_URL` is set.

**Tech Stack:** Go, GORM, Temporal Go SDK (`go.temporal.io/sdk`), PostgreSQL 16 (`(a, b) > (x, y)` row-value comparison for keyset pagination).

## Global Constraints

- Module path is `backend`. PG-only tests gate on `TEST_DATABASE_URL`; locally start a disposable cluster per the T3/T4 plan's convention (`initdb`/`pg_ctl` on port 5499, no Docker daemon on this Mac).
- **In scope:** replicate only. **Out of scope, deliberately deferred to T6:** purge, `SCAVENGER_RETENTION_DAYS`, `SCAVENGER_PURGE_ENABLED`, the claim-pool-exclusion predicate, pick-count parity verification. Don't add config or code for any of that here — it has no reader yet in this PR.
- The four archive replica tables have **no foreign keys** and copy only business/data columns — cloud-only sync bookkeeping columns (`claimed_at`, `drafts_claimed_at`, `skipped_at`, `last_transactions_fetched_at`, `last_transaction_leg_fetched`) are never copied; they're meaningless once replicated and no archive-side consumer reads them.
- All four replicate activities share one cursor shape: `{time, id}` keyset pagination with a 5-minute safety lag applied uniformly (the design doc only calls this out for transactions, but the same read-skew risk — a concurrently-committing row with an earlier timestamp appearing after the cursor has already passed it — applies to any timestamp-ordered scan under concurrent writers, so apply it everywhere for one consistent, easy-to-reason-about rule).
- Draft picks have no timestamp of their own; replication is watermarked on `sleeper_drafts.last_fetched_at` (set once, when picks land — see `internal/activities/data_fetch.go`'s `fetchDraftPicks`) as a proxy signal, then copies both the draft row and all of its picks together. A draft's intermediate status changes (`pre_draft`→`drafting`) before picks land are not separately watermarked (`sleeper_drafts.updated_at` is dead — never assigned by the upsert) — this is an accepted, documented gap, not something to solve here.
- Follow existing conventions exactly: raw-SQL activity queries (like `data_fetch.go`'s claim SQL consts), `workflow.ExecuteActivity` claim-drain loops (like `transaction_sync.go`), schedule registration via `client.ScheduleOptions`/`upsert` (like `schedules/register.go`), and `testsuite.WorkflowTestSuite` + testify `mock`/`require` for workflow tests (like `workflows_test.go`).

---

## File Structure

| File | Responsibility |
|---|---|
| `backend/internal/models/archive.go` (new) | `ArchiveSleeperLeague`, `ArchiveSleeperTransaction`, `ArchiveSleeperDraft`, `ArchiveSleeperDraftPick` — archive-side row shapes |
| `backend/migrations/archive/002_sleeper_leagues.sql` (new) | Archive `sleeper_leagues` table (no FKs) |
| `backend/migrations/archive/003_sleeper_transactions.sql` (new) | Archive `sleeper_transactions` table |
| `backend/migrations/archive/004_sleeper_drafts.sql` (new) | Archive `sleeper_drafts` table |
| `backend/migrations/archive/005_sleeper_draft_picks.sql` (new) | Archive `sleeper_draft_picks` table |
| `backend/internal/dbmigrate/dbmigrate_test.go` (modify) | Extend to verify all 4 new archive tables exist after `up` |
| `backend/internal/activities/scavenger.go` (new) | `ScavengerActivities`, cursor helpers, `GetScavengerConfig`, the four `Replicate*Batch` activities |
| `backend/internal/activities/scavenger_test.go` (new) | Two-schema (cloud + archive) PG integration tests for all of the above |
| `backend/internal/activities/params.go` (modify) | Add `ScavengerConfig`, `ReplicateBatchParams`, `ReplicateBatchResult`, `ScavengerReport` |
| `backend/internal/workflows/scavenger.go` (new) | `ScavengerDispatcher` workflow |
| `backend/internal/workflows/helpers.go` (modify) | Add `TaskQueueArchive` constant |
| `backend/internal/workflows/workflows_test.go` (modify) | `ScavengerDispatcher` tests |
| `backend/schedules/register.go` (modify) | `Register` gains an `archiveEnabled bool` param; adds the 6h scavenger schedule when true |
| `backend/cmd/worker/main.go` (modify) | Register the `archive-maintenance` worker + `ScavengerActivities` when `cfg.ArchiveDB.Enabled()`; pass the flag to `schedules.Register` |

---

### Task 1: Archive replica models + migrations

**Files:**
- Create: `backend/internal/models/archive.go`
- Create: `backend/migrations/archive/002_sleeper_leagues.sql`
- Create: `backend/migrations/archive/003_sleeper_transactions.sql`
- Create: `backend/migrations/archive/004_sleeper_drafts.sql`
- Create: `backend/migrations/archive/005_sleeper_draft_picks.sql`
- Modify: `backend/internal/dbmigrate/dbmigrate_test.go`

**Interfaces:**
- Produces: `models.ArchiveSleeperLeague`, `models.ArchiveSleeperTransaction`, `models.ArchiveSleeperDraft`, `models.ArchiveSleeperDraftPick` (each with `TableName()` returning the same table name as its cloud counterpart). Task 3–6 consume these as the write-side type for each replicate activity.

- [ ] **Step 1: Write the archive-side models**

```go
// backend/internal/models/archive.go
package models

import (
	"encoding/json"
	"time"
)

// Archive* mirror their cloud counterparts (see sleeper.go) but hold only
// the columns the archive DB stores: no FKs (arrival order must not matter
// during async replication), and no cloud-only claim/sync-bookkeeping
// columns (claimed_at, drafts_claimed_at, skipped_at, last_*_fetched_at,
// last_transaction_leg_fetched) — those are transient cloud sync state, not
// data, and no archive-side reader needs them. Distinct Go types (rather
// than reusing the cloud models) keep "what gets copied" an explicit,
// visible mapping in the replicate activities instead of an implicit field
// subset.

type ArchiveSleeperLeague struct {
	SleeperLeagueID string          `gorm:"primaryKey;column:sleeper_league_id"`
	Name            string          `gorm:"column:name"`
	Season          string          `gorm:"column:season"`
	Sport           string          `gorm:"column:sport"`
	Status          string          `gorm:"column:status"`
	TotalRosters    int             `gorm:"column:total_rosters"`
	PPR             *float64        `gorm:"column:ppr"`
	TEPremium       *float64        `gorm:"column:te_premium"`
	IsSuperflex     *bool           `gorm:"column:is_superflex"`
	DraftType       string          `gorm:"column:draft_type"`
	LeagueType      string          `gorm:"column:league_type"`
	ScoringSettings json.RawMessage `gorm:"column:scoring_settings;type:jsonb"`
	RosterPositions json.RawMessage `gorm:"column:roster_positions;type:jsonb"`
	CreatedAt       time.Time       `gorm:"column:created_at"`
	UpdatedAt       time.Time       `gorm:"column:updated_at"`
}

func (ArchiveSleeperLeague) TableName() string { return "sleeper_leagues" }

type ArchiveSleeperTransaction struct {
	SleeperTransactionID string          `gorm:"primaryKey;column:sleeper_transaction_id"`
	SleeperLeagueID      string          `gorm:"column:sleeper_league_id"`
	Type                 string          `gorm:"column:type"`
	Status               string          `gorm:"column:status"`
	CreatedAtSleeper      int64          `gorm:"column:created_at_sleeper"`
	Leg                  int             `gorm:"column:leg"`
	Adds                 json.RawMessage `gorm:"column:adds;type:jsonb"`
	Drops                json.RawMessage `gorm:"column:drops;type:jsonb"`
	DraftPicks           json.RawMessage `gorm:"column:draft_picks;type:jsonb"`
	WaiverBudget         json.RawMessage `gorm:"column:waiver_budget;type:jsonb"`
	CreatedAt            time.Time       `gorm:"column:created_at"`
}

func (ArchiveSleeperTransaction) TableName() string { return "sleeper_transactions" }

type ArchiveSleeperDraft struct {
	SleeperDraftID  string     `gorm:"primaryKey;column:sleeper_draft_id"`
	SleeperLeagueID string     `gorm:"column:sleeper_league_id"`
	Type            string     `gorm:"column:type"`
	Status          string     `gorm:"column:status"`
	Season          string     `gorm:"column:season"`
	LastFetchedAt   *time.Time `gorm:"column:last_fetched_at"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (ArchiveSleeperDraft) TableName() string { return "sleeper_drafts" }

type ArchiveSleeperDraftPick struct {
	SleeperDraftID  string          `gorm:"primaryKey;column:sleeper_draft_id"`
	Round           int             `gorm:"primaryKey;column:round"`
	PickNo          int             `gorm:"primaryKey;column:pick_no"`
	RosterID        int             `gorm:"column:roster_id"`
	PickedByUserID  string          `gorm:"column:picked_by_user_id"`
	SleeperPlayerID string          `gorm:"column:sleeper_player_id"`
	Metadata        json.RawMessage `gorm:"column:metadata;type:jsonb"`
}

func (ArchiveSleeperDraftPick) TableName() string { return "sleeper_draft_picks" }
```

- [ ] **Step 2: Write the four migrations**

```sql
-- backend/migrations/archive/002_sleeper_leagues.sql
-- +goose Up

CREATE TABLE sleeper_leagues (
    sleeper_league_id text PRIMARY KEY,
    name text,
    season text,
    sport text,
    status text,
    total_rosters integer,
    ppr double precision,
    te_premium double precision,
    is_superflex boolean,
    draft_type text,
    league_type text,
    scoring_settings jsonb,
    roster_positions jsonb,
    created_at timestamptz,
    updated_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_archive_sleeper_leagues_updated_at
    ON sleeper_leagues (updated_at, sleeper_league_id);

-- +goose Down

DROP TABLE sleeper_leagues;
```

```sql
-- backend/migrations/archive/003_sleeper_transactions.sql
-- +goose Up

CREATE TABLE sleeper_transactions (
    sleeper_transaction_id text PRIMARY KEY,
    sleeper_league_id text,
    type text,
    status text,
    created_at_sleeper bigint,
    leg integer,
    adds jsonb,
    drops jsonb,
    draft_picks jsonb,
    waiver_budget jsonb,
    created_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_archive_sleeper_transactions_created_at
    ON sleeper_transactions (created_at, sleeper_transaction_id);

-- +goose Down

DROP TABLE sleeper_transactions;
```

```sql
-- backend/migrations/archive/004_sleeper_drafts.sql
-- +goose Up

CREATE TABLE sleeper_drafts (
    sleeper_draft_id text PRIMARY KEY,
    sleeper_league_id text,
    type text,
    status text,
    season text,
    last_fetched_at timestamptz,
    created_at timestamptz,
    updated_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_archive_sleeper_drafts_created_at
    ON sleeper_drafts (created_at, sleeper_draft_id);
CREATE INDEX IF NOT EXISTS idx_archive_sleeper_drafts_last_fetched_at
    ON sleeper_drafts (last_fetched_at, sleeper_draft_id);

-- +goose Down

DROP TABLE sleeper_drafts;
```

```sql
-- backend/migrations/archive/005_sleeper_draft_picks.sql
-- +goose Up

CREATE TABLE sleeper_draft_picks (
    sleeper_draft_id text NOT NULL,
    round integer NOT NULL,
    pick_no integer NOT NULL,
    roster_id integer,
    picked_by_user_id text,
    sleeper_player_id text,
    metadata jsonb,
    PRIMARY KEY (sleeper_draft_id, round, pick_no)
);

CREATE INDEX IF NOT EXISTS idx_archive_sleeper_draft_picks_draft_id
    ON sleeper_draft_picks (sleeper_draft_id);

-- +goose Down

DROP TABLE sleeper_draft_picks;
```

All four are brand-new empty tables, so plain (transactional) `CREATE TABLE`/`CREATE INDEX` is fine — no `CONCURRENTLY`/`NO TRANSACTION` needed (that's only for indexing already-populated tables, like cloud migration 021).

- [ ] **Step 3: Extend the dbmigrate test to verify the new tables**

Add to `backend/internal/dbmigrate/dbmigrate_test.go` (after the existing `TestRun_ArchiveMigrations_CreatesSyncStateTable`):

```go
func TestRun_ArchiveMigrations_CreatesReplicaTables(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	scopedDSN := testutil.NewPGSchema(t, dsn, "archive_replica_migrate_test")

	if err := dbmigrate.Run(scopedDSN, archivemigrations.FS, "up", nil); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	for _, table := range []string{"sleeper_leagues", "sleeper_transactions", "sleeper_drafts", "sleeper_draft_picks"} {
		if !tableExists(t, scopedDSN, table) {
			t.Errorf("expected archive table %s to exist after migrate up", table)
		}
	}
}
```

(`tableExists` already exists in this file from the T3/T4 work — no new helper needed.)

- [ ] **Step 4: Run tests**

Run (with a disposable Postgres — `initdb`/`pg_ctl` on port 5499, `TEST_DATABASE_URL="postgres://$(whoami)@localhost:5499/postgres?sslmode=disable"`):
`cd backend && go build ./... && go test ./internal/dbmigrate/... -v`
Expected: all PASS, including the new `TestRun_ArchiveMigrations_CreatesReplicaTables`.

- [ ] **Step 5: Commit**

```bash
git add internal/models/archive.go migrations/archive/002_sleeper_leagues.sql migrations/archive/003_sleeper_transactions.sql migrations/archive/004_sleeper_drafts.sql migrations/archive/005_sleeper_draft_picks.sql internal/dbmigrate/dbmigrate_test.go
git commit -m "feat: add archive replica tables (leagues, transactions, drafts, draft_picks)"
```

---

### Task 2: Scavenger activities skeleton — config, cursor helpers, two-schema test setup

**Files:**
- Create: `backend/internal/activities/scavenger.go`
- Create: `backend/internal/activities/scavenger_test.go`
- Modify: `backend/internal/activities/params.go`

**Interfaces:**
- Produces: `ScavengerActivities{Cloud, Archive *gorm.DB}`, `ScavengerConfig{LeagueBatchSize, TxnBatchSize, DraftBatchSize, MaxBatchesPerRun int}`, `(a *ScavengerActivities) GetScavengerConfig(ctx) (ScavengerConfig, error)`, unexported `cursor{Time time.Time, ID string}` + `readCursor`/`writeCursor` + stream name constants + `scavengerSafetyLag`. Produces `newScavengerTestDBs(t) (cloud, archive *gorm.DB)` test helper, consumed by Tasks 3–6.
- Consumes: `models.Archive*` (Task 1), `internal/dbmigrate.Run` + `archivemigrations.FS` (existing, from T3/T4), `internal/testutil` (existing).

- [ ] **Step 1: Add config/param types to params.go**

Add to `backend/internal/activities/params.go`:

```go
// ScavengerConfig is read from env by GetScavengerConfig so the dispatcher
// workflow (which cannot read env deterministically) can be tuned without a
// redeploy of workflow code.
type ScavengerConfig struct {
	LeagueBatchSize  int // SCAVENGER_LEAGUE_BATCH_SIZE, default 500
	TxnBatchSize     int // SCAVENGER_TXN_BATCH_SIZE, default 5000
	DraftBatchSize   int // SCAVENGER_DRAFT_BATCH_SIZE, default 200 (drafts per batch; each draft's picks are copied alongside it)
	MaxBatchesPerRun int // SCAVENGER_MAX_BATCHES_PER_RUN, default 50
}

// ReplicateBatchParams is shared by all four Replicate*Batch activities —
// they differ only in which stream/table they read and write.
type ReplicateBatchParams struct {
	BatchSize int
}

// ReplicateBatchResult reports one batch's outcome. Drained means fewer than
// BatchSize rows were found — the stream is caught up for this run.
type ReplicateBatchResult struct {
	Replicated int
	Drained    bool
}

// ScavengerReport summarizes one ScavengerDispatcher run.
type ScavengerReport struct {
	LeaguesReplicated      int
	TransactionsReplicated int
	DraftHeadersReplicated int
	DraftPicksReplicated   int
}
```

- [ ] **Step 2: Write the failing test for GetScavengerConfig**

```go
// backend/internal/activities/scavenger_test.go
package activities_test

import (
	"context"
	"os"
	"testing"

	"gorm.io/gorm"

	"backend/internal/activities"
	"backend/internal/dbmigrate"
	"backend/internal/models"
	"backend/internal/testutil"
	archivemigrations "backend/migrations/archive"
)

func TestGetScavengerConfig_ReadsEnvWithDefaults(t *testing.T) {
	a := &activities.ScavengerActivities{}
	cfg, err := a.GetScavengerConfig(context.Background())
	if err != nil {
		t.Fatalf("GetScavengerConfig: %v", err)
	}
	if cfg.LeagueBatchSize != 500 || cfg.TxnBatchSize != 5000 || cfg.DraftBatchSize != 200 || cfg.MaxBatchesPerRun != 50 {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestGetScavengerConfig_ReadsOverrides(t *testing.T) {
	t.Setenv("SCAVENGER_LEAGUE_BATCH_SIZE", "10")
	t.Setenv("SCAVENGER_TXN_BATCH_SIZE", "20")
	t.Setenv("SCAVENGER_DRAFT_BATCH_SIZE", "30")
	t.Setenv("SCAVENGER_MAX_BATCHES_PER_RUN", "5")

	a := &activities.ScavengerActivities{}
	cfg, err := a.GetScavengerConfig(context.Background())
	if err != nil {
		t.Fatalf("GetScavengerConfig: %v", err)
	}
	want := activities.ScavengerConfig{LeagueBatchSize: 10, TxnBatchSize: 20, DraftBatchSize: 30, MaxBatchesPerRun: 5}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

// newScavengerTestDBs opens two throwaway schemas on TEST_DATABASE_URL — one
// migrated with the cloud sleeper models (AutoMigrate, matching the existing
// claim_pg_test.go convention), one migrated with the real archive goose
// migrations (dbmigrate.Run against archivemigrations.FS, so these tests
// also exercise the actual migration files, not just the Go structs).
func newScavengerTestDBs(t *testing.T) (cloud, archive *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	cloudDSN := testutil.NewPGSchema(t, dsn, "scavenger_cloud")
	cloud = testutil.OpenGORM(t, cloudDSN)
	if err := cloud.AutoMigrate(&models.SleeperLeague{}, &models.SleeperTransaction{}, &models.SleeperDraft{}, &models.SleeperDraftPick{}); err != nil {
		t.Fatalf("automigrate cloud: %v", err)
	}

	archiveDSN := testutil.NewPGSchema(t, dsn, "scavenger_archive")
	if err := dbmigrate.Run(archiveDSN, archivemigrations.FS, "up", nil); err != nil {
		t.Fatalf("migrate archive: %v", err)
	}
	archive = testutil.OpenGORM(t, archiveDSN)

	return cloud, archive
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd backend && go vet ./internal/activities/...`
Expected: FAIL — `activities.ScavengerActivities`/`ScavengerConfig`/`GetScavengerConfig` undefined (compile error).

- [ ] **Step 4: Implement the skeleton**

```go
// backend/internal/activities/scavenger.go
package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"backend/internal/helpers"
)

// ScavengerActivities holds dependencies for the archive scavenger's
// replicate-phase activities: Cloud is the hot 30-day store, Archive is the
// full-history store. Only the worker constructs this, and only when
// ARCHIVE_DATABASE_URL is set — see cmd/worker/main.go.
type ScavengerActivities struct {
	Cloud   *gorm.DB
	Archive *gorm.DB
}

// scavengerSafetyLag bounds every replicate query's upper timestamp edge.
// Guards against reading a row whose insert/update transaction hasn't
// become visible yet under concurrent writers — without it, a keyset cursor
// could advance past a timestamp before a concurrently-committing row at
// that same timestamp becomes visible, silently skipping it forever.
const scavengerSafetyLag = 5 * time.Minute

const (
	streamLeagues      = "sleeper_leagues"
	streamTransactions = "sleeper_transactions"
	streamDraftHeaders = "sleeper_drafts_headers"
	streamDraftPicks   = "sleeper_drafts_picks"
)

// GetScavengerConfig returns the scavenger's tuning knobs from env, clamped
// to at least 1 so a bad value can't stall replication or break a query's
// LIMIT.
func (a *ScavengerActivities) GetScavengerConfig(ctx context.Context) (ScavengerConfig, error) {
	return ScavengerConfig{
		LeagueBatchSize:  max(helpers.GetEnv("SCAVENGER_LEAGUE_BATCH_SIZE", 500), 1),
		TxnBatchSize:     max(helpers.GetEnv("SCAVENGER_TXN_BATCH_SIZE", 5000), 1),
		DraftBatchSize:   max(helpers.GetEnv("SCAVENGER_DRAFT_BATCH_SIZE", 200), 1),
		MaxBatchesPerRun: max(helpers.GetEnv("SCAVENGER_MAX_BATCHES_PER_RUN", 50), 1),
	}, nil
}

// cursor is the keyset position for one replicate stream: every stream
// orders by (timestamp, id) and stores its progress as this same shape in
// archive_sync_state.cursor_state.
type cursor struct {
	Time time.Time `json:"time"`
	ID   string    `json:"id"`
}

// readCursor loads stream's cursor from archive_sync_state. A missing row
// (first run) returns the zero cursor, which naturally selects everything
// on the first batch since every real timestamp is after time.Time{}.
func readCursor(ctx context.Context, archive *gorm.DB, stream string) (cursor, error) {
	var row struct {
		CursorState json.RawMessage `gorm:"column:cursor_state"`
	}
	err := archive.WithContext(ctx).
		Table("archive_sync_state").
		Select("cursor_state").
		Where("stream = ?", stream).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return cursor{}, nil
	}
	if err != nil {
		return cursor{}, err
	}
	var c cursor
	if err := json.Unmarshal(row.CursorState, &c); err != nil {
		return cursor{}, fmt.Errorf("unmarshal cursor for stream %s: %w", stream, err)
	}
	return c, nil
}

// writeCursor upserts stream's cursor inside tx, so the cursor advance
// commits atomically with the rows it describes: a crash between the two
// would otherwise risk the cursor moving past rows that were never actually
// written. Callers must run this inside the same transaction as the batch's
// row upserts.
func writeCursor(tx *gorm.DB, stream string, c cursor) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return tx.Exec(
		`INSERT INTO archive_sync_state (stream, cursor_state, updated_at) VALUES (?, ?, now())
		 ON CONFLICT (stream) DO UPDATE SET cursor_state = excluded.cursor_state, updated_at = excluded.updated_at`,
		stream, data,
	).Error
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd backend && go build ./... && go test ./internal/activities/... -run TestGetScavengerConfig -v`
Expected: both PASS (no DB needed for these two).

- [ ] **Step 6: Commit**

```bash
git add internal/activities/scavenger.go internal/activities/scavenger_test.go internal/activities/params.go
git commit -m "feat: add ScavengerActivities skeleton (config, cursor helpers, test DB setup)"
```

---

### Task 3: ReplicateLeaguesBatch

**Files:**
- Modify: `backend/internal/activities/scavenger.go`
- Modify: `backend/internal/activities/scavenger_test.go`

**Interfaces:**
- Produces: `(a *ScavengerActivities) ReplicateLeaguesBatch(ctx, ReplicateBatchParams) (ReplicateBatchResult, error)`. Consumed by Task 7's workflow.

- [ ] **Step 1: Write the failing tests**

Append to `scavenger_test.go`:

```go
func TestReplicateLeaguesBatch_CopiesRowsAndAdvancesCursor(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute) // outside the 5-min safety lag
	ppr := 1.0
	for i, id := range []string{"lg1", "lg2"} {
		if err := cloud.Create(&models.SleeperLeague{
			SleeperLeagueID: id, Name: "League " + id, Season: "2026", LeagueType: "redraft",
			PPR: &ppr, UpdatedAt: now.Add(time.Duration(i) * time.Second),
		}).Error; err != nil {
			t.Fatalf("seed league %s: %v", id, err)
		}
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.ReplicateLeaguesBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("ReplicateLeaguesBatch: %v", err)
	}
	if res.Replicated != 2 || !res.Drained {
		t.Errorf("res = %+v, want {Replicated: 2, Drained: true}", res)
	}

	var count int64
	archive.Table("sleeper_leagues").Count(&count)
	if count != 2 {
		t.Errorf("expected 2 archived leagues, got %d", count)
	}
	var got models.ArchiveSleeperLeague
	if err := archive.Where("sleeper_league_id = ?", "lg1").First(&got).Error; err != nil {
		t.Fatalf("fetch archived league: %v", err)
	}
	if got.Name != "League lg1" || got.LeagueType != "redraft" || got.PPR == nil || *got.PPR != 1.0 {
		t.Errorf("archived row mismatch: %+v", got)
	}
}

func TestReplicateLeaguesBatch_SecondRunIsNoOp(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	if err := cloud.Create(&models.SleeperLeague{SleeperLeagueID: "lg1", Season: "2026", UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	if _, err := a.ReplicateLeaguesBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	res, err := a.ReplicateLeaguesBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Replicated != 0 || !res.Drained {
		t.Errorf("second run = %+v, want {Replicated: 0, Drained: true}", res)
	}
}

func TestReplicateLeaguesBatch_RespectsSafetyLag(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	tooRecent := time.Now().UTC().Add(-1 * time.Minute) // inside the 5-min lag
	if err := cloud.Create(&models.SleeperLeague{SleeperLeagueID: "lg1", Season: "2026", UpdatedAt: tooRecent}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.ReplicateLeaguesBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("ReplicateLeaguesBatch: %v", err)
	}
	if res.Replicated != 0 {
		t.Errorf("expected the too-recent league to be excluded by the safety lag, got %+v", res)
	}
}

func TestReplicateLeaguesBatch_DrainedWhenFewerThanBatchSize(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	for i, id := range []string{"lg1", "lg2", "lg3"} {
		if err := cloud.Create(&models.SleeperLeague{SleeperLeagueID: id, Season: "2026", UpdatedAt: now.Add(time.Duration(i) * time.Second)}).Error; err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.ReplicateLeaguesBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 2})
	if err != nil {
		t.Fatalf("ReplicateLeaguesBatch: %v", err)
	}
	if res.Replicated != 2 || res.Drained {
		t.Errorf("expected a full, non-drained batch of 2, got %+v", res)
	}
}
```

Add `"time"` to the import block if not already present (it will be after Task 2's `newScavengerTestDBs`, which doesn't use `time` yet — add it now).

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go vet ./internal/activities/...`
Expected: FAIL — `ReplicateLeaguesBatch` undefined.

- [ ] **Step 3: Implement**

Append to `scavenger.go`:

```go
const selectLeaguesBatchSQL = `
SELECT sleeper_league_id, name, season, sport, status, total_rosters, ppr, te_premium, is_superflex,
       draft_type, league_type, scoring_settings, roster_positions, created_at, updated_at
FROM sleeper_leagues
WHERE (updated_at, sleeper_league_id) > (?, ?)
  AND updated_at <= ?
ORDER BY updated_at, sleeper_league_id
LIMIT ?`

// ReplicateLeaguesBatch copies up to BatchSize leagues from cloud to archive,
// ordered by (updated_at, sleeper_league_id), and advances the leagues
// cursor. Leagues are replicated because the ADP rollup (T7) joins drafts to
// leagues for league_type/ppr/total_rosters/is_superflex — nothing else in
// the archive currently needs sleeper_leagues, but it's small and this join
// dependency is enough to justify replicating it in full.
func (a *ScavengerActivities) ReplicateLeaguesBatch(ctx context.Context, params ReplicateBatchParams) (ReplicateBatchResult, error) {
	cur, err := readCursor(ctx, a.Archive, streamLeagues)
	if err != nil {
		return ReplicateBatchResult{}, err
	}

	var rows []models.SleeperLeague
	if err := a.Cloud.WithContext(ctx).Raw(selectLeaguesBatchSQL,
		cur.Time, cur.ID, time.Now().UTC().Add(-scavengerSafetyLag), params.BatchSize,
	).Scan(&rows).Error; err != nil {
		return ReplicateBatchResult{}, err
	}
	if len(rows) == 0 {
		return ReplicateBatchResult{Drained: true}, nil
	}

	archiveRows := make([]models.ArchiveSleeperLeague, len(rows))
	for i, r := range rows {
		archiveRows[i] = models.ArchiveSleeperLeague{
			SleeperLeagueID: r.SleeperLeagueID, Name: r.Name, Season: r.Season, Sport: r.Sport,
			Status: r.Status, TotalRosters: r.TotalRosters, PPR: r.PPR, TEPremium: r.TEPremium,
			IsSuperflex: r.IsSuperflex, DraftType: r.DraftType, LeagueType: r.LeagueType,
			ScoringSettings: r.ScoringSettings, RosterPositions: r.RosterPositions,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	}
	last := rows[len(rows)-1]
	newCursor := cursor{Time: last.UpdatedAt, ID: last.SleeperLeagueID}

	err = a.Archive.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "sleeper_league_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name", "season", "sport", "status", "total_rosters", "ppr", "te_premium",
				"is_superflex", "draft_type", "league_type", "scoring_settings", "roster_positions", "updated_at",
			}),
		}).CreateInBatches(archiveRows, 500).Error; err != nil {
			return err
		}
		return writeCursor(tx, streamLeagues, newCursor)
	})
	if err != nil {
		return ReplicateBatchResult{}, err
	}
	return ReplicateBatchResult{Replicated: len(rows), Drained: len(rows) < params.BatchSize}, nil
}
```

Add `"gorm.io/gorm/clause"` and `"backend/internal/models"` to the import block.

- [ ] **Step 4: Run tests to verify they pass**

Run (with `TEST_DATABASE_URL` set): `cd backend && go test ./internal/activities/... -run TestReplicateLeaguesBatch -v`
Expected: all 4 PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/activities/scavenger.go internal/activities/scavenger_test.go
git commit -m "feat: add ReplicateLeaguesBatch scavenger activity"
```

---

### Task 4: ReplicateTransactionsBatch

**Files:**
- Modify: `backend/internal/activities/scavenger.go`
- Modify: `backend/internal/activities/scavenger_test.go`

**Interfaces:**
- Produces: `(a *ScavengerActivities) ReplicateTransactionsBatch(ctx, ReplicateBatchParams) (ReplicateBatchResult, error)`. Consumed by Task 7.

- [ ] **Step 1: Write the failing tests**

Append to `scavenger_test.go`:

```go
func TestReplicateTransactionsBatch_CopiesRowsAndAdvancesCursor(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	for i, id := range []string{"t1", "t2"} {
		if err := cloud.Create(&models.SleeperTransaction{
			SleeperTransactionID: id, SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
			CreatedAtSleeper: 1000, CreatedAt: now.Add(time.Duration(i) * time.Second),
		}).Error; err != nil {
			t.Fatalf("seed txn %s: %v", id, err)
		}
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.ReplicateTransactionsBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("ReplicateTransactionsBatch: %v", err)
	}
	if res.Replicated != 2 || !res.Drained {
		t.Errorf("res = %+v, want {Replicated: 2, Drained: true}", res)
	}
	var got models.ArchiveSleeperTransaction
	if err := archive.Where("sleeper_transaction_id = ?", "t1").First(&got).Error; err != nil {
		t.Fatalf("fetch archived txn: %v", err)
	}
	if got.Type != "trade" || got.SleeperLeagueID != "lg1" {
		t.Errorf("archived row mismatch: %+v", got)
	}
}

func TestReplicateTransactionsBatch_SecondRunIsNoOp(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	if err := cloud.Create(&models.SleeperTransaction{SleeperTransactionID: "t1", CreatedAt: now}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	if _, err := a.ReplicateTransactionsBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	res, err := a.ReplicateTransactionsBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Replicated != 0 || !res.Drained {
		t.Errorf("second run = %+v, want {Replicated: 0, Drained: true}", res)
	}
}

func TestReplicateTransactionsBatch_RespectsSafetyLag(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	tooRecent := time.Now().UTC().Add(-1 * time.Minute)
	if err := cloud.Create(&models.SleeperTransaction{SleeperTransactionID: "t1", CreatedAt: tooRecent}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.ReplicateTransactionsBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("ReplicateTransactionsBatch: %v", err)
	}
	if res.Replicated != 0 {
		t.Errorf("expected the too-recent txn to be excluded by the safety lag, got %+v", res)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go vet ./internal/activities/...`
Expected: FAIL — `ReplicateTransactionsBatch` undefined.

- [ ] **Step 3: Implement**

Append to `scavenger.go`:

```go
const selectTransactionsBatchSQL = `
SELECT sleeper_transaction_id, sleeper_league_id, type, status, created_at_sleeper, leg,
       adds, drops, draft_picks, waiver_budget, created_at
FROM sleeper_transactions
WHERE (created_at, sleeper_transaction_id) > (?, ?)
  AND created_at <= ?
ORDER BY created_at, sleeper_transaction_id
LIMIT ?`

// ReplicateTransactionsBatch copies up to BatchSize transactions from cloud
// to archive, ordered by (created_at, sleeper_transaction_id). Transactions
// are insert-only and immutable in cloud, so the archive upsert is
// DoNothing on conflict — a replay can never need to overwrite a row.
func (a *ScavengerActivities) ReplicateTransactionsBatch(ctx context.Context, params ReplicateBatchParams) (ReplicateBatchResult, error) {
	cur, err := readCursor(ctx, a.Archive, streamTransactions)
	if err != nil {
		return ReplicateBatchResult{}, err
	}

	var rows []models.SleeperTransaction
	if err := a.Cloud.WithContext(ctx).Raw(selectTransactionsBatchSQL,
		cur.Time, cur.ID, time.Now().UTC().Add(-scavengerSafetyLag), params.BatchSize,
	).Scan(&rows).Error; err != nil {
		return ReplicateBatchResult{}, err
	}
	if len(rows) == 0 {
		return ReplicateBatchResult{Drained: true}, nil
	}

	archiveRows := make([]models.ArchiveSleeperTransaction, len(rows))
	for i, r := range rows {
		archiveRows[i] = models.ArchiveSleeperTransaction{
			SleeperTransactionID: r.SleeperTransactionID, SleeperLeagueID: r.SleeperLeagueID,
			Type: r.Type, Status: r.Status, CreatedAtSleeper: r.CreatedAtSleeper, Leg: r.Leg,
			Adds: r.Adds, Drops: r.Drops, DraftPicks: r.DraftPicks, WaiverBudget: r.WaiverBudget,
			CreatedAt: r.CreatedAt,
		}
	}
	last := rows[len(rows)-1]
	newCursor := cursor{Time: last.CreatedAt, ID: last.SleeperTransactionID}

	err = a.Archive.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(archiveRows, 500).Error; err != nil {
			return err
		}
		return writeCursor(tx, streamTransactions, newCursor)
	})
	if err != nil {
		return ReplicateBatchResult{}, err
	}
	return ReplicateBatchResult{Replicated: len(rows), Drained: len(rows) < params.BatchSize}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/activities/... -run TestReplicateTransactionsBatch -v`
Expected: all 3 PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/activities/scavenger.go internal/activities/scavenger_test.go
git commit -m "feat: add ReplicateTransactionsBatch scavenger activity"
```

---

### Task 5: ReplicateDraftHeadersBatch

**Files:**
- Modify: `backend/internal/activities/scavenger.go`
- Modify: `backend/internal/activities/scavenger_test.go`

**Interfaces:**
- Produces: `(a *ScavengerActivities) ReplicateDraftHeadersBatch(ctx, ReplicateBatchParams) (ReplicateBatchResult, error)`. Consumed by Task 7.

- [ ] **Step 1: Write the failing tests**

Append to `scavenger_test.go`:

```go
func TestReplicateDraftHeadersBatch_CopiesRowsAndAdvancesCursor(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	for i, id := range []string{"d1", "d2"} {
		if err := cloud.Create(&models.SleeperDraft{
			SleeperDraftID: id, SleeperLeagueID: "lg1", Type: "snake", Status: "pre_draft",
			Season: "2026", CreatedAt: now.Add(time.Duration(i) * time.Second),
		}).Error; err != nil {
			t.Fatalf("seed draft %s: %v", id, err)
		}
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.ReplicateDraftHeadersBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("ReplicateDraftHeadersBatch: %v", err)
	}
	if res.Replicated != 2 || !res.Drained {
		t.Errorf("res = %+v, want {Replicated: 2, Drained: true}", res)
	}
	var got models.ArchiveSleeperDraft
	if err := archive.Where("sleeper_draft_id = ?", "d1").First(&got).Error; err != nil {
		t.Fatalf("fetch archived draft: %v", err)
	}
	if got.Type != "snake" || got.Status != "pre_draft" {
		t.Errorf("archived row mismatch: %+v", got)
	}
}

func TestReplicateDraftHeadersBatch_SecondRunIsNoOp(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	if err := cloud.Create(&models.SleeperDraft{SleeperDraftID: "d1", Season: "2026", CreatedAt: now}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	if _, err := a.ReplicateDraftHeadersBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	res, err := a.ReplicateDraftHeadersBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Replicated != 0 || !res.Drained {
		t.Errorf("second run = %+v, want {Replicated: 0, Drained: true}", res)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go vet ./internal/activities/...`
Expected: FAIL — `ReplicateDraftHeadersBatch` undefined.

- [ ] **Step 3: Implement**

Append to `scavenger.go`:

```go
const selectDraftHeadersBatchSQL = `
SELECT sleeper_draft_id, sleeper_league_id, type, status, season, last_fetched_at, created_at, updated_at
FROM sleeper_drafts
WHERE (created_at, sleeper_draft_id) > (?, ?)
  AND created_at <= ?
ORDER BY created_at, sleeper_draft_id
LIMIT ?`

// ReplicateDraftHeadersBatch copies up to BatchSize draft rows from cloud to
// archive, ordered by (created_at, sleeper_draft_id) — this catches new
// drafts as they're first created. It does not catch later status changes on
// an existing draft (sleeper_drafts.updated_at is dead — never assigned by
// the upsert in data_fetch.go); those are caught separately, once picks
// land, by ReplicateDraftPicksBatch's last_fetched_at watermark.
func (a *ScavengerActivities) ReplicateDraftHeadersBatch(ctx context.Context, params ReplicateBatchParams) (ReplicateBatchResult, error) {
	cur, err := readCursor(ctx, a.Archive, streamDraftHeaders)
	if err != nil {
		return ReplicateBatchResult{}, err
	}

	var rows []models.SleeperDraft
	if err := a.Cloud.WithContext(ctx).Raw(selectDraftHeadersBatchSQL,
		cur.Time, cur.ID, time.Now().UTC().Add(-scavengerSafetyLag), params.BatchSize,
	).Scan(&rows).Error; err != nil {
		return ReplicateBatchResult{}, err
	}
	if len(rows) == 0 {
		return ReplicateBatchResult{Drained: true}, nil
	}

	archiveRows := make([]models.ArchiveSleeperDraft, len(rows))
	for i, r := range rows {
		archiveRows[i] = models.ArchiveSleeperDraft{
			SleeperDraftID: r.SleeperDraftID, SleeperLeagueID: r.SleeperLeagueID, Type: r.Type,
			Status: r.Status, Season: r.Season, LastFetchedAt: r.LastFetchedAt,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	}
	last := rows[len(rows)-1]
	newCursor := cursor{Time: last.CreatedAt, ID: last.SleeperDraftID}

	err = a.Archive.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "sleeper_draft_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"sleeper_league_id", "type", "status", "season", "last_fetched_at", "updated_at",
			}),
		}).CreateInBatches(archiveRows, 500).Error; err != nil {
			return err
		}
		return writeCursor(tx, streamDraftHeaders, newCursor)
	})
	if err != nil {
		return ReplicateBatchResult{}, err
	}
	return ReplicateBatchResult{Replicated: len(rows), Drained: len(rows) < params.BatchSize}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/activities/... -run TestReplicateDraftHeadersBatch -v`
Expected: both PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/activities/scavenger.go internal/activities/scavenger_test.go
git commit -m "feat: add ReplicateDraftHeadersBatch scavenger activity"
```

---

### Task 6: ReplicateDraftPicksBatch

**Files:**
- Modify: `backend/internal/activities/scavenger.go`
- Modify: `backend/internal/activities/scavenger_test.go`

**Interfaces:**
- Produces: `(a *ScavengerActivities) ReplicateDraftPicksBatch(ctx, ReplicateBatchParams) (ReplicateBatchResult, error)`. Consumed by Task 7.

- [ ] **Step 1: Write the failing tests**

Append to `scavenger_test.go`:

```go
func TestReplicateDraftPicksBatch_CopiesDraftAndPicksWhenLastFetchedAtSet(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	fetchedAt := now
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Type: "snake", Status: "complete",
		Season: "2026", LastFetchedAt: &fetchedAt, CreatedAt: now.Add(-time.Hour),
	}).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	for i := 1; i <= 2; i++ {
		if err := cloud.Create(&models.SleeperDraftPick{
			SleeperDraftID: "d1", Round: 1, PickNo: i, RosterID: i, SleeperPlayerID: fmt.Sprintf("p%d", i),
		}).Error; err != nil {
			t.Fatalf("seed pick %d: %v", i, err)
		}
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.ReplicateDraftPicksBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("ReplicateDraftPicksBatch: %v", err)
	}
	if res.Replicated != 1 || !res.Drained {
		t.Errorf("res = %+v, want {Replicated: 1, Drained: true} (1 draft)", res)
	}

	var draft models.ArchiveSleeperDraft
	if err := archive.Where("sleeper_draft_id = ?", "d1").First(&draft).Error; err != nil {
		t.Fatalf("fetch archived draft: %v", err)
	}
	if draft.Status != "complete" || draft.LastFetchedAt == nil {
		t.Errorf("archived draft mismatch: %+v", draft)
	}
	var pickCount int64
	archive.Model(&models.ArchiveSleeperDraftPick{}).Where("sleeper_draft_id = ?", "d1").Count(&pickCount)
	if pickCount != 2 {
		t.Errorf("expected 2 archived picks, got %d", pickCount)
	}
}

func TestReplicateDraftPicksBatch_SkipsDraftsWithoutPicksYet(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", Status: "pre_draft", Season: "2026", CreatedAt: now, LastFetchedAt: nil,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.ReplicateDraftPicksBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("ReplicateDraftPicksBatch: %v", err)
	}
	if res.Replicated != 0 || !res.Drained {
		t.Errorf("expected no drafts eligible (last_fetched_at NULL), got %+v", res)
	}
}

func TestReplicateDraftPicksBatch_SecondRunIsNoOp(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	fetchedAt := now
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", Status: "complete", Season: "2026", LastFetchedAt: &fetchedAt, CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	if _, err := a.ReplicateDraftPicksBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	res, err := a.ReplicateDraftPicksBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Replicated != 0 || !res.Drained {
		t.Errorf("second run = %+v, want {Replicated: 0, Drained: true}", res)
	}
}
```

Add `"fmt"` to the import block if not already present.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go vet ./internal/activities/...`
Expected: FAIL — `ReplicateDraftPicksBatch` undefined.

- [ ] **Step 3: Implement**

Append to `scavenger.go`:

```go
const selectDraftsByPicksWatermarkSQL = `
SELECT sleeper_draft_id, sleeper_league_id, type, status, season, last_fetched_at, created_at, updated_at
FROM sleeper_drafts
WHERE last_fetched_at IS NOT NULL
  AND (last_fetched_at, sleeper_draft_id) > (?, ?)
  AND last_fetched_at <= ?
ORDER BY last_fetched_at, sleeper_draft_id
LIMIT ?`

// ReplicateDraftPicksBatch copies up to BatchSize drafts (plus all of their
// picks) from cloud to archive, watermarked on sleeper_drafts.last_fetched_at
// — the signal that picks have landed (set once, in data_fetch.go's
// fetchDraftPicks). This also re-copies the draft row itself, so by the time
// a draft's picks are replicated its status is current too (picks are only
// fetched once a draft reaches "complete").
func (a *ScavengerActivities) ReplicateDraftPicksBatch(ctx context.Context, params ReplicateBatchParams) (ReplicateBatchResult, error) {
	cur, err := readCursor(ctx, a.Archive, streamDraftPicks)
	if err != nil {
		return ReplicateBatchResult{}, err
	}

	var drafts []models.SleeperDraft
	if err := a.Cloud.WithContext(ctx).Raw(selectDraftsByPicksWatermarkSQL,
		cur.Time, cur.ID, time.Now().UTC().Add(-scavengerSafetyLag), params.BatchSize,
	).Scan(&drafts).Error; err != nil {
		return ReplicateBatchResult{}, err
	}
	if len(drafts) == 0 {
		return ReplicateBatchResult{Drained: true}, nil
	}

	draftIDs := make([]string, len(drafts))
	archiveDrafts := make([]models.ArchiveSleeperDraft, len(drafts))
	for i, d := range drafts {
		draftIDs[i] = d.SleeperDraftID
		archiveDrafts[i] = models.ArchiveSleeperDraft{
			SleeperDraftID: d.SleeperDraftID, SleeperLeagueID: d.SleeperLeagueID, Type: d.Type,
			Status: d.Status, Season: d.Season, LastFetchedAt: d.LastFetchedAt,
			CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
		}
	}

	var picks []models.SleeperDraftPick
	if err := a.Cloud.WithContext(ctx).Where("sleeper_draft_id IN ?", draftIDs).Find(&picks).Error; err != nil {
		return ReplicateBatchResult{}, err
	}
	archivePicks := make([]models.ArchiveSleeperDraftPick, len(picks))
	for i, p := range picks {
		archivePicks[i] = models.ArchiveSleeperDraftPick{
			SleeperDraftID: p.SleeperDraftID, Round: p.Round, PickNo: p.PickNo, RosterID: p.RosterID,
			PickedByUserID: p.PickedByUserID, SleeperPlayerID: p.SleeperPlayerID, Metadata: p.Metadata,
		}
	}

	last := drafts[len(drafts)-1]
	newCursor := cursor{Time: *last.LastFetchedAt, ID: last.SleeperDraftID}

	err = a.Archive.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "sleeper_draft_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"sleeper_league_id", "type", "status", "season", "last_fetched_at", "updated_at",
			}),
		}).CreateInBatches(archiveDrafts, 500).Error; err != nil {
			return err
		}
		if len(archivePicks) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
				CreateInBatches(archivePicks, 500).Error; err != nil {
				return err
			}
		}
		return writeCursor(tx, streamDraftPicks, newCursor)
	})
	if err != nil {
		return ReplicateBatchResult{}, err
	}
	return ReplicateBatchResult{Replicated: len(drafts), Drained: len(drafts) < params.BatchSize}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/activities/... -v`
Expected: the full `internal/activities` package PASSes (all scavenger tests plus everything pre-existing).

- [ ] **Step 5: Commit**

```bash
git add internal/activities/scavenger.go internal/activities/scavenger_test.go
git commit -m "feat: add ReplicateDraftPicksBatch scavenger activity"
```

---

### Task 7: ScavengerDispatcher workflow

**Files:**
- Create: `backend/internal/workflows/scavenger.go`
- Modify: `backend/internal/workflows/helpers.go`
- Modify: `backend/internal/workflows/workflows_test.go`

**Interfaces:**
- Consumes: `activities.ScavengerActivities`, `GetScavengerConfig`, the four `Replicate*Batch` activities (Tasks 2–6).
- Produces: `workflows.ScavengerDispatcher(ctx workflow.Context) (activities.ScavengerReport, error)`, `workflows.TaskQueueArchive`. Consumed by Task 8.

- [ ] **Step 1: Add the task queue constant**

In `backend/internal/workflows/helpers.go`, add to the `const` block:

```go
	TaskQueueArchive      = "archive-maintenance"
```

(alongside the existing `TaskQueueDiscovery`, `TaskQueueDrafts`, etc.)

- [ ] **Step 2: Write the failing workflow tests**

Append to `workflows_test.go`:

```go
// ---- ScavengerDispatcher ----

func TestScavengerDispatcher_DrainsAllStreamsUntilShortBatch(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	cfg := activities.ScavengerConfig{LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)

	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, activities.ReplicateBatchParams{BatchSize: 500}).
		Return(activities.ReplicateBatchResult{Replicated: 3, Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateTransactionsBatch, mock.Anything, activities.ReplicateBatchParams{BatchSize: 5000}).
		Return(activities.ReplicateBatchResult{Replicated: 10, Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftHeadersBatch, mock.Anything, activities.ReplicateBatchParams{BatchSize: 200}).
		Return(activities.ReplicateBatchResult{Replicated: 2, Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftPicksBatch, mock.Anything, activities.ReplicateBatchParams{BatchSize: 200}).
		Return(activities.ReplicateBatchResult{Replicated: 1, Drained: true}, nil).Once()

	env.ExecuteWorkflow(workflows.ScavengerDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report activities.ScavengerReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, activities.ScavengerReport{
		LeaguesReplicated: 3, TransactionsReplicated: 10, DraftHeadersReplicated: 2, DraftPicksReplicated: 1,
	}, report)
	env.AssertExpectations(t)
}

func TestScavengerDispatcher_StreamFailureDoesNotBlockOtherStreams(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	cfg := activities.ScavengerConfig{LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)

	// Leagues fails outright; the other three streams must still run.
	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, activities.ReplicateBatchParams{BatchSize: 500}).
		Return(activities.ReplicateBatchResult{}, temporal.NewNonRetryableApplicationError("boom", "test", nil)).Once()
	env.OnActivity(sa.ReplicateTransactionsBatch, mock.Anything, activities.ReplicateBatchParams{BatchSize: 5000}).
		Return(activities.ReplicateBatchResult{Replicated: 5, Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftHeadersBatch, mock.Anything, activities.ReplicateBatchParams{BatchSize: 200}).
		Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftPicksBatch, mock.Anything, activities.ReplicateBatchParams{BatchSize: 200}).
		Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()

	env.ExecuteWorkflow(workflows.ScavengerDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError()) // stream failures are logged and swallowed
	var report activities.ScavengerReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, 0, report.LeaguesReplicated)
	require.Equal(t, 5, report.TransactionsReplicated)
	env.AssertExpectations(t)
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd backend && go vet ./internal/workflows/...`
Expected: FAIL — `workflows.ScavengerDispatcher` undefined.

- [ ] **Step 4: Implement the workflow**

```go
// backend/internal/workflows/scavenger.go
package workflows

import (
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// ScavengerDispatcher replicates cloud → archive across four streams, in
// order: leagues, transactions, draft headers, draft picks. Each stream
// drains independently up to MaxBatchesPerRun batches or until a short
// batch signals it's caught up; a stream's activity failure is logged and
// stops only that stream for this run — the cursor didn't move (advance
// commits atomically with the copied rows), so the next 6h run resumes from
// the same position. Runs on the archive-maintenance queue, which only
// exists when ARCHIVE_DATABASE_URL is set — see cmd/worker/main.go.
func ScavengerDispatcher(ctx workflow.Context) (activities.ScavengerReport, error) {
	sa := &activities.ScavengerActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	batchCtx := workflow.WithActivityOptions(ctx, batchActivityOptions)
	logger := workflow.GetLogger(ctx)

	var cfg activities.ScavengerConfig
	if err := workflow.ExecuteActivity(actCtx, sa.GetScavengerConfig).Get(ctx, &cfg); err != nil {
		return activities.ScavengerReport{}, err
	}

	var report activities.ScavengerReport

	for i := 0; i < cfg.MaxBatchesPerRun; i++ {
		var res activities.ReplicateBatchResult
		if err := workflow.ExecuteActivity(batchCtx, sa.ReplicateLeaguesBatch, activities.ReplicateBatchParams{BatchSize: cfg.LeagueBatchSize}).Get(ctx, &res); err != nil {
			logger.Error("replicate leagues batch failed; stopping leagues for this run", "error", err)
			break
		}
		report.LeaguesReplicated += res.Replicated
		if res.Drained {
			break
		}
	}

	for i := 0; i < cfg.MaxBatchesPerRun; i++ {
		var res activities.ReplicateBatchResult
		if err := workflow.ExecuteActivity(batchCtx, sa.ReplicateTransactionsBatch, activities.ReplicateBatchParams{BatchSize: cfg.TxnBatchSize}).Get(ctx, &res); err != nil {
			logger.Error("replicate transactions batch failed; stopping transactions for this run", "error", err)
			break
		}
		report.TransactionsReplicated += res.Replicated
		if res.Drained {
			break
		}
	}

	for i := 0; i < cfg.MaxBatchesPerRun; i++ {
		var res activities.ReplicateBatchResult
		if err := workflow.ExecuteActivity(batchCtx, sa.ReplicateDraftHeadersBatch, activities.ReplicateBatchParams{BatchSize: cfg.DraftBatchSize}).Get(ctx, &res); err != nil {
			logger.Error("replicate draft headers batch failed; stopping draft headers for this run", "error", err)
			break
		}
		report.DraftHeadersReplicated += res.Replicated
		if res.Drained {
			break
		}
	}

	for i := 0; i < cfg.MaxBatchesPerRun; i++ {
		var res activities.ReplicateBatchResult
		if err := workflow.ExecuteActivity(batchCtx, sa.ReplicateDraftPicksBatch, activities.ReplicateBatchParams{BatchSize: cfg.DraftBatchSize}).Get(ctx, &res); err != nil {
			logger.Error("replicate draft picks batch failed; stopping draft picks for this run", "error", err)
			break
		}
		report.DraftPicksReplicated += res.Replicated
		if res.Drained {
			break
		}
	}

	logger.Info("scavenger run complete", "leagues", report.LeaguesReplicated, "transactions", report.TransactionsReplicated,
		"draftHeaders", report.DraftHeadersReplicated, "draftPicks", report.DraftPicksReplicated)
	return report, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd backend && go test ./internal/workflows/... -run TestScavengerDispatcher -v`
Expected: both PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/workflows/scavenger.go internal/workflows/helpers.go internal/workflows/workflows_test.go
git commit -m "feat: add ScavengerDispatcher workflow"
```

---

### Task 8: Wire into cmd/worker and schedules

**Files:**
- Modify: `backend/schedules/register.go`
- Modify: `backend/cmd/worker/main.go`

**Interfaces:**
- Consumes: `workflows.ScavengerDispatcher`, `workflows.TaskQueueArchive` (Task 7), `activities.ScavengerActivities` (Task 2), `database.Archive`/`cfg.ArchiveDB.Enabled()` (existing, from T3/T4).

No new automated test — `cmd/worker` and `schedules` are untested today (confirmed baseline: `schedules` has `[no test files]`, `cmd/worker` is a `main` package). Verification is manual (Step 4), consistent with how the T3/T4 plan handled this same pair of files.

- [ ] **Step 1: Change `schedules.Register`'s signature**

In `backend/schedules/register.go`, change:

```go
func Register(ctx context.Context, c client.Client) error {
```

to:

```go
// Register creates the Temporal schedules for the Sleeper workers. If a
// schedule already exists it is left unchanged (idempotent). archiveEnabled
// gates the scavenger schedule — registering it when no worker polls
// archive-maintenance would just be a schedule that fires and returns a
// "no worker available" fail, forever, on a queue nobody's listening to.
func Register(ctx context.Context, c client.Client, archiveEnabled bool) error {
```

Then, replace the final `return upsert(ctx, c, client.ScheduleOptions{ ... "sleeper-adp-rollup-schedule" ... })` statement (currently the function's `return`) so the ADP schedule keeps its own `if err := ...; err != nil { return err }` guard, and add the scavenger schedule after it:

```go
	if err := upsert(ctx, c, client.ScheduleOptions{
		ID: "sleeper-adp-rollup-schedule",
		Spec: client.ScheduleSpec{
			Calendars: []client.ScheduleCalendarSpec{
				{
					Hour:   []client.ScheduleRange{{Start: 11}}, // 06:00 EST (UTC-5)
					Minute: []client.ScheduleRange{{Start: 0}},
				},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			Workflow:                 workflows.ADPRollupDispatcher,
			TaskQueue:                workflows.TaskQueueADP,
			WorkflowExecutionTimeout: 30 * time.Minute,
		},
	}); err != nil {
		return err
	}

	if !archiveEnabled {
		return nil
	}
	return upsert(ctx, c, client.ScheduleOptions{
		ID: "sleeper-scavenger-schedule",
		Spec: client.ScheduleSpec{
			Intervals: []client.ScheduleIntervalSpec{
				{Every: 6 * time.Hour},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			Workflow:  workflows.ScavengerDispatcher,
			TaskQueue: workflows.TaskQueueArchive,
		},
	})
}
```

- [ ] **Step 2: Update the call site and register the archive worker in `cmd/worker/main.go`**

Change the `schedules.Register` call:

```go
	if err := schedules.Register(context.Background(), c); err != nil {
		log.Fatalf("register schedules: %v", err)
	}
```

to:

```go
	if err := schedules.Register(context.Background(), c, cfg.ArchiveDB.Enabled()); err != nil {
		log.Fatalf("register schedules: %v", err)
	}
```

Immediately after the existing `workers := []worker.Worker{dw, draftsw, transactionsw, psw, wsw, adpw}` line (still valid Go — `append` works fine on a slice created via a literal), insert:

```go
	workers := []worker.Worker{dw, draftsw, transactionsw, psw, wsw, adpw}
	if cfg.ArchiveDB.Enabled() {
		sa := &activities.ScavengerActivities{Cloud: database.DB, Archive: database.Archive}
		aw := worker.New(c, workflows.TaskQueueArchive, worker.Options{
			DeploymentOptions: deploymentOpts,
			SysInfoProvider:   sysinfo.SysInfoProvider(),
		})
		aw.RegisterWorkflow(workflows.ScavengerDispatcher)
		aw.RegisterActivity(sa)
		workers = append(workers, aw)
	}
```

The following `for _, w := range workers { if err := w.Start(); ... }` loop and its deferred `Stop()` loop are unchanged — they already iterate whatever `workers` ends up holding.

- [ ] **Step 3: Build**

Run: `cd backend && go build ./...`
Expected: succeeds with no errors.

- [ ] **Step 4: Manual verification against a disposable two-database Postgres**

```bash
# Reuse the same disposable cluster from earlier work (or start one per the
# T3/T4 plan's Task 3 Step 4). Recreate the two throwaway databases:
psql "postgres://$(whoami)@localhost:5499/postgres?sslmode=disable" -c "DROP DATABASE IF EXISTS ffsims_cloud" -c "DROP DATABASE IF EXISTS ffsims_archive"
psql "postgres://$(whoami)@localhost:5499/postgres?sslmode=disable" -c "CREATE DATABASE ffsims_cloud" -c "CREATE DATABASE ffsims_archive"

cd backend
go build -o bin/migrate ./cmd/migrate
DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_cloud?sslmode=disable" ./bin/migrate up
# (archive migrations auto-run at worker startup — no separate migrate step needed for archive)

go build -o bin/backend-worker ./cmd/worker
DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_cloud?sslmode=disable" \
  ARCHIVE_DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_archive?sslmode=disable" \
  timeout 5 ./bin/backend-worker 2>&1 | head -20
```
Expected in the log: `goose: successfully migrated database to version: 5` (archive migrations 001–005), `Connected to archive database (...)`, then the usual `temporal dial: ... connection refused` (no local Temporal server — harmless, this step only verifies the archive wiring didn't blow up).

```bash
# Also verify the archive-disabled path is unaffected:
DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_cloud?sslmode=disable" timeout 5 ./bin/backend-worker 2>&1 | head -10
```
Expected: `ARCHIVE_DATABASE_URL not set — archive database disabled`, no archive-related log lines, same harmless Temporal dial failure at the end.

Clean up:
```bash
rm -f bin/migrate bin/backend-worker
psql "postgres://$(whoami)@localhost:5499/postgres?sslmode=disable" -c "DROP DATABASE IF EXISTS ffsims_cloud" -c "DROP DATABASE IF EXISTS ffsims_archive"
```

- [ ] **Step 5: Run the full backend test suite**

Run: `cd backend && go test ./... -v 2>&1 | tail -100`
Expected: everything PASSes (Postgres-gated tests PASS with `TEST_DATABASE_URL` set, SKIP otherwise — no FAILs either way).

- [ ] **Step 6: Commit**

```bash
git add schedules/register.go cmd/worker/main.go
git commit -m "feat: register archive-maintenance worker + 6h scavenger schedule"
```

---

## Verification

- [ ] `cd backend && go build ./...` and `go vet ./...` clean.
- [ ] `cd backend && go test ./...` passes with `TEST_DATABASE_URL` unset (PG-gated tests SKIP, nothing FAILs).
- [ ] Full pass with a disposable Postgres: `TEST_DATABASE_URL="postgres://$(whoami)@localhost:5499/postgres?sslmode=disable" go test ./... -v` — every test PASSes, including all new `internal/activities/scavenger_test.go` and `internal/workflows` scavenger tests.
- [ ] Task 8 Step 4's manual two-database boot check: archive migrations 001–005 auto-apply, archive connects, archive-disabled path is unaffected.
- [ ] Spot-check idempotence end-to-end: with real data (or the manual test DBs), run `ScavengerDispatcher` twice via `temporal workflow start --type ScavengerDispatcher --task-queue archive-maintenance` (or wait for the 6h schedule) — the second run's `ScavengerReport` should show all-zero counts.

## Self-Review

**Spec coverage:** T5's three named deliverables — "replicate phase," "6h schedule," "archive worker" — map to Tasks 1–6 (replicate), Task 8 (schedule), and Task 8 (worker registration) respectively. Explicitly out of scope per Global Constraints: purge (T6), retention/purge-enabled config (T6), the claim-pool-exclusion predicate (T6), ADP rollup reading from archive (T7).

**Placeholder scan:** no TBD/TODO markers; every step has literal code.

**Type consistency:** `ScavengerActivities{Cloud, Archive}` matches across Tasks 2–8. `ReplicateBatchParams{BatchSize}` / `ReplicateBatchResult{Replicated, Drained}` match across Tasks 2–7 (shared by all four replicate activities and the workflow). `ScavengerConfig` field names (`LeagueBatchSize`, `TxnBatchSize`, `DraftBatchSize`, `MaxBatchesPerRun`) match between Task 2's definition, its `GetScavengerConfig` implementation, and Task 7's workflow usage. `ScavengerReport` field names match between Task 2's definition, Task 7's workflow accumulation, and the workflow tests' assertions. `models.Archive*` type/field names match between Task 1's definitions and Tasks 3–6's usage.
