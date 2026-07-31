import { useEffect, useState } from "react";
import Link from "next/link";
import { teamsService, Team } from "@/services/teamsService";
import { expectedWinsService } from "@/services/expectedWinsService";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import EmptyState from "@/components/design-system/EmptyState";
import DataTable, {
  type DataTableColumn,
} from "@/components/design-system/DataTable";

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

  const handleSort = (field: string) => {
    const f = field as SortField;
    if (f === sortField) {
      setSortDirection((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortField(f);
      setSortDirection("desc");
    }
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

  const columns: DataTableColumn<Team>[] = [
    {
      id: "owner",
      header: "Owner",
      sortable: true,
      cell: (team) => (
        <Link
          href={`/league/${leagueId}/teams/${team.espnId}`}
          className="hover:underline"
          style={{ color: "var(--action-primary)" }}
        >
          {team.owner}
        </Link>
      ),
    },
    {
      id: "regularSeasonRecord",
      header: "Regular Season Record",
      sortable: true,
      align: "center",
      cell: (team) => (
        <span style={{ color: "var(--text-secondary)" }}>
          {team.record.wins}-{team.record.losses}
          {team.record.ties > 0 ? `-${team.record.ties}` : ""}
        </span>
      ),
    },
    {
      id: "playoffRecord",
      header: "Playoffs Record",
      sortable: true,
      align: "center",
      cell: (team) => (
        <span style={{ color: "var(--text-secondary)" }}>
          {team.playoffRecord.wins}-{team.playoffRecord.losses}
          {team.playoffRecord.ties > 0 ? `-${team.playoffRecord.ties}` : ""}
        </span>
      ),
    },
    {
      id: "pointsFor",
      header: "Points For",
      sortable: true,
      align: "center",
      cell: (team) => (
        <span style={{ color: "var(--text-secondary)" }}>
          {team.points.scored.toLocaleString(undefined, {
            minimumFractionDigits: 2,
            maximumFractionDigits: 2,
          })}
        </span>
      ),
    },
    {
      id: "pointsAgainst",
      header: "Points Against",
      sortable: true,
      align: "center",
      cell: (team) => (
        <span style={{ color: "var(--text-secondary)" }}>
          {team.points.against.toLocaleString(undefined, {
            minimumFractionDigits: 2,
            maximumFractionDigits: 2,
          })}
        </span>
      ),
    },
    {
      id: "expectedRecord",
      header: "Expected Record (Regular Season)",
      sortable: true,
      align: "center",
      cell: (team) => (
        <span style={{ color: "var(--text-secondary)" }}>
          {team.expectedWins?.expectedWins !== undefined &&
          team.expectedWins?.expectedLosses !== undefined
            ? `${team.expectedWins.expectedWins.toLocaleString(undefined, {
                minimumFractionDigits: 1,
                maximumFractionDigits: 1,
              })}-${team.expectedWins.expectedLosses.toLocaleString(undefined, {
                minimumFractionDigits: 1,
                maximumFractionDigits: 1,
              })}`
            : "N/A"}
        </span>
      ),
    },
    {
      id: "luck",
      header: "Luck",
      sortable: true,
      align: "center",
      cell: (team) =>
        team.expectedWins?.winLuck !== undefined ? (
          <span
            style={{
              color:
                team.expectedWins.winLuck > 0
                  ? "var(--status-success-fg)"
                  : team.expectedWins.winLuck < 0
                  ? "var(--status-danger-fg)"
                  : "var(--text-muted)",
            }}
          >
            {team.expectedWins.winLuck > 0 ? "+" : ""}
            {team.expectedWins.winLuck.toLocaleString(undefined, {
              minimumFractionDigits: 1,
              maximumFractionDigits: 1,
            })}
          </span>
        ) : (
          <span style={{ color: "var(--text-muted)" }}>N/A</span>
        ),
    },
  ];

  return (
    <Card>
      <CardContent className="p-6">
        <h2
          className="text-xl font-semibold mb-4"
          style={{ color: "var(--text-primary)" }}
        >
          All-Time Team Records
        </h2>
        {loading ? (
          <div className="space-y-2">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
        ) : teams.length === 0 ? (
          <EmptyState title="No team data available." />
        ) : (
          <DataTable
            columns={columns}
            rows={sortedTeams}
            rowKey={(team) => team.id}
            sortField={sortField}
            sortDirection={sortDirection}
            onSort={handleSort}
          />
        )}
      </CardContent>
    </Card>
  );
}
