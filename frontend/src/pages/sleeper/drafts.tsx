import { useEffect, useState } from "react";
import { useRouter } from "next/router";
import Layout from "../../components/Layout";
import ADPFilterBar from "../../components/ADPFilterBar";
import { useSleeperADP } from "../../hooks/useSleeperData";
import { SleeperADPFilters } from "../../types/models";

const LIMIT = 25;

function filtersFromQuery(query: Record<string, string | string[] | undefined>): SleeperADPFilters {
  return {
    league_size: typeof query.league_size === "string" ? query.league_size : undefined,
    scoring_format: typeof query.scoring_format === "string" ? query.scoring_format : undefined,
    superflex: typeof query.superflex === "string" ? query.superflex : undefined,
    season: typeof query.season === "string" ? query.season : undefined,
  };
}

export default function SleeperADPPage() {
  const router = useRouter();
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState<SleeperADPFilters>({});
  const [ready, setReady] = useState(false);

  useEffect(() => {
    if (!router.isReady) return;
    setFilters(filtersFromQuery(router.query));
    const p = parseInt(router.query.page as string);
    if (p > 0) setPage(p);
    setReady(true);
  }, [router.isReady, router.query]);

  const { items, season, availableSeasons, total, totalPages, isLoading, error } = useSleeperADP(
    ready ? page : 1,
    LIMIT,
    ready ? filters : {}
  );

  function applyFilters(next: SleeperADPFilters) {
    setFilters(next);
    setPage(1);
    const q: Record<string, string> = { page: "1" };
    if (next.league_size) q.league_size = next.league_size;
    if (next.scoring_format) q.scoring_format = next.scoring_format;
    if (next.superflex) q.superflex = next.superflex;
    if (next.season) q.season = next.season;
    router.push({ pathname: router.pathname, query: q }, undefined, { shallow: true });
  }

  function goToPage(p: number) {
    setPage(p);
    const q: Record<string, string> = { ...router.query as Record<string, string>, page: String(p) };
    router.push({ pathname: router.pathname, query: q }, undefined, { shallow: true });
  }

  return (
    <Layout>
      <div className="space-y-6">
        <div>
          <h1 className="text-3xl font-bold text-blue-600">Average Draft Position</h1>
          <p className="text-gray-600 dark:text-gray-300 mt-1">
            {isLoading ? "Loading…" : `${total.toLocaleString()} players${season ? ` — ${season} season` : ""}`}
          </p>
        </div>

        <ADPFilterBar filters={filters} onChange={applyFilters} availableSeasons={availableSeasons} />

        {error && (
          <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4 text-red-700 dark:text-red-300">
            Failed to load ADP: {error.message}
          </div>
        )}

        <div className="overflow-x-auto bg-white dark:bg-gray-800 rounded-lg shadow">
          <table className="w-full">
            <thead className="bg-gray-50 dark:bg-gray-700">
              <tr>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Rank</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Player</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Pos</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Team</th>
                <th className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300">Avg Pick</th>
                <th className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300">Range</th>
                <th className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300">95% CI</th>
                <th className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300">Drafts</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
              {isLoading ? (
                <tr>
                  <td colSpan={8} className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                    <div className="flex justify-center items-center space-x-2">
                      <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                      <span>Loading ADP…</span>
                    </div>
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td colSpan={8} className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                    No players found for this filter combination.
                  </td>
                </tr>
              ) : (
                items.map((player, i) => (
                  <tr key={player.sleeper_player_id} className="hover:bg-gray-50 dark:hover:bg-gray-700">
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">
                      {(page - 1) * LIMIT + i + 1}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-900 dark:text-gray-100 max-w-xs truncate">
                      {player.name}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{player.position}</td>
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{player.nfl_team}</td>
                    <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                      {player.avg_pick_no.toFixed(1)}
                    </td>
                    <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                      {player.min_pick_no}–{player.max_pick_no}
                    </td>
                    <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                      {player.ci_low_pick_no.toFixed(1)}–{player.ci_high_pick_no.toFixed(1)}
                    </td>
                    <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                      {player.pick_count}
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
