import { useState, useEffect } from "react";
import { useRouter } from "next/router";
import Link from "next/link";
import Layout from "../../components/Layout";
import {
  playersService,
  PlayerDetail,
  PlayerStats,
  GameLogEntry,
} from "../../services/playersService";

// Helper function to get position color
function getPositionColor(position: string): string {
  switch (position) {
    case "QB":
      return "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200";
    case "RB":
      return "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200";
    case "WR":
      return "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200";
    case "TE":
      return "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200";
    case "K":
      return "bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200";
    case "D/ST":
      return "bg-gray-100 text-gray-800 dark:bg-gray-900 dark:text-gray-200";
    default:
      return "bg-gray-100 text-gray-800 dark:bg-gray-900 dark:text-gray-200";
  }
}

// Helper function to filter game log entries
function filterGameLog(
  gameLog: GameLogEntry[],
  yearFilter: string,
  weekFilter: string
): GameLogEntry[] {
  return gameLog.filter((entry) => {
    const yearMatch =
      yearFilter === "all" || entry.year.toString() === yearFilter;
    const weekMatch =
      weekFilter === "all" || entry.week.toString() === weekFilter;
    return yearMatch && weekMatch;
  });
}

// Helper function to format stats based on position
function getRelevantStats(stats: PlayerStats, position: string) {
  switch (position) {
    case "QB":
      return [
        { label: "Passing Yards", value: stats.passingYards.toLocaleString() },
        { label: "Passing TDs", value: stats.passingTDs },
        { label: "Interceptions", value: stats.interceptions },
        { label: "Rushing Yards", value: stats.rushingYards.toLocaleString() },
        { label: "Rushing TDs", value: stats.rushingTDs },
      ];
    case "RB":
      return [
        { label: "Rushing Yards", value: stats.rushingYards.toLocaleString() },
        { label: "Rushing TDs", value: stats.rushingTDs },
        { label: "Receptions", value: stats.receptions },
        {
          label: "Receiving Yards",
          value: stats.receivingYards.toLocaleString(),
        },
        { label: "Receiving TDs", value: stats.receivingTDs },
        { label: "Fumbles", value: stats.fumbles },
      ];
    case "WR":
    case "TE":
      return [
        { label: "Receptions", value: stats.receptions },
        {
          label: "Receiving Yards",
          value: stats.receivingYards.toLocaleString(),
        },
        { label: "Receiving TDs", value: stats.receivingTDs },
        { label: "Rushing Yards", value: stats.rushingYards.toLocaleString() },
        { label: "Rushing TDs", value: stats.rushingTDs },
        { label: "Fumbles", value: stats.fumbles },
      ];
    case "K":
      return [
        { label: "Field Goals", value: stats.fieldGoals },
        { label: "Extra Points", value: stats.extraPoints },
      ];
    default:
      return [{ label: "Total Stats", value: "N/A" }];
  }
}

export default function PlayerDetailPage() {
  const router = useRouter();
  const { id } = router.query;

  const [isLoading, setIsLoading] = useState(true);
  const [player, setPlayer] = useState<PlayerDetail | null>(null);
  const [activeTab, setActiveTab] = useState("overview");
  const [error, setError] = useState<string | null>(null);

  // Add filter states for game log
  const [yearFilter, setYearFilter] = useState<string>("all");
  const [weekFilter, setWeekFilter] = useState<string>("all");

  useEffect(() => {
    if (!id) return;

    const fetchPlayerData = async () => {
      try {
        setIsLoading(true);
        setError(null);

        // Use playersService to fetch player data
        const playerData = await playersService.getPlayerDetail(id as string);
        setPlayer(playerData);
      } catch (err) {
        console.error("Error fetching player data:", err);
        setError(
          err instanceof Error ? err.message : "An unknown error occurred"
        );
      } finally {
        setIsLoading(false);
      }
    };

    fetchPlayerData();
  }, [id]);

  if (isLoading) {
    return (
      <Layout>
        <div className="flex flex-col items-center justify-center h-64 space-y-4">
          <div className="w-8 h-8 border-4 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
          <div className="text-center">
            <div className="text-lg font-medium">Loading player data...</div>
            <div className="text-sm text-gray-500 dark:text-gray-400 mt-2">
              This may take up to 10 seconds as we fetch data from the database
            </div>
          </div>
        </div>
      </Layout>
    );
  }

  if (error || !player) {
    return (
      <Layout>
        <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded">
          <h2 className="text-lg font-medium mb-2">Player not found</h2>
          <p>
            {error ||
              "We could not find a player with the requested ID. Please check the URL and try again."}
          </p>
          <Link
            href="/players"
            className="mt-4 inline-block text-blue-600 hover:text-blue-800 dark:hover:text-blue-400"
          >
            ← Back to Players
          </Link>
        </div>
      </Layout>
    );
  }

  return (
    <Layout>
      <div className="space-y-8">
        {/* Player Header */}
        <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
          <div className="flex flex-col md:flex-row justify-between md:items-center">
            <div>
              <div className="flex items-center mb-2">
                <h1 className="text-3xl md:text-4xl font-bold text-blue-600">
                  {player.name}
                </h1>
                <span
                  className={`ml-3 px-3 py-1 rounded-full text-sm font-medium ${getPositionColor(
                    player.position
                  )}`}
                >
                  {player.position}
                </span>
                <span className="ml-2 px-2 py-1 bg-gray-200 dark:bg-gray-600 rounded text-sm">
                  #{player.positionRank} {player.position}
                </span>
              </div>
            </div>

            <div className="mt-4 md:mt-0">
              <Link
                href="/players"
                className="text-blue-600 hover:text-blue-800 dark:hover:text-blue-400"
              >
                ← Back to Players
              </Link>
            </div>
          </div>
        </section>

        {/* Navigation Tabs */}
        <div className="border-b border-gray-200 dark:border-gray-700">
          <nav className="flex space-x-8">
            <button
              onClick={() => setActiveTab("overview")}
              className={`py-4 px-1 border-b-2 font-medium text-sm ${
                activeTab === "overview"
                  ? "border-blue-600 text-blue-600"
                  : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
              }`}
            >
              Overview
            </button>
            <button
              onClick={() => setActiveTab("stats")}
              className={`py-4 px-1 border-b-2 font-medium text-sm ${
                activeTab === "stats"
                  ? "border-blue-600 text-blue-600"
                  : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
              }`}
            >
              Statistics
            </button>
            <button
              onClick={() => setActiveTab("gamelog")}
              className={`py-4 px-1 border-b-2 font-medium text-sm ${
                activeTab === "gamelog"
                  ? "border-blue-600 text-blue-600"
                  : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
              }`}
            >
              Game Log
            </button>
          </nav>
        </div>

        {/* Tab Content */}
        <div className="space-y-6">
          {/* Overview Tab */}
          {activeTab === "overview" && (
            <>
              <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
                <h2 className="text-xl font-semibold mb-4">
                  Fantasy Performance
                </h2>
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
                  <div className="bg-gray-50 dark:bg-gray-800 p-4 rounded-lg">
                    <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                      Total Points
                    </h3>
                    <div className="text-2xl font-bold">
                      {player.totalFantasyPoints.toFixed(1)}
                    </div>
                    <div className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                      {player.avgFantasyPoints.toFixed(1)} per game
                    </div>
                  </div>

                  <div className="bg-gray-50 dark:bg-gray-800 p-4 rounded-lg">
                    <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                      Projected Points
                    </h3>
                    <div className="text-2xl font-bold">
                      {player.totalProjectedPoints.toFixed(1)}
                    </div>
                    <div className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                      {(
                        player.totalProjectedPoints / player.gamesPlayed
                      ).toFixed(1)}{" "}
                      per game
                    </div>
                  </div>

                  <div className="bg-gray-50 dark:bg-gray-800 p-4 rounded-lg">
                    <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                      Difference
                    </h3>
                    <div
                      className={`text-2xl font-bold ${
                        player.difference > 0
                          ? "text-green-600 dark:text-green-400"
                          : "text-red-600 dark:text-red-400"
                      }`}
                    >
                      {player.difference > 0 ? "+" : ""}
                      {player.difference.toFixed(1)}
                    </div>
                    <div className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                      vs projection
                    </div>
                  </div>

                  <div className="bg-gray-50 dark:bg-gray-800 p-4 rounded-lg">
                    <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                      Games Played
                    </h3>
                    <div className="text-2xl font-bold">
                      {player.gamesPlayed}
                    </div>
                    <div className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                      Position rank: #{player.positionRank}
                    </div>
                  </div>
                </div>
              </section>

              <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
                <h2 className="text-xl font-semibold mb-4">
                  Season Statistics
                </h2>
                <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-4">
                  {getRelevantStats(player.totalStats, player.position).map(
                    (stat, index) => (
                      <div key={index} className="text-center">
                        <div className="text-2xl font-bold text-blue-600">
                          {stat.value}
                        </div>
                        <div className="text-sm text-gray-500 dark:text-gray-400">
                          {stat.label}
                        </div>
                      </div>
                    )
                  )}
                </div>
              </section>

              {/* Annual Statistics Table */}
              <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
                <h2 className="text-xl font-semibold mb-4">
                  Annual Statistics
                </h2>
                {player.annualStats && player.annualStats.length > 0 ? (
                  <div className="overflow-x-auto">
                    <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-600">
                      <thead className="bg-gray-50 dark:bg-gray-800">
                        <tr>
                          <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                            Year
                          </th>
                          <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                            Games
                          </th>
                          <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                            Points
                          </th>
                          <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                            Projected
                          </th>
                          <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                            Average
                          </th>
                          <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                            Difference
                          </th>
                          <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                            Best Game
                          </th>
                          <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                            Worst Game
                          </th>
                          <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                            Consistency
                          </th>
                          {player.position === "QB" && (
                            <>
                              <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                                Pass Yds
                              </th>
                              <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                                Pass TDs
                              </th>
                              <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                                INTs
                              </th>
                            </>
                          )}
                          {(player.position === "RB" ||
                            player.position === "WR" ||
                            player.position === "TE") && (
                            <>
                              <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                                Rec
                              </th>
                              <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                                Rec Yds
                              </th>
                              <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                                Rec TDs
                              </th>
                            </>
                          )}
                          {player.position === "K" && (
                            <>
                              <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                                FG
                              </th>
                              <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                                XP
                              </th>
                            </>
                          )}
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
                        {player.annualStats.map((yearStats, index) => (
                          <tr
                            key={yearStats.year}
                            className={
                              index % 2 === 0
                                ? "bg-white dark:bg-gray-700"
                                : "bg-gray-50 dark:bg-gray-800"
                            }
                          >
                            <td className="py-3 px-4 text-sm font-medium text-blue-600">
                              {yearStats.year}
                            </td>
                            <td className="py-3 px-4 text-sm">
                              {yearStats.gamesPlayed}
                            </td>
                            <td className="py-3 px-4 text-sm font-bold">
                              {yearStats.totalFantasyPoints.toFixed(1)}
                            </td>
                            <td className="py-3 px-4 text-sm text-gray-500">
                              {yearStats.totalProjectedPoints.toFixed(1)}
                            </td>
                            <td className="py-3 px-4 text-sm">
                              {yearStats.avgFantasyPoints.toFixed(1)}
                            </td>
                            <td
                              className={`py-3 px-4 text-sm font-medium ${
                                yearStats.difference > 0
                                  ? "text-green-600 dark:text-green-400"
                                  : "text-red-600 dark:text-red-400"
                              }`}
                            >
                              {yearStats.difference > 0 ? "+" : ""}
                              {yearStats.difference.toFixed(1)}
                            </td>
                            <td className="py-3 px-4 text-sm">
                              <div className="font-medium text-green-600 dark:text-green-400">
                                {yearStats.bestGame.points.toFixed(1)}
                              </div>
                              <div className="text-xs text-gray-500">
                                Wk {yearStats.bestGame.week}
                              </div>
                            </td>
                            <td className="py-3 px-4 text-sm">
                              <div className="font-medium text-red-600 dark:text-red-400">
                                {yearStats.worstGame.points.toFixed(1)}
                              </div>
                              <div className="text-xs text-gray-500">
                                Wk {yearStats.worstGame.week}
                              </div>
                            </td>
                            <td className="py-3 px-4 text-sm">
                              <div className="font-medium">
                                {yearStats.consistencyScore.toFixed(1)}
                              </div>
                              <div className="text-xs text-gray-500">
                                {yearStats.consistencyScore < 5
                                  ? "Very consistent"
                                  : yearStats.consistencyScore < 8
                                  ? "Consistent"
                                  : yearStats.consistencyScore < 12
                                  ? "Variable"
                                  : "Inconsistent"}
                              </div>
                            </td>
                            {player.position === "QB" && (
                              <>
                                <td className="py-3 px-4 text-sm">
                                  {yearStats.totalStats.passingYards.toLocaleString()}
                                </td>
                                <td className="py-3 px-4 text-sm">
                                  {yearStats.totalStats.passingTDs}
                                </td>
                                <td className="py-3 px-4 text-sm">
                                  {yearStats.totalStats.interceptions}
                                </td>
                              </>
                            )}
                            {(player.position === "RB" ||
                              player.position === "WR" ||
                              player.position === "TE") && (
                              <>
                                <td className="py-3 px-4 text-sm">
                                  {yearStats.totalStats.receptions}
                                </td>
                                <td className="py-3 px-4 text-sm">
                                  {yearStats.totalStats.receivingYards.toLocaleString()}
                                </td>
                                <td className="py-3 px-4 text-sm">
                                  {yearStats.totalStats.receivingTDs}
                                </td>
                              </>
                            )}
                            {player.position === "K" && (
                              <>
                                <td className="py-3 px-4 text-sm">
                                  {yearStats.totalStats.fieldGoals}
                                </td>
                                <td className="py-3 px-4 text-sm">
                                  {yearStats.totalStats.extraPoints}
                                </td>
                              </>
                            )}
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <div className="text-center py-8 text-gray-500 dark:text-gray-400">
                    <p>No annual statistics available</p>
                  </div>
                )}
              </section>

              <section className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
                  <h2 className="text-lg font-semibold mb-4">
                    Performance Trends
                  </h2>
                  {/* TODO: Add chart showing points per game over time */}
                  <div className="text-center py-8 text-gray-500 dark:text-gray-400">
                    <p>Performance chart coming soon</p>
                    <p className="text-sm mt-2">
                      Will show weekly fantasy points trend
                    </p>
                  </div>
                </div>

                <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
                  <h2 className="text-lg font-semibold mb-4">Quick Stats</h2>
                  <div className="space-y-3">
                    <div className="flex justify-between">
                      <span>Best Game:</span>
                      <span className="font-medium text-green-600 dark:text-green-400">
                        {player.bestGame?.points > 0 ? (
                          <>
                            {player.bestGame.points.toFixed(1)} pts
                            <div className="text-xs text-gray-500 dark:text-gray-400">
                              {player.bestGame.year} Week {player.bestGame.week}
                            </div>
                          </>
                        ) : (
                          "No games"
                        )}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Worst Game:</span>
                      <span className="font-medium text-red-600 dark:text-red-400">
                        {player.worstGame?.points >= 0 &&
                        player.worstGame.points < 1000 ? (
                          <>
                            {player.worstGame.points.toFixed(1)} pts
                            <div className="text-xs text-gray-500 dark:text-gray-400">
                              {player.worstGame.year} Week{" "}
                              {player.worstGame.week}
                            </div>
                          </>
                        ) : (
                          "No games"
                        )}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Consistency:</span>
                      <span className="font-medium">
                        {player.consistencyScore > 0 ? (
                          <>
                            σ = {player.consistencyScore.toFixed(1)}
                            <div className="text-xs text-gray-500 dark:text-gray-400">
                              {player.consistencyScore < 5
                                ? "Very consistent"
                                : player.consistencyScore < 8
                                ? "Consistent"
                                : player.consistencyScore < 12
                                ? "Variable"
                                : "Inconsistent"}
                            </div>
                          </>
                        ) : (
                          "No data"
                        )}
                      </span>
                    </div>
                  </div>
                </div>
              </section>
            </>
          )}

          {/* Statistics Tab */}
          {activeTab === "stats" && (
            <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
              <h2 className="text-xl font-semibold mb-4">
                Detailed Statistics
              </h2>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
                {/* Offensive Stats */}
                <div>
                  <h3 className="text-lg font-medium mb-4">
                    Offensive Statistics
                  </h3>
                  <div className="space-y-3">
                    {player.position === "QB" && (
                      <>
                        <div className="flex justify-between py-2 border-b dark:border-gray-600">
                          <span>Passing Yards</span>
                          <span className="font-medium">
                            {player.totalStats.passingYards.toLocaleString()}
                          </span>
                        </div>
                        <div className="flex justify-between py-2 border-b dark:border-gray-600">
                          <span>Passing Touchdowns</span>
                          <span className="font-medium">
                            {player.totalStats.passingTDs}
                          </span>
                        </div>
                        <div className="flex justify-between py-2 border-b dark:border-gray-600">
                          <span>Interceptions</span>
                          <span className="font-medium">
                            {player.totalStats.interceptions}
                          </span>
                        </div>
                        <div className="flex justify-between py-2 border-b dark:border-gray-600">
                          <span>Rushing Yards</span>
                          <span className="font-medium">
                            {player.totalStats.rushingYards.toLocaleString()}
                          </span>
                        </div>
                        <div className="flex justify-between py-2">
                          <span>Rushing Touchdowns</span>
                          <span className="font-medium">
                            {player.totalStats.rushingTDs}
                          </span>
                        </div>
                      </>
                    )}

                    {(player.position === "RB" ||
                      player.position === "WR" ||
                      player.position === "TE") && (
                      <>
                        <div className="flex justify-between py-2 border-b dark:border-gray-600">
                          <span>Receptions</span>
                          <span className="font-medium">
                            {player.totalStats.receptions}
                          </span>
                        </div>
                        <div className="flex justify-between py-2 border-b dark:border-gray-600">
                          <span>Receiving Yards</span>
                          <span className="font-medium">
                            {player.totalStats.receivingYards.toLocaleString()}
                          </span>
                        </div>
                        <div className="flex justify-between py-2 border-b dark:border-gray-600">
                          <span>Receiving Touchdowns</span>
                          <span className="font-medium">
                            {player.totalStats.receivingTDs}
                          </span>
                        </div>
                        {player.position === "RB" && (
                          <>
                            <div className="flex justify-between py-2 border-b dark:border-gray-600">
                              <span>Rushing Yards</span>
                              <span className="font-medium">
                                {player.totalStats.rushingYards.toLocaleString()}
                              </span>
                            </div>
                            <div className="flex justify-between py-2 border-b dark:border-gray-600">
                              <span>Rushing Touchdowns</span>
                              <span className="font-medium">
                                {player.totalStats.rushingTDs}
                              </span>
                            </div>
                          </>
                        )}
                        <div className="flex justify-between py-2">
                          <span>Fumbles</span>
                          <span className="font-medium">
                            {player.totalStats.fumbles}
                          </span>
                        </div>
                      </>
                    )}

                    {player.position === "K" && (
                      <>
                        <div className="flex justify-between py-2 border-b dark:border-gray-600">
                          <span>Field Goals Made</span>
                          <span className="font-medium">
                            {player.totalStats.fieldGoals}
                          </span>
                        </div>
                        <div className="flex justify-between py-2">
                          <span>Extra Points Made</span>
                          <span className="font-medium">
                            {player.totalStats.extraPoints}
                          </span>
                        </div>
                      </>
                    )}
                  </div>
                </div>

                {/* Per Game Averages */}
                <div>
                  <h3 className="text-lg font-medium mb-4">
                    Per Game Averages
                  </h3>
                  <div className="space-y-3">
                    <div className="flex justify-between py-2 border-b dark:border-gray-600">
                      <span>Fantasy Points</span>
                      <span className="font-medium">
                        {player.avgFantasyPoints.toFixed(1)}
                      </span>
                    </div>
                    {player.position === "QB" && (
                      <>
                        <div className="flex justify-between py-2 border-b dark:border-gray-600">
                          <span>Passing Yards</span>
                          <span className="font-medium">
                            {(
                              player.totalStats.passingYards /
                              player.gamesPlayed
                            ).toFixed(1)}
                          </span>
                        </div>
                        <div className="flex justify-between py-2 border-b dark:border-gray-600">
                          <span>Passing TDs</span>
                          <span className="font-medium">
                            {(
                              player.totalStats.passingTDs / player.gamesPlayed
                            ).toFixed(1)}
                          </span>
                        </div>
                      </>
                    )}
                    {(player.position === "RB" ||
                      player.position === "WR" ||
                      player.position === "TE") && (
                      <>
                        <div className="flex justify-between py-2 border-b dark:border-gray-600">
                          <span>Receptions</span>
                          <span className="font-medium">
                            {(
                              player.totalStats.receptions / player.gamesPlayed
                            ).toFixed(1)}
                          </span>
                        </div>
                        <div className="flex justify-between py-2 border-b dark:border-gray-600">
                          <span>Receiving Yards</span>
                          <span className="font-medium">
                            {(
                              player.totalStats.receivingYards /
                              player.gamesPlayed
                            ).toFixed(1)}
                          </span>
                        </div>
                      </>
                    )}
                    {/* Add more per-game stats as needed */}
                  </div>
                </div>
              </div>
            </section>
          )}

          {/* Game Log Tab */}
          {activeTab === "gamelog" && (
            <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
              <h2 className="text-xl font-semibold mb-4">Game Log</h2>

              {/* Filters */}
              <div className="flex flex-col md:flex-row gap-4 mb-6">
                <div className="w-full md:w-auto">
                  <label
                    htmlFor="year-filter"
                    className="block text-sm font-medium text-gray-500 dark:text-gray-400 mb-1"
                  >
                    Filter by Year
                  </label>
                  <select
                    id="year-filter"
                    className="w-full md:w-auto p-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                    value={yearFilter}
                    onChange={(e) => setYearFilter(e.target.value)}
                  >
                    <option value="all">All Years</option>
                    <option value="2024">2024</option>
                    <option value="2023">2023</option>
                  </select>
                </div>

                <div className="w-full md:w-auto">
                  <label
                    htmlFor="week-filter"
                    className="block text-sm font-medium text-gray-500 dark:text-gray-400 mb-1"
                  >
                    Filter by Week
                  </label>
                  <select
                    id="week-filter"
                    className="w-full md:w-auto p-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                    value={weekFilter}
                    onChange={(e) => setWeekFilter(e.target.value)}
                  >
                    <option value="all">All Weeks</option>
                    {Array.from({ length: 18 }, (_, i) => i + 1).map((week) => (
                      <option key={week} value={week}>
                        Week {week}
                      </option>
                    ))}
                  </select>
                </div>

                {/* Reset button */}
                {(yearFilter !== "all" || weekFilter !== "all") && (
                  <div className="w-full md:w-auto flex items-end">
                    <button
                      onClick={() => {
                        setYearFilter("all");
                        setWeekFilter("all");
                      }}
                      className="py-2 px-4 bg-gray-100 hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600 text-gray-800 dark:text-gray-200 rounded-md border border-gray-300 dark:border-gray-600 transition-colors"
                    >
                      <div className="flex items-center">
                        <svg
                          xmlns="http://www.w3.org/2000/svg"
                          className="h-4 w-4 mr-1"
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                        >
                          <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth={2}
                            d="M6 18L18 6M6 6l12 12"
                          />
                        </svg>
                        Reset Filters
                      </div>
                    </button>
                  </div>
                )}
              </div>

              {/* Game Log Table */}
              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-600">
                  <thead className="bg-gray-50 dark:bg-gray-800">
                    <tr>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Week
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Year
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Points
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Projected
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Difference
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Started
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Date
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
                    {(() => {
                      const filteredGameLog = filterGameLog(
                        player.gameLog || [],
                        yearFilter,
                        weekFilter
                      );

                      if (filteredGameLog.length === 0) {
                        return (
                          <tr>
                            <td
                              colSpan={7}
                              className="py-8 text-center text-gray-500 dark:text-gray-400"
                            >
                              {player.gameLog?.length === 0
                                ? "No game log data available"
                                : "No games match the selected filters"}
                            </td>
                          </tr>
                        );
                      }

                      return filteredGameLog.map((game, index) => (
                        <tr
                          key={`${game.year}-${game.week}`}
                          className={
                            index % 2 === 0
                              ? "bg-white dark:bg-gray-700"
                              : "bg-gray-50 dark:bg-gray-800"
                          }
                        >
                          <td className="py-3 px-4 text-sm font-medium">
                            {game.week}
                          </td>
                          <td className="py-3 px-4 text-sm">{game.year}</td>
                          <td className="py-3 px-4 text-sm font-bold text-blue-600">
                            {game.actualPoints.toFixed(1)}
                          </td>
                          <td className="py-3 px-4 text-sm text-gray-500">
                            {game.projectedPoints.toFixed(1)}
                          </td>
                          <td
                            className={`py-3 px-4 text-sm font-medium ${
                              game.difference > 0
                                ? "text-green-600 dark:text-green-400"
                                : "text-red-600 dark:text-red-400"
                            }`}
                          >
                            {game.difference > 0 ? "+" : ""}
                            {game.difference.toFixed(1)}
                          </td>
                          <td className="py-3 px-4 text-sm">
                            <span
                              className={`inline-flex px-2 py-1 text-xs font-semibold rounded-full ${
                                game.startedFlag
                                  ? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
                                  : "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300"
                              }`}
                            >
                              {game.startedFlag ? "Started" : "Bench"}
                            </span>
                          </td>
                          <td className="py-3 px-4 text-sm text-gray-500">
                            {new Date(game.gameDate).toLocaleDateString()}
                          </td>
                        </tr>
                      ));
                    })()}
                  </tbody>
                </table>
              </div>
            </section>
          )}
        </div>
      </div>
    </Layout>
  );
}
