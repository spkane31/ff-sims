# Sleeper Sync Operations (transactions + drafts)

## Tuning knobs (env, per worker process)

| Var | Default | Meaning |
|-----|---------|---------|
| `SLEEPER_RPM` | 2000 | Sleeper API requests/minute budget for this process (per fleet IP). Start high, tune down if 429s appear in logs. |
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
pollers takes more of the queue. **Per-fleet** (each process reads its own
env): `SLEEPER_RPM`, `WORKER_ACTIVITY_SLOTS`, `WORKER_ACTIVITY_POLLERS`,
`DB_MAX_OPEN_CONNS`. **Global** (read once per dispatcher run by whichever
worker executes the config activity): all `TXN_SYNC_*` knobs — do not use
them to differentiate fleets.

### Scaling up the Raspberry Pi

The sync work is I/O-bound (the Pi idles under 10% CPU), so scale it by
raising its budgets in `/etc/ff-sims-worker.env` and restarting
`ff-sims-worker.service`:

```
WORKER_ACTIVITY_SLOTS=300
WORKER_ACTIVITY_POLLERS=10
SLEEPER_RPM=3000
DB_MAX_OPEN_CONNS=20
```

Also raise the global `TXN_SYNC_PARALLEL_BATCHES` (e.g. 8–12) so enough batch
activities are in flight for the Pi's extra slots to matter. Postgres
connections are the budget that bites first — route workers through the
DigitalOcean pgbouncer connection pool (port 25061, add
`default_query_exec_mode=simple_protocol` to the URL) before opening these
throttles.

## How it works

Every 10 minutes `TransactionSyncDispatcher` claims batches of stale leagues
(`claimed_at` + `FOR UPDATE SKIP LOCKED`, 20-minute claim TTL) and runs
`SyncLeagueTransactionsBatch` activities that stamp each league done as they
go. Both fleets (DigitalOcean + Raspberry Pi) poll the same queue and
partition work naturally via the claims. The per-league leg loop is capped at
the current NFL week (past seasons still sweep legs 1–18).

## Rollout / verification

1. Apply migration 018: `cd backend && go run ./cmd/migrate up` (adds
   `claimed_at` + partial index; `CREATE INDEX CONCURRENTLY`, safe live).
2. Deploy workers (DO promotes the new deployment version on start; Pi
   self-updates within minutes — see the worker versioning docs).
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

## Testing note

The claim-query tests (`claim_pg_test.go`) need real Postgres semantics
(`FOR UPDATE SKIP LOCKED`) and skip unless `TEST_DATABASE_URL` is set. CI runs
them against a postgres:16 service container.
