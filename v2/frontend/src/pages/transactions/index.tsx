import React, { useState } from "react";
import Link from "next/link";
import Layout from "../../components/Layout";
import { useDraftPicks } from "@/hooks/useTransactions";
import { useLeagueYears } from "@/hooks/useLeagues";

export default function Transactions() {
  const [selectedYear, setSelectedYear] = useState<number>(2024);
  const [hasInitialized, setHasInitialized] = useState<boolean>(false);
  const { draftPicks, isLoading, error } = useDraftPicks(selectedYear);
  const { years: availableYears, isLoading: yearsLoading } = useLeagueYears();

  // Set initial year to most recent year only once when data first loads
  React.useEffect(() => {
    if (availableYears && availableYears.length > 0 && !hasInitialized) {
      setSelectedYear(availableYears[0]);
      setHasInitialized(true);
    }
  }, [availableYears, hasInitialized]);


  return (
    <Layout>
      <div className="space-y-8">
        <section>
          <h1 className="text-3xl md:text-4xl font-bold text-blue-600 mb-6">
            Draft Picks
          </h1>
          <p className="text-lg text-gray-600 dark:text-gray-300 mb-8 max-w-3xl">
            View and analyze all draft picks from the fantasy football draft.
          </p>

          {/* Year Filter */}
          <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md mb-8">
            <div className="flex flex-col md:flex-row justify-between items-start md:items-center gap-4">
              <h2 className="text-xl font-semibold">Season Filter</h2>

              <div className="flex flex-wrap gap-4">
                <div>
                  <label htmlFor="yearFilter" className="block text-sm font-medium mb-1">
                    Season
                  </label>
                  <select
                    id="yearFilter"
                    value={selectedYear}
                    onChange={(e) => setSelectedYear(parseInt(e.target.value))}
                    className="w-full px-3 py-2 border dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
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
                </div>
              </div>
            </div>
          </div>

          {/* Draft Picks Table */}
          <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
            <h2 className="text-xl font-semibold mb-6">Draft Results</h2>

            {isLoading ? (
              <div className="flex items-center justify-center h-40">
                <div className="w-6 h-6 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                <span className="ml-2">Loading draft picks...</span>
              </div>
            ) : error ? (
              <div className="bg-red-100 dark:bg-red-900 p-4 rounded-lg text-red-700 dark:text-red-200">
                <h3 className="text-lg font-semibold">Error loading draft picks</h3>
                <p>{error.message}</p>
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
                    <tr className="border-b border-gray-200 dark:border-gray-600">
                      <th className="py-3 px-4 font-semibold text-gray-700 dark:text-gray-300">Round</th>
                      <th className="py-3 px-4 font-semibold text-gray-700 dark:text-gray-300">Pick #</th>
                      <th className="py-3 px-4 font-semibold text-gray-700 dark:text-gray-300">Drafting Owner</th>
                      <th className="py-3 px-4 font-semibold text-gray-700 dark:text-gray-300">Player</th>
                      <th className="py-3 px-4 font-semibold text-gray-700 dark:text-gray-300">Position</th>
                    </tr>
                  </thead>
                  <tbody>
                    {draftPicks?.map((pick, index) => (
                      <tr 
                        key={index} 
                        className="border-b border-gray-100 dark:border-gray-600 hover:bg-gray-50 dark:hover:bg-gray-600"
                      >
                        <td className="py-3 px-4 text-gray-900 dark:text-gray-100">{pick.round}</td>
                        <td className="py-3 px-4 text-gray-900 dark:text-gray-100">{pick.pick}</td>
                        <td className="py-3 px-4">
                          <Link 
                            href={`/teams/${pick.team_id}`}
                            className="text-blue-600 dark:text-blue-400 hover:text-blue-800 dark:hover:text-blue-200 font-medium underline decoration-transparent hover:decoration-current transition-colors"
                          >
                            {pick.owner}
                          </Link>
                        </td>
                        <td className="py-3 px-4">
                          <Link 
                            href={`/players/${pick.player_id}`}
                            className="text-blue-600 dark:text-blue-400 hover:text-blue-800 dark:hover:text-blue-200 font-medium underline decoration-transparent hover:decoration-current transition-colors"
                          >
                            {pick.player}
                          </Link>
                        </td>
                        <td className="py-3 px-4">
                          <span className={`px-2 py-1 text-xs rounded-full font-medium ${
                            pick.position === 'QB' ? 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200' :
                            pick.position === 'RB' ? 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200' :
                            pick.position === 'WR' ? 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200' :
                            pick.position === 'TE' ? 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200' :
                            pick.position === 'K' ? 'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200' :
                            pick.position === 'DEF' ? 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200' :
                            'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200'
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
          </div>
        </section>

      </div>
    </Layout>
  );
}

