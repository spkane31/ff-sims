import Link from "next/link";
import { useState } from "react";
import {
  CartesianGrid, Cell, ReferenceLine, ResponsiveContainer, Scatter, ScatterChart, Tooltip, XAxis, YAxis,
} from "recharts";
import type { DraftRewindReport } from "@/services/draftRewindService";
import EmptyState from "@/components/design-system/EmptyState";
import { FOCUS_RING } from "@/components/design-system/focus-ring";
import { formatDraftRank, formatDraftSlot } from "@/utils/draftRewind";

interface DraftFinishChartProps {
  report: DraftRewindReport;
  selectedId: number | undefined;
  onSelect: (id: number) => void;
}

const axisStyle = { fill: "var(--chart-axis-text)", fontSize: 12 };
const selectClass = `w-full min-w-0 rounded-md border border-[var(--border-strong)] bg-[var(--surface-raised)] px-3 py-2 text-sm ${FOCUS_RING}`;

export default function DraftFinishChart({ report, selectedId, onSelect }: DraftFinishChartProps) {
  const [team, setTeam] = useState("");
  const [round, setRound] = useState("");
  const rounds = [...new Set(report.picks.map((pick) => pick.round))];
  const points = report.picks.flatMap((pick) => pick.overall_rank === null
    ? []
    : [{ pick, finish: pick.overall_rank }],
  ).map((point, index) => ({ ...point, order: index + 1 }));

  if (points.length === 0) {
    return <EmptyState title="No ranked picks yet" description="The chart appears once this draft has usable regular-season player scores." />;
  }

  const partial = report.ranked_picks < report.total_picks;
  const axisMax = Math.max(2, points.length);
  const ticks = [...new Set([1, ...[0.25, 0.5, 0.75, 1].map((fraction) => Math.ceil(axisMax * fraction))])];
  const selected = points.find((point) => point.pick.id === selectedId);
  const movement = selected ? selected.order - selected.finish : 0;

  return (
    <section className="space-y-4 rounded-xl border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-3 sm:p-5">
      <div className="space-y-1">
        <h2 className="text-lg font-semibold">How each pick finished</h2>
        <p className="text-sm text-[var(--text-secondary)]">Picks above the diagonal finished ahead of their draft order.{partial && " Both axes compare ranked picks."}</p>
      </div>
      <div className="grid gap-3 sm:grid-cols-[1fr_1fr_1.5fr]">
        <label className="flex flex-col gap-1 text-xs font-medium">
          Highlight team
          <select className={selectClass} value={team} onChange={(event) => setTeam(event.target.value)}>
            <option value="">All teams</option>
            {report.teams.map((entry) => <option key={entry.id} value={entry.id}>{entry.name}</option>)}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium">
          Highlight round
          <select className={selectClass} value={round} onChange={(event) => setRound(event.target.value)}>
            <option value="">All rounds</option>
            {rounds.map((entry) => <option key={entry} value={entry}>Round {entry}</option>)}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium">
          Inspect a pick
          <select className={selectClass} value={selected?.pick.id ?? ""} onChange={(event) => onSelect(Number(event.target.value))}>
            <option value="" disabled>Choose a ranked pick</option>
            {points.map(({ pick }) => <option key={pick.id} value={pick.id}>{formatDraftSlot(pick)} · {pick.player}</option>)}
          </select>
        </label>
      </div>
      {selected && (
        <p className="text-sm tabular-nums" aria-live="polite">
          <strong>
            {selected.pick.player_id > 0 ? (
              <Link
                href={`/players/${selected.pick.player_id}`}
                className={`rounded-sm text-[var(--action-primary)] underline underline-offset-2 ${FOCUS_RING}`}
              >
                {selected.pick.player}
              </Link>
            ) : selected.pick.player}
          </strong>
          {" · "}{formatDraftRank(selected.order)} drafted → {formatDraftRank(selected.finish)} finish ·{" "}
          {movement === 0 ? "held draft rank" : `${Math.abs(movement)} spots ${movement > 0 ? "up" : "down"}`}
          {partial && " among ranked picks"}
        </p>
      )}
      <div className="h-80 min-w-0 sm:h-[27rem]">
        <ResponsiveContainer width="100%" height="100%" minWidth={0}>
          <ScatterChart margin={{ top: 12, right: 14, bottom: 26, left: 6 }}>
            <CartesianGrid stroke="var(--chart-grid)" strokeDasharray="2 4" />
            <XAxis
              type="number" dataKey="order" name="Draft order" domain={[1, axisMax]} ticks={ticks}
              allowDataOverflow tick={axisStyle} padding={{ left: 8, right: 8 }}
              label={{ value: "Draft order", position: "insideBottom", offset: -13, style: axisStyle }}
            />
            <YAxis
              type="number" dataKey="finish" name="Overall finish" domain={[1, axisMax]} ticks={ticks}
              allowDataOverflow reversed width={53} tick={axisStyle} padding={{ top: 8, bottom: 8 }}
              label={{ value: "Overall finish", angle: -90, position: "insideLeft", style: axisStyle }}
            />
            <ReferenceLine segment={[{ x: 1, y: 1 }, { x: axisMax, y: axisMax }]} stroke="var(--chart-zero-line)" strokeWidth={2} />
            <Tooltip
              cursor={false}
              content={({ active, payload }) => {
                const point = payload?.[0]?.payload as (typeof points)[number] | undefined;
                if (!active || !point) return null;
                return (
                  <div className="rounded-md border border-[var(--border-subtle)] bg-[var(--surface-raised)] px-3 py-2 text-sm shadow-sm">
                    <strong>{formatDraftSlot(point.pick)} · {point.pick.player}</strong>
                    <p>Drafted {formatDraftRank(point.order)} · Finished {formatDraftRank(point.finish)}</p>
                    <p className="mt-1 text-xs text-[var(--text-secondary)]">Click to select this pick</p>
                  </div>
                );
              }}
            />
            <Scatter data={points} isAnimationActive={false} onClick={(entry) => onSelect(entry.payload.pick.id)}>
              {points.map((point) => {
                const highlighted = (!team || String(point.pick.team_id) === team) && (!round || String(point.pick.round) === round);
                const active = point.pick.id === selectedId;
                const fill = point.order === point.finish ? "var(--chart-series-6)" :
                  point.order > point.finish ? "var(--chart-series-1)" : "var(--chart-series-2)";
                return (
                  <Cell key={point.pick.id} fill={fill} stroke={active ? "var(--text-primary)" : undefined}
                    strokeWidth={active ? 2 : 0} opacity={highlighted || active ? 1 : 0.22} className="cursor-pointer" />
                );
              })}
            </Scatter>
          </ScatterChart>
        </ResponsiveContainer>
      </div>
    </section>
  );
}
