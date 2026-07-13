# Archive Purge Runbook (T9)

Enables the purge phase the scavenger has shipped dark since T6, drains the
backlog of cloud rows already verified in the archive, and reclaims the
disk space that frees up. All of this runs against real production
infrastructure — the DigitalOcean cloud Postgres and the worker host — not
something with a meaningful local-dev equivalent, so this is a runbook, not
code.

## Preconditions — confirm before enabling

1. T1–T8 are not just merged but actually **deployed and running** on the
   worker host (`sudo systemctl status ff-sims-worker`).
2. The scavenger schedule (`sleeper-scavenger-schedule`) has been ticking
   every 6h with successful (non-red) runs — check via the Temporal UI or
   `temporal schedule describe --schedule-id sleeper-scavenger-schedule`.
3. **`ArchiveBackfillWorkflow` (T8) has been run to completion and its
   parity checks (`docs/archive-backfill.md`) have passed.** This is the
   precondition most likely to be silently unmet — the code has been merged
   since T8, but that's different from having actually *started* the
   workflow on the real archive DB. Purge only ever deletes cloud rows that
   are *verified present* in the archive; if most of the original ~20GB of
   history was never backfilled, enabling purge now won't delete much of
   anything — it'll just accumulate a growing "unverified" count and
   eventually trip the stalled-replication alarm (a red Temporal run) once
   the oldest unverified row crosses `retention + 15 days`. If you haven't
   run the backfill yet, do that first.
4. A backup exists, or the durability risk is knowingly accepted (currently
   the case — see issue #160). Once purge deletes cloud rows, the archive
   becomes the only copy of anything older than the retention window.

**Purge eligibility is based on when the data actually happened, not when
it was inserted.** Transactions use `created_at_sleeper` (Sleeper's own
event timestamp); drafts use `season`. This matters if you're backfilling
history for newly-discovered leagues — that data can be purge-eligible
immediately once verified in archive, not 30 days after whenever it
happened to be synced.

## Step 1: Enable purge

On the worker host:

```bash
sudo vi /etc/ff-sims-worker.env
# add or change:
SCAVENGER_PURGE_ENABLED=true
```

Then:

```bash
sudo systemctl restart ff-sims-worker
journalctl -u ff-sims-worker -n 50 --no-pager   # confirm it restarted cleanly
```

Purge only actually deletes anything once the *replicate* side of the same
stream has fully caught up within a given scavenger run (so it never scans
ahead of a backlog it doesn't yet know is safe) — see the next section for
what to expect on the runs right after flipping the switch.

## Step 2: Monitor the drain

Each 6h scavenger run logs one summary line:

```bash
journalctl -u ff-sims-worker | grep "scavenger run complete"
```

Fields to watch: `transactionsPurged`, `transactionsUnverified`,
`draftsPurged`, `draftsUnverified`.

- `transactionsPurged` / `draftsPurged` growing run over run = drain
  progressing normally.
- `transactionsUnverified` / `draftsUnverified` staying near zero =
  replication is keeping up. A large or growing unverified count means data
  hasn't been backfilled/replicated into archive yet for that range —
  investigate (see Precondition 3) before assuming purge is just slow.

Also watch the `ScavengerDispatcher` workflow on the `archive-maintenance`
task queue in the Temporal UI for a **red (failed) run** — that's the
built-in alarm. It fires when some row has sat unverified for more than
`retention + 15 days`, meaning replication has stalled, not just lagged.

Direct SQL check of the remaining backlog, run against the **cloud** DB —
note this is by Sleeper's own event time (`created_at_sleeper`, epoch
milliseconds), not insert time; a row can be purge-eligible the moment
it's replicated even if it was only just inserted (e.g. during a new
league's backfill):

```sql
SELECT count(*) FROM sleeper_transactions
WHERE created_at_sleeper < extract(epoch from now() - interval '30 days') * 1000;

SELECT count(*) FROM sleeper_drafts WHERE season < to_char(now(), 'YYYY');
```

(30 days is the `SCAVENGER_RETENTION_DAYS` default — adjust the interval if
you've overridden it.) Watch these counts drop toward zero over successive
6h runs.

With the default batch sizing (`SCAVENGER_TXN_BATCH_SIZE=5000`,
`SCAVENGER_DRAFT_BATCH_SIZE=200`, `SCAVENGER_MAX_BATCHES_PER_RUN=50`), one
run can purge up to 250,000 transactions or 10,000 drafts once verified —
for the ~20GB of history this project started with, expect the backlog to
clear within a handful of 6h cycles (well under a day), assuming backfill
already caught the archive up per Precondition 3.

## Step 3: Verify cloud actually shrunk

Once the backlog counts in Step 2 hit zero:

```sql
clear
```

Also check the DigitalOcean dashboard's database disk-usage graph — expect
it to plateau, not drop immediately (see Step 4 for why).

## Step 4: Reclaim disk (VACUUM / pg_repack)

Deleting rows doesn't return space to the OS by default — Postgres marks it
internally reusable (autovacuum handles that over time), but the table's
on-disk size doesn't shrink without an explicit reorg.

**Don't run `VACUUM FULL` on these tables without thinking it through** —
it takes an `ACCESS EXCLUSIVE` lock for the whole operation, blocking every
read and write (including the live API) until it finishes. On a table that
was ~20GB, that could be minutes to hours of downtime.

Prefer `pg_repack` if this DO plan supports it — it does the same reorg
online, without the exclusive lock. Check first:

```sql
SELECT * FROM pg_available_extensions WHERE name = 'pg_repack';
```

If available:

```sql
CREATE EXTENSION IF NOT EXISTS pg_repack;
```

```bash
pg_repack --host=<host> --port=<port> --username=<user> --dbname=<db> --table=sleeper_transactions
pg_repack --host=<host> --port=<port> --username=<user> --dbname=<db> --table=sleeper_drafts
pg_repack --host=<host> --port=<port> --username=<user> --dbname=<db> --table=sleeper_draft_picks
```

If `pg_repack` isn't available on this plan, a plain `VACUUM` (not `FULL`)
run during low-traffic hours won't reclaim OS-visible disk but will make
the freed space reusable for future writes, which caps further growth.
Treat `VACUUM FULL`'s real downtime as a deliberate, announced maintenance
window if disk reclaim is genuinely urgent — not a routine step here.

## Step 5: Confirm the win

Re-check the DO dashboard's disk-usage graph a day or two after
repack/vacuum completes.

**Expectation-setting: this does not lower the DO bill by itself.**
DigitalOcean's managed Postgres pricing is by plan size, not actual disk
usage — reclaiming space inside the existing cluster doesn't downsize the
plan. Real cost reduction requires migrating to a smaller cluster
(dump/restore at the new, post-purge size, then repoint `DATABASE_URL` for
both fleets) — that's the optional T12, and only makes sense once this step
is done.
