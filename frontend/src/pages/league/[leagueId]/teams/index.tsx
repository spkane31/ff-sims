import { useState, useMemo } from "react";
import { useRouter } from "next/router";
import Link from "next/link";
import { useTeams } from "@/hooks/useTeams";
import { useSchedule } from "@/hooks/useSchedule";
import AllTimeMatchupsGrid from "@/components/AllTimeMatchupsGrid";
import type { Team } from "@/services/teamsService";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import DataTable, {
  type DataTableColumn,
} from "@/components/design-system/DataTable";
import ErrorState from "@/components/design-system/ErrorState";

type SortField =
  | "rank"
  | "name"
  | "wins"
  | "losses"
  | "pf"
  | "pa"
  | "playoffs"
  | "diff";
type SortDirection = "asc" | "desc";

function formatAvgTotal(avg: number, total: number): string {
  return `${avg.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })} (${total.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })})`;
}

// Diff column formatting matches this page's original logic exactly: the
// sign prefix is only ever "+" for positive values (never "-" for negative)
// and the numeric magnitude is shown via Math.abs — a negative differential
// renders as magnitude-only, indistinguishable in sign from zero.
function formatSignedDiff(avg: number, total: number): string {
  const avgSign = avg > 0 ? "+" : "";
  const totalSign = total > 0 ? "+" : "";
  return `${avgSign}${Math.abs(avg).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })} (${totalSign}${Math.abs(total).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })})`;
}

function playoffBarColor(chance: number): string {
  if (chance > 75) return "var(--status-success-fg)";
  if (chance > 50) return "var(--action-primary)";
  if (chance > 25) return "var(--status-warning-fg)";
  return "var(--status-danger-fg)";
}

export default function Teams() {
  const router = useRouter();
  const leagueId = Number(router.query.leagueId);
  const { teams, isLoading, error } = useTeams(leagueId);
  const {
    schedule,
    isLoading: isScheduleLoading,
    error: scheduleError,
  } = useSchedule(leagueId);
  const [sortField, setSortField] = useState<SortField>("rank");
  const [sortDirection, setSortDirection] = useState<SortDirection>("asc");

  // Calculate head-to-head records between active teams (games vs hidden teams
  // are excluded from the grid since those teams aren't in the API response)
  const headToHeadRecords = useMemo(() => {
    if (!schedule?.data?.matchups || !teams) {
      return new Map<string, Map<string, { wins: number; losses: number }>>();
    }

    // Create a map: teamESPNId -> (opponentESPNId -> {wins, losses})
    const records = new Map<
      string,
      Map<string, { wins: number; losses: number }>
    >();

    // Initialize records for all active teams
    teams.forEach((team) => {
      records.set(team.espnId, new Map());
    });

    // Process all completed matchups (including playoffs)
    schedule.data.matchups.forEach((matchup) => {
      if (matchup.homeScore > 0 || matchup.awayScore > 0) {
        const homeId = matchup.homeTeamESPNID.toString();
        const awayId = matchup.awayTeamESPNID.toString();

        // Skip if either team is not an active (non-hidden) team
        if (!records.has(homeId) || !records.has(awayId)) {
          return;
        }

        // Initialize records if they don't exist
        if (!records.get(homeId)?.has(awayId)) {
          records.get(homeId)?.set(awayId, { wins: 0, losses: 0 });
        }
        if (!records.get(awayId)?.has(homeId)) {
          records.get(awayId)?.set(homeId, { wins: 0, losses: 0 });
        }

        // Determine winner and update records
        if (matchup.homeScore > matchup.awayScore) {
          // Home team won
          const homeRecord = records.get(homeId)?.get(awayId);
          const awayRecord = records.get(awayId)?.get(homeId);
          if (homeRecord) homeRecord.wins++;
          if (awayRecord) awayRecord.losses++;
        } else if (matchup.awayScore > matchup.homeScore) {
          // Away team won
          const homeRecord = records.get(homeId)?.get(awayId);
          const awayRecord = records.get(awayId)?.get(homeId);
          if (homeRecord) homeRecord.losses++;
          if (awayRecord) awayRecord.wins++;
        }
      }
    });

    return records;
  }, [schedule, teams]);

  // Calculate league statistics from schedule data
  const leagueStats = useMemo(() => {
    if (!schedule?.data?.matchups || !teams) {
      return {
        highestScore: { score: 0, teamName: "", week: 0 },
        closestMatchup: {
          homeTeam: "",
          awayTeam: "",
          homeScore: 0,
          awayScore: 0,
          week: 0,
          margin: 0,
        },
        biggestBlowout: {
          winner: "",
          loser: "",
          winnerScore: 0,
          loserScore: 0,
          week: 0,
          margin: 0,
        },
        averageScore: 0,
        totalGames: 0,
        completedGames: 0,
      };
    }

    const completedMatchups = schedule.data.matchups.filter(
      (matchup) => matchup.homeScore > 0 && matchup.awayScore > 0
    );
    let highestScore = { score: 0, teamName: "", week: 0 };
    let closestMatchup = {
      homeTeam: "",
      awayTeam: "",
      homeScore: 0,
      awayScore: 0,
      week: 0,
      margin: Infinity,
    };
    let biggestBlowout = {
      winner: "",
      loser: "",
      winnerScore: 0,
      loserScore: 0,
      week: 0,
      margin: 0,
    };
    let totalPoints = 0;
    let totalScores = 0;

    completedMatchups.forEach((matchup) => {
      const homeScore = matchup.homeScore;
      const awayScore = matchup.awayScore;
      const margin = Math.abs(homeScore - awayScore);

      totalPoints += homeScore + awayScore;
      totalScores += 2;

      // Check for highest score
      if (homeScore > highestScore.score) {
        highestScore = {
          score: homeScore,
          teamName: matchup.homeTeamName,
          week: matchup.week,
        };
      }
      if (awayScore > highestScore.score) {
        highestScore = {
          score: awayScore,
          teamName: matchup.awayTeamName,
          week: matchup.week,
        };
      }

      // Check for closest matchup
      if (margin < closestMatchup.margin) {
        closestMatchup = {
          homeTeam: matchup.homeTeamName,
          awayTeam: matchup.awayTeamName,
          homeScore: homeScore,
          awayScore: awayScore,
          week: matchup.week,
          margin: margin,
        };
      }

      // Check for biggest blowout
      if (margin > biggestBlowout.margin) {
        const winner =
          homeScore > awayScore ? matchup.homeTeamName : matchup.awayTeamName;
        const loser =
          homeScore > awayScore ? matchup.awayTeamName : matchup.homeTeamName;
        const winnerScore = Math.max(homeScore, awayScore);
        const loserScore = Math.min(homeScore, awayScore);

        biggestBlowout = {
          winner,
          loser,
          winnerScore,
          loserScore,
          week: matchup.week,
          margin,
        };
      }
    });

    const averageScore = totalScores > 0 ? totalPoints / totalScores : 0;

    return {
      highestScore,
      closestMatchup,
      biggestBlowout,
      averageScore,
      totalGames: schedule.data.matchups.length,
      completedGames: completedMatchups.length,
    };
  }, [schedule, teams]);

  const handleSort = (field: string) => {
    const f = field as SortField;
    if (f === sortField) {
      setSortDirection(sortDirection === "asc" ? "desc" : "asc");
    } else {
      setSortField(f);
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
            case "diff":
              fieldA = a.points.scored - a.points.against;
              fieldB = b.points.scored - b.points.against;
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

  const standingsColumns: DataTableColumn<Team>[] = [
    { id: "rank", header: "Rank", sortable: true, cell: (team) => team.rank },
    {
      id: "name",
      header: "Team",
      sortable: true,
      cell: (team) => (
        <div className="flex flex-col">
          <Link
            href={`/league/${leagueId}/teams/${team.espnId}`}
            className="font-medium hover:underline"
            style={{ color: "var(--text-primary)" }}
          >
            {team.name}
          </Link>
          <span className="text-xs" style={{ color: "var(--text-muted)" }}>
            {team.owner}
          </span>
        </div>
      ),
    },
    {
      id: "wins",
      header: "W",
      sortable: true,
      cell: (team) => team.record.wins,
    },
    {
      id: "losses",
      header: "L",
      sortable: true,
      cell: (team) => team.record.losses,
    },
    {
      id: "pf",
      header: "PF / G",
      sortable: true,
      cell: (team) => {
        const totalGames =
          team.record.wins + team.record.losses + team.record.ties;
        const avgPF = totalGames > 0 ? team.points.scored / totalGames : 0;
        return formatAvgTotal(avgPF, team.points.scored);
      },
    },
    {
      id: "pa",
      header: "PA / G",
      sortable: true,
      cell: (team) => {
        const totalGames =
          team.record.wins + team.record.losses + team.record.ties;
        const avgPA = totalGames > 0 ? team.points.against / totalGames : 0;
        return formatAvgTotal(avgPA, team.points.against);
      },
    },
    {
      id: "diff",
      header: "Diff",
      sortable: true,
      cell: (team) => {
        const totalGames =
          team.record.wins + team.record.losses + team.record.ties;
        const totalDiff = team.points.scored - team.points.against;
        const avgDiff = totalGames > 0 ? totalDiff / totalGames : 0;
        return formatSignedDiff(avgDiff, totalDiff);
      },
    },
    {
      id: "playoffs",
      header: "Playoff %",
      sortable: true,
      cell: (team) => (
        <div>
          <div
            className="h-2.5 w-full rounded-full"
            style={{ backgroundColor: "var(--surface-sunken)" }}
          >
            <div
              className="h-2.5 rounded-full"
              style={{
                width: `${team.playoffChance}%`,
                backgroundColor: playoffBarColor(team.playoffChance),
              }}
            />
          </div>
          <span
            className="mt-1 block text-xs"
            style={{ color: "var(--text-muted)" }}
          >
            {team.playoffChance}%
          </span>
        </div>
      ),
    },
  ];

  if (error || scheduleError) {
    console.error("Error loading data:", {
      error,
      scheduleError,
    });
    return (
      <ErrorState
        message={[error?.message, scheduleError?.message]
          .filter(Boolean)
          .join(" ")}
      />
    );
  }

  return (
    <div className="space-y-8">
      <section>
        <h1
          className="text-3xl md:text-4xl font-bold mb-6"
          style={{ color: "var(--text-primary)" }}
        >
          Fantasy Teams
        </h1>
        <p
          className="text-lg mb-8 max-w-3xl"
          style={{ color: "var(--text-muted)" }}
        >
          View all teams in your league, their records, and key statistics.
        </p>

        <Card>
          <CardContent className="p-6">
            <h2
              className="text-xl font-semibold mb-6"
              style={{ color: "var(--text-primary)" }}
            >
              Standings
            </h2>

            {isLoading ? (
              <div className="space-y-2">
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-10 w-full" />
              </div>
            ) : (
              <DataTable
                columns={standingsColumns}
                rows={sortedTeams}
                rowKey={(team) => team.id}
                sortField={sortField}
                sortDirection={sortDirection}
                onSort={handleSort}
              />
            )}
          </CardContent>
        </Card>
      </section>

      <section>
        <AllTimeMatchupsGrid
          teams={teams}
          headToHeadRecords={headToHeadRecords}
        />
      </section>

      <section className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <Card>
          <CardContent className="p-6">
            <h2
              className="text-lg font-semibold mb-4"
              style={{ color: "var(--text-primary)" }}
            >
              League Leaders
            </h2>
            <div className="space-y-4">
              <div>
                <h3
                  className="text-sm font-medium mb-2"
                  style={{ color: "var(--text-muted)" }}
                >
                  Most Points Scored
                </h3>
                <div className="overflow-hidden">
                  {isLoading ? (
                    <Skeleton className="h-6 w-full" />
                  ) : teams && teams.length > 0 ? (
                    [...teams]
                      .sort((a, b) => b.points.scored - a.points.scored)
                      .slice(0, 3)
                      .map((team) => (
                        <div
                          key={`pf-${team.id}`}
                          className="flex justify-between items-center py-2"
                        >
                          <Link
                            href={`/league/${leagueId}/teams/${team.espnId}`}
                            className="font-medium hover:underline"
                            style={{ color: "var(--text-primary)" }}
                          >
                            {team.owner}
                          </Link>
                          <span style={{ color: "var(--action-primary)" }}>
                            {team.points.scored.toLocaleString(undefined, {
                              minimumFractionDigits: 1,
                              maximumFractionDigits: 1,
                            })}{" "}
                            pts
                          </span>
                        </div>
                      ))
                  ) : (
                    <p
                      className="py-2 text-sm"
                      style={{ color: "var(--text-muted)" }}
                    >
                      No data available
                    </p>
                  )}
                </div>
              </div>

              <div>
                <h3
                  className="text-sm font-medium mb-2"
                  style={{ color: "var(--text-muted)" }}
                >
                  Most Points Against
                </h3>
                <div className="overflow-hidden">
                  {isLoading ? (
                    <Skeleton className="h-6 w-full" />
                  ) : teams && teams.length > 0 ? (
                    [...teams]
                      .sort((a, b) => b.points.against - a.points.against)
                      .slice(0, 3)
                      .map((team) => (
                        <div
                          key={`pa-${team.id}`}
                          className="flex justify-between items-center py-2"
                        >
                          <Link
                            href={`/league/${leagueId}/teams/${team.espnId}`}
                            className="font-medium hover:underline"
                            style={{ color: "var(--text-primary)" }}
                          >
                            {team.owner}
                          </Link>
                          <span style={{ color: "var(--status-danger-fg)" }}>
                            {team.points.against.toLocaleString(undefined, {
                              minimumFractionDigits: 1,
                              maximumFractionDigits: 1,
                            })}{" "}
                            pts
                          </span>
                        </div>
                      ))
                  ) : (
                    <p
                      className="py-2 text-sm"
                      style={{ color: "var(--text-muted)" }}
                    >
                      No data available
                    </p>
                  )}
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-6">
            <h2
              className="text-lg font-semibold mb-4"
              style={{ color: "var(--text-primary)" }}
            >
              League Summary
            </h2>
            <div className="space-y-4">
              <div>
                <span
                  className="block text-sm font-medium mb-1"
                  style={{ color: "var(--text-muted)" }}
                >
                  Average Score
                </span>
                <span
                  className="text-2xl font-bold"
                  style={{ color: "var(--text-primary)" }}
                >
                  {isScheduleLoading
                    ? "..."
                    : leagueStats.averageScore > 0
                    ? leagueStats.averageScore.toFixed(1)
                    : "0.0"}{" "}
                  pts
                </span>
                <span
                  className="text-sm ml-2"
                  style={{ color: "var(--text-muted)" }}
                >
                  per team/week
                </span>
              </div>

              <div>
                <span
                  className="block text-sm font-medium mb-1"
                  style={{ color: "var(--text-muted)" }}
                >
                  Highest Score
                </span>
                <span
                  className="text-2xl font-bold"
                  style={{ color: "var(--text-primary)" }}
                >
                  {isScheduleLoading
                    ? "..."
                    : leagueStats.highestScore.score > 0
                    ? leagueStats.highestScore.score.toFixed(1)
                    : "0"}{" "}
                  pts
                </span>
                <span
                  className="text-sm ml-2"
                  style={{ color: "var(--text-muted)" }}
                >
                  {!isScheduleLoading &&
                  leagueStats.highestScore.teamName &&
                  leagueStats.highestScore.week > 0
                    ? `${leagueStats.highestScore.teamName}, Week ${leagueStats.highestScore.week}`
                    : "No data"}
                </span>
              </div>

              <div>
                <span
                  className="block text-sm font-medium mb-1"
                  style={{ color: "var(--text-muted)" }}
                >
                  Closest Matchup
                </span>
                <span
                  className="text-2xl font-bold"
                  style={{ color: "var(--text-primary)" }}
                >
                  {isScheduleLoading
                    ? "..."
                    : leagueStats.closestMatchup.margin < Infinity
                    ? `${leagueStats.closestMatchup.homeScore.toFixed(
                        2
                      )}-${leagueStats.closestMatchup.awayScore.toFixed(2)}`
                    : "None"}
                </span>
                <span
                  className="text-sm ml-2"
                  style={{ color: "var(--text-muted)" }}
                >
                  {!isScheduleLoading &&
                  leagueStats.closestMatchup.margin < Infinity
                    ? `${leagueStats.closestMatchup.homeTeam} vs ${leagueStats.closestMatchup.awayTeam}, Week ${leagueStats.closestMatchup.week}`
                    : "No matchups"}
                </span>
              </div>

              <div>
                <span
                  className="block text-sm font-medium mb-1"
                  style={{ color: "var(--text-muted)" }}
                >
                  Biggest Blowout
                </span>
                <span
                  className="text-2xl font-bold"
                  style={{ color: "var(--text-primary)" }}
                >
                  {isScheduleLoading
                    ? "..."
                    : leagueStats.biggestBlowout.margin > 0
                    ? `${leagueStats.biggestBlowout.winnerScore.toFixed(
                        1
                      )}-${leagueStats.biggestBlowout.loserScore.toFixed(1)}`
                    : "None"}
                </span>
                <span
                  className="text-sm ml-2"
                  style={{ color: "var(--text-muted)" }}
                >
                  {!isScheduleLoading && leagueStats.biggestBlowout.margin > 0
                    ? `${leagueStats.biggestBlowout.winner} vs ${leagueStats.biggestBlowout.loser}, Week ${leagueStats.biggestBlowout.week}`
                    : "No games"}
                </span>
                <div className="mt-2">
                  <span
                    className="text-xs"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Margin:{" "}
                    {leagueStats.biggestBlowout.margin > 0
                      ? `${leagueStats.biggestBlowout.margin.toFixed(1)} pts`
                      : "0 pts"}
                  </span>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      </section>
    </div>
  );
}
