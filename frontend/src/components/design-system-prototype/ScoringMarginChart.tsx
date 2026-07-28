import { useEffect, useState } from 'react';
import {
  BarChart,
  Bar,
  Cell,
  LabelList,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ReferenceLine,
  ResponsiveContainer,
} from 'recharts';
import type { WeeklyMarginFixture } from './league-overview-fixtures';

interface ScoringMarginChartProps {
  data: WeeklyMarginFixture[];
}

const AXIS_TICK_STYLE = { fontSize: 11, fill: 'var(--chart-axis-text)' };
const TOOLTIP_CONTENT_STYLE = {
  backgroundColor: 'var(--chart-tooltip-bg)',
  borderColor: 'var(--chart-tooltip-border)',
  color: 'var(--chart-tooltip-text)',
};
const TOOLTIP_LABEL_STYLE = { color: 'var(--chart-tooltip-text)' };

function formatMargin(value: number): string {
  return value >= 0 ? `+${value.toFixed(1)}` : value.toFixed(1);
}

/** Tracks `prefers-reduced-motion` so the chart's bar-entry animation can be
 * turned off — recharts animates by default and has no CSS-only opt-out. */
function usePrefersReducedMotion(): boolean {
  const [reduced, setReduced] = useState(false);

  useEffect(() => {
    const query = window.matchMedia('(prefers-reduced-motion: reduce)');
    setReduced(query.matches);
    const listener = (event: MediaQueryListEvent) => setReduced(event.matches);
    query.addEventListener('change', listener);
    return () => query.removeEventListener('change', listener);
  }, []);

  return reduced;
}

/**
 * Single-series chart: weekly scoring margin (points for minus points
 * against) for the mock user's team. Bars are colored by sign using the
 * data-meaning `--chart-positive`/`--chart-negative` tokens (not the
 * qualitative `--chart-series-*` palette, since there's only one series).
 *
 * Sign is never color-only: the zero reference line plus each bar's
 * direction (above/below it) and its printed `+`/`-` label are the
 * non-color cues (responsive-rules.md Section 6).
 */
export default function ScoringMarginChart({ data }: ScoringMarginChartProps) {
  const prefersReducedMotion = usePrefersReducedMotion();

  return (
    <figure className="m-0">
      <ResponsiveContainer width="100%" height={260}>
        <BarChart data={data} margin={{ top: 16, right: 12, left: 0, bottom: 4 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--chart-grid)" opacity={0.5} />
          <XAxis dataKey="week" tick={AXIS_TICK_STYLE} />
          <YAxis tick={AXIS_TICK_STYLE} allowDecimals={false} />
          <Tooltip
            contentStyle={TOOLTIP_CONTENT_STYLE}
            labelStyle={TOOLTIP_LABEL_STYLE}
            formatter={(value) => [formatMargin(Number(value)), 'Margin']}
          />
          <ReferenceLine y={0} stroke="var(--chart-zero-line)" />
          <Bar dataKey="margin" name="Margin" isAnimationActive={!prefersReducedMotion}>
            {data.map((point) => (
              <Cell
                key={point.week}
                fill={point.margin >= 0 ? 'var(--chart-positive)' : 'var(--chart-negative)'}
              />
            ))}
            <LabelList
              dataKey="margin"
              position="top"
              formatter={(value: unknown) => formatMargin(value as number)}
              style={{ fill: 'var(--chart-axis-text)', fontSize: 11 }}
            />
          </Bar>
        </BarChart>
      </ResponsiveContainer>

      <figcaption className="mt-2 text-xs" style={{ color: 'var(--text-muted)' }}>
        Weekly scoring margin, weeks 7-14. Bars above the zero line are weeks
        the mock team outscored its opponent; bars below are weeks it didn&apos;t.
      </figcaption>

      {/* Text alternative for the chart data, per component-state-matrix.md's
          chart-container accessibility requirement ("Chart data has a text
          alternative"). Visually hidden since the figure + caption above
          already convey the same information sighted users need. */}
      <table className="sr-only">
        <caption>Weekly scoring margin by week</caption>
        <thead>
          <tr>
            <th scope="col">Week</th>
            <th scope="col">Margin</th>
          </tr>
        </thead>
        <tbody>
          {data.map((point) => (
            <tr key={point.week}>
              <td>{point.week}</td>
              <td>{formatMargin(point.margin)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </figure>
  );
}
