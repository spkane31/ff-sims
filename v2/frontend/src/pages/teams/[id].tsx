import { useState, useEffect } from 'react';
import { useRouter } from 'next/router';
import Link from 'next/link';
import Layout from '../../components/Layout';

// Type definitions
interface Player {
  id: number;
  name: string;
  position: string;
  team: string;
  points: number;
  projection: number;
  status: string;
}

interface DraftPick {
  round: number;
  pick: number;
  originalTeam: string;
  description: string;
  playerId: number; // Reference to associated player
}

interface Game {
  week: number;
  opponent: string;
  result: 'W' | 'L' | 'T' | '-';
  score: string;
  isHome: boolean;
}

interface Team {
  id: number;
  name: string;
  owner: string;
  record: {
    wins: number;
    losses: number;
    ties: number;
  };
  points: {
    scored: number;
    against: number;
  };
  rank: number;
  playoffChance: number;
  players: Player[];
  draftPicks: DraftPick[];
  schedule: Game[];
}

export default function TeamDetail() {
  const router = useRouter();
  const { id } = router.query;

  const [isLoading, setIsLoading] = useState(true);
  const [team, setTeam] = useState<Team | null>(null);
  const [activeTab, setActiveTab] = useState('overview');

  // Sample team data
  const sampleTeam: Team = {
    id: 1,
    name: "Team A",
    owner: "John Smith",
    record: { wins: 7, losses: 2, ties: 0 },
    points: { scored: 1050, against: 890 },
    rank: 1,
    playoffChance: 92,
    players: [
      { id: 101, name: "Patrick Mahomes", position: "QB", team: "KC", points: 225.6, projection: 24.5, status: "Active" },
      { id: 102, name: "Christian McCaffrey", position: "RB", team: "SF", points: 210.3, projection: 22.1, status: "Active" },
      { id: 103, name: "Justin Jefferson", position: "WR", team: "MIN", points: 187.4, projection: 18.9, status: "Active" },
      { id: 104, name: "Travis Kelce", position: "TE", team: "KC", points: 158.2, projection: 15.3, status: "Active" },
      { id: 105, name: "Nick Chubb", position: "RB", team: "CLE", points: 135.7, projection: 0, status: "IR" },
      { id: 106, name: "Amon-Ra St. Brown", position: "WR", team: "DET", points: 156.8, projection: 16.7, status: "Active" },
    ],
    draftPicks: [
      { round: 1, pick: 5, originalTeam: "Team A", description: "1st Round" , playerId: 101 },
      { round: 2, pick: 15, originalTeam: "Team C", description: "2nd Round (via Team C)" , playerId: 102 },
      { round: 3, pick: 25, originalTeam: "Team A", description: "3rd Round" , playerId: 103 },
    ],
    schedule: [
      { week: 1, opponent: "Team B", result: 'W', score: "120-105", isHome: true },
      { week: 2, opponent: "Team A", result: 'L', score: "125-130", isHome: false },
      { week: 3, opponent: "Team C", result: 'W', score: "140-90", isHome: true },
      { week: 10, opponent: "Team D", result: '-', score: "0-0", isHome: true },
      { week: 11, opponent: "Team A", result: '-', score: "0-0", isHome: false },
    ]
  };

  useEffect(() => {
    if (!id) return;

    // Simulate API call to fetch team data
    setIsLoading(true);
    setTimeout(() => {
      setTeam(sampleTeam);
      setIsLoading(false);
    }, 800);
  }, [id]);

  if (isLoading) {
    return (
      <Layout>
        <div className="flex items-center justify-center h-64">
          <div className="w-8 h-8 border-4 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
          <span className="ml-3 text-lg">Loading team data...</span>
        </div>
      </Layout>
    );
  }

  if (!team) {
    return (
      <Layout>
        <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded">
          <h2 className="text-lg font-medium mb-2">Team not found</h2>
          <p>We couldn't find a team with the requested ID. Please check the URL and try again.</p>
          <Link href="/teams" className="mt-4 inline-block text-blue-600 hover:text-blue-800 dark:hover:text-blue-400">
            ← Back to Teams
          </Link>
        </div>
      </Layout>
    );
  }

  return (
    <Layout>
      <div className="space-y-8">
        {/* Team Header */}
        <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
          <div className="flex flex-col md:flex-row justify-between md:items-center">
            <div>
              <div className="flex items-center">
                <h1 className="text-3xl md:text-4xl font-bold text-blue-600">{team.name}</h1>
                <span className="ml-3 px-2 py-1 bg-gray-200 dark:bg-gray-600 rounded text-sm">
                  Rank #{team.rank}
                </span>
              </div>
              <p className="text-lg text-gray-500 dark:text-gray-400">
                Managed by {team.owner}
              </p>
            </div>

            <div className="mt-4 md:mt-0">
              <Link href="/teams" className="text-blue-600 hover:text-blue-800 dark:hover:text-blue-400">
                ← Back to Teams
              </Link>
            </div>
          </div>
        </section>

        {/* Navigation Tabs */}
        <div className="border-b border-gray-200 dark:border-gray-700">
          <nav className="flex space-x-8">
            <button
              onClick={() => setActiveTab('overview')}
              className={`py-4 px-1 border-b-2 font-medium text-sm ${
                activeTab === 'overview'
                  ? 'border-blue-600 text-blue-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              Overview
            </button>
            <button
              onClick={() => setActiveTab('players')}
              className={`py-4 px-1 border-b-2 font-medium text-sm ${
                activeTab === 'players'
                  ? 'border-blue-600 text-blue-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              Players
            </button>
            <button
              onClick={() => setActiveTab('schedule')}
              className={`py-4 px-1 border-b-2 font-medium text-sm ${
                activeTab === 'schedule'
                  ? 'border-blue-600 text-blue-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              Schedule
            </button>
            <button
              onClick={() => setActiveTab('draft')}
              className={`py-4 px-1 border-b-2 font-medium text-sm ${
                activeTab === 'draft'
                  ? 'border-blue-600 text-blue-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              Draft Picks
            </button>
          </nav>
        </div>

        {/* Tab Content */}
        <div className="space-y-6">
          {/* Overview Tab */}
          {activeTab === 'overview' && (
            <>
              <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
                <h2 className="text-xl font-semibold mb-4">Team Overview</h2>
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
                  <div className="bg-gray-50 dark:bg-gray-800 p-4 rounded-lg">
                    <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">Record</h3>
                    <div className="text-2xl font-bold">
                      {team.record.wins}-{team.record.losses}{team.record.ties > 0 ? `-${team.record.ties}` : ''}
                    </div>
                  </div>

                  <div className="bg-gray-50 dark:bg-gray-800 p-4 rounded-lg">
                    <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">Points For</h3>
                    <div className="text-2xl font-bold">{team.points.scored}</div>
                  </div>

                  <div className="bg-gray-50 dark:bg-gray-800 p-4 rounded-lg">
                    <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">Points Against</h3>
                    <div className="text-2xl font-bold">{team.points.against}</div>
                  </div>

                  <div className="bg-gray-50 dark:bg-gray-800 p-4 rounded-lg">
                    <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">Avg. Points</h3>
                    <div className="text-2xl font-bold">
                      {(team.points.scored / (team.record.wins + team.record.losses + team.record.ties)).toFixed(1)}
                    </div>
                  </div>
                </div>
              </section>

              <section className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
                  <h2 className="text-lg font-semibold mb-4">Playoff Odds</h2>
                  <div className="space-y-4">
                    <div>
                      <div className="flex justify-between mb-1">
                        <span>Make Playoffs</span>
                        <span className="font-medium">{team.playoffChance}%</span>
                      </div>
                      <div className="w-full bg-gray-200 dark:bg-gray-600 rounded-full h-2.5">
                        <div
                          className="h-2.5 rounded-full bg-blue-600"
                          style={{ width: `${team.playoffChance}%` }}
                        ></div>
                      </div>
                    </div>

                    <div>
                      <div className="flex justify-between mb-1">
                        <span>Win Division</span>
                        <span className="font-medium">65%</span>
                      </div>
                      <div className="w-full bg-gray-200 dark:bg-gray-600 rounded-full h-2.5">
                        <div
                          className="h-2.5 rounded-full bg-green-600"
                          style={{ width: '65%' }}
                        ></div>
                      </div>
                    </div>

                    <div>
                      <div className="flex justify-between mb-1">
                        <span>Win Championship</span>
                        <span className="font-medium">28%</span>
                      </div>
                      <div className="w-full bg-gray-200 dark:bg-gray-600 rounded-full h-2.5">
                        <div
                          className="h-2.5 rounded-full bg-yellow-600"
                          style={{ width: '28%' }}
                        ></div>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
                  <h2 className="text-lg font-semibold mb-4">Recent Performance</h2>
                  <div className="space-y-3">
                    {team.schedule.slice(0, 4).map((game, i) => (
                      <div key={i} className="flex items-center justify-between py-2 border-b dark:border-gray-600 last:border-0">
                        <div>
                          <span className="font-medium">Week {game.week}</span>
                          <span className="mx-2 text-gray-400">vs</span>
                          <span>{game.opponent}</span>
                        </div>
                        <div className="flex items-center">
                          <span className="mr-2">{game.score}</span>
                          {game.result === 'W' && <span className="w-5 h-5 rounded-full bg-green-500 text-white flex items-center justify-center text-xs">W</span>}
                          {game.result === 'L' && <span className="w-5 h-5 rounded-full bg-red-500 text-white flex items-center justify-center text-xs">L</span>}
                          {game.result === 'T' && <span className="w-5 h-5 rounded-full bg-yellow-500 text-white flex items-center justify-center text-xs">T</span>}
                          {game.result === '-' && <span className="text-gray-400">Upcoming</span>}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              </section>
            </>
          )}

          {/* Players Tab */}
          {activeTab === 'players' && (
            <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
              <h2 className="text-xl font-semibold mb-4">Team Roster</h2>
              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-600">
                  <thead className="bg-gray-50 dark:bg-gray-800">
                    <tr>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Position</th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Player</th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Team</th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Points</th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Projection</th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Status</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
                    {team.players.map((player) => (
                      <tr key={player.id}>
                        <td className="py-4 px-4 whitespace-nowrap">{player.position}</td>
                        <td className="py-4 px-4 whitespace-nowrap font-medium">{player.name}</td>
                        <td className="py-4 px-4 whitespace-nowrap">{player.team}</td>
                        <td className="py-4 px-4 whitespace-nowrap">{player.points.toFixed(1)}</td>
                        <td className="py-4 px-4 whitespace-nowrap">{player.projection > 0 ? player.projection.toFixed(1) : "-"}</td>
                        <td className="py-4 px-4 whitespace-nowrap">
                          <span
                            className={`inline-flex px-2 py-1 text-xs rounded-full
                              ${player.status === 'Active' ? 'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-100' :
                                player.status === 'IR' ? 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-100' :
                                'bg-yellow-100 text-yellow-800 dark:bg-yellow-800 dark:text-yellow-100'
                              }
                            `}
                          >
                            {player.status}
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
          )}

          {/* Schedule Tab */}
          {activeTab === 'schedule' && (
            <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
              <h2 className="text-xl font-semibold mb-4">Team Schedule</h2>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {team.schedule.map((game, i) => (
                  <div
                    key={i}
                    className={`p-4 rounded-lg border ${
                      game.result === 'W' ? 'border-green-200 bg-green-50 dark:bg-green-900/20 dark:border-green-800' :
                      game.result === 'L' ? 'border-red-200 bg-red-50 dark:bg-red-900/20 dark:border-red-800' :
                      'border-gray-200 bg-gray-50 dark:bg-gray-800 dark:border-gray-700'
                    }`}
                  >
                    <div className="flex justify-between items-center mb-2">
                      <span className="font-medium">Week {game.week}</span>
                      {game.result !== '-' && (
                        <span
                          className={`w-5 h-5 rounded-full flex items-center justify-center text-xs text-white ${
                            game.result === 'W' ? 'bg-green-500' :
                            game.result === 'L' ? 'bg-red-500' : 'bg-yellow-500'
                          }`}
                        >
                          {game.result}
                        </span>
                      )}
                    </div>
                    <div className="mb-2">
                      <span className="text-gray-500 dark:text-gray-400">{game.isHome ? 'vs' : '@'}</span>
                      <span className="ml-2 font-medium">{game.opponent}</span>
                    </div>
                    <div>
                      {game.result !== '-' ? (
                        <span>{game.score}</span>
                      ) : (
                        <span className="text-gray-500 dark:text-gray-400">Upcoming</span>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </section>
          )}

          {/* Draft Picks Tab */}
          {activeTab === 'draft' && (
            <section className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
              <h2 className="text-xl font-semibold mb-4">Draft Capital</h2>
              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-600">
                  <thead className="bg-gray-50 dark:bg-gray-800">
                    <tr>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Round</th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Pick</th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Original Team</th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Description</th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Player</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
                    {team.draftPicks.map((pick, i) => (
                      <tr key={i} className={i % 2 === 0 ? "" : "bg-gray-50 dark:bg-gray-700"}>
                        <td className="py-4 px-4 whitespace-nowrap">{pick.round}</td>
                        <td className="py-4 px-4 whitespace-nowrap">{pick.pick}</td>
                        <td className="py-4 px-4 whitespace-nowrap">{pick.originalTeam}</td>
                        <td className="py-4 px-4">{pick.description}</td>
                        <td className="py-4 px-4">
                          {/* Find the player associated with this draft pick */}
                          {team.players.find(player => player.id === pick.playerId)?.name}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
          )}
        </div>
      </div>
    </Layout>
  );
}