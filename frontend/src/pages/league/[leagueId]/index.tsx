import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/router";
import Link from "next/link";
import { useLeague } from "@/hooks/useLeagues";
import { useTeams } from "@/hooks/useTeams";
import { useSchedule } from "@/hooks/useSchedule";
import type { Team } from "@/services/teamsService";
import {
  expectedWinsService,
  type CurrentSeasonStanding,
} from "@/services/expectedWinsService";
import { leaguesService } from "@/services/leaguesService";
import AllTimeMatchupsGrid from "@/components/AllTimeMatchupsGrid";
import HallOfFameWallOfShame from "@/components/HallOfFameWallOfShame";
import AllTimeRecordsTable from "@/components/AllTimeRecordsTable";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import StatCard from "@/components/design-system/StatCard";
import DataTable, { type DataTableColumn } from "@/components/design-system/DataTable";
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

function formatAvgTotal(avg: number, total: number, sign = false): string {
  const avgSign = sign && avg > 0 ? "+" : "";
  const totalSign = sign && total > 0 ? "+" : "";
  return `${avgSign}${avg.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })} (${totalSign}${total.toLocaleString(undefined, {
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

export default function LeagueDashboard() {
  const router = useRouter();
  const { leagueId } = router.query;
  const id = Number(leagueId);

  const { league, isLoading: isLeagueLoading } = useLeague(id || undefined);
  const { teams, isLoading, error } = useTeams(id);
  const {
    schedule,
    isLoading: isScheduleLoading,
    error: scheduleError,
  } = useSchedule(id);
  const [seasonYear, setSeasonYear] = useState<number | null>(null);
  const [seasonStandings, setSeasonStandings] = useState<Team[]>([]);
  const [isStandingsLoading, setIsStandingsLoading] = useState(true);
  const [standingsError, setStandingsError] = useState<Error | null>(null);
  const [sortField, setSortField] = useState<SortField>("rank");
  const [sortDirection, setSortDirection] = useState<SortDirection>("asc");

  useEffect(() => {
    if (!id) return;

    let cancelled = false;

    async function fetchCurrentStandings() {
      try {
        setIsStandingsLoading(true);
        setStandingsError(null);

        const { years } = await leaguesService.getLeagueYears(id);
        const latestYear = years[0];
        if (!latestYear) {
          if (!cancelled) {
            setSeasonYear(null);
            setSeasonStandings([]);
          }
          return;
        }

        const response = await expectedWinsService.getCurrentSeasonStandings(id, latestYear);
        if (cancelled) return;

        setSeasonYear(response.year);
        setSeasonStandings(
          response.standings.map((standing: CurrentSeasonStanding, index) => ({
            id: String(standing.team_id),
            espnId: standing.espn_id,
            name: standing.team_name,
            owner: standing.owner,
            record: standing.record,
            playoffRecord: { wins: 0, losses: 0, ties: 0 },
            points: standing.points,
            rank: index + 1,
            playoffChance: 0,
          }))
        );
      } catch (err) {
        if (!cancelled) {
          setStandingsError(
            err instanceof Error ? err : new Error("Failed to fetch current standings")
          );
        }
      } finally {
        if (!cancelled) setIsStandingsLoading(false);
      }
    }

    fetchCurrentStandings();
    return () => {
      cancelled = true;
    };
  }, [id]);

  const filteredTeams = useMemo(
    () =>
      teams?.filter(
        (team) =>
          !team.owner.includes("Knapp") && !team.owner.includes("Landry")
      ),
    [teams]
  );

  const visibleSeasonStandings = useMemo(
    () =>
      seasonStandings.filter(
        (team) =>
          !team.owner.includes("Knapp") && !team.owner.includes("Landry")
      ),
    [seasonStandings]
  );

  const headToHeadRecords = useMemo(() => {
    if (!schedule?.data?.matchups || !filteredTeams) {
      return new Map<string, Map<string, { wins: number; losses: number }>>();
    }

    const records = new Map<
      string,
      Map<string, { wins: number; losses: number }>
    >();

    filteredTeams.forEach((team) => {
      records.set(team.espnId, new Map());
    });

    schedule.data.matchups.forEach((matchup) => {
      if (matchup.homeScore > 0 || matchup.awayScore > 0) {
        const homeId = matchup.homeTeamESPNID.toString();
        const awayId = matchup.awayTeamESPNID.toString();

        if (!records.has(homeId) || !records.has(awayId)) {
          return;
        }

        if (!records.get(homeId)?.has(awayId)) {
          records.get(homeId)?.set(awayId, { wins: 0, losses: 0 });
        }
        if (!records.get(awayId)?.has(homeId)) {
          records.get(awayId)?.set(homeId, { wins: 0, losses: 0 });
        }

        if (matchup.homeScore > matchup.awayScore) {
          const homeRecord = records.get(homeId)?.get(awayId);
          const awayRecord = records.get(awayId)?.get(homeId);
          if (homeRecord) homeRecord.wins++;
          if (awayRecord) awayRecord.losses++;
        } else if (matchup.awayScore > matchup.homeScore) {
          const homeRecord = records.get(homeId)?.get(awayId);
          const awayRecord = records.get(awayId)?.get(homeId);
          if (homeRecord) homeRecord.losses++;
          if (awayRecord) awayRecord.wins++;
        }
      }
    });

    return records;
  }, [schedule, filteredTeams]);

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

      if (homeScore > highestScore.score) {
        highestScore = { score: homeScore, teamName: matchup.homeTeamName, week: matchup.week };
      }
      if (awayScore > highestScore.score) {
        highestScore = { score: awayScore, teamName: matchup.awayTeamName, week: matchup.week };
      }

      if (margin < closestMatchup.margin) {
        closestMatchup = {
          homeTeam: matchup.homeTeamName,
          awayTeam: matchup.awayTeamName,
          homeScore,
          awayScore,
          week: matchup.week,
          margin,
        };
      }

      if (margin > biggestBlowout.margin) {
        biggestBlowout = {
          winner: homeScore > awayScore ? matchup.homeTeamName : matchup.awayTeamName,
          loser: homeScore > awayScore ? matchup.awayTeamName : matchup.homeTeamName,
          winnerScore: Math.max(homeScore, awayScore),
          loserScore: Math.min(homeScore, awayScore),
          week: matchup.week,
          margin,
        };
      }
    });

    return {
      highestScore,
      closestMatchup,
      biggestBlowout,
      averageScore: totalScores > 0 ? totalPoints / totalScores : 0,
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
    isStandingsLoading
      ? []
      : [...visibleSeasonStandings].sort((a, b) => {
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
            href={`/league/${id}/teams/${team.espnId}`}
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
        const wins = team.record.wins;
        const losses = team.record.losses;
        const totalGames = wins + losses + team.record.ties;
        const avgPF = totalGames > 0 ? team.points.scored / totalGames : 0;
        return formatAvgTotal(avgPF, team.points.scored);
      },
    },
    {
      id: "pa",
      header: "PA / G",
      sortable: true,
      cell: (team) => {
        const wins = team.record.wins;
        const losses = team.record.losses;
        const totalGames = wins + losses + team.record.ties;
        const avgPA = totalGames > 0 ? team.points.against / totalGames : 0;
        return formatAvgTotal(avgPA, team.points.against);
      },
    },
    {
      id: "diff",
      header: "Diff",
      sortable: true,
      cell: (team) => {
        const wins = team.record.wins;
        const losses = team.record.losses;
        const totalGames = wins + losses + team.record.ties;
        const totalDiff = team.points.scored - team.points.against;
        const avgDiff = totalGames > 0 ? totalDiff / totalGames : 0;
        return formatAvgTotal(avgDiff, totalDiff, true);
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
          <span className="mt-1 block text-xs" style={{ color: "var(--text-muted)" }}>
            {team.playoffChance}%
          </span>
        </div>
      ),
    },
  ];

  if (error || scheduleError || standingsError) {
    return (
      <ErrorState
        message={[error?.message, scheduleError?.message, standingsError?.message]
          .filter(Boolean)
          .join(" ")}
      />
    );
  }

  return (
    <div className="space-y-10">
      {/* League header */}
      <section>
        {isLeagueLoading ? (
          <Skeleton className="h-10 w-64" />
        ) : league ? (
          <>
            <h1 className="text-4xl font-bold md:text-5xl" style={{ color: "var(--text-primary)" }}>
              {league.name}
            </h1>
            <p className="text-sm" style={{ color: "var(--text-muted)" }}>
              {league.platform && <span>Platform: {league.platform} · </span>}
              {league.current_week > 0 && (
                <span>
                  Week {league.current_week} of {league.total_weeks}
                </span>
              )}
            </p>
          </>
        ) : null}
      </section>

      {/* This season */}
      <section className="space-y-6">
        <h2 className="text-sm font-semibold uppercase tracking-wide" style={{ color: "var(--text-muted)" }}>
          This season
        </h2>

        <Card>
          <CardContent className="p-6">
            <h3 className="mb-6 text-xl font-semibold" style={{ color: "var(--text-primary)" }}>
              Standings{seasonYear ? ` (${seasonYear})` : ""}
            </h3>
            {isStandingsLoading ? (
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

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard
            label="Average Score"
            value={
              isScheduleLoading
                ? "…"
                : `${leagueStats.averageScore > 0 ? leagueStats.averageScore.toFixed(1) : "0.0"} pts`
            }
            detail="per team/week"
          />
          <StatCard
            label="Highest Score"
            value={
              isScheduleLoading
                ? "…"
                : `${leagueStats.highestScore.score > 0 ? leagueStats.highestScore.score.toFixed(1) : "0"} pts`
            }
            detail={
              !isScheduleLoading && leagueStats.highestScore.teamName && leagueStats.highestScore.week > 0
                ? `${leagueStats.highestScore.teamName}, Week ${leagueStats.highestScore.week}`
                : "No data"
            }
          />
          <StatCard
            label="Closest Matchup"
            value={
              isScheduleLoading
                ? "…"
                : leagueStats.closestMatchup.margin < Infinity
                ? `${leagueStats.closestMatchup.homeScore.toFixed(2)}-${leagueStats.closestMatchup.awayScore.toFixed(2)}`
                : "None"
            }
            detail={
              !isScheduleLoading && leagueStats.closestMatchup.margin < Infinity
                ? `${leagueStats.closestMatchup.homeTeam} vs ${leagueStats.closestMatchup.awayTeam}, Week ${leagueStats.closestMatchup.week}`
                : "No matchups"
            }
          />
          <StatCard
            label="Biggest Blowout"
            value={
              isScheduleLoading
                ? "…"
                : leagueStats.biggestBlowout.margin > 0
                ? `${leagueStats.biggestBlowout.winnerScore.toFixed(1)}-${leagueStats.biggestBlowout.loserScore.toFixed(1)}`
                : "None"
            }
            detail={
              !isScheduleLoading && leagueStats.biggestBlowout.margin > 0
                ? `${leagueStats.biggestBlowout.winner} vs ${leagueStats.biggestBlowout.loser}, Week ${leagueStats.biggestBlowout.week} (margin ${leagueStats.biggestBlowout.margin.toFixed(1)} pts)`
                : "No games"
            }
          />
        </div>

        <Card>
          <CardContent className="grid grid-cols-1 gap-6 p-6 md:grid-cols-2">
            <div>
              <h3 className="mb-2 text-sm font-medium" style={{ color: "var(--text-muted)" }}>
                Most Points Scored
              </h3>
              {isLoading ? (
                <Skeleton className="h-6 w-full" />
              ) : teams && teams.length > 0 ? (
                [...teams]
                  .filter(
                    (team) =>
                      !team.owner.includes("Knapp") && !team.owner.includes("Landry")
                  )
                  .sort((a, b) => b.points.scored - a.points.scored)
                  .slice(0, 3)
                  .map((team) => (
                    <div key={`pf-${team.id}`} className="flex items-center justify-between py-2">
                      <Link
                        href={`/league/${id}/teams/${team.espnId}`}
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
                <p className="py-2 text-sm" style={{ color: "var(--text-muted)" }}>
                  No data available
                </p>
              )}
            </div>

            <div>
              <h3 className="mb-2 text-sm font-medium" style={{ color: "var(--text-muted)" }}>
                Most Points Against
              </h3>
              {isLoading ? (
                <Skeleton className="h-6 w-full" />
              ) : teams && teams.length > 0 ? (
                [...teams]
                  .filter(
                    (team) =>
                      !team.owner.includes("Knapp") && !team.owner.includes("Landry")
                  )
                  .sort((a, b) => b.points.against - a.points.against)
                  .slice(0, 3)
                  .map((team) => (
                    <div key={`pa-${team.id}`} className="flex items-center justify-between py-2">
                      <Link
                        href={`/league/${id}/teams/${team.espnId}`}
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
                <p className="py-2 text-sm" style={{ color: "var(--text-muted)" }}>
                  No data available
                </p>
              )}
            </div>
          </CardContent>
        </Card>
      </section>

      {/* League history */}
      <section className="space-y-6">
        <h2 className="text-sm font-semibold uppercase tracking-wide" style={{ color: "var(--text-muted)" }}>
          League history
        </h2>

        <Card>
          <CardContent className="p-6">
            <AllTimeMatchupsGrid teams={filteredTeams} headToHeadRecords={headToHeadRecords} />
          </CardContent>
        </Card>

        <HallOfFameWallOfShame
          leagueId={id}
          schedule={schedule}
          isLoading={isScheduleLoading}
          teams={teams}
        />

        <AllTimeRecordsTable leagueId={id} />
      </section>
    </div>
  );
}
