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

# Every .timer must name a [Timer] section, a schedule, and a Unit= that
# actually exists next to it — a typo in any of these makes systemd load the
# timer and then never fire anything, with no error at install time.
timers=0
for timer in "$UNIT_DIR"/*.timer; do
  timers=$((timers + 1))
  name="$(basename "$timer")"
  grep -q '^\[Timer\]' "$timer" || fail "$name: missing [Timer] section"
  grep -qE '^(OnCalendar|OnBootSec|OnUnitActiveSec)=' "$timer" || fail "$name: no schedule directive"
  grep -q '^WantedBy=timers.target' "$timer" || fail "$name: not installable into timers.target"

  target="$(grep -h '^Unit=' "$timer" | head -1)"
  target="${target#Unit=}"
  [[ -n "$target" ]] || fail "$name: no Unit= line"
  [[ -f "$UNIT_DIR/$target" ]] || fail "$name: Unit=$target has no matching unit file"
done
[[ "$timers" -gt 0 ]] || fail "no timer units found — test fixture is stale"

# The player-valuation job: assert the scheduled invocation is exactly the one
# the plan pins (single segment, single season, fixed start, daily step), that
# placeholder substitution produces a real interpreter path, and that midnight
# UTC is not left to the host's local timezone.
pv_service="$UNIT_DIR/ff-sims-player-valuations.service"
pv_timer="$UNIT_DIR/ff-sims-player-valuations.timer"
[[ -f "$pv_service" && -f "$pv_timer" ]] || fail "player-valuation units are missing"

pv_exec="$(grep -h '^ExecStart=' "$pv_service")"
pv_exec="${pv_exec#ExecStart=}"
for arg in "--segment ppr-sf-10" "--season 2025" "--start 2025-08-25" "--step 24h"; do
  [[ "$pv_exec" == *"$arg"* ]] || fail "player-valuation ExecStart is missing '$arg'"
done
[[ "$pv_exec" != *"--end"* ]] || fail "player-valuation ExecStart must leave --end at its rolling default"
[[ "$pv_exec" != *"--backtest"* ]] || fail "player-valuation ExecStart must not use the removed --backtest mode"

substituted="${pv_exec//\{\{REPO_DIR\}\}/$REPO_DIR}"
[[ "$substituted" != *"{{"* ]] || fail "player-valuation ExecStart has unsubstituted placeholders: $substituted"
[[ "$substituted" == "$REPO_DIR/analysis/.venv/bin/python main.py"* ]] \
  || fail "player-valuation ExecStart should run the analysis venv's python: $substituted"

grep -q '^Environment=TZ=UTC' "$pv_service" || fail "player-valuation service must pin TZ=UTC"
# Without this, Python block-buffers its pipe to journald and every progress
# line lands at exit — no way to tell a slow replay from a stuck one.
grep -q '^Environment=PYTHONUNBUFFERED=1' "$pv_service" \
  || fail "player-valuation service must run Python unbuffered so progress reaches the journal"
grep -q '^TimeoutStartSec=2h' "$pv_service" || fail "player-valuation service must allow a 2h full replay"
grep -q '^OnCalendar=\*-\*-\* 00:00:00 UTC' "$pv_timer" || fail "player-valuation timer must fire at 00:00 UTC"
grep -q '^Persistent=' "$pv_timer" && fail "player-valuation timer must not catch up missed runs with a full replay"
grep -q '^OnBootSec=' "$pv_timer" && fail "player-valuation timer must not replay on boot"

# Both units have to be installed and enabled, or the timer never ships.
setup="$UNIT_DIR/setup.sh"
grep -q 'ff-sims-player-valuations.service' "$setup" || fail "setup.sh does not install the player-valuation service"
# Anchored to a real command line, so the `sudo systemctl start ...` hint in
# print_summary's heredoc is not mistaken for an actual invocation.
grep -qE '^[[:space:]]*systemctl enable .*ff-sims-player-valuations\.timer' "$setup" || fail "setup.sh does not enable the player-valuation timer"
grep -qE '^[[:space:]]*systemctl start .*ff-sims-player-valuations\.timer' "$setup" || fail "setup.sh does not start the player-valuation timer"
# ...but only once the archive DB it reads every input from actually exists,
# and never bundled into the unconditional enable/start line with the units
# that run fine without it.
grep -qE '^[[:space:]]*if archive_url_configured; then' "$setup" || fail "setup.sh does not gate the player-valuation timer on ARCHIVE_DATABASE_URL"
grep -qE '^[[:space:]]*systemctl (enable|start) .*ff-sims-worker\.service.*ff-sims-player-valuations' "$setup" \
  && fail "the player-valuation timer must not be armed unconditionally alongside the core services"
grep -qE '^[[:space:]]*systemctl start .*ff-sims-player-valuations\.service' "$setup" \
  && fail "setup.sh must not auto-start the player-valuation service — the first full replay is invoked by hand"

echo "PASS: unit file ExecStart targets ($checked checked), timers ($timers checked)"
