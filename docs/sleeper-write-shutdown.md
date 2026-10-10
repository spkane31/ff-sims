# Sleeper league and transaction write shutdown

League and transaction ingestion is retired in both the cloud and archive
databases. This change retains existing data and does not run a purge, truncate,
drop, or storage-reclamation operation.

## Persistent behavior

- `cron -job=discovery`, `-job=transactions`, and `-job=lifetime-counts` exit
  successfully with a retirement message before connecting to either database.
  This also stops transaction-value reconciliation, archive replication, and the
  optional cloud purge performed by lifetime-counts.
- Host setup disables and stops the three timers and their active services. Cron
  deployments do the same before replacing the binary, since an already-running
  process would otherwise keep executing old code.
- Host setup restarts the Go worker after rebuilding it. The normal deployment
  path also restarts it when worker code changes.
- Worker startup pauses `sleeper-draft-sync-schedule` if it exists and never
  creates it. A missing schedule is fine; other pause errors fail startup.
- The worker no longer registers or polls the `sleeper-drafts` and
  `archive-maintenance` queues, including their write activities. Historical
  workflow/activity implementations remain in the repository but are not wired
  into the deployed worker.

Player sync, week stats, ADP, ESPN, and player-valuation replay stay enabled.
Trade/draft ingestion and Sleeper growth snapshots stop refreshing. Derived
values can still be recomputed from the retained historical data.

## Rollout

1. Merge and deploy the change through the worker host's normal deploy service.
   The rollout must rebuild both worker and cron; confirm their new build IDs in
   deployment logs. A local source edit alone does not stop production writes.
2. If deploying manually from the updated checkout, run `make worker-host-setup`.
   It rebuilds the binaries, stops the retired cron jobs, and restarts the worker.
3. Confirm the old worker process has exited and no other host is running an old
   worker/cron binary. This repository's deployment assumes one worker host.
4. In Temporal, confirm `sleeper-draft-sync-schedule` is paused (or absent). Cancel
   outstanding `DraftSyncDispatcher` and `ArchiveBackfillWorkflow` executions,
   including any old pending runs. Removing pollers prevents these executions
   from writing through the new worker, but does not close them in Temporal.
   Schedule pause alone does not stop existing executions or in-flight activities.

For an immediate cron stop before the new binaries are deployed:

```bash
sudo systemctl disable --now \
  ff-sims-discovery.timer \
  ff-sims-transactions.timer \
  ff-sims-lifetime-counts.timer
sudo systemctl stop \
  ff-sims-discovery.service \
  ff-sims-transactions.service \
  ff-sims-lifetime-counts.service
```

Those commands alone do not stop the old Go worker's draft or archive activities.
Complete the worker rollout before treating the tables as inactive.

## Verification before storage reclamation

- Check `systemctl is-enabled` reports the three timers as disabled and
  `systemctl is-active` reports both timers and services as inactive.
- Check the worker's logged build ID matches the deployed change and only the
  remaining task queues have pollers.
- Verify there are no running draft/backfill activities on an older worker.
- Monitor inserts, updates, and deletes on both tables in both databases across
  their former scheduling intervals. A stable row count alone cannot detect
  league tracking updates or transaction-value reconciliation.
- Leave data deletion and disk-space reclamation for a separate reviewed change.
