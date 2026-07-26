# Sleeper Sync Operations (transactions + drafts)

User/league discovery moved off Temporal to a `cmd/cron`-driven job — see
`internal/discoverycron` and
`docs/superpowers/specs/2026-07-15-discovery-cron-migration-design.md` for
its tuning knobs (`CRON_DISCOVERY_*`), which are unrelated to the knobs
below.

**Update (2026-07-25):** The Temporal transaction-sync path
(`TransactionSyncDispatcher`, `SyncLeagueTransactionsBatch`, the
`sleeper-transaction-sync-schedule` Temporal Schedule, and the
`sleeper-transactions` task queue worker) has been deleted —
`internal/transactioncron` (job name `transactions`) is now the sole
transaction-sync pipeline. It no longer shares any code or activity path
with `cmd/worker`; the "Tuning knobs" and "How it works" sections below
describe transaction-sync and draft-sync separately since they're now two
genuinely different mechanisms. Draft-sync remains Temporal-only.

## Tuning knobs (env, per worker process)

The Sleeper client has no rate/concurrency-limiting env knob. It's a
process-wide singleton shared by draft-sync (in `cmd/worker`) and, in
separate processes, the transaction-sync and discovery cron jobs
(`cmd/cron`). An RPM-based token bucket and, briefly, a concurrency
semaphore were both tried and both let the higher-volume sync pipelines
starve other traffic out of its share. Throughput is governed reactively
instead — every 429 is logged (`sleeper: 429 rate limited`), so a real
problem surfaces in the logs rather than needing a pre-guessed budget.

### Draft sync (Temporal, `cmd/worker`)

| Var | Default | Meaning |
|-----|---------|---------|
| `DRAFT_SYNC_PARALLEL_BATCHES` | 2 | Draft claim→batch pipelines per dispatcher iteration. |
| `DRAFT_SYNC_BATCH_SIZE` | 100 | Leagues claimed per draft batch activity. |
| `DRAFT_SYNC_LEAGUE_CONCURRENCY` | 8 | Goroutines syncing leagues inside one draft batch activity. |
| `WORKER_ACTIVITY_SLOTS` | 100 | Max concurrent activities on the drafts queue for this process. |
| `WORKER_ACTIVITY_POLLERS` | SDK default | Activity task pollers on the drafts queue for this process; raise to win a larger share of queue tasks. |

Changing these needs only a worker restart — they're read by the
`GetDraftSyncConfig` activity each run, not baked into workflow code.

Draft sync claims via its own `drafts_claimed_at` column (`FOR UPDATE SKIP
LOCKED`, 20-minute claim TTL), a separate column from transaction-sync's
`claimed_at` so the two never contend even though transaction-sync no
longer runs through `cmd/worker` at all. Draft-specific behavior: picks are
fetch-once (completed drafts are immutable), and leagues whose drafting is
finished (`in_season`/`complete` with drafts fetched) leave the claim pool
entirely; `pre_draft`/`drafting` leagues recheck on cadence until their
drafts complete.

#### Per-fleet vs global knobs

Task distribution is pull-based: the fleet with more free activity slots and
pollers takes more of the queue — relevant if this ever runs across more than
one worker process again. **Per-fleet** (each process reads its own env):
`WORKER_ACTIVITY_SLOTS`, `WORKER_ACTIVITY_POLLERS`, `DB_MAX_OPEN_CONNS`.
**Global** (read once per dispatcher run by whichever worker executes the
config activity): all `DRAFT_SYNC_*` knobs — do not use them to
differentiate fleets.

#### Scaling up the worker host

The sync work is I/O-bound (the worker host idles under 10% CPU), so scale it
by raising its budgets in `/etc/ff-sims-worker.env` and restarting
`ff-sims-worker.service`:

```
WORKER_ACTIVITY_SLOTS=300
WORKER_ACTIVITY_POLLERS=10
DB_MAX_OPEN_CONNS=20
```

Postgres connections are the budget that bites first — route workers through
the DigitalOcean pgbouncer connection pool (port 25061, add
`default_query_exec_mode=simple_protocol` to the URL) before opening these
throttles.

### Transaction sync (`internal/transactioncron`, cron-only)

| Var | Default | Meaning |
|-----|---------|---------|
| `CRON_TXN_POOL_SIZE` | 10 | Max concurrent league-fetch goroutines in one cron run. |
| `CRON_TXN_REFILL_BATCH` | 4 | Free pool slots required before claiming more. |
| `CRON_TXN_BATCH_SIZE` | 20 | Fetched league results accumulated before one bulk flush write. |
| `CRON_TXN_BATCH_FLUSH_INTERVAL_DURATION` | 5s | Flush accumulated results at least this often, even short of `CRON_TXN_BATCH_SIZE` (a Go duration string, e.g. `5s`, `500ms`). |

These take effect on cron's next invocation — no restart needed or possible,
since `cron -job=transactions` is a fresh process each timer tick, not a
long-running worker.

## How it works

### Draft sync (Temporal)

`DraftSyncDispatcher` claims batches of stale leagues (`drafts_claimed_at` +
`FOR UPDATE SKIP LOCKED`, 20-minute claim TTL) and runs
`SyncLeagueDraftsBatch` activities that stamp each league done as they go.
Only the worker host runs `cmd/worker` and polls this queue (DigitalOcean
serves the API only).

### Transaction sync (`internal/transactioncron`)

`ff-sims-transactions.timer` runs `cron -job=transactions -max-duration=8m`
every 10 minutes (`OnUnitActiveSec=10min`, next run scheduled 10 minutes
after the previous one *finishes* — with an 8-minute deadline, overlap is
impossible by construction, same reasoning as `ff-sims-discovery.timer`).

`RunTransactionSync` drives `internal/fdb.RunPool`: a claim/dispatch/batch
pool that claims stale leagues (`claimed_at` + `FOR UPDATE SKIP LOCKED`,
20-minute claim TTL), fetches each claimed league's transactions from
Sleeper concurrently (`FetchLeagueTransactions` — no DB writes), and
periodically flushes accumulated results as one bulk write per batch
(`FlushLeagueTransactions`) instead of one write per league. The per-league
leg loop is capped at the current NFL week (past seasons still sweep legs
1–18).

Logs: `journalctl -u ff-sims-transactions -f`. A crashed or killed cron run,
or a batch whose flush failed, has nothing to restart — the affected
leagues simply keep their claim until it expires, and the next timer tick
picks them up again.

## Verification

- Watch `/admin` fetch-age buckets: "Never fetched" and "24h+" should stay
  low relative to total league count at default settings.
- Watch logs for `sleeper: 429 rate limited` — occasional, self-recovering
  occurrences are fine (the backoff working as intended); persistent
  occurrences are a signal one of the sync pipelines needs its own scoped
  limit rather than a global one.
- `ff-sims-deploy.timer` self-updates the worker host within minutes of a
  merge to `main`; it only rebuilds/restarts `cmd/worker` or swaps in a new
  `cmd/cron` binary when that binary's actual Go dependency graph changed
  (via `go list -deps`), so an unrelated change elsewhere in the repo is a
  no-op deploy for both.

## Failure modes

**Draft sync (Temporal):** worker dies mid-batch — its leagues stay claimed
for 20 minutes, then re-queue; heartbeat timeout (10m) retries the activity
sooner, and the retry re-processes only leagues that weren't already
stamped.

**Transaction sync (cron):** a killed cron process or a failed batch flush
leaves the affected leagues' claims in place; they re-queue once the
20-minute claim TTL expires and the next timer tick claims them again — no
heartbeat or activity retry involved, since there's no long-running process
to detect a crash in.

**Both:** Sleeper state endpoint down — the leg loop falls back to the full
18-leg sweep (slower, still correct). Claim query errors — logged, and the
next scheduled run retries.

## Testing note

The claim-query tests need real Postgres semantics (`FOR UPDATE SKIP
LOCKED`) and skip unless `TEST_DATABASE_URL` is set: drafts' and users'
claims in `internal/activities/claim_pg_test.go`, transactions' claim in
`internal/transactioncron/claim_pg_test.go`. CI runs them against a
postgres:16 service container.
