# Archive Backfill Workflow (T8) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement T8 from `docs/superpowers/plans/2026-07-07-two-database-archive.md` — a one-time, manually-started workflow that catches the archive DB up on the ~12GB of pre-existing cloud history the 6h scavenger (T5) doesn't touch (it only replicates forward from wherever its cursors already are), plus the runbook to run it, monitor it, verify parity, and fall back to `pg_dump` if the WAN link to the archive host is too slow.

**Architecture:** `ArchiveBackfillWorkflow` reuses the exact same four `Replicate*Batch` activities `ScavengerDispatcher` (T5) already uses — backfilling and steady-state replication are the same copy operation, just run back-to-back instead of capped per 6h tick. The per-stream "loop batches until drained" logic is extracted out of `ScavengerDispatcher` into a shared `drainStream` helper so both workflows use identical, single-sourced logic. Unlike the scheduled dispatchers in this codebase (which bound their claim loop via `MaxDispatchIterations` and rely on the next scheduled tick to pick up any remainder), `ArchiveBackfillWorkflow` has no schedule to fall back on — it uses Temporal's `ContinueAsNew` to hand off to a fresh execution whenever a stream hasn't fully drained within one execution's batch cap, keeping any single execution's event history small regardless of backlog size. It's registered on the same `archive-maintenance` queue as the scavenger but is never put on a schedule — started once, manually, via the Temporal CLI.

**Tech Stack:** Go, Temporal Go SDK (`go.temporal.io/sdk` v1.45.0) — specifically `workflow.NewContinueAsNewError` / `workflow.IsContinueAsNewError`, verified against the vendored SDK source at `$(go env GOPATH)/pkg/mod/go.temporal.io/sdk@v1.45.0/workflow/workflow.go` and `internal/internal_workflow_testsuite.go`.

## Global Constraints

- No new activities, no new config surface, no new archive tables. This task is 100% workflow orchestration + docs, reusing T5's `ScavengerActivities`, `ReplicateBatchParams`/`ReplicateBatchResult`, `ScavengerConfig`, and the four `Replicate*Batch` activities completely unchanged.
- `ArchiveBackfillWorkflow` is **not** added to `schedules/register.go` — it's a one-time job, started manually per the runbook, not a recurring schedule.
- On a stream's activity failure: `ScavengerDispatcher` (existing T5 behavior, unchanged) logs and moves to the next stream, since the next 6h tick self-heals. `ArchiveBackfillWorkflow` does the opposite — it **fails the whole execution** rather than silently treating a broken stream as "drained." This is a one-time, manually-monitored job; silently reporting "backfill complete" while a stream actually errored out would risk a human believing the archive is fully caught up when it isn't. The shared `drainStream` helper returns the error to each caller and lets them decide — it doesn't hardcode either policy.
- Parity verification (`count(*)` / min-max `created_at` / sampled pick-count) is **runbook SQL, not new Go code** — matches the design doc's "S code / M ops" sizing for this task. Don't build a verification tool; document the exact queries.
- Confirmed via the vendored SDK source (see Tech Stack): `workflow.IsContinueAsNewError(err error) bool` is the correct, exported way to detect continue-as-new in both production code and `testsuite.TestWorkflowEnvironment`-based tests. When a top-level (non-child) test workflow returns a `ContinueAsNewError`, `env.IsWorkflowCompleted()` is `true` and `env.GetWorkflowError()` returns a wrapped error that `workflow.IsContinueAsNewError` correctly unwraps via `errors.As`.

---

## File Structure

| File | Responsibility |
|---|---|
| `backend/internal/workflows/scavenger.go` (modify) | Extract `drainStream` helper; refactor `ScavengerDispatcher` to use it (behavior unchanged — existing tests are the safety net) |
| `backend/internal/workflows/backfill.go` (new) | `ArchiveBackfillWorkflow` |
| `backend/internal/workflows/workflows_test.go` (modify) | New tests for `ArchiveBackfillWorkflow`; existing `ScavengerDispatcher` tests re-verified unchanged |
| `backend/cmd/worker/main.go` (modify) | Register `ArchiveBackfillWorkflow` on the existing archive worker (no schedule) |
| `docs/archive-backfill.md` (new) | Runbook: starting the backfill, monitoring, parity-check SQL, `pg_dump` escape hatch |

---

### Task 1: Extract `drainStream` helper, refactor `ScavengerDispatcher`

**Files:**
- Modify: `backend/internal/workflows/scavenger.go`

**Interfaces:**
- Produces: `drainStream(ctx, actCtx workflow.Context, activityFn interface{}, batchSize, maxBatches int) (replicated int, drained bool, err error)`. Consumed by the refactored `ScavengerDispatcher` (this task) and `ArchiveBackfillWorkflow` (Task 2).

This is a refactor: `ScavengerDispatcher`'s observable behavior must not change. The existing tests (`TestScavengerDispatcher_DrainsAllStreamsUntilShortBatch`, `TestScavengerDispatcher_StreamFailureDoesNotBlockOtherStreams`) are the safety net — no new tests are written in this task.

- [x] **Step 1: Add the helper**

In `backend/internal/workflows/scavenger.go`, add above `ScavengerDispatcher`:

```go
// drainStream runs up to maxBatches batches of a Replicate*Batch activity
// (activityFn — one of ScavengerActivities' four replicate methods),
// accumulating the replicated count. Returns once a batch reports Drained
// (the stream is caught up), the batch cap is hit (more work remains), or
// the activity errors. Callers decide what an error means for their own
// context — ScavengerDispatcher logs and moves on (the next 6h tick
// self-heals); ArchiveBackfillWorkflow fails the whole execution (it has no
// next tick to fall back on).
func drainStream(ctx, actCtx workflow.Context, activityFn interface{}, batchSize, maxBatches int) (replicated int, drained bool, err error) {
	for i := 0; i < maxBatches; i++ {
		var res activities.ReplicateBatchResult
		if err := workflow.ExecuteActivity(actCtx, activityFn, activities.ReplicateBatchParams{BatchSize: batchSize}).Get(ctx, &res); err != nil {
			return replicated, false, err
		}
		replicated += res.Replicated
		if res.Drained {
			return replicated, true, nil
		}
	}
	return replicated, false, nil
}
```

- [x] **Step 2: Rewrite `ScavengerDispatcher` to use it**

Replace the entire body of `ScavengerDispatcher` (the four duplicated `for i := 0; i < cfg.MaxBatchesPerRun; i++ { ... }` loops) with:

```go
func ScavengerDispatcher(ctx workflow.Context) (activities.ScavengerReport, error) {
	sa := &activities.ScavengerActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	logger := workflow.GetLogger(ctx)

	var cfg activities.ScavengerConfig
	if err := workflow.ExecuteActivity(actCtx, sa.GetScavengerConfig).Get(ctx, &cfg); err != nil {
		return activities.ScavengerReport{}, err
	}

	var report activities.ScavengerReport

	replicated, _, err := drainStream(ctx, actCtx, sa.ReplicateLeaguesBatch, cfg.LeagueBatchSize, cfg.MaxBatchesPerRun)
	if err != nil {
		logger.Error("replicate leagues batch failed; stopping leagues for this run", "error", err)
	}
	report.LeaguesReplicated = replicated

	replicated, _, err = drainStream(ctx, actCtx, sa.ReplicateTransactionsBatch, cfg.TxnBatchSize, cfg.MaxBatchesPerRun)
	if err != nil {
		logger.Error("replicate transactions batch failed; stopping transactions for this run", "error", err)
	}
	report.TransactionsReplicated = replicated

	replicated, _, err = drainStream(ctx, actCtx, sa.ReplicateDraftHeadersBatch, cfg.DraftBatchSize, cfg.MaxBatchesPerRun)
	if err != nil {
		logger.Error("replicate draft headers batch failed; stopping draft headers for this run", "error", err)
	}
	report.DraftHeadersReplicated = replicated

	replicated, _, err = drainStream(ctx, actCtx, sa.ReplicateDraftPicksBatch, cfg.DraftBatchSize, cfg.MaxBatchesPerRun)
	if err != nil {
		logger.Error("replicate draft picks batch failed; stopping draft picks for this run", "error", err)
	}
	report.DraftPicksReplicated = replicated

	logger.Info("scavenger run complete", "leagues", report.LeaguesReplicated, "transactions", report.TransactionsReplicated,
		"draftHeaders", report.DraftHeadersReplicated, "draftPicks", report.DraftPicksReplicated)
	return report, nil
}
```

The doc comment above `ScavengerDispatcher` (the one describing the claim-drain shape) stays as-is — it's still accurate.

- [x] **Step 3: Run the existing tests to verify the refactor didn't change behavior**

Run: `cd backend && go build ./... && go test ./internal/workflows/... -run TestScavengerDispatcher -v`
Expected: both `TestScavengerDispatcher_DrainsAllStreamsUntilShortBatch` and `TestScavengerDispatcher_StreamFailureDoesNotBlockOtherStreams` PASS, unchanged.

- [x] **Step 4: Commit**

```bash
git add internal/workflows/scavenger.go
git commit -m "refactor: extract drainStream helper from ScavengerDispatcher"
```

---

### Task 2: `ArchiveBackfillWorkflow`

**Files:**
- Create: `backend/internal/workflows/backfill.go`
- Modify: `backend/internal/workflows/workflows_test.go`

**Interfaces:**
- Consumes: `drainStream` (Task 1), `activities.ScavengerActivities`, `activities.ScavengerConfig`, `activities.ReplicateBatchParams`/`ReplicateBatchResult` (all from T5, unchanged).
- Produces: `workflows.ArchiveBackfillWorkflow(ctx workflow.Context) error`. Consumed by Task 3 (`cmd/worker` registration).

- [x] **Step 1: Write the failing tests**

Append to `workflows_test.go`:

```go
// ---- ArchiveBackfillWorkflow ----

func TestArchiveBackfillWorkflow_CompletesWhenAllStreamsDrainWithinOneExecution(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	cfg := activities.ScavengerConfig{LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)
	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{Replicated: 3, Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateTransactionsBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{Replicated: 10, Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftHeadersBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{Replicated: 2, Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftPicksBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{Replicated: 1, Drained: true}, nil).Once()

	env.ExecuteWorkflow(workflows.ArchiveBackfillWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestArchiveBackfillWorkflow_ContinuesAsNewWhenAStreamHitsTheBatchCap(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	cfg := activities.ScavengerConfig{LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)
	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	// Transactions never reports Drained within this execution — the "huge
	// backlog" case that must trigger ContinueAsNew.
	env.OnActivity(sa.ReplicateTransactionsBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{Replicated: 1, Drained: false}, nil)
	env.OnActivity(sa.ReplicateDraftHeadersBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftPicksBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()

	env.ExecuteWorkflow(workflows.ArchiveBackfillWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.True(t, workflow.IsContinueAsNewError(env.GetWorkflowError()))
	env.AssertExpectations(t)
}

func TestArchiveBackfillWorkflow_StreamFailureFailsTheExecution(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	cfg := activities.ScavengerConfig{LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)
	// Non-retryable so the mock isn't consumed by activity retries
	// (defaultActivityOptions allows 3 attempts).
	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{}, temporal.NewNonRetryableApplicationError("boom", "test", nil))

	env.ExecuteWorkflow(workflows.ArchiveBackfillWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	require.False(t, workflow.IsContinueAsNewError(env.GetWorkflowError()))
}
```

Add `"go.temporal.io/sdk/workflow"` to the import block if not already present (it isn't — `workflows_test.go` currently imports `"go.temporal.io/sdk/activity"`, `"go.temporal.io/sdk/temporal"`, and `"go.temporal.io/sdk/testsuite"`, but not `workflow`).

- [x] **Step 2: Run tests to verify they fail**

Run: `cd backend && go vet ./internal/workflows/...`
Expected: FAIL — `workflows.ArchiveBackfillWorkflow` undefined.

- [x] **Step 3: Implement**

```go
// backend/internal/workflows/backfill.go
package workflows

import (
	"fmt"

	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// backfillBatchesPerExecution bounds how many batches each stream drains
// within a single workflow execution before ContinueAsNew hands off to a
// fresh one. Unlike the scheduled dispatchers (which rely on the next
// scheduled tick to pick up any remainder — see MaxDispatchIterations),
// ArchiveBackfillWorkflow has no schedule to fall back on, so it must keep
// itself going; ContinueAsNew keeps any single execution's history small
// regardless of how large the backlog is.
const backfillBatchesPerExecution = 100

// ArchiveBackfillWorkflow is started once, manually, to catch the archive up
// on pre-existing cloud history — the scavenger's 6h schedule only
// replicates forward from wherever the cursors already are. Reuses the same
// four replicate activities and cursors as ScavengerDispatcher; it's the
// same copy operation, just run back-to-back until there's nothing left
// instead of capped per 6h tick. Unlike the scheduled scavenger, a stream's
// activity failure here fails the whole execution rather than being logged
// and skipped: this is a manually-monitored one-time job, and silently
// reporting "drained" while a stream is actually broken risks leaving data
// behind unnoticed.
func ArchiveBackfillWorkflow(ctx workflow.Context) error {
	sa := &activities.ScavengerActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	logger := workflow.GetLogger(ctx)

	var cfg activities.ScavengerConfig
	if err := workflow.ExecuteActivity(actCtx, sa.GetScavengerConfig).Get(ctx, &cfg); err != nil {
		return err
	}

	allDrained := true

	leaguesReplicated, leaguesDrained, err := drainStream(ctx, actCtx, sa.ReplicateLeaguesBatch, cfg.LeagueBatchSize, backfillBatchesPerExecution)
	if err != nil {
		return fmt.Errorf("replicate leagues: %w", err)
	}
	allDrained = allDrained && leaguesDrained

	txnReplicated, txnDrained, err := drainStream(ctx, actCtx, sa.ReplicateTransactionsBatch, cfg.TxnBatchSize, backfillBatchesPerExecution)
	if err != nil {
		return fmt.Errorf("replicate transactions: %w", err)
	}
	allDrained = allDrained && txnDrained

	headersReplicated, headersDrained, err := drainStream(ctx, actCtx, sa.ReplicateDraftHeadersBatch, cfg.DraftBatchSize, backfillBatchesPerExecution)
	if err != nil {
		return fmt.Errorf("replicate draft headers: %w", err)
	}
	allDrained = allDrained && headersDrained

	picksReplicated, picksDrained, err := drainStream(ctx, actCtx, sa.ReplicateDraftPicksBatch, cfg.DraftBatchSize, backfillBatchesPerExecution)
	if err != nil {
		return fmt.Errorf("replicate draft picks: %w", err)
	}
	allDrained = allDrained && picksDrained

	logger.Info("backfill execution complete", "leagues", leaguesReplicated, "transactions", txnReplicated,
		"draftHeaders", headersReplicated, "draftPicks", picksReplicated, "allDrained", allDrained)

	if !allDrained {
		return workflow.NewContinueAsNewError(ctx, ArchiveBackfillWorkflow)
	}
	logger.Info("archive backfill complete")
	return nil
}
```

- [x] **Step 4: Run tests to verify they pass**

Run: `cd backend && go vet ./internal/workflows/... && go test ./internal/workflows/... -run TestArchiveBackfillWorkflow -v`
Expected: all 3 PASS.

- [x] **Step 5: Run the full workflows package to confirm no regressions**

Run: `cd backend && go test ./internal/workflows/... -v 2>&1 | grep -E "^(--- |FAIL|PASS|ok)"`
Expected: every test PASSes, including the Task 1 refactor's `TestScavengerDispatcher_*` tests.

- [x] **Step 6: Commit**

```bash
git add internal/workflows/backfill.go internal/workflows/workflows_test.go
git commit -m "feat: add ArchiveBackfillWorkflow"
```

---

### Task 3: Register the workflow in `cmd/worker`

**Files:**
- Modify: `backend/cmd/worker/main.go`

**Interfaces:**
- Consumes: `workflows.ArchiveBackfillWorkflow` (Task 2).

No new automated test — `cmd/worker` is a `main` package (`[no test files]`, confirmed baseline), same as every prior change to this file. Verification is manual (Step 3).

- [x] **Step 1: Register the workflow**

In `backend/cmd/worker/main.go`, find the archive worker block (added in T5):

```go
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

Add one line:

```go
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
	}
```

`ArchiveBackfillWorkflow` is **not** added to `schedules/register.go` — no schedule for it, per the Global Constraints.

- [x] **Step 2: Build**

Run: `cd backend && go build ./...`
Expected: succeeds.

- [x] **Step 3: Manual verification**

Reuse the two-throwaway-database pattern from the T3/T5 plans (disposable Postgres on :5499, two databases `ffsims_cloud`/`ffsims_archive`, `ARCHIVE_DATABASE_URL` set) to boot the worker and confirm both workflow types register without error:

```bash
# (assumes a disposable Postgres is already running and the two databases +
# migrations are already set up per the T5 plan's Task 8 Step 4 — if not,
# repeat that setup here)
go build -o bin/backend-worker ./cmd/worker
DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_cloud?sslmode=disable" \
  ARCHIVE_DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_archive?sslmode=disable" \
  TEMPORAL_HOST="localhost:7233" \
  timeout 5 ./bin/backend-worker 2>&1 | head -20
rm -f bin/backend-worker
```
Expected: `Connected to archive database (...)` and no fatal error before the (expected, harmless) `temporal dial: ... connection refused` if no local Temporal dev server is running. If a local `temporal server start-dev` is available, additionally run:
```bash
temporal workflow start --task-queue archive-maintenance --type ArchiveBackfillWorkflow --workflow-id test-backfill
temporal workflow list
```
and confirm the workflow starts and executes (it will fail fast against empty throwaway DBs since `GetScavengerConfig` and the replicate activities need real `Cloud`/`Archive` handles wired via the worker process — the point of this check is confirming Temporal accepts and routes the workflow type to the archive-maintenance queue, not exercising real data).

- [x] **Step 4: Commit**

```bash
git add cmd/worker/main.go
git commit -m "feat: register ArchiveBackfillWorkflow on the archive-maintenance worker"
```

---

### Task 4: Runbook

**Files:**
- Create: `docs/archive-backfill.md`

**Interfaces:** none — documentation only.

- [x] **Step 1: Write the runbook**

```markdown
# Archive Backfill Runbook

`ArchiveBackfillWorkflow` copies all pre-existing cloud history into the
archive DB — the 6h scavenger schedule (`ScavengerDispatcher`) only
replicates forward from wherever its cursors already are, so this is what
actually moves the ~12GB backlog of transactions/drafts/picks that predates
the archive DB's existence. Run this **once**, after the archive DB is
provisioned (T1) and the scavenger is deployed (T5) — starting it before the
archive DB exists will just fail immediately on `GetScavengerConfig`.

It reuses the exact same replicate activities and cursors as the regular 6h
scavenger, so anything it copies is copied exactly the way the scavenger
would copy it going forward — there's no separate "backfill format."

## Starting it

From a machine with `temporal` CLI access to the worker's namespace:

    temporal workflow start \
      --task-queue archive-maintenance \
      --type ArchiveBackfillWorkflow \
      --workflow-id archive-backfill-initial

It will run to completion via a chain of `ContinueAsNew` executions (each
one picks up right where the last left off — the state lives in
`archive_sync_state`, not in the workflow itself, so `ContinueAsNew` loses
nothing). Each `ContinueAsNew` starts a new Run ID under the same Workflow
ID.

## Monitoring

    temporal workflow describe --workflow-id archive-backfill-initial

`RunId` changing between calls means it's still working (each
`ContinueAsNew` is a new run). `Status: COMPLETED` on the current run with
no further `ContinueAsNew` means it's done — check the worker logs for the
"archive backfill complete" line to confirm all four streams finished, not
just this execution:

    journalctl -u ff-sims-worker -f | grep -i backfill

If it fails (`Status: FAILED`), the worker logs will show which stream
errored (`replicate leagues: ...` / `replicate transactions: ...` / etc.) —
fix the underlying issue, then just re-run the `temporal workflow start`
command above with a new `--workflow-id`. Every replicate activity is
idempotent (upsert-on-conflict, cursor advance is atomic with the row
writes), so re-running from scratch or resuming mid-way is always safe —
nothing gets double-counted or corrupted.

## Verifying parity

After the workflow reports complete, run these against **both** databases
and compare:

    -- row counts (cloud vs. archive)
    SELECT count(*) FROM sleeper_leagues;
    SELECT count(*) FROM sleeper_transactions;
    SELECT count(*) FROM sleeper_drafts;
    SELECT count(*) FROM sleeper_draft_picks;

    -- date-range parity (transactions/drafts only — leagues/picks have no
    -- comparable range to check)
    SELECT min(created_at), max(created_at) FROM sleeper_transactions;
    SELECT min(created_at), max(created_at) FROM sleeper_drafts;

Archive counts should be `>=` cloud counts (cloud may already be slightly
ahead if new rows landed during the backfill — the scavenger's normal 6h
schedule will pick those up). If archive is *short* by more than a handful
of rows, something didn't finish — check the worker logs before concluding
the backfill is done.

Sampled pick-count parity (picks are the highest-volume, easiest-to-miss
table since they're batched per-draft rather than individually cursored):

    -- run on cloud, then the same query on archive, for the same 20 draft IDs
    SELECT sleeper_draft_id, count(*)
    FROM sleeper_draft_picks
    WHERE sleeper_draft_id IN (
      SELECT sleeper_draft_id FROM sleeper_drafts
      WHERE status = 'complete'
      ORDER BY random() LIMIT 20
    )
    GROUP BY sleeper_draft_id
    ORDER BY sleeper_draft_id;

Every draft ID should show the same pick count on both sides.

## Escape hatch: WAN too slow

If the workflow is taking unreasonably long — 12GB replicated row-by-row
over `Replicate*Batch`'s SQL round trips can be WAN-latency-bound if the
worker running it isn't co-located with both databases — seed the archive
directly instead, then let the (already-running) scavenger take over from
there. This requires matching the archive tables' exact column lists
(they're a subset of cloud's — see `backend/migrations/archive/002-005`),
so a plain `pg_dump --data-only` of the cloud tables won't load directly;
use `\copy` with explicit column lists instead.

On a machine with access to the **cloud** DB:

    psql "$DATABASE_URL" -c "\copy (SELECT sleeper_league_id, name, season, sport, status, total_rosters, ppr, te_premium, is_superflex, draft_type, league_type, scoring_settings, roster_positions, created_at, updated_at FROM sleeper_leagues) TO 'leagues.csv' WITH CSV"
    psql "$DATABASE_URL" -c "\copy (SELECT sleeper_transaction_id, sleeper_league_id, type, status, created_at_sleeper, leg, adds, drops, draft_picks, waiver_budget, created_at FROM sleeper_transactions) TO 'transactions.csv' WITH CSV"
    psql "$DATABASE_URL" -c "\copy (SELECT sleeper_draft_id, sleeper_league_id, type, status, season, last_fetched_at, created_at, updated_at FROM sleeper_drafts) TO 'drafts.csv' WITH CSV"
    psql "$DATABASE_URL" -c "\copy (SELECT sleeper_draft_id, round, pick_no, roster_id, picked_by_user_id, sleeper_player_id, metadata FROM sleeper_draft_picks) TO 'draft_picks.csv' WITH CSV"

Copy the four CSVs to the archive host (`scp`), then on a machine with
access to the **archive** DB (order matters — leagues and drafts before
draft_picks, though there are no FK constraints to enforce it, keeping it
in dependency order avoids any confusion when spot-checking):

    psql "$ARCHIVE_DATABASE_URL" -c "\copy sleeper_leagues (sleeper_league_id, name, season, sport, status, total_rosters, ppr, te_premium, is_superflex, draft_type, league_type, scoring_settings, roster_positions, created_at, updated_at) FROM 'leagues.csv' WITH CSV"
    psql "$ARCHIVE_DATABASE_URL" -c "\copy sleeper_transactions (sleeper_transaction_id, sleeper_league_id, type, status, created_at_sleeper, leg, adds, drops, draft_picks, waiver_budget, created_at) FROM 'transactions.csv' WITH CSV"
    psql "$ARCHIVE_DATABASE_URL" -c "\copy sleeper_drafts (sleeper_draft_id, sleeper_league_id, type, status, season, last_fetched_at, created_at, updated_at) FROM 'drafts.csv' WITH CSV"
    psql "$ARCHIVE_DATABASE_URL" -c "\copy sleeper_draft_picks (sleeper_draft_id, round, pick_no, roster_id, picked_by_user_id, sleeper_player_id, metadata) FROM 'draft_picks.csv' WITH CSV"

Then set every stream's cursor to "now" so the regular scavenger picks up
from here going forward instead of re-replicating what the CSV seed just
loaded (the cursor shape is `{"time": "<RFC3339>", "id": ""}` — an empty
`id` is fine, the next real row's `(timestamp, id)` will still sort after
it):

    psql "$ARCHIVE_DATABASE_URL" -c "
      INSERT INTO archive_sync_state (stream, cursor_state, updated_at)
      SELECT stream, jsonb_build_object('time', now(), 'id', ''), now()
      FROM (VALUES ('sleeper_leagues'), ('sleeper_transactions'), ('sleeper_drafts_headers'), ('sleeper_drafts_picks')) AS s(stream)
      ON CONFLICT (stream) DO UPDATE SET cursor_state = excluded.cursor_state, updated_at = excluded.updated_at;
    "

Then run the parity checks above to confirm the CSV seed landed correctly
before considering the backfill done.
```

- [x] **Step 2: Commit**

```bash
git add ../docs/archive-backfill.md
git commit -m "docs: add archive backfill runbook"
```

(Path is relative to `backend/` — adjust if running from the repo root.)

---

## Verification

- [x] `cd backend && go build ./...` and `go vet ./...` clean.
- [x] `cd backend && go test ./internal/workflows/... -v` — every test PASSes, including the Task 1 refactor's unchanged `ScavengerDispatcher` tests and Task 2's three new `ArchiveBackfillWorkflow` tests.
- [x] `cd backend && go test ./...` — full suite passes with no regressions (PG-gated tests SKIP without `TEST_DATABASE_URL`, PASS with it).
- [x] Task 3 Step 3's manual worker-boot check: both workflow types register on the `archive-maintenance` queue without error.
- [x] Runbook exists at `docs/archive-backfill.md` and its SQL/`\copy` column lists match `backend/migrations/archive/002-005` exactly (self-check: diff the column names in the runbook's `\copy` commands against the migration files' `CREATE TABLE` column lists).

## Self-Review

**Spec coverage:** T8's three named deliverables — "workflow," "runbook," "parity checks" — map to Tasks 1–3 (workflow, including the `drainStream` extraction it needed), and Task 4 (runbook, which includes the parity-check queries and the `pg_dump`/`\copy` escape hatch, both called out explicitly in the design doc).

**Placeholder scan:** no TBD/TODO markers. The `ContinueAsNew` test API (`workflow.IsContinueAsNewError`) was verified against the actual vendored SDK source before being written into the plan, not guessed.

**Type consistency:** `drainStream`'s signature (`ctx, actCtx workflow.Context, activityFn interface{}, batchSize, maxBatches int) (replicated int, drained bool, err error)`) matches between its Task 1 definition and both call sites (`ScavengerDispatcher` in Task 1, `ArchiveBackfillWorkflow` in Task 2). `ArchiveBackfillWorkflow`'s reliance on `activities.ScavengerActivities`/`ScavengerConfig`/`ReplicateBatchParams`/`ReplicateBatchResult` matches their T5 definitions unchanged — this task adds no new activity types.
