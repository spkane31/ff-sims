#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
GO_VERSION="1.25.7"
ENV_FILE="/etc/ff-sims-worker.env"
PLACEHOLDER_MARKER="# WORKER-HOST-SETUP-PLACEHOLDER"
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

# Installed to a fixed system-wide path (not the per-user default of
# ~/.local/bin) so ff-sims-espn-worker.service's ExecStart can reference it by
# absolute path regardless of which user runs setup.sh vs. which user
# (SERVICE_USER) the service itself runs as.
ensure_uv() {
  if [[ -x /usr/local/bin/uv ]]; then
    echo "uv already installed ($(/usr/local/bin/uv --version))"
    return
  fi
  echo "Installing uv"
  curl -LsSf https://astral.sh/uv/install.sh | env UV_INSTALL_DIR=/usr/local/bin sh
}

# Masking these targets prevents the host from suspending/hibernating out from
# under the worker and deploy timer. Idempotent and harmless to re-run.
disable_sleep() {
  echo "Disabling sleep/suspend/hibernate targets"
  systemctl mask sleep.target suspend.target hibernate.target hybrid-sleep.target
}

write_env_template() {
  local target="$1"
  cat > "$target" <<EOF
${PLACEHOLDER_MARKER}
# Fill in real values below, then re-run \`make worker-host-setup\`.
DATABASE_URL=postgres://REPLACE_ME
TEMPORAL_NAMESPACE_ENDPOINT=REPLACE_ME.tmprl.cloud:7233
TEMPORAL_NAMESPACE=REPLACE_ME
TEMPORAL_API_KEY=REPLACE_ME

# Local archive Postgres (complete Sleeper history). Filled in from the URL
# that \`make worker-host-setup-archive-db\` prints — see README step 7.
# REQUIRED by ff-sims-player-valuations.service, which reads all of its model
# inputs from here and fails fast without it.
# Left EMPTY rather than REPLACE_ME on purpose: the Go worker and cron treat an
# empty value as "archive disabled" and keep running, whereas a placeholder URL
# would send them dialing a host that does not exist.
ARCHIVE_DATABASE_URL=
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
    echo "Wrote placeholder env file to $ENV_FILE — edit it with real values, then re-run 'make worker-host-setup'."
    return 1
  fi
  if env_file_is_placeholder "$ENV_FILE"; then
    echo ""
    echo "$ENV_FILE still has placeholder values — edit it with real values, then re-run 'make worker-host-setup'."
    return 1
  fi
  return 0
}

# The archive database is provisioned by a SEPARATE, later step
# (make worker-host-setup-archive-db, README step 7), so ARCHIVE_DATABASE_URL
# ships blank in the env template and a host can be fully, legitimately set up
# without it — the Go worker and cron just run with the archive disabled.
# ff-sims-player-valuations.service cannot: it reads every model input from the
# archive. Arming its timer before the URL exists would buy a failed unit every
# midnight, so the timer waits for a non-empty value instead.
archive_url_configured() {
  local target="${1:-$ENV_FILE}"
  [[ -f "$target" ]] || return 1
  grep -qE '^[[:space:]]*ARCHIVE_DATABASE_URL=[^[:space:]]+' "$target"
}

first_build() {
  echo "Building worker binary"
  local sha full_sha
  sha="$(git -C "$REPO_DIR" rev-parse --short=9 HEAD)"
  full_sha="$(git -C "$REPO_DIR" rev-parse HEAD)"
  (cd "$REPO_DIR/backend" && /usr/local/go/bin/go build -ldflags "-X 'main.buildID=${sha}' -X 'main.promoteOnStart=true'" -o worker ./cmd/worker)
  echo "Building cron binary"
  (cd "$REPO_DIR/backend" && /usr/local/go/bin/go build -ldflags "-X 'main.buildID=${sha}'" -o cron ./cmd/cron)

  # Seed deploy.sh's per-binary "last built" state so the first periodic
  # deploy check has a real baseline instead of forcing an immediate,
  # redundant rebuild on the very next cycle.
  echo "$full_sha" > "$REPO_DIR/backend/.worker-deployed-sha"
  echo "$full_sha" > "$REPO_DIR/backend/.cron-deployed-sha"
}

first_sync_espn() {
  echo "Syncing ESPN worker dependencies"
  local full_sha
  full_sha="$(git -C "$REPO_DIR" rev-parse HEAD)"
  (cd "$REPO_DIR/workers/espn" && /usr/local/bin/uv sync --frozen --no-dev)

  # Same baseline-seeding purpose as first_build's sha files, above.
  echo "$full_sha" > "$REPO_DIR/workers/espn/.espn-deployed-sha"
}

first_sync_analysis() {
  echo "Syncing player-valuation model dependencies"
  local full_sha
  full_sha="$(git -C "$REPO_DIR" rev-parse HEAD)"
  (cd "$REPO_DIR/analysis" && /usr/local/bin/uv sync --frozen --no-dev)

  # Same baseline-seeding purpose as first_build's sha files, above.
  echo "$full_sha" > "$REPO_DIR/analysis/.analysis-deployed-sha"
}

install_units() {
  echo "Installing systemd units"
  for unit in ff-sims-worker.service ff-sims-espn-worker.service ff-sims-deploy.service ff-sims-deploy.timer ff-sims-discovery.service ff-sims-discovery.timer ff-sims-lifetime-counts.service ff-sims-lifetime-counts.timer ff-sims-transactions.service ff-sims-transactions.timer ff-sims-player-valuations.service ff-sims-player-valuations.timer; do
    sed "s#{{REPO_DIR}}#${REPO_DIR}#g; s#{{SERVICE_USER}}#${SERVICE_USER}#g" \
      "$SCRIPT_DIR/$unit" > "$SYSTEMD_DIR/$unit"
  done
  systemctl daemon-reload
}

print_summary() {
  local ip
  ip="$(curl -4 -fsSL ifconfig.me || echo "<could not detect>")"
  cat <<EOF

Setup complete.

Worker host public IP: ${ip}
  -> Add this IP to the Postgres managed database's trusted sources
     in the DigitalOcean dashboard if you haven't already.

Logs:
  journalctl -u ff-sims-worker -f      # Go Temporal worker logs (drafts, etc.)
  journalctl -u ff-sims-espn-worker -f # Python ESPN Temporal worker logs
  journalctl -u ff-sims-deploy         # deploy-check history
  journalctl -u ff-sims-discovery -f   # discovery cron job logs (runs hourly)
  journalctl -u ff-sims-lifetime-counts -f   # lifetime-counts snapshot job logs (runs hourly)
  journalctl -u ff-sims-transactions -f      # transaction-sync cron job logs (runs every ~10min)
  journalctl -u ff-sims-player-valuations -f # player-valuation replay logs (runs daily at 00:00 UTC)

$(if archive_url_configured; then cat <<'ARMED'
The player-valuation timer is armed but does NOT run on start. Kick off the
first (full-season) replay yourself when you can watch it:
  sudo systemctl start ff-sims-player-valuations.service
ARMED
else cat <<DISARMED
The player-valuation timer is NOT armed: ARCHIVE_DATABASE_URL is empty in
${ENV_FILE}. Run 'make worker-host-setup-archive-db', add the URL it prints,
then re-run 'make worker-host-setup'.
DISARMED
fi)
EOF
}

main() {
  ensure_go
  ensure_uv
  ensure_service_user
  disable_sleep
  first_build
  first_sync_espn
  first_sync_analysis
  install_units

  if ensure_env_file; then
    systemctl enable ff-sims-worker.service ff-sims-espn-worker.service ff-sims-deploy.timer ff-sims-discovery.timer ff-sims-lifetime-counts.timer ff-sims-transactions.timer
    systemctl start ff-sims-worker.service ff-sims-espn-worker.service ff-sims-deploy.timer ff-sims-discovery.timer ff-sims-lifetime-counts.timer ff-sims-transactions.timer

    # Gated separately: everything above runs fine without the archive DB.
    # Converges either way, since this script is meant to be re-run — filling
    # ARCHIVE_DATABASE_URL in and re-running arms the timer, blanking it out
    # and re-running disarms it rather than leaving a unit that fails nightly.
    if archive_url_configured; then
      # Starting a .timer only arms it; ff-sims-player-valuations.service
      # itself is intentionally not started here (see the timer unit).
      systemctl enable ff-sims-player-valuations.timer
      systemctl start ff-sims-player-valuations.timer
    else
      systemctl stop ff-sims-player-valuations.timer 2>/dev/null || true
      systemctl disable ff-sims-player-valuations.timer 2>/dev/null || true
      echo ""
      echo "ARCHIVE_DATABASE_URL is empty in $ENV_FILE — leaving ff-sims-player-valuations.timer disabled."
      echo "  Run 'make worker-host-setup-archive-db', put the URL it prints in $ENV_FILE,"
      echo "  then re-run 'make worker-host-setup' to arm the daily valuation replay."
    fi
  else
    echo "Skipping service start until $ENV_FILE is filled in."
  fi

  print_summary
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
