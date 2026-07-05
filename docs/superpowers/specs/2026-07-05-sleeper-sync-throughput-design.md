# Sleeper Sync Throughput: Resilience, Worker Versioning, and Claim-Based Batch Activities

**Date:** 2026-07-05
**Status:** Approved
**Epic:** 5 issues — issues 1 and 2 are prerequisites and land first; issue 3 is the core redesign; issues 4 and 5 are follow-ups inside the epic.

## Problem

The transaction/draft sync pipeline cannot keep 2026 leagues fresh:

- `TransactionSyncDispatcher` runs every 5 minutes and dispatches `SyncBatchSize = 150`
  per-league child workflows — a hard ceiling of ~43k league-syncs/day.
- The 2026 backlog is ~44k never-fetched leagues plus ~20k more than 24h stale, and the
  steady-state goal (all active 2026 leagues refreshed every 3–6h to feed the player
  valuation model) requires ~300–600k league-syncs/day: roughly **10x** current capacity.
- Workers are mostly idle (`MaxConcurrentActivityExecutionSize: 100` on two fleets — the
  DigitalOcean app and the Raspberry Pi). The dispatcher, not worker capacity or the
  Sleeper API, is the bottleneck. No 429 rate limiting has been observed.
- Per-league child workflows cost ~8 Temporal actions per league sync. At the required
  volume that is millions of actions/day — free on Temporal staging, but the design must
  stay portable to a paid Temporal Cloud account or a simple self-hosted server.
- `FetchLeagueTransactions` always loops legs up to 18. In the 2026 offseason only leg 1
  exists, so a never-fetched league burns up to 18 HTTP calls (mostly 404s) to retrieve
  one leg of data.

Two reliability gaps become much more dangerous once batches replace per-league work
units, so they are fixed first:

1. **Sleeper client has no general retry.** Only 429s retry inside `client.get`;
   transport errors (`context deadline exceeded`, connection reset) fail the activity
   immediately. A single flaky request would fail an entire 250-league batch activity.
2. **No worker versioning.** Both fleets poll the same task queues and deploy at
   different times (the Pi self-updates minutes after DigitalOcean). A failed or lagging
   Pi deploy leaves stale code polling shared queues → non-determinism errors and failed
   batches.

## Goals

- All active 2026 leagues' transactions refreshed on a 3–6h cadence in-season.
- Clear the 2025–2026 transaction and draft backlogs. Pre-2025 seasons are out of scope.
- Action-frugal: a full sweep of ~74k leagues should cost thousands of Temporal actions,
  not hundreds of thousands.
- Safe independent deploys of the two worker fleets.

## Design

### Issue 1 — Sleeper client retry policy (prerequisite)

Rework `backend/internal/sleeper/client.go` `(*Client).get` into a unified retry loop.

**Retryable** (with backoff, up to ~6 attempts):
- Transport errors from `c.http.Do` — timeouts, connection resets, EOF — but **only when
  the parent context is still alive** (`ctx.Err() == nil`). If the parent context is
  canceled or past its deadline (activity cancellation, heartbeat timeout), return
  immediately; never fight Temporal's cancellation.
- HTTP 429 — honor a `Retry-After` header when present (seconds or HTTP-date), otherwise
  exponential backoff.
- HTTP 5xx.

**Non-retryable:**
- 404 → `NotFoundError` (unchanged).
- Any other 4xx → immediate error.

**Backoff:** exponential with **full jitter** (`rand.Float64() * min(cap, base*2^attempt)`),
base ~500ms, cap 30s. Jitter is required: once 100+ concurrent fetches hit a 429 storm,
non-jittered clients retry in lockstep and re-trigger the limit.

**Bug fix:** `defer resp.Body.Close()` currently sits inside the retry loop, so response
bodies accumulate until the function returns. Close the body at the end of each
iteration (drain + close before retrying so connections are reused).

The Temporal activity retry policy stays as the outer layer; the client absorbs blips so
activity retries are reserved for real outages.

**Tests** (`httptest` with scripted response sequences): 429-then-200 succeeds;
`Retry-After` honored; timeout-then-200 succeeds; 6×500 exhausts and errors; 404 returns
`NotFoundError` without retry; canceled parent context returns promptly without retry.

### Issue 2 — Worker deployment versioning (prerequisite)

Use Temporal Worker Deployment Versioning so each fleet only executes workflows pinned
to the build it is running.

- All six workers in `backend/cmd/worker/main.go` get:
  ```go
  DeploymentOptions: worker.DeploymentOptions{
      UseVersioning: true,
      Version: worker.WorkerDeploymentVersion{
          DeploymentName: "ff-sims-worker",
          BuildId:        buildID, // git SHA injected via -ldflags
      },
      DefaultVersioningBehavior: workflow.VersioningBehaviorPinned,
  }
  ```
- **Build ID = git commit SHA**, injected at build time via
  `-ldflags "-X main.buildID=$(git rev-parse --short HEAD)"` in both the DigitalOcean
  build and the Pi self-update build (see `c827bde` for the Pi deploy mechanism). Both
  fleets built from the same SHA report the same version and share work.
- **Pinned** behavior: all workflows here are short-lived (seconds to minutes), so a
  workflow started on version N completes on version N. A Pi stuck on an old build after
  a failed deploy keeps serving its pinned workflows and receives no new-version work —
  no NDEs, no failed batches.
- **Promotion:** a new version must be set "current" before new workflow executions
  route to it. Mechanism: env flag `TEMPORAL_PROMOTE_ON_START=true` set **only on the
  DigitalOcean worker**; on startup it calls the deployment API
  (`client.WorkerDeploymentClient`) to set its own version current, retrying briefly
  since the worker must register the version first. The Pi never promotes; it joins
  whatever version it built.
- **Stranded-workflow edge case:** if a rolling deploy briefly leaves a pinned workflow
  with no old-version worker, it waits until one returns or it is terminated. Acceptable:
  workflows are short, and once issue 3 lands, claim expiry re-queues the underlying data
  work regardless. Document `temporal worker deployment describe / set-current-version`
  commands in the repo for inspecting and draining versions.

### Issue 3 — Claim-based batch transaction sync (core)

Replace per-league child workflows with batch activities that claim work in Postgres.

**Schema:** add `claimed_at timestamptz NULL` to `sleeper_leagues`, plus a partial index
aligned with the stale-transactions predicate.

**Claim activity** — `ClaimLeaguesForTransactions(batchSize)`:
```sql
UPDATE sleeper_leagues SET claimed_at = now()
WHERE sleeper_league_id IN (
    SELECT sleeper_league_id FROM sleeper_leagues
    WHERE skipped_at IS NULL AND last_fetched_at IS NOT NULL AND season >= '2025'
      AND NOT (status = 'complete' AND last_transactions_fetched_at IS NOT NULL)
      AND (claimed_at IS NULL OR claimed_at < now() - interval '20 minutes')
    ORDER BY CASE WHEN last_transactions_fetched_at IS NULL THEN 0 ELSE 1 END,
             last_transactions_fetched_at ASC
    LIMIT @batch_size
    FOR UPDATE SKIP LOCKED
)
RETURNING sleeper_league_id, last_transaction_leg_fetched;
```
Atomic across both fleets; a dead worker's claims expire after 20 minutes.

**Batch activity** — `SyncLeagueTransactionsBatch(leagues)`:
- ~250 leagues per batch, processed with bounded in-activity concurrency (8–16
  goroutines), heartbeating progress every few leagues.
- One `GetNFLState` call per batch; the per-league leg loop runs from the cursor
  (`max(lastLeg-1, 1)`) **capped at the current NFL week** instead of 18.
- Per league on success: upsert transactions, stamp `last_transactions_fetched_at`,
  advance `last_transaction_leg_fetched`, and **clear `claimed_at`** in one update. An
  activity retry after a crash re-processes only unstamped leagues.
- Per-league errors are recorded (log + failure counter in the heartbeat details /
  activity result) and skipped — they never fail the batch. The league's claim expiry
  re-queues it naturally.

**Dispatcher** — `TransactionSyncDispatcher` becomes: fan out K batch activities in
parallel (K from env, default 4); while claims return full batches, claim and fan out
again; exit when a claim returns short (backlog drained). Schedule stays every 5 minutes
with overlap-skip. Throughput at K=4 × 250 leagues × ~2 calls/league comfortably exceeds
the 600k/day target.

**Rate limiter:** `golang.org/x/time/rate` limiter inside the Sleeper client, budget from
env **`SLEEPER_RPM` (default 2000)** — start high, tune down. Per process, so each fleet
independently respects its own IP's Sleeper limit.

**Cleanup:** delete `LeagueTransactionSyncWorkflow` and the per-league child-workflow
dispatch (worker versioning from issue 2 covers the transition; old pinned executions
drain on the old version).

**Action cost:** a full 74k-league sweep drops from ~600k actions to ~2k (heartbeats are
not billed actions).

### Issue 4 — Claim-based batch draft sync (follow-up)

Apply the issue 3 pattern to drafts: `claimed_drafts_at` (or reuse `claimed_at` with a
separate column — decide in implementation; separate column preferred so transaction and
draft claims don't contend), claim query mirroring `GetStaleLeaguesForDrafts`, batch
activity fetching `GetLeagueDrafts` + `GetDraftPicks` for completed drafts, per-league
stamping of `last_drafts_fetched_at`. Completed drafts are fetch-once; pre-draft leagues
recheck until complete. Delete `LeagueDraftSyncWorkflow`.

### Issue 5 — Adaptive refresh cadence (follow-up)

- Store `latest_transaction_at timestamptz` on `sleeper_leagues`, derived from the max
  `created` of fetched transactions in the batch activity.
- Staleness ordering/eligibility becomes activity-aware: leagues with a transaction in
  the last ~2 weeks refresh every ~3h; dormant leagues decay toward every 24h.
- This halves (or better) steady-state fetch volume in-season and is what makes the
  3–6h valuation recompute cadence cheap.

## Deleted/replaced code

- `backend/internal/workflows/transaction_sync.go`: per-league child workflow dispatch →
  batch fan-out (issue 3).
- `backend/internal/workflows/draft_sync.go`: same (issue 4).
- `GetStaleLeaguesForTransactions` / `GetStaleLeaguesForDrafts` are replaced by the
  claiming activities.

## Testing

- Issue 1: `httptest`-scripted retry tests (see above).
- Issue 3/4: claim-query tests need Postgres semantics (`FOR UPDATE SKIP LOCKED`); the
  existing test setup uses SQLite for some rollup tests, so claim tests either run
  against Postgres in CI or the claim SQL is exercised via a tagged integration test.
  Batch activity logic (leg capping, cursor advance, per-league error isolation) is unit
  tested with the fake Sleeper server.
- Issue 2: verified by deploying to staging: start worker vN+1 on DO with promote-on-start,
  confirm Pi on vN keeps completing pinned workflows and new executions land on vN+1.

## Rollout order

1. Issue 1 (client retry) — safe, immediate win for current activities too.
2. Issue 2 (versioning) — must be live before issue 3's workflow-shape change deploys.
3. Issue 3 (transactions batching) — the throughput fix.
4. Issue 4 (drafts batching).
5. Issue 5 (adaptive cadence) — before the season ramps (~August 2026).
