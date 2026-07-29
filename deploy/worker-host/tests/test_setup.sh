#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../setup.sh"

fail() { echo "FAIL: $1" >&2; exit 1; }

WORK_MISSING="$(mktemp -u)"  # a path that deliberately does not exist

# go_arch_for_uname
[[ "$(go_arch_for_uname aarch64)" == "arm64" ]] || fail "aarch64 -> arm64"
[[ "$(go_arch_for_uname armv7l)" == "armv6l" ]] || fail "armv7l -> armv6l"
[[ "$(go_arch_for_uname x86_64)" == "amd64" ]] || fail "x86_64 -> amd64"
if go_arch_for_uname riscv64 &>/dev/null; then fail "riscv64 should be unsupported"; fi

# env file template + placeholder detection
tmp_env="$(mktemp)"
write_env_template "$tmp_env"
env_file_is_placeholder "$tmp_env" || fail "freshly written template should be a placeholder"

# Every variable a unit needs has to appear in the template, or a fresh host
# gets a service that installs and then fails on every run. ARCHIVE_DATABASE_URL
# is required by ff-sims-player-valuations.service.
for var in DATABASE_URL ARCHIVE_DATABASE_URL TEMPORAL_NAMESPACE_ENDPOINT TEMPORAL_NAMESPACE TEMPORAL_API_KEY; do
  grep -qE "^${var}=" "$tmp_env" || fail "env template is missing $var"
done
# ...but the archive URL must default to empty, not a placeholder host: the Go
# worker/cron treat empty as "archive disabled", while a bogus URL would make
# them fail trying to connect.
grep -qE '^ARCHIVE_DATABASE_URL=$' "$tmp_env" || fail "ARCHIVE_DATABASE_URL should be empty in the template, not a placeholder URL"

perm=$(stat -f "%OLp" "$tmp_env" 2>/dev/null || stat -c "%a" "$tmp_env")
[[ "$perm" == "600" ]] || fail "env file should be mode 600, got $perm"

marker_escaped=$(printf '%s\n' "$PLACEHOLDER_MARKER" | sed 's/[.[\*^$/]/\\&/g')
sed -i.bak "/${marker_escaped}/d" "$tmp_env" && rm -f "${tmp_env}.bak"
if env_file_is_placeholder "$tmp_env"; then fail "template with marker removed should not be a placeholder"; fi

rm -f "$tmp_env"

# archive_url_configured: gates whether the player-valuation timer gets armed.
# A freshly written template must NOT count as configured, or setup would arm a
# timer that fails every midnight until the separate archive-provisioning step
# (README step 7) is done.
tmp_env="$(mktemp)"
write_env_template "$tmp_env"
if archive_url_configured "$tmp_env"; then fail "blank ARCHIVE_DATABASE_URL should not count as configured"; fi

printf 'ARCHIVE_DATABASE_URL=postgres://ffsims@localhost:5432/ff_sims_archive\n' >> "$tmp_env"
archive_url_configured "$tmp_env" || fail "a real ARCHIVE_DATABASE_URL should count as configured"

# a commented-out URL is not a configured one
tmp_commented="$(mktemp)"
printf '#ARCHIVE_DATABASE_URL=postgres://nope\n' > "$tmp_commented"
if archive_url_configured "$tmp_commented"; then fail "a commented-out ARCHIVE_DATABASE_URL should not count as configured"; fi

# neither is whitespace
tmp_blank="$(mktemp)"
printf 'ARCHIVE_DATABASE_URL=   \n' > "$tmp_blank"
if archive_url_configured "$tmp_blank"; then fail "a whitespace-only ARCHIVE_DATABASE_URL should not count as configured"; fi

if archive_url_configured "$WORK_MISSING"; then fail "a nonexistent env file should not count as configured"; fi

rm -f "$tmp_env" "$tmp_commented" "$tmp_blank"
echo "PASS: setup.sh unit tests"
