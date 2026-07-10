# Archive DB Handle + Cursor Indexes (T3+T4) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the second (archive) database handle and its migration plumbing (T3), and add the cursor indexes the future scavenger needs on the cloud DB (T4) — both from `docs/superpowers/plans/2026-07-07-two-database-archive.md`, shipped as one PR since neither has runtime behavior on its own and both are cheap, low-risk, mergeable-before-T1 groundwork.

**Architecture:** `internal/config.Config` gains an `ArchiveDBConfig` (empty `ConnectionString` = disabled). `internal/database` gains a second global (`database.Archive`) and `InitializeArchive`, mirroring the existing `DB`/`Initialize`. A new `backend/migrations/archive/` goose directory holds archive-only migrations (starting with a generic `archive_sync_state` cursor table), embedded and run through a new small shared runner package (`internal/dbmigrate`) used by both `cmd/migrate` (manual ops, now with a `-db cloud|archive` flag) and `cmd/worker` (auto-runs archive migrations at startup, gated on `ARCHIVE_DATABASE_URL` being set). Separately, cloud migration 021 adds the two indexes (`sleeper_transactions.created_at`, `sleeper_drafts.last_fetched_at`) called out in the design doc as currently missing and needed by the future scavenger's cursor queries.

**Tech Stack:** Go, GORM (`gorm.io/gorm`, `gorm.io/driver/postgres`), goose v3 (`github.com/pressly/goose/v3`) migrations over `database/sql` + pgx stdlib driver, PostgreSQL 16.

## Global Constraints

- Module path is `backend` (`backend/go.mod`) — all internal imports use `backend/...`.
- Goose is the migration tool (not golang-migrate). Cloud migrations live in `backend/migrations/*.sql`, embedded via `backend/migrations/fs.go`. Numbering is sequential zero-padded 3-digit; **021 is the next free cloud number**.
- `CREATE INDEX CONCURRENTLY` / `DROP INDEX CONCURRENTLY` migrations must repeat `-- +goose NO TRANSACTION` under **both** `-- +goose Up` and `-- +goose Down` (the newer 019/020 convention — copy that, not 012's single-annotation form).
- PG-only tests gate on `TEST_DATABASE_URL` (skip when unset) and already run in CI against a `postgres:16` service (`.github/workflows/ci.yml`). Locally there is no Docker daemon on this Mac — use `initdb`/`pg_ctl` (`/usr/local/bin`) to run a throwaway cluster on port 5499 with `-c unix_socket_directories=''` (the scratchpad path is too long for the default Unix socket). This plan's implementation work itself happens in the git worktree at `/Users/seankane/github.com/ff-sims/.claude/worktrees/archive-db-handle-and-indexes`; the disposable Postgres cluster can live anywhere (e.g. the scratchpad), it does not need to be inside the worktree.
- `database.Archive`/`InitializeArchive` are **plumbing only** in this PR — nothing reads or writes through them yet (that starts in T5). Don't add a Temporal `archive-maintenance` task queue or worker registration yet: an idle queue with no workflows/activities registered provides no test value and belongs with the code that first uses it (T5).
- Don't touch `ADPRollupActivities` (splitting it into `{Read, Write}` is T7's job, not T3's).
- Only the worker process initializes the archive handle — `cmd/server` is untouched.

---

## File Structure

| File | Responsibility |
|---|---|
| `backend/internal/testutil/pgschema.go` (new) | Shared throwaway-Postgres-schema helper for PG integration tests, extracted from `claim_pg_test.go`'s `newPGTestDB` |
| `backend/internal/activities/claim_pg_test.go` (modify) | Use `testutil` instead of its private schema helper |
| `backend/internal/config/config.go` (modify) | Add `ArchiveDBConfig` + `Config.ArchiveDB` + `Enabled()` |
| `backend/internal/config/config_test.go` (new) | Unit tests for `ArchiveDBConfig.Enabled()` and env parsing |
| `backend/internal/database/postgres.go` (modify) | Add `Archive` global + `InitializeArchive` |
| `backend/internal/database/postgres_test.go` (new) | PG integration test for `InitializeArchive` |
| `backend/internal/dbmigrate/dbmigrate.go` (new) | `Run(dsn string, fsys fs.FS, command string, args []string) error` — shared goose runner |
| `backend/internal/dbmigrate/dbmigrate_test.go` (new) | PG integration tests: archive migrations create `archive_sync_state`; cloud migrations (through 021) apply cleanly and create the new indexes |
| `backend/migrations/archive/fs.go` (new) | `//go:embed *.sql` for archive migrations, package `archivemigrations` |
| `backend/migrations/archive/001_archive_sync_state.sql` (new) | First archive migration: generic cursor-state table |
| `backend/migrations/021_scavenger_cursor_indexes.sql` (new) | T4: indexes on `sleeper_transactions(created_at)`, `sleeper_drafts(last_fetched_at)` |
| `backend/cmd/migrate/main.go` (modify) | Add `-db cloud|archive` flag; delegate to `dbmigrate.Run` |
| `backend/cmd/worker/main.go` (modify) | When `cfg.ArchiveDB.Enabled()`: auto-run archive migrations, then `database.InitializeArchive` |
| `backend/Makefile` (modify) | Add `migrate-archive`, `migrate-archive-status`, `migrate-archive-down` targets |

---

### Task 1: Extract shared PG throwaway-schema test helper

**Files:**
- Create: `backend/internal/testutil/pgschema.go`
- Modify: `backend/internal/activities/claim_pg_test.go`

**Interfaces:**
- Produces: `testutil.NewPGSchema(t *testing.T, dsn, prefix string) string` — creates a throwaway schema on `dsn`, returns a DSN with `search_path` pinned to it, drops the schema via `t.Cleanup`.
- Produces: `testutil.OpenGORM(t *testing.T, scopedDSN string) *gorm.DB` — opens a silent-logger GORM connection against `scopedDSN`, closes it via `t.Cleanup`.

This is a refactor: existing tests in `claim_pg_test.go` are the safety net. No new test is written in this task; Step 2 below re-runs the existing suite to prove behavior didn't change.

- [ ] **Step 1: Create the testutil package**

```go
// backend/internal/testutil/pgschema.go
package testutil

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewPGSchema creates a throwaway schema (named "<prefix>_<random>") on the
// Postgres database at dsn and returns a DSN with search_path pinned to it.
// search_path rides in the DSN itself (not a session SET) so every pooled
// connection — including concurrent goroutines sharing one *gorm.DB — sees
// the same schema. The schema and its contents are dropped via t.Cleanup.
func NewPGSchema(t *testing.T, dsn, prefix string) string {
	t.Helper()
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	schema := fmt.Sprintf("%s_%d", prefix, rand.Int63())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		sqlDB, _ := admin.DB()
		sqlDB.Close()
	})

	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

// OpenGORM opens a *gorm.DB against scopedDSN (as returned by NewPGSchema)
// with query logging silenced, closing it via t.Cleanup.
func OpenGORM(t *testing.T, scopedDSN string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(scopedDSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open postgres (schema-scoped): %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	})
	return db
}
```

- [ ] **Step 2: Rewrite `newPGTestDB` in claim_pg_test.go to use testutil**

Replace lines 1–63 of `backend/internal/activities/claim_pg_test.go` (package declaration through the end of `newPGTestDB`) with:

```go
package activities_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/testutil"
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
	scopedDSN := testutil.NewPGSchema(t, dsn, "claim_test")
	db := testutil.OpenGORM(t, scopedDSN)
	if err := db.AutoMigrate(&models.SleeperLeague{}, &models.SleeperUser{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}
```

The rest of the file (from `func seedLeague` onward, line 65 to the end) is unchanged — leave it exactly as is. Note the trimmed import list: `math/rand`, `strings`, `gorm.io/driver/postgres`, and `gorm.io/gorm/logger` are no longer used directly in this file and must be dropped, or `go build` will fail on unused imports.

- [ ] **Step 3: Verify build and existing tests still pass**

Run: `cd backend && go build ./... && go test ./internal/activities/... ./internal/testutil/... -v`
Expected: build succeeds; `claim_pg_test.go` tests report `SKIP` (no `TEST_DATABASE_URL` set yet) rather than compile errors or failures. `internal/testutil` has no test files yet, so it reports `[no test files]` — that's expected, it's exercised indirectly.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/testutil/pgschema.go backend/internal/activities/claim_pg_test.go
git commit -m "test: extract shared PG throwaway-schema helper into internal/testutil"
```

---

### Task 2: Archive DB config

**Files:**
- Modify: `backend/internal/config/config.go`
- Create: `backend/internal/config/config_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `config.ArchiveDBConfig{ConnectionString, PoolMaxOpenConns, PoolMaxIdleConns, PoolConnMaxLifetime}` with method `Enabled() bool`; `Config.ArchiveDB ArchiveDBConfig` field, populated by `Load()` from `ARCHIVE_DATABASE_URL` (default `""`), `ARCHIVE_DB_MAX_OPEN_CONNS` (default 10), `ARCHIVE_DB_MAX_IDLE_CONNS` (default 5), `ARCHIVE_DB_CONN_MAX_LIFETIME_SECS` (default 300). Task 3 (`database.InitializeArchive`) consumes this.

- [ ] **Step 1: Write the failing test**

```go
// backend/internal/config/config_test.go
package config

import "testing"

func TestArchiveDBConfig_Enabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  ArchiveDBConfig
		want bool
	}{
		{"empty connection string is disabled", ArchiveDBConfig{}, false},
		{"non-empty connection string is enabled", ArchiveDBConfig{ConnectionString: "postgres://x"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Enabled(); got != tc.want {
				t.Errorf("Enabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLoad_ArchiveDBDefaultsToDisabled(t *testing.T) {
	t.Setenv("ARCHIVE_DATABASE_URL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ArchiveDB.Enabled() {
		t.Errorf("expected ArchiveDB disabled by default, got ConnectionString=%q", cfg.ArchiveDB.ConnectionString)
	}
}

func TestLoad_ArchiveDBReadsEnv(t *testing.T) {
	t.Setenv("ARCHIVE_DATABASE_URL", "postgres://archive-host/db")
	t.Setenv("ARCHIVE_DB_MAX_OPEN_CONNS", "7")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.ArchiveDB.Enabled() {
		t.Fatal("expected ArchiveDB enabled")
	}
	if cfg.ArchiveDB.ConnectionString != "postgres://archive-host/db" {
		t.Errorf("ConnectionString = %q", cfg.ArchiveDB.ConnectionString)
	}
	if cfg.ArchiveDB.PoolMaxOpenConns != 7 {
		t.Errorf("PoolMaxOpenConns = %d, want 7", cfg.ArchiveDB.PoolMaxOpenConns)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/config/... -v`
Expected: FAIL — `ArchiveDBConfig` and `Config.ArchiveDB` undefined (compile error).

- [ ] **Step 3: Implement**

In `backend/internal/config/config.go`, change the `Config` struct (lines 11–15) to:

```go
// Config contains all configuration for the application
type Config struct {
	Server    ServerConfig
	DB        DBConfig
	ArchiveDB ArchiveDBConfig
}
```

Add after the existing `DBConfig` struct (after line 30):

```go
// ArchiveDBConfig contains archive-database-specific configuration. An empty
// ConnectionString means the archive DB is disabled — local dev and any
// fleet without ARCHIVE_DATABASE_URL set keep working unchanged.
type ArchiveDBConfig struct {
	ConnectionString string
	// Pool limits prevent exhausting connection slots on managed-DB instances.
	PoolMaxOpenConns    int
	PoolMaxIdleConns    int
	PoolConnMaxLifetime int // seconds
}

// Enabled reports whether an archive database is configured.
func (c ArchiveDBConfig) Enabled() bool {
	return c.ConnectionString != ""
}
```

In `Load()` (lines 33–48), add the `ArchiveDB` field to the returned `cfg`:

```go
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: getEnvAsInt("SERVER_PORT", 8080),
			Env:  getEnv("ENV", "development"),
		},
		DB: DBConfig{
			ConnectionString:    getEnv("DATABASE_URL", "postgresql://postgres@localhost:5432/ffsims"),
			PoolMaxOpenConns:    getEnvAsInt("DB_MAX_OPEN_CONNS", 10),
			PoolMaxIdleConns:    getEnvAsInt("DB_MAX_IDLE_CONNS", 5),
			PoolConnMaxLifetime: getEnvAsInt("DB_CONN_MAX_LIFETIME_SECS", 300),
		},
		ArchiveDB: ArchiveDBConfig{
			ConnectionString:    getEnv("ARCHIVE_DATABASE_URL", ""),
			PoolMaxOpenConns:    getEnvAsInt("ARCHIVE_DB_MAX_OPEN_CONNS", 10),
			PoolMaxIdleConns:    getEnvAsInt("ARCHIVE_DB_MAX_IDLE_CONNS", 5),
			PoolConnMaxLifetime: getEnvAsInt("ARCHIVE_DB_CONN_MAX_LIFETIME_SECS", 300),
		},
	}

	return cfg, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/config/... -v`
Expected: PASS — `TestArchiveDBConfig_Enabled`, `TestLoad_ArchiveDBDefaultsToDisabled`, `TestLoad_ArchiveDBReadsEnv` all pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/config/config.go backend/internal/config/config_test.go
git commit -m "feat: add ArchiveDBConfig (ARCHIVE_DATABASE_URL, empty = disabled)"
```

---

### Task 3: Archive DB handle (`database.Archive` / `InitializeArchive`)

**Files:**
- Modify: `backend/internal/database/postgres.go`
- Create: `backend/internal/database/postgres_test.go`

**Interfaces:**
- Consumes: `config.ArchiveDBConfig` (Task 2).
- Produces: `database.Archive *gorm.DB` (package global, nil until initialized), `database.InitializeArchive(cfg *config.Config) error`. Task 6 (`cmd/worker`) consumes this.

- [ ] **Step 1: Write the failing tests**

```go
// backend/internal/database/postgres_test.go
package database_test

import (
	"os"
	"testing"

	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/testutil"
)

func TestInitializeArchive_ErrorsWhenDisabled(t *testing.T) {
	cfg := &config.Config{} // ArchiveDB.ConnectionString is empty
	if err := database.InitializeArchive(cfg); err == nil {
		t.Fatal("expected error when ARCHIVE_DATABASE_URL is not configured")
	}
}

func TestInitializeArchive_ConnectsWhenConfigured(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	scopedDSN := testutil.NewPGSchema(t, dsn, "archive_handle_test")

	cfg := &config.Config{ArchiveDB: config.ArchiveDBConfig{
		ConnectionString:    scopedDSN,
		PoolMaxOpenConns:    5,
		PoolMaxIdleConns:    2,
		PoolConnMaxLifetime: 60,
	}}

	if err := database.InitializeArchive(cfg); err != nil {
		t.Fatalf("InitializeArchive: %v", err)
	}
	if database.Archive == nil {
		t.Fatal("expected database.Archive to be set")
	}
	sqlDB, err := database.Archive.DB()
	if err != nil {
		t.Fatalf("get underlying sql.DB: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping archive db: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/database/... -v`
Expected: FAIL — `database.InitializeArchive` undefined (compile error).

- [ ] **Step 3: Implement**

In `backend/internal/database/postgres.go`, add after the existing `Initialize` function (after line 44):

```go
// Archive is the global archive-database instance. Nil unless
// InitializeArchive has been called — only the worker does so, and only when
// cfg.ArchiveDB.Enabled() (i.e. ARCHIVE_DATABASE_URL is set).
var Archive *gorm.DB

// InitializeArchive sets up the archive database connection and configures
// its connection pool. Mirrors Initialize but targets cfg.ArchiveDB. Callers
// must check cfg.ArchiveDB.Enabled() first — this returns an error rather
// than silently no-op-ing when ConnectionString is empty, so a misconfigured
// call site fails loudly instead of leaving Archive nil.
func InitializeArchive(cfg *config.Config) error {
	if !cfg.ArchiveDB.Enabled() {
		return fmt.Errorf("archive database not configured (ARCHIVE_DATABASE_URL is empty)")
	}
	var err error
	slog.Debug("Initializing archive database connection", "connectionString", cfg.ArchiveDB.ConnectionString)
	Archive, err = gorm.Open(postgres.Open(cfg.ArchiveDB.ConnectionString), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to archive database: %w", err)
	}

	sqlDB, err := Archive.DB()
	if err != nil {
		return fmt.Errorf("get underlying archive sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.ArchiveDB.PoolMaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.ArchiveDB.PoolMaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ArchiveDB.PoolConnMaxLifetime) * time.Second)

	log.Printf("Connected to archive database (maxOpen=%d, maxIdle=%d, connLifetime=%ds)",
		cfg.ArchiveDB.PoolMaxOpenConns, cfg.ArchiveDB.PoolMaxIdleConns, cfg.ArchiveDB.PoolConnMaxLifetime)
	return nil
}
```

No new imports are needed — `fmt`, `log`, `log/slog`, `time`, `config`, `postgres`, `gorm`, `logger` are all already imported by this file.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/database/... -v`
Expected: `TestInitializeArchive_ErrorsWhenDisabled` PASSes unconditionally. `TestInitializeArchive_ConnectsWhenConfigured` PASSes if `TEST_DATABASE_URL` is set, else SKIPs.

To exercise the skipped path for real, start a disposable Postgres (no Docker daemon on this machine):
```bash
initdb -D /tmp/ff-sims-pgtest 2>&1 | tail -5   # or reuse an existing cluster dir
pg_ctl -D /tmp/ff-sims-pgtest -o "-c unix_socket_directories='' -p 5499" -l /tmp/ff-sims-pgtest.log start
TEST_DATABASE_URL="postgres://$(whoami)@localhost:5499/postgres?sslmode=disable" go test ./internal/database/... -v
pg_ctl -D /tmp/ff-sims-pgtest stop
```
Expected: both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/database/postgres.go backend/internal/database/postgres_test.go
git commit -m "feat: add database.Archive / InitializeArchive second DB handle"
```

---

### Task 4: Archive migrations directory + first migration

**Files:**
- Create: `backend/migrations/archive/fs.go`
- Create: `backend/migrations/archive/001_archive_sync_state.sql`

**Interfaces:**
- Produces: `archivemigrations.FS embed.FS` (package `archivemigrations`) — Task 5 (`dbmigrate`) and Task 6 (`cmd/migrate`, `cmd/worker`) consume this.
- No Go code to unit test here — Task 5's `dbmigrate_test.go` proves this migration applies cleanly and creates the table.

- [ ] **Step 1: Create the embed file**

```go
// backend/migrations/archive/fs.go
package archivemigrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 2: Create the first archive migration**

Archive replica tables have no FKs (per the design doc, arrival order must not matter), so this table needs no foreign keys either. `cursor_state` is a flexible jsonb blob rather than fixed watermark columns — the exact cursor shape per stream (transactions: `created_at`+id; drafts: two independent watermarks; leagues: `updated_at`) is the scavenger's concern (T5), not this plumbing task's. This is a brand-new, empty table — no existing rows to worry about, so a plain (transactional) `CREATE TABLE`/`CREATE INDEX` is fine; `CONCURRENTLY` is only needed for indexing large, already-populated tables like migration 021's targets (Task 8).

```sql
-- backend/migrations/archive/001_archive_sync_state.sql
-- +goose Up

CREATE TABLE archive_sync_state (
    stream text PRIMARY KEY,
    cursor_state jsonb NOT NULL DEFAULT '{}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down

DROP TABLE archive_sync_state;
```

- [ ] **Step 3: Verify it compiles into the build**

Run: `cd backend && go build ./...`
Expected: succeeds (the `//go:embed *.sql` directive requires at least one matching file at compile time — confirms the migration file is picked up).

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/archive/
git commit -m "feat: add archive migrations dir with archive_sync_state table"
```

---

### Task 5: Shared goose runner (`internal/dbmigrate`)

**Files:**
- Create: `backend/internal/dbmigrate/dbmigrate.go`
- Create: `backend/internal/dbmigrate/dbmigrate_test.go`

**Interfaces:**
- Consumes: `archivemigrations.FS` (Task 4), `migrations.FS` (existing cloud migrations).
- Produces: `dbmigrate.Run(dsn string, fsys fs.FS, command string, args []string) error`. Task 6 (`cmd/migrate`, `cmd/worker`) consumes this.

- [ ] **Step 1: Write the failing tests**

```go
// backend/internal/dbmigrate/dbmigrate_test.go
package dbmigrate_test

import (
	"os"
	"testing"

	"backend/internal/dbmigrate"
	"backend/internal/testutil"
	"backend/migrations"
	archivemigrations "backend/migrations/archive"
)

func tableExists(t *testing.T, scopedDSN, table string) bool {
	t.Helper()
	db := testutil.OpenGORM(t, scopedDSN)
	var exists bool
	if err := db.Raw(
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = ?)", table,
	).Scan(&exists).Error; err != nil {
		t.Fatalf("check table %s exists: %v", table, err)
	}
	return exists
}

func indexExists(t *testing.T, scopedDSN, index string) bool {
	t.Helper()
	db := testutil.OpenGORM(t, scopedDSN)
	var exists bool
	if err := db.Raw(
		"SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = ?)", index,
	).Scan(&exists).Error; err != nil {
		t.Fatalf("check index %s exists: %v", index, err)
	}
	return exists
}

func TestRun_ArchiveMigrations_CreatesSyncStateTable(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	scopedDSN := testutil.NewPGSchema(t, dsn, "archive_migrate_test")

	if err := dbmigrate.Run(scopedDSN, archivemigrations.FS, "up", nil); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if !tableExists(t, scopedDSN, "archive_sync_state") {
		t.Error("expected archive_sync_state table to exist after migrate up")
	}

	// Idempotent: re-running up against an up-to-date schema is a no-op, not
	// an error — this is what makes it safe for cmd/worker to call on every
	// startup.
	if err := dbmigrate.Run(scopedDSN, archivemigrations.FS, "up", nil); err != nil {
		t.Fatalf("migrate up (second run): %v", err)
	}
}

func TestRun_CloudMigrations_ApplyCleanlyAndCreateScavengerIndexes(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	scopedDSN := testutil.NewPGSchema(t, dsn, "cloud_migrate_test")

	if err := dbmigrate.Run(scopedDSN, migrations.FS, "up", nil); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	for _, idx := range []string{"idx_sleeper_transactions_created_at", "idx_sleeper_drafts_last_fetched_at"} {
		if !indexExists(t, scopedDSN, idx) {
			t.Errorf("expected index %s to exist after migrate up", idx)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/dbmigrate/... -v`
Expected: FAIL — `backend/internal/dbmigrate` package doesn't exist yet (compile error). (These tests also depend on migration `021_scavenger_cursor_indexes.sql` from Task 8, which doesn't exist yet either — that's fine, `TestRun_CloudMigrations_ApplyCleanlyAndCreateScavengerIndexes` will keep failing the index-existence assertions until Task 8 lands; note this now so it isn't mistaken for a regression later.)

- [ ] **Step 3: Implement**

```go
// backend/internal/dbmigrate/dbmigrate.go
package dbmigrate

import (
	"database/sql"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Run applies goose command (e.g. "up", "down", "status") from fsys against
// the database at dsn. Shared by cmd/migrate (manual ops, either DB) and
// cmd/worker (auto-runs archive migrations at startup).
//
// goose.SetBaseFS sets a package-level global in the goose library, so Run
// must not be called concurrently with a different fsys from multiple
// goroutines in the same process. Every current caller is single-shot
// (cmd/migrate) or runs once at startup before serving traffic (cmd/worker),
// so this is safe today; it would need revisiting if that changes.
func Run(dsn string, fsys fs.FS, command string, args []string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(fsys)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}
	if err := goose.Run(command, db, ".", args...); err != nil {
		return fmt.Errorf("goose %s: %w", command, err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/dbmigrate/... -v`
Expected: with `TEST_DATABASE_URL` unset, both SKIP. With it set (see Task 3 Step 4 for how to start a disposable Postgres), `TestRun_ArchiveMigrations_CreatesSyncStateTable` PASSes; `TestRun_CloudMigrations_ApplyCleanlyAndCreateScavengerIndexes` still FAILs until Task 8 adds migration 021 — that's expected at this point in the plan, re-run it after Task 8.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/dbmigrate/
git commit -m "feat: add internal/dbmigrate shared goose runner"
```

---

### Task 6: Wire `-db` flag into `cmd/migrate`, auto-migrate into `cmd/worker`

**Files:**
- Modify: `backend/cmd/migrate/main.go`
- Modify: `backend/cmd/worker/main.go`

**Interfaces:**
- Consumes: `dbmigrate.Run` (Task 5), `archivemigrations.FS` (Task 4), `config.ArchiveDBConfig.Enabled()` (Task 2), `database.InitializeArchive` (Task 3).
- Produces: nothing new consumed by later tasks — this is the integration point where Tasks 2–5 come together into the two entrypoints.

This task has no new automated test — `cmd/migrate` and `cmd/worker` are `main` packages with `[no test files]` today (confirmed baseline), and the behavior they wire together is already covered by Tasks 2, 3, and 5's tests. Verification here is manual (Step 4).

- [ ] **Step 1: Rewrite `cmd/migrate/main.go`**

```go
// backend/cmd/migrate/main.go
package main

import (
	"flag"
	"io/fs"
	"log"
	"os"

	_ "github.com/joho/godotenv/autoload"

	"backend/internal/dbmigrate"
	"backend/migrations"
	archivemigrations "backend/migrations/archive"
)

func main() {
	dbFlag := flag.String("db", "cloud", "which database to migrate: cloud or archive")
	flag.Parse()

	var dsn string
	var fsys fs.FS
	switch *dbFlag {
	case "cloud":
		dsn = os.Getenv("DATABASE_URL")
		if dsn == "" {
			log.Fatal("DATABASE_URL is not set")
		}
		fsys = migrations.FS
	case "archive":
		dsn = os.Getenv("ARCHIVE_DATABASE_URL")
		if dsn == "" {
			log.Fatal("ARCHIVE_DATABASE_URL is not set")
		}
		fsys = archivemigrations.FS
	default:
		log.Fatalf("unknown -db value %q (want cloud or archive)", *dbFlag)
	}

	command := "up"
	args := []string{}
	if flag.NArg() > 0 {
		command = flag.Arg(0)
		args = flag.Args()[1:]
	}

	if err := dbmigrate.Run(dsn, fsys, command, args); err != nil {
		log.Fatal(err)
	}
}
```

Note this preserves backward compatibility: `flag.Parse()` stops parsing at the first non-flag argument, so the existing invocation `./bin/migrate up` still works unchanged (no `-db` present → defaults to `"cloud"`; `flag.Arg(0)` is `"up"`). New: `./bin/migrate -db=archive up`.

- [ ] **Step 2: Wire archive auto-migrate + handle init into `cmd/worker/main.go`**

Add to the import block (after `"backend/internal/database"` on line 19):

```go
	"backend/internal/dbmigrate"
```

and after the existing `"backend/schedules"` import, add:

```go
	archivemigrations "backend/migrations/archive"
```

Replace lines 60–62 (the `database.Initialize` block) with:

```go
	if err := database.Initialize(cfg); err != nil {
		log.Fatalf("db connect: %v", err)
	}

	if cfg.ArchiveDB.Enabled() {
		if err := dbmigrate.Run(cfg.ArchiveDB.ConnectionString, archivemigrations.FS, "up", nil); err != nil {
			log.Fatalf("archive db migrate: %v", err)
		}
		if err := database.InitializeArchive(cfg); err != nil {
			log.Fatalf("archive db connect: %v", err)
		}
	} else {
		log.Println("ARCHIVE_DATABASE_URL not set — archive database disabled")
	}
```

- [ ] **Step 3: Build**

Run: `cd backend && go build ./...`
Expected: succeeds with no unused-import or type errors.

- [ ] **Step 4: Manual verification against a disposable Postgres**

```bash
# Start a disposable cluster (see Task 3 Step 4 for initdb/pg_ctl commands),
# then create two databases inside it to stand in for cloud + archive:
psql "postgres://$(whoami)@localhost:5499/postgres?sslmode=disable" -c "CREATE DATABASE ffsims_cloud"
psql "postgres://$(whoami)@localhost:5499/postgres?sslmode=disable" -c "CREATE DATABASE ffsims_archive"

cd backend
go build -o bin/migrate ./cmd/migrate

DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_cloud?sslmode=disable" ./bin/migrate up
DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_cloud?sslmode=disable" ./bin/migrate status
# Expected: all cloud migrations through 021 applied.

ARCHIVE_DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_archive?sslmode=disable" ./bin/migrate -db=archive up
ARCHIVE_DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_archive?sslmode=disable" ./bin/migrate -db=archive status
# Expected: 001_archive_sync_state applied.

# Worker boot with archive disabled (default local dev):
DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_cloud?sslmode=disable" \
  go run ./cmd/worker &
WORKER_PID=$!
sleep 2
kill $WORKER_PID
# Expected in the log: "ARCHIVE_DATABASE_URL not set — archive database disabled",
# and no fatal errors before that (it will likely fail later trying to reach
# a local Temporal server — that's fine, this step only verifies the archive
# branch didn't blow up).

# Worker boot with archive enabled, pointed at the already-migrated archive DB:
DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_cloud?sslmode=disable" \
  ARCHIVE_DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_archive?sslmode=disable" \
  go run ./cmd/worker &
WORKER_PID=$!
sleep 2
kill $WORKER_PID
# Expected in the log: "Connected to archive database (maxOpen=... maxIdle=... connLifetime=...)"
# with no archive-migrate or archive-connect fatal error.
```

- [ ] **Step 5: Commit**

```bash
git add backend/cmd/migrate/main.go backend/cmd/worker/main.go
git commit -m "feat: -db cloud|archive flag for cmd/migrate; auto-migrate + connect archive DB in cmd/worker"
```

---

### Task 7: Makefile targets for archive migrations

**Files:**
- Modify: `backend/Makefile`

**Interfaces:**
- Consumes: the `-db=archive` flag from Task 6.
- Produces: nothing consumed by later tasks — this is a convenience wrapper for ops.

- [ ] **Step 1: Add the targets**

In `backend/Makefile`, update the `.PHONY` line (line 1) to include the new targets:

```makefile
.PHONY: build clean run etl migrate migrate-status migrate-down migrate-archive migrate-archive-status migrate-archive-down deduplicate help
```

Add after the existing `migrate-down` target:

```makefile
migrate-archive: build ## Apply all pending goose migrations to the archive DB (requires ARCHIVE_DATABASE_URL)
	@./bin/migrate -db=archive up

migrate-archive-status: build ## Show goose migration status for the archive DB (requires ARCHIVE_DATABASE_URL)
	@./bin/migrate -db=archive status

migrate-archive-down: build ## Roll back the last archive migration (requires ARCHIVE_DATABASE_URL)
	@./bin/migrate -db=archive down
```

- [ ] **Step 2: Verify**

Run: `cd backend && make help`
Expected: output lists `migrate-archive`, `migrate-archive-status`, `migrate-archive-down` alongside the existing `migrate*` targets.

- [ ] **Step 3: Commit**

```bash
git add backend/Makefile
git commit -m "chore: add migrate-archive Makefile targets"
```

---

### Task 8: Cloud migration 021 — scavenger cursor indexes (T4)

**Files:**
- Create: `backend/migrations/021_scavenger_cursor_indexes.sql`

**Interfaces:**
- Produces: indexes `idx_sleeper_transactions_created_at`, `idx_sleeper_drafts_last_fetched_at`. Consumed by Task 5's `TestRun_CloudMigrations_ApplyCleanlyAndCreateScavengerIndexes` (already written in Task 5 — this task makes it pass) and, later, by T5's scavenger cursor queries (out of scope here).

`sleeper_transactions.created_at` (GORM `autoCreateTime`, insert time) has no index today — the existing `012_sleeper_indexes.sql` only indexes `created_at_sleeper` (Sleeper's own epoch timestamp, a different column), `type`, `status`, and `sleeper_league_id`. `sleeper_drafts.last_fetched_at` also has no index today. Both are needed by the future scavenger's cursor-based replication queries (`ORDER BY created_at` / `ORDER BY last_fetched_at`).

- [ ] **Step 1: Write the migration**

```sql
-- backend/migrations/021_scavenger_cursor_indexes.sql
-- +goose Up
-- +goose NO TRANSACTION

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_transactions_created_at
    ON sleeper_transactions (created_at);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_drafts_last_fetched_at
    ON sleeper_drafts (last_fetched_at);

-- +goose Down
-- +goose NO TRANSACTION

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_drafts_last_fetched_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_transactions_created_at;
```

- [ ] **Step 2: Run the dbmigrate tests from Task 5 to verify this closes the gap**

Run (with `TEST_DATABASE_URL` set — see Task 3 Step 4):
`cd backend && go test ./internal/dbmigrate/... -v -run TestRun_CloudMigrations_ApplyCleanlyAndCreateScavengerIndexes`
Expected: PASS — both indexes now exist after `migrate up`. (This test was written in Task 5 and was failing on the index-existence assertions until now — that failure is now resolved, not a new test.)

- [ ] **Step 3: Run the full backend test suite**

Run: `cd backend && go test ./... -v 2>&1 | tail -80`
Expected: everything PASSes (Postgres-gated tests PASS if `TEST_DATABASE_URL` is set, otherwise SKIP — no FAILs either way).

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/021_scavenger_cursor_indexes.sql
git commit -m "feat: add indexes on sleeper_transactions.created_at and sleeper_drafts.last_fetched_at"
```

---

## Verification

- [ ] `cd backend && go build ./...` succeeds.
- [ ] `cd backend && go vet ./...` reports nothing new.
- [ ] `cd backend && go test ./...` passes with `TEST_DATABASE_URL` unset (PG-gated tests SKIP, nothing FAILs).
- [ ] Full pass with a disposable Postgres per Task 3 Step 4: `TEST_DATABASE_URL="postgres://$(whoami)@localhost:5499/postgres?sslmode=disable" go test ./... -v` — every test PASSes, including the new ones in `internal/config`, `internal/database`, `internal/dbmigrate`, and the refactored `internal/activities/claim_pg_test.go`.
- [ ] Manual worker-boot check from Task 6 Step 4: both the archive-disabled and archive-enabled paths log the expected line with no fatal error in the archive branch.
- [ ] `go run ./cmd/worker` with `ARCHIVE_DATABASE_URL` unset behaves identically to `main` before this PR (no archive-related code runs).

## Self-Review

**Spec coverage:** T3 ("Second DB handle + archive migrations plumbing") — Tasks 1–7. T4 ("Cloud migration 021: CONCURRENTLY indexes on txn `created_at`, draft `last_fetched_at`") — Task 8. The design doc's other T3-adjacent claims (`ScavengerActivities{Cloud, Archive}`, `ADPRollupActivities{Read, Write}`, the `archive-maintenance` queue, dual-write alternative) are explicitly out of scope per the Global Constraints — they belong to T5/T7 and are call-site changes with nothing to inject into yet.

**Placeholder scan:** no TBD/TODO markers; every step has literal code or an exact command with expected output.

**Type consistency:** `dbmigrate.Run(dsn string, fsys fs.FS, command string, args []string) error` is the same signature used in Task 5 (definition + tests) and Task 6 (both call sites). `config.ArchiveDBConfig` / `Config.ArchiveDB` / `Enabled()` match across Tasks 2, 3, and 6. `database.Archive` / `InitializeArchive(cfg *config.Config) error` match across Tasks 3 and 6. `testutil.NewPGSchema(t, dsn, prefix) string` and `testutil.OpenGORM(t, scopedDSN) *gorm.DB` match across Tasks 1, 3, and 5.
