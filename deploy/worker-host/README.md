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
   initial `uv sync` for the ESPN worker, installs the systemd units, and writes a
   placeholder env file at `/etc/ff-sims-worker.env` — then stops and tells you to edit it.
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
   `setup-archive-db.sh`). It prints an `ARCHIVE_DATABASE_URL` — add it to
   `/etc/ff-sims-worker.env` and `sudo systemctl restart ff-sims-worker` to pick it up.
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
  - A docs/frontend-only push just logs "up to date, no ... changes" for all three and skips
    every rebuild/resync.
- Force an immediate deploy check without waiting for the timer: `sudo systemctl start ff-sims-deploy.service`
- Discovery cron job logs (runs hourly, `Type=oneshot`): `journalctl -u ff-sims-discovery -f`
- Force an immediate discovery run without waiting for the timer: `sudo systemctl start ff-sims-discovery.service`
- The Go worker runs the *same* `backend/cmd/worker` binary as production, so it polls all
  five Temporal task queues (drafts, transactions, player-sync, week-stats, ADP), not just
  transactions — the idle pollers on the other queues cost nothing.
- This host is the promoting fleet for the shared Temporal Worker Deployment
  (`ff-sims-worker`) — see [`docs/worker-versioning.md`](../../docs/worker-versioning.md) for
  how versioning works and how to inspect/promote versions. That doc covers the Go worker
  only; the Python ESPN worker isn't on Worker Deployment Versioning.
