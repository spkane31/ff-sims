import { useState, useEffect } from "react";
import { useRouter } from "next/router";
import Link from "next/link";
import {
  playersService,
  PlayerDetail,
  PlayerStats,
  AnnualStatsEntry,
  GameLogEntry,
} from "@/services/playersService";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import EmptyState from "@/components/design-system/EmptyState";
import ErrorState from "@/components/design-system/ErrorState";
import DataTable, { type DataTableColumn } from "@/components/design-system/DataTable";

type Tab = "overview" | "stats" | "gamelog";

// Colorblind-safe qualitative palette (--chart-series-*) — position is an
// unordered category label, not a status, so the semantic --status-* tokens
// don't apply here. Mirrors the mapping established in
// league/[leagueId]/transactions/index.tsx (positionColor) and
// pages/players/index.tsx (getPositionColor).
function getPositionColor(position: string): string {
  switch (position) {
    case "QB":
      return "var(--chart-series-1)";
    case "RB":
      return "var(--chart-series-2)";
    case "WR":
      return "var(--chart-series-3)";
    case "TE":
      return "var(--chart-series-4)";
    case "K":
      return "var(--chart-series-5)";
    case "D/ST":
      return "var(--chart-series-6)";
    default:
      return "var(--chart-series-6)";
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
  const { playerId } = router.query;

  const [isLoading, setIsLoading] = useState(true);
  const [player, setPlayer] = useState<PlayerDetail | null>(null);
  const [activeTab, setActiveTab] = useState<Tab>("overview");
  const [error, setError] = useState<string | null>(null);

  // Add filter states for game log
  const [yearFilter, setYearFilter] = useState<string>("all");
  const [weekFilter, setWeekFilter] = useState<string>("all");

  useEffect(() => {
    if (!playerId) return;

    const fetchPlayerData = async () => {
      try {
        setIsLoading(true);
        setError(null);

        // Use playersService to fetch player data
        const playerData = await playersService.getPlayerDetail(playerId as string);
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
  }, [playerId]);

  if (isLoading) {
    return (
      <div className="flex flex-col items-center justify-center h-64 space-y-4">
        <Skeleton className="h-8 w-8 rounded-full" />
        <div className="text-center">
          <div className="text-lg font-medium" style={{ color: "var(--text-primary)" }}>
            Loading player data...
          </div>
          <div className="text-sm mt-2" style={{ color: "var(--text-muted)" }}>
            This may take up to 10 seconds as we fetch data from the database
          </div>
        </div>
      </div>
    );
  }

  if (error || !player) {
    return (
      <div className="space-y-4">
        <h2 className="text-lg font-medium" style={{ color: "var(--text-primary)" }}>
          Player not found
        </h2>
        <ErrorState
          message={
            error ||
            "We could not find a player with the requested ID. Please check the URL and try again."
          }
        />
        <Link
          href="/players"
          className="inline-block hover:underline"
          style={{ color: "var(--action-primary)" }}
        >
          ← Back to Players
        </Link>
      </div>
    );
  }

  // Game log filtered by the year/week selects (Game Log tab)
  const filteredGameLog = filterGameLog(
    player.gameLog || [],
    yearFilter,
    weekFilter
  );

  const annualStatsColumns: DataTableColumn<AnnualStatsEntry>[] = [
    {
      id: "year",
      header: "Year",
      cell: (y) => (
        <span className="font-medium" style={{ color: "var(--action-primary)" }}>
          {y.year}
        </span>
      ),
    },
    {
      id: "games",
      header: "Games",
      cell: (y) => <span style={{ color: "var(--text-primary)" }}>{y.gamesPlayed}</span>,
    },
    {
      id: "points",
      header: "Points",
      cell: (y) => (
        <span className="font-bold" style={{ color: "var(--text-primary)" }}>
          {y.totalFantasyPoints.toFixed(1)}
        </span>
      ),
    },
    {
      id: "projected",
      header: "Projected",
      cell: (y) => (
        <span style={{ color: "var(--text-muted)" }}>
          {y.totalProjectedPoints.toFixed(1)}
        </span>
      ),
    },
    {
      id: "average",
      header: "Average",
      cell: (y) => <span style={{ color: "var(--text-primary)" }}>{y.avgFantasyPoints.toFixed(1)}</span>,
    },
    {
      id: "difference",
      header: "Difference",
      cell: (y) => (
        <span
          className="font-medium"
          style={{
            color:
              y.difference > 0
                ? "var(--status-success-fg)"
                : "var(--status-danger-fg)",
          }}
        >
          {y.difference > 0 ? "+" : ""}
          {y.difference.toFixed(1)}
        </span>
      ),
    },
    {
      id: "bestGame",
      header: "Best Game",
      cell: (y) => (
        <div>
          <div className="font-medium" style={{ color: "var(--status-success-fg)" }}>
            {y.bestGame.points.toFixed(1)}
          </div>
          <div className="text-xs" style={{ color: "var(--text-muted)" }}>
            Wk {y.bestGame.week}
          </div>
        </div>
      ),
    },
    {
      id: "worstGame",
      header: "Worst Game",
      cell: (y) => (
        <div>
          <div className="font-medium" style={{ color: "var(--status-danger-fg)" }}>
            {y.worstGame.points.toFixed(1)}
          </div>
          <div className="text-xs" style={{ color: "var(--text-muted)" }}>
            Wk {y.worstGame.week}
          </div>
        </div>
      ),
    },
    {
      id: "consistency",
      header: "Consistency",
      cell: (y) => (
        <div>
          <div className="font-medium" style={{ color: "var(--text-primary)" }}>
            {y.consistencyScore.toFixed(1)}
          </div>
          <div className="text-xs" style={{ color: "var(--text-muted)" }}>
            {y.consistencyScore < 5
              ? "Very consistent"
              : y.consistencyScore < 8
              ? "Consistent"
              : y.consistencyScore < 12
              ? "Variable"
              : "Inconsistent"}
          </div>
        </div>
      ),
    },
  ];

  if (player.position === "QB") {
    annualStatsColumns.push(
      {
        id: "passYds",
        header: "Pass Yds",
        cell: (y) => (
          <span style={{ color: "var(--text-primary)" }}>
            {y.totalStats.passingYards.toLocaleString()}
          </span>
        ),
      },
      {
        id: "passTDs",
        header: "Pass TDs",
        cell: (y) => <span style={{ color: "var(--text-primary)" }}>{y.totalStats.passingTDs}</span>,
      },
      {
        id: "ints",
        header: "INTs",
        cell: (y) => <span style={{ color: "var(--text-primary)" }}>{y.totalStats.interceptions}</span>,
      }
    );
  } else if (
    player.position === "RB" ||
    player.position === "WR" ||
    player.position === "TE"
  ) {
    annualStatsColumns.push(
      {
        id: "rec",
        header: "Rec",
        cell: (y) => <span style={{ color: "var(--text-primary)" }}>{y.totalStats.receptions}</span>,
      },
      {
        id: "recYds",
        header: "Rec Yds",
        cell: (y) => (
          <span style={{ color: "var(--text-primary)" }}>
            {y.totalStats.receivingYards.toLocaleString()}
          </span>
        ),
      },
      {
        id: "recTDs",
        header: "Rec TDs",
        cell: (y) => <span style={{ color: "var(--text-primary)" }}>{y.totalStats.receivingTDs}</span>,
      }
    );
  } else if (player.position === "K") {
    annualStatsColumns.push(
      {
        id: "fg",
        header: "FG",
        cell: (y) => <span style={{ color: "var(--text-primary)" }}>{y.totalStats.fieldGoals}</span>,
      },
      {
        id: "xp",
        header: "XP",
        cell: (y) => <span style={{ color: "var(--text-primary)" }}>{y.totalStats.extraPoints}</span>,
      }
    );
  }

  const gameLogColumns: DataTableColumn<GameLogEntry>[] = [
    {
      id: "week",
      header: "Week",
      cell: (g) => (
        <span className="font-medium" style={{ color: "var(--text-primary)" }}>
          {g.week}
        </span>
      ),
    },
    {
      id: "year",
      header: "Year",
      cell: (g) => <span style={{ color: "var(--text-primary)" }}>{g.year}</span>,
    },
    {
      id: "points",
      header: "Points",
      cell: (g) => (
        <span className="font-bold" style={{ color: "var(--action-primary)" }}>
          {g.actualPoints.toFixed(1)}
        </span>
      ),
    },
    {
      id: "projected",
      header: "Projected",
      cell: (g) => <span style={{ color: "var(--text-muted)" }}>{g.projectedPoints.toFixed(1)}</span>,
    },
    {
      id: "difference",
      header: "Difference",
      cell: (g) => (
        <span
          className="font-medium"
          style={{
            color:
              g.difference > 0
                ? "var(--status-success-fg)"
                : "var(--status-danger-fg)",
          }}
        >
          {g.difference > 0 ? "+" : ""}
          {g.difference.toFixed(1)}
        </span>
      ),
    },
    {
      id: "started",
      header: "Started",
      cell: (g) =>
        g.startedFlag ? (
          <Badge
            variant="outline"
            style={{
              color: "var(--status-success-fg)",
              borderColor: "var(--status-success-fg)",
              backgroundColor: "var(--status-success-bg)",
            }}
          >
            Started
          </Badge>
        ) : (
          <Badge variant="secondary">Bench</Badge>
        ),
    },
    {
      id: "date",
      header: "Date",
      cell: (g) => (
        <span style={{ color: "var(--text-muted)" }}>
          {new Date(g.gameDate).toLocaleDateString()}
        </span>
      ),
    },
  ];

  return (
    <div className="space-y-8">
      {/* Player Header */}
      <Card>
        <CardContent className="p-6">
          <div className="flex flex-col md:flex-row justify-between md:items-center">
            <div>
              <div className="flex items-center mb-2">
                <h1
                  className="text-3xl md:text-4xl font-bold"
                  style={{ color: "var(--action-primary)" }}
                >
                  {player.name}
                </h1>
                <Badge
                  variant="outline"
                  className="ml-3"
                  style={{
                    color: getPositionColor(player.position),
                    borderColor: getPositionColor(player.position),
                  }}
                >
                  {player.position}
                </Badge>
                <Badge variant="secondary" className="ml-2">
                  #{player.positionRank} {player.position}
                </Badge>
              </div>
            </div>

            <div className="mt-4 md:mt-0">
              <Link
                href="/players"
                className="hover:underline"
                style={{ color: "var(--action-primary)" }}
              >
                ← Back to Players
              </Link>
            </div>
          </div>
        </CardContent>
      </Card>

      <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as Tab)}>
        <TabsList variant="line" className="mb-2">
          <TabsTrigger value="overview" className="px-4">
            Overview
          </TabsTrigger>
          <TabsTrigger value="stats" className="px-4">
            Statistics
          </TabsTrigger>
          <TabsTrigger value="gamelog" className="px-4">
            Game Log
          </TabsTrigger>
        </TabsList>

        {/* Overview Tab */}
        <TabsContent value="overview" className="space-y-6">
          <Card>
            <CardContent className="p-6">
              <h2 className="text-xl font-semibold mb-4" style={{ color: "var(--text-primary)" }}>
                Fantasy Performance
              </h2>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
                <div className="p-4 rounded-lg" style={{ backgroundColor: "var(--surface-sunken)" }}>
                  <h3 className="text-sm font-medium mb-1" style={{ color: "var(--text-muted)" }}>
                    Total Points
                  </h3>
                  <div className="text-2xl font-bold" style={{ color: "var(--text-primary)" }}>
                    {player.totalFantasyPoints.toFixed(1)}
                  </div>
                  <div className="mt-1 text-sm" style={{ color: "var(--text-muted)" }}>
                    {player.avgFantasyPoints.toFixed(1)} per game
                  </div>
                </div>

                <div className="p-4 rounded-lg" style={{ backgroundColor: "var(--surface-sunken)" }}>
                  <h3 className="text-sm font-medium mb-1" style={{ color: "var(--text-muted)" }}>
                    Projected Points
                  </h3>
                  <div className="text-2xl font-bold" style={{ color: "var(--text-primary)" }}>
                    {player.totalProjectedPoints.toFixed(1)}
                  </div>
                  <div className="mt-1 text-sm" style={{ color: "var(--text-muted)" }}>
                    {(
                      player.totalProjectedPoints / player.gamesPlayed
                    ).toFixed(1)}{" "}
                    per game
                  </div>
                </div>

                <div className="p-4 rounded-lg" style={{ backgroundColor: "var(--surface-sunken)" }}>
                  <h3 className="text-sm font-medium mb-1" style={{ color: "var(--text-muted)" }}>
                    Difference
                  </h3>
                  <div
                    className="text-2xl font-bold"
                    style={{
                      color:
                        player.difference > 0
                          ? "var(--status-success-fg)"
                          : "var(--status-danger-fg)",
                    }}
                  >
                    {player.difference > 0 ? "+" : ""}
                    {player.difference.toFixed(1)}
                  </div>
                  <div className="mt-1 text-sm" style={{ color: "var(--text-muted)" }}>
                    vs projection
                  </div>
                </div>

                <div className="p-4 rounded-lg" style={{ backgroundColor: "var(--surface-sunken)" }}>
                  <h3 className="text-sm font-medium mb-1" style={{ color: "var(--text-muted)" }}>
                    Games Played
                  </h3>
                  <div className="text-2xl font-bold" style={{ color: "var(--text-primary)" }}>
                    {player.gamesPlayed}
                  </div>
                  <div className="mt-1 text-sm" style={{ color: "var(--text-muted)" }}>
                    Position rank: #{player.positionRank}
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardContent className="p-6">
              <h2 className="text-xl font-semibold mb-4" style={{ color: "var(--text-primary)" }}>
                Season Statistics
              </h2>
              <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-4">
                {getRelevantStats(player.totalStats, player.position).map(
                  (stat, index) => (
                    <div key={index} className="text-center">
                      <div className="text-2xl font-bold" style={{ color: "var(--action-primary)" }}>
                        {stat.value}
                      </div>
                      <div className="text-sm" style={{ color: "var(--text-muted)" }}>
                        {stat.label}
                      </div>
                    </div>
                  )
                )}
              </div>
            </CardContent>
          </Card>

          {/* Annual Statistics Table */}
          <Card>
            <CardContent className="p-6">
              <h2 className="text-xl font-semibold mb-4" style={{ color: "var(--text-primary)" }}>
                Annual Statistics
              </h2>
              {player.annualStats && player.annualStats.length > 0 ? (
                <DataTable
                  columns={annualStatsColumns}
                  rows={player.annualStats}
                  rowKey={(y) => String(y.year)}
                />
              ) : (
                <EmptyState title="No annual statistics available" />
              )}
            </CardContent>
          </Card>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <Card>
              <CardContent className="p-6">
                <h2 className="text-lg font-semibold mb-4" style={{ color: "var(--text-primary)" }}>
                  Performance Trends
                </h2>
                {/* TODO: Add chart showing points per game over time */}
                <div className="text-center py-8" style={{ color: "var(--text-muted)" }}>
                  <p>Performance chart coming soon</p>
                  <p className="text-sm mt-2">
                    Will show weekly fantasy points trend
                  </p>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardContent className="p-6">
                <h2 className="text-lg font-semibold mb-4" style={{ color: "var(--text-primary)" }}>
                  Quick Stats
                </h2>
                <div className="space-y-3">
                  <div className="flex justify-between">
                    <span style={{ color: "var(--text-primary)" }}>Best Game:</span>
                    <span className="font-medium" style={{ color: "var(--status-success-fg)" }}>
                      {player.bestGame?.points > 0 ? (
                        <>
                          {player.bestGame.points.toFixed(1)} pts
                          <div className="text-xs" style={{ color: "var(--text-muted)" }}>
                            {player.bestGame.year} Week {player.bestGame.week}
                          </div>
                        </>
                      ) : (
                        "No games"
                      )}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span style={{ color: "var(--text-primary)" }}>Worst Game:</span>
                    <span className="font-medium" style={{ color: "var(--status-danger-fg)" }}>
                      {player.worstGame?.points >= 0 &&
                      player.worstGame.points < 1000 ? (
                        <>
                          {player.worstGame.points.toFixed(1)} pts
                          <div className="text-xs" style={{ color: "var(--text-muted)" }}>
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
                    <span style={{ color: "var(--text-primary)" }}>Consistency:</span>
                    <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                      {player.consistencyScore > 0 ? (
                        <>
                          σ = {player.consistencyScore.toFixed(1)}
                          <div className="text-xs" style={{ color: "var(--text-muted)" }}>
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
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        {/* Statistics Tab */}
        <TabsContent value="stats" className="space-y-6">
          <Card>
            <CardContent className="p-6">
              <h2 className="text-xl font-semibold mb-4" style={{ color: "var(--text-primary)" }}>
                Detailed Statistics
              </h2>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
                {/* Offensive Stats */}
                <div>
                  <h3 className="text-lg font-medium mb-4" style={{ color: "var(--text-primary)" }}>
                    Offensive Statistics
                  </h3>
                  <div className="space-y-3">
                    {player.position === "QB" && (
                      <>
                        <div className="flex justify-between py-2 border-b">
                          <span style={{ color: "var(--text-primary)" }}>Passing Yards</span>
                          <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                            {player.totalStats.passingYards.toLocaleString()}
                          </span>
                        </div>
                        <div className="flex justify-between py-2 border-b">
                          <span style={{ color: "var(--text-primary)" }}>Passing Touchdowns</span>
                          <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                            {player.totalStats.passingTDs}
                          </span>
                        </div>
                        <div className="flex justify-between py-2 border-b">
                          <span style={{ color: "var(--text-primary)" }}>Interceptions</span>
                          <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                            {player.totalStats.interceptions}
                          </span>
                        </div>
                        <div className="flex justify-between py-2 border-b">
                          <span style={{ color: "var(--text-primary)" }}>Rushing Yards</span>
                          <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                            {player.totalStats.rushingYards.toLocaleString()}
                          </span>
                        </div>
                        <div className="flex justify-between py-2">
                          <span style={{ color: "var(--text-primary)" }}>Rushing Touchdowns</span>
                          <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                            {player.totalStats.rushingTDs}
                          </span>
                        </div>
                      </>
                    )}

                    {(player.position === "RB" ||
                      player.position === "WR" ||
                      player.position === "TE") && (
                      <>
                        <div className="flex justify-between py-2 border-b">
                          <span style={{ color: "var(--text-primary)" }}>Receptions</span>
                          <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                            {player.totalStats.receptions}
                          </span>
                        </div>
                        <div className="flex justify-between py-2 border-b">
                          <span style={{ color: "var(--text-primary)" }}>Receiving Yards</span>
                          <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                            {player.totalStats.receivingYards.toLocaleString()}
                          </span>
                        </div>
                        <div className="flex justify-between py-2 border-b">
                          <span style={{ color: "var(--text-primary)" }}>Receiving Touchdowns</span>
                          <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                            {player.totalStats.receivingTDs}
                          </span>
                        </div>
                        {player.position === "RB" && (
                          <>
                            <div className="flex justify-between py-2 border-b">
                              <span style={{ color: "var(--text-primary)" }}>Rushing Yards</span>
                              <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                                {player.totalStats.rushingYards.toLocaleString()}
                              </span>
                            </div>
                            <div className="flex justify-between py-2 border-b">
                              <span style={{ color: "var(--text-primary)" }}>Rushing Touchdowns</span>
                              <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                                {player.totalStats.rushingTDs}
                              </span>
                            </div>
                          </>
                        )}
                        <div className="flex justify-between py-2">
                          <span style={{ color: "var(--text-primary)" }}>Fumbles</span>
                          <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                            {player.totalStats.fumbles}
                          </span>
                        </div>
                      </>
                    )}

                    {player.position === "K" && (
                      <>
                        <div className="flex justify-between py-2 border-b">
                          <span style={{ color: "var(--text-primary)" }}>Field Goals Made</span>
                          <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                            {player.totalStats.fieldGoals}
                          </span>
                        </div>
                        <div className="flex justify-between py-2">
                          <span style={{ color: "var(--text-primary)" }}>Extra Points Made</span>
                          <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                            {player.totalStats.extraPoints}
                          </span>
                        </div>
                      </>
                    )}
                  </div>
                </div>

                {/* Per Game Averages */}
                <div>
                  <h3 className="text-lg font-medium mb-4" style={{ color: "var(--text-primary)" }}>
                    Per Game Averages
                  </h3>
                  <div className="space-y-3">
                    <div className="flex justify-between py-2 border-b">
                      <span style={{ color: "var(--text-primary)" }}>Fantasy Points</span>
                      <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                        {player.avgFantasyPoints.toFixed(1)}
                      </span>
                    </div>
                    {player.position === "QB" && (
                      <>
                        <div className="flex justify-between py-2 border-b">
                          <span style={{ color: "var(--text-primary)" }}>Passing Yards</span>
                          <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                            {(
                              player.totalStats.passingYards /
                              player.gamesPlayed
                            ).toFixed(1)}
                          </span>
                        </div>
                        <div className="flex justify-between py-2 border-b">
                          <span style={{ color: "var(--text-primary)" }}>Passing TDs</span>
                          <span className="font-medium" style={{ color: "var(--text-primary)" }}>
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
                        <div className="flex justify-between py-2 border-b">
                          <span style={{ color: "var(--text-primary)" }}>Receptions</span>
                          <span className="font-medium" style={{ color: "var(--text-primary)" }}>
                            {(
                              player.totalStats.receptions / player.gamesPlayed
                            ).toFixed(1)}
                          </span>
                        </div>
                        <div className="flex justify-between py-2 border-b">
                          <span style={{ color: "var(--text-primary)" }}>Receiving Yards</span>
                          <span className="font-medium" style={{ color: "var(--text-primary)" }}>
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
            </CardContent>
          </Card>
        </TabsContent>

        {/* Game Log Tab */}
        <TabsContent value="gamelog" className="space-y-6">
          <Card>
            <CardContent className="p-6">
              <h2 className="text-xl font-semibold mb-4" style={{ color: "var(--text-primary)" }}>
                Game Log
              </h2>

              {/* Filters */}
              <div className="flex flex-col md:flex-row gap-4 mb-6">
                <div className="w-full md:w-auto">
                  <label
                    htmlFor="year-filter"
                    className="block text-sm font-medium mb-1"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Filter by Year
                  </label>
                  <Select value={yearFilter} onValueChange={(v) => setYearFilter(v)}>
                    <SelectTrigger id="year-filter" aria-label="Filter by Year" className="w-full md:w-auto">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All Years</SelectItem>
                      <SelectItem value="2024">2024</SelectItem>
                      <SelectItem value="2023">2023</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="w-full md:w-auto">
                  <label
                    htmlFor="week-filter"
                    className="block text-sm font-medium mb-1"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Filter by Week
                  </label>
                  <Select value={weekFilter} onValueChange={(v) => setWeekFilter(v)}>
                    <SelectTrigger id="week-filter" aria-label="Filter by Week" className="w-full md:w-auto">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All Weeks</SelectItem>
                      {Array.from({ length: 18 }, (_, i) => i + 1).map((week) => (
                        <SelectItem key={week} value={String(week)}>
                          Week {week}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                {/* Reset button */}
                {(yearFilter !== "all" || weekFilter !== "all") && (
                  <div className="w-full md:w-auto flex items-end">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        setYearFilter("all");
                        setWeekFilter("all");
                      }}
                    >
                      <svg
                        xmlns="http://www.w3.org/2000/svg"
                        className="h-4 w-4"
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
                    </Button>
                  </div>
                )}
              </div>

              {/* Game Log Table */}
              {filteredGameLog.length === 0 ? (
                <EmptyState
                  title={
                    player.gameLog?.length === 0
                      ? "No game log data available"
                      : "No games match the selected filters"
                  }
                />
              ) : (
                <DataTable
                  columns={gameLogColumns}
                  rows={filteredGameLog}
                  rowKey={(g) => `${g.year}-${g.week}`}
                />
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
