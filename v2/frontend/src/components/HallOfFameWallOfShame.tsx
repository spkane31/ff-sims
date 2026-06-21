import Link from "next/link";
import { GetScheduleResponse } from "@/services/scheduleService";
import { Matchup } from "@/types/models";
import { Team } from "@/services/teamsService";

interface Props {
  leagueId: number;
  schedule: GetScheduleResponse | null;
  isLoading: boolean;
  teams?: Team[];
}

interface YearResult {
  year: number;
  owner: string;
  record: string;
  points: number;
}

function calculateWinnersAndLosers(
  schedule: GetScheduleResponse
): { winners: YearResult[]; losers: YearResult[] } {
  const matchups: Matchup[] = schedule.data.matchups;

  const currentYear = new Date().getFullYear();
  const years = Array.from(new Set(matchups.map((g) => g.year))).sort(
    (a, b) => b - a
  );

  const completedYears = years.filter((year) => {
    if (year >= currentYear) return false;
    const regularGames = matchups.filter(
      (g) => g.year === year && g.gameType === "NONE"
    );
    return regularGames.length > 0 && regularGames.every((g) => g.homeScore > 0 || g.awayScore > 0);
  });

  const winners: YearResult[] = completedYears
    .map((year) => {
      const yearGames = matchups.filter(
        (g) => g.year === year && (g.homeScore > 0 || g.awayScore > 0)
      );

      const playoffWins: Record<string, { wins: number; owner: string }> = {};

      yearGames
        .filter((g) => g.gameType === "WINNERS_BRACKET")
        .forEach((g) => {
          if (g.homeScore > g.awayScore) {
            const k = g.homeTeamName;
            if (!playoffWins[k])
              playoffWins[k] = { wins: 0, owner: g.homeTeam?.owner_name || k };
            playoffWins[k].wins++;
          } else if (g.awayScore > g.homeScore) {
            const k = g.awayTeamName;
            if (!playoffWins[k])
              playoffWins[k] = { wins: 0, owner: g.awayTeam?.owner_name || k };
            playoffWins[k].wins++;
          }
        });

      if (Object.keys(playoffWins).length === 0) return null;

      const winner = Object.entries(playoffWins).reduce(
        (max, [team, s]) => (s.wins > max.wins ? { team, ...s } : max),
        { team: "", wins: 0, owner: "" }
      );

      const teamStats: Record<string, { wins: number; losses: number; points: number }> = {};
      yearGames
        .filter((g) => g.gameType === "NONE")
        .forEach((g) => {
          if (!teamStats[g.homeTeamName])
            teamStats[g.homeTeamName] = { wins: 0, losses: 0, points: 0 };
          if (!teamStats[g.awayTeamName])
            teamStats[g.awayTeamName] = { wins: 0, losses: 0, points: 0 };
          teamStats[g.homeTeamName].points += g.homeScore;
          teamStats[g.awayTeamName].points += g.awayScore;
          if (g.homeScore > g.awayScore) {
            teamStats[g.homeTeamName].wins++;
            teamStats[g.awayTeamName].losses++;
          } else if (g.awayScore > g.homeScore) {
            teamStats[g.awayTeamName].wins++;
            teamStats[g.homeTeamName].losses++;
          }
        });

      const ws = teamStats[winner.team] || { wins: 0, losses: 0, points: 0 };
      return { year, owner: winner.owner, record: `${ws.wins}-${ws.losses}`, points: ws.points };
    })
    .filter((w): w is YearResult => !!w && !!w.owner);

  const losers: YearResult[] = completedYears
    .map((year) => {
      const yearGames = matchups.filter(
        (g) => g.year === year && (g.homeScore > 0 || g.awayScore > 0)
      );

      const teamStats: Record<string, { wins: number; losses: number; points: number; owner: string }> = {};
      yearGames
        .filter((g) => g.gameType === "NONE")
        .forEach((g) => {
          if (!teamStats[g.homeTeamName])
            teamStats[g.homeTeamName] = { wins: 0, losses: 0, points: 0, owner: g.homeTeam?.owner_name || g.homeTeamName };
          if (!teamStats[g.awayTeamName])
            teamStats[g.awayTeamName] = { wins: 0, losses: 0, points: 0, owner: g.awayTeam?.owner_name || g.awayTeamName };
          teamStats[g.homeTeamName].points += g.homeScore;
          teamStats[g.awayTeamName].points += g.awayScore;
          if (g.homeScore > g.awayScore) {
            teamStats[g.homeTeamName].wins++;
            teamStats[g.awayTeamName].losses++;
          } else if (g.awayScore > g.homeScore) {
            teamStats[g.awayTeamName].wins++;
            teamStats[g.homeTeamName].losses++;
          }
        });

      if (Object.keys(teamStats).length === 0) return null;

      const loser = Object.entries(teamStats).reduce(
        (max, [team, s]) => {
          if (s.losses > max.losses || (s.losses === max.losses && s.points < max.points))
            return { team, ...s };
          return max;
        },
        { team: "", wins: 0, losses: 0, points: 0, owner: "" }
      );

      return { year, owner: loser.owner, record: `${loser.wins}-${loser.losses}`, points: loser.points };
    })
    .filter((l): l is YearResult => !!l && !!l.owner);

  return { winners, losers };
}

const PLACEHOLDER: YearResult[] = [{ year: 0, owner: "Loading...", record: "0-0", points: 0 }];

export default function HallOfFameWallOfShame({ leagueId, schedule, isLoading, teams }: Props) {
  const { winners, losers } =
    !isLoading && schedule
      ? calculateWinnersAndLosers(schedule)
      : { winners: PLACEHOLDER, losers: PLACEHOLDER };

  const champRows = isLoading ? PLACEHOLDER : winners;
  const loseRows = isLoading ? PLACEHOLDER : losers;

  const getTeamLink = (ownerName: string) => {
    const team = teams?.find((t) => t.owner === ownerName);
    return team ? `/league/${leagueId}/teams/${team.espnId}` : null;
  };

  return (
    <section className="py-6">
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        {/* Hall of Fame */}
        <div className="bg-gradient-to-br from-yellow-50 to-yellow-100 dark:from-yellow-900/20 dark:to-yellow-800/20 p-6 rounded-lg shadow-md">
          <h2 className="text-2xl font-semibold mb-6 text-yellow-800 dark:text-yellow-200 flex items-center">
            <span className="text-3xl mr-3">🏆</span>
            Hall of Fame
          </h2>
          <div className="space-y-4">
            {champRows.map((champion, index) => (
              <div
                key={champion.year || index}
                className={`bg-white dark:bg-gray-800 p-3 rounded-lg shadow-sm border-l-4 ${
                  index === 0 ? "border-yellow-500" : "border-yellow-300"
                } hover:shadow-md transition-shadow`}
              >
                <div className="flex justify-between items-center">
                  <div>
                    <h3 className="font-semibold text-lg text-gray-900 dark:text-gray-100">
                      {isLoading ? "Loading..." : `${champion.year} Champion`}
                    </h3>
                    {isLoading ? (
                      <p className="text-blue-600 dark:text-blue-400 font-medium">{champion.owner}</p>
                    ) : getTeamLink(champion.owner) ? (
                      <Link
                        href={getTeamLink(champion.owner)!}
                        className="text-blue-600 dark:text-blue-400 font-medium hover:underline"
                      >
                        {champion.owner}
                      </Link>
                    ) : (
                      <p className="text-blue-600 dark:text-blue-400 font-medium">{champion.owner}</p>
                    )}
                  </div>
                  {!isLoading && (
                    <div className="text-right">
                      <div className="text-sm font-medium text-green-600 dark:text-green-400">
                        {champion.record}
                      </div>
                      <div className="text-xs text-gray-500 dark:text-gray-400">
                        {champion.points.toLocaleString()} pts
                      </div>
                    </div>
                  )}
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
            {loseRows.map((lastPlace, index) => (
              <div
                key={lastPlace.year || index}
                className={`bg-white dark:bg-gray-800 p-3 rounded-lg shadow-sm border-l-4 ${
                  index === 0 ? "border-red-500" : "border-red-300"
                } hover:shadow-md transition-shadow`}
              >
                <div className="flex justify-between items-center">
                  <div>
                    <h3 className="font-semibold text-lg text-gray-900 dark:text-gray-100">
                      {isLoading ? "Loading..." : `${lastPlace.year} Last Place`}
                    </h3>
                    {isLoading ? (
                      <p className="text-red-600 dark:text-red-400 font-medium">{lastPlace.owner}</p>
                    ) : getTeamLink(lastPlace.owner) ? (
                      <Link
                        href={getTeamLink(lastPlace.owner)!}
                        className="text-red-600 dark:text-red-400 font-medium hover:underline"
                      >
                        {lastPlace.owner}
                      </Link>
                    ) : (
                      <p className="text-red-600 dark:text-red-400 font-medium">{lastPlace.owner}</p>
                    )}
                  </div>
                  {!isLoading && (
                    <div className="text-right">
                      <div className="text-sm font-medium text-red-600 dark:text-red-400">
                        {lastPlace.record}
                      </div>
                      <div className="text-xs text-gray-500 dark:text-gray-400">
                        {lastPlace.points.toLocaleString()} pts
                      </div>
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
