import { useState } from "react";
import Layout from "../../components/Layout";
import { useSchedule } from "../../hooks/useSchedule";
import { Matchup } from "@/types/models";
import Link from "next/link";

interface TeamStrength {
  team: string;
  difficulty: "Easy" | "Med" | "Hard";
  strengthPercentage: number;
}

export default function Schedule() {
  const [selectedWeek, setSelectedWeek] = useState<string>("all");
  const [selectedYear, setSelectedYear] = useState<string>("all");
  const [selectedGameType, setSelectedGameType] = useState<string>("all");
  const { schedule, isLoading, error } = useSchedule({ gameType: selectedGameType });

  // Server-side filtering now handles playoff detection

  // Transform API data - server now handles filtering
  const scheduleData: Matchup[] =
    !isLoading && schedule
      ? schedule.data.matchups
          .flat()
          .map((game) => ({
            leagueId: 1, // TODO: this might not be necessary
            id: game.id,
            createdAt: "2023-10-01T00:00:00Z",
            updatedAt: "2023-10-01T00:00:00Z",
            season: game.year,
            year: game.year,
            week: game.week,
            homeTeamId: game.homeTeamId || 0,
            awayTeamId: game.awayTeamId || 0,
            homeTeamESPNID: game.homeTeamESPNID || 0,
            awayTeamESPNID: game.awayTeamESPNID || 0,
            homeTeamName: game.homeTeamName,
            awayTeamName: game.awayTeamName,
            homeScore: game.homeScore,
            awayScore: game.awayScore,
            homeProjectedScore: game.homeProjectedScore,
            awayProjectedScore: game.awayProjectedScore,
            completed: game.homeScore > 0 || game.awayScore > 0,
            homeTeam: game.homeTeam,
            awayTeam: game.awayTeam,
            gameType: game.gameType,
            playoffGameType: game.playoffGameType,
            isPlayoff: game.isPlayoff || false,
          }))
      : [];

  const weeks: number[] = Array.from(
    new Set(scheduleData.map((game) => game.week))
  ).sort((a, b) => a - b);

  const years: number[] = Array.from(
    new Set(scheduleData.map((game) => game.year))
  ).sort((a, b) => b - a);

  // Calculate playoff start week for each year based on first WINNERS_BRACKET game
  const playoffStartWeeks: Record<number, number> = {};

  years.forEach((year) => {
    const yearGames = scheduleData.filter((game) => game.year === year);
    const playoffGames = yearGames.filter(
      (game) => game.gameType === "WINNERS_BRACKET"
    );
    if (playoffGames.length > 0) {
      playoffStartWeeks[year] = Math.min(
        ...playoffGames.map((game) => game.week)
      );
    }
  });

  const filteredGames: Matchup[] = scheduleData.filter((game) => {
    // Apply year filter
    const yearMatch =
      selectedYear === "all" || game.year.toString() === selectedYear;

    // Apply week filter
    const weekMatch =
      selectedWeek === "all" || game.week.toString() === selectedWeek;

    // Game type filtering is now handled server-side via the useSchedule hook
    return yearMatch && weekMatch;
  });

  // Strength of schedule data
  // This would ideally be calculated based on actual team data
  const remainingStrength: TeamStrength[] = [
    { team: "Team A", difficulty: "Hard", strengthPercentage: 80 },
    { team: "Team B", difficulty: "Easy", strengthPercentage: 65 },
    { team: "Team C", difficulty: "Hard", strengthPercentage: 50 },
    { team: "Team D", difficulty: "Easy", strengthPercentage: 35 },
  ];

  const overallStrength: TeamStrength[] = [
    { team: "Team A", difficulty: "Hard", strengthPercentage: 70 },
    { team: "Team B", difficulty: "Med", strengthPercentage: 60 },
    { team: "Team C", difficulty: "Easy", strengthPercentage: 50 },
    { team: "Team D", difficulty: "Hard", strengthPercentage: 40 },
  ];

  // Helper function to get color based on difficulty
  const getDifficultyColor = (
    difficulty: TeamStrength["difficulty"]
  ): string => {
    switch (difficulty) {
      case "Hard":
        return "bg-red-500";
      case "Med":
        return "bg-yellow-500";
      case "Easy":
        return "bg-green-500";
      default:
        return "bg-gray-500";
    }
  };

  return (
    <Layout>
      <div className="space-y-8">
        <section>
          <h1 className="text-3xl md:text-4xl font-bold text-blue-600 mb-6">
            League Schedule
          </h1>
          <p className="text-lg text-gray-600 dark:text-gray-300 mb-8 max-w-3xl">
            View upcoming matchups and past results for all teams in your
            league.
          </p>

          <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
            <div className="flex flex-col md:flex-row justify-between items-start md:items-center mb-6">
              <h2 className="text-xl font-semibold mb-3 md:mb-0">Matchups</h2>

              <div className="w-full md:w-auto space-y-4 md:space-y-0 md:space-x-4">
                <label
                  htmlFor="yearFilter"
                  className="block text-sm font-medium mb-1 md:hidden"
                >
                  Select Year
                </label>
                <select
                  id="yearFilter"
                  value={selectedYear}
                  onChange={(e) => setSelectedYear(e.target.value)}
                  className="w-full md:w-auto px-3 py-2 border dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                  disabled={isLoading}
                >
                  <option value="all">All Years</option>
                  {years.map((year) => (
                    <option key={year} value={year}>
                      {year}
                    </option>
                  ))}
                </select>
                <label
                  htmlFor="weekFilter"
                  className="block text-sm font-medium mb-1 md:hidden"
                >
                  Select Week
                </label>
                <select
                  id="weekFilter"
                  value={selectedWeek}
                  onChange={(e) => setSelectedWeek(e.target.value)}
                  className="w-full md:w-auto px-3 py-2 border dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                  disabled={isLoading}
                >
                  <option value="all">All Weeks</option>
                  {weeks.map((week) => (
                    <option key={week} value={week}>
                      Week {week}
                    </option>
                  ))}
                </select>
                <label
                  htmlFor="gameTypeFilter"
                  className="block text-sm font-medium mb-1 md:hidden"
                >
                  Select Game Type
                </label>
                <select
                  id="gameTypeFilter"
                  value={selectedGameType}
                  onChange={(e) => setSelectedGameType(e.target.value)}
                  className="w-full md:w-auto px-3 py-2 border dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                  disabled={isLoading}
                >
                  <option value="all">All Games</option>
                  <option value="regular">Regular Season</option>
                  <option value="playoffs">Playoffs</option>
                </select>
              </div>
            </div>

            {isLoading ? (
              <div className="flex items-center justify-center h-40">
                <div className="w-6 h-6 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                <span className="ml-2">Loading schedule data...</span>
              </div>
            ) : error ? (
              <div className="bg-red-100 dark:bg-red-900 p-4 rounded-lg text-red-700 dark:text-red-200">
                <h3 className="text-lg font-semibold">
                  Error loading schedule
                </h3>
                <p>{error.message}</p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
                  <thead className="bg-gray-50 dark:bg-gray-800">
                    <tr>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Year
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Week
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Matchup
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Score
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Projected Score
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Status
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Details
                      </th>
                    </tr>
                  </thead>
                  <tbody className="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
                    {filteredGames.map((game, i) => (
                      <tr
                        key={i}
                        className={
                          i % 2 === 0
                            ? "bg-white dark:bg-gray-800"
                            : "bg-gray-50 dark:bg-gray-700"
                        }
                      >
                        <td className="py-4 px-4 whitespace-nowrap">
                          {game.year}
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          {game.playoffGameType === "CHAMPIONSHIP"
                            ? "Championship"
                            : game.playoffGameType === "THIRD_PLACE"
                            ? "Third Place Game"
                            : game.playoffGameType === "PLAYOFF"
                            ? `Playoffs (Round ${
                                game.week - (playoffStartWeeks[game.year] - 1)
                              })`
                            : `Week ${game.week}`}
                        </td>
                        <td className="py-4 px-4">
                          <div className="flex flex-col md:flex-row md:items-center">
                            <Link
                              href={`/teams/${game.homeTeamESPNID}`}
                              className={`font-medium hover:text-blue-600 dark:hover:text-blue-400 transition-colors duration-200 ${
                                game.completed &&
                                game.homeScore > game.awayScore
                                  ? "text-green-600"
                                  : ""
                              }`}
                            >
                              {game.homeTeamName}
                            </Link>
                            <span className="hidden md:inline mx-2">vs</span>
                            <span className="md:hidden">@</span>

                            <Link
                              href={`/teams/${game.awayTeamESPNID}`}
                              className={`font-medium hover:text-blue-600 dark:hover:text-blue-400 transition-colors duration-200 ${
                                game.completed &&
                                game.awayScore > game.homeScore
                                  ? "text-green-600"
                                  : ""
                              }`}
                            >
                              {game.awayTeamName}
                            </Link>
                          </div>
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          {game.completed ? (
                            <span>
                              {game.homeScore.toFixed(2)} -{" "}
                              {game.awayScore.toFixed(2)}
                            </span>
                          ) : (
                            <span className="text-gray-500 dark:text-gray-400">
                              Upcoming
                            </span>
                          )}
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          {game.completed ? (
                            <span>
                              {game.homeProjectedScore === -1
                                ? "NA"
                                : game.homeProjectedScore.toFixed(2)}{" "}
                              -{" "}
                              {game.awayProjectedScore === -1
                                ? "NA"
                                : game.awayProjectedScore.toFixed(2)}
                            </span>
                          ) : (
                            <span className="text-gray-500 dark:text-gray-400">
                              Upcoming
                            </span>
                          )}
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          {game.completed ? (
                            <span className="px-2 py-1 text-xs rounded-full bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-100">
                              Final
                            </span>
                          ) : (
                            <span className="px-2 py-1 text-xs rounded-full bg-blue-100 text-blue-800 dark:bg-blue-800 dark:text-blue-100">
                              Upcoming
                            </span>
                          )}
                        </td>

                        <td className="py-4 px-4 whitespace-nowrap">
                          <Link
                            href={`/schedule/${game.id}`}
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

        <section className="bg-gray-100 dark:bg-gray-700 p-6 rounded-lg">
          <h2 className="text-xl font-semibold mb-4">Strength of Schedule</h2>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <div>
              <h3 className="text-lg font-medium mb-3">Remaining</h3>
              <div className="space-y-3">
                {remainingStrength.map(
                  ({ team, difficulty, strengthPercentage }) => (
                    <div key={team} className="flex items-center">
                      <span className="w-20 text-sm">{team}</span>
                      <div className="flex-1 h-5 bg-gray-200 dark:bg-gray-600 rounded-full overflow-hidden">
                        <div
                          className={`h-full ${getDifficultyColor(difficulty)}`}
                          style={{ width: `${strengthPercentage}%` }}
                        ></div>
                      </div>
                      <span className="w-14 text-right text-sm">
                        {difficulty}
                      </span>
                    </div>
                  )
                )}
              </div>
            </div>

            <div>
              <h3 className="text-lg font-medium mb-3">Season Overall</h3>
              <div className="space-y-3">
                {overallStrength.map(
                  ({ team, difficulty, strengthPercentage }) => (
                    <div key={team} className="flex items-center">
                      <span className="w-20 text-sm">{team}</span>
                      <div className="flex-1 h-5 bg-gray-200 dark:bg-gray-600 rounded-full overflow-hidden">
                        <div
                          className={`h-full ${getDifficultyColor(difficulty)}`}
                          style={{ width: `${strengthPercentage}%` }}
                        ></div>
                      </div>
                      <span className="w-14 text-right text-sm">
                        {difficulty}
                      </span>
                    </div>
                  )
                )}
              </div>
            </div>
          </div>
        </section>
      </div>
    </Layout>
  );
}
