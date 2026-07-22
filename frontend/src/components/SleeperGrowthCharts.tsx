import { useMemo } from "react";
import {
  ComposedChart,
  LineChart,
  Area,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ReferenceLine,
  ResponsiveContainer,
} from "recharts";
import { SleeperStats } from "../types/models";

// Theme-aware chart colors, defined in globals.css as CSS variables that flip
// with prefers-color-scheme — avoids recharts' default gray (#666) tick/legend
// text, which is hard to read against the dark-mode card background.
const AXIS_TICK_STYLE = { fontSize: 11, fill: "var(--chart-axis-text)" };
const LEGEND_STYLE = { fontSize: 12, color: "var(--chart-legend-text)" };
const TOOLTIP_CONTENT_STYLE = {
  backgroundColor: "var(--chart-tooltip-bg)",
  borderColor: "var(--chart-tooltip-border)",
  color: "var(--chart-tooltip-text)",
};
const TOOLTIP_LABEL_STYLE = { color: "var(--chart-tooltip-text)" };

interface ChartPoint {
  snapshotAt: string;
  label: string;
  usersTotal: number;
  usersExpanded: number;
  usersPending: number;
  usersSkipped: number;
  leaguesTotal: number;
  leaguesExpanded: number;
  leaguesPending: number;
  leaguesSkipped: number;
  transactionsTotal?: number;
  tradeCount: number;
  draftCount: number;
}

function formatTick(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
  });
}

// prepareChartData reverses sleeper_lifetime_counts' most-recent-first series
// into chronological order (oldest to newest) for the charts below.
function prepareChartData(snapshots: SleeperStats[]): ChartPoint[] {
  return snapshots
    .slice()
    .sort((a, b) => new Date(a.snapshot_at).getTime() - new Date(b.snapshot_at).getTime())
    .map((s) => ({
      snapshotAt: s.snapshot_at,
      label: formatTick(s.snapshot_at),
      usersTotal: s.users_total,
      usersExpanded: s.users_expanded,
      usersPending: s.users_pending,
      usersSkipped: s.users_skipped,
      leaguesTotal: s.leagues_total,
      leaguesExpanded: s.leagues_expanded,
      leaguesPending: s.leagues_pending,
      leaguesSkipped: s.leagues_skipped,
      transactionsTotal: s.transactions_total,
      tradeCount: s.trade_count,
      draftCount: s.draft_count,
    }));
}

function ChartCard({
  title,
  description,
  className,
  children,
}: {
  title: string;
  description: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div
      className={`bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md border border-gray-100 dark:border-gray-600 ${className ?? ""}`}
    >
      <h3 className="text-lg font-semibold text-gray-800 dark:text-gray-100 mb-1">{title}</h3>
      <p className="text-sm text-gray-600 dark:text-gray-400 mb-4">{description}</p>
      {children}
    </div>
  );
}

function EmptyChartState() {
  return (
    <div className="flex items-center justify-center h-[300px] text-gray-500 dark:text-gray-400 text-sm">
      No snapshots yet.
    </div>
  );
}

interface GrowthChartProps {
  snapshots: SleeperStats[];
}

export function UsersDiscoveryChart({ snapshots }: GrowthChartProps) {
  const data = useMemo(() => prepareChartData(snapshots), [snapshots]);

  return (
    <ChartCard
      title="Users Discovered Over Time"
      description="User discovery backlog — expanded (fetched), pending (frontier still to crawl), and skipped — against the running total, from the hourly rollup."
    >
      {data.length === 0 ? (
        <EmptyChartState />
      ) : (
        <ResponsiveContainer width="100%" height={300}>
          <ComposedChart data={data} margin={{ top: 5, right: 20, left: 0, bottom: 5 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="var(--chart-grid)" opacity={0.5} />
            <XAxis dataKey="label" tick={AXIS_TICK_STYLE} minTickGap={40} />
            <YAxis tick={AXIS_TICK_STYLE} allowDecimals={false} />
            <Tooltip contentStyle={TOOLTIP_CONTENT_STYLE} labelStyle={TOOLTIP_LABEL_STYLE} />
            <Legend wrapperStyle={LEGEND_STYLE} />
            <Area
              type="monotone"
              dataKey="usersExpanded"
              name="Expanded"
              stackId="users"
              stroke="#059669"
              fill="#059669"
              fillOpacity={0.6}
            />
            <Area
              type="monotone"
              dataKey="usersPending"
              name="Pending"
              stackId="users"
              stroke="#F59E0B"
              fill="#F59E0B"
              fillOpacity={0.6}
            />
            <Area
              type="monotone"
              dataKey="usersSkipped"
              name="Skipped"
              stackId="users"
              stroke="#9CA3AF"
              fill="#9CA3AF"
              fillOpacity={0.6}
            />
            <Line
              type="monotone"
              dataKey="usersTotal"
              name="Total"
              stroke="#3B82F6"
              strokeDasharray="6 3"
              strokeWidth={2}
              dot={false}
            />
          </ComposedChart>
        </ResponsiveContainer>
      )}
    </ChartCard>
  );
}

export function LeaguesDiscoveryChart({ snapshots }: GrowthChartProps) {
  const data = useMemo(() => prepareChartData(snapshots), [snapshots]);

  return (
    <ChartCard
      title="Leagues Discovered Over Time"
      description="League discovery backlog — expanded (fetched), pending (frontier still to crawl), and skipped — against the running total, from the hourly rollup."
    >
      {data.length === 0 ? (
        <EmptyChartState />
      ) : (
        <ResponsiveContainer width="100%" height={300}>
          <ComposedChart data={data} margin={{ top: 5, right: 20, left: 0, bottom: 5 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="var(--chart-grid)" opacity={0.5} />
            <XAxis dataKey="label" tick={AXIS_TICK_STYLE} minTickGap={40} />
            <YAxis tick={AXIS_TICK_STYLE} allowDecimals={false} />
            <Tooltip contentStyle={TOOLTIP_CONTENT_STYLE} labelStyle={TOOLTIP_LABEL_STYLE} />
            <Legend wrapperStyle={LEGEND_STYLE} />
            <Area
              type="monotone"
              dataKey="leaguesExpanded"
              name="Expanded"
              stackId="leagues"
              stroke="#059669"
              fill="#059669"
              fillOpacity={0.6}
            />
            <Area
              type="monotone"
              dataKey="leaguesPending"
              name="Pending"
              stackId="leagues"
              stroke="#F59E0B"
              fill="#F59E0B"
              fillOpacity={0.6}
            />
            <Area
              type="monotone"
              dataKey="leaguesSkipped"
              name="Skipped"
              stackId="leagues"
              stroke="#9CA3AF"
              fill="#9CA3AF"
              fillOpacity={0.6}
            />
            <Line
              type="monotone"
              dataKey="leaguesTotal"
              name="Total"
              stroke="#3B82F6"
              strokeDasharray="6 3"
              strokeWidth={2}
              dot={false}
            />
          </ComposedChart>
        </ResponsiveContainer>
      )}
    </ChartCard>
  );
}

export function ArchiveGrowthChart({ snapshots }: GrowthChartProps) {
  const data = useMemo(() => prepareChartData(snapshots), [snapshots]);
  const hasTransactionsData = useMemo(
    () => data.some((d) => d.transactionsTotal !== undefined && d.transactionsTotal !== null),
    [data]
  );

  return (
    <ChartCard
      className="md:col-span-2"
      title="Archive Growth"
      description="All-time transaction and completed trade/draft counts from the archive database, immune to the cloud database's purge window."
    >
      {data.length === 0 ? (
        <EmptyChartState />
      ) : (
        <>
          <ResponsiveContainer width="100%" height={300}>
            <LineChart data={data} margin={{ top: 5, right: 20, left: 0, bottom: 5 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--chart-grid)" opacity={0.5} />
              <XAxis dataKey="label" tick={AXIS_TICK_STYLE} minTickGap={40} />
              <YAxis tick={AXIS_TICK_STYLE} allowDecimals={false} />
              <Tooltip contentStyle={TOOLTIP_CONTENT_STYLE} labelStyle={TOOLTIP_LABEL_STYLE} />
              <Legend wrapperStyle={LEGEND_STYLE} />
              {hasTransactionsData && (
                <Line
                  type="monotone"
                  dataKey="transactionsTotal"
                  name="Transactions"
                  stroke="#3B82F6"
                  strokeWidth={2}
                  dot={false}
                  connectNulls
                />
              )}
              <Line
                type="monotone"
                dataKey="tradeCount"
                name="Trades"
                stroke="#059669"
                strokeWidth={2}
                dot={false}
              />
              <Line
                type="monotone"
                dataKey="draftCount"
                name="Drafts"
                stroke="#8B5CF6"
                strokeWidth={2}
                dot={false}
              />
            </LineChart>
          </ResponsiveContainer>
          {!hasTransactionsData && (
            <p className="text-xs text-gray-500 dark:text-gray-400 mt-2">
              Transaction totals aren&apos;t shown for this window because no archive database was
              configured for those snapshots.
            </p>
          )}
        </>
      )}
    </ChartCard>
  );
}

interface RatePoint {
  label: string;
  usersTotalDelta: number;
  usersExpandedDelta: number;
  usersPendingDelta: number;
  usersSkippedDelta: number;
  leaguesTotalDelta: number;
  leaguesExpandedDelta: number;
  leaguesPendingDelta: number;
  leaguesSkippedDelta: number;
  transactionsTotalDelta?: number;
  tradeCountDelta: number;
  draftCountDelta: number;
}

// prepareRateData turns consecutive hourly snapshots into per-snapshot deltas,
// i.e. the rate of change, so growth vs. plateau is visible at a glance.
function prepareRateData(data: ChartPoint[]): RatePoint[] {
  const rates: RatePoint[] = [];
  for (let i = 1; i < data.length; i++) {
    const prev = data[i - 1];
    const curr = data[i];
    rates.push({
      label: curr.label,
      usersTotalDelta: curr.usersTotal - prev.usersTotal,
      usersExpandedDelta: curr.usersExpanded - prev.usersExpanded,
      usersPendingDelta: curr.usersPending - prev.usersPending,
      usersSkippedDelta: curr.usersSkipped - prev.usersSkipped,
      leaguesTotalDelta: curr.leaguesTotal - prev.leaguesTotal,
      leaguesExpandedDelta: curr.leaguesExpanded - prev.leaguesExpanded,
      leaguesPendingDelta: curr.leaguesPending - prev.leaguesPending,
      leaguesSkippedDelta: curr.leaguesSkipped - prev.leaguesSkipped,
      transactionsTotalDelta:
        curr.transactionsTotal !== undefined && prev.transactionsTotal !== undefined
          ? curr.transactionsTotal - prev.transactionsTotal
          : undefined,
      tradeCountDelta: curr.tradeCount - prev.tradeCount,
      draftCountDelta: curr.draftCount - prev.draftCount,
    });
  }
  return rates;
}

export function UsersDiscoveryRateChart({ snapshots }: GrowthChartProps) {
  const data = useMemo(() => prepareRateData(prepareChartData(snapshots)), [snapshots]);

  return (
    <ChartCard
      title="Users Discovery Rate"
      description="Change since the previous hourly snapshot for each users series above — a flat line near zero means that count has plateaued."
    >
      {data.length === 0 ? (
        <EmptyChartState />
      ) : (
        <ResponsiveContainer width="100%" height={300}>
          <LineChart data={data} margin={{ top: 5, right: 20, left: 0, bottom: 5 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="var(--chart-grid)" opacity={0.5} />
            <XAxis dataKey="label" tick={AXIS_TICK_STYLE} minTickGap={40} />
            <YAxis tick={AXIS_TICK_STYLE} allowDecimals={false} />
            <Tooltip contentStyle={TOOLTIP_CONTENT_STYLE} labelStyle={TOOLTIP_LABEL_STYLE} />
            <Legend wrapperStyle={LEGEND_STYLE} />
            <ReferenceLine y={0} stroke="var(--chart-zero-line)" />
            <Line
              type="monotone"
              dataKey="usersExpandedDelta"
              name="Expanded Δ"
              stroke="#059669"
              strokeWidth={2}
              dot={false}
            />
            <Line
              type="monotone"
              dataKey="usersPendingDelta"
              name="Pending Δ"
              stroke="#F59E0B"
              strokeWidth={2}
              dot={false}
            />
            <Line
              type="monotone"
              dataKey="usersSkippedDelta"
              name="Skipped Δ"
              stroke="#9CA3AF"
              strokeWidth={2}
              dot={false}
            />
            <Line
              type="monotone"
              dataKey="usersTotalDelta"
              name="Total Δ"
              stroke="#3B82F6"
              strokeDasharray="6 3"
              strokeWidth={2}
              dot={false}
            />
          </LineChart>
        </ResponsiveContainer>
      )}
    </ChartCard>
  );
}

export function LeaguesDiscoveryRateChart({ snapshots }: GrowthChartProps) {
  const data = useMemo(() => prepareRateData(prepareChartData(snapshots)), [snapshots]);

  return (
    <ChartCard
      title="Leagues Discovery Rate"
      description="Change since the previous hourly snapshot for each leagues series above — a flat line near zero means that count has plateaued."
    >
      {data.length === 0 ? (
        <EmptyChartState />
      ) : (
        <ResponsiveContainer width="100%" height={300}>
          <LineChart data={data} margin={{ top: 5, right: 20, left: 0, bottom: 5 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="var(--chart-grid)" opacity={0.5} />
            <XAxis dataKey="label" tick={AXIS_TICK_STYLE} minTickGap={40} />
            <YAxis tick={AXIS_TICK_STYLE} allowDecimals={false} />
            <Tooltip contentStyle={TOOLTIP_CONTENT_STYLE} labelStyle={TOOLTIP_LABEL_STYLE} />
            <Legend wrapperStyle={LEGEND_STYLE} />
            <ReferenceLine y={0} stroke="var(--chart-zero-line)" />
            <Line
              type="monotone"
              dataKey="leaguesExpandedDelta"
              name="Expanded Δ"
              stroke="#059669"
              strokeWidth={2}
              dot={false}
            />
            <Line
              type="monotone"
              dataKey="leaguesPendingDelta"
              name="Pending Δ"
              stroke="#F59E0B"
              strokeWidth={2}
              dot={false}
            />
            <Line
              type="monotone"
              dataKey="leaguesSkippedDelta"
              name="Skipped Δ"
              stroke="#9CA3AF"
              strokeWidth={2}
              dot={false}
            />
            <Line
              type="monotone"
              dataKey="leaguesTotalDelta"
              name="Total Δ"
              stroke="#3B82F6"
              strokeDasharray="6 3"
              strokeWidth={2}
              dot={false}
            />
          </LineChart>
        </ResponsiveContainer>
      )}
    </ChartCard>
  );
}

export function ArchiveGrowthRateChart({ snapshots }: GrowthChartProps) {
  const chartData = useMemo(() => prepareChartData(snapshots), [snapshots]);
  const data = useMemo(() => prepareRateData(chartData), [chartData]);
  const hasTransactionsData = useMemo(
    () => data.some((d) => d.transactionsTotalDelta !== undefined && d.transactionsTotalDelta !== null),
    [data]
  );

  return (
    <ChartCard
      className="md:col-span-2"
      title="Archive Growth Rate"
      description="Change since the previous hourly snapshot in transaction and completed trade/draft counts — useful for spotting when archive replication stalls."
    >
      {data.length === 0 ? (
        <EmptyChartState />
      ) : (
        <>
          <ResponsiveContainer width="100%" height={300}>
            <LineChart data={data} margin={{ top: 5, right: 20, left: 0, bottom: 5 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--chart-grid)" opacity={0.5} />
              <XAxis dataKey="label" tick={AXIS_TICK_STYLE} minTickGap={40} />
              <YAxis tick={AXIS_TICK_STYLE} allowDecimals={false} />
              <Tooltip contentStyle={TOOLTIP_CONTENT_STYLE} labelStyle={TOOLTIP_LABEL_STYLE} />
              <Legend wrapperStyle={LEGEND_STYLE} />
              <ReferenceLine y={0} stroke="var(--chart-zero-line)" />
              {hasTransactionsData && (
                <Line
                  type="monotone"
                  dataKey="transactionsTotalDelta"
                  name="Transactions Δ"
                  stroke="#3B82F6"
                  strokeWidth={2}
                  dot={false}
                  connectNulls
                />
              )}
              <Line
                type="monotone"
                dataKey="tradeCountDelta"
                name="Trades Δ"
                stroke="#059669"
                strokeWidth={2}
                dot={false}
              />
              <Line
                type="monotone"
                dataKey="draftCountDelta"
                name="Drafts Δ"
                stroke="#8B5CF6"
                strokeWidth={2}
                dot={false}
              />
            </LineChart>
          </ResponsiveContainer>
          {!hasTransactionsData && (
            <p className="text-xs text-gray-500 dark:text-gray-400 mt-2">
              Transaction totals aren&apos;t shown for this window because no archive database was
              configured for those snapshots.
            </p>
          )}
        </>
      )}
    </ChartCard>
  );
}
