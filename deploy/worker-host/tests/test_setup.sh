#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../setup.sh"

fail() { echo "FAIL: $1" >&2; exit 1; }

# go_arch_for_uname
[[ "$(go_arch_for_uname aarch64)" == "arm64" ]] || fail "aarch64 -> arm64"
[[ "$(go_arch_for_uname armv7l)" == "armv6l" ]] || fail "armv7l -> armv6l"
[[ "$(go_arch_for_uname x86_64)" == "amd64" ]] || fail "x86_64 -> amd64"
if go_arch_for_uname riscv64 &>/dev/null; then fail "riscv64 should be unsupported"; fi

# env file template + placeholder detection
tmp_env="$(mktemp)"
write_env_template "$tmp_env"
env_file_is_placeholder "$tmp_env" || fail "freshly written template should be a placeholder"

perm=$(stat -f "%OLp" "$tmp_env" 2>/dev/null || stat -c "%a" "$tmp_env")
[[ "$perm" == "600" ]] || fail "env file should be mode 600, got $perm"

marker_escaped=$(printf '%s\n' "$PLACEHOLDER_MARKER" | sed 's/[.[\*^$/]/\\&/g')
sed -i.bak "/${marker_escaped}/d" "$tmp_env" && rm -f "${tmp_env}.bak"
if env_file_is_placeholder "$tmp_env"; then fail "template with marker removed should not be a placeholder"; fi

rm -f "$tmp_env"
echo "PASS: setup.sh unit tests"
