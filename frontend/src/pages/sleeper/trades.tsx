import { useEffect, useState } from "react";
import { useRouter } from "next/router";
import Layout from "../../components/Layout";
import LeagueFilterBar from "../../components/LeagueFilterBar";
import { useSleeperTrades } from "../../hooks/useSleeperData";
import { SleeperLeagueFilters, SleeperTrade } from "../../types/models";

const LIMIT = 25;

function formatDate(unixMs: number): string {
  if (!unixMs) return "—";
  return new Date(unixMs).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    second: "2-digit",
  });
}

function sideParts(side: SleeperTrade["sides"][number] | undefined): string[] {
  if (!side) return [];
  return [
    ...(side.players ?? []).map((p) => (p.position ? `${p.name} (${p.position})` : p.name)),
    ...(side.picks ?? []),
  ];
}

function formatValue(total: number | null | undefined): string {
  if (total === null || total === undefined) return "—";
  return Math.round(total).toLocaleString();
}

// Index of the side the model thinks won (higher total value), or null when
// either side has no valuation or the totals tie.
function winningSide(trade: SleeperTrade): number | null {
  const a = trade.sides?.[0]?.total_value;
  const b = trade.sides?.[1]?.total_value;
  if (a === null || a === undefined || b === null || b === undefined || a === b) return null;
  return a > b ? 0 : 1;
}

function filtersFromQuery(query: Record<string, string | string[] | undefined>): SleeperLeagueFilters {
  return {
    league_size: typeof query.league_size === "string" ? query.league_size : undefined,
    scoring_format: typeof query.scoring_format === "string" ? query.scoring_format : undefined,
    draft_type: typeof query.draft_type === "string" ? query.draft_type : undefined,
    league_type: typeof query.league_type === "string" ? query.league_type : undefined,
    exclude_picks: typeof query.exclude_picks === "string" ? query.exclude_picks : undefined,
  };
}

export default function SleeperTradesPage() {
  const router = useRouter();
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState<SleeperLeagueFilters>({});
  const [ready, setReady] = useState(false);

  useEffect(() => {
    if (!router.isReady) return;
    setFilters(filtersFromQuery(router.query));
    const p = parseInt(router.query.page as string);
    if (p > 0) setPage(p);
    setReady(true);
  }, [router.isReady, router.query]);

  const { items, total, totalPages, isLoading, error } = useSleeperTrades(
    ready ? page : 1,
    LIMIT,
    ready ? filters : {}
  );

  function applyFilters(next: SleeperLeagueFilters) {
    setFilters(next);
    setPage(1);
    const q: Record<string, string> = { page: "1" };
    if (next.league_size) q.league_size = next.league_size;
    if (next.scoring_format) q.scoring_format = next.scoring_format;
    if (next.draft_type) q.draft_type = next.draft_type;
    if (next.league_type) q.league_type = next.league_type;
    if (next.exclude_picks) q.exclude_picks = next.exclude_picks;
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
          <h1 className="text-3xl font-bold text-blue-600">Sleeper Trades</h1>
          <p className="text-gray-600 dark:text-gray-300 mt-1">
            {isLoading ? "Loading…" : `${total.toLocaleString()} completed trades`}
          </p>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
            Values are the model&apos;s player valuations as of the trade date, using the valuation
            segment matching the trade&apos;s league format (full-PPR superflex redraft, 8/10/12-team);
            trades from other formats show &quot;—&quot;. The highlighted side is the one the model
            favored. Draft picks are not valued.
          </p>
        </div>

        <LeagueFilterBar filters={filters} onChange={applyFilters} showPicksFilter />

        {error && (
          <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4 text-red-700 dark:text-red-300">
            Failed to load trades: {error.message}
          </div>
        )}

        <div className="overflow-x-auto bg-white dark:bg-gray-800 rounded-lg shadow">
          <table className="w-full">
            <thead className="bg-gray-50 dark:bg-gray-700">
              <tr>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Date &amp; Time</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">League</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Season</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Side A</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-gray-700 dark:text-gray-300">Value A</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Side B</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-gray-700 dark:text-gray-300">Value B</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
              {isLoading ? (
                <tr>
                  <td colSpan={7} className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                    <div className="flex justify-center items-center space-x-2">
                      <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                      <span>Loading trades…</span>
                    </div>
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                    No trades found.
                  </td>
                </tr>
              ) : (
                items.map((trade) => {
                  const winner = winningSide(trade);
                  const winClass = "bg-green-50 dark:bg-green-900/20";
                  return (
                    <tr key={trade.id} className="hover:bg-gray-50 dark:hover:bg-gray-700">
                      <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300 whitespace-nowrap">
                        {formatDate(trade.created_at)}
                      </td>
                      <td className="px-4 py-3 text-sm text-gray-900 dark:text-gray-100 max-w-xs truncate">
                        {trade.league_name}
                      </td>
                      <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{trade.season}</td>
                      <td
                        className={`px-4 py-3 text-sm text-gray-600 dark:text-gray-300 align-top max-w-xs ${winner === 0 ? winClass : ""}`}
                      >
                        {sideParts(trade.sides?.[0]).length > 0 ? (
                          <ul className="space-y-0.5">
                            {sideParts(trade.sides?.[0]).map((part, i) => (
                              <li key={i}>{part}</li>
                            ))}
                          </ul>
                        ) : (
                          "—"
                        )}
                      </td>
                      <td
                        className={`px-4 py-3 text-sm text-right align-top whitespace-nowrap ${
                          winner === 0
                            ? `${winClass} font-semibold text-green-700 dark:text-green-400`
                            : "text-gray-600 dark:text-gray-300"
                        }`}
                      >
                        {formatValue(trade.sides?.[0]?.total_value)}
                      </td>
                      <td
                        className={`px-4 py-3 text-sm text-gray-600 dark:text-gray-300 align-top max-w-xs ${winner === 1 ? winClass : ""}`}
                      >
                        {sideParts(trade.sides?.[1]).length > 0 ? (
                          <ul className="space-y-0.5">
                            {sideParts(trade.sides?.[1]).map((part, i) => (
                              <li key={i}>{part}</li>
                            ))}
                          </ul>
                        ) : (
                          "—"
                        )}
                      </td>
                      <td
                        className={`px-4 py-3 text-sm text-right align-top whitespace-nowrap ${
                          winner === 1
                            ? `${winClass} font-semibold text-green-700 dark:text-green-400`
                            : "text-gray-600 dark:text-gray-300"
                        }`}
                      >
                        {formatValue(trade.sides?.[1]?.total_value)}
                      </td>
                    </tr>
                  );
                })
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
