# Sleeper Sync Operations (transactions + drafts)

User/league discovery moved off Temporal to a `cmd/cron`-driven job — see
`internal/discoverycron` and
`docs/superpowers/specs/2026-07-15-discovery-cron-migration-design.md` for
its tuning knobs (`CRON_DISCOVERY_*`), which are unrelated to the
dispatcher-based knobs below.

**Update (2026-07-20):** Transaction-sync has a second, `cmd/cron`-driven
path now too (`internal/transactioncron`, job name `transactions`), running
alongside `TransactionSyncDispatcher` — mirroring discovery's own migration.
Both claim through the exact same `sleeper_leagues.claimed_at` column via
`FOR UPDATE SKIP LOCKED`, so running them concurrently is safe by
construction; the cron path was added because the Temporal worker depends on
`ff-sims-worker.service` staying up on the worker host, and that's a single
point of failure this table's staleness has already hit once. See "How it
works" below for the cron path's tuning knobs (`CRON_TXN_*`) and cadence.
Draft-sync remains Temporal-only for now — this migration covers
transactions specifically, not the whole `cmd/worker` sync surface.

## Tuning knobs (env, per worker process)

The Sleeper client has no rate/concurrency-limiting env knob. It's a
process-wide singleton shared by draft-sync and transaction-sync (and, in a
separate process, the discovery cron job); an RPM-based token bucket and,
briefly, a concurrency semaphore were both tried and both let the
higher-volume sync pipelines starve other traffic out of its share.
Throughput is governed reactively instead — every 429 is logged
(`sleeper: 429 rate limited`), so a real problem surfaces in the worker
logs rather than needing a pre-guessed budget.

| Var | Default | Meaning |
|-----|---------|---------|
| `TXN_SYNC_PARALLEL_BATCHES` | 4 | Transaction claim→batch pipelines per dispatcher iteration. |
| `TXN_SYNC_BATCH_SIZE` | 250 | Leagues claimed per transaction batch activity. |
| `TXN_SYNC_LEAGUE_CONCURRENCY` | 12 | Goroutines syncing leagues inside one transaction batch activity. |
| `DRAFT_SYNC_PARALLEL_BATCHES` | 4 | Draft claim→batch pipelines per dispatcher iteration. |
| `DRAFT_SYNC_BATCH_SIZE` | 250 | Leagues claimed per draft batch activity. |
| `DRAFT_SYNC_LEAGUE_CONCURRENCY` | 12 | Goroutines syncing leagues inside one draft batch activity. |
| `WORKER_ACTIVITY_SLOTS` | 100 | Max concurrent activities on each sync queue (drafts, transactions) for this process. |
| `WORKER_ACTIVITY_POLLERS` | SDK default | Activity task pollers on each sync queue for this process; raise to win a larger share of queue tasks. |

Changing dispatcher knobs needs only a worker restart (they're read by the
`GetTransactionSyncConfig` / `GetDraftSyncConfig` activities each run, not
baked into workflow code).

Draft sync mirrors the transaction design on a separate claim column
(`drafts_claimed_at`), so the two paths never contend. Draft-specific
behavior: picks are fetch-once (completed drafts are immutable), and leagues
whose drafting is finished (`in_season`/`complete` with drafts fetched) leave
the claim pool entirely; `pre_draft`/`drafting` leagues recheck on cadence
until their drafts complete.

### Per-fleet vs global knobs

Task distribution is pull-based: the fleet with more free activity slots and
pollers takes more of the queue — relevant if this ever runs across more than
one worker process again. **Per-fleet** (each process reads its own env):
`WORKER_ACTIVITY_SLOTS`, `WORKER_ACTIVITY_POLLERS`,
`DB_MAX_OPEN_CONNS`. **Global** (read once per dispatcher run by whichever
worker executes the config activity): all `TXN_SYNC_*` knobs — do not use
them to differentiate fleets.

### Scaling up the worker host

The sync work is I/O-bound (the worker host idles under 10% CPU), so scale it
by raising its budgets in `/etc/ff-sims-worker.env` and restarting
`ff-sims-worker.service`:

```
WORKER_ACTIVITY_SLOTS=300
WORKER_ACTIVITY_POLLERS=10
DB_MAX_OPEN_CONNS=20
```

Also raise the global `TXN_SYNC_PARALLEL_BATCHES` (e.g. 8–12) so enough batch
activities are in flight for the worker host's extra slots to matter. Postgres
connections are the budget that bites first — route workers through the
DigitalOcean pgbouncer connection pool (port 25061, add
`default_query_exec_mode=simple_protocol` to the URL) before opening these
throttles.

## How it works

Every 10 minutes `TransactionSyncDispatcher` claims batches of stale leagues
(`claimed_at` + `FOR UPDATE SKIP LOCKED`, 20-minute claim TTL) and runs
`SyncLeagueTransactionsBatch` activities that stamp each league done as they
go. Only the worker host runs `cmd/worker` and polls this queue (DigitalOcean
serves the API only). The per-league leg loop is capped at the current NFL
week (past seasons still sweep legs 1–18).

### Cron path (`internal/transactioncron`)

`ff-sims-transactions.timer` runs `cron -job=transactions -max-duration=8m`
every 10 minutes (`OnUnitActiveSec=10min`, next run scheduled 10 minutes
after the previous one *finishes* — with an 8-minute deadline, overlap is
impossible by construction, same reasoning as `ff-sims-discovery.timer`).
`RunTransactionSync` runs a single claim-batch/process/refill pool (see
`internal/cronpool`, extracted from discoverycron's identical pool runner)
against `ClaimLeaguesForTransactions`/`SyncOneLeagueTransactions` — the exact
same activity code the Temporal dispatcher calls, just invoked per-item
instead of via `SyncLeagueTransactionsBatch`'s batch wrapper. Tuning knobs:

| Var | Default | Meaning |
|-----|---------|---------|
| `CRON_TXN_POOL_SIZE` | 8 | Max concurrent league-sync goroutines in one cron run. |
| `CRON_TXN_REFILL_BATCH` | 4 | Free pool slots required before claiming more. |

Logs: `journalctl -u ff-sims-transactions -f`. Unlike the Temporal path, a
crashed or killed cron run has nothing to restart — the next timer tick picks
up wherever claims expired, same as every other `cmd/cron` job.

Whether to eventually retire `TransactionSyncDispatcher` (as discovery's
Temporal path was, once its cron replacement proved reliable — see the
"Update (2026-07-19)" note in the discovery cron migration design doc) is a
follow-up decision, not part of this change.

## Rollout / verification

1. Apply migration 018: `cd backend && go run ./cmd/migrate up` (adds
   `claimed_at` + partial index; `CREATE INDEX CONCURRENTLY`, safe live).
2. Deploy the worker host (self-updates within minutes via its deploy timer
   and promotes the new deployment version on start — see the worker
   versioning docs).
3. Watch `/admin` fetch-age buckets: "Never fetched" and "24h+" should shrink
   visibly within hours at default settings (~4 × 250 leagues per claim wave).
4. Watch worker logs for `sleeper: 429 rate limited` — occasional, self-recovering occurrences are fine (that's the backoff working as intended); if it's persistent, that's a signal one of the sync pipelines needs its own scoped limit rather than a global one.

## Failure modes

- Worker dies mid-batch: its leagues stay claimed for 20 minutes, then
  re-queue. Heartbeat timeout (2m) retries the activity sooner; the retry
  re-processes only leagues that weren't already stamped.
- Sleeper state endpoint down: batches fall back to the full 18-leg sweep
  (slower, still correct).
- Claim query errors: dispatcher logs and exits; next scheduled run retries.

## Testing note

The claim-query tests (`claim_pg_test.go`) need real Postgres semantics
(`FOR UPDATE SKIP LOCKED`) and skip unless `TEST_DATABASE_URL` is set. CI runs
them against a postgres:16 service container.
