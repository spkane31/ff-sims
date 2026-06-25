import { useState } from "react";
import Layout from "../../components/Layout";
import { useSleeperTrades } from "../../hooks/useSleeperData";
import { SleeperTrade } from "../../types/models";

const LIMIT = 25;

function formatDate(unixMs: number): string {
  if (!unixMs) return "—";
  return new Date(unixMs).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function playerList(map: SleeperTrade["adds"] | SleeperTrade["drops"]): string {
  if (!map || Object.keys(map).length === 0) return "—";
  return Object.keys(map).join(", ");
}

export default function SleeperTradesPage() {
  const [page, setPage] = useState(1);
  const { items, total, totalPages, isLoading, error } = useSleeperTrades(page, LIMIT);

  return (
    <Layout>
      <div className="space-y-6">
        <div>
          <h1 className="text-3xl font-bold text-blue-600">Sleeper Trades</h1>
          <p className="text-gray-600 dark:text-gray-300 mt-1">
            {isLoading ? "Loading…" : `${total.toLocaleString()} completed trades`}
          </p>
        </div>

        {error && (
          <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4 text-red-700 dark:text-red-300">
            Failed to load trades: {error.message}
          </div>
        )}

        <div className="overflow-x-auto bg-white dark:bg-gray-800 rounded-lg shadow">
          <table className="w-full">
            <thead className="bg-gray-50 dark:bg-gray-700">
              <tr>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Date</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">League</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Season</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Players Added</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Players Dropped</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
              {isLoading ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                    <div className="flex justify-center items-center space-x-2">
                      <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                      <span>Loading trades…</span>
                    </div>
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                    No trades found. Data may still be syncing.
                  </td>
                </tr>
              ) : (
                items.map((trade) => (
                  <tr key={trade.id} className="hover:bg-gray-50 dark:hover:bg-gray-700">
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300 whitespace-nowrap">
                      {formatDate(trade.created_at)}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-900 dark:text-gray-100 max-w-xs truncate">
                      {trade.league_name}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{trade.season}</td>
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300 max-w-xs truncate">
                      {playerList(trade.adds)}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300 max-w-xs truncate">
                      {playerList(trade.drops)}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        {totalPages > 1 && (
          <div className="flex items-center justify-between">
            <button
              className="px-4 py-2 text-sm bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-md disabled:opacity-40 hover:bg-gray-50 dark:hover:bg-gray-600 transition-colors"
              onClick={() => setPage((p) => p - 1)}
              disabled={page <= 1 || isLoading}
            >
              Previous
            </button>
            <span className="text-sm text-gray-600 dark:text-gray-300">
              Page {page} of {totalPages}
            </span>
            <button
              className="px-4 py-2 text-sm bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-md disabled:opacity-40 hover:bg-gray-50 dark:hover:bg-gray-600 transition-colors"
              onClick={() => setPage((p) => p + 1)}
              disabled={page >= totalPages || isLoading}
            >
              Next
            </button>
          </div>
        )}
      </div>
    </Layout>
  );
}
