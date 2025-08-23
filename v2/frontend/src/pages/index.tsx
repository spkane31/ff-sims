import { useEffect, useState } from "react";
import Layout from "../components/Layout";
import Link from "next/link";
import { teamsService, Team } from "../services/teamsService";
import { useSchedule } from "../hooks/useSchedule";
import { Matchup } from "@/types/models";

type SortField =
  | "owner"
  | "regularSeasonRecord"
  | "playoffRecord"
  | "pointsFor"
  | "pointsAgainst";
type SortDirection = "asc" | "desc";

export default function Home() {
  const [teams, setTeams] = useState<Team[]>([]);
  const [teamsLoading, setTeamsLoading] = useState(true);
  const [sortField, setSortField] = useState<SortField>("regularSeasonRecord");
  const [sortDirection, setSortDirection] = useState<SortDirection>("desc");
  const { schedule, isLoading: scheduleLoading } = useSchedule();

  useEffect(() => {
    async function fetchTeamsData() {
      try {
        setTeamsLoading(true);
        const response = await teamsService.getAllTeams();
        setTeams(response.teams);
      } catch (error) {
        console.error("Error fetching teams data:", error);
      } finally {
        setTeamsLoading(false);
      }
    }

    fetchTeamsData();
  }, []);

  const handleSort = (field: SortField) => {
    if (field === sortField) {
      setSortDirection(sortDirection === "asc" ? "desc" : "asc");
    } else {
      setSortField(field);
      setSortDirection("desc"); // Default to descending for most fields
    }
  };

  const sortedTeams = [...teams]
    .filter((team) => team.espnId !== "2" && team.espnId !== "8")
    .sort((a, b) => {
      let fieldA: string | number;
      let fieldB: string | number;

      switch (sortField) {
        case "owner":
          fieldA = a.owner.toLowerCase();
          fieldB = b.owner.toLowerCase();
          break;
        case "regularSeasonRecord":
          // Sort by wins, then by win percentage
          fieldA = a.record.wins;
          fieldB = b.record.wins;
          if (fieldA === fieldB) {
            // If wins are equal, sort by win percentage
            const totalGamesA = a.record.wins + a.record.losses + a.record.ties;
            const totalGamesB = b.record.wins + b.record.losses + b.record.ties;
            const winPctA = totalGamesA > 0 ? a.record.wins / totalGamesA : 0;
            const winPctB = totalGamesB > 0 ? b.record.wins / totalGamesB : 0;
            fieldA = winPctA;
            fieldB = winPctB;
          }
          break;
        case "playoffRecord":
          // Sort by playoff wins, then by playoff win percentage
          fieldA = a.playoffRecord.wins;
          fieldB = b.playoffRecord.wins;
          if (fieldA === fieldB) {
            const totalGamesA =
              a.playoffRecord.wins +
              a.playoffRecord.losses +
              a.playoffRecord.ties;
            const totalGamesB =
              b.playoffRecord.wins +
              b.playoffRecord.losses +
              b.playoffRecord.ties;
            const winPctA =
              totalGamesA > 0 ? a.playoffRecord.wins / totalGamesA : 0;
            const winPctB =
              totalGamesB > 0 ? b.playoffRecord.wins / totalGamesB : 0;
            fieldA = winPctA;
            fieldB = winPctB;
          }
          break;
        case "pointsFor":
          fieldA = a.points.scored;
          fieldB = b.points.scored;
          break;
        case "pointsAgainst":
          fieldA = a.points.against;
          fieldB = b.points.against;
          break;
        default:
          fieldA = a.owner.toLowerCase();
          fieldB = b.owner.toLowerCase();
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

  // Calculate winners and losers from schedule data
  const calculateWinnersAndLosers = () => {
    if (scheduleLoading || !schedule?.data?.matchups) {
      return { winners: [], losers: [] };
    }

    const scheduleData: Matchup[] = schedule.data.matchups
      .flat()
      .map((game) => ({
        leagueId: 1,
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
        isPlayoff: game.isPlayoff || false,
      }));

    const years = Array.from(
      new Set(scheduleData.map((game) => game.year))
    ).sort((a, b) => b - a);

    const winners = years
      .map((year) => {
        const yearGames = scheduleData.filter(
          (game) => game.year === year && game.completed
        );
        const playoffGames = yearGames.filter((game) => {
          if (game.gameType === "WINNERS_BRACKET") return true;

          // Check if this is a third place game (WINNERS_CONSOLATION_LADDER in last week between semifinal losers)
          if (game.gameType === "WINNERS_CONSOLATION_LADDER") {
            const lastWeek = Math.max(...yearGames.map((g) => g.week));
            if (game.week !== lastWeek) return false;

            const secondToLastWeek = lastWeek - 1;
            const semifinalGames = yearGames.filter(
              (g) =>
                g.gameType === "WINNERS_BRACKET" && g.week === secondToLastWeek
            );

            if (semifinalGames.length === 0) return false;

            // Get the losers from the semifinal games
            const semifinalLosers: number[] = [];
            semifinalGames.forEach((semifinal) => {
              if (semifinal.homeScore > semifinal.awayScore) {
                // Away team lost
                semifinalLosers.push(semifinal.awayTeamId);
              } else if (semifinal.awayScore > semifinal.homeScore) {
                // Home team lost
                semifinalLosers.push(semifinal.homeTeamId);
              }
              // If tied, we can't determine a loser, so skip
            });

            // Check if both teams in the third place game are semifinal losers
            const gameTeams = [game.homeTeamId, game.awayTeamId];
            return (
              gameTeams.every((teamId) => semifinalLosers.includes(teamId)) &&
              semifinalLosers.length >= 2
            );
          }

          return false;
        });

        // Count playoff wins per team
        const playoffWins: Record<
          string,
          { wins: number; owner: string; totalPoints: number }
        > = {};

        playoffGames.forEach((game) => {
          const homeWin = game.homeScore > game.awayScore;
          const awayWin = game.awayScore > game.homeScore;

          if (homeWin) {
            const key = game.homeTeamName;
            if (!playoffWins[key])
              playoffWins[key] = {
                wins: 0,
                owner: game.homeTeam?.owner_name || key,
                totalPoints: 0,
              };
            playoffWins[key].wins++;
          }
          if (awayWin) {
            const key = game.awayTeamName;
            if (!playoffWins[key])
              playoffWins[key] = {
                wins: 0,
                owner: game.awayTeam?.owner_name || key,
                totalPoints: 0,
              };
            playoffWins[key].wins++;
          }
        });

        // Get regular season record and points for winner
        const regularSeasonGames = yearGames.filter(
          (game) => game.gameType === "NONE"
        );
        const teamStats: Record<
          string,
          { wins: number; losses: number; points: number }
        > = {};

        regularSeasonGames.forEach((game) => {
          const homeWin = game.homeScore > game.awayScore;
          const awayWin = game.awayScore > game.homeScore;

          if (!teamStats[game.homeTeamName])
            teamStats[game.homeTeamName] = { wins: 0, losses: 0, points: 0 };
          if (!teamStats[game.awayTeamName])
            teamStats[game.awayTeamName] = { wins: 0, losses: 0, points: 0 };

          teamStats[game.homeTeamName].points += game.homeScore;
          teamStats[game.awayTeamName].points += game.awayScore;

          if (homeWin) {
            teamStats[game.homeTeamName].wins++;
            teamStats[game.awayTeamName].losses++;
          } else if (awayWin) {
            teamStats[game.awayTeamName].wins++;
            teamStats[game.homeTeamName].losses++;
          }
        });

        // Find team with most playoff wins
        const winner = Object.entries(playoffWins).reduce(
          (max, [team, stats]) => {
            return stats.wins > max.wins ? { team, ...stats } : max;
          },
          { team: "", wins: 0, owner: "", totalPoints: 0 }
        );

        const winnerStats = teamStats[winner.team] || {
          wins: 0,
          losses: 0,
          points: 0,
        };

        return {
          year,
          owner: winner.owner,
          record: `${winnerStats.wins}-${winnerStats.losses}`,
          points: winnerStats.points,
        };
      })
      .filter((w) => w.owner);

    const losers = years
      .map((year) => {
        const yearGames = scheduleData.filter(
          (game) => game.year === year && game.completed
        );
        const regularSeasonGames = yearGames.filter(
          (game) => game.gameType === "NONE"
        );

        // Count regular season losses and points per team
        const teamStats: Record<
          string,
          { wins: number; losses: number; points: number; owner: string }
        > = {};

        regularSeasonGames.forEach((game) => {
          const homeWin = game.homeScore > game.awayScore;
          const awayWin = game.awayScore > game.homeScore;

          if (!teamStats[game.homeTeamName]) {
            teamStats[game.homeTeamName] = {
              wins: 0,
              losses: 0,
              points: 0,
              owner: game.homeTeam?.owner_name || game.homeTeamName,
            };
          }
          if (!teamStats[game.awayTeamName]) {
            teamStats[game.awayTeamName] = {
              wins: 0,
              losses: 0,
              points: 0,
              owner: game.awayTeam?.owner_name || game.awayTeamName,
            };
          }

          teamStats[game.homeTeamName].points += game.homeScore;
          teamStats[game.awayTeamName].points += game.awayScore;

          if (homeWin) {
            teamStats[game.homeTeamName].wins++;
            teamStats[game.awayTeamName].losses++;
          } else if (awayWin) {
            teamStats[game.awayTeamName].wins++;
            teamStats[game.homeTeamName].losses++;
          }
        });

        // Find team with most losses (tiebreak by lowest points)
        const loser = Object.entries(teamStats).reduce(
          (max, [team, stats]) => {
            if (
              stats.losses > max.losses ||
              (stats.losses === max.losses && stats.points < max.points)
            ) {
              return { team, ...stats };
            }
            return max;
          },
          { team: "", wins: 0, losses: 0, points: 0, owner: "" }
        );

        return {
          year,
          owner: loser.owner,
          record: `${loser.wins}-${loser.losses}`,
          points: loser.points,
        };
      })
      .filter((l) => l.owner);

    return { winners, losers };
  };

  const { winners, losers } = calculateWinnersAndLosers();

  return (
    <Layout>
      <div className="space-y-8">
        {/* Hero Section */}
        <section className="text-center md:text-left">
          <h1 className="text-4xl md:text-5xl font-bold text-blue-600 mb-4">
            Fantasy Football Simulations
          </h1>
          <p className="text-lg md:text-xl text-gray-600 dark:text-gray-300 max-w-3xl mb-8">
            Analyze, simulate, and optimize your fantasy football strategy with
            data-driven insights.
          </p>
          <div className="flex flex-wrap gap-4 justify-center md:justify-start">
            <Link
              href="/simulations"
              className="bg-blue-600 hover:bg-blue-700 text-white px-6 py-3 rounded-md font-medium transition-colors"
            >
              Run Simulation
            </Link>
            <Link
              href="/teams"
              className="border border-blue-600 text-blue-600 hover:bg-blue-50 dark:hover:bg-blue-900/20 px-6 py-3 rounded-md font-medium transition-colors"
            >
              View Teams
            </Link>
          </div>
        </section>

        {/* Features Section */}
        <section className="py-6">
          <h2 className="text-2xl font-semibold mb-6">Features</h2>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            {[
              {
                title: "Team Simulations",
                description: "Simulate matchups and project season outcomes.",
                link: "/simulations",
              },
              {
                title: "Schedule Analysis",
                description: "Analyze strength of schedule and key matchups.",
                link: "/schedule",
              },
              {
                title: "Team Management",
                description: "View and compare team rosters and performance.",
                link: "/teams",
              },
            ].map((feature, index) => (
              <div
                key={index}
                className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md hover:shadow-lg transition-shadow"
              >
                <h3 className="text-xl font-medium text-blue-600 mb-2">
                  {feature.title}
                </h3>
                <p className="mb-4 text-gray-600 dark:text-gray-300">
                  {feature.description}
                </p>
                <Link
                  href={feature.link}
                  className="text-blue-600 hover:text-blue-800 dark:hover:text-blue-400 font-medium"
                >
                  Learn more →
                </Link>
              </div>
            ))}
          </div>
        </section>

        {/* Hall of Fame & Wall of Shame Section */}
        <section className="py-6">
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
            {/* Hall of Fame */}
            <div className="bg-gradient-to-br from-yellow-50 to-yellow-100 dark:from-yellow-900/20 dark:to-yellow-800/20 p-6 rounded-lg shadow-md">
              <h2 className="text-2xl font-semibold mb-6 text-yellow-800 dark:text-yellow-200 flex items-center">
                <span className="text-3xl mr-3">🏆</span>
                Hall of Fame
              </h2>
              <div className="space-y-4">
                {(scheduleLoading
                  ? [
                      {
                        year: 2023,
                        owner: "Loading...",
                        record: "0-0",
                        points: 0,
                      },
                    ]
                  : winners
                ).map((champion, index) => (
                  <div
                    key={champion.year}
                    className={`bg-white dark:bg-gray-800 p-3 rounded-lg shadow-sm border-l-4 ${
                      index === 0 ? "border-yellow-500" : "border-yellow-300"
                    } hover:shadow-md transition-shadow`}
                  >
                    <div className="flex justify-between items-center">
                      <div>
                        <h3 className="font-semibold text-lg text-gray-900 dark:text-gray-100">
                          {champion.year} Champion
                        </h3>
                        <p className="text-blue-600 dark:text-blue-400 font-medium">
                          {champion.owner}
                        </p>
                      </div>
                      <div className="text-right">
                        <div className="text-sm font-medium text-green-600 dark:text-green-400">
                          {champion.record}
                        </div>
                        <div className="text-xs text-gray-500 dark:text-gray-400">
                          {champion.points.toLocaleString()} pts
                        </div>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>

            {/* Wall of Shame */}
            <div className="bg-gradient-to-br from-red-50 to-red-100 dark:from-red-900/20 dark:to-red-800/20 p-6 rounded-lg shadow-md">
              <h2 className="text-2xl font-semibold mb-6 text-red-800 dark:text-red-200 flex items-center">
                <span className="text-3xl mr-3">💩</span>
                Wall of Shame
              </h2>
              <div className="space-y-4">
                {(scheduleLoading
                  ? [
                      {
                        year: 2023,
                        owner: "Loading...",
                        record: "0-0",
                        points: 0,
                      },
                    ]
                  : losers
                ).map((lastPlace, index) => (
                  <div
                    key={lastPlace.year}
                    className={`bg-white dark:bg-gray-800 p-3 rounded-lg shadow-sm border-l-4 ${
                      index === 0 ? "border-red-500" : "border-red-300"
                    } hover:shadow-md transition-shadow`}
                  >
                    <div className="flex justify-between items-center">
                      <div>
                        <h3 className="font-semibold text-lg text-gray-900 dark:text-gray-100">
                          {lastPlace.year} Last Place
                        </h3>
                        <p className="text-red-600 dark:text-red-400 font-medium">
                          {lastPlace.owner}
                        </p>
                      </div>
                      <div className="text-right">
                        <div className="text-sm font-medium text-red-600 dark:text-red-400">
                          {lastPlace.record}
                        </div>
                        <div className="text-xs text-gray-500 dark:text-gray-400">
                          {lastPlace.points.toLocaleString()} pts
                        </div>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </section>

        {/* Teams Data Section */}
        <section className="bg-gray-100 dark:bg-gray-700 rounded-lg p-6">
          <h2 className="text-xl font-semibold mb-4">All-Time Team Records</h2>
          {teamsLoading ? (
            <div className="flex items-center space-x-2">
              <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
              <p>Loading teams data...</p>
            </div>
          ) : (
            <div>
              {teams.length > 0 ? (
                <div className="overflow-x-auto">
                  <table className="w-full bg-white dark:bg-gray-800 rounded-lg">
                    <thead className="bg-gray-50 dark:bg-gray-600">
                      <tr>
                        <th
                          className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-500 transition-colors"
                          onClick={() => handleSort("owner")}
                        >
                          Owner{renderSortIcon("owner")}
                        </th>
                        <th
                          className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-500 transition-colors"
                          onClick={() => handleSort("regularSeasonRecord")}
                        >
                          Regular Season Record
                          {renderSortIcon("regularSeasonRecord")}
                        </th>
                        <th
                          className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-500 transition-colors"
                          onClick={() => handleSort("playoffRecord")}
                        >
                          Playoffs Record{renderSortIcon("playoffRecord")}
                        </th>
                        <th
                          className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-500 transition-colors"
                          onClick={() => handleSort("pointsFor")}
                        >
                          Points For{renderSortIcon("pointsFor")}
                        </th>
                        <th
                          className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-500 transition-colors"
                          onClick={() => handleSort("pointsAgainst")}
                        >
                          Points Against{renderSortIcon("pointsAgainst")}
                        </th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
                      {sortedTeams.map((team) => (
                        <tr
                          key={team.id}
                          className="hover:bg-gray-50 dark:hover:bg-gray-700"
                        >
                          <td className="px-4 py-3 text-sm">
                            <Link
                              href={`/teams/${team.espnId}`}
                              className="text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 hover:underline transition-colors"
                            >
                              {team.owner}
                            </Link>
                          </td>
                          <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                            {team.record.wins}-{team.record.losses}
                            {team.record.ties > 0 ? `-${team.record.ties}` : ""}
                          </td>
                          <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                            {team.playoffRecord.wins}-
                            {team.playoffRecord.losses}
                            {team.playoffRecord.ties > 0
                              ? `-${team.playoffRecord.ties}`
                              : ""}
                          </td>
                          <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                            {team.points.scored.toLocaleString(undefined, {
                              minimumFractionDigits: 2,
                              maximumFractionDigits: 2,
                            })}
                          </td>
                          <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                            {team.points.against.toLocaleString(undefined, {
                              minimumFractionDigits: 2,
                              maximumFractionDigits: 2,
                            })}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <p className="text-gray-600 dark:text-gray-300">
                  No teams found.
                </p>
              )}
            </div>
          )}
        </section>
      </div>
    </Layout>
  );
}
