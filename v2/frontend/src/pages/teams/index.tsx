import { useState } from "react";
import Layout from "../../components/Layout";
import Link from "next/link";
import { useTeams } from "../../hooks/useTeams";

type SortField = "rank" | "name" | "wins" | "losses" | "pf" | "pa" | "playoffs";
type SortDirection = "asc" | "desc";

export default function Teams() {
  const { teams, isLoading, error } = useTeams();
  const [sortField, setSortField] = useState<SortField>("rank");
  const [sortDirection, setSortDirection] = useState<SortDirection>("asc");

  const handleSort = (field: SortField) => {
    if (field === sortField) {
      setSortDirection(sortDirection === "asc" ? "desc" : "asc");
    } else {
      setSortField(field);
      setSortDirection("asc");
    }
  };

  const sortedTeams =
    isLoading || !teams
      ? []
      : [...teams].sort((a, b) => {
          let fieldA: string | number;
          let fieldB: string | number;

          switch (sortField) {
            case "name":
              fieldA = a.name;
              fieldB = b.name;
              break;
            case "wins":
              fieldA = a.record.wins;
              fieldB = b.record.wins;
              break;
            case "losses":
              fieldA = a.record.losses;
              fieldB = b.record.losses;
              break;
            case "pf":
              fieldA = a.points.scored;
              fieldB = b.points.scored;
              break;
            case "pa":
              fieldA = a.points.against;
              fieldB = b.points.against;
              break;
            case "playoffs":
              fieldA = a.playoffChance;
              fieldB = b.playoffChance;
              break;
            case "rank":
            default:
              fieldA = a.rank;
              fieldB = b.rank;
          }

          if (fieldA === fieldB) return 0;

          const result = fieldA > fieldB ? 1 : -1;
          return sortDirection === "asc" ? result : -result;
        });

  const renderSortIcon = (field: SortField) => {
    if (sortField !== field) return null;

    return (
      <span className="ml-1 text-gray-400">
        {sortDirection === "asc" ? "↑" : "↓"}
      </span>
    );
  };

  if (error) {
    return (
      <Layout>
        <div className="bg-red-100 dark:bg-red-900 p-4 rounded-lg text-red-700 dark:text-red-200">
          <h2 className="text-xl font-semibold">Error loading teams</h2>
          <p>{error.message}</p>
        </div>
      </Layout>
    );
  }

  return (
    <Layout>
      <div className="space-y-8">
        <section>
          <h1 className="text-3xl md:text-4xl font-bold text-blue-600 mb-6">
            Fantasy Teams
          </h1>
          <p className="text-lg text-gray-600 dark:text-gray-300 mb-8 max-w-3xl">
            View all teams in your league, their records, and key statistics.
          </p>

          <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
            <h2 className="text-xl font-semibold mb-6">Standings</h2>

            {isLoading ? (
              <div className="flex items-center justify-center h-40">
                <div className="w-6 h-6 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                <span className="ml-2">Loading teams...</span>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
                  <thead className="bg-gray-50 dark:bg-gray-800">
                    <tr>
                      <th
                        className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider cursor-pointer"
                        onClick={() => handleSort("rank")}
                      >
                        Rank {renderSortIcon("rank")}
                      </th>
                      <th
                        className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider cursor-pointer"
                        onClick={() => handleSort("name")}
                      >
                        Team {renderSortIcon("name")}
                      </th>
                      <th
                        className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider cursor-pointer"
                        onClick={() => handleSort("wins")}
                      >
                        W {renderSortIcon("wins")}
                      </th>
                      <th
                        className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider cursor-pointer"
                        onClick={() => handleSort("losses")}
                      >
                        L {renderSortIcon("losses")}
                      </th>
                      <th
                        className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider cursor-pointer"
                        onClick={() => handleSort("pf")}
                      >
                        PF {renderSortIcon("pf")}
                      </th>
                      <th
                        className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider cursor-pointer"
                        onClick={() => handleSort("pa")}
                      >
                        PA {renderSortIcon("pa")}
                      </th>
                      <th
                        className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider cursor-pointer"
                        onClick={() => handleSort("playoffs")}
                      >
                        Playoff % {renderSortIcon("playoffs")}
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Actions
                      </th>
                    </tr>
                  </thead>
                  <tbody className="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
                    {sortedTeams.map((team, i) => (
                      <tr
                        key={team.id}
                        className={
                          i % 2 === 0
                            ? "bg-white dark:bg-gray-800"
                            : "bg-gray-50 dark:bg-gray-700"
                        }
                      >
                        <td className="py-4 px-4 whitespace-nowrap">
                          {team.rank}
                        </td>
                        <td className="py-4 px-4">
                          <div className="flex flex-col">
                            <span className="font-medium">{team.name}</span>
                            <span className="text-xs text-gray-500 dark:text-gray-400">
                              {team.owner}
                            </span>
                          </div>
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          {team.record.wins}
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          {team.record.losses}
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          {team.points.scored}
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          {team.points.against}
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          <div className="w-full bg-gray-200 dark:bg-gray-600 rounded-full h-2.5">
                            <div
                              className={`h-2.5 rounded-full ${
                                team.playoffChance > 75
                                  ? "bg-green-500"
                                  : team.playoffChance > 50
                                  ? "bg-blue-500"
                                  : team.playoffChance > 25
                                  ? "bg-yellow-500"
                                  : "bg-red-500"
                              }`}
                              style={{ width: `${team.playoffChance}%` }}
                            />
                          </div>
                          <span className="text-xs mt-1 block">
                            {team.playoffChance}%
                          </span>
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          <Link
                            href={`/teams/${team.id}`}
                            className="text-blue-600 hover:text-blue-800 dark:hover:text-blue-400"
                          >
                            View Details
                          </Link>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </section>

        <section className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
            <h2 className="text-lg font-semibold mb-4">League Leaders</h2>
            <div className="space-y-4">
              <div>
                <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">
                  Most Points Scored
                </h3>
                <div className="overflow-hidden">
                  {isLoading ? (
                    <div className="py-2 text-sm text-gray-500">Loading...</div>
                  ) : teams && teams.length > 0 ? (
                    [...teams]
                      .sort((a, b) => b.points.scored - a.points.scored)
                      .slice(0, 3)
                      .map((team) => (
                        <div
                          key={`pf-${team.id}`}
                          className="flex justify-between items-center py-2"
                        >
                          <span className="font-medium">{team.name}</span>
                          <span className="text-blue-600">
                            {team.points.scored} pts
                          </span>
                        </div>
                      ))
                  ) : (
                    <div className="py-2 text-sm text-gray-500">
                      No data available
                    </div>
                  )}
                </div>
              </div>

              <div>
                <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">
                  Most Points Against
                </h3>
                <div className="overflow-hidden">
                  {isLoading ? (
                    <div className="py-2 text-sm text-gray-500">Loading...</div>
                  ) : teams && teams.length > 0 ? (
                    [...teams]
                      .sort((a, b) => b.points.against - a.points.against)
                      .slice(0, 3)
                      .map((team) => (
                        <div
                          key={`pa-${team.id}`}
                          className="flex justify-between items-center py-2"
                        >
                          <span className="font-medium">{team.name}</span>
                          <span className="text-red-600">
                            {team.points.against} pts
                          </span>
                        </div>
                      ))
                  ) : (
                    <div className="py-2 text-sm text-gray-500">
                      No data available
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>

          <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
            <h2 className="text-lg font-semibold mb-4">League Summary</h2>
            <div className="space-y-4">
              <div>
                <span className="block text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                  Average Score
                </span>
                <span className="text-2xl font-bold">
                  {isLoading
                    ? "..."
                    : teams && teams.length > 0
                    ? (teams.reduce(
                        (acc, team) => acc + team.points.scored,
                        0
                      ) / teams.length).toFixed(1)
                    : "0.0"}{" "}
                  pts
                </span>
                <span className="text-sm text-gray-500 dark:text-gray-400 ml-2">
                  per team/week
                </span>
              </div>

              <div>
                <span className="block text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                  Highest Score
                </span>
                <span className="text-2xl font-bold">140 pts</span>
                <span className="text-sm text-gray-500 dark:text-gray-400 ml-2">
                  Team A, Week 3
                </span>
              </div>

              <div>
                <span className="block text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                  Closest Matchup
                </span>
                <span className="text-2xl font-bold">130-125</span>
                <span className="text-sm text-gray-500 dark:text-gray-400 ml-2">
                  Team D vs Team A, Week 2
                </span>
              </div>

              <div>
                <span className="block text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                  League Competitiveness
                </span>
                <span className="text-2xl font-bold">High</span>
                <div className="w-full bg-gray-200 dark:bg-gray-600 rounded-full h-2.5 mt-1">
                  <div
                    className="h-2.5 rounded-full bg-green-600"
                    style={{ width: "85%" }}
                  ></div>
                </div>
              </div>
            </div>
          </div>
        </section>
      </div>
    </Layout>
  );
}