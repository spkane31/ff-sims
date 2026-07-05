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
