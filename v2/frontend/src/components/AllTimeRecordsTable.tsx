import { useEffect, useState } from "react";
import Link from "next/link";
import { teamsService, Team } from "@/services/teamsService";
import { expectedWinsService } from "@/services/expectedWinsService";

interface Props {
  leagueId: number;
}

type SortField =
  | "owner"
  | "regularSeasonRecord"
  | "playoffRecord"
  | "pointsFor"
  | "pointsAgainst"
  | "expectedRecord"
  | "luck";
type SortDirection = "asc" | "desc";

export default function AllTimeRecordsTable({ leagueId }: Props) {
  const [teams, setTeams] = useState<Team[]>([]);
  const [loading, setLoading] = useState(true);
  const [sortField, setSortField] = useState<SortField>("regularSeasonRecord");
  const [sortDirection, setSortDirection] = useState<SortDirection>("desc");

  useEffect(() => {
    if (!leagueId) return;
    setLoading(true);
    Promise.all([
      teamsService.getAllTeams(leagueId),
      expectedWinsService
        .getAllTimeExpectedWins(leagueId)
        .catch(() => ({ data: [] })),
    ])
      .then(([teamsResponse, expectedWinsResponse]) => {
        const merged = teamsResponse.teams.map((team) => {
          const ew = expectedWinsResponse.data.find(
            (e) =>
              e.team_id.toString() === team.id || e.owner === team.owner
          );
          return {
            ...team,
            expectedWins: ew
              ? {
                  expectedWins: ew.total_expected_wins,
                  expectedLosses: ew.total_expected_losses,
                  winLuck: ew.total_win_luck,
                  seasonsPlayed: ew.seasons_played,
                }
              : undefined,
          };
        });
        setTeams(merged);
      })
      .finally(() => setLoading(false));
  }, [leagueId]);

  const handleSort = (field: SortField) => {
    if (field === sortField) {
      setSortDirection((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortField(field);
      setSortDirection("desc");
    }
  };

  const renderSortIcon = (field: SortField) => {
    if (sortField !== field) return null;
    return (
      <span className="ml-1 text-gray-400">
        {sortDirection === "asc" ? "↑" : "↓"}
      </span>
    );
  };

  const sortedTeams = [...teams].sort((a, b) => {
    let fieldA: string | number = 0;
    let fieldB: string | number = 0;

    switch (sortField) {
      case "owner":
        fieldA = a.owner.toLowerCase();
        fieldB = b.owner.toLowerCase();
        break;
      case "regularSeasonRecord": {
        fieldA = a.record.wins;
        fieldB = b.record.wins;
        if (fieldA === fieldB) {
          const totalA = a.record.wins + a.record.losses + a.record.ties;
          const totalB = b.record.wins + b.record.losses + b.record.ties;
          fieldA = totalA > 0 ? a.record.wins / totalA : 0;
          fieldB = totalB > 0 ? b.record.wins / totalB : 0;
        }
        break;
      }
      case "playoffRecord": {
        fieldA = a.playoffRecord.wins;
        fieldB = b.playoffRecord.wins;
        if (fieldA === fieldB) {
          const totalA =
            a.playoffRecord.wins + a.playoffRecord.losses + a.playoffRecord.ties;
          const totalB =
            b.playoffRecord.wins + b.playoffRecord.losses + b.playoffRecord.ties;
          fieldA = totalA > 0 ? a.playoffRecord.wins / totalA : 0;
          fieldB = totalB > 0 ? b.playoffRecord.wins / totalB : 0;
        }
        break;
      }
      case "pointsFor":
        fieldA = a.points.scored;
        fieldB = b.points.scored;
        break;
      case "pointsAgainst":
        fieldA = a.points.against;
        fieldB = b.points.against;
        break;
      case "expectedRecord":
        fieldA = a.expectedWins?.expectedWins ?? 0;
        fieldB = b.expectedWins?.expectedWins ?? 0;
        break;
      case "luck":
        fieldA = a.expectedWins?.winLuck ?? 0;
        fieldB = b.expectedWins?.winLuck ?? 0;
        break;
    }

    if (fieldA === fieldB) return 0;
    const result = fieldA > fieldB ? 1 : -1;
    return sortDirection === "asc" ? result : -result;
  });

  const thClass =
    "px-4 py-3 text-sm font-medium text-gray-700 dark:text-gray-300 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-500 transition-colors";

  return (
    <section className="bg-gray-100 dark:bg-gray-700 rounded-lg p-6">
      <h2 className="text-xl font-semibold mb-4">All-Time Team Records</h2>
      {loading ? (
        <div className="flex items-center space-x-2">
          <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin" />
          <p>Loading teams data...</p>
        </div>
      ) : teams.length === 0 ? (
        <p className="text-gray-500">No team data available.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full bg-white dark:bg-gray-800 rounded-lg">
            <thead className="bg-gray-50 dark:bg-gray-600">
              <tr>
                <th
                  className={`${thClass} text-left`}
                  onClick={() => handleSort("owner")}
                >
                  Owner{renderSortIcon("owner")}
                </th>
                <th
                  className={`${thClass} text-center`}
                  onClick={() => handleSort("regularSeasonRecord")}
                >
                  Regular Season Record{renderSortIcon("regularSeasonRecord")}
                </th>
                <th
                  className={`${thClass} text-center`}
                  onClick={() => handleSort("playoffRecord")}
                >
                  Playoffs Record{renderSortIcon("playoffRecord")}
                </th>
                <th
                  className={`${thClass} text-center`}
                  onClick={() => handleSort("pointsFor")}
                >
                  Points For{renderSortIcon("pointsFor")}
                </th>
                <th
                  className={`${thClass} text-center`}
                  onClick={() => handleSort("pointsAgainst")}
                >
                  Points Against{renderSortIcon("pointsAgainst")}
                </th>
                <th
                  className={`${thClass} text-center`}
                  onClick={() => handleSort("expectedRecord")}
                >
                  Expected Record (Regular Season)
                  {renderSortIcon("expectedRecord")}
                </th>
                <th
                  className={`${thClass} text-center`}
                  onClick={() => handleSort("luck")}
                >
                  Luck{renderSortIcon("luck")}
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
                      href={`/league/${leagueId}/teams/${team.espnId}`}
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
                    {team.playoffRecord.wins}-{team.playoffRecord.losses}
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
                  <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                    {team.expectedWins?.expectedWins !== undefined &&
                    team.expectedWins?.expectedLosses !== undefined
                      ? `${team.expectedWins.expectedWins.toLocaleString(
                          undefined,
                          { minimumFractionDigits: 1, maximumFractionDigits: 1 }
                        )}-${team.expectedWins.expectedLosses.toLocaleString(
                          undefined,
                          { minimumFractionDigits: 1, maximumFractionDigits: 1 }
                        )}`
                      : "N/A"}
                  </td>
                  <td className="px-4 py-3 text-sm text-center">
                    {team.expectedWins?.winLuck !== undefined ? (
                      <span
                        className={
                          team.expectedWins.winLuck > 0
                            ? "text-green-600 dark:text-green-400"
                            : team.expectedWins.winLuck < 0
                            ? "text-red-600 dark:text-red-400"
                            : "text-gray-500 dark:text-gray-400"
                        }
                      >
                        {team.expectedWins.winLuck > 0 ? "+" : ""}
                        {team.expectedWins.winLuck.toLocaleString(undefined, {
                          minimumFractionDigits: 1,
                          maximumFractionDigits: 1,
                        })}
                      </span>
                    ) : (
                      <span className="text-gray-400">N/A</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
