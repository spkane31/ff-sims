import Layout from "../../components/Layout";
import { useAdminBacklog } from "../../hooks/useAdminBacklog";

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
      </div>
    </Layout>
  );
}
