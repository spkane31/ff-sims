#!/usr/bin/env bash
set -euo pipefail

# Regression test: ff-sims-deploy.service once pointed ExecStart at
# deploy/raspberry-pi/deploy.sh after that directory was renamed to
# deploy/worker-host/ (T1, #153) without updating the unit file's own
# self-reference. install_units() in setup.sh only substitutes the
# {{REPO_DIR}}/{{SERVICE_USER}} placeholders — it never validates that the
# resulting paths exist — so a stale path like that silently ships and the
# 5-minute deploy timer fails forever instead of ever rebuilding the worker.
# This asserts every deploy/-relative ExecStart target in every unit file
# actually exists in the repo.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNIT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_DIR="$(cd "$UNIT_DIR/../.." && pwd)"

fail() { echo "FAIL: $1" >&2; exit 1; }

checked=0
for unit in "$UNIT_DIR"/*.service; do
  exec_line="$(grep -h '^ExecStart=' "$unit" || true)"
  [[ -n "$exec_line" ]] || continue

  target="${exec_line#ExecStart=}"
  target="${target%% *}"                       # drop any ExecStart args
  target="${target//\{\{REPO_DIR\}\}/$REPO_DIR}"

  case "$target" in
    "$REPO_DIR"/deploy/*)
      checked=$((checked + 1))
      [[ -f "$target" ]] || fail "$(basename "$unit"): ExecStart target does not exist: $target"
      ;;
  esac
done

[[ "$checked" -gt 0 ]] || fail "no deploy/-relative ExecStart targets found — test fixture is stale"

echo "PASS: unit file ExecStart targets ($checked checked)"
