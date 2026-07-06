# Transaction Sync Operations

## Tuning knobs (env, per worker process)

| Var | Default | Meaning |
|-----|---------|---------|
| `SLEEPER_RPM` | 2000 | Sleeper API requests/minute budget for this process (per fleet IP). Start high, tune down if 429s appear in logs. |
| `TXN_SYNC_PARALLEL_BATCHES` | 4 | Claim→batch pipelines per dispatcher iteration. |
| `TXN_SYNC_BATCH_SIZE` | 250 | Leagues claimed per batch activity. |
| `TXN_SYNC_LEAGUE_CONCURRENCY` | 12 | Goroutines syncing leagues inside one batch activity. |

Changing dispatcher knobs needs only a worker restart (they're read by the
`GetTransactionSyncConfig` activity each run, not baked into workflow code).

## How it works

Every 5 minutes `TransactionSyncDispatcher` claims batches of stale leagues
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
