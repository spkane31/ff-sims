# Raspberry Pi Temporal worker

Runs `backend/cmd/worker` on this Pi as a native systemd service, pointed at the same
Temporal Cloud namespace and Postgres database as production. A systemd timer checks
`origin/main` every 5 minutes and rebuilds + restarts the worker if there's a new commit —
mirroring how the DigitalOcean-hosted container auto-deploys on push to `main`, without
Docker or a self-hosted CI runner.

## One-time setup (or after an SD card swap)

1. Flash Raspberry Pi OS, boot, and `git clone` this repo onto the Pi.
2. From the repo root: `make pi-setup`
3. The first run installs the pinned Go toolchain, does an initial build, installs the
   systemd units, and writes a placeholder env file at `/etc/ff-sims-worker.env` — then
   stops and tells you to edit it.
4. Edit `/etc/ff-sims-worker.env` with real values for `DATABASE_URL`,
   `TEMPORAL_NAMESPACE_ENDPOINT`, `TEMPORAL_NAMESPACE`, `TEMPORAL_API_KEY`.
5. Re-run `make pi-setup` — this time it starts `ff-sims-worker.service` and
   `ff-sims-deploy.timer`, and prints the Pi's public IPv4 address (e.g. `73.243.246.158`).
6. Add that IP to the Postgres managed database's trusted sources in the DigitalOcean
   dashboard — it expects a plain IPv4 address in that format — the worker can't reach the
   database until you do.

`make pi-setup` is safe to re-run at any point (e.g. after fixing the env file, or after a
full reinstall) — it picks up wherever it left off.

## Operating

- Worker logs: `journalctl -u ff-sims-worker -f`
- Deploy-check history (whether it found a new commit, built, restarted): `journalctl -u ff-sims-deploy`
- Force an immediate deploy check without waiting for the timer: `sudo systemctl start ff-sims-deploy.service`
- This runs the *same* `backend/cmd/worker` binary as production, so it polls all six
  Temporal task queues (discovery, drafts, transactions, player-sync, week-stats, ADP), not
  just discovery/transactions — the idle pollers on the other queues cost nothing.
