import { useState, useEffect } from "react";
import Link from "next/link";
import {
  playersService,
  GetPlayersResponse,
} from "@/services/playersService";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import EmptyState from "@/components/design-system/EmptyState";
import ErrorState from "@/components/design-system/ErrorState";
import DataTable, {
  type DataTableColumn,
} from "@/components/design-system/DataTable";
import { FOCUS_RING } from "@/components/design-system/focus-ring";

type PlayerRow = GetPlayersResponse["players"][number];

// Colorblind-safe qualitative palette (--chart-series-*) — position is an
// unordered category label, not a status, so the semantic --status-* tokens
// don't apply here. Mirrors the mapping established in
// league/[leagueId]/transactions/index.tsx (positionColor), extended with
// D/ST since this page uses that label instead of DEF.
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

export default function PlayersIndex() {
  const [playersData, setPlayersData] = useState<GetPlayersResponse | null>(
    null
  );
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filter states
  const [positionFilter, setPositionFilter] = useState<string>("all");
  const [yearFilter, setYearFilter] = useState<string>("all");
  const [rankFilter, setRankFilter] = useState<
    | "fantasy_points"
    | "avg_points"
    | "projected_points"
    | "games_played"
    | "vs_projection"
  >("fantasy_points");
  const [searchFilter, setSearchFilter] = useState<string>("");

  // Pagination states
  const [currentPage, setCurrentPage] = useState(1);
  const pageSize = 50;

  useEffect(() => {
    const fetchPlayers = async () => {
      try {
        setIsLoading(true);
        setError(null);

        const params = {
          year: yearFilter,
          rank: rankFilter,
          page: currentPage,
          limit: pageSize,
          ...(positionFilter !== "all" && { position: positionFilter }),
        };

        const data = await playersService.getPlayers(params);
        setPlayersData(data);
      } catch (err) {
        console.error("Error fetching players:", err);
        setError(
          err instanceof Error ? err.message : "An unknown error occurred"
        );
      } finally {
        setIsLoading(false);
      }
    };

    fetchPlayers();
  }, [positionFilter, yearFilter, rankFilter, currentPage]);

  // Filter players by search term (server already handles ranking)
  const filteredPlayers =
    playersData?.players?.filter(
      (player) =>
        player.name.toLowerCase().includes(searchFilter.toLowerCase()) ||
        player.team.toLowerCase().includes(searchFilter.toLowerCase())
    ) || [];

  const positions = ["QB", "RB", "WR", "TE", "K", "D/ST"];

  if (isLoading) {
    return (
      <div className="space-y-6">
        <Card>
          <CardContent className="p-6 space-y-3">
            <Skeleton className="h-8 w-40" />
            <Skeleton className="h-4 w-full max-w-md" />
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-6">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-5">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-16 w-full" />
              ))}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-6 space-y-2">
            {Array.from({ length: 8 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </CardContent>
        </Card>
      </div>
    );
  }

  if (error) {
    return <ErrorState message={error} />;
  }

  const totalPages = Math.ceil((playersData?.total || 0) / pageSize);

  const summaryStats: { label: string; value: number; color?: string }[] = [
    { label: "Total Players", value: playersData?.total || 0 },
    {
      label: "Active Players",
      value: filteredPlayers.filter((p) => p.totalFantasyPoints > 0).length,
    },
    {
      label: "Outperforming Projections",
      value: filteredPlayers.filter((p) => p.difference > 0).length,
      color: "var(--status-success-fg)",
    },
    {
      label: "Underperforming Projections",
      value: filteredPlayers.filter((p) => p.difference < 0).length,
      color: "var(--status-danger-fg)",
    },
  ];

  const columns: DataTableColumn<PlayerRow>[] = [
    {
      id: "rank",
      header: "Rank",
      cell: (player) => (
        <span style={{ color: "var(--text-muted)" }}>
          #{player.positionRank} {player.position}
        </span>
      ),
    },
    {
      id: "player",
      header: "Player",
      cell: (player) => (
        <Link
          href={`/players/${player.id}`}
          className="font-medium hover:underline"
          style={{ color: "var(--action-primary)" }}
        >
          {player.name}
        </Link>
      ),
    },
    {
      id: "position",
      header: "Position",
      cell: (player) => (
        <Badge
          variant="outline"
          style={{
            color: getPositionColor(player.position),
            borderColor: getPositionColor(player.position),
          }}
        >
          {player.position}
        </Badge>
      ),
    },
    {
      id: "fantasyPoints",
      header: "Fantasy Points",
      cell: (player) => (
        <span
          className="font-medium"
          style={{ color: "var(--text-primary)" }}
        >
          {player.totalFantasyPoints.toFixed(1)}
        </span>
      ),
    },
    {
      id: "avgPerGame",
      header: "Avg/Game",
      cell: (player) => (
        <span style={{ color: "var(--text-primary)" }}>
          {player.avgFantasyPoints.toFixed(1)}
        </span>
      ),
    },
    {
      id: "vsProjection",
      header: "vs Projection",
      cell: (player) => (
        <span
          className="font-medium"
          style={{
            color:
              player.difference > 0
                ? "var(--status-success-fg)"
                : "var(--status-danger-fg)",
          }}
        >
          {player.difference > 0 ? "+" : ""}
          {player.difference.toFixed(1)}
        </span>
      ),
    },
    {
      id: "games",
      header: "Games",
      cell: (player) => (
        <span style={{ color: "var(--text-primary)" }}>
          {player.gamesPlayed}
        </span>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      {/* Header */}
      <Card>
        <CardContent className="p-6">
          <h1
            className="text-3xl font-bold mb-2"
            style={{ color: "var(--text-primary)" }}
          >
            Players
          </h1>
          <p style={{ color: "var(--text-muted)" }}>
            View career statistics and performance data for all fantasy
            players. Use the season filter to view specific year data.
          </p>
        </CardContent>
      </Card>

      {/* Filters */}
      <Card>
        <CardContent className="p-6">
          <div className="grid grid-cols-1 md:grid-cols-5 gap-4">
            {/* Search Filter */}
            <div>
              <label
                htmlFor="search"
                className="block text-sm font-medium mb-1"
                style={{ color: "var(--text-muted)" }}
              >
                Search Players
              </label>
              <input
                type="text"
                id="search"
                placeholder="Search by name or team..."
                className={`w-full rounded-lg border bg-transparent px-2.5 py-2 text-sm ${FOCUS_RING}`}
                style={{
                  borderColor: "var(--border-subtle)",
                  color: "var(--text-primary)",
                }}
                value={searchFilter}
                onChange={(e) => setSearchFilter(e.target.value)}
              />
            </div>

            {/* Position Filter */}
            <div>
              <label
                htmlFor="position"
                className="block text-sm font-medium mb-1"
                style={{ color: "var(--text-muted)" }}
              >
                Position
              </label>
              <Select
                value={positionFilter}
                onValueChange={(v) => setPositionFilter(v)}
              >
                <SelectTrigger id="position" aria-label="Position" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Positions</SelectItem>
                  {positions.map((position) => (
                    <SelectItem key={position} value={position}>
                      {position}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Year Filter */}
            <div>
              <label
                htmlFor="year"
                className="block text-sm font-medium mb-1"
                style={{ color: "var(--text-muted)" }}
              >
                Season
              </label>
              <Select value={yearFilter} onValueChange={(v) => setYearFilter(v)}>
                <SelectTrigger id="year" aria-label="Season" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Time</SelectItem>
                  {Array.from(
                    { length: new Date().getFullYear() - 2019 + 1 },
                    (_, i) => new Date().getFullYear() - i
                  ).map((year) => (
                    <SelectItem key={year} value={String(year)}>
                      {year}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Ranking Filter */}
            <div>
              <label
                htmlFor="rank"
                className="block text-sm font-medium mb-1"
                style={{ color: "var(--text-muted)" }}
              >
                Rank By
              </label>
              <Select
                value={rankFilter}
                onValueChange={(v) =>
                  setRankFilter(
                    v as
                      | "fantasy_points"
                      | "avg_points"
                      | "projected_points"
                      | "games_played"
                      | "vs_projection"
                  )
                }
              >
                <SelectTrigger id="rank" aria-label="Rank By" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="fantasy_points">Fantasy Points</SelectItem>
                  <SelectItem value="avg_points">Avg Points/Game</SelectItem>
                  <SelectItem value="projected_points">
                    Projected Points
                  </SelectItem>
                  <SelectItem value="games_played">Games Played</SelectItem>
                  <SelectItem value="vs_projection">vs Projection</SelectItem>
                </SelectContent>
              </Select>
            </div>

            {/* Reset Filters */}
            <div className="flex items-end">
              <Button
                variant="outline"
                className="w-full"
                onClick={() => {
                  setPositionFilter("all");
                  setYearFilter("all");
                  setRankFilter("fantasy_points");
                  setSearchFilter("");
                  setCurrentPage(1);
                }}
              >
                Reset Filters
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Players Table */}
      <Card>
        <CardContent className="p-6">
          {filteredPlayers.length > 0 ? (
            <DataTable
              columns={columns}
              rows={filteredPlayers}
              rowKey={(player) => player.id}
            />
          ) : (
            <EmptyState title="No players found matching your filters." />
          )}
        </CardContent>

        {/* Pagination */}
        {totalPages > 1 && (
          <CardContent className="flex items-center justify-between border-t pt-4">
            <span className="text-sm" style={{ color: "var(--text-muted)" }}>
              Showing {(currentPage - 1) * pageSize + 1} to{" "}
              {Math.min(currentPage * pageSize, playersData?.total || 0)} of{" "}
              {playersData?.total || 0} players
            </span>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                onClick={() => setCurrentPage(Math.max(1, currentPage - 1))}
                disabled={currentPage === 1}
              >
                Previous
              </Button>
              <span className="text-sm" style={{ color: "var(--text-secondary)" }}>
                Page {currentPage} of {totalPages}
              </span>
              <Button
                variant="outline"
                onClick={() =>
                  setCurrentPage(Math.min(totalPages, currentPage + 1))
                }
                disabled={currentPage === totalPages}
              >
                Next
              </Button>
            </div>
          </CardContent>
        )}
      </Card>

      {/* Summary Stats */}
      <Card>
        <CardContent className="p-6">
          <h2
            className="text-lg font-semibold mb-4"
            style={{ color: "var(--text-primary)" }}
          >
            Summary
          </h2>
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
            {summaryStats.map((stat) => (
              <div key={stat.label} className="text-center">
                <div
                  className="text-2xl font-bold"
                  style={{ color: stat.color ?? "var(--text-primary)" }}
                >
                  {stat.value}
                </div>
                <div className="text-sm" style={{ color: "var(--text-muted)" }}>
                  {stat.label}
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
