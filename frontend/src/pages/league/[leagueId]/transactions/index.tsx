import React, { useEffect, useState } from "react";
import { useRouter } from "next/router";
import Link from "next/link";
import Layout from "@/components/Layout";
import { useDraftPicks, useTransactions } from "@/hooks/useTransactions";
import { useLeagueYears } from "@/hooks/useLeagues";

const LIMIT = 25;

type Tab = "transactions" | "draft-picks";

function txTypeLabel(type: string): string {
  switch (type) {
    case "trade": return "Trade";
    case "waiver": return "Waiver";
    case "draft": return "Draft";
    default: return type;
  }
}

export default function Transactions() {
  const router = useRouter();
  const leagueId = Number(router.query.leagueId);
  const [tab, setTab] = useState<Tab>("transactions");
  const [selectedYear, setSelectedYear] = useState<number>(2024);
  const [page, setPage] = useState(1);
  const [hasInitialized, setHasInitialized] = useState(false);

  const { years: availableYears, isLoading: yearsLoading } = useLeagueYears(leagueId);

  const {
    transactions,
    total: txTotal,
    totalPages: txTotalPages,
    isLoading: txLoading,
    error: txError,
  } = useTransactions(leagueId, page, LIMIT, tab === "transactions" ? selectedYear : undefined);

  const {
    draftPicks,
    total: dpTotal,
    totalPages: dpTotalPages,
    isLoading: dpLoading,
    error: dpError,
  } = useDraftPicks(leagueId, selectedYear, page, LIMIT);

  useEffect(() => {
    if (!router.isReady) return;
    if (availableYears && availableYears.length > 0 && !hasInitialized) {
      const urlYear = parseInt(router.query.year as string);
      setSelectedYear(urlYear > 0 ? urlYear : availableYears[0]);
      const urlPage = parseInt(router.query.page as string);
      if (urlPage > 0) setPage(urlPage);
      const urlTab = router.query.tab as string;
      if (urlTab === "draft-picks" || urlTab === "transactions") setTab(urlTab);
      setHasInitialized(true);
    }
  }, [router.isReady, router.query, availableYears, hasInitialized]);

  function changeTab(next: Tab) {
    setTab(next);
    setPage(1);
    router.push(
      { pathname: router.pathname, query: { ...router.query, tab: next, page: "1" } },
      undefined,
      { shallow: true }
    );
  }

  function changeYear(year: number) {
    setSelectedYear(year);
    setPage(1);
    router.push(
      { pathname: router.pathname, query: { ...router.query, year: String(year), page: "1" } },
      undefined,
      { shallow: true }
    );
  }

  function goToPage(p: number) {
    setPage(p);
    router.push(
      { pathname: router.pathname, query: { ...router.query, page: String(p) } },
      undefined,
      { shallow: true }
    );
  }

  const isLoading = tab === "transactions" ? txLoading : dpLoading;
  const total = tab === "transactions" ? txTotal : dpTotal;
  const error = tab === "transactions" ? txError : dpError;

  return (
    <Layout>
      <div className="space-y-8">
        <section>
          <h1 className="text-3xl md:text-4xl font-bold text-blue-600 mb-6">
            Transactions
          </h1>

          {/* Tab switcher */}
          <div className="flex border-b border-gray-200 dark:border-gray-600 mb-6">
            <button
              onClick={() => changeTab("transactions")}
              className={`px-6 py-3 text-sm font-medium border-b-2 transition-colors ${
                tab === "transactions"
                  ? "border-blue-600 text-blue-600 dark:text-blue-400"
                  : "border-transparent text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200"
              }`}
            >
              Transactions
            </button>
            <button
              onClick={() => changeTab("draft-picks")}
              className={`px-6 py-3 text-sm font-medium border-b-2 transition-colors ${
                tab === "draft-picks"
                  ? "border-blue-600 text-blue-600 dark:text-blue-400"
                  : "border-transparent text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200"
              }`}
            >
              Draft Picks
            </button>
          </div>

          {/* Year filter */}
          <div className="bg-white dark:bg-gray-700 p-4 rounded-lg shadow-md mb-6">
            <div className="flex items-center gap-4">
              <label htmlFor="yearFilter" className="text-sm font-medium">
                Season
              </label>
              <select
                id="yearFilter"
                value={selectedYear}
                onChange={(e) => changeYear(parseInt(e.target.value))}
                className="px-3 py-2 border dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                disabled={yearsLoading}
              >
                {yearsLoading ? (
                  <option>Loading years...</option>
                ) : (
                  availableYears.map((year) => (
                    <option key={year} value={year}>
                      {year}
                    </option>
                  ))
                )}
              </select>
              {!isLoading && total > 0 && (
                <span className="text-sm text-gray-500 dark:text-gray-400">
                  {total.toLocaleString()} {tab === "transactions" ? "transactions" : "picks"}
                </span>
              )}
            </div>
          </div>

          {error && (
            <div className="bg-red-100 dark:bg-red-900 p-4 rounded-lg text-red-700 dark:text-red-200 mb-6">
              <h3 className="text-lg font-semibold">Error loading data</h3>
              <p>{error.message}</p>
            </div>
          )}

          {/* Transactions tab */}
          {tab === "transactions" && (
            <div className="bg-white dark:bg-gray-700 rounded-lg shadow-md overflow-hidden">
              {txLoading ? (
                <div className="flex items-center justify-center h-40">
                  <div className="w-6 h-6 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                  <span className="ml-2">Loading transactions...</span>
                </div>
              ) : !transactions || transactions.length === 0 ? (
                <div className="text-center py-10 text-gray-500 dark:text-gray-400">
                  <p>No transactions found for {selectedYear}</p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-left border-collapse">
                    <thead>
                      <tr className="border-b border-gray-200 dark:border-gray-600 bg-gray-50 dark:bg-gray-800">
                        <th className="py-3 px-4 font-semibold text-gray-700 dark:text-gray-300">Date</th>
                        <th className="py-3 px-4 font-semibold text-gray-700 dark:text-gray-300">Type</th>
                        <th className="py-3 px-4 font-semibold text-gray-700 dark:text-gray-300">Team</th>
                        <th className="py-3 px-4 font-semibold text-gray-700 dark:text-gray-300">Players</th>
                      </tr>
                    </thead>
                    <tbody>
                      {transactions.map((tx) => (
                        <tr
                          key={tx.id}
                          className="border-b border-gray-100 dark:border-gray-600 hover:bg-gray-50 dark:hover:bg-gray-600"
                        >
                          <td className="py-3 px-4 text-sm text-gray-600 dark:text-gray-300 whitespace-nowrap">
                            {tx.date}
                          </td>
                          <td className="py-3 px-4">
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
                          <td className="py-3 px-4 text-sm text-gray-900 dark:text-gray-100">
                            {tx.teams?.[0] ?? "—"}
                          </td>
                          <td className="py-3 px-4 text-sm text-gray-600 dark:text-gray-300">
                            {tx.players?.map((p) => p.name).join(", ") || "—"}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
              {txTotalPages > 1 && (
                <div className="flex items-center justify-between p-4 border-t border-gray-200 dark:border-gray-600">
                  <button
                    className="px-4 py-2 text-sm bg-white dark:bg-gray-600 border border-gray-300 dark:border-gray-500 rounded-md disabled:opacity-40 hover:bg-gray-50 dark:hover:bg-gray-500 transition-colors"
                    onClick={() => goToPage(page - 1)}
                    disabled={page <= 1 || txLoading}
                  >
                    Previous
                  </button>
                  <span className="text-sm text-gray-600 dark:text-gray-300">
                    Page {page} of {txTotalPages}
                  </span>
                  <button
                    className="px-4 py-2 text-sm bg-white dark:bg-gray-600 border border-gray-300 dark:border-gray-500 rounded-md disabled:opacity-40 hover:bg-gray-50 dark:hover:bg-gray-500 transition-colors"
                    onClick={() => goToPage(page + 1)}
                    disabled={page >= txTotalPages || txLoading}
                  >
                    Next
                  </button>
                </div>
              )}
            </div>
          )}

          {/* Draft Picks tab */}
          {tab === "draft-picks" && (
            <div className="bg-white dark:bg-gray-700 rounded-lg shadow-md overflow-hidden">
              {dpLoading ? (
                <div className="flex items-center justify-center h-40">
                  <div className="w-6 h-6 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                  <span className="ml-2">Loading draft picks...</span>
                </div>
              ) : !draftPicks || draftPicks.length === 0 ? (
                <div className="text-center py-10 text-gray-500 dark:text-gray-400">
                  <p className="mb-2">No draft picks found</p>
                  <p className="text-sm">Try changing the year to see draft results</p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-left border-collapse">
                    <thead>
                      <tr className="border-b border-gray-200 dark:border-gray-600 bg-gray-50 dark:bg-gray-800">
                        <th className="py-3 px-4 font-semibold text-gray-700 dark:text-gray-300">Round</th>
                        <th className="py-3 px-4 font-semibold text-gray-700 dark:text-gray-300">Pick #</th>
                        <th className="py-3 px-4 font-semibold text-gray-700 dark:text-gray-300">Drafting Owner</th>
                        <th className="py-3 px-4 font-semibold text-gray-700 dark:text-gray-300">Player</th>
                        <th className="py-3 px-4 font-semibold text-gray-700 dark:text-gray-300">Position</th>
                      </tr>
                    </thead>
                    <tbody>
                      {draftPicks.map((pick, index) => (
                        <tr
                          key={index}
                          className="border-b border-gray-100 dark:border-gray-600 hover:bg-gray-50 dark:hover:bg-gray-600"
                        >
                          <td className="py-3 px-4 text-gray-900 dark:text-gray-100">{pick.round}</td>
                          <td className="py-3 px-4 text-gray-900 dark:text-gray-100">{pick.pick}</td>
                          <td className="py-3 px-4">
                            <Link
                              href={`/league/${leagueId}/teams/${pick.team_id}`}
                              className="text-blue-600 dark:text-blue-400 hover:text-blue-800 dark:hover:text-blue-200 font-medium underline decoration-transparent hover:decoration-current transition-colors"
                            >
                              {pick.owner}
                            </Link>
                          </td>
                          <td className="py-3 px-4">
                            <Link
                              href={`/league/${leagueId}/players/${pick.player_id}`}
                              className="text-blue-600 dark:text-blue-400 hover:text-blue-800 dark:hover:text-blue-200 font-medium underline decoration-transparent hover:decoration-current transition-colors"
                            >
                              {pick.player}
                            </Link>
                          </td>
                          <td className="py-3 px-4">
                            <span className={`px-2 py-1 text-xs rounded-full font-medium ${
                              pick.position === "QB" ? "bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200" :
                              pick.position === "RB" ? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200" :
                              pick.position === "WR" ? "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200" :
                              pick.position === "TE" ? "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200" :
                              pick.position === "K" ? "bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200" :
                              pick.position === "DEF" ? "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200" :
                              "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200"
                            }`}>
                              {pick.position}
                            </span>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
              {dpTotalPages > 1 && (
                <div className="flex items-center justify-between p-4 border-t border-gray-200 dark:border-gray-600">
                  <button
                    className="px-4 py-2 text-sm bg-white dark:bg-gray-600 border border-gray-300 dark:border-gray-500 rounded-md disabled:opacity-40 hover:bg-gray-50 dark:hover:bg-gray-500 transition-colors"
                    onClick={() => goToPage(page - 1)}
                    disabled={page <= 1 || dpLoading}
                  >
                    Previous
                  </button>
                  <span className="text-sm text-gray-600 dark:text-gray-300">
                    Page {page} of {dpTotalPages}
                  </span>
                  <button
                    className="px-4 py-2 text-sm bg-white dark:bg-gray-600 border border-gray-300 dark:border-gray-500 rounded-md disabled:opacity-40 hover:bg-gray-50 dark:hover:bg-gray-500 transition-colors"
                    onClick={() => goToPage(page + 1)}
                    disabled={page >= dpTotalPages || dpLoading}
                  >
                    Next
                  </button>
                </div>
              )}
            </div>
          )}
        </section>
      </div>
    </Layout>
  );
}
