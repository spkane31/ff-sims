import { useRouter } from 'next/router';
import Layout from '../../components/Layout';
import { useMatchupDetail } from '../../hooks/useMatchupDetail';
import Link from 'next/link';

export default function MatchupDetail() {
  const router = useRouter();
  const { id } = router.query;
  const { matchup, isLoading, error } = useMatchupDetail(id as string);

  // Mock data for demonstration (will be replaced by real data from API)
  // const mockData = {
  //   data: {
  //     id: '1',
  //     year: 2024,
  //     week: 3,
  //     homeTeam: {
  //       id: 1,
  //       name: 'Team Alpha',
  //       score: 125.6,
  //       projectedScore: 118.2,
  //       players: [
  //         { id: '101', name: 'Patrick Mahomes', position: 'QB', team: 'KC', projectedPoints: 24.5, actualPoints: 27.8, isStarter: true },
  //         { id: '102', name: 'Austin Ekeler', position: 'RB', team: 'LAC', projectedPoints: 18.2, actualPoints: 15.6, isStarter: true },
  //         { id: '103', name: 'Tyreek Hill', position: 'WR', team: 'MIA', projectedPoints: 20.1, actualPoints: 26.3, isStarter: true },
  //         { id: '104', name: 'Travis Kelce', position: 'TE', team: 'KC', projectedPoints: 14.8, actualPoints: 16.2, isStarter: true },
  //         { id: '105', name: 'Justin Jefferson', position: 'WR', team: 'MIN', projectedPoints: 19.3, actualPoints: 22.7, isStarter: true },
  //         { id: '106', name: 'Derrick Henry', position: 'RB', team: 'BAL', projectedPoints: 17.5, actualPoints: 13.8, isStarter: true },
  //         { id: '107', name: 'Eagles D/ST', position: 'DST', team: 'PHI', projectedPoints: 7.2, actualPoints: 3.0, isStarter: true },
  //         { id: '108', name: 'Harrison Butker', position: 'K', team: 'KC', projectedPoints: 8.5, actualPoints: 10.0, isStarter: true },
  //         { id: '109', name: 'DeVon Achane', position: 'RB', team: 'MIA', projectedPoints: 12.6, actualPoints: 10.2, isStarter: false },
  //         { id: '110', name: 'DK Metcalf', position: 'WR', team: 'SEA', projectedPoints: 13.7, actualPoints: 6.4, isStarter: false },
  //       ]
  //     },
  //     awayTeam: {
  //       id: 2,
  //       name: 'Team Omega',
  //       score: 118.3,
  //       projectedScore: 122.5,
  //       players: [
  //         { id: '201', name: 'Josh Allen', position: 'QB', team: 'BUF', projectedPoints: 23.8, actualPoints: 20.2, isStarter: true },
  //         { id: '202', name: 'Christian McCaffrey', position: 'RB', team: 'SF', projectedPoints: 22.5, actualPoints: 18.4, isStarter: true },
  //         { id: '203', name: 'CeeDee Lamb', position: 'WR', team: 'DAL', projectedPoints: 18.3, actualPoints: 16.7, isStarter: true },
  //         { id: '204', name: 'Mark Andrews', position: 'TE', team: 'BAL', projectedPoints: 13.6, actualPoints: 11.2, isStarter: true },
  //         { id: '205', name: 'JaMarr Chase', position: 'WR', team: 'CIN', projectedPoints: 18.9, actualPoints: 24.5, isStarter: true },
  //         { id: '206', name: 'Saquon Barkley', position: 'RB', team: 'PHI', projectedPoints: 16.8, actualPoints: 14.3, isStarter: true },
  //         { id: '207', name: '49ers D/ST', position: 'DST', team: 'SF', projectedPoints: 8.6, actualPoints: 12.0, isStarter: true },
  //         { id: '208', name: 'Justin Tucker', position: 'K', team: 'BAL', projectedPoints: 8.0, actualPoints: 9.0, isStarter: true },
  //         { id: '209', name: 'Kenneth Walker III', position: 'RB', team: 'SEA', projectedPoints: 13.4, actualPoints: 8.2, isStarter: false },
  //         { id: '210', name: 'Mike Evans', position: 'WR', team: 'TB', projectedPoints: 14.2, actualPoints: 11.8, isStarter: false },
  //       ]
  //     },
  //     matchupStatistics: {
  //       pointDifferential: 7.3,
  //       accuracyPercentage: 89,
  //       playoffImplications: "High - Winner moves to first place in division",
  //       winProbability: 62.5
  //     }
  //   }
  // };


  if (isLoading || matchup === null) {
    return (
      <Layout>
        <div className="flex items-center justify-center min-h-screen">
          <div className="w-8 h-8 border-4 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
          <span className="ml-2 text-lg">Loading matchup details...</span>
        </div>
      </Layout>
    );
  }

  if (error) {
    return (
      <Layout>
        <div className="bg-red-100 dark:bg-red-900 p-6 rounded-lg text-red-700 dark:text-red-200 max-w-4xl mx-auto my-8">
          <h2 className="text-xl font-semibold mb-2">Error loading matchup details</h2>
          <p>{error.message}</p>
          <Link href="/schedule" className="mt-4 inline-block px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700">
            Return to Schedule
          </Link>
        </div>
      </Layout>
    );
  }



  // Use mock data for now, later it will use real API data
  const matchupData = matchup ;//|| mockData;
  // const matchupData = matchup; //mockData;

  const { data } = matchupData;
  const { homeTeam, awayTeam, year, week } = data;
  const matchupStatistics = {
        pointDifferential: 7.3,
        accuracyPercentage: 89,
        playoffImplications: "High - Winner moves to first place in division",
        winProbability: 62.5
      }

  // Calculate winner
  const homeWon = homeTeam.score > awayTeam.score;
  const awayWon = awayTeam.score > homeTeam.score;
  const isTied = homeTeam.score === awayTeam.score;

  // Calculate if game is completed
  const isCompleted = homeTeam.score > 0 || awayTeam.score > 0;

  // Calculate projected vs actual differential
  const homeProjectedDiff = homeTeam.score - homeTeam.projectedScore;
  const awayProjectedDiff = awayTeam.score - awayTeam.projectedScore;

  return (
    <Layout>
      <div className="max-w-7xl mx-auto px-4 py-8">
        {/* Breadcrumbs and title */}
        <div className="mb-6">
          <div className="flex items-center text-gray-500 dark:text-gray-400 mb-2">
            <Link href="/schedule" className="hover:text-blue-600">Schedule</Link>
            <span className="mx-2">›</span>
            <span>Year {year}, Week {week}</span>
          </div>
          <h1 className="text-3xl md:text-4xl font-bold text-blue-600 mb-2">
            {homeTeam.name} vs {awayTeam.name}
          </h1>
          <div className="text-lg text-gray-600 dark:text-gray-400">
            Week {week}, Year {year}
          </div>
        </div>

        {/* Matchup summary card */}
        <div className="bg-white dark:bg-gray-800 shadow-md rounded-lg p-6 mb-8">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-8 items-center">
            {/* Home Team */}
            <div className={`text-center ${homeWon ? 'bg-green-50 dark:bg-green-900/20 p-4 rounded-lg' : ''}`}>
              <div className="text-xl font-semibold mb-1">{homeTeam.name}</div>
              <div className={`text-4xl font-bold mb-2 ${homeWon ? 'text-green-600 dark:text-green-400' : ''}`}>
                {homeTeam.score.toFixed(1)}
              </div>
              <div className="text-sm text-gray-500 dark:text-gray-400">
                Projected: {homeTeam.projectedScore.toFixed(1)}
                {homeProjectedDiff !== 0 && (
                  <span className={`ml-2 ${homeProjectedDiff > 0 ? 'text-green-600' : 'text-red-600'}`}>
                    ({homeProjectedDiff > 0 ? '+' : ''}{homeProjectedDiff.toFixed(1)})
                  </span>
                )}
              </div>
              {homeWon && isCompleted && <div className="mt-2 text-green-600 font-medium">WINNER</div>}
            </div>

            {/* Matchup Status */}
            <div className="flex flex-col items-center justify-center text-center">
              <div className="text-lg mb-2">
                {isCompleted ? (
                  <span className="px-3 py-1 bg-green-100 dark:bg-green-800 text-green-800 dark:text-green-100 rounded-full text-sm font-medium">
                    Final
                  </span>
                ) : (
                  <span className="px-3 py-1 bg-blue-100 dark:bg-blue-800 text-blue-800 dark:text-blue-100 rounded-full text-sm font-medium">
                    Upcoming
                  </span>
                )}
              </div>

              <div className="text-2xl font-bold my-2">vs</div>

              {isCompleted && (
                <div className="text-sm text-gray-500 dark:text-gray-400 mb-2">
                  Point Differential: <span className="font-medium">{(homeTeam.score - awayTeam.score).toFixed(2)}</span>
                </div>
              )}

              {!isCompleted && (
                <div className="text-sm text-gray-500 dark:text-gray-400">
                  Win Probability: <span className="font-medium">{matchupStatistics.winProbability}%</span>
                </div>
              )}
            </div>

            {/* Away Team */}
            <div className={`text-center ${awayWon ? 'bg-green-50 dark:bg-green-900/20 p-4 rounded-lg' : ''}`}>
              <div className="text-xl font-semibold mb-1">{awayTeam.name}</div>
              <div className={`text-4xl font-bold mb-2 ${awayWon ? 'text-green-600 dark:text-green-400' : ''}`}>
                {awayTeam.score.toFixed(1)}
              </div>
              <div className="text-sm text-gray-500 dark:text-gray-400">
                Projected: {awayTeam.projectedScore.toFixed(1)}
                {awayProjectedDiff !== 0 && (
                  <span className={`ml-2 ${awayProjectedDiff > 0 ? 'text-green-600' : 'text-red-600'}`}>
                    ({awayProjectedDiff > 0 ? '+' : ''}{awayProjectedDiff.toFixed(1)})
                  </span>
                )}
              </div>
              {awayWon && isCompleted && <div className="mt-2 text-green-600 font-medium">WINNER</div>}
            </div>
          </div>
        </div>

        {/* Game Statistics */}
        <div className="bg-white dark:bg-gray-800 shadow-md rounded-lg p-6 mb-8">
          <h2 className="text-xl font-semibold mb-4 pb-2 border-b border-gray-200 dark:border-gray-700">
            Matchup Statistics
          </h2>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
            <div>
              <div className="text-sm text-gray-500 dark:text-gray-400">Point Differential</div>
              <div className="text-xl font-semibold">{matchupStatistics.pointDifferential.toFixed(1)}</div>
            </div>

            <div>
              <div className="text-sm text-gray-500 dark:text-gray-400">Projection Accuracy</div>
              <div className="text-xl font-semibold">{matchupStatistics.accuracyPercentage}%</div>
            </div>

            <div>
              <div className="text-sm text-gray-500 dark:text-gray-400">Playoff Implications</div>
              <div className="text-xl font-semibold">{matchupStatistics.playoffImplications}</div>
            </div>

            <div>
              <div className="text-sm text-gray-500 dark:text-gray-400">Win Probability</div>
              <div className="text-xl font-semibold">{matchupStatistics.winProbability}%</div>
            </div>
          </div>
        </div>

        {/* Team Lineups */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-8 mb-8">
          {/* Home Team Lineup */}
          <div className="bg-white dark:bg-gray-800 shadow-md rounded-lg p-6">
            <h2 className="text-xl font-semibold mb-4">
              {homeTeam.name} Lineup
            </h2>

            <div className="overflow-x-auto">
              <table className="min-w-full">
                <thead>
                  <tr className="border-b border-gray-200 dark:border-gray-700">
                    <th className="py-2 px-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Player</th>
                    <th className="py-2 px-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Pos</th>
                    <th className="py-2 px-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Proj</th>
                    <th className="py-2 px-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Actual</th>
                    <th className="py-2 px-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Diff</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                  {homeTeam.players.map(player => {
                    const diff = player.points - player.projectedPoints;
                    return (
                      <tr key={player.id} className={`${!player.isStarter ? 'text-gray-500 dark:text-gray-400 bg-gray-50 dark:bg-gray-700/50' : ''}`}>
                        <td className="py-2 px-3">
                          <div className="flex items-center">
                            {!player.isStarter && (
                              <span className="inline-block w-4 mr-2 text-gray-400">BN</span>
                            )}
                            <span>{player.playerName}</span>
                          </div>
                        </td>
                        <td className="py-2 px-3">{player.playerPosition}</td>
                        <td className="py-2 px-3 text-right">{player.projectedPoints.toFixed(1)}</td>
                        <td className="py-2 px-3 text-right">{player.points.toFixed(1)}</td>
                        <td className={`py-2 px-3 text-right ${diff > 0 ? 'text-green-600 dark:text-green-400' : diff < 0 ? 'text-red-600 dark:text-red-400' : ''}`}>
                          {diff > 0 && '+'}{diff.toFixed(1)}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
                <tfoot>
                  <tr className="border-t border-gray-300 dark:border-gray-600 font-semibold">
                    <td colSpan={2} className="py-2 px-3 text-left">Total</td>
                    <td className="py-2 px-3 text-right">{homeTeam.projectedScore.toFixed(1)}</td>
                    <td className="py-2 px-3 text-right">{homeTeam.score.toFixed(1)}</td>
                    <td className={`py-2 px-3 text-right ${homeProjectedDiff > 0 ? 'text-green-600 dark:text-green-400' : homeProjectedDiff < 0 ? 'text-red-600 dark:text-red-400' : ''}`}>
                      {homeProjectedDiff > 0 && '+'}{homeProjectedDiff.toFixed(1)}
                    </td>
                  </tr>
                </tfoot>
              </table>
            </div>
          </div>

          {/* Away Team Lineup */}
          <div className="bg-white dark:bg-gray-800 shadow-md rounded-lg p-6">
            <h2 className="text-xl font-semibold mb-4">
              {awayTeam.name} Lineup
            </h2>

            <div className="overflow-x-auto">
              <table className="min-w-full">
                <thead>
                  <tr className="border-b border-gray-200 dark:border-gray-700">
                    <th className="py-2 px-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Player</th>
                    <th className="py-2 px-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Pos</th>
                    <th className="py-2 px-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Proj</th>
                    <th className="py-2 px-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Actual</th>
                    <th className="py-2 px-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Diff</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                  {awayTeam.players.map(player => {
                    const diff = player.points - player.projectedPoints;
                    return (
                      <tr key={player.id} className={`${!player.isStarter ? 'text-gray-500 dark:text-gray-400 bg-gray-50 dark:bg-gray-700/50' : ''}`}>
                        <td className="py-2 px-3">
                          <div className="flex items-center">
                            {!player.isStarter && (
                              <span className="inline-block w-4 mr-2 text-gray-400">BN</span>
                            )}
                            <span>{player.playerName}</span>
                          </div>
                        </td>
                        <td className="py-2 px-3">{player.playerPosition}</td>
                        <td className="py-2 px-3 text-right">{player.projectedPoints.toFixed(1)}</td>
                        <td className="py-2 px-3 text-right">{player.points.toFixed(1)}</td>
                        <td className={`py-2 px-3 text-right ${diff > 0 ? 'text-green-600 dark:text-green-400' : diff < 0 ? 'text-red-600 dark:text-red-400' : ''}`}>
                          {diff > 0 && '+'}{diff.toFixed(1)}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
                <tfoot>
                  <tr className="border-t border-gray-300 dark:border-gray-600 font-semibold">
                    <td colSpan={2} className="py-2 px-3 text-left">Total</td>
                    <td className="py-2 px-3 text-right">{awayTeam.projectedScore.toFixed(1)}</td>
                    <td className="py-2 px-3 text-right">{awayTeam.score.toFixed(1)}</td>
                    <td className={`py-2 px-3 text-right ${awayProjectedDiff > 0 ? 'text-green-600 dark:text-green-400' : awayProjectedDiff < 0 ? 'text-red-600 dark:text-red-400' : ''}`}>
                      {awayProjectedDiff > 0 && '+'}{awayProjectedDiff.toFixed(1)}
                    </td>
                  </tr>
                </tfoot>
              </table>
            </div>
          </div>
        </div>

        {/* Key Performances */}
        <div className="bg-white dark:bg-gray-800 shadow-md rounded-lg p-6 mb-8">
          <h2 className="text-xl font-semibold mb-4 pb-2 border-b border-gray-200 dark:border-gray-700">
            Key Performances
          </h2>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            {/* Best Performers */}
            <div>
              <h3 className="text-lg font-medium mb-3">Top Performers</h3>
              <ul className="space-y-3">
                {[...homeTeam.players, ...awayTeam.players]
                  .filter(p => p.isStarter)
                  .sort((a, b) => b.points - a.points)
                  .slice(0, 3)
                  .map(player => (
                    <li key={player.id} className="flex items-center justify-between bg-gray-50 dark:bg-gray-700 p-3 rounded-lg">
                      <div>
                        <div className="font-medium">{player.playerName}</div>
                        <div className="text-sm text-gray-500 dark:text-gray-400">{player.playerPosition} · {player.team}</div>
                      </div>
                      <div className="text-xl font-semibold">{player.points.toFixed(1)}</div>
                    </li>
                  ))}
              </ul>
            </div>

            {/* Underperformers */}
            <div>
              <h3 className="text-lg font-medium mb-3">Biggest Underperformers</h3>
              <ul className="space-y-3">
                {[...homeTeam.players, ...awayTeam.players]
                  .filter(p => p.isStarter)
                  .sort((a, b) => (a.points - a.projectedPoints) - (b.points - b.projectedPoints))
                  .slice(0, 3)
                  .map(player => {
                    const diff = player.points - player.projectedPoints;
                    return (
                      <li key={player.id} className="flex items-center justify-between bg-gray-50 dark:bg-gray-700 p-3 rounded-lg">
                        <div>
                          <div className="font-medium">{player.playerName}</div>
                          <div className="text-sm text-gray-500 dark:text-gray-400">{player.playerPosition} · {player.team}</div>
                        </div>
                        <div className="text-right">
                          <div className="text-xl font-semibold">{player.points.toFixed(1)}</div>
                          <div className="text-sm text-red-600">
                            {diff > 0 ? '+' : ''}{diff.toFixed(1)} vs proj
                          </div>
                        </div>
                      </li>
                    );
                  })}
              </ul>
            </div>
          </div>
        </div>

        {/* Points Left on Bench */}
        <div className="bg-white dark:bg-gray-800 shadow-md rounded-lg p-6">
          <h2 className="text-xl font-semibold mb-4 pb-2 border-b border-gray-200 dark:border-gray-700">
            Bench Analysis
          </h2>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
            {/* Home Team Bench */}
            <div>
              <h3 className="text-lg font-medium mb-3">{homeTeam.name} Bench</h3>

              {homeTeam.players.filter(p => !p.isStarter).length > 0 ? (
                <>
                  <div className="mb-4">
                    <div className="text-sm text-gray-500 dark:text-gray-400">Total Points on Bench</div>
                    <div className="text-2xl font-semibold">
                      {homeTeam.players.filter(p => !p.isStarter).reduce((sum, p) => sum + p.points, 0).toFixed(1)}
                    </div>
                  </div>

                  <ul className="space-y-2">
                    {homeTeam.players
                      .filter(p => !p.isStarter)
                      .sort((a, b) => b.points - a.points)
                      .map(player => (
                        <li key={player.id} className="flex items-center justify-between">
                          <div>
                            <span className="font-medium">{player.playerName}</span>
                            <span className="text-sm text-gray-500 dark:text-gray-400 ml-2">{player.playerPosition}</span>
                          </div>
                          <div className="font-medium">{player.points.toFixed(1)}</div>
                        </li>
                      ))}
                  </ul>
                </>
              ) : (
                <div className="text-gray-500 dark:text-gray-400">No bench players</div>
              )}
            </div>

            {/* Away Team Bench */}
            <div>
              <h3 className="text-lg font-medium mb-3">{awayTeam.name} Bench</h3>

              {awayTeam.players.filter(p => !p.isStarter).length > 0 ? (
                <>
                  <div className="mb-4">
                    <div className="text-sm text-gray-500 dark:text-gray-400">Total Points on Bench</div>
                    <div className="text-2xl font-semibold">
                      {awayTeam.players.filter(p => !p.isStarter).reduce((sum, p) => sum + p.points, 0).toFixed(1)}
                    </div>
                  </div>

                  <ul className="space-y-2">
                    {awayTeam.players
                      .filter(p => !p.isStarter)
                      .sort((a, b) => b.points - a.points)
                      .map(player => (
                        <li key={player.id} className="flex items-center justify-between">
                          <div>
                            <span className="font-medium">{player.playerName}</span>
                            <span className="text-sm text-gray-500 dark:text-gray-400 ml-2">{player.playerPosition}</span>
                          </div>
                          <div className="font-medium">{player.points.toFixed(1)}</div>
                        </li>
                      ))}
                  </ul>
                </>
              ) : (
                <div className="text-gray-500 dark:text-gray-400">No bench players</div>
              )}
            </div>
          </div>
        </div>
      </div>
    </Layout>
  );
}