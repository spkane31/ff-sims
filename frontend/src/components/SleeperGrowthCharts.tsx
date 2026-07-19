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
  ResponsiveContainer,
} from "recharts";
import { SleeperStats } from "../types/models";

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
            <CartesianGrid strokeDasharray="3 3" stroke="#E5E7EB" opacity={0.3} />
            <XAxis dataKey="label" tick={{ fontSize: 11 }} minTickGap={40} />
            <YAxis tick={{ fontSize: 11 }} allowDecimals={false} />
            <Tooltip />
            <Legend wrapperStyle={{ fontSize: 12 }} />
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
            <CartesianGrid strokeDasharray="3 3" stroke="#E5E7EB" opacity={0.3} />
            <XAxis dataKey="label" tick={{ fontSize: 11 }} minTickGap={40} />
            <YAxis tick={{ fontSize: 11 }} allowDecimals={false} />
            <Tooltip />
            <Legend wrapperStyle={{ fontSize: 12 }} />
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
              <CartesianGrid strokeDasharray="3 3" stroke="#E5E7EB" opacity={0.3} />
              <XAxis dataKey="label" tick={{ fontSize: 11 }} minTickGap={40} />
              <YAxis tick={{ fontSize: 11 }} allowDecimals={false} />
              <Tooltip />
              <Legend wrapperStyle={{ fontSize: 12 }} />
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
