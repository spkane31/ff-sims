import { useState } from "react";
import DataTable, { type DataTableColumn } from "@/components/design-system/DataTable";
import { FOCUS_RING } from "@/components/design-system/focus-ring";
import type { DraftRewindPick, DraftRewindReport, DraftRewindTeam } from "@/services/draftRewindService";
import {
  draftProjectionDifference, draftScoreReason, formatDraftDifference, formatDraftPoints, formatDraftRank, formatDraftSlot,
} from "@/utils/draftRewind";

type SortableColumn<T> = DataTableColumn<T> & {
  sortValue: (row: T) => string | number | null;
  descending?: boolean;
};

function SortedResults<T extends { id: number }>({
  columns, rows, initialSort,
}: {
  columns: SortableColumn<T>[];
  rows: T[];
  initialSort: string;
}) {
  const [sort, setSort] = useState<{ id: string; direction: "asc" | "desc" }>({ id: initialSort, direction: "asc" });
  const column = columns.find((entry) => entry.id === sort.id);
  const ordered = column ? [...rows].sort((a, b) => {
    const left = column.sortValue(a);
    const right = column.sortValue(b);
    if (left === null) return right === null ? 0 : 1;
    if (right === null) return -1;
    const cmp = typeof left === "number" && typeof right === "number"
      ? left - right : String(left).localeCompare(String(right));
    return sort.direction === "asc" ? cmp : -cmp;
  }) : rows;

  return (
    <DataTable
      columns={columns.map((entry) => ({ ...entry, sortable: true }))}
      rows={ordered}
      rowKey={(row) => String(row.id)}
      sortField={sort.id}
      sortDirection={sort.direction}
      onSort={(id) => setSort({
        id,
        direction: sort.id === id
          ? (sort.direction === "asc" ? "desc" : "asc")
          : (columns.find((entry) => entry.id === id)?.descending ? "desc" : "asc"),
      })}
    />
  );
}

const teamColumns: SortableColumn<DraftRewindTeam>[] = [
  { id: "team", header: "Team", cell: (row) => row.name, sortValue: (row) => row.name },
  { id: "points", header: "Actual pts", align: "right", cell: (row) => formatDraftPoints(row.actual_points), sortValue: (row) => row.actual_points, descending: true },
  { id: "projected", header: "Projected pts", align: "right", cell: (row) => formatDraftPoints(row.projected_points), sortValue: (row) => row.projected_points, descending: true },
  { id: "delta", header: "Vs. projection", align: "right", cell: (row) => formatDraftDifference(draftProjectionDifference(row)), sortValue: draftProjectionDifference, descending: true },
  { id: "coverage", header: "Picks with projections", align: "right", cell: (row) => `${row.projected_picks} / ${row.total_picks}`, sortValue: (row) => row.projected_picks, descending: true },
];

export default function DraftResults({
  report, selectedId, onSelect,
}: {
  report: DraftRewindReport;
  selectedId: number | undefined;
  onSelect: (id: number) => void;
}) {
  const teamNames = new Map(report.teams.map((team) => [team.id, team.name]));
  const slots = new Map(report.picks.map((pick, index) => [pick.id, index]));
  const pickColumns: SortableColumn<DraftRewindPick>[] = [
    { id: "slot", header: "Pick", cell: formatDraftSlot, sortValue: (pick) => slots.get(pick.id) ?? null },
    {
      id: "player",
      header: "Player",
      cell: (pick) => (
        <button
          type="button"
          aria-pressed={selectedId === pick.id}
          className={`rounded-sm font-semibold text-[var(--action-primary)] hover:underline ${FOCUS_RING}`}
          onClick={() => onSelect(pick.id)}
        >
          {pick.player}
        </button>
      ),
      sortValue: (pick) => pick.player,
    },
    { id: "team", header: "Drafted by", cell: (pick) => teamNames.get(pick.team_id), sortValue: (pick) => teamNames.get(pick.team_id) ?? null },
    { id: "points", header: "Actual pts", align: "right", cell: (pick) => pick.actual_points === null ? <span title={draftScoreReason(pick)}>Unranked</span> : formatDraftPoints(pick.actual_points), sortValue: (pick) => pick.actual_points, descending: true },
    { id: "round", header: "Round rank", align: "right", cell: (pick) => formatDraftRank(pick.round_rank), sortValue: (pick) => pick.round_rank },
    { id: "overall", header: "Overall rank", align: "right", cell: (pick) => formatDraftRank(pick.overall_rank), sortValue: (pick) => pick.overall_rank },
    { id: "projected", header: "Projected pts", align: "right", cell: (pick) => formatDraftPoints(pick.projected_points), sortValue: (pick) => pick.projected_points, descending: true },
    { id: "delta", header: "Vs. projection", align: "right", cell: (pick) => formatDraftDifference(draftProjectionDifference(pick)), sortValue: draftProjectionDifference, descending: true },
    { id: "coverage", header: "Weeks projected", align: "right", cell: (pick) => pick.projection_weeks, sortValue: (pick) => pick.projection_weeks, descending: true },
  ];
  return (
    <div className="space-y-6">
      <section className="overflow-hidden rounded-xl border border-[var(--border-subtle)]">
        <div className="space-y-1 border-b border-[var(--border-subtle)] p-4">
          <h2 className="text-lg font-semibold">Drafted players’ points by team</h2>
          <p className="text-sm text-[var(--text-secondary)]">
            Each team gets credit for its original picks, including points those players scored after a trade or drop.
          </p>
        </div>
        <SortedResults columns={teamColumns} rows={report.teams} initialSort="team" />
      </section>
      <section className="overflow-hidden rounded-xl border border-[var(--border-subtle)]">
        <div className="space-y-1 border-b border-[var(--border-subtle)] p-4">
          <h2 className="text-lg font-semibold">Pick results</h2>
          <p className="text-sm text-[var(--text-secondary)]">
            Projection differences use only the weeks with both an actual score and a usable ESPN projection.
          </p>
        </div>
        <SortedResults columns={pickColumns} rows={report.picks} initialSort="slot" />
      </section>
    </div>
  );
}
