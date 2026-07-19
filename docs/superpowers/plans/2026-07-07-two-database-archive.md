# Two-Database Architecture: Cloud Hot Store + Local Archive

## Problem

The DO managed Postgres is ~20GB, nearly all `sleeper_transactions` + `sleeper_drafts`/`sleeper_draft_picks`. Result: dropped connections, failed requests, slow pages. Only the trailing ~30 days is needed for the product.

## Target state

- **Cloud DB (DO)** — ESPN/actual-league + simulation tables, sleeper support tables (`sleeper_leagues`, `sleeper_users`, `sleeper_players`, stats, valuations, `draft_adp`), and only the last ~30 days of sleeper transactions + drafts. API reads unchanged, just faster.
- **Archive DB (Postgres 16 on the local machine)** — a repurposed personal desktop, wiped to Ubuntu LTS (replaces the earlier Raspberry Pi plan). Holds the **complete history** of sleeper transactions, drafts, picks, plus a `sleeper_leagues` replica (the ADP rollup joins leagues for `league_type`/`ppr`/`total_rosters`/`is_superflex`). Long-term store; dumps can move to object storage or another DB later.
- **Scavenger** — Temporal workflow every 6h. **Copy-early, purge-late**: replicate rows to the archive shortly after ingest; delete from cloud only when >30 days old **and** verified present in the archive. The archive is always a superset — full-history jobs read one DB, and replication doubles as continuous off-site protection.
- **All workers on the local machine** — DO container's worker retired; the Pi worker decommissioned after cutover. Sheds cloud DB connections immediately.
- **ADP rollup reads the archive**, writes `draft_adp` back to cloud.
- **Valuation model (important future work)** — runs on the local machine on a cadence against the full-history archive (trades + drafts), writing `player_valuations` results to cloud. Out of scope here, but the archive is what makes it possible; anticipate it in archive indexes.
- **Daily backup** — `pg_dump -Fc` of the archive to a pluggable rclone remote (destination TBD), systemd timer.

## Facts that shaped the design (repo @ `main` 08a44a2)

- Activities already take injected `*gorm.DB` — clean seam for a second handle. Single global `database.DB` today.
- `sleeper_transactions`: PK `sleeper_transaction_id`, insert-only, `created_at` = insert time. **No index on `created_at`** — must add.
- `sleeper_drafts.updated_at` is **dead** (upsert never assigns it). Cursors must use `created_at` (header) + `last_fetched_at` (set once, when picks land).
- `sleeper_draft_picks`: **no timestamps**, composite PK, FK to drafts → replicate via parent draft; verify by pick-count parity; delete picks before drafts.
- **Purge must mirror the draft claim-pool exclusion** (`data_fetch.go:43-54`): purging a draft whose league is still in the sync pool triggers a full pick-refetch loop.
- Worker versioning: one Deployment `ff-sims-worker`, Pinned; only the Dockerfile sets `promoteOnStart=true`. Exactly one promoting fleet must hold at every instant during cutover. **`deploy.sh` is shared by every host that runs it** — the Pi must stop self-deploying before `promoteOnStart` lands there.
- `deploy/raspberry-pi/` scripts port to the desktop nearly verbatim: `setup.sh` already maps `x86_64`→amd64, everything is plain systemd + native Go builds, no Docker.
- Both fleets already use pgbouncer (`:25061/connpool?sslmode=require`).
- Disk: data (~19GB) + indexes + WAL + local backups wants ≥ ~60GB free — verify during machine setup (a desktop drive makes this a non-issue, vs. the Pi's 28GB SD).

## Design

**Second handle.** `ARCHIVE_DATABASE_URL` (empty = disabled) + `database.Archive` / `InitializeArchive`. Only the worker initializes it. Injection: `ScavengerActivities{Cloud, Archive}`, `ADPRollupActivities{Read, Write}`.

*Decision: explicit handles, no type-based routing wrapper.* A reflection/model-routed DB (or GORM's DBResolver plugin) picks the database from the model type — but here the same tables exist in **both** DBs and the destination depends on the *operation* (sync writes hot to cloud; scavenger reads cloud + writes archive; ADP reads archive, writes cloud). Type-routing can't express that and hides which DB a query hits. Named injected handles keep it visible at every call site.

**Archive schema.** Separate goose dir `backend/migrations/archive/` (cloud migrations carry ESPN FKs that can't run there). `cmd/migrate` gets a `-db archive|cloud` flag; archive migrations auto-run at worker startup (no ssh step per self-deploy). Tables: the 4 replicas **without FKs** (arrival order must not matter) + `archive_sync_state` (watermarks live in the archive DB so cursor advance commits atomically with the copied rows; replays absorbed by `OnConflict DoNothing`).

**Queue/versioning.** New queue `archive-maintenance`; the worker registers the archive worker (and second handle) only when `ARCHIVE_DATABASE_URL` is set — local dev keeps working without an archive DB. Same deployment/buildID. `promoteOnStart` moves to the local machine's builds in the same atomic PR that removes the worker from the Dockerfile (Pi already stopped by then).

**Scavenger** (every 6h, overlap = Skip):
1. Config activity: `SCAVENGER_BATCH_SIZE` (5000 txn / 200 draft), `SCAVENGER_MAX_BATCHES_PER_RUN` (50), `SCAVENGER_RETENTION_DAYS` (30), `SCAVENGER_PURGE_ENABLED` (**default false** — kill-switch).
2. Replicate: leagues → transactions → drafts. Cursors: txns on `(created_at, id)` with a 5-min safety lag; drafts on **two watermarks** (`created_at` headers, `last_fetched_at` picks+status flip); leagues on `updated_at`.
3. Purge (only if enabled and caught up): select cloud IDs whose *event* age (transactions: `created_at_sleeper`; drafts: `season`) is past 30d/the current season → verify in archive → chunked deletes in short transactions. Drafts additionally require the claim-pool-exclusion predicate and pick-count parity. Unverified rows are skipped and counted; oldest unverified (by insert time) > retention+15d ⇒ activity error ⇒ **red run in Temporal UI = replication-stalled alarm**.
4. Result: `ScavengerReport{Replicated, Purged, LagSeconds, Unverified}` — the observability surface.

**Failure modes.** Machine down N days → cloud accumulates, drains over later runs. Watermark/rows can't diverge (same txn). Failed deletes re-verify next run. Archive unreachable → purge never reached. Clock skew irrelevant (cloud-side `now()`).

**Backfill.** `ArchiveBackfillWorkflow` = same replicate activities + shared cursors, ContinueAsNew until caught up; started once manually. Escape hatch if WAN is too slow: `pg_dump --data-only` seed + set watermarks to max.

*Alternative considered — dual-write at ingest.* With workers local, sync activities could write the archive (localhost) alongside cloud, removing the replicate phase. Deferred: touches every hot write path in `data_fetch.go`, couples sync success to archive health, and verify-before-purge + backfill still need the scavenger. Viable later optimization — replicate then degrades to a cheap reconciler.

**Age-based write routing (T13) — adopted, narrower than dual-write.** Not "write everything twice" — route each row to exactly *one* database at ingest time, based on whether the event itself (transaction `created_at_sleeper`; draft/picks `season`) already falls outside the retention window. A league's first-ever sync (or catch-up after downtime) pulls multiple seasons of transactions and drafts in one pass; today all of it lands in cloud, then the scavenger copies the old parts to archive, then purge deletes them from cloud — three writes for data that was never going to be "hot." Routing old rows straight to archive-only at insert time collapses that to one write, at the cost of `DataFetchActivities` needing a second DB handle and an age check on the hot sync path (the same "touches every hot write path" cost the dual-write alternative was deferred for — but this is a narrower slice: only the *old-data* code path gains a dependency on archive health, not every write). Current-window data keeps writing to cloud only, unchanged; the scavenger still replicates it forward normally once it ages out. Leagues are unaffected (always cloud-authoritative, archive copy stays scavenger-replicated — see the ADP join dependency above). Threshold is configurable, conceptually the same "how old is too old" knob as `SCAVENGER_RETENTION_DAYS` (T6) — worth confirming during T13's design whether to literally share that env var or give it its own, and worth sequencing after T6 lands to avoid both touching `data_fetch.go`/`scavenger.go` at once.

**Backup.** systemd (mirrors `ff-sims-deploy.timer`): dump to a local backups dir, `rclone copy` to `${BACKUP_RCLONE_REMOTE}`, prune local keep-3 / remote >60d. Documented restore drill.

## Tasks (each ≈ one PR)

| # | Task | Size | Depends on | Status |
|---|------|------|-----------|--------|
| T1 | Provision local machine: wipe → Ubuntu LTS, generalize `deploy/raspberry-pi/` → `deploy/worker-host/`, run setup (worker fleet joins), Postgres 16 + archive DB (`setup-archive-db.sh`), disable sleep/auto-reboot | M | — | Done — PR #153 |
| T2 | Cutover: stop Pi worker + deploy timer (ops), then atomic PR — retire DO worker from Dockerfile, add `promoteOnStart` to host builds | S | T1 | Done — PR #153 |
| T3 | Second DB handle + archive migrations plumbing (tracer bullet) | S/M | T1* | Done — PR #151 |
| T4 | Cloud migration 021: CONCURRENTLY indexes on txn `created_at`, draft `last_fetched_at` | S | — | Done — PR #151 |
| T5 | Scavenger replicate phase + 6h schedule + archive worker | L | T3, T4 | Done — PR #152 |
| T6 | Purge phase — ships dark behind `SCAVENGER_PURGE_ENABLED=false` | M | T5 | Done — PR #155 |
| T7 | ADP rollup reads archive (`{Read, Write}`) | S/M | T2, T5 | Done — PR #157 |
| T8 | Initial backfill (workflow + runbook; parity checks) | S code / M ops | T1, T5 | Done — PR #154 |
| T9 | Enable purge; drain; `VACUUM` + `pg_repack` to reclaim cloud disk | S code / M ops | T6–T8 | In progress — issue #167. Enabled in production 2026-07-12 on a verified-current build (see T14 and the deploy-pipeline note below); draining, disk reclaim (`VACUUM`/`pg_repack`) not yet done |
| T10 | Daily backup (pg_dump + rclone, systemd timer) | M | T1 | Deferred — issue #160 (durability risk accepted for now; not required before T9) |
| T11 | Docs: `docs/archive-operations.md`, runbook/versioning updates | S | rolling | Not started — issue #168 |
| T12 | Optional: migrate cloud to a smaller DO cluster (dump/restore + repoint `DATABASE_URL`) | S ops | T9 | Not started — issue #169 |
| T13 | Age-based write routing: `DataFetchActivities` writes already-old transactions/drafts/picks straight to archive, skipping cloud, instead of write-then-replicate-then-purge | L | T3, **T6** (shared retention concept; sequence after to avoid file conflicts) | Done — PR #159 |
| T14 | Fix purge eligibility to use event time (`created_at_sleeper`/`season`), not insert time — see Risk #5 | S/M | T6 | Done — PR #163 |
| T15 | Close T13's current-season gap for drafts: route ALL drafts+picks (not just past-season) straight to archive when configured — draft data is immutable with no live-API reader, so cloud never needs any of it | S | T13 | Done — PR #179 |

\* T3 is env-gated and mergeable before T1; it just can't connect until the archive DB exists.

Parallel-friendly: T1 and T4 are independent starting points; T3 code can merge any time.

**Future work (not in scope):** valuation model on a cadence on the local machine — reads full-history archive (trades/drafts), writes `player_valuations` to cloud; likely a Temporal schedule on `archive-maintenance` invoking the Python job, or a systemd timer. Design the archive indexes with it in mind (transactions by league/time already covered).

## Verification

- PG-specific tests on local :5499 (`TEST_DATABASE_URL`, two throwaway schemas as cloud/archive), SQLite for the rest; `go test ./...`.
- T2: DO logs show no worker; `temporal worker deployment describe` shows the host build promoting; Pi units stopped/disabled.
- T5 tracer: watermarks advance, archive rows grow, second run is a no-op (idempotence).
- T8: `count(*)` / `min-max(created_at)` parity, sampled pick-count parity.
- T9: cloud txn count drops to ~30d window; sleeper API endpoints fast + correct; disk reclaimed on DO dashboard.
- T10: restore drill into a scratch DB.

## Risks / open questions

1. **Durability inversion** — post-purge the local machine holds the only >30d copy. Originally gated on T10 (backup) + restore drill; **2026-07-10: risk knowingly accepted for now** (test/personal project) — T10 deferred to issue #160, no longer a hard prerequisite for T9. Revisit before this becomes anything more than a test project.
2. **Stats counts shrink** — `GetSleeperStats` and pagination totals collapse to the 30d window (user-visible). Cheap follow-up if it matters: scavenger-maintained all-time-counts row in cloud.
3. **DO won't downsize in place** — `pg_repack` reclaims disk inside the cluster, but the bill is set by plan size. Real cost reduction = new smaller cluster + dump/restore (minutes, at post-purge size) + repoint both fleets — hence optional T12, only possible after T9.
4. **Backfill over WAN** — may take many hours; pg_dump seed is the escape hatch.
5. ~~**Insert-time retention**~~ — **fixed (T14, 2026-07-11)**: purge originally filtered by `created_at` (insert time), meaning freshly-backfilled old data stayed hot for a full 30 days regardless of how old the underlying event actually was — this wasn't a deliberate tradeoff, it was a defect, discovered in production when ~40M backfilled transactions were all insert-time-recent and therefore zero were purge-eligible. Purge now filters transactions by `created_at_sleeper` (event time) and drafts by `season`. Replicate (T5) is unaffected — its cursor correctly still needs insert-time's monotonic-arrival guarantee.
6. **Dual-use machine** — it's also a personal computer: disable sleep/hibernate, pin unattended-upgrade reboots to a window, accept that OS reinstalls/tinkering pause sync + replication (both fail safe: data goes stale, purge stalls). Single machine now runs everything — add a basic uptime ping later.
7. **PG version parity** — local PGDG 16 = cloud = CI image; keep aligned for dump/restore and `PERCENTILE_CONT` parity.
8. **Two promoting fleets during cutover** — `deploy.sh` is shared; the Pi must be stopped before `promoteOnStart` merges (enforced by T2's ordering).
9. **Silent deploy drift (fixed, 2026-07-12)** — unrelated to the archive design itself, but it masked T14's fix for a day: the T1 rename (`deploy/raspberry-pi/` → `deploy/worker-host/`, PR #153) left `ff-sims-deploy.service`'s `ExecStart` pointing at the old path, so rosebud's 5-minute self-update timer had been failing every run since — the worker binary silently stayed pinned to commit `f606acc` (T13) while `main` moved on, including T14. `make build` also never covered `cmd/worker` at all, making a correct manual rebuild easy to get wrong mid-incident. Fixed by PR #164 (stale path + `make build-worker` + a unit-file regression test) and PR #165 (a related bug where `deploy.sh`'s own `git fetch` failure was silently swallowed by a bash command-substitution quirk, reporting "up to date" instead of erroring). Lesson: after any future rosebud deploy, verify `journalctl -u ff-sims-worker | grep build_id` against `git log -1` on the host rather than assuming a restart means new code is live.
