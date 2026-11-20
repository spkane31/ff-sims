import { FOCUS_RING } from "@/components/design-system/focus-ring";
import type { DraftRewindPick, DraftRewindReport } from "@/services/draftRewindService";
import { draftScoreReason, formatDraftPoints, formatDraftRank, formatDraftSlot } from "@/utils/draftRewind";

export type DraftRankMode = "round" | "overall";

interface DraftBoardProps {
  report: DraftRewindReport;
  mode: DraftRankMode;
  selectedId: number | undefined;
  onSelect: (id: number) => void;
}

export default function DraftBoard({ report, mode, selectedId, onSelect }: DraftBoardProps) {
  const rounds = new Map<number, DraftRewindPick[]>();
  for (const pick of report.picks) {
    const round = rounds.get(pick.round);
    if (round) round.push(pick);
    else rounds.set(pick.round, [pick]);
  }
  const teamNames = new Map(report.teams.map((team) => [team.id, team.name]));
  const roundRows = Array.from(rounds, ([number, picks]) => {
    const roundRanked = picks.filter((pick) => pick.round_rank !== null).length;
    const cohortSize = mode === "round" ? roundRanked : report.ranked_picks;
    const cells = new Map<number, DraftRewindPick[]>();
    for (const pick of picks) {
      const cell = cells.get(pick.team_id);
      if (cell) cell.push(pick);
      else cells.set(pick.team_id, [pick]);
    }
    return { number, picks, cells, cohortSize };
  });

  function renderPick(pick: DraftRewindPick, cohortSize: number, showTeam = false) {
    const rank = mode === "round" ? pick.round_rank : pick.overall_rank;
    const tint = rank === null || cohortSize <= 1
      ? 0
      : Math.round(6 + 31 * (cohortSize - rank) / (cohortSize - 1));
    const selected = pick.id === selectedId;
    const scope = mode === "round" ? "round" : "overall";
    return (
      <button
        key={pick.id}
        type="button"
        className={`flex min-h-20 w-full flex-col gap-1 rounded-lg border px-2.5 py-2 text-left hover:border-[var(--action-primary)] ${FOCUS_RING}`}
        style={{
          background: `color-mix(in oklab, var(--chart-series-1) ${tint}%, var(--surface-sunken))`,
          borderColor: selected ? "var(--action-primary)" : undefined,
          boxShadow: selected ? "inset 0 0 0 1px var(--action-primary)" : undefined,
        }}
        aria-pressed={selected}
        aria-label={`${formatDraftSlot(pick)} ${pick.player}, ${teamNames.get(pick.team_id) ?? "Unknown team"}, ${rank === null ? draftScoreReason(pick) : `${formatDraftPoints(pick.actual_points)} points, ${scope} rank ${rank}`}`}
        onClick={() => onSelect(pick.id)}
      >
        <span className="flex w-full items-baseline justify-between gap-1 text-xs tabular-nums">
          <span className="text-[var(--text-secondary)]">{formatDraftSlot(pick)}</span>
          {rank !== null && <strong>{formatDraftRank(rank)}</strong>}
        </span>
        <span className="text-xs font-semibold leading-tight">{pick.player}</span>
        <span className="text-xs tabular-nums text-[var(--text-secondary)]">
          {pick.actual_points === null ? "Unranked" : `${formatDraftPoints(pick.actual_points)} pts`}
          {showTeam && ` · ${teamNames.get(pick.team_id) ?? "Unknown team"}`}
        </span>
      </button>
    );
  }

  return (
    <>
      <div className="hidden overflow-x-auto md:block">
        <table className="w-full border-separate border-spacing-1.5">
          <caption className="sr-only">Draft picks by round and drafting team, ranked {mode === "round" ? "within each round" : "across the entire draft"}.</caption>
          <thead>
            <tr>
              <th scope="col" className="sticky left-0 z-10 bg-[var(--surface-canvas)] px-2 py-1 text-left text-xs text-[var(--text-secondary)]">Round</th>
              {report.teams.map((team) => (
                <th key={team.id} scope="col" className="min-w-28 max-w-40 px-2 py-1 text-left text-xs font-semibold leading-tight">{team.name}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {roundRows.map((round) => (
              <tr key={round.number}>
                <th scope="row" className="sticky left-0 z-10 bg-[var(--surface-canvas)] px-2 text-left text-sm tabular-nums">{round.number}</th>
                {report.teams.map((team) => (
                  <td key={team.id} className="align-top">
                    {round.cells.has(team.id) ? (
                      <div className="space-y-1.5">
                        {round.cells.get(team.id)?.map((pick) => renderPick(pick, round.cohortSize))}
                      </div>
                    ) : (
                      <span className="flex min-h-20 items-center justify-center text-[var(--text-muted)]">
                        <span aria-hidden="true">—</span><span className="sr-only">No pick</span>
                      </span>
                    )}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="space-y-5 md:hidden">
        {roundRows.map((round) => (
          <section key={round.number}>
            <h3 className="mb-2 text-sm font-semibold">Round {round.number}</h3>
            <ul className="grid grid-cols-2 gap-2">
              {round.picks.map((pick) => <li key={pick.id}>{renderPick(pick, round.cohortSize, true)}</li>)}
            </ul>
          </section>
        ))}
      </div>
    </>
  );
}
