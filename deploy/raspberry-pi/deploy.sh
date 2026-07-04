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
