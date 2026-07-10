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
