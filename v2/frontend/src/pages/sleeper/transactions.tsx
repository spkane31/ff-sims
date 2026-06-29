import { useEffect, useState } from "react";
import { useRouter } from "next/router";
import Layout from "../../components/Layout";
import LeagueFilterBar from "../../components/LeagueFilterBar";
import { useSleeperTransactions } from "../../hooks/useSleeperData";
import { SleeperLeagueFilters } from "../../types/models";

const LIMIT = 25;

function formatDate(unixMs: number): string {
  if (!unixMs) return "—";
  return new Date(unixMs).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function txTypeLabel(type: string): string {
  switch (type) {
    case "trade": return "Trade";
    case "waiver": return "Waiver";
    case "free_agent": return "Free agent";
    default: return type;
  }
}

function filtersFromQuery(query: Record<string, string | string[] | undefined>): SleeperLeagueFilters {
  return {
    league_size: typeof query.league_size === "string" ? query.league_size : undefined,
    scoring_format: typeof query.scoring_format === "string" ? query.scoring_format : undefined,
    draft_type: typeof query.draft_type === "string" ? query.draft_type : undefined,
    league_type: typeof query.league_type === "string" ? query.league_type : undefined,
  };
}

export default function SleeperTransactionsPage() {
  const router = useRouter();
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState<SleeperLeagueFilters>({});
  const [txType, setTxType] = useState("");
  const [ready, setReady] = useState(false);

  useEffect(() => {
    if (!router.isReady) return;
    setFilters(filtersFromQuery(router.query));
    setTxType(typeof router.query.type === "string" ? router.query.type : "");
    const p = parseInt(router.query.page as string);
    if (p > 0) setPage(p);
    setReady(true);
  }, [router.isReady, router.query]);

  const { items, total, totalPages, isLoading, error } = useSleeperTransactions(
    ready ? page : 1,
    LIMIT,
    ready ? txType : "",
    ready ? filters : {}
  );

  function buildQuery(nextFilters: SleeperLeagueFilters, nextType: string, nextPage: number) {
    const q: Record<string, string> = { page: String(nextPage) };
    if (nextType) q.type = nextType;
    if (nextFilters.league_size) q.league_size = nextFilters.league_size;
    if (nextFilters.scoring_format) q.scoring_format = nextFilters.scoring_format;
    if (nextFilters.draft_type) q.draft_type = nextFilters.draft_type;
    if (nextFilters.league_type) q.league_type = nextFilters.league_type;
    return q;
  }

  function applyFilters(next: SleeperLeagueFilters) {
    setFilters(next);
    setPage(1);
    router.push({ pathname: router.pathname, query: buildQuery(next, txType, 1) }, undefined, { shallow: true });
  }

  function applyTxType(next: string) {
    setTxType(next);
    setPage(1);
    router.push({ pathname: router.pathname, query: buildQuery(filters, next, 1) }, undefined, { shallow: true });
  }

  function goToPage(p: number) {
    setPage(p);
    router.push({ pathname: router.pathname, query: buildQuery(filters, txType, p) }, undefined, { shallow: true });
  }

  return (
    <Layout>
      <div className="space-y-6">
        <div>
          <h1 className="text-3xl font-bold text-blue-600">Sleeper Transactions</h1>
          <p className="text-gray-600 dark:text-gray-300 mt-1">
            {isLoading ? "Loading…" : `${total.toLocaleString()} transactions`}
          </p>
        </div>

        <LeagueFilterBar
          filters={filters}
          onChange={applyFilters}
          txType={txType}
          onTxTypeChange={applyTxType}
        />

        {error && (
          <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4 text-red-700 dark:text-red-300">
            Failed to load transactions: {error.message}
          </div>
        )}

        <div className="overflow-x-auto bg-white dark:bg-gray-800 rounded-lg shadow">
          <table className="w-full">
            <thead className="bg-gray-50 dark:bg-gray-700">
              <tr>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Date</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Type</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">League</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Season</th>
                <th className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300">Players</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
              {isLoading ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                    <div className="flex justify-center items-center space-x-2">
                      <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                      <span>Loading transactions…</span>
                    </div>
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                    No transactions found.
                  </td>
                </tr>
              ) : (
                items.map((tx) => (
                  <tr key={tx.id} className="hover:bg-gray-50 dark:hover:bg-gray-700">
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300 whitespace-nowrap">
                      {formatDate(tx.created_at)}
                    </td>
                    <td className="px-4 py-3 text-sm">
                      <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${
                        tx.type === "trade"
                          ? "bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300"
                          : tx.type === "waiver"
                          ? "bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300"
                          : "bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300"
                      }`}>
                        {txTypeLabel(tx.type)}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-900 dark:text-gray-100 max-w-xs truncate">
                      {tx.league_name}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{tx.season}</td>
                    <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                      {tx.player_count}
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
