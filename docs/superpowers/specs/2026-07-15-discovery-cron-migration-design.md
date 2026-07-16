# Discovery Cron Migration: Replace Temporal for User/League Discovery

**Date:** 2026-07-15
**Status:** Approved

## Problem

`DiscoveryBatchDispatcher` (Temporal workflow) and `DiscoverUsersBatch` (Temporal
activity) have been the subject of four consecutive incident-response PRs (#174-177,
2026-07-13 to -15) and are still not reliably keeping the discovery queue drained. Each
fix uncovered a deeper layer, but a pattern emerged across all four: essentially none of
the debugging was about the actual business logic (claim a user, fetch their leagues,
write to the database) — it was `StartToCloseTimeout`, `HeartbeatTimeout`,
`WorkflowExecutionTimeout`, `MaxDispatchIterations`, schedule overlap policy, and
orphaned/overlapping-attempt semantics. Temporal's orchestration abstraction is costing
more than it returns for this specific workload shape.

The reason it's redundant here: durability and retry semantics are **already
database-native**. The claim-based design (`claimed_at` + `FOR UPDATE SKIP LOCKED` +
20-minute TTL, from the sync-throughput epic — see
`docs/superpowers/specs/2026-07-05-sleeper-sync-throughput-design.md`) already tolerates
a crashed worker: an in-flight claim simply expires and gets picked up again. Temporal's
durable-execution and automatic-retry machinery duplicates a resilience mechanism this
system already has at the database layer, while adding a large configuration surface
(five different timeout/retry knobs across two Temporal concepts) that has to be kept in
sync with the actual workload's timing characteristics — and hasn't been.

## Goals

- Replace `DiscoveryBatchDispatcher`/`DiscoverUsersBatch` with a plain Go process,
  triggered by a systemd timer, with a hard deadline shorter than its cadence so overlap
  is impossible by construction (not by policy).
- Preserve all the resilience the claim-based design already provides — no regression in
  crash tolerance, no new bookkeeping to reproduce what claim-expiry already does.
- Structurally eliminate the redundant-refetch problem: today, a shared league gets its
  members/details re-fetched once per member who happens to be discovered, because
  league work is inlined into user work. Decouple league work into its own
  independently-claimed queue so each league is fetched once, period.
- Leave everything else — draft-sync, transaction-sync, player-sync, week-stats,
  scavenger, ADP-rollup, and the ESPN Python worker — on Temporal, unchanged. This is a
  scoped migration for one pipeline, not a platform rewrite. (ESPN's own move to cron is
  a distinct, later effort.)
- Don't remove the existing Temporal discovery path yet. Both run concurrently against
  the same claim queue until the new path is proven; removing the old path is a
  follow-up decision made later, separately.

## Non-goals

- Migrating transaction-sync, draft-sync, or any other pipeline. `sleeper_leagues`
  transaction/draft fetching stays exactly as it is today.
- Deleting `workflows/dispatcher.go`, the `sleeper-discovery-schedule` Temporal
  Schedule, or the `sleeper-discovery` task queue registration in `cmd/worker/main.go`.
  All three keep running unchanged.
- A generic multi-job CLI framework. `cmd/cron` gets exactly the amount of structure
  needed to add a second job later without restructuring — not more.

## Design

### Architecture

A new `cmd/cron` binary: a small job runner. It takes a job name via a `-job` flag and a
`-max-duration` flag — both per-invocation parameters set in the systemd unit's
`ExecStart` line, not environment state — looks up the job in an in-process registry
(`map[string]func(ctx context.Context) error` or equivalent — no CLI framework needed
for one job), builds a deadline context (`context.WithTimeout(ctx, maxDuration)`), runs
the job, logs a final summary, and exits (non-zero on error, so systemd/journald reflect
failure). The registry is what makes adding draft-sync/transaction-sync/etc. later a
matter of registering another function under its own `-job=<name>`, not restructuring
the binary.

New systemd units on the worker host, mirroring the existing `ff-sims-deploy.timer`
pattern already there:

- `ff-sims-discovery.service` — `Type=oneshot`, `ExecStart=/path/to/cmd-cron -job=discovery -max-duration=50m`
- `ff-sims-discovery.timer` — `OnCalendar=hourly`

50 minutes on an hourly cadence guarantees a minimum 10-minute gap between runs even in
the worst case, so no overlap-policy is needed the way Temporal's schedule needed
`SCHEDULE_OVERLAP_POLICY_BUFFER_ONE`.

`cmd/worker` and Temporal Cloud are untouched. Both the old (Temporal) and new (cron)
discovery paths run concurrently against the same `sleeper_users`/`sleeper_leagues`
claim queues. This is safe by construction: `FOR UPDATE SKIP LOCKED` is explicitly
designed so independent claimers partition the queue without double-claiming — the
existing `ParallelBatches` mechanism inside the Temporal dispatcher already does the same
thing with itself. The cron path is just another claimer competing for the same
backlog. As a side effect of being a separate OS process with its own `sleeper.Client`,
the cron path also can never be starved by draft-sync/transaction-sync's request volume
the way the old shared-singleton client could be (see PR #177) — independent of whatever
that PR's throttling debate concluded.

### Data model change

New migration: `sleeper_leagues.discovery_claimed_at TIMESTAMPTZ NULL`, plus a partial
index mirroring the existing claim-query pattern (`WHERE discovery_claimed_at IS NULL OR
discovery_claimed_at < now() - interval '20 minutes'`, likely combined with the
`status <> 'complete' OR last_fetched_at IS NULL` exclusion — see claim query below).

A new column is required because `sleeper_leagues.claimed_at` is already used by
transaction-sync's `ClaimLeaguesForTransactions`, and `drafts_claimed_at` is already used
by draft-sync's `ClaimLeaguesForDrafts`. Reusing either would collide claim state between
unrelated pipelines sharing the same table.

`last_fetched_at` on `sleeper_leagues` keeps its current meaning (members + details
fetched — today set by `FetchLeagueDetails`); under the new design it becomes the
completion marker for discovery's league-work item specifically, analogous to how
`sleeper_users.last_fetched_at` marks user-work completion.

### Components

**Reused as-is** (already plain Go, no Temporal coupling):
- `FetchUserLeagues`, `FetchLeagueMembers`, `FetchLeagueDetails` (`internal/activities/discovery.go`)
- The existing `sleeper_users` claim query and claim/stamp pattern

**New, in a new `internal/discoverycron` package:**
- `ClaimStaleLeagues(ctx, batchSize) ([]string, error)` — mirrors `ClaimStaleUsers`, but
  against `sleeper_leagues.discovery_claimed_at`, excluding leagues already
  `status='complete' AND last_fetched_at IS NOT NULL` from the query itself (not
  claimed-then-skipped, unlike today's `leagueFullySynced` post-claim check).
- `ProcessUser(ctx, userID) error` — claim-scoped wrapper around `FetchUserLeagues`:
  fetch a user's leagues across seasons, upsert league rows (new leagues land with both
  `discovery_claimed_at` and `last_fetched_at` NULL, making them immediately eligible for
  league work) + junction rows, stamp the user done. No longer fetches members/details
  inline — that's the league pool's job now.
- `ProcessLeague(ctx, leagueID) error` — fetch members + details for one league, write
  both in a single DB transaction, stamp `last_fetched_at`, clear `discovery_claimed_at`.
- A generic pool runner (see Concurrency model) parameterized by a claim function and a
  process function, instantiated twice — once for users, once for leagues.
- `RunDiscovery(ctx context.Context, deps ...) (Report, error)` — the job entrypoint
  registered in `cmd/cron`'s registry: builds both pools, runs them concurrently until
  `ctx` is done, returns a summary.

**New `cmd/cron/main.go`**: parses `-job` and `-max-duration`, wires DB + a fresh
`sleeper.Client` (mirroring `cmd/worker/main.go`'s existing setup), builds the deadline
context, runs the job, logs a final summary.

### Concurrency model

Two independent pools (user pool, league pool), each running the same generic
claim-batch/process/refill-at-threshold loop, as two goroutines under the shared
top-level deadline:

```
for ctx not done:
    drain completed-work signals (non-blocking); track free slot count
    if free >= refillBatchSize:
        ids := claim(free)                      # one query, up to `free` items
        if len(ids) == 0: brief sleep; continue  # avoid busy-loop when queue's empty
        for each id: launch a goroutine bounded by
            min(time left until deadline, itemTimeoutSeconds)
    else:
        wait for next completion signal, or a short poll interval
wait for in-flight work to finish (bounded by their own sub-timeouts)
```

Both pools get independently env-configurable pool size and refill-batch size, starting
small (pool size 3-5, refill batch 1-2) and scalable up later without a code change. A
`CRON_DISCOVERY_*` prefix keeps these distinct from the existing `DISCOVERY_*` vars still
read by the Temporal path:

| Var | Starting default | Meaning |
|-----|-------------------|---------|
| `CRON_DISCOVERY_USER_POOL_SIZE` | 4 | Max concurrent user-work goroutines. |
| `CRON_DISCOVERY_USER_REFILL_BATCH` | 2 | Free user slots required before claiming more. |
| `CRON_DISCOVERY_USER_TIMEOUT_SECONDS` | 90 | Per-user sub-context bound (mirrors today's `DISCOVERY_USER_TIMEOUT_SECONDS`). |
| `CRON_DISCOVERY_LEAGUE_POOL_SIZE` | 4 | Max concurrent league-work goroutines. |
| `CRON_DISCOVERY_LEAGUE_REFILL_BATCH` | 2 | Free league slots required before claiming more. |
| `CRON_DISCOVERY_LEAGUE_TIMEOUT_SECONDS` | 30 | Per-league sub-context bound (one members call + one details call, no fan-out — should be much faster than a user's multi-league work). |

(Exact starting numbers are easy to revise at plan/implementation time; the point fixed
here is that they exist, are independent per pool, and are env-tunable without a
redeploy of orchestration code.)

### Error handling

- **Per-item failure or timeout:** logged, claim left in place. Naturally retried once
  its 20-minute TTL expires — either later in the same run (if it's still going) or by
  next hour's run. No batch-level retry-count bookkeeping, per the earlier decision to
  rely purely on claim expiry — it's the same recovery path a crashed worker already
  needs, so a separate retry mechanism would just be duplicating it.
- **Deadline reached:** top-level `ctx` cancels; both pools stop claiming new work;
  in-flight items wind down on their own sub-timeouts (already bounded well under the
  remaining time by construction); the job logs a final summary and exits 0 — this is
  the expected outcome most hours, not a failure.
- **Process crash:** no special handling. Same DB-native resilience as today — claims
  simply expire and the next hourly run (or the still-running Temporal path) picks them
  back up.

### Observability

Reuse the `discovery_trace` tag and log-line shapes from PR #176 (per-item duration,
per-run summary) — written to stdout/stderr, captured by journald under the new
`ff-sims-discovery.service` unit. `journalctl -u ff-sims-discovery` becomes the primary
debugging entry point for this path, separate from `ff-sims-worker`'s journal.

### Testing

- New Postgres-backed test for the league claim query (`discovery_claimed_at`),
  mirroring the existing `claim_pg_test.go` pattern (needs real `FOR UPDATE SKIP LOCKED`
  semantics, skips without `TEST_DATABASE_URL`).
- Pool-runner loop tested against fake claim/process functions and a controllable clock:
  refill only triggers at the configured threshold, the loop respects a deadline, in-flight
  work is drained (not abandoned) on shutdown, and an empty claim doesn't busy-loop.
- `FetchUserLeagues`/`FetchLeagueMembers`/`FetchLeagueDetails` tests reused unchanged —
  their behavior doesn't change, only who calls them and when.
- `cmd/cron`'s job registry and flag parsing get a thin test (unknown job name errors
  cleanly, deadline flag is honored).

### Deployment

1. New migration (`discovery_claimed_at` + partial index) — safe to apply live
   (`CREATE INDEX CONCURRENTLY`, matching the pattern used for prior claim-column
   migrations).
2. New `cmd/cron` binary added to the existing worker-host build/deploy pipeline
   (`deploy/worker-host/{deploy,setup}.sh`), alongside `cmd/worker`.
3. New systemd service + timer files, installed by the same setup script that installs
   `ff-sims-worker.service` today.
4. No Worker Deployment Versioning needed for `cmd/cron` — unlike `cmd/worker`, it's a
   one-shot process with nothing in-flight to preserve across a deploy, so a plain binary
   swap between runs is sufficient. This is a genuine simplification, not a gap.
5. Existing Temporal discovery schedule, task queue, and workflow code: untouched, no
   deploy action needed for them.
