#!/usr/bin/env bash
set -euo pipefail

# Export all tables from CockroachDB to CSV files.
# Uses psql \COPY to avoid pg_dump system catalog incompatibilities.
#
# Usage:
#   COCKROACHDB_URL="postgresql://..." ./scripts/export_crdb.sh
#   ./scripts/export_crdb.sh --out ./my_export_dir
#
# Output: one CSV per table in the output directory (default: crdb_export/)

OUT_DIR="crdb_export"

while [[ $# -gt 0 ]]; do
  case $1 in
    --out) OUT_DIR="$2"; shift 2 ;;
    *) echo "Unknown argument: $1"; exit 1 ;;
  esac
done

if [[ -z "${COCKROACHDB_URL:-}" ]]; then
  echo "Error: COCKROACHDB_URL is not set"
  exit 1
fi

TABLES=(
  leagues
  players
  teams
  team_name_histories
  team_players
  matchups
  box_scores
  simulations
  sim_results
  sim_team_results
  transactions
  draft_selections
  weekly_expected_wins
  season_expected_wins
)

mkdir -p "$OUT_DIR"
echo "Exporting to $OUT_DIR/"
echo ""

for table in "${TABLES[@]}"; do
  printf "  %-30s" "$table"
  psql "$COCKROACHDB_URL" -q -c "\COPY $table TO '${OUT_DIR}/${table}.csv' CSV HEADER"
  rows=$(wc -l < "${OUT_DIR}/${table}.csv")
  echo "$(( rows - 1 )) rows"
done

echo ""
echo "Export complete: $(ls "$OUT_DIR"/*.csv | wc -l | tr -d ' ') tables written to $OUT_DIR/"
