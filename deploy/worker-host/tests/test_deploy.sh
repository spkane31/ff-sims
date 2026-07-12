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

mkdir -p "$REPO/backend/cmd/worker" "$REPO/deploy/worker-host"
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
bash "$REPO/deploy/worker-host/deploy.sh"
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

bash "$REPO/deploy/worker-host/deploy.sh"
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

if bash "$REPO/deploy/worker-host/deploy.sh"; then
  fail "deploy.sh should exit non-zero on a build failure"
fi
new_hash="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
[[ "$old_hash" == "$new_hash" ]] || fail "worker binary should be unchanged after a failed build"
[[ ! -s "$CALLS" ]] || fail "systemctl should not have been called after a failed build"

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

set +e
deploy_output="$(bash "$REPO/deploy/worker-host/deploy.sh" 2>&1)"
deploy_status=$?
set -e

[[ "$deploy_status" -ne 0 ]] || fail "deploy.sh should exit non-zero when git fetch fails"
[[ "$deploy_output" != *"up to date"* ]] || fail "deploy.sh must not report 'up to date' when it never successfully checked origin/main, got: $deploy_output"
worker_hash_after="$(shasum -a 256 "$REPO/backend/worker" | awk '{print $1}')"
[[ "$worker_hash_before" == "$worker_hash_after" ]] || fail "worker binary should be unchanged when git fetch fails"
[[ ! -s "$CALLS" ]] || fail "systemctl should not have been called when git fetch fails"

git -C "$REPO" remote set-url origin "$ORIGIN"

echo "PASS: deploy.sh integration tests"
