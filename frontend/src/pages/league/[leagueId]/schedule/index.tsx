import { useState } from "react";
import { useRouter } from "next/router";
import { useSchedule } from "@/hooks/useSchedule";
import { useStrengthOfSchedule, TeamStrength } from "@/hooks/useStrengthOfSchedule";
import { Matchup } from "@/types/models";
import Link from "next/link";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import ErrorState from "@/components/design-system/ErrorState";
import EmptyState from "@/components/design-system/EmptyState";
import DataTable, {
  type DataTableColumn,
} from "@/components/design-system/DataTable";

export default function Schedule() {
  const router = useRouter();
  const leagueId = Number(router.query.leagueId);
  const [selectedWeek, setSelectedWeek] = useState<string>("all");
  const [selectedYear, setSelectedYear] = useState<string>("all");
  const [selectedGameType, setSelectedGameType] = useState<string>("all");
  const [showFutureMatchups, setShowFutureMatchups] = useState<boolean>(false);
  const { schedule, isLoading, error } = useSchedule(leagueId, {
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

  // Determine the target year for calculations
  const targetYear = selectedYear !== "all" ? parseInt(selectedYear) : undefined;

  // Calculate strength of schedule using custom hook
  const { overallStrength, remainingStrength } = useStrengthOfSchedule(scheduleData, targetYear);

  // Helper function to get color based on difficulty
  const getDifficultyColor = (
    difficulty: TeamStrength["difficulty"]
  ): string => {
    switch (difficulty) {
      case "Hard":
        return "var(--status-danger-fg)";
      case "Med":
        return "var(--status-warning-fg)";
      case "Easy":
        return "var(--status-success-fg)";
      default:
        return "var(--text-muted)";
    }
  };

  const scheduleColumns: DataTableColumn<Matchup>[] = [
    {
      id: "year",
      header: "Year",
      cell: (game) => game.year,
    },
    {
      id: "week",
      header: "Week",
      cell: (game) =>
        game.playoffGameType === "CHAMPIONSHIP"
          ? "Championship"
          : game.playoffGameType === "THIRD_PLACE"
          ? "Third Place Game"
          : game.playoffGameType === "PLAYOFF"
          ? `Playoffs (Round ${
              game.week - (playoffStartWeeks[game.year] - 1)
            })`
          : `Week ${game.week}`,
    },
    {
      id: "matchup",
      header: "Matchup",
      cell: (game) => (
        <div className="flex flex-col md:flex-row md:items-center">
          <Link
            href={`/league/${leagueId}/teams/${game.homeTeamESPNID}`}
            className="font-medium hover:underline transition-colors duration-200"
            style={{
              color:
                game.completed && game.homeScore > game.awayScore
                  ? "var(--status-success-fg)"
                  : "var(--text-primary)",
            }}
          >
            {game.homeTeamName}
          </Link>
          <span className="hidden md:inline mx-2">vs</span>
          <span className="md:hidden">@</span>

          <Link
            href={`/league/${leagueId}/teams/${game.awayTeamESPNID}`}
            className="font-medium hover:underline transition-colors duration-200"
            style={{
              color:
                game.completed && game.awayScore > game.homeScore
                  ? "var(--status-success-fg)"
                  : "var(--text-primary)",
            }}
          >
            {game.awayTeamName}
          </Link>
        </div>
      ),
    },
    {
      id: "score",
      header: "Score",
      cell: (game) =>
        game.completed ? (
          <span>
            {game.homeScore.toFixed(2)} - {game.awayScore.toFixed(2)}
          </span>
        ) : (
          <span style={{ color: "var(--text-muted)" }}>Upcoming</span>
        ),
    },
    {
      id: "projectedScore",
      header: "Projected Score",
      cell: (game) =>
        game.completed ? (
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
          <span style={{ color: "var(--text-muted)" }}>Upcoming</span>
        ),
    },
    {
      id: "status",
      header: "Status",
      cell: (game) =>
        game.completed ? (
          <Badge
            variant="outline"
            className="rounded-full"
            style={{
              color: "var(--status-success-fg)",
              borderColor: "var(--status-success-fg)",
              backgroundColor: "var(--status-success-bg)",
            }}
          >
            Final
          </Badge>
        ) : (
          <Badge
            variant="outline"
            className="rounded-full"
            style={{
              color: "var(--status-info-fg)",
              borderColor: "var(--status-info-fg)",
              backgroundColor: "var(--status-info-bg)",
            }}
          >
            Upcoming
          </Badge>
        ),
    },
    {
      id: "details",
      header: "Details",
      cell: (game) => (
        <Link
          href={`/league/${leagueId}/schedule/${game.id}`}
          className="hover:underline"
          style={{ color: "var(--action-primary)" }}
        >
          View Details
        </Link>
      ),
    },
  ];

  return (
    <div className="space-y-8">
      <section>
        <h1
          className="text-3xl md:text-4xl font-bold mb-6"
          style={{ color: "var(--text-primary)" }}
        >
          League Schedule
        </h1>
        <p
          className="text-lg mb-8 max-w-3xl"
          style={{ color: "var(--text-muted)" }}
        >
          View upcoming matchups and past results for all teams in your
          league.
        </p>

        <Card className="mb-8">
          <CardContent className="p-6">
            <h2
              className="text-xl font-semibold mb-4"
              style={{ color: "var(--text-primary)" }}
            >
              Strength of Schedule
            </h2>
            {isLoading ? (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div className="space-y-3">
                  <Skeleton className="h-4 w-32" />
                  <Skeleton className="h-5 w-full" />
                  <Skeleton className="h-5 w-full" />
                  <Skeleton className="h-5 w-full" />
                </div>
                <div className="space-y-3">
                  <Skeleton className="h-4 w-32" />
                  <Skeleton className="h-5 w-full" />
                  <Skeleton className="h-5 w-full" />
                  <Skeleton className="h-5 w-full" />
                </div>
              </div>
            ) : overallStrength.length === 0 ? (
              <EmptyState title="No strength of schedule data available for the selected year. Make sure future matchups are loaded." />
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div>
                  <h3 className="text-lg font-medium mb-3">Remaining</h3>
                  <div
                    className="flex items-center mb-2 text-xs font-medium"
                    style={{ color: "var(--text-muted)" }}
                  >
                    <span className="w-40">Team</span>
                    <span className="flex-1 ml-3">Schedule Difficulty</span>
                    <span className="w-20 text-right ml-3">Opp Win %</span>
                  </div>
                  <div className="space-y-3">
                    {remainingStrength.map(
                      ({ team, difficulty, strengthPercentage }) => (
                        <div key={team} className="flex items-center">
                          <span className="w-40 text-sm truncate">{team}</span>
                          <div
                            className="flex-1 h-5 rounded-full overflow-hidden ml-3"
                            style={{ backgroundColor: "var(--surface-sunken)" }}
                          >
                            <div
                              className="h-full"
                              style={{
                                width: `${strengthPercentage}%`,
                                backgroundColor: getDifficultyColor(difficulty),
                              }}
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
                  <div
                    className="flex items-center mb-2 text-xs font-medium"
                    style={{ color: "var(--text-muted)" }}
                  >
                    <span className="w-40">Team</span>
                    <span className="flex-1 ml-3">Schedule Difficulty</span>
                    <span className="w-20 text-right ml-3">Opp Win %</span>
                  </div>
                  <div className="space-y-3">
                    {overallStrength.map(
                      ({ team, difficulty, strengthPercentage }) => (
                        <div key={team} className="flex items-center">
                          <span className="w-40 text-sm truncate">{team}</span>
                          <div
                            className="flex-1 h-5 rounded-full overflow-hidden ml-3"
                            style={{ backgroundColor: "var(--surface-sunken)" }}
                          >
                            <div
                              className="h-full"
                              style={{
                                width: `${strengthPercentage}%`,
                                backgroundColor: getDifficultyColor(difficulty),
                              }}
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
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-6">
            <div className="flex flex-col md:flex-row justify-between items-start md:items-center mb-6">
              <h2 className="text-xl font-semibold mb-3 md:mb-0" style={{ color: "var(--text-primary)" }}>
                Matchups
              </h2>

              <div className="w-full md:w-auto flex flex-col md:flex-row gap-4">
                <div className="flex items-center space-x-2">
                  <input
                    type="checkbox"
                    id="showFutureMatchups"
                    checked={showFutureMatchups}
                    onChange={(e) => setShowFutureMatchups(e.target.checked)}
                    className="h-4 w-4 rounded"
                    style={{ accentColor: "var(--action-primary)" }}
                    disabled={isLoading}
                  />
                  <label
                    htmlFor="showFutureMatchups"
                    className="text-sm font-medium"
                    style={{ color: "var(--text-primary)" }}
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
                  <Select
                    value={selectedYear}
                    onValueChange={(v) => setSelectedYear(v)}
                    disabled={isLoading}
                  >
                    <SelectTrigger id="yearFilter" aria-label="Select Year" className="w-full md:w-auto">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All Years</SelectItem>
                      {years.map((year) => (
                        <SelectItem key={year} value={String(year)}>
                          {year}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <label
                    htmlFor="weekFilter"
                    className="block text-sm font-medium mb-1 md:hidden"
                  >
                    Select Week
                  </label>
                  <Select
                    value={selectedWeek}
                    onValueChange={(v) => setSelectedWeek(v)}
                    disabled={isLoading}
                  >
                    <SelectTrigger id="weekFilter" aria-label="Select Week" className="w-full md:w-auto">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All Weeks</SelectItem>
                      {weeks.map((week) => (
                        <SelectItem key={week} value={String(week)}>
                          Week {week}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <label
                    htmlFor="gameTypeFilter"
                    className="block text-sm font-medium mb-1 md:hidden"
                  >
                    Select Game Type
                  </label>
                  <Select
                    value={selectedGameType}
                    onValueChange={(v) => setSelectedGameType(v)}
                    disabled={isLoading}
                  >
                    <SelectTrigger id="gameTypeFilter" aria-label="Select Game Type" className="w-full md:w-auto">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All Games</SelectItem>
                      <SelectItem value="regular">Regular Season</SelectItem>
                      <SelectItem value="playoffs">Playoffs</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
            </div>

            {isLoading ? (
              <div className="space-y-2">
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-10 w-full" />
              </div>
            ) : error ? (
              <ErrorState message={error.message} />
            ) : (
              <DataTable
                columns={scheduleColumns}
                rows={filteredGames}
                rowKey={(game) => String(game.id)}
              />
            )}
          </CardContent>
        </Card>
      </section>
    </div>
  );
}
