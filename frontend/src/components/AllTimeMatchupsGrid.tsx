import { Card, CardContent } from "@/components/ui/card";

interface Team {
  id: string;
  espnId: string;
  name: string;
  owner: string;
}

interface AllTimeMatchupsGridProps {
  teams?: Team[];
  headToHeadRecords?: Map<
    string,
    Map<string, { wins: number; losses: number }>
  >;
}

interface CellColors {
  background: string;
  foreground: string;
}

export default function AllTimeMatchupsGrid({
  teams,
  headToHeadRecords,
}: AllTimeMatchupsGridProps) {
  // Status-token color pair for a win/loss cell based on win percentage:
  // >50% is a win-leaning record (success), <50% is a loss-leaning record
  // (danger), and exactly 50% (including no games played) is neutral.
  const getCellColors = (winPct: number): CellColors => {
    if (winPct > 50) {
      return {
        background: "var(--status-success-bg)",
        foreground: "var(--status-success-fg)",
      };
    }
    if (winPct < 50) {
      return {
        background: "var(--status-danger-bg)",
        foreground: "var(--status-danger-fg)",
      };
    }
    return {
      background: "var(--surface-sunken)",
      foreground: "var(--text-secondary)",
    };
  };

  // Get head-to-head record for rowTeam vs colTeam (from rowTeam's perspective)
  const getRecord = (
    rowTeamESPNId: string,
    colTeamESPNId: string
  ): { wins: number; losses: number } | null => {
    if (rowTeamESPNId === colTeamESPNId) return null; // Team doesn't play itself

    if (!headToHeadRecords) {
      return { wins: 0, losses: 0 };
    }

    const record = headToHeadRecords.get(rowTeamESPNId)?.get(colTeamESPNId);
    return record || { wins: 0, losses: 0 };
  };

  // Calculate total record for a team (as row team - their wins/losses against all opponents)
  const getRowTotal = (
    teamESPNId: string
  ): { wins: number; losses: number } => {
    if (!headToHeadRecords) {
      return { wins: 0, losses: 0 };
    }

    let totalWins = 0;
    let totalLosses = 0;

    const teamRecords = headToHeadRecords.get(teamESPNId);
    if (teamRecords) {
      teamRecords.forEach((record) => {
        totalWins += record.wins;
        totalLosses += record.losses;
      });
    }

    return { wins: totalWins, losses: totalLosses };
  };

  // Calculate total record for a team (as column team - how all opponents did against them)
  const getColTotal = (
    teamESPNId: string
  ): { wins: number; losses: number } => {
    if (!headToHeadRecords) {
      return { wins: 0, losses: 0 };
    }

    let totalWins = 0;
    let totalLosses = 0;

    // Iterate through all teams' records to find games against this team
    headToHeadRecords.forEach((opponentRecords, opponentId) => {
      if (opponentId !== teamESPNId) {
        const record = opponentRecords.get(teamESPNId);
        if (record) {
          // Opponent's wins/losses against this team (from opponent's perspective)
          totalWins += record.wins;
          totalLosses += record.losses;
        }
      }
    });

    return { wins: totalWins, losses: totalLosses };
  };

  const displayTeams = teams || [];

  return (
    <Card>
      <CardContent className="p-6">
        <h2
          className="text-xl font-semibold mb-4"
          style={{ color: "var(--text-primary)" }}
        >
          Head-to-Head Records
        </h2>
        <p className="text-sm mb-6" style={{ color: "var(--text-muted)" }}>
          All-time records between teams. Rows show team&apos;s record vs column
          team.
        </p>

        <div className="overflow-x-auto">
          <div className="flex justify-center">
            <table className="border-collapse">
              <thead>
                {/* Title row */}
                <tr>
                  {/* Team name column header */}
                  <th
                    rowSpan={2}
                    className="sticky left-0 z-20 border p-2 min-w-[120px]"
                    style={{
                      backgroundColor: "var(--surface-sunken)",
                      borderColor: "var(--border-subtle)",
                    }}
                  >
                    <div className="flex flex-col items-start">
                      <span
                        className="text-xs font-medium uppercase"
                        style={{ color: "var(--text-muted)" }}
                      >
                        Team
                      </span>
                      <span
                        className="text-xs normal-case"
                        style={{ color: "var(--text-secondary)" }}
                      >
                        (Wins by Team ↓)
                      </span>
                    </div>
                  </th>

                  {/* Column title spanning all opponent columns */}
                  <th
                    colSpan={displayTeams.length}
                    className="border p-2"
                    style={{
                      backgroundColor: "var(--surface-sunken)",
                      borderColor: "var(--border-subtle)",
                    }}
                  >
                    <span
                      className="text-xs font-medium uppercase"
                      style={{ color: "var(--text-secondary)" }}
                    >
                      Losses vs Opponent →
                    </span>
                  </th>

                  {/* Total column title */}
                  <th
                    rowSpan={2}
                    className="border p-2 min-w-[90px]"
                    style={{
                      backgroundColor: "var(--surface-sunken)",
                      borderColor: "var(--border-subtle)",
                    }}
                  >
                    <span
                      className="text-xs font-medium uppercase"
                      style={{ color: "var(--text-secondary)" }}
                    >
                      Total
                    </span>
                  </th>
                </tr>

                {/* Team name row */}
                <tr>
                  {/* Column headers - team names */}
                  {displayTeams.map((team) => (
                    <th
                      key={`col-${team.espnId}`}
                      className="border p-2 min-w-[80px]"
                      style={{
                        backgroundColor: "var(--surface-sunken)",
                        borderColor: "var(--border-subtle)",
                      }}
                    >
                      <div className="flex flex-col items-center">
                        <span
                          className="text-xs font-medium text-center"
                          style={{ color: "var(--text-secondary)" }}
                        >
                          {team.owner || team.name}
                        </span>
                      </div>
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {displayTeams.map((rowTeam) => (
                  <tr key={`row-${rowTeam.espnId}`}>
                    {/* Team name row header */}
                    <th
                      className="sticky left-0 z-10 border p-2 text-left"
                      style={{
                        backgroundColor: "var(--surface-sunken)",
                        borderColor: "var(--border-subtle)",
                      }}
                    >
                      <span
                        className="text-sm font-medium"
                        style={{ color: "var(--text-secondary)" }}
                      >
                        {rowTeam.owner || rowTeam.name}
                      </span>
                    </th>

                    {/* Data cells */}
                    {displayTeams.map((colTeam) => {
                      const record = getRecord(rowTeam.espnId, colTeam.espnId);

                      if (!record) {
                        // Diagonal - team vs itself
                        return (
                          <td
                            key={`cell-${rowTeam.espnId}-${colTeam.espnId}`}
                            className="border p-2"
                            style={{
                              backgroundColor: "var(--surface-sunken)",
                              borderColor: "var(--border-subtle)",
                            }}
                          >
                            <div className="flex items-center justify-center h-12">
                              <span style={{ color: "var(--text-muted)" }}>
                                —
                              </span>
                            </div>
                          </td>
                        );
                      }

                      const totalGames = record.wins + record.losses;
                      const winPct =
                        totalGames > 0 ? (record.wins / totalGames) * 100 : 50; // Default to neutral if no games
                      const colors = getCellColors(winPct);

                      return (
                        <td
                          key={`cell-${rowTeam.espnId}-${colTeam.espnId}`}
                          className="border p-2"
                          style={{ borderColor: "var(--border-subtle)" }}
                        >
                          <div
                            className="flex flex-col items-center justify-center h-12 rounded px-2"
                            style={{ backgroundColor: colors.background }}
                          >
                            <span
                              className="text-sm font-semibold"
                              style={{ color: colors.foreground }}
                            >
                              {record.wins}-{record.losses}
                            </span>
                            {totalGames > 0 && (
                              <span
                                className="text-xs opacity-80"
                                style={{ color: colors.foreground }}
                              >
                                {winPct.toFixed(0)}%
                              </span>
                            )}
                          </div>
                        </td>
                      );
                    })}

                    {/* Row total cell */}
                    {(() => {
                      const rowTotal = getRowTotal(rowTeam.espnId);
                      const totalGames = rowTotal.wins + rowTotal.losses;
                      const winPct =
                        totalGames > 0 ? (rowTotal.wins / totalGames) * 100 : 50;
                      const colors = getCellColors(winPct);

                      return (
                        <td
                          key={`total-${rowTeam.espnId}`}
                          className="border p-2"
                          style={{
                            backgroundColor: "var(--surface-sunken)",
                            borderColor: "var(--border-subtle)",
                          }}
                        >
                          <div
                            className="flex flex-col items-center justify-center h-12 rounded px-2"
                            style={{ backgroundColor: colors.background }}
                          >
                            <span
                              className="text-sm font-bold"
                              style={{ color: colors.foreground }}
                            >
                              {rowTotal.wins}-{rowTotal.losses}
                            </span>
                            {totalGames > 0 && (
                              <span
                                className="text-xs opacity-80"
                                style={{ color: colors.foreground }}
                              >
                                {winPct.toFixed(0)}%
                              </span>
                            )}
                          </div>
                        </td>
                      );
                    })()}
                  </tr>
                ))}

                {/* Total row */}
                <tr>
                  {/* Total row header */}
                  <th
                    className="sticky left-0 z-10 border p-2 text-left"
                    style={{
                      backgroundColor: "var(--surface-sunken)",
                      borderColor: "var(--border-subtle)",
                    }}
                  >
                    <span
                      className="text-sm font-medium uppercase"
                      style={{ color: "var(--text-secondary)" }}
                    >
                      Total
                    </span>
                  </th>

                  {/* Column total cells */}
                  {displayTeams.map((colTeam) => {
                    const colTotal = getColTotal(colTeam.espnId);
                    const totalGames = colTotal.wins + colTotal.losses;
                    const winPct =
                      totalGames > 0 ? (colTotal.wins / totalGames) * 100 : 50;
                    const colors = getCellColors(winPct);

                    return (
                      <td
                        key={`col-total-${colTeam.espnId}`}
                        className="border p-2"
                        style={{
                          backgroundColor: "var(--surface-sunken)",
                          borderColor: "var(--border-subtle)",
                        }}
                      >
                        <div
                          className="flex flex-col items-center justify-center h-12 rounded px-2"
                          style={{ backgroundColor: colors.background }}
                        >
                          <span
                            className="text-sm font-bold"
                            style={{ color: colors.foreground }}
                          >
                            {colTotal.wins}-{colTotal.losses}
                          </span>
                          {totalGames > 0 && (
                            <span
                              className="text-xs opacity-80"
                              style={{ color: colors.foreground }}
                            >
                              {winPct.toFixed(0)}%
                            </span>
                          )}
                        </div>
                      </td>
                    );
                  })}

                  {/* Bottom-right corner cell (total of totals) */}
                  <td
                    className="border p-2"
                    style={{
                      backgroundColor: "var(--surface-sunken)",
                      borderColor: "var(--border-subtle)",
                    }}
                  >
                    <div className="flex items-center justify-center h-12">
                      <span style={{ color: "var(--text-muted)" }}>—</span>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
