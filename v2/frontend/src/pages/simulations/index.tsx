import { useState, useEffect } from "react";
import Link from "next/link";
import Layout from "../../components/Layout";
import {
  simulationsService,
  TeamStats,
} from "../../services/simulationsService";
import { Simulator } from "../../utils/simulator";
import {
  TeamScoringData,
  TeamAverage,
  Schedule,
  Matchup,
} from "../../types/simulation";
import { scheduleService } from "../../services/scheduleService";

export default function Simulations() {
  const [simulating, setSimulating] = useState(false);
  const [results, setResults] = useState<string | null>(null);
  const [teamStats, setTeamStats] = useState<TeamStats[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [iterations, setIterations] = useState(1000);
  const [startWeek, setStartWeek] = useState("current");
  const [useActualResults, setUseActualResults] = useState(true);

  // Simulator state
  const [simulator, setSimulator] = useState<Simulator | null>(null);
  const [simulationResults, setSimulationResults] = useState<TeamScoringData[]>(
    []
  );

  useEffect(() => {
    const fetchTeamStats = async () => {
      try {
        setLoading(true);
        const response = await simulationsService.getStats();
        setTeamStats(response.teamStats);
        setError(null);
      } catch (err) {
        setError("Failed to load team statistics");
        console.error("Error fetching team stats:", err);
      } finally {
        setLoading(false);
      }
    };

    fetchTeamStats();
  }, []);

  // Add functions to fetch team averages and schedule data
  const fetchTeamAverages = async (): Promise<Record<string, TeamAverage>> => {
    try {
      // Use the simulationsService to get team stats
      const response = await simulationsService.getStats();

      // Convert to the format expected by the simulator
      const teamAvgs: Record<string, TeamAverage> = {};
      response.teamStats.forEach((team, index) => {
        teamAvgs[index.toString()] = {
          id: parseInt(team.teamId),
          owner: team.teamOwner,
          averageScore: team.averagePoints,
          stddevScore: team.stdDevPoints,
        };
      });

      return teamAvgs;
    } catch (error) {
      console.error("Error fetching team averages:", error);
      throw error;
    }
  };

  const fetchScheduleData = async (): Promise<Schedule> => {
    try {
      // Use the v2 schedule service to get matchup data
      const response = await scheduleService.getFullSchedule();

      // Convert v2 API format to simulator format
      const schedule: Schedule = [];
      const weekMap = new Map<number, Matchup[]>();

      response.data.matchups.forEach((matchup) => {
        if (!weekMap.has(matchup.week)) {
          weekMap.set(matchup.week, []);
        }

        weekMap.get(matchup.week)?.push({
          home_team_espn_id: matchup.home_team_espn_id,
          away_team_espn_id: matchup.away_team_espn_id,
          home_team_final_score: matchup.home_score,
          away_team_final_score: matchup.away_score,
          completed: matchup.home_score > 0 || matchup.away_score > 0,
          week: matchup.week,
        });
      });

      // Convert map to ordered array by week
      const sortedWeeks = Array.from(weekMap.keys()).sort((a, b) => a - b);
      sortedWeeks.forEach((week) => {
        const weekGames = weekMap.get(week) || [];
        schedule.push(weekGames);
      });

      return schedule;
    } catch (error) {
      console.error("Error fetching schedule data:", error);
      throw error;
    }
  };

  const handleSimulation = async () => {
    setSimulating(true);
    setResults(null);

    try {
      // Fetch the data needed for simulation
      const [teamAverages, schedule] = await Promise.all([
        fetchTeamAverages(),
        fetchScheduleData(),
      ]);

      // Create and run the simulator
      const sim = new Simulator(teamAverages, schedule);

      // Run the specified number of simulations
      for (let i = 0; i < iterations; i++) {
        sim.step();
      }

      // Update state with results
      setSimulator(sim);
      setSimulationResults(sim.getTeamScoringData());
      setResults(
        `Simulation completed with ${iterations.toLocaleString()} iterations (ε = ${sim.epsilon.toFixed(
          6
        )})`
      );

      console.log("simulator: ", simulator);
      console.log("simulationResults: ", simulationResults);
    } catch (err) {
      setError("Failed to run simulation");
      console.error("Simulation error:", err);
    } finally {
      setSimulating(false);
    }
  };

  return (
    <Layout>
      <div className="space-y-8">
        <section>
          <h1 className="text-3xl md:text-4xl font-bold text-blue-600 mb-6">
            Run Fantasy Football Simulations
          </h1>
          <p className="text-lg text-gray-600 dark:text-gray-300 mb-8 max-w-3xl">
            Simulate the rest of your fantasy season to see projections for
            final standings, playoff odds, and championship probabilities.
          </p>

          {/* Simulation Parameters */}
          <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
            <h2 className="text-xl font-semibold mb-4">
              Simulation Parameters
            </h2>

            <div className="space-y-4 mb-6">
              <div>
                <label
                  htmlFor="iterations"
                  className="block text-sm font-medium mb-1"
                >
                  Number of Iterations
                </label>
                <input
                  type="number"
                  id="iterations"
                  value={iterations}
                  onChange={(e) => setIterations(Number(e.target.value))}
                  className="w-full md:w-64 px-3 py-2 border dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                />
              </div>

              <div>
                <label
                  htmlFor="startWeek"
                  className="block text-sm font-medium mb-1"
                >
                  Start Week
                </label>
                <select
                  id="startWeek"
                  value={startWeek}
                  onChange={(e) => setStartWeek(e.target.value)}
                  className="w-full md:w-64 px-3 py-2 border dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                >
                  <option value="current">Current (Week 10)</option>
                  <option value="11">Week 11</option>
                  <option value="12">Week 12</option>
                  <option value="13">Week 13</option>
                </select>
              </div>

              <div className="flex items-center">
                <input
                  type="checkbox"
                  id="useActualResults"
                  checked={useActualResults}
                  onChange={(e) => setUseActualResults(e.target.checked)}
                  className="h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
                />
                <label
                  htmlFor="useActualResults"
                  className="ml-2 block text-sm"
                >
                  Use actual results for completed weeks
                </label>
              </div>
            </div>

            <button
              onClick={handleSimulation}
              disabled={simulating}
              className={`${
                simulating
                  ? "bg-blue-400 cursor-not-allowed"
                  : "bg-blue-600 hover:bg-blue-700"
              } text-white px-6 py-2 rounded-md font-medium transition-colors flex items-center`}
            >
              {simulating ? (
                <>
                  <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin mr-2"></div>
                  Simulating...
                </>
              ) : (
                "Run Simulation"
              )}
            </button>
          </div>
        </section>
        <section className="mt-8">
          {/* Team Statistics Section */}
          <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md mb-8">
            <h2 className="text-xl font-semibold mb-4">Team Statistics</h2>

            {loading ? (
              <div className="flex justify-center items-center py-8">
                <div className="w-8 h-8 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                <span className="ml-2">Loading team statistics...</span>
              </div>
            ) : error ? (
              <div className="text-red-600 py-4">{error}</div>
            ) : (
              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
                  <thead className="bg-gray-50 dark:bg-gray-800">
                    <tr>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Owner
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Avg Points
                      </th>
                      <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                        Std Dev
                      </th>
                    </tr>
                  </thead>
                  <tbody className="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
                    {teamStats.map((team, index) => (
                      <tr
                        key={team.teamId}
                        className={
                          team.teamId === "league_average"
                            ? "bg-blue-50 dark:bg-blue-900 font-semibold border-t-2 border-blue-200 dark:border-blue-700"
                            : index % 2 === 0
                            ? "bg-white dark:bg-gray-800"
                            : "bg-gray-50 dark:bg-gray-700"
                        }
                      >
                        <td className="py-2 px-4 whitespace-nowrap">
                          {team.teamId === "league_average" ? (
                            team.teamOwner
                          ) : (
                            <Link
                              href={`/teams/${team.teamId}`}
                              className="text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 hover:underline transition-colors"
                            >
                              {team.teamOwner}
                            </Link>
                          )}
                        </td>
                        <td className="py-2 px-4 whitespace-nowrap">
                          {team.averagePoints.toFixed(2)}
                        </td>
                        <td className="py-2 px-4 whitespace-nowrap">
                          {team.stdDevPoints.toFixed(2)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </section>

        {results && simulationResults.length > 0 && (
          <section className="bg-gray-100 dark:bg-gray-700 p-6 rounded-lg">
            <h2 className="text-xl font-semibold mb-4">Simulation Results</h2>

            <div className="space-y-6">
              <p className="text-green-600 font-medium">{results}</p>

              <div>
                <h3 className="text-lg font-medium mb-2">Playoff Odds</h3>
                <div className="bg-white dark:bg-gray-800 p-4 rounded-md shadow-sm">
                  <p className="text-sm text-gray-500 mb-4">
                    Probability of making playoffs
                  </p>

                  <div className="space-y-3">
                    {simulationResults
                      .filter(
                        (team) =>
                          team.teamName !== "League Average" && team.id !== -1
                      )
                      .sort((a, b) => b.playoff_odds - a.playoff_odds)
                      .map((team) => (
                        <div key={team.id} className="flex items-center">
                          <span
                            className="w-32 text-sm truncate"
                            title={team.teamName}
                          >
                            {team.teamName}
                          </span>
                          <div className="flex-1 h-5 bg-gray-200 dark:bg-gray-600 rounded-full overflow-hidden mx-3">
                            <div
                              className="h-full bg-blue-600"
                              style={{
                                width: `${(team.playoff_odds * 100).toFixed(
                                  1
                                )}%`,
                              }}
                            ></div>
                          </div>
                          <span className="w-14 text-right text-sm">
                            {(team.playoff_odds * 100).toFixed(1)}%
                          </span>
                        </div>
                      ))}
                  </div>
                </div>
              </div>

              <div>
                <h3 className="text-lg font-medium mb-2">
                  Projected Final Standings
                </h3>
                <div className="overflow-x-auto">
                  <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
                    <thead className="bg-gray-50 dark:bg-gray-800">
                      <tr>
                        <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          Rank
                        </th>
                        <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          Team
                        </th>
                        <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          Avg Wins
                        </th>
                        <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          Avg Losses
                        </th>
                        <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          Avg Points For
                        </th>
                        <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          Playoff %
                        </th>
                        <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          Last Place %
                        </th>
                      </tr>
                    </thead>
                    <tbody className="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
                      {simulationResults
                        .filter(
                          (team) =>
                            team.teamName !== "League Average" && team.id !== -1
                        )
                        .sort((a, b) => b.wins - a.wins)
                        .map((team, index) => (
                          <tr
                            key={team.id}
                            className={
                              index % 2 === 0
                                ? "bg-white dark:bg-gray-800"
                                : "bg-gray-50 dark:bg-gray-700"
                            }
                          >
                            <td className="py-2 px-4 whitespace-nowrap font-medium">
                              {index + 1}
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap">
                              <Link
                                href={`/teams/${team.id}`}
                                className="text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 hover:underline transition-colors"
                              >
                                {team.teamName}
                              </Link>
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap">
                              {team.wins.toLocaleString(undefined, {
                                minimumFractionDigits: 2,
                                maximumFractionDigits: 2,
                              })}
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap">
                              {team.losses.toLocaleString(undefined, {
                                minimumFractionDigits: 2,
                                maximumFractionDigits: 2,
                              })}
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap">
                              {team.pointsFor.toLocaleString(undefined, {
                                minimumFractionDigits: 2,
                                maximumFractionDigits: 2,
                              })}
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap">
                              <span
                                className={`font-medium ${
                                  team.playoff_odds > 0.5
                                    ? "text-green-600 dark:text-green-400"
                                    : team.playoff_odds > 0.25
                                    ? "text-yellow-600 dark:text-yellow-400"
                                    : "text-red-600 dark:text-red-400"
                                }`}
                              >
                                {(team.playoff_odds * 100).toFixed(1)}%
                              </span>
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap">
                              <span
                                className={`font-medium ${
                                  team.last_place_odds > 0.5
                                    ? "text-red-600 dark:text-red-400"
                                    : team.last_place_odds > 0.25
                                    ? "text-yellow-600 dark:text-yellow-400"
                                    : "text-green-600 dark:text-green-400"
                                }`}
                              >
                                {(team.last_place_odds * 100).toFixed(1)}%
                              </span>
                            </td>
                          </tr>
                        ))}
                    </tbody>
                  </table>
                </div>
              </div>

              {/* Additional detailed results section */}
              <div>
                <h3 className="text-lg font-medium mb-2">
                  Championship Odds (Regular Season Finish)
                </h3>
                <div className="overflow-x-auto">
                  <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
                    <thead className="bg-gray-50 dark:bg-gray-800">
                      <tr>
                        <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          Team
                        </th>
                        <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          1st
                        </th>
                        <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          2nd
                        </th>
                        <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          3rd
                        </th>
                        <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          Last
                        </th>
                      </tr>
                    </thead>
                    <tbody className="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
                      {simulationResults
                        .filter(
                          (team) =>
                            team.teamName !== "League Average" && team.id !== -1
                        )
                        .sort((a, b) => b.playoff_odds - a.playoff_odds)
                        .map((team, index) => (
                          <tr
                            key={team.id}
                            className={
                              index % 2 === 0
                                ? "bg-white dark:bg-gray-800"
                                : "bg-gray-50 dark:bg-gray-700"
                            }
                          >
                            <td className="py-2 px-4 whitespace-nowrap">
                              {team.teamName}
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap text-center">
                              {team.regular_season_result.length > 0
                                ? (team.regular_season_result[0] * 100).toFixed(
                                    1
                                  ) + "%"
                                : "0.0%"}
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap text-center">
                              {team.regular_season_result.length > 1
                                ? (team.regular_season_result[1] * 100).toFixed(
                                    1
                                  ) + "%"
                                : "0.0%"}
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap text-center">
                              {team.regular_season_result.length > 2
                                ? (team.regular_season_result[2] * 100).toFixed(
                                    1
                                  ) + "%"
                                : "0.0%"}
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap text-center">
                              <span className="text-red-600 dark:text-red-400 font-medium">
                                {(team.last_place_odds * 100).toFixed(1)}%
                              </span>
                            </td>
                          </tr>
                        ))}
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          </section>
        )}
      </div>
    </Layout>
  );
}
