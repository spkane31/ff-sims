# ADP Rollup Reads Archive (T7) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement T7 from `docs/superpowers/plans/2026-07-07-two-database-archive.md` — the ADP rollup reads its draft/pick/league join from the archive DB (which the scavenger, T5, already keeps as a full-history superset) instead of cloud, and continues writing the computed `draft_adp` rows to cloud (that table only ever lives in cloud — it's a small derived rollup, not part of what's archived). This is what makes ADP safe once purge (T6/T9) eventually trims cloud's draft history down to 30 days.

**Architecture:** `ADPRollupActivities` splits its single `DB *gorm.DB` field into `{Read, Write *gorm.DB}` — `Read` for the `sleeper_draft_picks`/`sleeper_drafts`/`sleeper_leagues` join query in both `ListADPSeasons` and `ComputeSegmentSeasonADP`, `Write` for the final `draft_adp` upsert. This mirrors the existing `ScavengerActivities{Cloud, Archive}` pattern (T5) and the design doc's own naming (`ADPRollupActivities{Read, Write}`). Since ADP now fundamentally depends on the archive DB to function, its entire worker/schedule is gated on `cfg.ArchiveDB.Enabled()` — the same pattern T5 already established for the archive-maintenance worker, extended here to a second, pre-existing queue (`sleeper-adp`). Wherever `ARCHIVE_DATABASE_URL` isn't set (local dev, or any environment without an archive DB), ADP rollup simply doesn't run — a deliberate behavior change, not an oversight.

**Tech Stack:** Go, GORM, Temporal Go SDK — no new dependencies.

## Global Constraints

- No archive schema changes, no new migrations — the archive already has everything `ListADPSeasons`/`ComputeSegmentSeasonADP` need (`sleeper_leagues`, `sleeper_drafts`, `sleeper_draft_picks`, all fully replicated by the T5 scavenger's `ReplicateLeaguesBatch`/`ReplicateDraftHeadersBatch`/`ReplicateDraftPicksBatch` — `sleeper_leagues` in particular is copied in full, not age-filtered, specifically because of this join dependency).
- `draft_adp` stays cloud-only. It's a small derived/computed rollup, not raw history — nothing about T7 touches where it's stored, only where its *inputs* are read from.
- **The whole ADP worker + schedule moves inside the `cfg.ArchiveDB.Enabled()` gate**, alongside the existing archive-maintenance worker. Without an archive DB, `database.Archive` is `nil` — constructing `ADPRollupActivities{Read: nil, ...}` and running it would panic the first time `a.Read.WithContext(...)` is called. Gating the whole worker (not adding nil-checks inside every activity method) matches how T5 handled the same problem for the scavenger, and is architecturally honest: ADP rollup's whole reason for existing in its post-T7 form is reading the archive, so there's nothing useful for it to do without one.
- `adpSelectClause`'s Postgres-dialect check (for the `PERCENTILE_CONT` 95% CI columns) must key off `a.Read.Dialector.Name()`, not `a.Write` — the dialect-sensitive query is the read-side join, not the write-side upsert.
- Existing SQLite-backed unit tests (`adp_rollup_test.go`) are the refactor safety net for the `{Read, Write}` split — update their construction to `{Read: db, Write: db}` (same DB for both, since they're not testing the cross-DB split itself) and confirm they still pass unchanged. New PG integration tests (two throwaway schemas, same pattern as T5/T8) are what actually prove the split works.

---

## File Structure

| File | Responsibility |
|---|---|
| `backend/internal/activities/adp_rollup.go` (modify) | `ADPRollupActivities{Read, Write}`; `ListADPSeasons`/`ComputeSegmentSeasonADP` read from `Read`, upsert to `Write` |
| `backend/internal/activities/adp_rollup_test.go` (modify) | Existing tests updated to `{Read: db, Write: db}`; new two-schema PG tests proving read/write separation |
| `backend/cmd/worker/main.go` (modify) | Move ADP worker construction inside the `cfg.ArchiveDB.Enabled()` block, `Read: database.Archive, Write: database.DB` |
| `backend/schedules/register.go` (modify) | Move the ADP schedule below the `archiveEnabled` gate, alongside the scavenger schedule |

---

### Task 1: Split `ADPRollupActivities` into `{Read, Write}`

**Files:**
- Modify: `backend/internal/activities/adp_rollup.go`
- Modify: `backend/internal/activities/adp_rollup_test.go`

**Interfaces:**
- Produces: `ADPRollupActivities{Read, Write *gorm.DB}`. Consumed by Task 2 (`cmd/worker`).

- [ ] **Step 1: Update the struct and both methods**

In `backend/internal/activities/adp_rollup.go`, change:

```go
// ADPRollupActivities holds dependencies for the daily ADP rollup worker.
type ADPRollupActivities struct {
	DB *gorm.DB
}
```

to:

```go
// ADPRollupActivities holds dependencies for the daily ADP rollup worker.
// Read is the archive DB (full draft/pick history — see the T5 scavenger);
// Write is cloud, where the small derived draft_adp rollup lives.
type ADPRollupActivities struct {
	Read  *gorm.DB
	Write *gorm.DB
}
```

Change `ListADPSeasons`:

```go
func (a *ADPRollupActivities) ListADPSeasons(ctx context.Context) ([]string, error) {
	var seasons []string
	err := a.Read.WithContext(ctx).
		Table("sleeper_drafts d").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id").
		Where("d.status = ? AND d.type IN ? AND l.league_type = ?", "complete", qualifyingDraftTypes, "redraft").
		Distinct("d.season").
		Pluck("d.season", &seasons).Error
	return seasons, err
}
```

Change `ComputeSegmentSeasonADP` (two call sites inside it — the read query's `a.DB` → `a.Read`, and the final upsert's `a.DB` → `a.Write`):

```go
func (a *ADPRollupActivities) ComputeSegmentSeasonADP(ctx context.Context, params ComputeSegmentSeasonADPParams) error {
	db := a.Read.WithContext(ctx).
		Table("sleeper_draft_picks p").
		Select(adpSelectClause(a.Read.Dialector.Name())).
		Joins("JOIN sleeper_drafts d ON d.sleeper_draft_id = p.sleeper_draft_id").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id").
		Where("d.status = ? AND d.type IN ? AND l.league_type = ? AND d.season = ?",
			"complete", qualifyingDraftTypes, "redraft", params.Season).
		Where("p.sleeper_player_id != ''")
	db = applySegmentPredicate(db, params.Segment)

	var rows []adpRow
	if err := db.Group("p.sleeper_player_id").Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	segmentKey := params.Segment.Key()
	records := make([]models.DraftADP, len(rows))
	for i, r := range rows {
		records[i] = models.DraftADP{
			Segment:         segmentKey,
			Season:          params.Season,
			SleeperPlayerID: r.SleeperPlayerID,
			AvgPickNo:       r.AvgPickNo,
			PickCount:       r.PickCount,
			MinPickNo:       r.MinPickNo,
			MaxPickNo:       r.MaxPickNo,
			CILowPickNo:     r.CILowPickNo,
			CIHighPickNo:    r.CIHighPickNo,
		}
	}

	return a.Write.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "segment"}, {Name: "season"}, {Name: "sleeper_player_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"avg_pick_no", "pick_count", "min_pick_no", "max_pick_no", "ci_low_pick_no", "ci_high_pick_no", "updated_at",
		}),
	}).CreateInBatches(&records, 500).Error
}
```

The rest of the file (`qualifyingDraftTypes`, `adpRow`, `baseADPSelect`, `postgresPercentileSelect`, `adpSelectClause`, `applySegmentPredicate`) is unchanged.

- [ ] **Step 2: Update the existing tests' construction**

In `backend/internal/activities/adp_rollup_test.go`, every `&activities.ADPRollupActivities{DB: db}` (6 occurrences, one per test function) becomes `&activities.ADPRollupActivities{Read: db, Write: db}`.

- [ ] **Step 3: Run the existing tests to verify they still pass**

Run: `cd backend && go build ./... && go test ./internal/activities/... -run TestListADPSeasons -v && go test ./internal/activities/... -run TestComputeSegmentSeasonADP -v`
Expected: all 7 tests (`TestListADPSeasons_ReturnsOnlyQualifyingSeasons` + the 6 `TestComputeSegmentSeasonADP_*`) PASS, unchanged behavior.

- [ ] **Step 4: Write the failing cross-DB tests**

Append to `adp_rollup_test.go`:

```go
// newADPCrossDBTest opens two throwaway PG schemas — read simulates the
// archive (leagues/drafts/picks), write simulates cloud (draft_adp only) —
// proving Read and Write are actually two different databases, not just two
// field names pointing at the same one.
func newADPCrossDBTest(t *testing.T) (read, write *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	readDSN := testutil.NewPGSchema(t, dsn, "adp_read")
	read = testutil.OpenGORM(t, readDSN)
	if err := read.AutoMigrate(&models.SleeperLeague{}, &models.SleeperDraft{}, &models.SleeperDraftPick{}); err != nil {
		t.Fatalf("automigrate read: %v", err)
	}

	writeDSN := testutil.NewPGSchema(t, dsn, "adp_write")
	write = testutil.OpenGORM(t, writeDSN)
	if err := write.AutoMigrate(&models.DraftADP{}); err != nil {
		t.Fatalf("automigrate write: %v", err)
	}

	return read, write
}

func TestListADPSeasons_ReadsFromArchiveOnly(t *testing.T) {
	read, write := newADPCrossDBTest(t)
	seedADPLeague(t, read, "lg1", 12, 1.0, true, "redraft")
	seedADPDraft(t, read, "d1", "lg1", "snake", "complete", "2024")

	a := &activities.ADPRollupActivities{Read: read, Write: write}
	seasons, err := a.ListADPSeasons(context.Background())
	if err != nil {
		t.Fatalf("ListADPSeasons: %v", err)
	}
	if len(seasons) != 1 || seasons[0] != "2024" {
		t.Errorf("expected [2024], got %v", seasons)
	}
}

func TestComputeSegmentSeasonADP_ReadsFromArchiveWritesToCloud(t *testing.T) {
	read, write := newADPCrossDBTest(t)
	seedADPLeague(t, read, "lg1", 12, 1.0, true, "redraft")
	seedADPDraft(t, read, "d1", "lg1", "snake", "complete", "2024")
	seedADPPick(t, read, "d1", 1, 1, "p1")
	seedADPPick(t, read, "d1", 1, 2, "p2")

	a := &activities.ADPRollupActivities{Read: read, Write: write}
	if err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	}); err != nil {
		t.Fatalf("ComputeSegmentSeasonADP: %v", err)
	}

	var writeCount int64
	write.Model(&models.DraftADP{}).Count(&writeCount)
	if writeCount != 2 {
		t.Errorf("expected 2 draft_adp rows in write DB, got %d", writeCount)
	}

	// Read DB never gets a draft_adp table touched — draft_adp is cloud-only.
	var readTableExists bool
	read.Raw("SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'draft_adp')").Scan(&readTableExists)
	if readTableExists {
		t.Error("expected no draft_adp table in the read (archive) DB")
	}
}
```

Add `"os"` to the import block, and `"backend/internal/dbmigrate"`... actually no — this test doesn't need `dbmigrate` (it AutoMigrates directly, not via goose), so only add `"os"` and `"backend/internal/testutil"` to the existing `import` block in `adp_rollup_test.go`.

- [ ] **Step 5: Run the new tests**

`Read`/`Write` already exist on `ADPRollupActivities` as of Step 1, so this compiles immediately — there's no red step here the way there is for a from-scratch activity; run directly (with a disposable Postgres — `initdb`/`pg_ctl` on port 5499, `TEST_DATABASE_URL` set):

`cd backend && go test ./internal/activities/... -run "TestListADPSeasons_ReadsFromArchiveOnly|TestComputeSegmentSeasonADP_ReadsFromArchiveWritesToCloud" -v`
Expected: both PASS.

- [ ] **Step 6: Run the full activities package for regressions**

Run: `cd backend && go test ./internal/activities/... -v 2>&1 | grep -E "^(--- |FAIL|PASS|ok)"`
Expected: everything PASSes.

- [ ] **Step 7: Commit**

```bash
git add internal/activities/adp_rollup.go internal/activities/adp_rollup_test.go
git commit -m "feat: split ADPRollupActivities into {Read, Write} — read archive, write cloud"
```

---

### Task 2: Gate the ADP worker + schedule on archive being enabled

**Files:**
- Modify: `backend/cmd/worker/main.go`
- Modify: `backend/schedules/register.go`

**Interfaces:**
- Consumes: `ADPRollupActivities{Read, Write}` (Task 1), `cfg.ArchiveDB.Enabled()`/`database.Archive` (existing, T3).

No new automated tests — both files are already untested (`cmd/worker` is `main`, `schedules` has `[no test files]`), same as every prior change to them across T3/T5/T8. Verification is manual (Step 3).

- [ ] **Step 1: Move the ADP worker into the archive-enabled block in `cmd/worker/main.go`**

Remove line 100 (`aa := &activities.ADPRollupActivities{DB: database.DB}`) from its current spot in the unconditional activity-construction block (alongside `da`, `dfa`, `psa`, `wsa`).

Remove the entire unconditional ADP worker block (currently lines 157–166):

```go
	// ADP worker: ADPRollupDispatcher + SegmentSeasonADPRollupWorkflow
	adpw := worker.New(c, workflows.TaskQueueADP, worker.Options{
		MaxConcurrentActivityExecutionSize: 50,
		MaxConcurrentWorkflowTaskPollers:   10,
		DeploymentOptions:                  deploymentOpts,
		SysInfoProvider:                    sysinfo.SysInfoProvider(),
	})
	adpw.RegisterWorkflow(workflows.ADPRollupDispatcher)
	adpw.RegisterWorkflow(workflows.SegmentSeasonADPRollupWorkflow)
	adpw.RegisterActivity(aa)
```

Remove `adpw` from the `workers := []worker.Worker{dw, draftsw, transactionsw, psw, wsw, adpw}` literal — it becomes:

```go
	workers := []worker.Worker{dw, draftsw, transactionsw, psw, wsw}
```

Then add the ADP worker construction into the existing `if cfg.ArchiveDB.Enabled() { ... }` block, alongside the archive-maintenance worker:

```go
	workers := []worker.Worker{dw, draftsw, transactionsw, psw, wsw}
	if cfg.ArchiveDB.Enabled() {
		sa := &activities.ScavengerActivities{Cloud: database.DB, Archive: database.Archive}
		aw := worker.New(c, workflows.TaskQueueArchive, worker.Options{
			DeploymentOptions: deploymentOpts,
			SysInfoProvider:   sysinfo.SysInfoProvider(),
		})
		aw.RegisterWorkflow(workflows.ScavengerDispatcher)
		aw.RegisterWorkflow(workflows.ArchiveBackfillWorkflow)
		aw.RegisterActivity(sa)
		workers = append(workers, aw)

		// ADP worker: ADPRollupDispatcher + SegmentSeasonADPRollupWorkflow.
		// Requires the archive DB (Read) — see ADPRollupActivities.
		aa := &activities.ADPRollupActivities{Read: database.Archive, Write: database.DB}
		adpw := worker.New(c, workflows.TaskQueueADP, worker.Options{
			MaxConcurrentActivityExecutionSize: 50,
			MaxConcurrentWorkflowTaskPollers:   10,
			DeploymentOptions:                  deploymentOpts,
			SysInfoProvider:                    sysinfo.SysInfoProvider(),
		})
		adpw.RegisterWorkflow(workflows.ADPRollupDispatcher)
		adpw.RegisterWorkflow(workflows.SegmentSeasonADPRollupWorkflow)
		adpw.RegisterActivity(aa)
		workers = append(workers, adpw)
	}
```

- [ ] **Step 2: Gate the ADP schedule the same way in `schedules/register.go`**

Move the `"sleeper-adp-rollup-schedule"` `upsert(...)` block (currently unconditional, immediately before the `if !archiveEnabled { return nil }` check) to *after* that check, alongside the scavenger schedule:

```go
	if !archiveEnabled {
		return nil
	}

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

Update the doc comment on `Register` (currently says `archiveEnabled` gates only "the scavenger schedule") to reflect it now gates two schedules:

```go
// Register creates the Temporal schedules for the Sleeper workers. If a
// schedule already exists it is left unchanged (idempotent). archiveEnabled
// gates the ADP rollup and scavenger schedules — registering either when no
// worker polls their queue would just be a schedule that fires and returns
// a "no worker available" fail, forever, on a queue nobody's listening to.
func Register(ctx context.Context, c client.Client, archiveEnabled bool) error {
```

- [ ] **Step 3: Build, then manually verify both archive-enabled and archive-disabled paths**

Run: `cd backend && go build ./...`
Expected: succeeds.

Reuse the two-throwaway-database + local Temporal dev server setup from the T8 plan's Task 3 Step 3 (disposable Postgres on :5499, `ffsims_cloud`/`ffsims_archive`, `temporal server start-dev --headless`, `temporal worker deployment set-current-version` to promote the `dev` build so pollers actually receive tasks):

```bash
# Archive disabled: confirm no ADP worker starts, no panic.
DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_cloud?sslmode=disable" timeout 5 ./bin/backend-worker 2>&1 | grep -i "adp\|archive"
```
Expected: only `ARCHIVE_DATABASE_URL not set — archive database disabled` — no `Started Worker ... TaskQueue sleeper-adp` line, no panic.

```bash
# Archive enabled: confirm the ADP worker starts.
DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_cloud?sslmode=disable" \
  ARCHIVE_DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_archive?sslmode=disable" \
  timeout 5 ./bin/backend-worker 2>&1 | grep -i "adp\|archive"
```
Expected: `Connected to archive database (...)` and `Started Worker Namespace default TaskQueue sleeper-adp ...`.

If a local Temporal dev server is available (per T8's verification), go further: start it, promote the deployment version, and confirm `temporal task-queue describe --task-queue sleeper-adp` shows a poller once the worker is running with archive enabled.

- [ ] **Step 4: Run the full backend test suite**

Run: `cd backend && go test ./... -v 2>&1 | tail -60` (with `TEST_DATABASE_URL` set for the full PG-gated suite)
Expected: everything PASSes, no regressions.

- [ ] **Step 5: Commit**

```bash
git add cmd/worker/main.go schedules/register.go
git commit -m "feat: gate ADP rollup worker + schedule on archive DB being enabled"
```

---

## Verification

- [ ] `cd backend && go build ./...` and `go vet ./...` clean.
- [ ] `cd backend && go test ./...` passes with `TEST_DATABASE_URL` unset (PG-gated tests SKIP, nothing FAILs).
- [ ] Full pass with a disposable Postgres: every test PASSes, including the two new cross-DB ADP tests and the 7 pre-existing (now `{Read, Write}`-constructed) SQLite tests.
- [ ] Task 2 Step 3's manual checks: archive-disabled boot has no `sleeper-adp` worker and doesn't panic; archive-enabled boot starts it and (if a local Temporal server is available) actually receives polled tasks.

## Self-Review

**Spec coverage:** T7's stated deliverable — `ADPRollupActivities{Read, Write}` reading archive, writing cloud — is Task 1. The design doc doesn't explicitly call out gating the ADP worker/schedule on archive availability, but it's a direct, necessary consequence of `Read` now requiring a non-nil archive handle — Task 2 makes that consequence explicit and tested rather than leaving it as an implicit nil-pointer landmine.

**Placeholder scan:** no TBD/TODO markers; every step has literal code or an exact command with expected output.

**Type consistency:** `ADPRollupActivities{Read, Write *gorm.DB}` matches between Task 1's definition, its two methods, the updated existing tests, the new cross-DB tests, and Task 2's `cmd/worker` construction (`Read: database.Archive, Write: database.DB`).
