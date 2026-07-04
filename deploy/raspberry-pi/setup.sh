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
