# Raspberry Pi Temporal Worker Deploy — Design

## Context

The Sleeper/Temporal discovery and transaction-sync backlog (see admin discovery frontier
page) is bottlenecked by `BatchSize` and worker throughput, not the code. A dormant
Raspberry Pi (5+ years old, unknown exact model/OS, not booted in years) can add worker
capacity by running the same `backend/cmd/worker` binary against the same Temporal Cloud
namespace and Postgres database used in production.

Production itself runs as a single container on DigitalOcean App Platform, which watches
the GitHub repo directly and rebuilds/redeploys from the root `Dockerfile` on every push to
`main` — there is no in-repo CI step that pushes an image or triggers this; it's configured
entirely on the DigitalOcean dashboard.

This design covers only how the Pi builds and redeploys itself on every push to `main`, and
the one-time setup needed to get there. It does not change worker code, task-queue
partitioning, or `BatchSize` tuning — those are separate, already-diagnosed follow-ups.

## Goals

- Pi runs `backend/cmd/worker` continuously, polling the same task queues as production
  (discovery, drafts, transactions, player-sync, week-stats, ADP) against the same Temporal
  Cloud namespace and Postgres instance.
- Pi automatically rebuilds and restarts the worker within a few minutes of a push to
  `main`, without any manual intervention, mirroring the "auto-updates on push" behavior of
  the DigitalOcean deployment.
- One-time setup is a single `make pi-setup` command run on the Pi itself.
- No new standing security surface: no self-hosted GitHub Actions runner, no inbound network
  access required, no Docker required on the Pi.

## Non-goals

- No Docker on the Pi — the worker runs as a native Go binary under systemd.
- No container registry, no Watchtower, no webhook receiver.
- No change to `cmd/worker/main.go`'s task-queue registration — the Pi runs the same binary,
  unmodified, and therefore polls all six task queues. Idle pollers on queues the Pi isn't
  "meant" to help with cost nothing.
- No automated Postgres allowlist management (e.g. via `doctl`) — the setup script prints
  the Pi's public IP and a reminder; adding it to the trusted-sources list is a manual
  one-time step in the DigitalOcean dashboard.

## Architecture

Three systemd units plus a checkout of the repo on the Pi, all installed by one idempotent
setup script:

- **`ff-sims-worker.service`** — long-running service. `ExecStart=/opt/ff-sims/backend/worker`
  (the compiled binary), `EnvironmentFile=/etc/ff-sims-worker.env`, `Restart=on-failure`, runs
  as a dedicated non-root service user.
- **`ff-sims-deploy.timer`** — fires every 5 minutes (`OnBootSec=2min`, `OnUnitActiveSec=5min`).
- **`ff-sims-deploy.service`** (`Type=oneshot`) — runs `deploy.sh`, which checks `origin/main`
  for new commits and rebuilds/restarts the worker if there are any.

All of this is versioned in-repo under `deploy/raspberry-pi/`:

```
deploy/raspberry-pi/
  setup.sh                  # idempotent one-time (and re-runnable) setup
  deploy.sh                 # the git-pull-and-rebuild script the timer runs
  ff-sims-worker.service
  ff-sims-deploy.service
  ff-sims-deploy.timer
  README.md                 # what this is, how to re-run setup after an SD card swap
```

## `deploy.sh` behavior

Run every 5 minutes by the timer:

1. `git fetch origin main`; compare local `HEAD` to `origin/main`. If equal, exit 0 —
   nothing to do.
2. If different: `git reset --hard origin/main`, then `cd backend && go build -o worker.new
   ./cmd/worker`.
3. Only on a successful build: `mv worker.new worker`, `systemctl restart
   ff-sims-worker.service`.
4. On any failure (network down, a commit on `main` that doesn't compile) — exit non-zero,
   leaving the currently-running worker binary and service untouched. The timer retries on
   its next tick; failures are visible via `journalctl -u ff-sims-deploy`.

No lock file is needed: systemd will not start a new run of a `Type=oneshot` service while
the previous invocation of that same unit is still active, so a slow build can't overlap
with the next tick.

## `make pi-setup`

Root `Makefile` gets one thin target, matching the existing `docker-build` style:

```makefile
pi-setup: ## Set up this Pi as a Temporal worker host (run on the Pi itself, with sudo)
	sudo ./deploy/raspberry-pi/setup.sh
```

`deploy/raspberry-pi/setup.sh` does the actual work, idempotently (safe to re-run after
partial failure or after editing the env file):

1. **Go toolchain**: detect `uname -m`; if `go version` is missing or doesn't match the
   version pinned in `backend/go.mod`, download the matching official Go tarball (arm64 or
   armv6l) from `go.dev/dl` and install to `/usr/local/go`.
2. **First build**: `cd backend && go build -o worker ./cmd/worker`, so a binary exists
   immediately.
3. **Env file**: if `/etc/ff-sims-worker.env` doesn't exist, write a template (mode 600)
   with placeholder values for `DATABASE_URL`, `TEMPORAL_NAMESPACE_ENDPOINT`,
   `TEMPORAL_NAMESPACE`, `TEMPORAL_API_KEY`, then stop and print instructions to fill it in
   before re-running. This step never auto-starts the worker on placeholder credentials.
4. **systemd units**: template the three unit files with the actual checkout path (e.g. via
   `sed`), install to `/etc/systemd/system/`, `systemctl daemon-reload`, `systemctl enable
   --now ff-sims-worker.service ff-sims-deploy.timer`.
5. **Final printout**: the Pi's public IP (e.g. via `curl ifconfig.me`) with a reminder to
   add it to the Postgres managed database's trusted-sources allowlist on DigitalOcean, plus
   the `journalctl` commands to tail worker logs (`journalctl -u ff-sims-worker -f`) and
   deploy-check logs (`journalctl -u ff-sims-deploy`).

Re-running `make pi-setup` later (after filling in the env file, or after a full OS
reinstall) picks up wherever it left off rather than erroring on things that already exist.

## Prerequisites / operational notes

- **Networking**: outbound-only — Temporal Cloud (port 7233) and Postgres. No inbound access
  or port-forwarding needed, which fits a home NAT.
- **Postgres allowlist**: the managed Postgres instance restricts connections to trusted
  sources. The Pi's home IP must be added there manually (step 5 above surfaces the IP but
  does not perform this).
- **Secrets**: `/etc/ff-sims-worker.env` is created once by hand (with real values, after the
  placeholder template) and is never committed to the repo.

## Testing / rollout

1. Run `make pi-setup` on the Pi; fill in the real env file; re-run `make pi-setup` to finish
   enabling the service and timer.
2. Confirm `systemctl status ff-sims-worker` is active and `journalctl -u ff-sims-worker -f`
   shows Temporal worker startup / polling logs for all six task queues.
3. Push a trivial commit to `main` and confirm `journalctl -u ff-sims-deploy` shows a
   rebuild + restart within 5 minutes, and `ff-sims-worker`'s uptime resets accordingly.
4. Push a commit that intentionally fails to compile (in a scratch branch, tested by
   temporarily pointing `deploy.sh` at it, or simulated locally) to confirm the deploy script
   leaves the previous good binary running rather than crashing the service.
