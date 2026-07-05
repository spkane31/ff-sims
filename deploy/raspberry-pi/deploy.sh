#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="${REPO_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
WORKER_SERVICE="${WORKER_SERVICE:-ff-sims-worker.service}"
GO_BIN="${GO_BIN:-/usr/local/go/bin/go}"
[[ -x "$GO_BIN" ]] || GO_BIN="go"

current_and_remote_sha() {
  git -C "$REPO_DIR" fetch origin main --quiet
  local local_sha remote_sha
  local_sha=$(git -C "$REPO_DIR" rev-parse HEAD)
  remote_sha=$(git -C "$REPO_DIR" rev-parse origin/main)
  echo "$local_sha $remote_sha"
}

build_worker() {
  local sha
  sha="$(git -C "$REPO_DIR" rev-parse --short=9 HEAD)"
  (cd "$REPO_DIR/backend" && "$GO_BIN" build -ldflags "-X 'main.buildID=${sha}'" -o worker.new ./cmd/worker)
}

install_and_restart() {
  local sha
  sha="$(git -C "$REPO_DIR" rev-parse HEAD)"

  if ! build_worker; then
    echo "build failed at $sha, leaving previous worker binary running" >&2
    return 1
  fi

  mv "$REPO_DIR/backend/worker.new" "$REPO_DIR/backend/worker"
  systemctl restart "$WORKER_SERVICE"
  echo "deployed $sha"
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

  # Re-exec into the freshly-pulled script instead of continuing in this process:
  # build_worker (and this function) were already parsed from the OLD file before
  # the reset above, so calling them in-process would silently build with stale,
  # pre-update logic on the exact deploy that changes them. This bit the Worker
  # Deployment Versioning rollout: the commit that added ldflags to build_worker
  # was itself applied using the old, ldflags-less build_worker, so that cycle's
  # binary reported the Go source default build ID instead of a real git SHA.
  exec "${BASH_SOURCE[0]}" --build-only
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  if [[ "${1:-}" == "--build-only" ]]; then
    install_and_restart
  else
    deploy
  fi
fi
