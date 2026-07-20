#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="${REPO_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
WORKER_SERVICE="${WORKER_SERVICE:-ff-sims-worker.service}"
ESPN_SERVICE="${ESPN_SERVICE:-ff-sims-espn-worker.service}"
GO_BIN="${GO_BIN:-/usr/local/go/bin/go}"
[[ -x "$GO_BIN" ]] || GO_BIN="go"
UV_BIN="${UV_BIN:-/usr/local/bin/uv}"
[[ -x "$UV_BIN" ]] || UV_BIN="uv"

# Persists the sha each binary/service was last actually deployed from.
# Needed because git reset --hard (in deploy(), below) already advances HEAD
# to the new remote sha before install_and_restart can run, and
# install_and_restart runs in a re-exec'd process that doesn't share
# deploy()'s local variables — so without this, there'd be no way to know
# what "previously deployed" means. These live as untracked files next to
# what they track (see backend/.gitignore, workers/espn/.gitignore) and are
# only updated after a successful build/sync, so a failure doesn't get
# silently treated as "already deployed" on the next cycle.
WORKER_SHA_FILE="$REPO_DIR/backend/.worker-deployed-sha"
CRON_SHA_FILE="$REPO_DIR/backend/.cron-deployed-sha"
ESPN_SHA_FILE="$REPO_DIR/workers/espn/.espn-deployed-sha"

current_and_remote_sha() {
  # `|| return 1` is required, not decorative: this function runs inside a
  # $(...) command substitution (see deploy()), and bash does not inherit
  # -e/errexit into command substitutions by default. Without an explicit
  # check here, a failed `git fetch` (e.g. a broken SSH key) falls through
  # silently to rev-parse'ing stale cached refs, and deploy() would report
  # "up to date" without ever having actually checked origin/main.
  git -C "$REPO_DIR" fetch origin main --quiet || return 1
  local local_sha remote_sha
  local_sha=$(git -C "$REPO_DIR" rev-parse HEAD)
  remote_sha=$(git -C "$REPO_DIR" rev-parse origin/main)
  echo "$local_sha $remote_sha"
}

# Prints whether $2 (new sha) touches anything $3 (a Go package pattern, e.g.
# ./cmd/worker) actually depends on, diffed against $1 (old sha). The
# dependency set is computed fresh via `go list -deps` against the
# already-updated checkout, rather than a hand-maintained path list, so it
# can't silently go stale if worker/cron start importing a new internal
# package — a false negative here (skipping a rebuild the binary actually
# needed) is worse than a false positive (an unnecessary rebuild).
relevant_changed() {
  local old_sha="$1" new_sha="$2" pkg="$3"
  local deps
  if ! deps="$(cd "$REPO_DIR/backend" && "$GO_BIN" list -deps -f '{{.ImportPath}}' "$pkg" 2>&1)"; then
    echo "warning: 'go list -deps $pkg' failed, assuming changes are relevant: $deps" >&2
    return 0
  fi

  local pathspecs=() dep
  while IFS= read -r dep; do
    [[ "$dep" == backend/* ]] && pathspecs+=("$dep")
  done <<< "$deps"
  # go.mod/go.sum aren't Go packages so `go list -deps` never names them, but
  # a dependency version bump can change any imported package's behavior
  # without touching a single .go file.
  pathspecs+=("backend/go.mod" "backend/go.sum")

  [[ -n "$(git -C "$REPO_DIR" diff --name-only "$old_sha" "$new_sha" -- "${pathspecs[@]}")" ]]
}

build_worker() {
  local sha
  sha="$(git -C "$REPO_DIR" rev-parse --short=9 HEAD)"
  (cd "$REPO_DIR/backend" && "$GO_BIN" build -ldflags "-X 'main.buildID=${sha}' -X 'main.promoteOnStart=true'" -o worker.new ./cmd/worker)
}

build_cron() {
  local sha
  sha="$(git -C "$REPO_DIR" rev-parse --short=9 HEAD)"
  (cd "$REPO_DIR/backend" && "$GO_BIN" build -ldflags "-X 'main.buildID=${sha}'" -o cron.new ./cmd/cron)
}

# Simpler than relevant_changed: the ESPN worker has no compiled binary and no
# `go list -deps`-style dependency graph to walk, so this just checks whether
# anything under workers/espn actually changed between the two shas.
espn_relevant_changed() {
  local old_sha="$1" new_sha="$2"
  [[ -n "$(git -C "$REPO_DIR" diff --name-only "$old_sha" "$new_sha" -- workers/espn)" ]]
}

sync_espn_worker() {
  (cd "$REPO_DIR/workers/espn" && "$UV_BIN" sync --frozen --no-dev)
}

install_and_restart() {
  local sha last_worker_sha last_cron_sha last_espn_sha
  sha="$(git -C "$REPO_DIR" rev-parse HEAD)"
  last_worker_sha=""
  last_cron_sha=""
  last_espn_sha=""
  [[ -f "$WORKER_SHA_FILE" ]] && last_worker_sha="$(<"$WORKER_SHA_FILE")"
  [[ -f "$CRON_SHA_FILE" ]] && last_cron_sha="$(<"$CRON_SHA_FILE")"
  [[ -f "$ESPN_SHA_FILE" ]] && last_espn_sha="$(<"$ESPN_SHA_FILE")"

  local rebuild_worker=1 rebuild_cron=1 rebuild_espn=1
  if [[ -n "$last_worker_sha" ]] && ! relevant_changed "$last_worker_sha" "$sha" ./cmd/worker; then
    rebuild_worker=0
  fi
  if [[ -n "$last_cron_sha" ]] && ! relevant_changed "$last_cron_sha" "$sha" ./cmd/cron; then
    rebuild_cron=0
  fi
  if [[ -n "$last_espn_sha" ]] && ! espn_relevant_changed "$last_espn_sha" "$sha"; then
    rebuild_espn=0
  fi

  if [[ "$rebuild_worker" -eq 0 ]]; then
    echo "worker: up to date, no worker-relevant changes since ${last_worker_sha:0:5}"
  elif ! build_worker; then
    echo "build failed at $sha, leaving previous worker binary running" >&2
    return 1
  fi

  if [[ "$rebuild_cron" -eq 0 ]]; then
    echo "cron: up to date, no cron-relevant changes since ${last_cron_sha:0:5}"
  elif ! build_cron; then
    echo "build failed at $sha, leaving previous cron binary in place" >&2
    return 1
  fi

  if [[ "$rebuild_espn" -eq 0 ]]; then
    echo "espn-worker: up to date, no changes since ${last_espn_sha:0:5}"
  elif ! sync_espn_worker; then
    echo "uv sync failed at $sha, leaving previous espn-worker deps in place" >&2
    return 1
  fi

  if [[ "$rebuild_worker" -eq 1 ]]; then
    mv "$REPO_DIR/backend/worker.new" "$REPO_DIR/backend/worker"
    systemctl restart "$WORKER_SERVICE"
    echo "$sha" > "$WORKER_SHA_FILE"
    echo "deployed worker $sha"
  fi

  if [[ "$rebuild_cron" -eq 1 ]]; then
    mv "$REPO_DIR/backend/cron.new" "$REPO_DIR/backend/cron"
    echo "$sha" > "$CRON_SHA_FILE"
    echo "deployed cron $sha"
  fi

  if [[ "$rebuild_espn" -eq 1 ]]; then
    systemctl restart "$ESPN_SERVICE"
    echo "$sha" > "$ESPN_SHA_FILE"
    echo "deployed espn-worker $sha"
  fi
}

deploy() {
  local shas local_sha remote_sha
  if ! shas="$(current_and_remote_sha)"; then
    echo "failed to fetch/compare origin/main (see git error above); leaving previous deploy in place" >&2
    return 1
  fi
  read -r local_sha remote_sha <<< "$shas"

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
