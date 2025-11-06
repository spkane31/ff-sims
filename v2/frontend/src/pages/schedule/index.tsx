import { useState, useMemo } from "react";
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
  const [showFutureMatchups, setShowFutureMatchups] = useState<boolean>(false);
  const { schedule, isLoading, error } = useSchedule({
    gameType: selectedGameType,
  });

  // Server-side filtering now handles playoff detection

  // Transform API data - server now handles filtering
  const scheduleData: Matchup[] =
    !isLoading && schedule
      ? schedule.data.matchups.flat().map((game) => ({
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

    // Apply future matchup filter
    // If showFutureMatchups is OFF (default): only show completed games
    // If showFutureMatchups is ON: show all games (completed and future)
    const futureMatch = showFutureMatchups ? true : game.completed;

    // Game type filtering is handled server-side via the useSchedule hook
    return yearMatch && weekMatch && futureMatch;
  });

  // Calculate strength of schedule client-side from schedule data
  const { overallStrength, remainingStrength } = useMemo(() => {
    if (!scheduleData || scheduleData.length === 0) {
      return { overallStrength: [], remainingStrength: [] };
    }

    // Determine the target year for calculations
    const targetYear = selectedYear !== "all" ? parseInt(selectedYear) : Math.max(...scheduleData.map(g => g.year));

    // Filter to regular season games only for the target year
    const regularSeasonGames = scheduleData.filter(
      g => g.year === targetYear && !g.isPlayoff && g.gameType === "NONE"
    );

    if (regularSeasonGames.length === 0) {
      return { overallStrength: [], remainingStrength: [] };
    }

    // Get all unique teams
    const teams = new Set<string>();
    regularSeasonGames.forEach(game => {
      teams.add(game.homeTeamName);
      teams.add(game.awayTeamName);
    });

    // Calculate win percentage for each team from completed games
    const teamWinPct: Record<string, { wins: number; losses: number; pct: number }> = {};
    teams.forEach(team => {
      const completedGames = regularSeasonGames.filter(
        g => g.completed && (g.homeTeamName === team || g.awayTeamName === team)
      );

      let wins = 0;
      let losses = 0;

      completedGames.forEach(game => {
        if (game.homeTeamName === team) {
          if (game.homeScore > game.awayScore) wins++;
          else if (game.homeScore < game.awayScore) losses++;
        } else {
          if (game.awayScore > game.homeScore) wins++;
          else if (game.awayScore < game.homeScore) losses++;
        }
      });

      const total = wins + losses;
      teamWinPct[team] = {
        wins,
        losses,
        pct: total > 0 ? wins / total : 0.5 // Default to 0.5 if no games played
      };
    });

    // Calculate SOS for each team - average opponent win percentage
    const calculateSOS = (games: Matchup[]) => {
      const teamSOS: Record<string, { totalOppPct: number; oppCount: number }> = {};

      games.forEach(game => {
        const homeTeam = game.homeTeamName;
        const awayTeam = game.awayTeamName;

        // Initialize
        if (!teamSOS[homeTeam]) teamSOS[homeTeam] = { totalOppPct: 0, oppCount: 0 };
        if (!teamSOS[awayTeam]) teamSOS[awayTeam] = { totalOppPct: 0, oppCount: 0 };

        // Add opponent's win percentage
        teamSOS[homeTeam].totalOppPct += teamWinPct[awayTeam]?.pct ?? 0.5;
        teamSOS[homeTeam].oppCount++;

        teamSOS[awayTeam].totalOppPct += teamWinPct[homeTeam]?.pct ?? 0.5;
        teamSOS[awayTeam].oppCount++;
      });

      return teamSOS;
    };

    // Calculate overall SOS (all games)
    const overallSOS = calculateSOS(regularSeasonGames);

    // Calculate remaining SOS (future games only)
    const futureGames = regularSeasonGames.filter(g => !g.completed);
    const remainingSOS = calculateSOS(futureGames);

    // Convert to display format
    const convertToStrength = (sosData: Record<string, { totalOppPct: number; oppCount: number }>): TeamStrength[] => {
      return Object.entries(sosData)
        .map(([team, data]) => {
          const avgOppPct = data.oppCount > 0 ? data.totalOppPct / data.oppCount : 0.5;
          const strengthPercentage = Math.round(avgOppPct * 100);

          let difficulty: "Easy" | "Med" | "Hard";
          if (strengthPercentage >= 55) {
            difficulty = "Hard";
          } else if (strengthPercentage >= 45) {
            difficulty = "Med";
          } else {
            difficulty = "Easy";
          }

          return { team, difficulty, strengthPercentage };
        })
        .sort((a, b) => b.strengthPercentage - a.strengthPercentage);
    };

    return {
      overallStrength: convertToStrength(overallSOS),
      remainingStrength: convertToStrength(remainingSOS)
    };
  }, [scheduleData, selectedYear]);

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

          <div className="bg-gray-100 dark:bg-gray-700 p-6 rounded-lg mb-8">
            <h2 className="text-xl font-semibold mb-4">Strength of Schedule</h2>
            {isLoading ? (
              <div className="flex items-center justify-center h-40">
                <div className="w-6 h-6 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                <span className="ml-2">Loading schedule data...</span>
              </div>
            ) : overallStrength.length === 0 ? (
              <div className="text-center text-gray-500 dark:text-gray-400 py-8">
                No strength of schedule data available for the selected year. Make sure future matchups are loaded.
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div>
                  <h3 className="text-lg font-medium mb-3">Remaining</h3>
                  <div className="flex items-center mb-2 text-xs text-gray-500 dark:text-gray-400 font-medium">
                    <span className="w-40">Team</span>
                    <span className="flex-1 ml-3">Schedule Difficulty</span>
                    <span className="w-20 text-right ml-3">Opp Win %</span>
                  </div>
                  <div className="space-y-3">
                    {remainingStrength.map(
                      ({ team, difficulty, strengthPercentage }) => (
                        <div key={team} className="flex items-center">
                          <span className="w-40 text-sm truncate">{team}</span>
                          <div className="flex-1 h-5 bg-gray-200 dark:bg-gray-600 rounded-full overflow-hidden ml-3">
                            <div
                              className={`h-full ${getDifficultyColor(difficulty)}`}
                              style={{ width: `${strengthPercentage}%` }}
                            ></div>
                          </div>
                          <span className="w-20 text-right text-sm ml-3">
                            {strengthPercentage}%
                          </span>
                        </div>
                      )
                    )}
                  </div>
                </div>

                <div>
                  <h3 className="text-lg font-medium mb-3">Season Overall</h3>
                  <div className="flex items-center mb-2 text-xs text-gray-500 dark:text-gray-400 font-medium">
                    <span className="w-40">Team</span>
                    <span className="flex-1 ml-3">Schedule Difficulty</span>
                    <span className="w-20 text-right ml-3">Opp Win %</span>
                  </div>
                  <div className="space-y-3">
                    {overallStrength.map(
                      ({ team, difficulty, strengthPercentage }) => (
                        <div key={team} className="flex items-center">
                          <span className="w-40 text-sm truncate">{team}</span>
                          <div className="flex-1 h-5 bg-gray-200 dark:bg-gray-600 rounded-full overflow-hidden ml-3">
                            <div
                              className={`h-full ${getDifficultyColor(difficulty)}`}
                              style={{ width: `${strengthPercentage}%` }}
                            ></div>
                          </div>
                          <span className="w-20 text-right text-sm ml-3">
                            {strengthPercentage}%
                          </span>
                        </div>
                      )
                    )}
                  </div>
                </div>
              </div>
            )}
          </div>

          <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
            <div className="flex flex-col md:flex-row justify-between items-start md:items-center mb-6">
              <h2 className="text-xl font-semibold mb-3 md:mb-0">Matchups</h2>

              <div className="w-full md:w-auto flex flex-col md:flex-row gap-4">
                <div className="flex items-center space-x-2">
                  <input
                    type="checkbox"
                    id="showFutureMatchups"
                    checked={showFutureMatchups}
                    onChange={(e) => setShowFutureMatchups(e.target.checked)}
                    className="w-4 h-4 text-blue-600 bg-gray-100 border-gray-300 rounded focus:ring-blue-500 dark:focus:ring-blue-600 dark:ring-offset-gray-800 focus:ring-2 dark:bg-gray-700 dark:border-gray-600"
                    disabled={isLoading}
                  />
                  <label
                    htmlFor="showFutureMatchups"
                    className="text-sm font-medium text-gray-900 dark:text-gray-300"
                  >
                    Show future matchups
                  </label>
                </div>

                <div className="flex flex-col md:flex-row gap-4">
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
      </div>
    </Layout>
  );
}
