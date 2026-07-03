import Layout from "../../components/Layout";
import { useAdminBacklog } from "../../hooks/useAdminBacklog";
import { useAdminSegments } from "../../hooks/useAdminSegments";
import { useAdminDatabaseSize } from "../../hooks/useAdminDatabaseSize";
import { useAdminDiscoveryFrontier } from "../../hooks/useAdminDiscoveryFrontier";

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
    <section>
      <h2 className="text-2xl font-bold text-blue-600 mb-2">Segment Distribution</h2>
      <p className="text-gray-600 dark:text-gray-300 mb-4">
        Fetched leagues bucketed by scoring type, superflex, and league size — used to decide
        which segments are worth adding to the player-valuation model.
      </p>

      {isLoading && (
        <div className="flex items-center space-x-2">
          <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin" />
          <p>Loading segments...</p>
        </div>
      )}

      {error && <p className="text-red-600">Failed to load segment distribution.</p>}

      {!isLoading && !error && segments && (
        <div className="bg-white dark:bg-gray-700 rounded-lg shadow-md border border-gray-100 dark:border-gray-600 overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead className="bg-gray-50 dark:bg-gray-800">
              <tr>
                <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  Scoring
                </th>
                <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  Superflex
                </th>
                <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  League Size
                </th>
                <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  Leagues
                </th>
                <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  % of Total
                </th>
                <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  Transactions
                </th>
                <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  % of Total
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
              {segments.segments.map((row) => (
                <tr key={`${row.scoring}-${row.superflex}-${row.league_size}`}>
                  <td className="py-2 px-4 text-gray-800 dark:text-gray-100">{row.scoring}</td>
                  <td className="py-2 px-4 text-gray-800 dark:text-gray-100">
                    {row.superflex ? "Yes" : "No"}
                  </td>
                  <td className="py-2 px-4 text-gray-800 dark:text-gray-100">{row.league_size}</td>
                  <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                    {row.leagues.toLocaleString()}
                  </td>
                  <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                    {segments.total_leagues > 0
                      ? `${((row.leagues / segments.total_leagues) * 100).toFixed(1)}%`
                      : "—"}
                  </td>
                  <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                    {row.transactions.toLocaleString()}
                  </td>
                  <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                    {segments.total_transactions > 0
                      ? `${((row.transactions / segments.total_transactions) * 100).toFixed(1)}%`
                      : "—"}
                  </td>
                </tr>
              ))}
              {segments.segments.length === 0 && (
                <tr>
                  <td colSpan={7} className="py-4 px-4 text-center text-gray-500 dark:text-gray-400">
                    No fetched leagues yet.
                  </td>
                </tr>
              )}
            </tbody>
            {segments.segments.length > 0 && (
              <tfoot className="bg-gray-50 dark:bg-gray-800">
                <tr>
                  <td colSpan={3} className="py-2 px-4 font-medium text-gray-800 dark:text-gray-100">
                    Total
                  </td>
                  <td className="py-2 px-4 text-right font-medium text-gray-800 dark:text-gray-100">
                    {segments.total_leagues.toLocaleString()}
                  </td>
                  <td className="py-2 px-4 text-right font-medium text-gray-800 dark:text-gray-100">
                    100%
                  </td>
                  <td className="py-2 px-4 text-right font-medium text-gray-800 dark:text-gray-100">
                    {segments.total_transactions.toLocaleString()}
                  </td>
                  <td className="py-2 px-4 text-right font-medium text-gray-800 dark:text-gray-100">
                    100%
                  </td>
                </tr>
              </tfoot>
            )}
          </table>
        </div>
      )}
    </section>
  );
}

function DatabaseSize() {
  const { databaseSize, isLoading, error } = useAdminDatabaseSize();

  return (
    <section>
      <h2 className="text-2xl font-bold text-blue-600 mb-2">Database Size</h2>
      <p className="text-gray-600 dark:text-gray-300 mb-4">
        Total Postgres database size and a per-table breakdown, used to spot which tables are
        driving storage growth. Per-table sizes include their indexes and won&apos;t sum exactly
        to the total (which also covers system catalogs and free space).
      </p>

      {isLoading && (
        <div className="flex items-center space-x-2">
          <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin" />
          <p>Loading database size...</p>
        </div>
      )}

      {error && <p className="text-red-600">Failed to load database size.</p>}

      {!isLoading && !error && databaseSize && (
        <>
          <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md border border-gray-100 dark:border-gray-600 mb-4 max-w-xs">
            <div className="text-3xl font-bold text-blue-600 mb-1">
              {formatBytes(databaseSize.total_bytes)}
            </div>
            <div className="text-lg font-medium text-gray-800 dark:text-gray-100">
              Total database size
            </div>
          </div>

          <div className="bg-white dark:bg-gray-700 rounded-lg shadow-md border border-gray-100 dark:border-gray-600 overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
              <thead className="bg-gray-50 dark:bg-gray-800">
                <tr>
                  <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Table
                  </th>
                  <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Size
                  </th>
                  <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    % of Total
                  </th>
                  <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Rows (est.)
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
                {databaseSize.tables.map((row) => (
                  <tr key={row.table_name}>
                    <td className="py-2 px-4 text-gray-800 dark:text-gray-100">{row.table_name}</td>
                    <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                      {formatBytes(row.size_bytes)}
                    </td>
                    <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                      {databaseSize.total_bytes > 0
                        ? `${((row.size_bytes / databaseSize.total_bytes) * 100).toFixed(1)}%`
                        : "—"}
                    </td>
                    <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                      {row.row_estimate.toLocaleString()}
                    </td>
                  </tr>
                ))}
                {databaseSize.tables.length === 0 && (
                  <tr>
                    <td colSpan={4} className="py-4 px-4 text-center text-gray-500 dark:text-gray-400">
                      No tables found.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </>
      )}
    </section>
  );
}

function DiscoveryFrontier() {
  const { frontier, isLoading, error } = useAdminDiscoveryFrontier();

  return (
    <section>
      <h2 className="text-2xl font-bold text-blue-600 mb-2">Discovery Frontier</h2>
      <p className="text-gray-600 dark:text-gray-300 mb-4">
        How much of the league/user discovery graph is known but not yet expanded by the
        recursive discovery workflow — pending counts are the frontier still left to fetch.
      </p>

      {isLoading && (
        <div className="flex items-center space-x-2">
          <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin" />
          <p>Loading discovery frontier...</p>
        </div>
      )}

      {error && <p className="text-red-600">Failed to load discovery frontier.</p>}

      {!isLoading && !error && frontier && (
        <>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-6 mb-4">
            <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md border border-gray-100 dark:border-gray-600">
              <div className="text-3xl font-bold text-blue-600 mb-1">
                {frontier.users.total.toLocaleString()}
              </div>
              <div className="text-lg font-medium text-gray-800 dark:text-gray-100">
                Users discovered
              </div>
            </div>

            <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md border border-gray-100 dark:border-gray-600">
              <div className="text-3xl font-bold text-blue-600 mb-1">
                {frontier.users.expanded.toLocaleString()}
              </div>
              <div className="text-lg font-medium text-gray-800 dark:text-gray-100">
                Users expanded
              </div>
            </div>

            <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md border border-gray-100 dark:border-gray-600">
              <div className="text-3xl font-bold text-blue-600 mb-1">
                {frontier.users.pending.toLocaleString()}
              </div>
              <div className="text-lg font-medium text-gray-800 dark:text-gray-100">
                Users pending
              </div>
            </div>

            <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md border border-gray-100 dark:border-gray-600">
              <div className="text-3xl font-bold text-blue-600 mb-1">
                {frontier.users.skipped.toLocaleString()}
              </div>
              <div className="text-lg font-medium text-gray-800 dark:text-gray-100">
                Users skipped
              </div>
            </div>
          </div>

          <div className="bg-white dark:bg-gray-700 rounded-lg shadow-md border border-gray-100 dark:border-gray-600 overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
              <thead className="bg-gray-50 dark:bg-gray-800">
                <tr>
                  <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Season
                  </th>
                  <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Total
                  </th>
                  <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Expanded
                  </th>
                  <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Pending
                  </th>
                  <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Skipped
                  </th>
                  <th className="py-3 px-4 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    % Pending
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
                {frontier.leagues_by_season.map((row) => (
                  <tr key={row.season}>
                    <td className="py-2 px-4 text-gray-800 dark:text-gray-100">{row.season}</td>
                    <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                      {row.total.toLocaleString()}
                    </td>
                    <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                      {row.expanded.toLocaleString()}
                    </td>
                    <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                      {row.pending.toLocaleString()}
                    </td>
                    <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                      {row.skipped.toLocaleString()}
                    </td>
                    <td className="py-2 px-4 text-right text-gray-800 dark:text-gray-100">
                      {row.total > 0 ? `${((row.pending / row.total) * 100).toFixed(1)}%` : "—"}
                    </td>
                  </tr>
                ))}
                {frontier.leagues_by_season.length === 0 && (
                  <tr>
                    <td colSpan={6} className="py-4 px-4 text-center text-gray-500 dark:text-gray-400">
                      No leagues discovered yet.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </>
      )}
    </section>
  );
}

export default function AdminBacklog() {
  const { backlog, isLoading, error } = useAdminBacklog();

  return (
    <Layout>
      <div className="space-y-8">
        <section>
          <h1 className="text-3xl font-bold text-blue-600 mb-2">Admin: Sync Backlog</h1>
          <p className="text-gray-600 dark:text-gray-300">
            Sleeper transaction sync backlog for the current season, used to gauge how much to
            scale the Temporal workers.
          </p>
        </section>

        {isLoading && (
          <div className="flex items-center space-x-2">
            <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin" />
            <p>Loading backlog...</p>
          </div>
        )}

        {error && <p className="text-red-600">Failed to load backlog.</p>}

        {!isLoading && !error && backlog && (
          <section className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md border border-gray-100 dark:border-gray-600">
              <div className="text-3xl font-bold text-blue-600 mb-1">
                {backlog.never_fetched_count.toLocaleString()} / {backlog.total_leagues.toLocaleString()}
              </div>
              <div className="text-lg font-medium text-gray-800 dark:text-gray-100">
                Leagues never fetched (season {backlog.season || "—"})
              </div>
            </div>

            <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md border border-gray-100 dark:border-gray-600">
              <div className="text-3xl font-bold text-blue-600 mb-1">
                {backlog.oldest_transactions_fetched_at
                  ? formatRelativeTime(backlog.oldest_transactions_fetched_at)
                  : backlog.total_leagues === 0
                    ? "No leagues"
                    : "None fetched yet"}
              </div>
              <div className="text-lg font-medium text-gray-800 dark:text-gray-100">
                Oldest transactions fetch
              </div>
            </div>
          </section>
        )}

        <SegmentDistribution />

        <DatabaseSize />

        <DiscoveryFrontier />
      </div>
    </Layout>
  );
}
