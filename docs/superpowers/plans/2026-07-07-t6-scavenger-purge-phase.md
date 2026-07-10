# T6: Scavenger Purge Phase Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the purge phase to the archive scavenger — cloud rows older than the retention window get deleted once verified present in the archive, shipped dark behind `SCAVENGER_PURGE_ENABLED=false`.

**Architecture:** Two new stateless (non-cursor) batch activities, `PurgeTransactionsBatch` and `PurgeDraftsBatch`, added to the existing `ScavengerActivities` from T5 (PR #152). Each batch scans the oldest cloud rows past `SCAVENGER_RETENTION_DAYS`, verifies presence (and, for drafts, pick-count parity) in the archive, deletes verified rows in short chunked transactions, and leaves unverified rows in place to retry next batch/run. `ScavengerDispatcher` calls these only when `SCAVENGER_PURGE_ENABLED=true` **and** the corresponding replicate stream(s) drained this run — purge never races ahead of replication. Unlike replicate-stream failures (logged and swallowed), a purge activity only ever errors when the oldest unverified row is older than `retention+15d` — that error is **not** swallowed, so the workflow run fails (red in Temporal UI = the replication-stalled alarm).

**Tech Stack:** Go, GORM (raw SQL + query builder), Temporal SDK (activities/workflow), PostgreSQL 16, goose migrations.

## Global Constraints

- `SCAVENGER_PURGE_ENABLED` default **false** — this task must not change purge behavior in production; it ships dark.
- `SCAVENGER_RETENTION_DAYS` default **30**.
- Alarm threshold is **retention + 15 days**: an unverified row older than that makes the purge activity return an error.
- Drafts additionally require the claim-pool-exclusion predicate (`sleeper_leagues.status IN ('in_season','complete') AND sleeper_leagues.last_drafts_fetched_at IS NOT NULL`) — mirrors `internal/activities/data_fetch.go:43-54`'s claim query so a purged draft can never be re-claimed and pick-refetched.
- `sleeper_draft_picks` has an FK to `sleeper_drafts` in the cloud schema (`migrations/005_sleeper_tables.sql`) with no `ON DELETE CASCADE` — picks must be deleted before the parent draft row.
- Deletes run in short, chunked transactions (not one transaction per whole batch) so purge never holds long locks on the hot cloud tables the API reads from.
- Reuse the existing `SCAVENGER_TXN_BATCH_SIZE` / `SCAVENGER_DRAFT_BATCH_SIZE` knobs for purge scan sizing — no new batch-size env vars.
- All PG-specific tests are gated on `TEST_DATABASE_URL` (skip if unset), using `testutil.NewPGSchema` + `testutil.OpenGORM`, matching `internal/activities/scavenger_test.go`'s existing convention.

---

## File Structure

| File | Change |
|---|---|
| `backend/internal/activities/params.go` | Add `PurgeBatchParams`, `PurgeBatchResult`; extend `ScavengerConfig` (RetentionDays, PurgeEnabled) and `ScavengerReport` (TransactionsPurged/Unverified, DraftsPurged/Unverified) |
| `backend/internal/activities/scavenger.go` | Add `PurgeTransactionsBatch`, `PurgeDraftsBatch`, and shared helpers (`purgeCandidate`, `splitVerifiedCandidates`, `deleteInChunks`, `checkUnverifiedAlarm`, `pickCountsByDraft`); extend `GetScavengerConfig` |
| `backend/internal/activities/scavenger_test.go` | Update the two existing config tests for new fields; add purge activity tests |
| `backend/internal/workflows/scavenger.go` | Wire the two purge loops into `ScavengerDispatcher`, gated on `PurgeEnabled` + per-stream drained flags |
| `backend/internal/workflows/workflows_test.go` | Add `ScavengerDispatcher` purge-gating and purge-error tests |
| `backend/migrations/022_scavenger_purge_indexes.sql` | New — `CONCURRENTLY` index on `sleeper_drafts(created_at)` for the purge candidate scan |
| `backend/internal/dbmigrate/dbmigrate_test.go` | Add the new index to the existing migration-index assertion list |
| `docs/superpowers/plans/2026-07-07-two-database-archive.md` | Mark T6's row done once the PR is up |

---

## Task 1: Purge config knobs (`SCAVENGER_RETENTION_DAYS`, `SCAVENGER_PURGE_ENABLED`)

**Files:**
- Modify: `backend/internal/activities/params.go`
- Modify: `backend/internal/activities/scavenger.go` (`GetScavengerConfig` only)
- Test: `backend/internal/activities/scavenger_test.go`

**Interfaces:**
- Produces: `ScavengerConfig.RetentionDays int`, `ScavengerConfig.PurgeEnabled bool` — consumed by Tasks 3-5.

- [ ] **Step 1: Write the failing tests**

In `backend/internal/activities/scavenger_test.go`, replace `TestGetScavengerConfig_ReadsEnvWithDefaults` and `TestGetScavengerConfig_ReadsOverrides` with versions that also check the new fields, and add a defaults-only assertion for the new fields:

```go
func TestGetScavengerConfig_ReadsEnvWithDefaults(t *testing.T) {
	a := &activities.ScavengerActivities{}
	cfg, err := a.GetScavengerConfig(context.Background())
	if err != nil {
		t.Fatalf("GetScavengerConfig: %v", err)
	}
	if cfg.LeagueBatchSize != 500 || cfg.TxnBatchSize != 5000 || cfg.DraftBatchSize != 200 || cfg.MaxBatchesPerRun != 50 {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if cfg.RetentionDays != 30 {
		t.Errorf("RetentionDays = %d, want 30", cfg.RetentionDays)
	}
	if cfg.PurgeEnabled {
		t.Errorf("PurgeEnabled = true, want false (kill-switch defaults off)")
	}
}

func TestGetScavengerConfig_ReadsOverrides(t *testing.T) {
	t.Setenv("SCAVENGER_LEAGUE_BATCH_SIZE", "10")
	t.Setenv("SCAVENGER_TXN_BATCH_SIZE", "20")
	t.Setenv("SCAVENGER_DRAFT_BATCH_SIZE", "30")
	t.Setenv("SCAVENGER_MAX_BATCHES_PER_RUN", "5")
	t.Setenv("SCAVENGER_RETENTION_DAYS", "45")
	t.Setenv("SCAVENGER_PURGE_ENABLED", "true")

	a := &activities.ScavengerActivities{}
	cfg, err := a.GetScavengerConfig(context.Background())
	if err != nil {
		t.Fatalf("GetScavengerConfig: %v", err)
	}
	want := activities.ScavengerConfig{
		LeagueBatchSize: 10, TxnBatchSize: 20, DraftBatchSize: 30, MaxBatchesPerRun: 5,
		RetentionDays: 45, PurgeEnabled: true,
	}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/activities/... -run TestGetScavengerConfig -v`
Expected: FAIL — compile error (`cfg.RetentionDays undefined`) since the field doesn't exist yet.

- [ ] **Step 3: Add the fields**

In `backend/internal/activities/params.go`, extend `ScavengerConfig`:

```go
// ScavengerConfig is read from env by GetScavengerConfig so the dispatcher
// workflow (which cannot read env deterministically) can be tuned without a
// redeploy of workflow code.
type ScavengerConfig struct {
	LeagueBatchSize  int  // SCAVENGER_LEAGUE_BATCH_SIZE, default 500
	TxnBatchSize     int  // SCAVENGER_TXN_BATCH_SIZE, default 5000
	DraftBatchSize   int  // SCAVENGER_DRAFT_BATCH_SIZE, default 200 (drafts per batch; each draft's picks are copied alongside it)
	MaxBatchesPerRun int  // SCAVENGER_MAX_BATCHES_PER_RUN, default 50
	RetentionDays    int  // SCAVENGER_RETENTION_DAYS, default 30 — cloud rows older than this are purge candidates
	PurgeEnabled     bool // SCAVENGER_PURGE_ENABLED, default false — kill-switch; purge activities only run when true
}
```

In `backend/internal/activities/scavenger.go`, extend `GetScavengerConfig`:

```go
// GetScavengerConfig returns the scavenger's tuning knobs from env, clamped
// to at least 1 so a bad value can't stall replication or break a query's
// LIMIT. PurgeEnabled has no clamp — it's a bool kill-switch, not a size.
func (a *ScavengerActivities) GetScavengerConfig(ctx context.Context) (ScavengerConfig, error) {
	return ScavengerConfig{
		LeagueBatchSize:  max(helpers.GetEnv("SCAVENGER_LEAGUE_BATCH_SIZE", 500), 1),
		TxnBatchSize:     max(helpers.GetEnv("SCAVENGER_TXN_BATCH_SIZE", 5000), 1),
		DraftBatchSize:   max(helpers.GetEnv("SCAVENGER_DRAFT_BATCH_SIZE", 200), 1),
		MaxBatchesPerRun: max(helpers.GetEnv("SCAVENGER_MAX_BATCHES_PER_RUN", 50), 1),
		RetentionDays:    max(helpers.GetEnv("SCAVENGER_RETENTION_DAYS", 30), 1),
		PurgeEnabled:     helpers.GetEnv("SCAVENGER_PURGE_ENABLED", false),
	}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd backend && go test ./internal/activities/... -run TestGetScavengerConfig -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/activities/params.go backend/internal/activities/scavenger.go backend/internal/activities/scavenger_test.go
git commit -m "Add SCAVENGER_RETENTION_DAYS and SCAVENGER_PURGE_ENABLED config knobs"
```

---

## Task 2: Cloud index for the draft purge scan

**Files:**
- Create: `backend/migrations/022_scavenger_purge_indexes.sql`
- Modify: `backend/internal/dbmigrate/dbmigrate_test.go`

**Interfaces:**
- Produces: index `idx_sleeper_drafts_created_at` on cloud `sleeper_drafts(created_at)`, consumed by Task 4's candidate query (`WHERE d.created_at < ? ORDER BY d.created_at, ...`).

- [ ] **Step 1: Write the failing test**

In `backend/internal/dbmigrate/dbmigrate_test.go`, extend the index list in `TestRun_CloudMigrations_ApplyCleanlyAndCreateScavengerIndexes`:

```go
	for _, idx := range []string{"idx_sleeper_transactions_created_at", "idx_sleeper_drafts_last_fetched_at", "idx_sleeper_drafts_created_at"} {
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd backend && TEST_DATABASE_URL=<your-local-pg-dsn> go test ./internal/dbmigrate/... -run TestRun_CloudMigrations_ApplyCleanlyAndCreateScavengerIndexes -v`
Expected: FAIL — `expected index idx_sleeper_drafts_created_at to exist after migrate up`
(If `TEST_DATABASE_URL` isn't available in your environment, skip straight to Step 3 — this test is skipped without it, same as the rest of the PG-gated suite; the migration must still be validated when a PG instance is available before merging.)

- [ ] **Step 3: Add the migration**

Create `backend/migrations/022_scavenger_purge_indexes.sql`:

```sql
-- +goose Up
-- +goose NO TRANSACTION

-- Supports the purge candidate scan in PurgeDraftsBatch: WHERE created_at <
-- cutoff ORDER BY created_at, sleeper_draft_id. Mirrors 021's transactions
-- index — sleeper_drafts had no created_at index until now.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_drafts_created_at
    ON sleeper_drafts (created_at);

-- +goose Down
-- +goose NO TRANSACTION

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_drafts_created_at;
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd backend && TEST_DATABASE_URL=<your-local-pg-dsn> go test ./internal/dbmigrate/... -run TestRun_CloudMigrations_ApplyCleanlyAndCreateScavengerIndexes -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/migrations/022_scavenger_purge_indexes.sql backend/internal/dbmigrate/dbmigrate_test.go
git commit -m "Add cloud index on sleeper_drafts(created_at) for the purge scan"
```

---

## Task 3: `PurgeTransactionsBatch` activity

**Files:**
- Modify: `backend/internal/activities/params.go`
- Modify: `backend/internal/activities/scavenger.go`
- Test: `backend/internal/activities/scavenger_test.go`

**Interfaces:**
- Consumes: `a.Cloud *gorm.DB`, `a.Archive *gorm.DB` (from `ScavengerActivities`, already present since T5).
- Produces: `PurgeBatchParams{BatchSize int, RetentionDays int}`, `PurgeBatchResult{Purged int, Unverified int, Drained bool}`, `(a *ScavengerActivities) PurgeTransactionsBatch(ctx, PurgeBatchParams) (PurgeBatchResult, error)` — consumed by Task 5's dispatcher wiring. Also produces shared helpers `purgeCandidate`, `splitVerifiedCandidates`, `deleteInChunks`, `checkUnverifiedAlarm` — consumed by Task 4's `PurgeDraftsBatch`.

- [ ] **Step 1: Write the failing tests**

Append to `backend/internal/activities/scavenger_test.go`:

```go
func TestPurgeTransactionsBatch_DeletesVerifiedOldRows(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	old := time.Now().UTC().AddDate(0, 0, -40)
	for i, id := range []string{"t1", "t2"} {
		if err := cloud.Create(&models.SleeperTransaction{
			SleeperTransactionID: id, SleeperLeagueID: "lg1", CreatedAt: old.Add(time.Duration(i) * time.Second),
		}).Error; err != nil {
			t.Fatalf("seed cloud txn %s: %v", id, err)
		}
		if err := archive.Create(&models.ArchiveSleeperTransaction{
			SleeperTransactionID: id, SleeperLeagueID: "lg1", CreatedAt: old.Add(time.Duration(i) * time.Second),
		}).Error; err != nil {
			t.Fatalf("seed archive txn %s: %v", id, err)
		}
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeTransactionsBatch: %v", err)
	}
	if res.Purged != 2 || res.Unverified != 0 || !res.Drained {
		t.Errorf("res = %+v, want {Purged: 2, Unverified: 0, Drained: true}", res)
	}
	var count int64
	cloud.Model(&models.SleeperTransaction{}).Count(&count)
	if count != 0 {
		t.Errorf("expected cloud transactions purged, got %d remaining", count)
	}
}

func TestPurgeTransactionsBatch_SkipsUnverifiedRows(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	old := time.Now().UTC().AddDate(0, 0, -40)
	if err := cloud.Create(&models.SleeperTransaction{SleeperTransactionID: "t1", CreatedAt: old}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Not replicated to archive.

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeTransactionsBatch: %v", err)
	}
	if res.Purged != 0 || res.Unverified != 1 {
		t.Errorf("res = %+v, want {Purged: 0, Unverified: 1}", res)
	}
	var count int64
	cloud.Model(&models.SleeperTransaction{}).Count(&count)
	if count != 1 {
		t.Errorf("expected the unverified row to remain in cloud, got %d rows", count)
	}
}

func TestPurgeTransactionsBatch_IgnoresRowsWithinRetention(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	recent := time.Now().UTC().AddDate(0, 0, -5) // within the 30-day retention window
	if err := cloud.Create(&models.SleeperTransaction{SleeperTransactionID: "t1", CreatedAt: recent}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeTransactionsBatch: %v", err)
	}
	if res.Purged != 0 || res.Unverified != 0 || !res.Drained {
		t.Errorf("res = %+v, want no candidates found (row is within retention)", res)
	}
}

func TestPurgeTransactionsBatch_ErrorsWhenOldestUnverifiedPastAlarmThreshold(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	waaayOld := time.Now().UTC().AddDate(0, 0, -46) // 30d retention + 15d alarm + 1
	if err := cloud.Create(&models.SleeperTransaction{SleeperTransactionID: "t1", CreatedAt: waaayOld}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Not replicated to archive — stalled.

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	_, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err == nil {
		t.Fatal("expected an error once the oldest unverified row exceeds retention+15d, got nil")
	}
}

func TestPurgeTransactionsBatch_DrainedWhenFewerThanBatchSize(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	old := time.Now().UTC().AddDate(0, 0, -40)
	for i, id := range []string{"t1", "t2", "t3"} {
		if err := cloud.Create(&models.SleeperTransaction{SleeperTransactionID: id, CreatedAt: old.Add(time.Duration(i) * time.Second)}).Error; err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
		if err := archive.Create(&models.ArchiveSleeperTransaction{SleeperTransactionID: id, CreatedAt: old.Add(time.Duration(i) * time.Second)}).Error; err != nil {
			t.Fatalf("seed archive %s: %v", id, err)
		}
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 2, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeTransactionsBatch: %v", err)
	}
	if res.Purged != 2 || res.Drained {
		t.Errorf("expected a full, non-drained batch of 2, got %+v", res)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && TEST_DATABASE_URL=<your-local-pg-dsn> go test ./internal/activities/... -run TestPurgeTransactionsBatch -v`
Expected: FAIL — compile error (`a.PurgeTransactionsBatch undefined`).

- [ ] **Step 3: Add the types and implementation**

In `backend/internal/activities/params.go`, add after `ReplicateBatchResult` and extend `ScavengerReport`:

```go
// PurgeBatchParams is shared by PurgeTransactionsBatch and PurgeDraftsBatch —
// they differ only in which table(s) and verification rule they use.
type PurgeBatchParams struct {
	BatchSize     int
	RetentionDays int
}

// PurgeBatchResult reports one purge batch's outcome. Purged counts rows
// actually deleted from cloud. Unverified counts candidates left in place
// because they couldn't be confirmed present (and, for drafts, pick-count
// matched) in the archive yet — they are retried by the next batch/run.
// Drained means fewer than BatchSize purge candidates were found past the
// retention cutoff — this data type is caught up for this run.
type PurgeBatchResult struct {
	Purged     int
	Unverified int
	Drained    bool
}
```

Extend `ScavengerReport`:

```go
// ScavengerReport summarizes one ScavengerDispatcher run.
type ScavengerReport struct {
	LeaguesReplicated      int
	TransactionsReplicated int
	DraftHeadersReplicated int
	DraftPicksReplicated   int
	TransactionsPurged     int
	TransactionsUnverified int
	DraftsPurged           int
	DraftsUnverified       int
}
```

In `backend/internal/activities/scavenger.go`, add near the bottom of the file (after `ReplicateTransactionsBatch`):

```go
// purgeDeleteChunkSize caps each purge delete transaction so a single
// batch's worth of deletes (up to a few thousand rows) doesn't hold row
// locks on the hot cloud tables for one long transaction while the API is
// serving reads.
const purgeDeleteChunkSize = 500

// purgeCandidate is one row eligible for purge consideration: its ID and the
// timestamp used both to order the scan and to report the alarm age when the
// row can't be verified.
type purgeCandidate struct {
	ID        string    `gorm:"column:id"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// splitVerifiedCandidates partitions candidates (ordered oldest-first) into
// IDs safe to delete (present in verified) and a count/oldest-timestamp of
// the rest. Because candidates are ordered ascending by CreatedAt, the first
// unverified row encountered is the oldest unverified row in the whole
// candidate set, not just this batch.
func splitVerifiedCandidates(candidates []purgeCandidate, verified map[string]bool) (toDelete []string, unverifiedCount int, oldestUnverified *time.Time) {
	for _, c := range candidates {
		if verified[c.ID] {
			toDelete = append(toDelete, c.ID)
			continue
		}
		unverifiedCount++
		if oldestUnverified == nil {
			t := c.CreatedAt
			oldestUnverified = &t
		}
	}
	return toDelete, unverifiedCount, oldestUnverified
}

// deleteInChunks runs deleteFn against ids in chunks of purgeDeleteChunkSize,
// each in its own short transaction, so a purge batch never holds one long
// transaction's worth of row locks on a hot cloud table.
func deleteInChunks(ctx context.Context, db *gorm.DB, ids []string, deleteFn func(tx *gorm.DB, chunk []string) error) error {
	for i := 0; i < len(ids); i += purgeDeleteChunkSize {
		chunk := ids[i:min(i+purgeDeleteChunkSize, len(ids))]
		if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return deleteFn(tx, chunk)
		}); err != nil {
			return err
		}
	}
	return nil
}

// checkUnverifiedAlarm returns an error when oldest — the oldest unverified
// candidate's timestamp seen in a purge batch — is older than
// retentionDays+15d. That means a row has sat unpurgeable for two full
// scavenger cycles past retention: replication has stalled, not just
// lagged. Unlike a replicate stream's swallowed failure, this error is
// meant to fail the activity (and the workflow run) so Temporal shows a red
// run — the intended stalled-replication alarm.
func checkUnverifiedAlarm(stream string, oldest *time.Time, retentionDays int) error {
	if oldest == nil {
		return nil
	}
	alarmAge := time.Duration(retentionDays+15) * 24 * time.Hour
	if age := time.Since(*oldest); age > alarmAge {
		return fmt.Errorf("scavenger purge: oldest unverified %s row is %s old (retention %dd + 15d alarm threshold) — replication appears stalled",
			stream, age.Round(time.Hour), retentionDays)
	}
	return nil
}

const selectPurgeTransactionCandidatesSQL = `
SELECT sleeper_transaction_id AS id, created_at
FROM sleeper_transactions
WHERE created_at < ?
ORDER BY created_at, sleeper_transaction_id
LIMIT ?`

// PurgeTransactionsBatch deletes up to BatchSize of the oldest cloud
// transactions older than RetentionDays that are verified present in the
// archive. Unverified rows (not yet replicated) are left in place — the next
// batch/run naturally retries them since only verified rows are ever
// deleted. Returns an error (see checkUnverifiedAlarm) if the oldest
// unverified row has sat past retention+15d.
func (a *ScavengerActivities) PurgeTransactionsBatch(ctx context.Context, params PurgeBatchParams) (PurgeBatchResult, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -params.RetentionDays)

	var candidates []purgeCandidate
	if err := a.Cloud.WithContext(ctx).Raw(selectPurgeTransactionCandidatesSQL, cutoff, params.BatchSize).
		Scan(&candidates).Error; err != nil {
		return PurgeBatchResult{}, err
	}
	if len(candidates) == 0 {
		return PurgeBatchResult{Drained: true}, nil
	}

	ids := make([]string, len(candidates))
	for i, c := range candidates {
		ids[i] = c.ID
	}

	var archiveIDs []string
	if err := a.Archive.WithContext(ctx).Table("sleeper_transactions").
		Where("sleeper_transaction_id IN ?", ids).
		Pluck("sleeper_transaction_id", &archiveIDs).Error; err != nil {
		return PurgeBatchResult{}, err
	}
	verified := make(map[string]bool, len(archiveIDs))
	for _, id := range archiveIDs {
		verified[id] = true
	}

	toDelete, unverifiedCount, oldestUnverified := splitVerifiedCandidates(candidates, verified)

	if len(toDelete) > 0 {
		if err := deleteInChunks(ctx, a.Cloud, toDelete, func(tx *gorm.DB, chunk []string) error {
			return tx.Where("sleeper_transaction_id IN ?", chunk).Delete(&models.SleeperTransaction{}).Error
		}); err != nil {
			return PurgeBatchResult{}, err
		}
	}

	if err := checkUnverifiedAlarm(streamTransactions, oldestUnverified, params.RetentionDays); err != nil {
		return PurgeBatchResult{}, err
	}

	return PurgeBatchResult{
		Purged:     len(toDelete),
		Unverified: unverifiedCount,
		Drained:    len(candidates) < params.BatchSize,
	}, nil
}
```

`scavenger.go` already imports `"fmt"` (used by `readCursor`'s error wrapping) — no import changes needed.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd backend && TEST_DATABASE_URL=<your-local-pg-dsn> go test ./internal/activities/... -run TestPurgeTransactionsBatch -v`
Expected: PASS (all 5 subtests)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/activities/params.go backend/internal/activities/scavenger.go backend/internal/activities/scavenger_test.go
git commit -m "Add PurgeTransactionsBatch: verify-then-delete cloud transactions past retention"
```

---

## Task 4: `PurgeDraftsBatch` activity (claim-pool exclusion + pick-count parity)

**Files:**
- Modify: `backend/internal/activities/scavenger.go`
- Test: `backend/internal/activities/scavenger_test.go`

**Interfaces:**
- Consumes: `purgeCandidate`, `splitVerifiedCandidates`, `deleteInChunks`, `checkUnverifiedAlarm`, `PurgeBatchParams`, `PurgeBatchResult` from Task 3.
- Produces: `(a *ScavengerActivities) PurgeDraftsBatch(ctx, PurgeBatchParams) (PurgeBatchResult, error)` — consumed by Task 5.

- [ ] **Step 1: Write the failing tests**

Append to `backend/internal/activities/scavenger_test.go`. These seed a league that already satisfies the claim-pool-exclusion predicate (`status='complete'`, `last_drafts_fetched_at` set) unless a test name says otherwise:

```go
func TestPurgeDraftsBatch_DeletesVerifiedDraftAndPicks(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	fetchedAt := time.Now().UTC()
	old := time.Now().UTC().AddDate(0, 0, -40)
	if err := cloud.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg1", Season: "2026", Status: "complete", LastDraftsFetchedAt: &fetchedAt,
	}).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: "2026",
		LastFetchedAt: &fetchedAt, CreatedAt: old,
	}).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	if err := cloud.Create(&models.SleeperDraftPick{SleeperDraftID: "d1", Round: 1, PickNo: 1, SleeperPlayerID: "p1"}).Error; err != nil {
		t.Fatalf("seed pick: %v", err)
	}
	if err := archive.Create(&models.ArchiveSleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: "2026",
		LastFetchedAt: &fetchedAt, CreatedAt: old,
	}).Error; err != nil {
		t.Fatalf("seed archive draft: %v", err)
	}
	if err := archive.Create(&models.ArchiveSleeperDraftPick{SleeperDraftID: "d1", Round: 1, PickNo: 1, SleeperPlayerID: "p1"}).Error; err != nil {
		t.Fatalf("seed archive pick: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeDraftsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeDraftsBatch: %v", err)
	}
	if res.Purged != 1 || res.Unverified != 0 || !res.Drained {
		t.Errorf("res = %+v, want {Purged: 1, Unverified: 0, Drained: true}", res)
	}
	var draftCount, pickCount int64
	cloud.Model(&models.SleeperDraft{}).Count(&draftCount)
	cloud.Model(&models.SleeperDraftPick{}).Count(&pickCount)
	if draftCount != 0 || pickCount != 0 {
		t.Errorf("expected draft and picks purged from cloud, got draftCount=%d pickCount=%d", draftCount, pickCount)
	}
}

func TestPurgeDraftsBatch_SkipsLeagueStillInSyncPool(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	old := time.Now().UTC().AddDate(0, 0, -40)
	if err := cloud.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg1", Season: "2026", Status: "pre_draft", // not yet excluded from the claim pool
	}).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
	fetchedAt := time.Now().UTC()
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: "2026",
		LastFetchedAt: &fetchedAt, CreatedAt: old,
	}).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	if err := archive.Create(&models.ArchiveSleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: "2026",
		LastFetchedAt: &fetchedAt, CreatedAt: old,
	}).Error; err != nil {
		t.Fatalf("seed archive draft: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeDraftsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeDraftsBatch: %v", err)
	}
	if res.Purged != 0 || !res.Drained {
		t.Errorf("expected the draft to be excluded from purge candidates entirely (league still claimable), got %+v", res)
	}
	var draftCount int64
	cloud.Model(&models.SleeperDraft{}).Count(&draftCount)
	if draftCount != 1 {
		t.Errorf("expected the draft to remain in cloud, got %d", draftCount)
	}
}

func TestPurgeDraftsBatch_SkipsPickCountMismatch(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	fetchedAt := time.Now().UTC()
	old := time.Now().UTC().AddDate(0, 0, -40)
	if err := cloud.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg1", Season: "2026", Status: "complete", LastDraftsFetchedAt: &fetchedAt,
	}).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: "2026",
		LastFetchedAt: &fetchedAt, CreatedAt: old,
	}).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	for _, pickNo := range []int{1, 2} {
		if err := cloud.Create(&models.SleeperDraftPick{SleeperDraftID: "d1", Round: 1, PickNo: pickNo}).Error; err != nil {
			t.Fatalf("seed pick %d: %v", pickNo, err)
		}
	}
	if err := archive.Create(&models.ArchiveSleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: "2026",
		LastFetchedAt: &fetchedAt, CreatedAt: old,
	}).Error; err != nil {
		t.Fatalf("seed archive draft: %v", err)
	}
	// Only 1 of 2 picks made it to archive — parity mismatch.
	if err := archive.Create(&models.ArchiveSleeperDraftPick{SleeperDraftID: "d1", Round: 1, PickNo: 1}).Error; err != nil {
		t.Fatalf("seed archive pick: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeDraftsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeDraftsBatch: %v", err)
	}
	if res.Purged != 0 || res.Unverified != 1 {
		t.Errorf("res = %+v, want {Purged: 0, Unverified: 1} (pick count mismatch)", res)
	}
	var draftCount int64
	cloud.Model(&models.SleeperDraft{}).Count(&draftCount)
	if draftCount != 1 {
		t.Errorf("expected the draft to remain in cloud, got %d", draftCount)
	}
}

func TestPurgeDraftsBatch_IgnoresRecentDrafts(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	fetchedAt := time.Now().UTC()
	recent := time.Now().UTC().AddDate(0, 0, -5)
	if err := cloud.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg1", Season: "2026", Status: "complete", LastDraftsFetchedAt: &fetchedAt,
	}).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: "2026",
		LastFetchedAt: &fetchedAt, CreatedAt: recent,
	}).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeDraftsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeDraftsBatch: %v", err)
	}
	if res.Purged != 0 || !res.Drained {
		t.Errorf("expected no candidates (draft is within retention), got %+v", res)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && TEST_DATABASE_URL=<your-local-pg-dsn> go test ./internal/activities/... -run TestPurgeDraftsBatch -v`
Expected: FAIL — compile error (`a.PurgeDraftsBatch undefined`).

- [ ] **Step 3: Add the implementation**

In `backend/internal/activities/scavenger.go`, add after `PurgeTransactionsBatch`:

```go
const selectPurgeDraftCandidatesSQL = `
SELECT d.sleeper_draft_id AS id, d.created_at
FROM sleeper_drafts d
JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id
WHERE d.created_at < ?
  AND l.status IN ('in_season', 'complete')
  AND l.last_drafts_fetched_at IS NOT NULL
ORDER BY d.created_at, d.sleeper_draft_id
LIMIT ?`

// pickCountsByDraft returns sleeper_draft_id -> pick count for draftIDs,
// used by PurgeDraftsBatch to verify pick-count parity between cloud and
// archive before deleting. A draft absent from the result has zero picks.
func pickCountsByDraft(ctx context.Context, db *gorm.DB, draftIDs []string) (map[string]int, error) {
	var rows []struct {
		SleeperDraftID string `gorm:"column:sleeper_draft_id"`
		Count          int    `gorm:"column:count"`
	}
	if err := db.WithContext(ctx).Table("sleeper_draft_picks").
		Select("sleeper_draft_id, count(*) as count").
		Where("sleeper_draft_id IN ?", draftIDs).
		Group("sleeper_draft_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(rows))
	for _, r := range rows {
		counts[r.SleeperDraftID] = r.Count
	}
	return counts, nil
}

// PurgeDraftsBatch deletes up to BatchSize of the oldest cloud drafts (and
// their picks) older than RetentionDays whose owning league satisfies the
// claim-pool-exclusion predicate — status IN ('in_season','complete') AND
// last_drafts_fetched_at IS NOT NULL, the same condition that permanently
// excludes a league from ClaimLeaguesForDrafts (data_fetch.go:43-54).
// Purging a draft whose league could still be re-claimed would let
// syncOneLeagueDrafts recreate the header with last_fetched_at = NULL and
// trigger a full pick-refetch loop.
//
// A draft is verified only when its header is present in the archive AND
// its cloud and archive pick counts match exactly. Unverified drafts are
// left in place — the next batch/run retries them. Picks are deleted before
// the draft header (FK, no ON DELETE CASCADE in the cloud schema).
func (a *ScavengerActivities) PurgeDraftsBatch(ctx context.Context, params PurgeBatchParams) (PurgeBatchResult, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -params.RetentionDays)

	var candidates []purgeCandidate
	if err := a.Cloud.WithContext(ctx).Raw(selectPurgeDraftCandidatesSQL, cutoff, params.BatchSize).
		Scan(&candidates).Error; err != nil {
		return PurgeBatchResult{}, err
	}
	if len(candidates) == 0 {
		return PurgeBatchResult{Drained: true}, nil
	}

	ids := make([]string, len(candidates))
	for i, c := range candidates {
		ids[i] = c.ID
	}

	var archiveDraftIDs []string
	if err := a.Archive.WithContext(ctx).Table("sleeper_drafts").
		Where("sleeper_draft_id IN ?", ids).
		Pluck("sleeper_draft_id", &archiveDraftIDs).Error; err != nil {
		return PurgeBatchResult{}, err
	}
	headerPresent := make(map[string]bool, len(archiveDraftIDs))
	for _, id := range archiveDraftIDs {
		headerPresent[id] = true
	}

	cloudPickCounts, err := pickCountsByDraft(ctx, a.Cloud, ids)
	if err != nil {
		return PurgeBatchResult{}, err
	}
	archivePickCounts, err := pickCountsByDraft(ctx, a.Archive, ids)
	if err != nil {
		return PurgeBatchResult{}, err
	}

	verified := make(map[string]bool, len(ids))
	for _, id := range ids {
		if headerPresent[id] && cloudPickCounts[id] == archivePickCounts[id] {
			verified[id] = true
		}
	}

	toDelete, unverifiedCount, oldestUnverified := splitVerifiedCandidates(candidates, verified)

	if len(toDelete) > 0 {
		if err := deleteInChunks(ctx, a.Cloud, toDelete, func(tx *gorm.DB, chunk []string) error {
			if err := tx.Where("sleeper_draft_id IN ?", chunk).Delete(&models.SleeperDraftPick{}).Error; err != nil {
				return err
			}
			return tx.Where("sleeper_draft_id IN ?", chunk).Delete(&models.SleeperDraft{}).Error
		}); err != nil {
			return PurgeBatchResult{}, err
		}
	}

	if err := checkUnverifiedAlarm("sleeper_drafts", oldestUnverified, params.RetentionDays); err != nil {
		return PurgeBatchResult{}, err
	}

	return PurgeBatchResult{
		Purged:     len(toDelete),
		Unverified: unverifiedCount,
		Drained:    len(candidates) < params.BatchSize,
	}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd backend && TEST_DATABASE_URL=<your-local-pg-dsn> go test ./internal/activities/... -run TestPurgeDraftsBatch -v`
Expected: PASS (all 4 subtests)

Then run the full activities package to confirm nothing else broke:

Run: `cd backend && TEST_DATABASE_URL=<your-local-pg-dsn> go test ./internal/activities/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/activities/scavenger.go backend/internal/activities/scavenger_test.go
git commit -m "Add PurgeDraftsBatch: claim-pool-exclusion + pick-count-parity verified purge"
```

---

## Task 5: Wire the purge phase into `ScavengerDispatcher`

**Files:**
- Modify: `backend/internal/workflows/scavenger.go`
- Test: `backend/internal/workflows/workflows_test.go`
- Modify: `docs/superpowers/plans/2026-07-07-two-database-archive.md` (final step)

**Interfaces:**
- Consumes: `activities.ScavengerConfig.{RetentionDays,PurgeEnabled}`, `activities.PurgeBatchParams`, `activities.PurgeBatchResult`, `sa.PurgeTransactionsBatch`, `sa.PurgeDraftsBatch` (Tasks 1, 3, 4).
- Produces: `ScavengerReport.{TransactionsPurged,TransactionsUnverified,DraftsPurged,DraftsUnverified}` populated by `ScavengerDispatcher`.

- [ ] **Step 1: Write the failing tests**

Append to `backend/internal/workflows/workflows_test.go`, after the existing two `ScavengerDispatcher` tests:

```go
func TestScavengerDispatcher_PurgeDisabledByDefault_NeverCallsPurgeActivities(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	cfg := activities.ScavengerConfig{
		LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50,
		RetentionDays: 30, PurgeEnabled: false,
	}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)
	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateTransactionsBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftHeadersBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftPicksBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	// No PurgeTransactionsBatch / PurgeDraftsBatch mocks registered: if the
	// dispatcher calls them anyway, the test environment fails on the
	// unmocked activity call.

	env.ExecuteWorkflow(workflows.ScavengerDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestScavengerDispatcher_PurgeEnabledAndCaughtUp_RunsPurgeAndAccumulatesReport(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	cfg := activities.ScavengerConfig{
		LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50,
		RetentionDays: 30, PurgeEnabled: true,
	}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)
	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateTransactionsBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftHeadersBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftPicksBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.PurgeTransactionsBatch, mock.Anything, activities.PurgeBatchParams{BatchSize: 5000, RetentionDays: 30}).
		Return(activities.PurgeBatchResult{Purged: 100, Unverified: 2, Drained: true}, nil).Once()
	env.OnActivity(sa.PurgeDraftsBatch, mock.Anything, activities.PurgeBatchParams{BatchSize: 200, RetentionDays: 30}).
		Return(activities.PurgeBatchResult{Purged: 4, Unverified: 1, Drained: true}, nil).Once()

	env.ExecuteWorkflow(workflows.ScavengerDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report activities.ScavengerReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, 100, report.TransactionsPurged)
	require.Equal(t, 2, report.TransactionsUnverified)
	require.Equal(t, 4, report.DraftsPurged)
	require.Equal(t, 1, report.DraftsUnverified)
	env.AssertExpectations(t)
}

func TestScavengerDispatcher_PurgeSkippedWhenReplicateNotCaughtUp(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	// MaxBatchesPerRun: 1 with Drained: false means every stream hits the
	// iteration cap without catching up this run.
	cfg := activities.ScavengerConfig{
		LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 1,
		RetentionDays: 30, PurgeEnabled: true,
	}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)
	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Replicated: 500, Drained: false}, nil).Once()
	env.OnActivity(sa.ReplicateTransactionsBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Replicated: 5000, Drained: false}, nil).Once()
	env.OnActivity(sa.ReplicateDraftHeadersBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Replicated: 200, Drained: false}, nil).Once()
	env.OnActivity(sa.ReplicateDraftPicksBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Replicated: 200, Drained: false}, nil).Once()
	// No purge mocks: neither stream drained, so purge must not run even
	// though PurgeEnabled is true.

	env.ExecuteWorkflow(workflows.ScavengerDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestScavengerDispatcher_PurgeActivityErrorFailsTheWorkflowRun(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	cfg := activities.ScavengerConfig{
		LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50,
		RetentionDays: 30, PurgeEnabled: true,
	}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)
	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateTransactionsBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftHeadersBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftPicksBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.PurgeTransactionsBatch, mock.Anything, mock.Anything).
		Return(activities.PurgeBatchResult{}, temporal.NewNonRetryableApplicationError("replication stalled", "test", nil)).Once()

	env.ExecuteWorkflow(workflows.ScavengerDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError()) // unlike replicate stream failures, purge errors must NOT be swallowed
	env.AssertExpectations(t)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/workflows/... -run TestScavengerDispatcher_Purge -v`
Expected: By this point Tasks 3-4 are already merged, so `sa.PurgeTransactionsBatch`/`sa.PurgeDraftsBatch` compile fine — but the dispatcher doesn't call them yet. `TestScavengerDispatcher_PurgeEnabledAndCaughtUp_RunsPurgeAndAccumulatesReport` and `TestScavengerDispatcher_PurgeActivityErrorFailsTheWorkflowRun` FAIL: their `.Once()` purge-activity expectations are never satisfied (`env.AssertExpectations(t)` fails), and the report's Purged/Unverified fields stay zero. `TestScavengerDispatcher_PurgeDisabledByDefault_NeverCallsPurgeActivities` and `TestScavengerDispatcher_PurgeSkippedWhenReplicateNotCaughtUp` trivially PASS already (the dispatcher calls no purge activities either way, pre- or post-wiring) — they exist to guard against regressions once Step 3 adds the gating logic, not to prove it red-green.

- [ ] **Step 3: Wire the purge loops into the dispatcher**

Replace the body of `backend/internal/workflows/scavenger.go` with:

```go
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
// the same position. All five (four replicate + config) activity calls use
// defaultActivityOptions (not batchActivityOptions): unlike the per-league
// sync batch activities, these are fast single-query DB-to-DB copies with no
// external API calls and no activity.RecordHeartbeat — batchActivityOptions'
// HeartbeatTimeout is for activities that actually heartbeat. Runs on the
// archive-maintenance queue, which only exists when ARCHIVE_DATABASE_URL is
// set — see cmd/worker/main.go.
//
// After replication, the purge phase (transactions, then drafts+picks)
// deletes verified-old cloud rows — but only when cfg.PurgeEnabled is true
// (SCAVENGER_PURGE_ENABLED, default false: purge ships dark) AND the
// corresponding replicate stream(s) drained this run, so purge never scans
// ahead of a backlog it already knows exists. Unlike the replicate loops
// above, a purge activity error is NOT swallowed: PurgeTransactionsBatch and
// PurgeDraftsBatch only ever return an error when the oldest unverified row
// has sat past retention+15d, meaning replication has stalled — that must
// surface as a failed (red) run, the intended Temporal-UI alarm.
func ScavengerDispatcher(ctx workflow.Context) (activities.ScavengerReport, error) {
	sa := &activities.ScavengerActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	logger := workflow.GetLogger(ctx)

	var cfg activities.ScavengerConfig
	if err := workflow.ExecuteActivity(actCtx, sa.GetScavengerConfig).Get(ctx, &cfg); err != nil {
		return activities.ScavengerReport{}, err
	}

	var report activities.ScavengerReport

	for i := 0; i < cfg.MaxBatchesPerRun; i++ {
		var res activities.ReplicateBatchResult
		if err := workflow.ExecuteActivity(actCtx, sa.ReplicateLeaguesBatch, activities.ReplicateBatchParams{BatchSize: cfg.LeagueBatchSize}).Get(ctx, &res); err != nil {
			logger.Error("replicate leagues batch failed; stopping leagues for this run", "error", err)
			break
		}
		report.LeaguesReplicated += res.Replicated
		if res.Drained {
			break
		}
	}

	txnDrained := false
	for i := 0; i < cfg.MaxBatchesPerRun; i++ {
		var res activities.ReplicateBatchResult
		if err := workflow.ExecuteActivity(actCtx, sa.ReplicateTransactionsBatch, activities.ReplicateBatchParams{BatchSize: cfg.TxnBatchSize}).Get(ctx, &res); err != nil {
			logger.Error("replicate transactions batch failed; stopping transactions for this run", "error", err)
			break
		}
		report.TransactionsReplicated += res.Replicated
		if res.Drained {
			txnDrained = true
			break
		}
	}

	draftHeadersDrained := false
	for i := 0; i < cfg.MaxBatchesPerRun; i++ {
		var res activities.ReplicateBatchResult
		if err := workflow.ExecuteActivity(actCtx, sa.ReplicateDraftHeadersBatch, activities.ReplicateBatchParams{BatchSize: cfg.DraftBatchSize}).Get(ctx, &res); err != nil {
			logger.Error("replicate draft headers batch failed; stopping draft headers for this run", "error", err)
			break
		}
		report.DraftHeadersReplicated += res.Replicated
		if res.Drained {
			draftHeadersDrained = true
			break
		}
	}

	draftPicksDrained := false
	for i := 0; i < cfg.MaxBatchesPerRun; i++ {
		var res activities.ReplicateBatchResult
		if err := workflow.ExecuteActivity(actCtx, sa.ReplicateDraftPicksBatch, activities.ReplicateBatchParams{BatchSize: cfg.DraftBatchSize}).Get(ctx, &res); err != nil {
			logger.Error("replicate draft picks batch failed; stopping draft picks for this run", "error", err)
			break
		}
		report.DraftPicksReplicated += res.Replicated
		if res.Drained {
			draftPicksDrained = true
			break
		}
	}

	if cfg.PurgeEnabled && txnDrained {
		for i := 0; i < cfg.MaxBatchesPerRun; i++ {
			var res activities.PurgeBatchResult
			if err := workflow.ExecuteActivity(actCtx, sa.PurgeTransactionsBatch, activities.PurgeBatchParams{
				BatchSize: cfg.TxnBatchSize, RetentionDays: cfg.RetentionDays,
			}).Get(ctx, &res); err != nil {
				return report, err
			}
			report.TransactionsPurged += res.Purged
			report.TransactionsUnverified += res.Unverified
			if res.Drained {
				break
			}
		}
	}

	if cfg.PurgeEnabled && draftHeadersDrained && draftPicksDrained {
		for i := 0; i < cfg.MaxBatchesPerRun; i++ {
			var res activities.PurgeBatchResult
			if err := workflow.ExecuteActivity(actCtx, sa.PurgeDraftsBatch, activities.PurgeBatchParams{
				BatchSize: cfg.DraftBatchSize, RetentionDays: cfg.RetentionDays,
			}).Get(ctx, &res); err != nil {
				return report, err
			}
			report.DraftsPurged += res.Purged
			report.DraftsUnverified += res.Unverified
			if res.Drained {
				break
			}
		}
	}

	logger.Info("scavenger run complete", "leagues", report.LeaguesReplicated, "transactions", report.TransactionsReplicated,
		"draftHeaders", report.DraftHeadersReplicated, "draftPicks", report.DraftPicksReplicated,
		"transactionsPurged", report.TransactionsPurged, "transactionsUnverified", report.TransactionsUnverified,
		"draftsPurged", report.DraftsPurged, "draftsUnverified", report.DraftsUnverified)
	return report, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd backend && go test ./internal/workflows/... -run TestScavengerDispatcher -v`
Expected: PASS (all 6 `ScavengerDispatcher` tests, including the two pre-existing ones)

Then run the full backend suite:

Run: `cd backend && go build ./... && go vet ./... && go test ./...`
Expected: PASS (PG-gated tests skip cleanly if `TEST_DATABASE_URL` is unset; everything else runs)

- [ ] **Step 5: Update the spec's task table, commit, and open the PR**

In `docs/superpowers/plans/2026-07-07-two-database-archive.md`, change the T6 row's Status column from `Not started` to `In review — PR #<the PR number you just opened>` once the PR exists (mirrors how T3/T4/T5's rows were updated in their own PRs).

```bash
git add backend/internal/workflows/scavenger.go backend/internal/workflows/workflows_test.go
git commit -m "Wire the purge phase into ScavengerDispatcher, gated on PurgeEnabled + caught-up replication"
```

Then open the PR, and in a follow-up commit on the same branch, update the spec table row with the real PR number (`git add docs/superpowers/plans/2026-07-07-two-database-archive.md && git commit -m "Update T6 status in the two-database-archive spec"`).

---

## Manual / Integration Verification

- `go build ./... && go vet ./... && go test ./...` from `backend/` — full suite green.
- With `TEST_DATABASE_URL` set to a scratch Postgres 16 instance: `go test ./internal/activities/... ./internal/workflows/... ./internal/dbmigrate/... -v` — confirms the new purge activities, dispatcher wiring, and migration index all work against real Postgres (composite `IN` queries, chunked-transaction deletes, `CONCURRENTLY` index creation).
- Confirm `SCAVENGER_PURGE_ENABLED` is unset (or `false`) in every current deploy environment (worker host `.env` / systemd unit) — this task must not change production behavior. That's `env` inspection, not a code change; do it as a final sanity check before merging, not as a plan step.
- Do **not** flip `SCAVENGER_PURGE_ENABLED=true` anywhere as part of this task — enabling purge in production is T9's job, after T7 (ADP reads archive), T8 (backfill/parity checks), and T10 (verified backup) are done, per the spec's dependency table.
