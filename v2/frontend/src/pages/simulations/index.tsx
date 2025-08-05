import { useState, useEffect } from "react";
import Link from "next/link";
import Layout from "../../components/Layout";
import { Simulator } from "../../utils/simulator";
import { TeamScoringData, Schedule, Matchup } from "../../types/simulation";
import { scheduleService } from "../../services/scheduleService";

export default function Simulations() {
  const [simulating, setSimulating] = useState(false);
  const [results, setResults] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [iterations, setIterations] = useState(5000);
  const [startWeek, setStartWeek] = useState("current");

  // New state for dynamic week options
  const [availableWeeks, setAvailableWeeks] = useState<number[]>([]);
  const [currentWeek, setCurrentWeek] = useState<number>(1);
  const [scheduleLoaded, setScheduleLoaded] = useState(false);

  // Simulator state
  const [simulationResults, setSimulationResults] = useState<TeamScoringData[]>(
    []
  );

  // New useEffect to load schedule and determine available weeks
  useEffect(() => {
    const loadScheduleInfo = async () => {
      try {
        const schedule = await fetchScheduleData();

        // Filter schedule to only include regular season weeks (gameType === "NONE")
        // We don't simulate playoff games
        const regularSeasonSchedule = schedule.filter((week) =>
          week.every((matchup) => matchup.gameType === "NONE")
        );

        // Get all available weeks from regular season schedule
        const regularSeasonWeeks = regularSeasonSchedule.map(
          (_, index) => index + 1
        );
        setAvailableWeeks(regularSeasonWeeks);

        // Find current week (first week with incomplete games) within regular season
        const currentWeekIndex = regularSeasonSchedule.findIndex((week) =>
          week.some((matchup) => !matchup.completed)
        );
        const detectedCurrentWeek =
          currentWeekIndex === -1
            ? regularSeasonWeeks.length
            : currentWeekIndex + 1;
        setCurrentWeek(detectedCurrentWeek);

        setScheduleLoaded(true);
      } catch (err) {
        console.error("Failed to load schedule info:", err);
        setError("Failed to load schedule information");
      }
    };

    loadScheduleInfo();
  }, []);

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
          homeTeamName: matchup.homeTeamName,
          awayTeamName: matchup.awayTeamName,
          homeTeamESPNID: matchup.homeTeamESPNID,
          awayTeamESPNID: matchup.awayTeamESPNID,
          homeTeamFinalScore: matchup.homeScore,
          awayTeamFinalScore: matchup.awayScore,
          completed: matchup.homeScore > 0 || matchup.awayScore > 0,
          week: matchup.week,
          gameType: matchup.gameType,
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
      // Fetch the schedule data for simulation
      const schedule = await fetchScheduleData();

      // Parse the start week value
      let startWeekNum = 1;
      if (startWeek === "current") {
        // Find the current week based on completed games
        startWeekNum = schedule.findIndex((week) =>
          week.some((matchup) => !matchup.completed)
        );
        if (startWeekNum === 0) startWeekNum = schedule.length; // All games completed
      } else {
        startWeekNum = parseInt(startWeek);
      }

      // Create and run the simulator with the new constructor
      const sim = new Simulator(schedule, startWeekNum);

      // Run the specified number of simulations
      for (let i = 0; i < iterations; i++) {
        sim.step();
      }

      // Update state with results
      setSimulationResults(sim.getTeamScoringData());
      setResults(
        `Simulation completed with ${iterations.toLocaleString()} iterations starting from week ${startWeekNum} (ε = ${sim.epsilon.toFixed(
          6
        )})`
      );
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
                  {scheduleLoaded && (
                    <span className="text-xs text-gray-500 ml-2">
                      (Current: Week {currentWeek})
                    </span>
                  )}
                </label>
                <select
                  id="startWeek"
                  value={startWeek}
                  onChange={(e) => setStartWeek(e.target.value)}
                  disabled={!scheduleLoaded}
                  className="w-full md:w-64 px-3 py-2 border dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <option value="current">
                    Current Week ({scheduleLoaded ? currentWeek : "..."})
                  </option>
                  {availableWeeks.map((week) => (
                    <option key={week} value={week.toString()}>
                      Week {week}
                      {week === currentWeek ? " (Current)" : ""}
                      {week < currentWeek ? " (Past)" : ""}
                      {week > currentWeek ? " (Future)" : ""}
                    </option>
                  ))}
                </select>
                {!scheduleLoaded && (
                  <p className="text-xs text-gray-500 mt-1">
                    Loading schedule data...
                  </p>
                )}
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

          {/* Error Display */}
          {error && (
            <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 p-4 rounded-lg">
              <div className="flex items-center">
                <div className="flex-shrink-0">
                  <svg
                    className="h-5 w-5 text-red-400"
                    viewBox="0 0 20 20"
                    fill="currentColor"
                  >
                    <path
                      fillRule="evenodd"
                      d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
                      clipRule="evenodd"
                    />
                  </svg>
                </div>
                <div className="ml-3">
                  <h3 className="text-sm font-medium text-red-800 dark:text-red-200">
                    Error
                  </h3>
                  <div className="mt-2 text-sm text-red-700 dark:text-red-300">
                    {error}
                  </div>
                  <div className="mt-3">
                    <button
                      onClick={() => setError(null)}
                      className="text-sm bg-red-100 dark:bg-red-800/30 text-red-800 dark:text-red-200 px-3 py-1 rounded-md hover:bg-red-200 dark:hover:bg-red-800/50 transition-colors"
                    >
                      Dismiss
                    </button>
                  </div>
                </div>
              </div>
            </div>
          )}
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
                      .sort((a, b) => b.playoffOdds - a.playoffOdds)
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
                              className={`h-full ${
                                team.playoffOdds > 0.67
                                  ? "bg-green-600"
                                  : team.playoffOdds > 0.33
                                  ? "bg-yellow-600"
                                  : "bg-red-600"
                              }`}
                              style={{
                                width: `${(team.playoffOdds * 100).toFixed(
                                  1
                                )}%`,
                              }}
                            ></div>
                          </div>
                          <span className="w-14 text-right text-sm">
                            {(team.playoffOdds * 100).toFixed(1)}%
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
                                  team.playoffOdds > 0.5
                                    ? "text-green-600 dark:text-green-400"
                                    : team.playoffOdds > 0.25
                                    ? "text-yellow-600 dark:text-yellow-400"
                                    : "text-red-600 dark:text-red-400"
                                }`}
                              >
                                {(team.playoffOdds * 100).toFixed(1)}%
                              </span>
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap">
                              <span
                                className={`font-medium ${
                                  team.lastPlaceOdds > 0.5
                                    ? "text-red-600 dark:text-red-400"
                                    : team.lastPlaceOdds > 0.25
                                    ? "text-yellow-600 dark:text-yellow-400"
                                    : "text-green-600 dark:text-green-400"
                                }`}
                              >
                                {(team.lastPlaceOdds * 100).toFixed(1)}%
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
                        .sort((a, b) => b.playoffOdds - a.playoffOdds)
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
                              {team.regularSeasonResult.length > 0
                                ? (team.regularSeasonResult[0] * 100).toFixed(
                                    1
                                  ) + "%"
                                : "0.0%"}
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap text-center">
                              {team.regularSeasonResult.length > 1
                                ? (team.regularSeasonResult[1] * 100).toFixed(
                                    1
                                  ) + "%"
                                : "0.0%"}
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap text-center">
                              {team.regularSeasonResult.length > 2
                                ? (team.regularSeasonResult[2] * 100).toFixed(
                                    1
                                  ) + "%"
                                : "0.0%"}
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap text-center">
                              <span
                                className={`font-medium ${
                                  team.lastPlaceOdds < 0.1
                                    ? "text-green-600 dark:text-green-400"
                                    : team.lastPlaceOdds <= 0.3
                                    ? "text-yellow-600 dark:text-yellow-400"
                                    : "text-red-600 dark:text-red-400"
                                }`}
                              >
                                {(team.lastPlaceOdds * 100).toFixed(1)}%
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
