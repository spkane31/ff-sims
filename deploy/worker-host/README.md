# Worker host

Runs `backend/cmd/worker` on this machine as a native systemd service, pointed at the same
Temporal Cloud namespace and Postgres database as production, plus a local archive Postgres
database for full-history Sleeper data. A systemd timer checks `origin/main` every 5 minutes
and rebuilds + restarts the worker if there's a new commit — no Docker or a self-hosted CI
runner required.

This is the sole fleet running `backend/cmd/worker`: DigitalOcean only builds/serves
`cmd/server` (the API) and the Python ESPN worker — see
[`docs/worker-versioning.md`](../../docs/worker-versioning.md) for how the cutover from a
two-fleet (DigitalOcean + Raspberry Pi) setup happened.

## One-time setup

1. `git clone` this repo onto the machine.
2. From the repo root: `make worker-host-setup`
3. The first run installs the pinned Go toolchain, creates the service user, masks
   sleep/suspend/hibernate targets, does an initial build, installs the systemd units, and
   writes a placeholder env file at `/etc/ff-sims-worker.env` — then stops and tells you to
   edit it.
4. Edit `/etc/ff-sims-worker.env` with real values for `DATABASE_URL`,
   `TEMPORAL_NAMESPACE_ENDPOINT`, `TEMPORAL_NAMESPACE`, `TEMPORAL_API_KEY`.
5. Re-run `make worker-host-setup` — this time it starts `ff-sims-worker.service` and
   `ff-sims-deploy.timer`, and prints the machine's public IPv4 address (e.g.
   `73.243.246.158`).
6. Add that IP to the Postgres managed database's trusted sources in the DigitalOcean
   dashboard — it expects a plain IPv4 address in that format — the worker can't reach the
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

- Worker logs: `journalctl -u ff-sims-worker -f`
- Deploy-check history (whether it found a new commit, built, restarted): `journalctl -u ff-sims-deploy`
- Force an immediate deploy check without waiting for the timer: `sudo systemctl start ff-sims-deploy.service`
- This runs the *same* `backend/cmd/worker` binary as production, so it polls all six
  Temporal task queues (discovery, drafts, transactions, player-sync, week-stats, ADP), not
  just discovery/transactions — the idle pollers on the other queues cost nothing.
- This host is the promoting fleet for the shared Temporal Worker Deployment
  (`ff-sims-worker`) — see [`docs/worker-versioning.md`](../../docs/worker-versioning.md) for
  how versioning works and how to inspect/promote versions.
