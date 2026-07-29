# Worker host

Runs `backend/cmd/worker` (Go, Temporal) and `workers/espn` (Python, Temporal) on this
machine as native systemd services, pointed at the same Temporal Cloud namespace and
Postgres database as production, plus a local archive Postgres database for full-history
Sleeper data. A systemd timer checks `origin/main` every 5 minutes and rebuilds/resyncs +
restarts whichever service has relevant changes — no Docker or a self-hosted CI runner
required, and `journalctl` gives direct log access instead of digging through container
logs.

This is the sole fleet running `backend/cmd/worker` and `workers/espn`: DigitalOcean only
builds/serves `cmd/server` (the API) — see
[`docs/worker-versioning.md`](../../docs/worker-versioning.md) for how the cutover from a
two-fleet (DigitalOcean + Raspberry Pi) setup happened for the Go worker.

## One-time setup

1. `git clone` this repo onto the machine.
2. From the repo root: `make worker-host-setup`
3. The first run installs the pinned Go toolchain and `uv` (Python package/venv manager —
   also provisions Python 3.12 itself if it's not already on the machine), creates the
   service user, masks sleep/suspend/hibernate targets, does an initial Go build and an
   initial `uv sync` for both Python environments (the ESPN worker and the
   player-valuation model), installs the systemd units, and writes a placeholder env file
   at `/etc/ff-sims-worker.env` — then stops and tells you to edit it.
4. Edit `/etc/ff-sims-worker.env` with real values for `DATABASE_URL`,
   `TEMPORAL_NAMESPACE_ENDPOINT`, `TEMPORAL_NAMESPACE`, `TEMPORAL_API_KEY`. Both the Go and
   Python workers read this same file — ESPN league credentials (SWID/`espn_s2`) are stored
   per-league in Postgres via `workers/espn/register_league.py`, not in this env file.
5. Re-run `make worker-host-setup` — this time it starts `ff-sims-worker.service`,
   `ff-sims-espn-worker.service`, and `ff-sims-deploy.timer`, and prints the machine's public
   IPv4 address (e.g. `73.243.246.158`).
6. Add that IP to the Postgres managed database's trusted sources in the DigitalOcean
   dashboard — it expects a plain IPv4 address in that format — the workers can't reach the
   database until you do.
7. Run `make worker-host-setup-archive-db` to provision the local archive Postgres (see
   `setup-archive-db.sh`). It prints an `ARCHIVE_DATABASE_URL` — fill in the (empty)
   `ARCHIVE_DATABASE_URL=` line in `/etc/ff-sims-worker.env` and `sudo systemctl restart
   ff-sims-worker` to pick it up. This step is optional for the Go worker (it just runs
   with the archive disabled) but **required** by `ff-sims-player-valuations.service`,
   which reads every model input from the archive and exits non-zero without it — so
   `setup.sh` leaves that timer disabled until the URL is set. Re-run `make
   worker-host-setup` afterwards to arm it.
8. Disable unattended-upgrades' automatic reboot so it can't restart the machine out from
   under the worker/archive DB: set `Unattended-Upgrade::Automatic-Reboot "false";` in
   `/etc/apt/apt.conf.d/50unattended-upgrades`. This is a one-time, eyes-on edit — not
   scripted, since editing that file unattended is riskier than the sleep mask `setup.sh`
   already applies.

`make worker-host-setup` is safe to re-run at any point (e.g. after fixing the env file, or
after a full reinstall) — it picks up wherever it left off. Same for
`make worker-host-setup-archive-db`.

## Operating

- Go worker logs: `journalctl -u ff-sims-worker -f`
- Python ESPN worker logs: `journalctl -u ff-sims-espn-worker -f`
- Deploy-check history (whether it found a new commit, built/synced, restarted):
  `journalctl -u ff-sims-deploy`. The checkout always advances to `origin/main`, but:
  - The worker and cron binaries are only rebuilt (and the worker service only restarted)
    when the new commits actually touch a path either binary depends on (computed via `go
    list -deps` against `backend/cmd/worker` / `backend/cmd/cron`).
  - The ESPN worker's dependencies are only re-synced (`uv sync`) and the service only
    restarted when the new commits touch anything under `workers/espn`.
  - The player-valuation model's dependencies are only re-synced when the new commits
    touch anything under `analysis`. Nothing is restarted — it's a `Type=oneshot` timer
    job, so a restart would launch an unscheduled full replay; the next 00:00 UTC tick
    picks up the new checkout on its own.
  - A docs/frontend-only push just logs "up to date, no ... changes" for all four and skips
    every rebuild/resync.
- Force an immediate deploy check without waiting for the timer: `sudo systemctl start ff-sims-deploy.service`

### Rolling out a new or changed systemd unit

**`deploy.sh` never installs unit files** — it only rebuilds binaries, re-syncs Python
environments, and restarts services. Unit files reach `/etc/systemd/system` solely through
`install_units()` in `setup.sh`. So when a commit adds a unit (or edits an existing one's
`ExecStart`, timer schedule, or `TimeoutStartSec`), the 5-minute deploy timer will happily
advance the checkout and leave the host running the *old* unit — or, for a brand-new unit,
no unit at all.

After merging any unit change, run this once on the host:

```bash
make worker-host-setup   # idempotent: reinstalls units, daemon-reloads, enables + starts timers
```

This applies to every unit here, not just the new player-valuation one; it is why
`make worker-host-setup` is documented as safe to re-run at any time.
- Discovery cron job logs (runs hourly, `Type=oneshot`): `journalctl -u ff-sims-discovery -f`
- Force an immediate discovery run without waiting for the timer: `sudo systemctl start ff-sims-discovery.service`
- Player-valuation replay logs (runs daily at 00:00 UTC, `Type=oneshot`):
  `journalctl -u ff-sims-player-valuations -f`
  - Each run is a **full replay** of the 2025 `ppr-sf-10` season from 2025-08-25 through
    a snapshot dated the current UTC day (covering events through the end of yesterday),
    not an incremental tick. It reads inputs from the archive
    database and writes every `player_valuations` row to the cloud database, so it needs
    both `ARCHIVE_DATABASE_URL` and `DATABASE_URL` in `/etc/ff-sims-worker.env`.
  - `setup.sh` only arms this timer once `ARCHIVE_DATABASE_URL` is non-empty in
    `/etc/ff-sims-worker.env` (step 7 below); until then it is left disabled rather than
    failing every midnight. Every other service starts regardless — they run fine with the
    archive disabled. Fill the URL in and re-run `make worker-host-setup` to arm it.
  - Once armed, the timer deliberately does not fire on boot or catch up missed runs, so
    nothing starts a surprise full replay. Kick off the first production run by hand:
    `sudo systemctl start ff-sims-player-valuations.service`
  - Check the schedule: `systemctl list-timers ff-sims-player-valuations.timer`
  - Every run prints its staged Parquet directory (default
    `/tmp/ff-sims-player-valuations/<run-id>/`), kept on success and failure for
    inspection and pruned after 14 days. See [`analysis/README.md`](../../analysis/README.md)
    for the artifact layout and the retention/staging env vars.
  - A run that finds another replay already holding the advisory lock exits `2` without
    touching cloud output — that is expected if you start it by hand while the timer run
    is still going.
- The Go worker runs the *same* `backend/cmd/worker` binary as production, so it polls all
  five Temporal task queues (drafts, transactions, player-sync, week-stats, ADP), not just
  transactions — the idle pollers on the other queues cost nothing.
- This host is the promoting fleet for the shared Temporal Worker Deployment
  (`ff-sims-worker`) — see [`docs/worker-versioning.md`](../../docs/worker-versioning.md) for
  how versioning works and how to inspect/promote versions. That doc covers the Go worker
  only; the Python ESPN worker isn't on Worker Deployment Versioning.
