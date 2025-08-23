import { useState, useEffect } from "react";
import Link from "next/link";
import Layout from "../../components/Layout";
import {
  playersService,
  GetPlayersResponse,
  PlayerSummary,
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

export default function PlayersIndex() {
  const [playersData, setPlayersData] = useState<GetPlayersResponse | null>(
    null
  );
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filter states
  const [positionFilter, setPositionFilter] = useState<string>("all");
  const [yearFilter, setYearFilter] = useState<string>("2024");
  const [searchFilter, setSearchFilter] = useState<string>("");

  // Pagination states
  const [currentPage, setCurrentPage] = useState(1);
  const pageSize = 50;

  // Sorting states
  const [sortBy, setSortBy] = useState<string>("positionRank");
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("asc");

  // Sort function
  const handleSort = (column: string) => {
    if (sortBy === column) {
      setSortDirection(sortDirection === "asc" ? "desc" : "asc");
    } else {
      setSortBy(column);
      setSortDirection("asc");
    }
  };

  // SortHeader component
  const SortHeader = ({
    column,
    children,
  }: {
    column: string;
    children: React.ReactNode;
  }) => (
    <th
      className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
      onClick={() => handleSort(column)}
    >
      <div className="flex items-center space-x-1">
        <span>{children}</span>
        {sortBy === column && (
          <span className="text-blue-600">
            {sortDirection === "asc" ? "↑" : "↓"}
          </span>
        )}
      </div>
    </th>
  );

  useEffect(() => {
    const fetchPlayers = async () => {
      try {
        setIsLoading(true);
        setError(null);

        const params = {
          year: yearFilter,
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
  }, [positionFilter, yearFilter, currentPage]);

  // Filter and sort players
  const filteredPlayers = (
    playersData?.players?.filter(
      (player) =>
        player.name.toLowerCase().includes(searchFilter.toLowerCase()) ||
        player.team.toLowerCase().includes(searchFilter.toLowerCase())
    ) || []
  ).sort((a, b) => {
    const getValue = (player: PlayerSummary, key: string) => {
      switch (key) {
        case "positionRank":
          return player.positionRank;
        case "name":
          return player.name.toLowerCase();
        case "position":
          return player.position;
        case "totalFantasyPoints":
          return player.totalFantasyPoints;
        case "avgFantasyPoints":
          return player.avgFantasyPoints;
        case "difference":
          return player.difference;
        case "gamesPlayed":
          return player.gamesPlayed;
        default:
          return 0;
      }
    };

    const aValue = getValue(a, sortBy);
    const bValue = getValue(b, sortBy);

    if (aValue < bValue) {
      return sortDirection === "asc" ? -1 : 1;
    }
    if (aValue > bValue) {
      return sortDirection === "asc" ? 1 : -1;
    }
    return 0;
  });

  const positions = ["QB", "RB", "WR", "TE", "K", "D/ST"];

  if (isLoading) {
    return (
      <Layout>
        <div className="flex flex-col items-center justify-center h-64 space-y-4">
          <div className="w-8 h-8 border-4 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
          <div className="text-center">
            <div className="text-lg font-medium">Loading players...</div>
            <div className="text-sm text-gray-500 dark:text-gray-400 mt-2">
              This may take up to 10 seconds as we fetch data from the database
            </div>
          </div>
        </div>
      </Layout>
    );
  }

  if (error) {
    return (
      <Layout>
        <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded">
          <h2 className="text-lg font-medium mb-2">Error loading players</h2>
          <p>{error}</p>
        </div>
      </Layout>
    );
  }

  const totalPages = Math.ceil((playersData?.total || 0) / pageSize);

  return (
    <Layout>
      <div className="space-y-6">
        {/* Header */}
        <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
          <h1 className="text-3xl font-bold text-blue-600 mb-2">Players</h1>
          <p className="text-gray-600 dark:text-gray-400">
            View detailed statistics and performance data for all fantasy
            players
          </p>
        </section>

        {/* Filters */}
        <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
            {/* Search Filter */}
            <div>
              <label
                htmlFor="search"
                className="block text-sm font-medium text-gray-500 dark:text-gray-400 mb-1"
              >
                Search Players
              </label>
              <input
                type="text"
                id="search"
                placeholder="Search by name or team..."
                className="w-full p-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                value={searchFilter}
                onChange={(e) => setSearchFilter(e.target.value)}
              />
            </div>

            {/* Position Filter */}
            <div>
              <label
                htmlFor="position"
                className="block text-sm font-medium text-gray-500 dark:text-gray-400 mb-1"
              >
                Position
              </label>
              <select
                id="position"
                className="w-full p-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                value={positionFilter}
                onChange={(e) => setPositionFilter(e.target.value)}
              >
                <option value="all">All Positions</option>
                {positions.map((position) => (
                  <option key={position} value={position}>
                    {position}
                  </option>
                ))}
              </select>
            </div>

            {/* Year Filter */}
            <div>
              <label
                htmlFor="year"
                className="block text-sm font-medium text-gray-500 dark:text-gray-400 mb-1"
              >
                Season
              </label>
              <select
                id="year"
                className="w-full p-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                value={yearFilter}
                onChange={(e) => setYearFilter(e.target.value)}
              >
                <option value="2024">2024</option>
                <option value="2023">2023</option>
              </select>
            </div>

            {/* Reset Filters */}
            <div className="flex items-end">
              <button
                onClick={() => {
                  setPositionFilter("all");
                  setYearFilter("2024");
                  setSearchFilter("");
                  setCurrentPage(1);
                }}
                className="w-full py-2 px-4 bg-gray-100 hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600 text-gray-800 dark:text-gray-200 rounded-md border border-gray-300 dark:border-gray-600 transition-colors"
              >
                Reset Filters
              </button>
            </div>
          </div>
        </section>

        {/* Players Table */}
        <section className="bg-white dark:bg-gray-700 rounded-lg shadow-md overflow-hidden">
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-600">
              <thead className="bg-gray-50 dark:bg-gray-800">
                <tr>
                  <SortHeader column="positionRank">Rank</SortHeader>
                  <SortHeader column="name">Player</SortHeader>
                  <SortHeader column="position">Position</SortHeader>
                  <SortHeader column="totalFantasyPoints">
                    Fantasy Points
                  </SortHeader>
                  <SortHeader column="avgFantasyPoints">Avg/Game</SortHeader>
                  <SortHeader column="difference">vs Projection</SortHeader>
                  <SortHeader column="gamesPlayed">Games</SortHeader>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
                {filteredPlayers.length > 0 ? (
                  filteredPlayers.map((player) => (
                    <tr
                      key={player.id}
                      className="hover:bg-gray-50 dark:hover:bg-gray-600 transition-colors"
                    >
                      <td className="py-4 px-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                        #{player.positionRank} {player.position}
                      </td>
                      <td className="py-4 px-4 whitespace-nowrap">
                        <Link
                          href={`/players/${player.id}`}
                          className="text-blue-600 hover:text-blue-800 dark:hover:text-blue-400 font-medium transition-colors"
                        >
                          {player.name}
                        </Link>
                      </td>
                      <td className="py-4 px-4 whitespace-nowrap">
                        <span
                          className={`inline-flex px-2 py-1 text-xs rounded-full font-medium ${getPositionColor(
                            player.position
                          )}`}
                        >
                          {player.position}
                        </span>
                      </td>
                      <td className="py-4 px-4 whitespace-nowrap text-sm font-medium text-gray-900 dark:text-gray-100">
                        {player.totalFantasyPoints.toFixed(1)}
                      </td>
                      <td className="py-4 px-4 whitespace-nowrap text-sm text-gray-900 dark:text-gray-100">
                        {player.avgFantasyPoints.toFixed(1)}
                      </td>
                      <td className="py-4 px-4 whitespace-nowrap text-sm">
                        <span
                          className={`font-medium ${
                            player.difference > 0
                              ? "text-green-600 dark:text-green-400"
                              : "text-red-600 dark:text-red-400"
                          }`}
                        >
                          {player.difference > 0 ? "+" : ""}
                          {player.difference.toFixed(1)}
                        </span>
                      </td>
                      <td className="py-4 px-4 whitespace-nowrap text-sm text-gray-900 dark:text-gray-100">
                        {player.gamesPlayed}
                      </td>
                    </tr>
                  ))
                ) : (
                  <tr>
                    <td
                      colSpan={8}
                      className="py-8 text-center text-gray-500 dark:text-gray-400"
                    >
                      No players found matching your filters.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="bg-gray-50 dark:bg-gray-800 px-4 py-3 border-t border-gray-200 dark:border-gray-600">
              <div className="flex items-center justify-between">
                <div className="flex items-center text-sm text-gray-500 dark:text-gray-400">
                  <span>
                    Showing {(currentPage - 1) * pageSize + 1} to{" "}
                    {Math.min(currentPage * pageSize, playersData?.total || 0)}{" "}
                    of {playersData?.total || 0} players
                  </span>
                </div>
                <div className="flex space-x-2">
                  <button
                    onClick={() => setCurrentPage(Math.max(1, currentPage - 1))}
                    disabled={currentPage === 1}
                    className="px-3 py-1 text-sm bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-md disabled:opacity-50 disabled:cursor-not-allowed hover:bg-gray-50 dark:hover:bg-gray-600 transition-colors"
                  >
                    Previous
                  </button>
                  <span className="px-3 py-1 text-sm text-gray-700 dark:text-gray-300">
                    Page {currentPage} of {totalPages}
                  </span>
                  <button
                    onClick={() =>
                      setCurrentPage(Math.min(totalPages, currentPage + 1))
                    }
                    disabled={currentPage === totalPages}
                    className="px-3 py-1 text-sm bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-md disabled:opacity-50 disabled:cursor-not-allowed hover:bg-gray-50 dark:hover:bg-gray-600 transition-colors"
                  >
                    Next
                  </button>
                </div>
              </div>
            </div>
          )}
        </section>

        {/* Summary Stats */}
        <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
          <h2 className="text-lg font-semibold mb-4">League Summary</h2>
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
            <div className="text-center">
              <div className="text-2xl font-bold text-blue-600">
                {playersData?.total || 0}
              </div>
              <div className="text-sm text-gray-500 dark:text-gray-400">
                Total Players
              </div>
            </div>
            <div className="text-center">
              <div className="text-2xl font-bold text-green-600">
                {filteredPlayers.filter((p) => p.totalFantasyPoints > 0).length}
              </div>
              <div className="text-sm text-gray-500 dark:text-gray-400">
                Active Players
              </div>
            </div>
            <div className="text-center">
              <div className="text-2xl font-bold text-yellow-600">
                {filteredPlayers.filter((p) => p.difference > 0).length}
              </div>
              <div className="text-sm text-gray-500 dark:text-gray-400">
                Outperforming Projections
              </div>
            </div>
            <div className="text-center">
              <div className="text-2xl font-bold text-red-600">
                {filteredPlayers.filter((p) => p.difference < 0).length}
              </div>
              <div className="text-sm text-gray-500 dark:text-gray-400">
                Underperforming Projections
              </div>
            </div>
          </div>
        </section>
      </div>
    </Layout>
  );
}
