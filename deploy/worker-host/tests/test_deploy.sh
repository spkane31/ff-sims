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

mkdir -p "$REPO/backend/cmd/worker" "$REPO/backend/cmd/cron" "$REPO/workers/espn" "$REPO/deploy/worker-host"
cp "$SCRIPT_DIR/../deploy.sh" "$REPO/deploy/worker-host/deploy.sh"
chmod +x "$REPO/deploy/worker-host/deploy.sh"

cat > "$REPO/backend/go.mod" <<'EOF'
module backend

go 1.21
EOF
cat > "$REPO/backend/cmd/worker/main.go" <<'EOF'
package main

func main() {}
EOF
cat > "$REPO/backend/cmd/cron/main.go" <<'EOF'
package main

func main() {}
EOF
echo "v1" > "$REPO/workers/espn/worker.py"

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

# --- stub uv so ESPN-worker deploy assertions don't depend on real network
# access / dependency resolution — same idea as the systemctl stub above.
# UV_FAIL_FLAG lets a scenario simulate a sync failure on demand.
UV_CALLS="$WORK/uv_calls"
UV_FAIL_FLAG="$WORK/uv_should_fail"
: > "$UV_CALLS"
cat > "$BIN/uv" <<EOF
#!/usr/bin/env bash
echo "\$@" >> "$UV_CALLS"
[[ -f "$UV_FAIL_FLAG" ]] && exit 1
exit 0
EOF
chmod +x "$BIN/uv"
export UV_BIN="$BIN/uv"

# --- scenario 1: no new commits -> no rebuild, no restart ---
bash "$REPO/deploy/worker-host/deploy.sh"
[[ ! -f "$REPO/backend/worker" ]] || fail "should not have built a worker binary when up to date"
[[ ! -f "$REPO/backend/cron" ]] || fail "should not have built a cron binary when up to date"
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

bash "$REPO/deploy/worker-host/deploy.sh"
[[ -x "$REPO/backend/worker" ]] || fail "expected a worker binary to be built"
[[ -x "$REPO/backend/cron" ]] || fail "expected a cron binary to be built"
grep -q "restart ff-sims-worker.service" "$CALLS" || fail "expected systemctl restart to be called"
grep -q "sync --frozen --no-dev" "$UV_CALLS" || fail "expected uv sync to be called for the ESPN worker on first deploy"
grep -q "restart ff-sims-espn-worker.service" "$CALLS" || fail "expected systemctl restart to be called for the ESPN worker"
[[ -f "$REPO/workers/espn/.espn-deployed-sha" ]] || fail "expected ESPN sha file to be written after first deploy"

# --- scenario 3: a new commit that fails to compile -> old binary + service left alone ---
old_hash="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
: > "$CALLS"
: > "$UV_CALLS"
cat > "$CLONE/backend/cmd/worker/main.go" <<'EOF'
package main

func main() { this is not valid go }
EOF
git -C "$CLONE" -c user.email=test@example.com -c user.name=test commit -aqm "broken"
git -C "$CLONE" push -q origin main

if bash "$REPO/deploy/worker-host/deploy.sh"; then
  fail "deploy.sh should exit non-zero on a build failure"
fi
new_hash="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
[[ "$old_hash" == "$new_hash" ]] || fail "worker binary should be unchanged after a failed build"
[[ ! -s "$CALLS" ]] || fail "systemctl should not have been called after a failed build"
[[ ! -s "$UV_CALLS" ]] || fail "uv should not have been called when the worker build already failed earlier in the cycle"

# --- scenario 4: a commit that changes build_worker itself must take effect on
# THIS deploy cycle, not the next ---
#
# Regression test for the incident that motivated the re-exec in deploy(): this
# process has already parsed build_worker's OLD body by the time `git reset
# --hard` rewrites deploy.sh on disk, so calling build_worker in-process would
# silently keep using the stale, pre-pull logic. That's exactly what happened
# when a commit added -ldflags to build_worker: the deploy that shipped it built
# the worker with the OLD (ldflags-less) build_worker, so the binary reported
# the Go source's default build ID instead of a real git SHA.
cat > "$CLONE/backend/cmd/worker/main.go" <<'EOF'
package main

var marker = "unset"

func main() { println(marker) }
EOF
sed "s/-X 'main.buildID=\${sha}'/-X 'main.buildID=\${sha}' -X 'main.marker=updated'/" \
  "$CLONE/deploy/worker-host/deploy.sh" > "$CLONE/deploy/worker-host/deploy.sh.new"
grep -q "main.marker=updated" "$CLONE/deploy/worker-host/deploy.sh.new" || fail "test setup: sed did not patch build_worker's ldflags"
mv "$CLONE/deploy/worker-host/deploy.sh.new" "$CLONE/deploy/worker-host/deploy.sh"
chmod +x "$CLONE/deploy/worker-host/deploy.sh"
git -C "$CLONE" -c user.email=test@example.com -c user.name=test commit -aqm "build_worker now sets marker"
git -C "$CLONE" push -q origin main

bash "$REPO/deploy/worker-host/deploy.sh"
built_output="$("$REPO/backend/worker" 2>&1)"
[[ "$built_output" == "updated" ]] || fail "expected the deploy that changed build_worker to use the NEW build_worker on the same cycle, got: $built_output"

# --- scenario 5: git fetch fails (e.g. broken SSH access) -> deploy.sh must
# exit non-zero and must NOT report "up to date" ---
#
# Regression test for the incident where a root SSH key without GitHub access
# caused `git fetch` to fail, but deploy.sh printed "up to date" anyway and
# exited 0: current_and_remote_sha() runs inside a $(...) command
# substitution, which bash does not run under -e/errexit by default, so the
# failed fetch fell through to rev-parse'ing stale cached refs instead of
# aborting.
worker_hash_before="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
git -C "$REPO" remote set-url origin "$WORK/does-not-exist.git"
: > "$CALLS"
: > "$UV_CALLS"

set +e
deploy_output="$(bash "$REPO/deploy/worker-host/deploy.sh" 2>&1)"
deploy_status=$?
set -e

[[ "$deploy_status" -ne 0 ]] || fail "deploy.sh should exit non-zero when git fetch fails"
[[ "$deploy_output" != *"up to date"* ]] || fail "deploy.sh must not report 'up to date' when it never successfully checked origin/main, got: $deploy_output"
worker_hash_after="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
[[ "$worker_hash_before" == "$worker_hash_after" ]] || fail "worker binary should be unchanged when git fetch fails"
[[ ! -s "$CALLS" ]] || fail "systemctl should not have been called when git fetch fails"
[[ ! -s "$UV_CALLS" ]] || fail "uv should not have been called when git fetch fails"

git -C "$REPO" remote set-url origin "$ORIGIN"

# --- scenario 6: a commit outside backend/ entirely (e.g. docs) -> the local
# checkout still advances, but neither binary rebuilds and the service is not
# restarted; a clear reason is logged for both ---
worker_hash_before="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
cron_hash_before="$(shasum -a 256 "$REPO/backend/cron" | awk '{print $1}')"
: > "$CALLS"
: > "$UV_CALLS"
echo "unrelated change" > "$CLONE/NOTES.md"
git -C "$CLONE" add NOTES.md
git -C "$CLONE" -c user.email=test@example.com -c user.name=test commit -qm "docs: unrelated change"
git -C "$CLONE" push -q origin main

deploy_output="$(bash "$REPO/deploy/worker-host/deploy.sh" 2>&1)"
echo "$deploy_output" | grep -q "worker: up to date, no worker-relevant changes" || fail "expected a clear skip reason for the worker, got: $deploy_output"
echo "$deploy_output" | grep -q "cron: up to date, no cron-relevant changes" || fail "expected a clear skip reason for cron, got: $deploy_output"
echo "$deploy_output" | grep -q "espn-worker: up to date, no changes" || fail "expected a clear skip reason for the ESPN worker, got: $deploy_output"

[[ "$(git -C "$REPO" rev-parse HEAD)" == "$(git -C "$REPO" rev-parse origin/main)" ]] \
  || fail "local checkout should still advance to origin/main even when the build is skipped"
worker_hash_after="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
cron_hash_after="$(shasum -a 256 "$REPO/backend/cron" | awk '{print $1}')"
[[ "$worker_hash_before" == "$worker_hash_after" ]] || fail "worker binary should not rebuild for a docs-only change"
[[ "$cron_hash_before" == "$cron_hash_after" ]] || fail "cron binary should not rebuild for a docs-only change"
[[ ! -s "$CALLS" ]] || fail "systemctl should not have been called for a docs-only change"
[[ ! -s "$UV_CALLS" ]] || fail "uv should not have been called for a docs-only change"

# --- scenario 7: a change to cmd/cron only -> cron rebuilds independently of
# the worker (each binary is gated on its own dependency graph); the worker
# binary and its service restart are left untouched ---
worker_hash_before="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
cron_hash_before="$(shasum -a 256 "$REPO/backend/cron" | awk '{print $1}')"
: > "$CALLS"
: > "$UV_CALLS"
cat > "$CLONE/backend/cmd/cron/main.go" <<'EOF'
package main

func main() { println("cron v2") }
EOF
git -C "$CLONE" -c user.email=test@example.com -c user.name=test commit -aqm "cron v2"
git -C "$CLONE" push -q origin main

bash "$REPO/deploy/worker-host/deploy.sh"
worker_hash_after="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
cron_hash_after="$(shasum -a 256 "$REPO/backend/cron" | awk '{print $1}')"
[[ "$worker_hash_before" == "$worker_hash_after" ]] || fail "worker binary should not rebuild for a cron-only change"
[[ "$cron_hash_before" != "$cron_hash_after" ]] || fail "cron binary should have been rebuilt for a cron-only change"
[[ ! -s "$CALLS" ]] || fail "systemctl should not restart the worker service for a cron-only change"
[[ ! -s "$UV_CALLS" ]] || fail "uv should not have been called for a cron-only change"

# --- scenario 8: a build failure must keep being retried on later cycles even
# if the intervening commit doesn't touch worker paths itself ---
#
# Regression test for exactly the risk called out in issue #172: if deploy.sh
# diffed from whatever sha was last checked out (which already advances past
# a failed build via git reset --hard) instead of from the last sha the
# worker was actually, successfully built from, a broken worker commit
# followed by an unrelated commit would look like "nothing worker-relevant
# changed since last time" and the rebuild would silently stop being
# retried -- permanently freezing the worker on the last-good binary with no
# further failures ever logged. Uses a type error (not a syntax error) so
# `go list -deps` succeeds and the git-diff comparison itself is what's under
# test, not the "go list failed, assume relevant" fallback.
worker_hash_before="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"

cat > "$CLONE/backend/cmd/worker/main.go" <<'EOF'
package main

func main() { var x int = "not an int"; _ = x }
EOF
git -C "$CLONE" -c user.email=test@example.com -c user.name=test commit -aqm "broken (type error)"
git -C "$CLONE" push -q origin main

if bash "$REPO/deploy/worker-host/deploy.sh"; then
  fail "deploy.sh should exit non-zero on this build failure"
fi
worker_hash_after_break="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
[[ "$worker_hash_before" == "$worker_hash_after_break" ]] || fail "worker binary should be unchanged after this failed build"

echo "more unrelated" >> "$CLONE/NOTES.md"
git -C "$CLONE" add NOTES.md
git -C "$CLONE" -c user.email=test@example.com -c user.name=test commit -qm "docs: more unrelated"
git -C "$CLONE" push -q origin main

if bash "$REPO/deploy/worker-host/deploy.sh"; then
  fail "deploy.sh should still retry (and fail) the worker build, since the source is still broken relative to the last successful build"
fi
worker_hash_after_unrelated="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
[[ "$worker_hash_before" == "$worker_hash_after_unrelated" ]] || fail "worker binary should still be unchanged"

# fix it back up so the suite ends in a clean, buildable state
cat > "$CLONE/backend/cmd/worker/main.go" <<'EOF'
package main

func main() { println("v3") }
EOF
git -C "$CLONE" -c user.email=test@example.com -c user.name=test commit -aqm "fixed"
git -C "$CLONE" push -q origin main
bash "$REPO/deploy/worker-host/deploy.sh"
worker_hash_after_fix="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
[[ "$worker_hash_before" != "$worker_hash_after_fix" ]] || fail "expected worker to rebuild once fixed"

# --- scenario 9: an ESPN-only change syncs + restarts the ESPN worker
# independently of the Go worker/cron; a uv sync failure leaves it alone and
# is retried on a later cycle, mirroring the Go build-failure behavior above ---
worker_hash_before="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
cron_hash_before="$(shasum -a 256 "$REPO/backend/cron" | awk '{print $1}')"
: > "$CALLS"
: > "$UV_CALLS"
echo "v2" > "$CLONE/workers/espn/worker.py"
git -C "$CLONE" add workers/espn/worker.py
git -C "$CLONE" -c user.email=test@example.com -c user.name=test commit -qm "espn worker v2"
git -C "$CLONE" push -q origin main

bash "$REPO/deploy/worker-host/deploy.sh"
grep -q "sync --frozen --no-dev" "$UV_CALLS" || fail "expected uv sync for an ESPN-relevant change"
grep -q "restart ff-sims-espn-worker.service" "$CALLS" || fail "expected the ESPN worker service to be restarted"
worker_hash_after="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
cron_hash_after="$(shasum -a 256 "$REPO/backend/cron" | awk '{print $1}')"
[[ "$worker_hash_before" == "$worker_hash_after" ]] || fail "worker binary should not rebuild for an ESPN-only change"
[[ "$cron_hash_before" == "$cron_hash_after" ]] || fail "cron binary should not rebuild for an ESPN-only change"

# a subsequent deploy with no further workers/espn changes should skip the sync
: > "$CALLS"
: > "$UV_CALLS"
cat > "$CLONE/backend/cmd/worker/main.go" <<'EOF'
package main

func main() { println("v4") }
EOF
git -C "$CLONE" -c user.email=test@example.com -c user.name=test commit -aqm "v4"
git -C "$CLONE" push -q origin main

deploy_output="$(bash "$REPO/deploy/worker-host/deploy.sh" 2>&1)"
echo "$deploy_output" | grep -q "espn-worker: up to date, no changes" || fail "expected the ESPN worker to skip when nothing under workers/espn changed, got: $deploy_output"
[[ ! -s "$UV_CALLS" ]] || fail "uv should not have been called when nothing under workers/espn changed"

# a uv sync failure must not restart the service, and must be retried later
: > "$CALLS"
: > "$UV_CALLS"
touch "$UV_FAIL_FLAG"
echo "v3" > "$CLONE/workers/espn/worker.py"
git -C "$CLONE" -c user.email=test@example.com -c user.name=test commit -aqm "espn worker v3 (uv will fail)"
git -C "$CLONE" push -q origin main

if bash "$REPO/deploy/worker-host/deploy.sh"; then
  fail "deploy.sh should exit non-zero when uv sync fails"
fi
grep -q "restart ff-sims-espn-worker.service" "$CALLS" && fail "the ESPN worker service should not be restarted after a failed uv sync"
rm -f "$UV_FAIL_FLAG"

# git reset --hard already advanced the local checkout to the failing commit,
# so a bare re-run would see local_sha == remote_sha and report "up to date"
# without ever re-checking the ESPN worker. Push one more (unrelated) commit
# so deploy() has a new remote sha to diff against — exactly how scenario 8
# above retries a stuck Go build.
: > "$CALLS"
: > "$UV_CALLS"
echo "trigger retry" >> "$CLONE/NOTES.md"
git -C "$CLONE" add NOTES.md
git -C "$CLONE" -c user.email=test@example.com -c user.name=test commit -qm "docs: trigger retry"
git -C "$CLONE" push -q origin main
bash "$REPO/deploy/worker-host/deploy.sh"
grep -q "sync --frozen --no-dev" "$UV_CALLS" || fail "expected the ESPN sync to be retried once uv succeeds again"
grep -q "restart ff-sims-espn-worker.service" "$CALLS" || fail "expected the ESPN worker service to be restarted once the retried sync succeeds"

echo "PASS: deploy.sh integration tests"
