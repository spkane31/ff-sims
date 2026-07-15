# Sleeper Sync Operations (discovery + transactions + drafts)

## Tuning knobs (env, per worker process)

| Var | Default | Meaning |
|-----|---------|---------|
| `SLEEPER_MAX_CONCURRENT_REQUESTS` | 50 | Max simultaneous in-flight Sleeper requests for this process (per fleet IP). Not a throughput target — throughput is governed reactively via 429 `Retry-After`/backoff; this only bounds worst-case burst size. |
| `TXN_SYNC_PARALLEL_BATCHES` | 4 | Transaction claim→batch pipelines per dispatcher iteration. |
| `TXN_SYNC_BATCH_SIZE` | 250 | Leagues claimed per transaction batch activity. |
| `TXN_SYNC_LEAGUE_CONCURRENCY` | 12 | Goroutines syncing leagues inside one transaction batch activity. |
| `DRAFT_SYNC_PARALLEL_BATCHES` | 4 | Draft claim→batch pipelines per dispatcher iteration. |
| `DRAFT_SYNC_BATCH_SIZE` | 250 | Leagues claimed per draft batch activity. |
| `DRAFT_SYNC_LEAGUE_CONCURRENCY` | 12 | Goroutines syncing leagues inside one draft batch activity. |
| `DISCOVERY_PARALLEL_BATCHES` | 2 | Discovery claim→batch pipelines per dispatcher iteration. |
| `DISCOVERY_BATCH_SIZE` | 50 | Users claimed per discovery batch activity (smaller — each user fans out into per-league fetches). |
| `DISCOVERY_USER_CONCURRENCY` | 8 | Goroutines discovering users inside one discovery batch activity. |
| `WORKER_ACTIVITY_SLOTS` | 100 | Max concurrent activities on each sync queue (drafts, transactions) for this process. |
| `WORKER_ACTIVITY_POLLERS` | SDK default | Activity task pollers on each sync queue for this process; raise to win a larger share of queue tasks. |

Changing dispatcher knobs needs only a worker restart (they're read by the
`GetTransactionSyncConfig` / `GetDraftSyncConfig` / `GetDiscoveryConfig`
activities each run, not baked into workflow code).

Draft sync mirrors the transaction design on a separate claim column
(`drafts_claimed_at`), so the two paths never contend. Draft-specific
behavior: picks are fetch-once (completed drafts are immutable), and leagues
whose drafting is finished (`in_season`/`complete` with drafts fetched) leave
the claim pool entirely; `pre_draft`/`drafting` leagues recheck on cadence
until their drafts complete.

User discovery uses the same claim model on `sleeper_users.claimed_at`.
Because dispatcher ticks *claim* users instead of re-selecting the stalest
ones and deduping on child-workflow IDs, a slow or stuck cohort can never
head-of-line-block discovery of the users behind it.

### Per-fleet vs global knobs

Task distribution is pull-based: the fleet with more free activity slots and
pollers takes more of the queue — relevant if this ever runs across more than
one worker process again. **Per-fleet** (each process reads its own env):
`SLEEPER_MAX_CONCURRENT_REQUESTS`, `WORKER_ACTIVITY_SLOTS`, `WORKER_ACTIVITY_POLLERS`,
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
SLEEPER_MAX_CONCURRENT_REQUESTS=75
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
serves the API and the Python ESPN worker only). The per-league leg loop is
capped at the current NFL week (past seasons still sweep legs 1–18).

## Rollout / verification

1. Apply migration 018: `cd backend && go run ./cmd/migrate up` (adds
   `claimed_at` + partial index; `CREATE INDEX CONCURRENTLY`, safe live).
2. Deploy the worker host (self-updates within minutes via its deploy timer
   and promotes the new deployment version on start — see the worker
   versioning docs).
3. Watch `/admin` fetch-age buckets: "Never fetched" and "24h+" should shrink
   visibly within hours at default settings (~4 × 250 leagues per claim wave).
4. Watch worker logs for `rate limited (429)` — if it's persistent (not just occasional, self-recovering retries), lower `SLEEPER_MAX_CONCURRENT_REQUESTS`.

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
