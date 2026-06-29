import { useEffect, useState } from "react";
import { useRouter } from "next/router";
import Layout from "../../components/Layout";
import { useSleeperDrafts } from "../../hooks/useSleeperData";

const LIMIT = 25;

export default function SleeperDraftsPage() {
  const router = useRouter();
  const [page, setPage] = useState(1);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    if (!router.isReady) return;
    const p = parseInt(router.query.page as string);
    if (p > 0) setPage(p);
    setReady(true);
  }, [router.isReady, router.query.page]);

  const { items, total, totalPages, isLoading, error } = useSleeperDrafts(
    ready ? page : 1,
    LIMIT
  );

  function goToPage(p: number) {
    setPage(p);
    router.push({ pathname: router.pathname, query: { page: String(p) } }, undefined, { shallow: true });
  }

  return (
    <Layout>
      <div className="space-y-6">
        <div>
          <h1 className="text-3xl font-bold text-blue-600">Sleeper Drafts</h1>
          <p className="text-gray-600 dark:text-gray-300 mt-1">
            {isLoading ? "Loading…" : `${total.toLocaleString()} completed drafts`}
          </p>
        </div>

        {error && (
          <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4 text-red-700 dark:text-red-300">
            Failed to load drafts: {error.message}
          </div>
        )}

        <div className="overflow-x-auto bg-white dark:bg-gray-800 rounded-lg shadow">
          <table className="w-full">
            <thead className="bg-gray-50 dark:bg-gray-700">
              <tr>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Season</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">League</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Type</th>
                <th className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300">Picks</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
              {isLoading ? (
                <tr>
                  <td colSpan={4} className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                    <div className="flex justify-center items-center space-x-2">
                      <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                      <span>Loading drafts…</span>
                    </div>
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td colSpan={4} className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                    No drafts found. Data may still be syncing.
                  </td>
                </tr>
              ) : (
                items.map((draft) => (
                  <tr key={draft.id} className="hover:bg-gray-50 dark:hover:bg-gray-700">
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{draft.season}</td>
                    <td className="px-4 py-3 text-sm text-gray-900 dark:text-gray-100 max-w-xs truncate">
                      {draft.league_name}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300 capitalize">{draft.type}</td>
                    <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                      {draft.pick_count}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        {totalPages > 1 && (
          <div className="flex items-center justify-between">
            <button
              className="px-4 py-2 text-sm bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-md disabled:opacity-40 hover:bg-gray-50 dark:hover:bg-gray-600 transition-colors"
              onClick={() => goToPage(page - 1)}
              disabled={page <= 1 || isLoading}
            >
              Previous
            </button>
            <span className="text-sm text-gray-600 dark:text-gray-300">
              Page {page} of {totalPages}
            </span>
            <button
              className="px-4 py-2 text-sm bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-md disabled:opacity-40 hover:bg-gray-50 dark:hover:bg-gray-600 transition-colors"
              onClick={() => goToPage(page + 1)}
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
