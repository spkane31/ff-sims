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

export default function AllTimeMatchupsGrid({
  teams,
  headToHeadRecords,
}: AllTimeMatchupsGridProps) {
  // Helper function to get gradient background color based on win percentage
  const getGradientColor = (winPct: number, isDarkMode: boolean = false) => {
    // winPct is from 0-100
    const ratio = winPct / 100; // Convert to 0-1

    if (isDarkMode) {
      // Dark mode colors
      if (ratio < 0.5) {
        // 0-50%: Red gradient to neutral
        const factor = ratio * 2; // 0-1 for this range
        const r = Math.round(180 - factor * 60); // 180 -> 120 (darker red to neutral)
        const g = Math.round(80 + factor * 40); // 80 -> 120
        const b = Math.round(80 + factor * 40); // 80 -> 120
        return `rgb(${r}, ${g}, ${b})`;
      } else {
        // 50-100%: Neutral to green gradient
        const factor = (ratio - 0.5) * 2; // 0-1 for this range
        const r = Math.round(120 - factor * 40); // 120 -> 80
        const g = Math.round(120 + factor * 60); // 120 -> 180 (neutral to darker green)
        const b = Math.round(120 - factor * 40); // 120 -> 80
        return `rgb(${r}, ${g}, ${b})`;
      }
    } else {
      // Light mode colors
      if (ratio < 0.5) {
        // 0-50%: Red gradient to neutral
        const factor = ratio * 2; // 0-1 for this range
        const r = Math.round(255 - factor * 15); // 255 -> 240 (lighter red to neutral)
        const g = Math.round(200 + factor * 40); // 200 -> 240
        const b = Math.round(200 + factor * 40); // 200 -> 240
        return `rgb(${r}, ${g}, ${b})`;
      } else {
        // 50-100%: Neutral to green gradient
        const factor = (ratio - 0.5) * 2; // 0-1 for this range
        const r = Math.round(240 - factor * 40); // 240 -> 200
        const g = Math.round(240 + factor * 15); // 240 -> 255 (neutral to lighter green)
        const b = Math.round(240 - factor * 40); // 240 -> 200
        return `rgb(${r}, ${g}, ${b})`;
      }
    }
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
    <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
      <h2 className="text-xl font-semibold mb-4">Head-to-Head Records</h2>
      <p className="text-sm text-gray-600 dark:text-gray-400 mb-6">
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
                  className="sticky left-0 z-20 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-600 p-2 min-w-[120px]"
                >
                  <div className="flex flex-col items-start">
                    <span className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
                      Team
                    </span>
                    <span className="text-xs text-gray-600 dark:text-gray-300 normal-case">
                      (Wins by Team ↓)
                    </span>
                  </div>
                </th>

                {/* Column title spanning all opponent columns */}
                <th
                  colSpan={displayTeams.length}
                  className="bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-600 p-2"
                >
                  <span className="text-xs font-medium text-gray-600 dark:text-gray-300 uppercase">
                    Losses vs Opponent →
                  </span>
                </th>

                {/* Total column title */}
                <th
                  rowSpan={2}
                  className="bg-gray-100 dark:bg-gray-900 border border-gray-300 dark:border-gray-600 p-2 min-w-[90px]"
                >
                  <span className="text-xs font-medium text-gray-700 dark:text-gray-300 uppercase">
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
                    className="bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-600 p-2 min-w-[80px]"
                  >
                    <div className="flex flex-col items-center">
                      <span className="text-xs font-medium text-gray-700 dark:text-gray-300 text-center">
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
                  <th className="sticky left-0 z-10 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-600 p-2 text-left">
                    <span className="text-sm font-medium text-gray-700 dark:text-gray-300">
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
                          className="bg-gray-100 dark:bg-gray-900 border border-gray-300 dark:border-gray-600 p-2"
                        >
                          <div className="flex items-center justify-center h-12">
                            <span className="text-gray-400 dark:text-gray-600">
                              —
                            </span>
                          </div>
                        </td>
                      );
                    }

                    const totalGames = record.wins + record.losses;
                    const winPct =
                      totalGames > 0 ? (record.wins / totalGames) * 100 : 50; // Default to neutral if no games

                    return (
                      <td
                        key={`cell-${rowTeam.espnId}-${colTeam.espnId}`}
                        className="border border-gray-300 dark:border-gray-600 p-2 transition-all relative"
                      >
                        {/* Light mode background */}
                        <div
                          className="flex flex-col items-center justify-center h-12 rounded px-2 dark:hidden"
                          style={{
                            backgroundColor: getGradientColor(winPct, false),
                          }}
                        >
                          <span className="text-sm font-semibold text-gray-900">
                            {record.wins}-{record.losses}
                          </span>
                          {totalGames > 0 && (
                            <span className="text-xs text-gray-700">
                              {winPct.toFixed(0)}%
                            </span>
                          )}
                        </div>
                        {/* Dark mode background */}
                        <div
                          className="hidden dark:flex flex-col items-center justify-center h-12 rounded px-2"
                          style={{
                            backgroundColor: getGradientColor(winPct, true),
                          }}
                        >
                          <span className="text-sm font-semibold text-white">
                            {record.wins}-{record.losses}
                          </span>
                          {totalGames > 0 && (
                            <span className="text-xs text-gray-200">
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

                    return (
                      <td
                        key={`total-${rowTeam.espnId}`}
                        className="bg-gray-100 dark:bg-gray-900 border border-gray-300 dark:border-gray-600 p-2 transition-all relative"
                      >
                        {/* Light mode background */}
                        <div
                          className="flex flex-col items-center justify-center h-12 rounded px-2 dark:hidden"
                          style={{
                            backgroundColor: getGradientColor(winPct, false),
                          }}
                        >
                          <span className="text-sm font-bold text-gray-900">
                            {rowTotal.wins}-{rowTotal.losses}
                          </span>
                          {totalGames > 0 && (
                            <span className="text-xs text-gray-700">
                              {winPct.toFixed(0)}%
                            </span>
                          )}
                        </div>
                        {/* Dark mode background */}
                        <div
                          className="hidden dark:flex flex-col items-center justify-center h-12 rounded px-2"
                          style={{
                            backgroundColor: getGradientColor(winPct, true),
                          }}
                        >
                          <span className="text-sm font-bold text-white">
                            {rowTotal.wins}-{rowTotal.losses}
                          </span>
                          {totalGames > 0 && (
                            <span className="text-xs text-gray-200">
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
                <th className="sticky left-0 z-10 bg-gray-100 dark:bg-gray-900 border border-gray-300 dark:border-gray-600 p-2 text-left">
                  <span className="text-sm font-medium text-gray-700 dark:text-gray-300 uppercase">
                    Total
                  </span>
                </th>

                {/* Column total cells */}
                {displayTeams.map((colTeam) => {
                  const colTotal = getColTotal(colTeam.espnId);
                  const totalGames = colTotal.wins + colTotal.losses;
                  const winPct =
                    totalGames > 0 ? (colTotal.wins / totalGames) * 100 : 50;

                  return (
                    <td
                      key={`col-total-${colTeam.espnId}`}
                      className="bg-gray-100 dark:bg-gray-900 border border-gray-300 dark:border-gray-600 p-2 transition-all relative"
                    >
                      {/* Light mode background */}
                      <div
                        className="flex flex-col items-center justify-center h-12 rounded px-2 dark:hidden"
                        style={{
                          backgroundColor: getGradientColor(winPct, false),
                        }}
                      >
                        <span className="text-sm font-bold text-gray-900">
                          {colTotal.wins}-{colTotal.losses}
                        </span>
                        {totalGames > 0 && (
                          <span className="text-xs text-gray-700">
                            {winPct.toFixed(0)}%
                          </span>
                        )}
                      </div>
                      {/* Dark mode background */}
                      <div
                        className="hidden dark:flex flex-col items-center justify-center h-12 rounded px-2"
                        style={{
                          backgroundColor: getGradientColor(winPct, true),
                        }}
                      >
                        <span className="text-sm font-bold text-white">
                          {colTotal.wins}-{colTotal.losses}
                        </span>
                        {totalGames > 0 && (
                          <span className="text-xs text-gray-200">
                            {winPct.toFixed(0)}%
                          </span>
                        )}
                      </div>
                    </td>
                  );
                })}

                {/* Bottom-right corner cell (total of totals) */}
                <td className="bg-gray-200 dark:bg-gray-800 border border-gray-300 dark:border-gray-600 p-2">
                  <div className="flex items-center justify-center h-12">
                    <span className="text-gray-400 dark:text-gray-600">—</span>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
