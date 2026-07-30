import { useAdminBacklog } from "../../hooks/useAdminBacklog";
import { useAdminSegments } from "../../hooks/useAdminSegments";
import { useAdminDatabaseSize } from "../../hooks/useAdminDatabaseSize";
import { useAdminDiscoveryFrontier } from "../../hooks/useAdminDiscoveryFrontier";
import { useSleeperStatsHistory } from "../../hooks/useSleeperData";
import {
  AdminBacklogResponse,
  AdminBacklogBucketRow,
  AdminTableSizeRow,
  AdminDiscoveryLeagueSeasonRow,
} from "../../services/adminService";
import {
  UsersDiscoveryChart,
  LeaguesDiscoveryChart,
  ArchiveGrowthChart,
  UsersDiscoveryRateChart,
  LeaguesDiscoveryRateChart,
  ArchiveGrowthRateChart,
} from "../../components/SleeperGrowthCharts";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import ErrorState from "@/components/design-system/ErrorState";
import EmptyState from "@/components/design-system/EmptyState";
import StatCard from "@/components/design-system/StatCard";
import DataTable, {
  type DataTableColumn,
} from "@/components/design-system/DataTable";

function formatRelativeTime(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime();
  const totalSeconds = Math.max(0, Math.floor(diffMs / 1000));
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  const unit = (n: number, name: string) => `${n} ${name}${n === 1 ? "" : "s"}`;

  if (days > 0) return `${unit(days, "day")} ${unit(hours, "hour")} ago`;
  if (hours > 0) return `${unit(hours, "hour")} ${unit(minutes, "minute")} ago`;
  if (minutes > 0) return `${unit(minutes, "minute")} ${unit(seconds, "second")} ago`;
  return `${unit(seconds, "second")} ago`;
}

function formatBytes(bytes: number): string {
  if (bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / Math.pow(1024, exponent);
  return `${value.toFixed(exponent === 0 ? 0 : 1)} ${units[exponent]}`;
}

function SegmentDistribution() {
  const { segments, isLoading, error } = useAdminSegments();

  return (
    <Card>
      <CardContent className="p-6">
        <h2 className="text-2xl font-bold mb-2" style={{ color: "var(--text-primary)" }}>
          Segment Distribution
        </h2>
        <p className="mb-4" style={{ color: "var(--text-muted)" }}>
          Fetched leagues bucketed by scoring type, superflex, and league size — used to decide
          which segments are worth adding to the player-valuation model.
        </p>

        {isLoading && (
          <div className="space-y-2">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
        )}

        {error && <ErrorState message="Failed to load segment distribution." />}

        {!isLoading && !error && segments && (
          <div className="overflow-x-auto">
            <table className="min-w-full border-collapse text-sm">
              <thead>
                <tr style={{ borderBottom: "1px solid var(--border-subtle)" }}>
                  <th
                    className="whitespace-nowrap px-4 py-3 text-left text-xs font-medium uppercase tracking-wider"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Scoring
                  </th>
                  <th
                    className="whitespace-nowrap px-4 py-3 text-left text-xs font-medium uppercase tracking-wider"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Superflex
                  </th>
                  <th
                    className="whitespace-nowrap px-4 py-3 text-left text-xs font-medium uppercase tracking-wider"
                    style={{ color: "var(--text-muted)" }}
                  >
                    League Size
                  </th>
                  <th
                    className="whitespace-nowrap px-4 py-3 text-right text-xs font-medium uppercase tracking-wider"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Leagues
                  </th>
                  <th
                    className="whitespace-nowrap px-4 py-3 text-right text-xs font-medium uppercase tracking-wider"
                    style={{ color: "var(--text-muted)" }}
                  >
                    % of Total
                  </th>
                  <th
                    className="whitespace-nowrap px-4 py-3 text-right text-xs font-medium uppercase tracking-wider"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Transactions
                  </th>
                  <th
                    className="whitespace-nowrap px-4 py-3 text-right text-xs font-medium uppercase tracking-wider"
                    style={{ color: "var(--text-muted)" }}
                  >
                    % of Total
                  </th>
                </tr>
              </thead>
              <tbody>
                {segments.segments.map((row, i) => (
                  <tr
                    key={`${row.scoring}-${row.superflex}-${row.league_size}`}
                    style={{
                      backgroundColor:
                        i % 2 === 0 ? "var(--surface-raised)" : "var(--surface-sunken)",
                      borderBottom: "1px solid var(--border-subtle)",
                    }}
                  >
                    <td className="whitespace-nowrap px-4 py-4" style={{ color: "var(--text-primary)" }}>
                      {row.scoring}
                    </td>
                    <td className="whitespace-nowrap px-4 py-4" style={{ color: "var(--text-primary)" }}>
                      {row.superflex ? "Yes" : "No"}
                    </td>
                    <td className="whitespace-nowrap px-4 py-4" style={{ color: "var(--text-primary)" }}>
                      {row.league_size}
                    </td>
                    <td
                      className="whitespace-nowrap px-4 py-4 text-right"
                      style={{ color: "var(--text-primary)" }}
                    >
                      {row.leagues.toLocaleString()}
                    </td>
                    <td
                      className="whitespace-nowrap px-4 py-4 text-right"
                      style={{ color: "var(--text-primary)" }}
                    >
                      {segments.total_leagues > 0
                        ? `${((row.leagues / segments.total_leagues) * 100).toFixed(1)}%`
                        : "—"}
                    </td>
                    <td
                      className="whitespace-nowrap px-4 py-4 text-right"
                      style={{ color: "var(--text-primary)" }}
                    >
                      {row.transactions.toLocaleString()}
                    </td>
                    <td
                      className="whitespace-nowrap px-4 py-4 text-right"
                      style={{ color: "var(--text-primary)" }}
                    >
                      {segments.total_transactions > 0
                        ? `${((row.transactions / segments.total_transactions) * 100).toFixed(1)}%`
                        : "—"}
                    </td>
                  </tr>
                ))}
                {segments.segments.length === 0 && (
                  <tr>
                    <td
                      colSpan={7}
                      className="px-4 py-4 text-center"
                      style={{ color: "var(--text-muted)" }}
                    >
                      No fetched leagues yet.
                    </td>
                  </tr>
                )}
              </tbody>
              {segments.segments.length > 0 && (
                <tfoot>
                  <tr style={{ borderTop: "2px solid var(--border-strong)" }}>
                    <td
                      colSpan={3}
                      className="whitespace-nowrap px-4 py-3 font-medium"
                      style={{ color: "var(--text-primary)" }}
                    >
                      Total
                    </td>
                    <td
                      className="whitespace-nowrap px-4 py-3 text-right font-medium"
                      style={{ color: "var(--text-primary)" }}
                    >
                      {segments.total_leagues.toLocaleString()}
                    </td>
                    <td
                      className="whitespace-nowrap px-4 py-3 text-right font-medium"
                      style={{ color: "var(--text-primary)" }}
                    >
                      100%
                    </td>
                    <td
                      className="whitespace-nowrap px-4 py-3 text-right font-medium"
                      style={{ color: "var(--text-primary)" }}
                    >
                      {segments.total_transactions.toLocaleString()}
                    </td>
                    <td
                      className="whitespace-nowrap px-4 py-3 text-right font-medium"
                      style={{ color: "var(--text-primary)" }}
                    >
                      100%
                    </td>
                  </tr>
                </tfoot>
              )}
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function DatabaseSize() {
  const { databaseSize, isLoading, error } = useAdminDatabaseSize();

  const columns: DataTableColumn<AdminTableSizeRow>[] = [
    { id: "table_name", header: "Table", cell: (row) => row.table_name },
    {
      id: "size_bytes",
      header: "Size",
      align: "right",
      cell: (row) => formatBytes(row.size_bytes),
    },
    {
      id: "pct_of_total",
      header: "% of Total",
      align: "right",
      cell: (row) =>
        databaseSize && databaseSize.total_bytes > 0
          ? `${((row.size_bytes / databaseSize.total_bytes) * 100).toFixed(1)}%`
          : "—",
    },
    {
      id: "row_estimate",
      header: "Rows (est.)",
      align: "right",
      cell: (row) => row.row_estimate.toLocaleString(),
    },
  ];

  return (
    <Card>
      <CardContent className="p-6">
        <h2 className="text-2xl font-bold mb-2" style={{ color: "var(--text-primary)" }}>
          Database Size
        </h2>
        <p className="mb-4" style={{ color: "var(--text-muted)" }}>
          Total Postgres database size and a per-table breakdown, used to spot which tables are
          driving storage growth. Per-table sizes include their indexes and won&apos;t sum exactly
          to the total (which also covers system catalogs and free space).
        </p>

        {isLoading && (
          <div className="space-y-4">
            <Skeleton className="h-24 w-full max-w-xs" />
            <div className="space-y-2">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          </div>
        )}

        {error && <ErrorState message="Failed to load database size." />}

        {!isLoading && !error && databaseSize && (
          <>
            <div className="mb-4 max-w-xs">
              <StatCard label="Total database size" value={formatBytes(databaseSize.total_bytes)} />
            </div>

            {databaseSize.tables.length > 0 ? (
              <DataTable
                columns={columns}
                rows={databaseSize.tables}
                rowKey={(row) => row.table_name}
              />
            ) : (
              <EmptyState title="No tables found." />
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}

function DiscoveryFrontier({ backlog }: { backlog: AdminBacklogResponse | null }) {
  const { frontier, isLoading, error } = useAdminDiscoveryFrontier();

  const seasonColumns: DataTableColumn<AdminDiscoveryLeagueSeasonRow>[] = [
    { id: "season", header: "Season", cell: (row) => row.season },
    { id: "total", header: "Total", align: "right", cell: (row) => row.total.toLocaleString() },
    {
      id: "expanded",
      header: "Expanded",
      align: "right",
      cell: (row) => row.expanded.toLocaleString(),
    },
    {
      id: "pending",
      header: "Pending",
      align: "right",
      cell: (row) => row.pending.toLocaleString(),
    },
    {
      id: "skipped",
      header: "Skipped",
      align: "right",
      cell: (row) => row.skipped.toLocaleString(),
    },
    {
      id: "pct_pending",
      header: "% Pending",
      align: "right",
      cell: (row) => (row.total > 0 ? `${((row.pending / row.total) * 100).toFixed(1)}%` : "—"),
    },
  ];

  const bucketColumns: DataTableColumn<AdminBacklogBucketRow>[] = [
    { id: "label", header: "Bucket", cell: (row) => row.label },
    {
      id: "leagues",
      header: "Leagues",
      align: "right",
      cell: (row) => row.leagues.toLocaleString(),
    },
    {
      id: "pct_of_total",
      header: "% of Total",
      align: "right",
      cell: (row) =>
        backlog && backlog.total_leagues > 0
          ? `${((row.leagues / backlog.total_leagues) * 100).toFixed(1)}%`
          : "—",
    },
  ];

  return (
    <Card>
      <CardContent className="p-6">
        <h2 className="text-2xl font-bold mb-2" style={{ color: "var(--text-primary)" }}>
          Discovery Frontier
        </h2>
        <p className="mb-4" style={{ color: "var(--text-muted)" }}>
          How much of the league/user discovery graph is known but not yet expanded by the
          recursive discovery workflow — pending counts are the frontier still left to fetch.
        </p>

        {isLoading && (
          <div className="space-y-4">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-6">
              <Skeleton className="h-24 w-full" />
              <Skeleton className="h-24 w-full" />
              <Skeleton className="h-24 w-full" />
              <Skeleton className="h-24 w-full" />
            </div>
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
        )}

        {error && <ErrorState message="Failed to load discovery frontier." />}

        {!isLoading && !error && frontier && (
          <>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-6 mb-4">
              <StatCard label="Users discovered" value={frontier.users.total.toLocaleString()} />
              <StatCard label="Users expanded" value={frontier.users.expanded.toLocaleString()} />
              <StatCard label="Users pending" value={frontier.users.pending.toLocaleString()} />
              <StatCard label="Users skipped" value={frontier.users.skipped.toLocaleString()} />
            </div>

            {frontier.leagues_by_season.length > 0 ? (
              <DataTable
                columns={seasonColumns}
                rows={frontier.leagues_by_season}
                rowKey={(row) => row.season}
              />
            ) : (
              <EmptyState title="No leagues discovered yet." />
            )}

            <p className="text-sm mt-2" style={{ color: "var(--text-muted)" }}>
              Total is every league discovered that season; Expanded means the discovery workflow
              has fetched it (<code>last_fetched_at</code> set); Pending is discovered but not yet
              expanded — the frontier left to crawl; Skipped is permanently excluded and doesn&apos;t
              count toward pending.
            </p>

            <h3 className="text-xl font-semibold mt-8 mb-2" style={{ color: "var(--text-primary)" }}>
              Transaction Fetch Age (season {backlog?.season || "—"})
            </h3>

            {backlog && backlog.total_leagues > 0 ? (
              <DataTable
                columns={bucketColumns}
                rows={backlog.buckets}
                rowKey={(row) => row.label}
              />
            ) : (
              <EmptyState title="No leagues yet." />
            )}

            <p className="text-sm mt-2" style={{ color: "var(--text-muted)" }}>
              How stale each current-season league&apos;s transaction sync is, bucketed in 4-hour
              increments, to help gauge how much to scale the Temporal workers.
            </p>
          </>
        )}
      </CardContent>
    </Card>
  );
}

function LifetimeGrowth() {
  const { snapshots, isLoading, error } = useSleeperStatsHistory();

  return (
    <Card>
      <CardContent className="p-6">
        <h2 className="text-2xl font-bold mb-2" style={{ color: "var(--text-primary)" }}>
          Lifetime Growth
        </h2>
        <p className="mb-4" style={{ color: "var(--text-muted)" }}>
          Hourly snapshots of discovery state and all-time totals from{" "}
          <code>sleeper_lifetime_counts</code>, over the last 7 days — the same rollup the home
          page&apos;s totals are drawn from, so it stays accurate even after the cloud database
          purges old data.
        </p>

        {isLoading && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-64 w-full" />
            ))}
          </div>
        )}

        {error && <ErrorState message="Failed to load lifetime growth." />}

        {!isLoading && !error && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <UsersDiscoveryChart snapshots={snapshots} />
            <LeaguesDiscoveryChart snapshots={snapshots} />
            <ArchiveGrowthChart snapshots={snapshots} />
            <UsersDiscoveryRateChart snapshots={snapshots} />
            <LeaguesDiscoveryRateChart snapshots={snapshots} />
            <ArchiveGrowthRateChart snapshots={snapshots} />
          </div>
        )}
      </CardContent>
    </Card>
  );
}

export default function AdminBacklog() {
  const { backlog, isLoading, error } = useAdminBacklog();

  return (
    <div className="space-y-8">
      <section>
        <h1 className="text-3xl font-bold mb-2" style={{ color: "var(--text-primary)" }}>
          Admin: Sync Backlog
        </h1>
        <p style={{ color: "var(--text-muted)" }}>
          Sleeper transaction sync backlog for the current season, used to gauge how much to
          scale the Temporal workers.
        </p>
      </section>

      {isLoading && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-24 w-full" />
        </div>
      )}

      {error && <ErrorState message="Failed to load backlog." />}

      {!isLoading && !error && backlog && (
        <section className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <StatCard
            label={`Leagues never fetched (season ${backlog.season || "—"})`}
            value={`${backlog.never_fetched_count.toLocaleString()} / ${backlog.total_leagues.toLocaleString()}`}
          />
          <StatCard
            label="Oldest transactions fetch"
            value={
              backlog.oldest_transactions_fetched_at
                ? formatRelativeTime(backlog.oldest_transactions_fetched_at)
                : backlog.total_leagues === 0
                  ? "No leagues"
                  : "None fetched yet"
            }
          />
        </section>
      )}

      <SegmentDistribution />

      <DatabaseSize />

      <DiscoveryFrontier backlog={backlog} />

      <LifetimeGrowth />
    </div>
  );
}
