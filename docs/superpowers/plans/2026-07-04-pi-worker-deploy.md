# Raspberry Pi Temporal Worker Deploy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a Raspberry Pi run `backend/cmd/worker` as a self-updating systemd service, rebuilding and restarting itself within 5 minutes of any push to `main`, set up with a single `make pi-setup`.

**Architecture:** A `deploy.sh` script (run every 5 minutes by a systemd timer) checks `origin/main` for new commits, and on a change does a hard reset + native `go build` + binary swap + `systemctl restart` — leaving the previous binary running untouched if the build fails. A `setup.sh` script (invoked once, and safely re-runnable, via `make pi-setup`) installs the pinned Go toolchain, creates a dedicated service user, does the first build, installs the three systemd units, and either starts the worker (if `/etc/ff-sims-worker.env` already has real credentials) or stops short with instructions if it still has placeholder values.

**Tech Stack:** Bash, systemd (service + timer units), Go 1.25.7, git. No Docker, no container registry, no self-hosted CI runner.

## Global Constraints

- No Docker on the Pi — native Go binary run under systemd (per spec's Non-goals).
- Go toolchain pinned to `1.25.7` (from `backend/go.mod`'s `go 1.25.7` directive) — install this exact version, not just "latest".
- Deploy check runs every 5 minutes: `OnBootSec=2min`, `OnUnitActiveSec=5min`.
- On build failure, the previously-running worker binary and service must be left untouched — never partially overwrite the binary in place.
- No lock file needed for overlap prevention — a `Type=oneshot` systemd service naturally can't be double-started by its own timer while still running.
- Env file lives at `/etc/ff-sims-worker.env`, mode `600`, holding `DATABASE_URL`, `TEMPORAL_NAMESPACE_ENDPOINT`, `TEMPORAL_NAMESPACE`, `TEMPORAL_API_KEY` — never committed to the repo, never auto-populated with real values.
- Everything lives under `deploy/raspberry-pi/` in the repo, versioned, not hand-configured on the Pi ad hoc.
- `make pi-setup` (root `Makefile`) is the only supported entry point for setup; it must be safe to re-run.
- No `doctl`/automated Postgres allowlist changes — only print the Pi's public IP and a reminder.

---

### Task 1: `deploy.sh` — git-pull-and-rebuild-and-restart script

**Files:**
- Create: `deploy/raspberry-pi/deploy.sh`
- Test: `deploy/raspberry-pi/tests/test_deploy.sh`

**Interfaces:**
- Consumes: nothing from other tasks (self-contained). Reads env var `REPO_DIR` (defaults to two directories up from the script's own location) and `WORKER_SERVICE` (defaults to `ff-sims-worker.service` — must match the unit name Task 2 installs).
- Produces: `deploy()` function, invoked automatically when the script is run directly (not when sourced) via a `BASH_SOURCE[0] == $0` guard, so Task 2's tests (or any future caller) can `source` this file without triggering a live deploy.

- [ ] **Step 1: Write `deploy.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="${REPO_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
WORKER_SERVICE="${WORKER_SERVICE:-ff-sims-worker.service}"

current_and_remote_sha() {
  git -C "$REPO_DIR" fetch origin main --quiet
  local local_sha remote_sha
  local_sha=$(git -C "$REPO_DIR" rev-parse HEAD)
  remote_sha=$(git -C "$REPO_DIR" rev-parse origin/main)
  echo "$local_sha $remote_sha"
}

build_worker() {
  (cd "$REPO_DIR/backend" && go build -o worker.new ./cmd/worker)
}

deploy() {
  local local_sha remote_sha
  read -r local_sha remote_sha <<< "$(current_and_remote_sha)"

  if [[ "$local_sha" == "$remote_sha" ]]; then
    echo "up to date at $local_sha"
    return 0
  fi

  echo "updating $local_sha -> $remote_sha"
  git -C "$REPO_DIR" reset --hard origin/main

  if ! build_worker; then
    echo "build failed at $remote_sha, leaving previous worker binary running" >&2
    return 1
  fi

  mv "$REPO_DIR/backend/worker.new" "$REPO_DIR/backend/worker"
  systemctl restart "$WORKER_SERVICE"
  echo "deployed $remote_sha"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  deploy
fi
```

- [ ] **Step 2: Make it executable**

Run: `chmod +x deploy/raspberry-pi/deploy.sh`

- [ ] **Step 3: Write the test suite (fixture repo + stubbed systemctl, no real Pi/systemd needed)**

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

fail() { echo "FAIL: $1" >&2; exit 1; }

# --- fixture: bare "origin" repo + a local checkout playing the role of /opt/ff-sims ---
ORIGIN="$WORK/origin.git"
REPO="$WORK/repo"
git init --bare -q "$ORIGIN"
git init -q "$REPO"
git -C "$REPO" checkout -q -b main
git -C "$REPO" config user.email test@example.com
git -C "$REPO" config user.name test

mkdir -p "$REPO/backend/cmd/worker" "$REPO/deploy/raspberry-pi"
cp "$SCRIPT_DIR/../deploy.sh" "$REPO/deploy/raspberry-pi/deploy.sh"
chmod +x "$REPO/deploy/raspberry-pi/deploy.sh"

cat > "$REPO/backend/go.mod" <<'EOF'
module backend

go 1.21
EOF
cat > "$REPO/backend/cmd/worker/main.go" <<'EOF'
package main

func main() {}
EOF

git -C "$REPO" add -A
git -C "$REPO" commit -q -m "initial"
git -C "$REPO" remote add origin "$ORIGIN"
git -C "$REPO" push -q -u origin main

# --- stub systemctl so we can assert on restart calls without real systemd ---
BIN="$WORK/bin"
mkdir -p "$BIN"
CALLS="$WORK/systemctl_calls"
: > "$CALLS"
cat > "$BIN/systemctl" <<EOF
#!/usr/bin/env bash
echo "\$@" >> "$CALLS"
EOF
chmod +x "$BIN/systemctl"
export PATH="$BIN:$PATH"
export REPO_DIR="$REPO"

# --- scenario 1: no new commits -> no rebuild, no restart ---
bash "$REPO/deploy/raspberry-pi/deploy.sh"
[[ ! -f "$REPO/backend/worker" ]] || fail "should not have built a worker binary when up to date"
[[ ! -s "$CALLS" ]] || fail "systemctl should not have been called when up to date"

# --- scenario 2: a new good commit -> rebuild + restart ---
CLONE="$WORK/clone"
git clone -q "$ORIGIN" "$CLONE"
cat > "$CLONE/backend/cmd/worker/main.go" <<'EOF'
package main

func main() { println("v2") }
EOF
git -C "$CLONE" -c user.email=test@example.com -c user.name=test commit -aqm "v2"
git -C "$CLONE" push -q origin main

bash "$REPO/deploy/raspberry-pi/deploy.sh"
[[ -x "$REPO/backend/worker" ]] || fail "expected a worker binary to be built"
grep -q "restart ff-sims-worker.service" "$CALLS" || fail "expected systemctl restart to be called"

# --- scenario 3: a new commit that fails to compile -> old binary + service left alone ---
old_hash="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
: > "$CALLS"
cat > "$CLONE/backend/cmd/worker/main.go" <<'EOF'
package main

func main() { this is not valid go }
EOF
git -C "$CLONE" -c user.email=test@example.com -c user.name=test commit -aqm "broken"
git -C "$CLONE" push -q origin main

if bash "$REPO/deploy/raspberry-pi/deploy.sh"; then
  fail "deploy.sh should exit non-zero on a build failure"
fi
new_hash="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
[[ "$old_hash" == "$new_hash" ]] || fail "worker binary should be unchanged after a failed build"
[[ ! -s "$CALLS" ]] || fail "systemctl should not have been called after a failed build"

echo "PASS: deploy.sh integration tests"
```

- [ ] **Step 4: Run the test and verify it passes**

Run: `chmod +x deploy/raspberry-pi/tests/test_deploy.sh && bash deploy/raspberry-pi/tests/test_deploy.sh`
Expected: `PASS: deploy.sh integration tests` with no `FAIL:` lines.

- [ ] **Step 5: Commit**

```bash
git add deploy/raspberry-pi/deploy.sh deploy/raspberry-pi/tests/test_deploy.sh
git commit -m "feat(pi-deploy): add self-update deploy script for Pi worker"
```

---

### Task 2: `setup.sh` + systemd unit templates

**Files:**
- Create: `deploy/raspberry-pi/setup.sh`
- Create: `deploy/raspberry-pi/ff-sims-worker.service`
- Create: `deploy/raspberry-pi/ff-sims-deploy.service`
- Create: `deploy/raspberry-pi/ff-sims-deploy.timer`
- Test: `deploy/raspberry-pi/tests/test_setup.sh`

**Interfaces:**
- Consumes: `deploy/raspberry-pi/deploy.sh` from Task 1 (referenced by absolute path in the installed `ff-sims-deploy.service` unit; must be executable).
- Produces: `go_arch_for_uname()`, `write_env_template()`, `env_file_is_placeholder()`, `ensure_env_file()`, `main()` — guarded the same way as Task 1 (`BASH_SOURCE[0] == $0`) so tests can source it without running `main`. Installs systemd units named exactly `ff-sims-worker.service`, `ff-sims-deploy.service`, `ff-sims-deploy.timer` — Task 3's Makefile target and README reference these same names.

- [ ] **Step 1: Write the systemd unit templates**

`deploy/raspberry-pi/ff-sims-worker.service`:
```ini
[Unit]
Description=ff-sims Temporal worker
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User={{SERVICE_USER}}
WorkingDirectory={{REPO_DIR}}/backend
EnvironmentFile=/etc/ff-sims-worker.env
ExecStart={{REPO_DIR}}/backend/worker
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

`deploy/raspberry-pi/ff-sims-deploy.service`:
```ini
[Unit]
Description=ff-sims Pi self-update check
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
WorkingDirectory={{REPO_DIR}}
ExecStart={{REPO_DIR}}/deploy/raspberry-pi/deploy.sh
```

`deploy/raspberry-pi/ff-sims-deploy.timer`:
```ini
[Unit]
Description=Run ff-sims-deploy every 5 minutes

[Timer]
OnBootSec=2min
OnUnitActiveSec=5min
Unit=ff-sims-deploy.service

[Install]
WantedBy=timers.target
```

Note: `ff-sims-deploy.service` has no `User=` line (runs as root), since `setup.sh` itself requires `sudo` and it needs to call `systemctl restart` on the worker unit. `ff-sims-worker.service` runs as the unprivileged `{{SERVICE_USER}}` — the built binary is world-executable by default (`go build`'s default `0755`), so the root-run deploy step doesn't need to `chown` it for the unprivileged worker service to run it.

- [ ] **Step 2: Write `setup.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
GO_VERSION="1.25.7"
ENV_FILE="/etc/ff-sims-worker.env"
PLACEHOLDER_MARKER="# PI-SETUP-PLACEHOLDER"
SERVICE_USER="ffsims"
SYSTEMD_DIR="/etc/systemd/system"

go_arch_for_uname() {
  case "$1" in
    aarch64) echo "arm64" ;;
    armv6l|armv7l) echo "armv6l" ;;
    x86_64) echo "amd64" ;;
    *) return 1 ;;
  esac
}

ensure_go() {
  if [[ -x /usr/local/go/bin/go ]] && /usr/local/go/bin/go version | grep -q "go${GO_VERSION}"; then
    echo "Go ${GO_VERSION} already installed"
    return
  fi
  local go_arch
  go_arch="$(go_arch_for_uname "$(uname -m)")" || { echo "unsupported architecture: $(uname -m)" >&2; exit 1; }
  echo "Installing Go ${GO_VERSION} (${go_arch})"
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${go_arch}.tar.gz" -o /tmp/go.tar.gz
  rm -rf /usr/local/go
  tar -C /usr/local -xzf /tmp/go.tar.gz
  rm -f /tmp/go.tar.gz
}

ensure_service_user() {
  if ! id -u "$SERVICE_USER" &>/dev/null; then
    echo "Creating service user $SERVICE_USER"
    useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
  fi
}

write_env_template() {
  local target="$1"
  cat > "$target" <<EOF
${PLACEHOLDER_MARKER}
# Fill in real values below, then re-run \`make pi-setup\`.
DATABASE_URL=postgres://REPLACE_ME
TEMPORAL_NAMESPACE_ENDPOINT=REPLACE_ME.tmprl.cloud:7233
TEMPORAL_NAMESPACE=REPLACE_ME
TEMPORAL_API_KEY=REPLACE_ME
EOF
  chmod 600 "$target"
}

env_file_is_placeholder() {
  local target="$1"
  [[ -f "$target" ]] && grep -q "^${PLACEHOLDER_MARKER}$" "$target"
}

ensure_env_file() {
  if [[ ! -f "$ENV_FILE" ]]; then
    write_env_template "$ENV_FILE"
    echo ""
    echo "Wrote placeholder env file to $ENV_FILE — edit it with real values, then re-run 'make pi-setup'."
    return 1
  fi
  if env_file_is_placeholder "$ENV_FILE"; then
    echo ""
    echo "$ENV_FILE still has placeholder values — edit it with real values, then re-run 'make pi-setup'."
    return 1
  fi
  return 0
}

first_build() {
  echo "Building worker binary"
  (cd "$REPO_DIR/backend" && /usr/local/go/bin/go build -o worker ./cmd/worker)
}

install_units() {
  echo "Installing systemd units"
  for unit in ff-sims-worker.service ff-sims-deploy.service ff-sims-deploy.timer; do
    sed "s#{{REPO_DIR}}#${REPO_DIR}#g; s#{{SERVICE_USER}}#${SERVICE_USER}#g" \
      "$SCRIPT_DIR/$unit" > "$SYSTEMD_DIR/$unit"
  done
  systemctl daemon-reload
}

print_summary() {
  local ip
  ip="$(curl -fsSL ifconfig.me || echo "<could not detect>")"
  cat <<EOF

Setup complete.

Pi public IP: ${ip}
  -> Add this IP to the Postgres managed database's trusted sources
     in the DigitalOcean dashboard if you haven't already.

Logs:
  journalctl -u ff-sims-worker -f      # worker logs
  journalctl -u ff-sims-deploy         # deploy-check history
EOF
}

main() {
  ensure_go
  ensure_service_user
  first_build
  install_units

  if ensure_env_file; then
    systemctl enable --now ff-sims-worker.service ff-sims-deploy.timer
  else
    echo "Skipping service start until $ENV_FILE is filled in."
  fi

  print_summary
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
```

- [ ] **Step 3: Make it executable**

Run: `chmod +x deploy/raspberry-pi/setup.sh`

- [ ] **Step 4: Write the test suite (pure-function unit tests, no root/network/systemd needed)**

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../setup.sh"

fail() { echo "FAIL: $1" >&2; exit 1; }

# go_arch_for_uname
[[ "$(go_arch_for_uname aarch64)" == "arm64" ]] || fail "aarch64 -> arm64"
[[ "$(go_arch_for_uname armv7l)" == "armv6l" ]] || fail "armv7l -> armv6l"
[[ "$(go_arch_for_uname x86_64)" == "amd64" ]] || fail "x86_64 -> amd64"
if go_arch_for_uname riscv64 &>/dev/null; then fail "riscv64 should be unsupported"; fi

# env file template + placeholder detection
tmp_env="$(mktemp)"
write_env_template "$tmp_env"
env_file_is_placeholder "$tmp_env" || fail "freshly written template should be a placeholder"

perm=$(stat -f "%OLp" "$tmp_env" 2>/dev/null || stat -c "%a" "$tmp_env")
[[ "$perm" == "600" ]] || fail "env file should be mode 600, got $perm"

marker_escaped=$(printf '%s\n' "$PLACEHOLDER_MARKER" | sed 's/[.[\*^$/]/\\&/g')
sed -i.bak "/${marker_escaped}/d" "$tmp_env" && rm -f "${tmp_env}.bak"
if env_file_is_placeholder "$tmp_env"; then fail "template with marker removed should not be a placeholder"; fi

rm -f "$tmp_env"
echo "PASS: setup.sh unit tests"
```

- [ ] **Step 5: Run the test and verify it passes**

Run: `chmod +x deploy/raspberry-pi/tests/test_setup.sh && bash deploy/raspberry-pi/tests/test_setup.sh`
Expected: `PASS: setup.sh unit tests` with no `FAIL:` lines.

- [ ] **Step 6: Commit**

```bash
git add deploy/raspberry-pi/setup.sh deploy/raspberry-pi/ff-sims-worker.service \
  deploy/raspberry-pi/ff-sims-deploy.service deploy/raspberry-pi/ff-sims-deploy.timer \
  deploy/raspberry-pi/tests/test_setup.sh
git commit -m "feat(pi-deploy): add setup.sh and systemd unit templates for Pi worker"
```

---

### Task 3: `make pi-setup` target + README

**Files:**
- Modify: `Makefile:1-2` (the `.PHONY` line and adding a new target)
- Create: `deploy/raspberry-pi/README.md`

**Interfaces:**
- Consumes: `deploy/raspberry-pi/setup.sh` from Task 2 (invoked via `sudo ./deploy/raspberry-pi/setup.sh`).
- Produces: nothing consumed by later tasks — this is the last task.

- [ ] **Step 1: Add the `pi-setup` target to the root Makefile**

In `Makefile`, change:
```makefile
.PHONY: help docker-run
```
to:
```makefile
.PHONY: help docker-run pi-setup
```

Then add, after the `docker-stop` target at the end of the file:
```makefile

pi-setup: ## Set up this Pi as a Temporal worker host (run on the Pi itself, with sudo)
	sudo ./deploy/raspberry-pi/setup.sh
```

- [ ] **Step 2: Write `deploy/raspberry-pi/README.md`**

```markdown
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
   `ff-sims-deploy.timer`, and prints the Pi's public IP.
6. Add that IP to the Postgres managed database's trusted sources in the DigitalOcean
   dashboard — the worker can't reach the database until you do.

`make pi-setup` is safe to re-run at any point (e.g. after fixing the env file, or after a
full reinstall) — it picks up wherever it left off.

## Operating

- Worker logs: `journalctl -u ff-sims-worker -f`
- Deploy-check history (whether it found a new commit, built, restarted): `journalctl -u ff-sims-deploy`
- Force an immediate deploy check without waiting for the timer: `sudo systemctl start ff-sims-deploy.service`
- This runs the *same* `backend/cmd/worker` binary as production, so it polls all six
  Temporal task queues (discovery, drafts, transactions, player-sync, week-stats, ADP), not
  just discovery/transactions — the idle pollers on the other queues cost nothing.
```

- [ ] **Step 3: Syntax-check both scripts and dry-run the Makefile target**

Run: `bash -n deploy/raspberry-pi/setup.sh && bash -n deploy/raspberry-pi/deploy.sh && echo "syntax OK"`
Expected: `syntax OK` with no errors.

Run: `make -n pi-setup`
Expected: prints `sudo ./deploy/raspberry-pi/setup.sh` (dry run — does not execute it).

- [ ] **Step 4: Re-run both test suites to confirm nothing regressed**

Run: `bash deploy/raspberry-pi/tests/test_deploy.sh && bash deploy/raspberry-pi/tests/test_setup.sh`
Expected: both print their `PASS:` line.

- [ ] **Step 5: Commit**

```bash
git add Makefile deploy/raspberry-pi/README.md
git commit -m "feat(pi-deploy): add make pi-setup target and Pi worker README"
```
