import { useState, useEffect } from "react";
import { useRouter } from "next/router";
import Link from "next/link";
import Layout from "../../components/Layout";
import {
  teamsService,
  TeamDetail as TeamDetailType,
  ScheduleGame,
} from "../../services/teamsService";

// Type definitions for the legacy UI components
interface Player {
  id: number | string;
  name: string;
  position: string;
  team: string;
  points: number;
  projection: number;
  status: string;
}

interface DraftPick {
  round: number;
  pick: number;
  overall: number; // Calculated based on round and pick
  description: string;
  playerId: number | string; // Reference to associated player
  player: string;
  position: string;
}

interface Game {
  week: number;
  year: number;
  opponent: string;
  result: "W" | "L" | "T" | "-";
  score: string;
  isHome: boolean;
}

// This mapping function converts API data to UI component format
function mapApiDataToUiFormat(teamData: TeamDetailType): {
  id: number;
  name: string;
  owner: string;
  record: {
    wins: number;
    losses: number;
    ties: number;
  };
  points: {
    scored: number;
    against: number;
  };
  rank: number; // TODO: Actual rank from standings
  playoffChance: number; // TODO: Calculate from simulation data
  players: Player[];
  draftPicks: DraftPick[];
  schedule: Game[];
} {
  // Convert API players to UI format
  const players: Player[] = teamData.currentPlayers.map((player) => ({
    id: player.id,
    name: player.name,
    position: player.position,
    team: player.team,
    points: player.fantasyPoints,
    projection: 0, // TODO: Get projections from API
    status: player.status,
  }));

  // Convert API draft picks to UI format
  const draftPicks: DraftPick[] = teamData.draftPicks.map((pick) => ({
    round: pick.round,
    pick: pick.pick,
    overall: (pick.round - 1) * 10 + pick.pick, // Assuming 12 teams per round
    description: `${pick.round}${getOrdinalSuffix(pick.round)} Round (${
      pick.year
    })`,
    playerId: 0, // TODO: Link draft picks to players when API supports it
    player: pick.player,
    position: pick.position,
  }));

  let pointsScored = 0;
  let pointsAgainst = 0;
  teamData.schedule.forEach((game) => {
    if (game.completed) {
      pointsScored += game.teamScore;
      pointsAgainst += game.opponentScore;
    }
  });

  let wins = 0;
  let losses = 0;
  let ties = 0;
  teamData.schedule.forEach((game) => {
    if (game.completed) {
      if (game.result === "W") wins++;
      else if (game.result === "L") losses++;
      else if (game.result === "T") ties++;
    }
  });

  // Convert API schedule to UI format
  const schedule: Game[] = teamData.schedule.map((game) => ({
    week: game.week,
    year: game.year,
    opponent: game.opponent,
    result: game.result as "W" | "L" | "T" | "-",
    score: game.completed
      ? `${game.teamScore.toFixed(2)}-${game.opponentScore.toFixed(2)}`
      : "0-0",
    isHome: game.isHome,
  }));

  return {
    id: parseInt(teamData.id),
    name: teamData.name,
    owner: teamData.owner,
    record: {
      wins,
      losses,
      ties,
    },
    points: {
      scored: pointsScored,
      against: pointsAgainst,
    },
    rank: 1, // TODO: Get actual rank from standings
    playoffChance: 0, // TODO: Calculate from simulation data
    players,
    draftPicks,
    schedule,
  };
}

// Helper function to get ordinal suffix (1st, 2nd, 3rd, etc.)
function getOrdinalSuffix(num: number): string {
  const j = num % 10;
  const k = num % 100;
  if (j === 1 && k !== 11) {
    return "st";
  }
  if (j === 2 && k !== 12) {
    return "nd";
  }
  if (j === 3 && k !== 13) {
    return "rd";
  }
  return "th";
}

// Calculate team performance metrics from schedule data
function calculateTeamStats(games: Game[]) {
  // Filter only completed games (games with results)
  const completedGames = games.filter((game) => game.result !== "-");
  if (completedGames.length === 0) return null;

  // Current streak calculation
  const currentStreakGames = [...completedGames].sort((a, b) => {
    if (a.year !== b.year) return b.year - a.year;
    return b.week - a.week;
  });

  let streakType = currentStreakGames[0]?.result;
  let streakCount = 0;

  for (const game of currentStreakGames) {
    if (game.result === streakType) {
      streakCount++;
    } else {
      break;
    }
  }

  // Home vs Away performance
  const homeGames = completedGames.filter((game) => game.isHome);
  const awayGames = completedGames.filter((game) => !game.isHome);

  const homeRecord = {
    wins: homeGames.filter((game) => game.result === "W").length,
    losses: homeGames.filter((game) => game.result === "L").length,
    ties: homeGames.filter((game) => game.result === "T").length,
  };

  const awayRecord = {
    wins: awayGames.filter((game) => game.result === "W").length,
    losses: awayGames.filter((game) => game.result === "L").length,
    ties: awayGames.filter((game) => game.result === "T").length,
  };

  // Recent form (last 5 games)
  const recentGames = [...completedGames]
    .sort((a, b) => {
      if (a.year !== b.year) return b.year - a.year;
      return b.week - a.week;
    })
    .slice(0, 5);

  const recentForm = recentGames.map((game) => game.result);

  // Opponent analysis - find most common opponents and record against them
  const opponentStats: Record<
    string,
    { wins: number; losses: number; ties: number }
  > = {};

  completedGames.forEach((game) => {
    if (!opponentStats[game.opponent]) {
      opponentStats[game.opponent] = { wins: 0, losses: 0, ties: 0 };
    }

    if (game.result === "W") opponentStats[game.opponent].wins++;
    else if (game.result === "L") opponentStats[game.opponent].losses++;
    else if (game.result === "T") opponentStats[game.opponent].ties++;
  });

  // Get top 3 most played against opponents
  const topOpponents = Object.entries(opponentStats)
    .map(([opponent, record]) => ({
      opponent,
      totalGames: record.wins + record.losses + record.ties,
      winPercentage:
        (record.wins / (record.wins + record.losses + record.ties)) * 100,
      record,
    }))
    .sort((a, b) => b.totalGames - a.totalGames)
    .slice(0, 3);

  return {
    streak: {
      type: streakType,
      count: streakCount,
    },
    homeRecord,
    awayRecord,
    recentForm,
    topOpponents,
  };
}

// Helper to get formatted streak text
function getStreakText(streak: { type: string; count: number }) {
  if (!streak || streak.count === 0) return "No streak";

  const streakType =
    streak.type === "W" ? "Win" : streak.type === "L" ? "Loss" : "Tie";
  return `${streakType} ${streak.count}`;
}

export default function TeamDetail() {
  const router = useRouter();
  const { id } = router.query;

  const [isLoading, setIsLoading] = useState(true);
  const [team, setTeam] = useState<ReturnType<
    typeof mapApiDataToUiFormat
  > | null>(null);
  const [activeTab, setActiveTab] = useState("overview");
  const [error, setError] = useState<string | null>(null);

  // Add these state variables at the top of the TeamDetail function component
  const [yearFilter, setYearFilter] = useState<string>("all");
  const [opponentFilter, setOpponentFilter] = useState<string>("all");

  // Add these state variables
  const [teamStats, setTeamStats] = useState<ReturnType<
    typeof calculateTeamStats
  > | null>(null);

  useEffect(() => {
    if (!id) return;

    const fetchTeamData = async () => {
      try {
        setIsLoading(true);
        setError(null);

        // Use teamsService to fetch detailed team data
        const teamData = await teamsService.getTeamDetail(id as string);

        // Map API data to the format expected by the UI
        const mappedTeam = mapApiDataToUiFormat(teamData);
        setTeam(mappedTeam);

        // Calculate additional team statistics
        if (mappedTeam) {
          const stats = calculateTeamStats(mappedTeam.schedule);
          setTeamStats(stats);
          console.log("Team stats calculated:", stats);
        }
      } catch (err) {
        console.error("Error fetching team data:", err);
        setError(
          err instanceof Error ? err.message : "An unknown error occurred"
        );
      } finally {
        setIsLoading(false);
      }
    };

    fetchTeamData();
  }, [id]);

  if (isLoading) {
    return (
      <Layout>
        <div className="flex items-center justify-center h-64">
          <div className="w-8 h-8 border-4 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
          <span className="ml-3 text-lg">Loading team data...</span>
        </div>
      </Layout>
    );
  }

  if (!team) {
    return (
      <Layout>
        <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded">
          <h2 className="text-lg font-medium mb-2">Team not found</h2>
          <p>
            We couldn't find a team with the requested ID. Please check the URL
            and try again.
          </p>
          <Link
            href="/teams"
            className="mt-4 inline-block text-blue-600 hover:text-blue-800 dark:hover:text-blue-400"
          >
            ← Back to Teams
          </Link>
        </div>
      </Layout>
    );
  }

  return (
    <Layout>
      <div className="space-y-8">
        {/* Team Header */}
        <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
          <div className="flex flex-col md:flex-row justify-between md:items-center">
            <div>
              <div className="flex items-center">
                <h1 className="text-3xl md:text-4xl font-bold text-blue-600">
                  {team.name}
                </h1>
                <span className="ml-3 px-2 py-1 bg-gray-200 dark:bg-gray-600 rounded text-sm">
                  Rank #{team.rank}
                </span>
              </div>
              <p className="text-lg text-gray-500 dark:text-gray-400">
                Managed by {team.owner}
              </p>
            </div>

            <div className="mt-4 md:mt-0">
              <Link
                href="/teams"
                className="text-blue-600 hover:text-blue-800 dark:hover:text-blue-400"
              >
                ← Back to Teams
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
              onClick={() => setActiveTab("players")}
              className={`py-4 px-1 border-b-2 font-medium text-sm ${
                activeTab === "players"
                  ? "border-blue-600 text-blue-600"
                  : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
              }`}
            >
              Players
            </button>
            <button
              onClick={() => setActiveTab("schedule")}
              className={`py-4 px-1 border-b-2 font-medium text-sm ${
                activeTab === "schedule"
                  ? "border-blue-600 text-blue-600"
                  : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
              }`}
            >
              Schedule
            </button>
            <button
              onClick={() => setActiveTab("draft")}
              className={`py-4 px-1 border-b-2 font-medium text-sm ${
                activeTab === "draft"
                  ? "border-blue-600 text-blue-600"
                  : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
              }`}
            >
              Draft Picks
            </button>
          </nav>
        </div>

        {/* Tab Content */}
        <div className="space-y-6">
          {/* Overview Tab */}
          {activeTab === "overview" && (
            <>
              <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
                <h2 className="text-xl font-semibold mb-4">Team Overview</h2>
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
                  <div className="bg-gray-50 dark:bg-gray-800 p-4 rounded-lg">
                    <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                      Record
                    </h3>
                    <div className="text-2xl font-bold">
                      {teamStats
                        ? teamStats.awayRecord.wins + teamStats.homeRecord.wins
                        : "-"}
                      -
                      {teamStats
                        ? teamStats.awayRecord.losses +
                          teamStats.homeRecord.losses
                        : "-"}
                      {teamStats
                        ? teamStats.awayRecord.ties +
                            teamStats.homeRecord.ties >
                          0
                          ? teamStats.awayRecord.ties +
                            teamStats.homeRecord.ties
                          : ""
                        : ""}
                    </div>
                    {teamStats?.streak && (
                      <div className="mt-1 text-sm">
                        <span
                          className={`font-medium ${
                            teamStats.streak.type === "W"
                              ? "text-green-600 dark:text-green-400"
                              : teamStats.streak.type === "L"
                              ? "text-red-600 dark:text-red-400"
                              : "text-yellow-600 dark:text-yellow-400"
                          }`}
                        >
                          {getStreakText(teamStats.streak)}
                        </span>
                      </div>
                    )}
                  </div>

                  <div className="bg-gray-50 dark:bg-gray-800 p-4 rounded-lg">
                    <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                      Points For
                    </h3>
                    <div className="text-2xl font-bold">
                      {team.points.scored.toLocaleString(undefined, {
                        minimumFractionDigits: 2,
                        maximumFractionDigits: 2,
                      })}
                    </div>
                    <div className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                      {(
                        team.points.scored /
                        (team.record.wins +
                          team.record.losses +
                          team.record.ties)
                      ).toFixed(2)}{" "}
                      per game
                    </div>
                  </div>

                  <div className="bg-gray-50 dark:bg-gray-800 p-4 rounded-lg">
                    <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                      Points Against
                    </h3>
                    <div className="text-2xl font-bold">
                      {team.points.against.toLocaleString(undefined, {
                        minimumFractionDigits: 2,
                        maximumFractionDigits: 2,
                      })}
                    </div>
                    <div className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                      {(
                        team.points.against /
                        (team.record.wins +
                          team.record.losses +
                          team.record.ties)
                      ).toFixed(2)}{" "}
                      per game
                    </div>
                  </div>

                  <div className="bg-gray-50 dark:bg-gray-800 p-4 rounded-lg">
                    <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                      Point Differential
                    </h3>
                    <div
                      className={`text-2xl font-bold ${
                        team.points.scored - team.points.against > 0
                          ? "text-green-600 dark:text-green-400"
                          : "text-red-600 dark:text-red-400"
                      }`}
                    >
                      {team.points.scored - team.points.against > 0 ? "+" : ""}
                      {(team.points.scored - team.points.against).toFixed(2)}
                    </div>
                    <div className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                      {(
                        (team.points.scored - team.points.against) /
                        (team.record.wins +
                          team.record.losses +
                          team.record.ties)
                      ).toFixed(2)}{" "}
                      per game
                    </div>
                  </div>
                </div>
              </section>

              {/* Add detailed performance metrics section */}
              <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
                <h2 className="text-xl font-semibold mb-4">
                  Performance Breakdown
                </h2>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  <div className="border rounded-lg p-4 dark:border-gray-600">
                    <h3 className="font-medium mb-3 text-lg">Home vs Away</h3>
                    <div className="flex flex-col space-y-4">
                      <div>
                        <div className="flex justify-between mb-1">
                          <span>Home Record</span>
                          <span className="font-medium">
                            {teamStats?.homeRecord
                              ? `${teamStats.homeRecord.wins}-${
                                  teamStats.homeRecord.losses
                                }${
                                  teamStats.homeRecord.ties > 0
                                    ? `-${teamStats.homeRecord.ties}`
                                    : ""
                                }`
                              : "N/A"}
                          </span>
                        </div>
                        {teamStats?.homeRecord && (
                          <div className="w-full bg-gray-200 dark:bg-gray-600 rounded-full h-2.5">
                            <div
                              className="h-2.5 rounded-full bg-blue-600"
                              style={{
                                width: `${
                                  (teamStats.homeRecord.wins /
                                    (teamStats.homeRecord.wins +
                                      teamStats.homeRecord.losses +
                                      teamStats.homeRecord.ties)) *
                                  100
                                }%`,
                              }}
                            ></div>
                          </div>
                        )}
                      </div>

                      <div>
                        <div className="flex justify-between mb-1">
                          <span>Away Record</span>
                          <span className="font-medium">
                            {teamStats?.awayRecord
                              ? `${teamStats.awayRecord.wins}-${
                                  teamStats.awayRecord.losses
                                }${
                                  teamStats.awayRecord.ties > 0
                                    ? `-${teamStats.awayRecord.ties}`
                                    : ""
                                }`
                              : "N/A"}
                          </span>
                        </div>
                        {teamStats?.awayRecord && (
                          <div className="w-full bg-gray-200 dark:bg-gray-600 rounded-full h-2.5">
                            <div
                              className="h-2.5 rounded-full bg-green-600"
                              style={{
                                width: `${
                                  (teamStats.awayRecord.wins /
                                    (teamStats.awayRecord.wins +
                                      teamStats.awayRecord.losses +
                                      teamStats.awayRecord.ties)) *
                                  100
                                }%`,
                              }}
                            ></div>
                          </div>
                        )}
                      </div>
                    </div>
                  </div>

                  <div className="border rounded-lg p-4 dark:border-gray-600">
                    <h3 className="font-medium mb-3 text-lg">Recent Form</h3>
                    {teamStats?.recentForm &&
                    teamStats.recentForm.length > 0 ? (
                      <div className="flex space-x-2 mb-2">
                        {teamStats.recentForm.map((result, i) => (
                          <div
                            key={i}
                            className={`w-8 h-8 rounded-full flex items-center justify-center text-white font-medium ${
                              result === "W"
                                ? "bg-green-500"
                                : result === "L"
                                ? "bg-red-500"
                                : "bg-yellow-500"
                            }`}
                          >
                            {result}
                          </div>
                        ))}
                      </div>
                    ) : (
                      <p className="text-gray-500 dark:text-gray-400">
                        No recent games available
                      </p>
                    )}

                    <div className="mt-4">
                      <h4 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                        Last 5 Games
                      </h4>
                      <div className="text-sm">
                        {teamStats?.recentForm && (
                          <span>
                            {
                              teamStats.recentForm.filter((r) => r === "W")
                                .length
                            }{" "}
                            wins,{" "}
                            {
                              teamStats.recentForm.filter((r) => r === "L")
                                .length
                            }{" "}
                            losses
                            {teamStats.recentForm.filter((r) => r === "T")
                              .length > 0 &&
                              `, ${
                                teamStats.recentForm.filter((r) => r === "T")
                                  .length
                              } ties`}
                          </span>
                        )}
                      </div>
                    </div>
                  </div>
                </div>
              </section>

              <section className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
                  <h2 className="text-lg font-semibold mb-4">Playoff Odds</h2>
                  <div className="space-y-4">
                    <div>
                      <div className="flex justify-between mb-1">
                        <span>Make Playoffs</span>
                        <span className="font-medium">
                          {team.playoffChance}%
                        </span>
                      </div>
                      <div className="w-full bg-gray-200 dark:bg-gray-600 rounded-full h-2.5">
                        <div
                          className="h-2.5 rounded-full bg-blue-600"
                          style={{ width: `${team.playoffChance}%` }}
                        ></div>
                      </div>
                    </div>

                    <div>
                      <div className="flex justify-between mb-1">
                        <span>First Round Bye</span>
                        <span className="font-medium">65%</span>
                      </div>
                      <div className="w-full bg-gray-200 dark:bg-gray-600 rounded-full h-2.5">
                        <div
                          className="h-2.5 rounded-full bg-green-600"
                          style={{ width: "65%" }}
                        ></div>
                      </div>
                    </div>

                    <div>
                      <div className="flex justify-between mb-1">
                        <span>Win Championship</span>
                        <span className="font-medium">28%</span>
                      </div>
                      <div className="w-full bg-gray-200 dark:bg-gray-600 rounded-full h-2.5">
                        <div
                          className="h-2.5 rounded-full bg-yellow-600"
                          style={{ width: "28%" }}
                        ></div>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
                  <h2 className="text-lg font-semibold mb-4">
                    Recent Performance
                  </h2>
                  <div className="space-y-3">
                    {team.schedule
                      .slice(-5)
                      .reverse()
                      .map((game, i) => (
                        <div
                          key={i}
                          className="flex items-center justify-between py-2 border-b dark:border-gray-600 last:border-0"
                        >
                          <div>
                            <span className="font-medium">
                              Week {game.week}
                            </span>
                            <span className="mx-2 text-gray-400">vs</span>
                            <span>{game.opponent}</span>
                          </div>
                          <div className="flex items-center">
                            <span className="mr-2">{game.score}</span>
                            {game.result === "W" && (
                              <span className="w-5 h-5 rounded-full bg-green-500 text-white flex items-center justify-center text-xs">
                                W
                              </span>
                            )}
                            {game.result === "L" && (
                              <span className="w-5 h-5 rounded-full bg-red-500 text-white flex items-center justify-center text-xs">
                                L
                              </span>
                            )}
                            {game.result === "T" && (
                              <span className="w-5 h-5 rounded-full bg-yellow-500 text-white flex items-center justify-center text-xs">
                                T
                              </span>
                            )}
                            {game.result === "-" && (
                              <span className="text-gray-400">Upcoming</span>
                            )}
                          </div>
                        </div>
                      ))}
                  </div>
                </div>
              </section>

              {/* Add history vs top opponents section */}
              {teamStats?.topOpponents && teamStats.topOpponents.length > 0 && (
                <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
                  <h2 className="text-xl font-semibold mb-4">
                    History vs Top Opponents
                  </h2>
                  <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                    {teamStats.topOpponents.map((opponentData, i) => (
                      <div
                        key={i}
                        className="border rounded-lg p-4 dark:border-gray-600"
                      >
                        <h3 className="font-medium mb-2">
                          {opponentData.opponent}
                        </h3>
                        <div className="flex items-center justify-between mb-2">
                          <span className="text-sm text-gray-500 dark:text-gray-400">
                            Record:
                          </span>
                          <span className="font-medium">
                            {opponentData.record.wins}-
                            {opponentData.record.losses}
                            {opponentData.record.ties > 0
                              ? `-${opponentData.record.ties}`
                              : ""}
                          </span>
                        </div>
                        <div className="flex items-center justify-between">
                          <span className="text-sm text-gray-500 dark:text-gray-400">
                            Win %:
                          </span>
                          <span
                            className={`font-medium ${
                              opponentData.winPercentage >= 50
                                ? "text-green-600 dark:text-green-400"
                                : "text-red-600 dark:text-red-400"
                            }`}
                          >
                            {opponentData.winPercentage.toFixed(1)}%
                          </span>
                        </div>
                      </div>
                    ))}
                  </div>
                </section>
              )}
            </>
          )}

          {/* Players Tab */}
          {activeTab === "players" && (
            <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
              <h2 className="text-xl font-semibold mb-4">Team Roster</h2>
              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-600">
                  <thead className="bg-gray-50 dark:bg-gray-800">
                    <tr>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Position
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Player
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Team
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Points
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Projection
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Status
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
                    {team.players.map((player) => (
                      <tr key={player.id}>
                        <td className="py-4 px-4 whitespace-nowrap">
                          {player.position}
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap font-medium">
                          {player.name}
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          {player.team}
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          {player.points.toFixed(2)}
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          {player.projection > 0
                            ? player.projection.toFixed(2)
                            : "-"}
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          <span
                            className={`inline-flex px-2 py-1 text-xs rounded-full
                              ${
                                player.status === "Active"
                                  ? "bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-100"
                                  : player.status === "IR"
                                  ? "bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-100"
                                  : "bg-yellow-100 text-yellow-800 dark:bg-yellow-800 dark:text-yellow-100"
                              }
                            `}
                          >
                            {player.status}
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
          )}

          {/* Schedule Tab */}
          {activeTab === "schedule" && (
            <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
              <h2 className="text-xl font-semibold mb-4">Team Schedule</h2>

              {/* Add filters */}
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
                    {Array.from(new Set(team.schedule.map((game) => game.year)))
                      .sort((a, b) => b - a) // Sort years in descending order
                      .map((year) => (
                        <option key={`year-${year}`} value={year}>
                          {year}
                        </option>
                      ))}
                  </select>
                </div>

                <div className="w-full md:w-auto">
                  <label
                    htmlFor="opponent-filter"
                    className="block text-sm font-medium text-gray-500 dark:text-gray-400 mb-1"
                  >
                    Filter by Opponent
                  </label>
                  <select
                    id="opponent-filter"
                    className="w-full md:w-auto p-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                    value={opponentFilter}
                    onChange={(e) => setOpponentFilter(e.target.value)}
                  >
                    <option value="all">All Opponents</option>
                    {Array.from(
                      new Set(team.schedule.map((game) => game.opponent))
                    )
                      .sort((a, b) => a.localeCompare(b)) // Sort opponents alphabetically
                      .map((opponent) => (
                        <option key={`opponent-${opponent}`} value={opponent}>
                          {opponent}
                        </option>
                      ))}
                  </select>
                </div>

                {/* Reset button - only show when filters are active */}
                {(yearFilter !== "all" || opponentFilter !== "all") && (
                  <div className="w-full md:w-auto flex items-end">
                    <button
                      onClick={() => {
                        setYearFilter("all");
                        setOpponentFilter("all");
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

              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {team.schedule
                  // Filter games by year
                  .filter(
                    (game) =>
                      yearFilter === "all" ||
                      game.year.toString() === yearFilter
                  )
                  // Filter games by opponent
                  .filter(
                    (game) =>
                      opponentFilter === "all" ||
                      game.opponent === opponentFilter
                  )
                  // Sort by most recent
                  .sort((a, b) => {
                    if (a.year !== b.year) return b.year - a.year; // Most recent year first
                    return b.week - a.week; // Most recent week first
                  })
                  .map((game, i) => (
                    <div
                      key={i}
                      className={`p-4 rounded-lg border ${
                        game.result === "W"
                          ? "border-green-200 bg-green-50 dark:bg-green-900/20 dark:border-green-800"
                          : game.result === "L"
                          ? "border-red-200 bg-red-50 dark:bg-red-900/20 dark:border-red-800"
                          : "border-gray-200 bg-gray-50 dark:bg-gray-800 dark:border-gray-700"
                      }`}
                    >
                      <div className="flex justify-between items-center mb-2">
                        <span className="font-medium">
                          Week {game.week} ({game.year})
                        </span>
                        {game.result !== "-" && (
                          <span
                            className={`w-5 h-5 rounded-full flex items-center justify-center text-xs text-white ${
                              game.result === "W"
                                ? "bg-green-500"
                                : game.result === "L"
                                ? "bg-red-500"
                                : "bg-yellow-500"
                            }`}
                          >
                            {game.result}
                          </span>
                        )}
                      </div>
                      <div className="mb-2">
                        <span className="text-gray-500 dark:text-gray-400">
                          {game.isHome ? "vs" : "@"}
                        </span>
                        <span className="ml-2 font-medium">
                          {game.opponent}
                        </span>
                      </div>
                      <div>
                        {game.result !== "-" ? (
                          <span>{game.score}</span>
                        ) : (
                          <span className="text-gray-500 dark:text-gray-400">
                            Upcoming
                          </span>
                        )}
                      </div>
                    </div>
                  ))}
              </div>
              {/* Show message when no games match filters */}
              {team.schedule
                .filter(
                  (game) =>
                    yearFilter === "all" || game.year.toString() === yearFilter
                )
                .filter(
                  (game) =>
                    opponentFilter === "all" || game.opponent === opponentFilter
                ).length === 0 && (
                <div className="text-center py-6 text-gray-500 dark:text-gray-400">
                  No games match the selected filters.
                </div>
              )}
            </section>
          )}

          {/* Draft Picks Tab */}
          {activeTab === "draft" && (
            <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
              <h2 className="text-xl font-semibold mb-4">Draft Capital</h2>
              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-600">
                  <thead className="bg-gray-50 dark:bg-gray-800">
                    <tr>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Round
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Pick
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Overall
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Player
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
                    {team.draftPicks.map((pick, i) => (
                      <tr
                        key={i}
                        className={
                          i % 2 === 0 ? "" : "bg-gray-50 dark:bg-gray-700"
                        }
                      >
                        <td className="py-4 px-4 whitespace-nowrap">
                          {pick.round}
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          {pick.pick}
                        </td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          {pick.overall}
                        </td>
                        <td className="py-4 px-4">
                          <Link
                            href="#"
                            onClick={(e) => e.preventDefault()}
                            className="text-blue-600 hover:text-blue-800 dark:hover:text-blue-400"
                          >
                            {pick.player} ({pick.position})
                          </Link>
                          <span className="ml-2 text-gray-500">
                            (TODO: Implement player page)
                          </span>
                        </td>
                      </tr>
                    ))}
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
