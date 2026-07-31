import Link from "next/link";
import { GetScheduleResponse } from "@/services/scheduleService";
import { Matchup } from "@/types/models";
import { Team } from "@/services/teamsService";
import { Card, CardContent } from "@/components/ui/card";

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

interface CardTheme {
  accentColor: string;
  ownerColor: string;
  recordColor: string;
  emoji: string;
  title: string;
  rowLabel: string;
}

const THEMES: Record<"fame" | "shame", CardTheme> = {
  fame: {
    accentColor: "var(--status-success-fg)",
    ownerColor: "var(--action-primary)",
    recordColor: "var(--status-success-fg)",
    emoji: "🏆",
    title: "Hall of Fame",
    rowLabel: "Champion",
  },
  shame: {
    accentColor: "var(--status-danger-fg)",
    ownerColor: "var(--status-danger-fg)",
    recordColor: "var(--status-danger-fg)",
    emoji: "💩",
    title: "Wall of Shame",
    rowLabel: "Last Place",
  },
};

interface YearResultCardProps {
  rows: YearResult[];
  isLoading: boolean;
  theme: CardTheme;
  getTeamLink: (ownerName: string) => string | null;
}

function YearResultCard({ rows, isLoading, theme, getTeamLink }: YearResultCardProps) {
  return (
    <Card className="border-l-4" style={{ borderLeftColor: theme.accentColor }}>
      <CardContent className="p-6">
        <h2
          className="text-2xl font-semibold mb-6 flex items-center"
          style={{ color: theme.accentColor }}
        >
          <span className="text-3xl mr-3">{theme.emoji}</span>
          {theme.title}
        </h2>
        <div className="space-y-4">
          {rows.map((row, index) => (
            <div
              key={row.year || index}
              className="p-3 rounded-lg border-l-4 hover:shadow-md transition-shadow"
              style={{
                backgroundColor: "var(--surface-sunken)",
                borderLeftColor: theme.accentColor,
              }}
            >
              <div className="flex justify-between items-center">
                <div>
                  <h3 className="font-semibold text-lg" style={{ color: "var(--text-primary)" }}>
                    {isLoading ? "Loading..." : `${row.year} ${theme.rowLabel}`}
                  </h3>
                  {isLoading ? (
                    <p className="font-medium" style={{ color: theme.ownerColor }}>
                      {row.owner}
                    </p>
                  ) : getTeamLink(row.owner) ? (
                    <Link
                      href={getTeamLink(row.owner)!}
                      className="font-medium hover:underline"
                      style={{ color: theme.ownerColor }}
                    >
                      {row.owner}
                    </Link>
                  ) : (
                    <p className="font-medium" style={{ color: theme.ownerColor }}>
                      {row.owner}
                    </p>
                  )}
                </div>
                {!isLoading && (
                  <div className="text-right">
                    <div className="text-sm font-medium" style={{ color: theme.recordColor }}>
                      {row.record}
                    </div>
                    <div className="text-xs" style={{ color: "var(--text-muted)" }}>
                      {row.points.toLocaleString()} pts
                    </div>
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

export default function HallOfFameWallOfShame({ leagueId, schedule, isLoading, teams }: Props) {
  const { winners, losers } =
    !isLoading && schedule
      ? calculateWinnersAndLosers(schedule)
      : { winners: PLACEHOLDER, losers: PLACEHOLDER };

  const getTeamLink = (ownerName: string) => {
    const team = teams?.find((t) => t.owner === ownerName);
    return team ? `/league/${leagueId}/teams/${team.espnId}` : null;
  };

  return (
    <section className="py-6">
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        <YearResultCard
          rows={isLoading ? PLACEHOLDER : winners}
          isLoading={isLoading}
          theme={THEMES.fame}
          getTeamLink={getTeamLink}
        />
        <YearResultCard
          rows={isLoading ? PLACEHOLDER : losers}
          isLoading={isLoading}
          theme={THEMES.shame}
          getTeamLink={getTeamLink}
        />
      </div>
    </section>
  );
}
