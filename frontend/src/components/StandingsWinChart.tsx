import { Bar, BarChart, CartesianGrid, Tooltip, XAxis, YAxis } from "recharts";

export function getWinChartScale(distributions: (number[] | undefined)[]) {
  const count = Math.max(0, ...distributions.map((values) => values?.length ?? 0));
  const wins = Array.from({ length: count }, (_, i) => i);
  const tickStep = Math.max(1, Math.ceil((count - 1) / 7));
  const ticks = wins.filter((win) => win % tickStep === 0);
  const peak = Math.max(0, ...distributions.map((values) => Math.max(0, ...(values ?? []))));
  return { wins, ticks, peak };
}

type WinChartScale = ReturnType<typeof getWinChartScale>;

export function WinChartHeader({ scale }: { scale: WinChartScale }) {
  return (
    <div className="min-w-[clamp(9rem,36vw,23rem)]">
      Simulated wins
      <div aria-hidden="true" className="relative mt-2 h-3 text-[10px] font-normal tabular-nums">
        {scale.ticks.map((wins) => (
          <span
            key={wins}
            className="absolute -translate-x-1/2"
            style={{ left: `${100 * (wins + 0.5) / scale.wins.length}%` }}
          >
            {wins}
          </span>
        ))}
      </div>
    </div>
  );
}

interface StandingsWinChartProps {
  name: string;
  rank: number;
  distribution?: number[];
  scale: WinChartScale;
}

export function StandingsWinChart({ name, rank, distribution, scale }: StandingsWinChartProps) {
  if (!distribution?.length || !scale.peak) {
    return <span aria-label="Win distribution unavailable">—</span>;
  }

  const data = scale.wins.map((wins) => ({ wins, probability: distribution[wins] ?? 0 }));
  return (
    <div className="relative h-10 border-b border-[var(--chart-grid)]">
      <div className="absolute inset-0">
        <BarChart
          responsive width="100%" height="100%" data={data} barCategoryGap={0}
          title={`${name} simulated wins for games played`}
          margin={{ top: 0, bottom: 0, left: 0, right: 0 }}
        >
          <XAxis hide dataKey="wins" />
          <YAxis hide domain={[0, scale.peak]} />
          <CartesianGrid horizontal={false} verticalValues={scale.ticks} stroke="var(--chart-grid)" />
          <Bar dataKey="probability" fill={`var(--chart-series-${(rank - 1) % 6 + 1})`} isAnimationActive={false} />
          <Tooltip
            isAnimationActive={false}
            cursor={{ fill: "var(--chart-grid)", opacity: 0.35 }}
            content={({ active, label, payload }) => {
              const probability = payload?.[0]?.value;
              if (!active || typeof probability !== "number") return null;
              const percent = probability * 100;
              return (
                <div role="status" className="rounded border bg-[var(--chart-tooltip-bg)] px-2 py-1 text-xs text-[var(--chart-tooltip-text)] shadow-sm">
                  {label} {label === 1 ? "win" : "wins"} · {percent > 0 && percent < 0.1 ? "<0.1" : percent.toFixed(1)}%
                </div>
              );
            }}
          />
        </BarChart>
      </div>
    </div>
  );
}
