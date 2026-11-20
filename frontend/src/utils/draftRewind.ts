import type { DraftProjection, DraftRewindPick } from "@/services/draftRewindService";

const pointsFormat = new Intl.NumberFormat("en-US", {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

export function formatDraftPoints(points: number | null) {
  return points === null ? "—" : pointsFormat.format(points);
}

export function formatDraftSlot(pick: DraftRewindPick) {
  return `${pick.round}.${String(pick.pick).padStart(2, "0")}`;
}

export function formatDraftRank(rank: number | null) {
  return rank === null ? "—" : `#${rank}`;
}

export function draftProjectionDifference(row: DraftProjection) {
  if (row.projected_points === null || row.projection_actual_points === null) return null;
  return Math.round((row.projection_actual_points - row.projected_points) * 100) / 100;
}

export function formatDraftDifference(points: number | null) {
  return points !== null && points > 0 ? `+${formatDraftPoints(points)}` : formatDraftPoints(points);
}

export function draftScoreReason(pick: DraftRewindPick) {
  return pick.score_status === "conflicting_scores"
    ? "Conflicting player scores"
    : "No player box scores";
}
