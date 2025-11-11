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
  const [startWeek, setStartWeek] = useState("");

  // New state for dynamic week options
  const [availableWeeks, setAvailableWeeks] = useState<number[]>([]);
  const [currentWeek, setCurrentWeek] = useState<number>(1);
  const [scheduleLoaded, setScheduleLoaded] = useState(false);

  // Year filter state
  const [availableYears, setAvailableYears] = useState<number[]>([]);
  const [selectedYear, setSelectedYear] = useState<number | null>(null);

  // Simulator state
  const [simulationResults, setSimulationResults] = useState<TeamScoringData[]>(
    []
  );
  const [simulator, setSimulator] = useState<Simulator | null>(null);

  // Interactive "Choose Your Own Results" state
  const [remainingMatchups, setRemainingMatchups] = useState<
    Map<number, Matchup[]>
  >(new Map());
  const [selectedResults, setSelectedResults] = useState<Map<string, number>>(
    new Map()
  ); // key: "week-homeTeamId-awayTeamId", value: winning team ID
  const [filteredResults, setFilteredResults] = useState<TeamScoringData[]>([]);
  const [matchingSimCount, setMatchingSimCount] = useState<number>(iterations);

  // New useEffect to load schedule and determine available weeks/years
  useEffect(() => {
    const loadScheduleInfo = async () => {
      try {
        // Get all schedule data to determine available years
        const response = await scheduleService.getFullSchedule();

        // Extract unique years and set available years
        const uniqueYears = Array.from(
          new Set(response.data.matchups.map((matchup) => matchup.year))
        ).sort((a, b) => b - a); // Sort in descending order (newest first)
        setAvailableYears(uniqueYears);

        // Set default selected year to the most recent year
        const defaultYear = uniqueYears[0];
        setSelectedYear(defaultYear);

        // Convert to the format expected by the simulator for the default year
        const schedule = await fetchScheduleDataForYear(defaultYear);

        // Get all available weeks from the full schedule
        // The simulator will handle filtering out playoff games at the matchup level
        const allWeeks = schedule.map((_, index) => index + 1);
        setAvailableWeeks(allWeeks);

        // Find current week (first week with incomplete games) in the full schedule
        const currentWeekIndex = schedule.findIndex((week) =>
          week.some((matchup) => !matchup.completed)
        );
        const detectedCurrentWeek =
          currentWeekIndex === -1 ? allWeeks.length : currentWeekIndex + 1;
        setCurrentWeek(detectedCurrentWeek);

        // Set the default startWeek to the current week
        setStartWeek(detectedCurrentWeek.toString());

        setScheduleLoaded(true);
      } catch (err) {
        console.error("Failed to load schedule info:", err);
        setError("Failed to load schedule information");
      }
    };

    loadScheduleInfo();
  }, []);

  // Separate useEffect to handle year changes
  useEffect(() => {
    if (selectedYear === null) return;

    const loadScheduleForYear = async () => {
      try {
        setScheduleLoaded(false);
        const schedule = await fetchScheduleDataForYear(selectedYear);

        // Get all available weeks from the full schedule
        // The simulator will handle filtering out playoff games at the matchup level
        const allWeeks = schedule.map(
          (_: Matchup[], index: number) => index + 1
        );
        setAvailableWeeks(allWeeks);

        // Find current week (first week with incomplete games) in the full schedule
        const currentWeekIndex = schedule.findIndex((week: Matchup[]) =>
          week.some((matchup: Matchup) => !matchup.completed)
        );
        const detectedCurrentWeek =
          currentWeekIndex === -1 ? allWeeks.length : currentWeekIndex + 1;
        setCurrentWeek(detectedCurrentWeek);

        // Set the default startWeek to the current week
        setStartWeek(detectedCurrentWeek.toString());

        setScheduleLoaded(true);
      } catch (err) {
        console.error("Failed to load schedule for year:", err);
        setError("Failed to load schedule information for selected year");
      }
    };

    loadScheduleForYear();
  }, [selectedYear]);

  const fetchScheduleDataForYear = async (year: number): Promise<Schedule> => {
    try {
      // Use the v2 schedule service to get all matchup data, then filter by year
      const response = await scheduleService.getFullSchedule();

      // Filter matchups by the selected year
      const yearMatchups = response.data.matchups.filter(
        (matchup) => matchup.year === year
      );

      // Convert v2 API format to simulator format
      const schedule: Schedule = [];
      const weekMap = new Map<number, Matchup[]>();

      yearMatchups.forEach((matchup) => {
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

      // Ensure we have a full regular season (weeks 1-14) for simulation
      // If we're missing future weeks, create placeholder incomplete matchups
      const completedWeeks = Array.from(weekMap.keys()).sort((a, b) => a - b);
      const lastCompletedWeek = completedWeeks[completedWeeks.length - 1] || 0;

      // Get all unique teams from completed matchups to generate future matchups
      const teams = new Set<number>();
      const teamNames = new Map<number, string>();

      yearMatchups.forEach((matchup) => {
        teams.add(matchup.homeTeamESPNID);
        teams.add(matchup.awayTeamESPNID);
        teamNames.set(matchup.homeTeamESPNID, matchup.homeTeamName);
        teamNames.set(matchup.awayTeamESPNID, matchup.awayTeamName);
      });

      const teamList = Array.from(teams);

      // Generate incomplete matchups for remaining regular season weeks (up to week 14)
      for (let week = lastCompletedWeek + 1; week <= 14; week++) {
        if (!weekMap.has(week)) {
          weekMap.set(week, []);

          // Create placeholder matchups for this week
          // This is a simple pairing - in reality, you'd want the actual schedule pattern
          // But for simulation purposes, we just need to ensure all teams play
          for (let i = 0; i < teamList.length; i += 2) {
            if (i + 1 < teamList.length) {
              const homeTeamId = teamList[i];
              const awayTeamId = teamList[i + 1];

              weekMap.get(week)?.push({
                homeTeamName: teamNames.get(homeTeamId) || `Team ${homeTeamId}`,
                awayTeamName: teamNames.get(awayTeamId) || `Team ${awayTeamId}`,
                homeTeamESPNID: homeTeamId,
                awayTeamESPNID: awayTeamId,
                homeTeamFinalScore: 0,
                awayTeamFinalScore: 0,
                completed: false,
                week: week,
                gameType: "NONE",
              });
            }
          }
        }
      }

      // Convert map to ordered array by week
      const sortedWeeks = Array.from(weekMap.keys()).sort((a, b) => a - b);
      sortedWeeks.forEach((week) => {
        const weekGames = weekMap.get(week) || [];
        schedule.push(weekGames);
      });

      return schedule;
    } catch (error) {
      console.error("Error fetching schedule data for year:", error);
      throw error;
    }
  };

  const handleMatchupClick = (
    matchup: Matchup,
    teamId: number,
    opponentId: number
  ) => {
    const matchupKey = `${matchup.week}-${matchup.homeTeamESPNID}-${matchup.awayTeamESPNID}`;

    setSelectedResults((prev) => {
      const newResults = new Map(prev);
      const currentWinner = newResults.get(matchupKey);

      // Cycle through states: no-action -> win (teamId) -> loss (opponentId) -> no-action
      if (currentWinner === undefined) {
        // No action -> Win (set current team as winner)
        newResults.set(matchupKey, teamId);
      } else if (currentWinner === teamId) {
        // Win -> Loss (set opponent as winner)
        newResults.set(matchupKey, opponentId);
      } else {
        // Loss -> No action (remove entry)
        newResults.delete(matchupKey);
      }

      // Update filtered results based on new selections
      if (simulator) {
        const filtered = simulator.getFilteredTeamScoringData(newResults);
        setFilteredResults(filtered.data);
        setMatchingSimCount(filtered.matchingCount);
      }

      return newResults;
    });
  };

  const getMatchupState = (
    matchup: Matchup,
    teamId: number
  ): "win" | "loss" | "none" => {
    const matchupKey = `${matchup.week}-${matchup.homeTeamESPNID}-${matchup.awayTeamESPNID}`;
    const winner = selectedResults.get(matchupKey);

    if (winner === undefined) return "none";
    if (winner === teamId) return "win";
    return "loss";
  };

  const calculateMatchingSimulations = (): {
    matching: number;
    total: number;
    percentage: number;
  } => {
    const percentage = (matchingSimCount / iterations) * 100;
    return {
      matching: matchingSimCount,
      total: iterations,
      percentage: percentage,
    };
  };

  const handleResetSelections = () => {
    setSelectedResults(new Map());
    setFilteredResults(simulationResults);
    setMatchingSimCount(iterations);
  };

  const extractRemainingMatchups = (
    schedule: Schedule,
    startWeek: number
  ): Map<number, Matchup[]> => {
    const teamMatchups = new Map<number, Matchup[]>();
    const maxWeeksToShow = 4; // Limit to 4 weeks for UI readability
    const endWeek = startWeek + maxWeeksToShow - 1;

    schedule.forEach((week, weekIndex) => {
      const currentWeek = weekIndex + 1;

      // Only process weeks from startWeek up to 4 weeks ahead, skip playoff games
      if (currentWeek >= startWeek && currentWeek <= endWeek) {
        week.forEach((matchup) => {
          // Skip playoff games
          if (matchup.gameType !== "NONE") {
            return;
          }

          const homeTeamId = matchup.homeTeamESPNID;
          const awayTeamId = matchup.awayTeamESPNID;

          // Add matchup to home team's list
          if (!teamMatchups.has(homeTeamId)) {
            teamMatchups.set(homeTeamId, []);
          }
          teamMatchups.get(homeTeamId)!.push(matchup);

          // Add matchup to away team's list
          if (!teamMatchups.has(awayTeamId)) {
            teamMatchups.set(awayTeamId, []);
          }
          teamMatchups.get(awayTeamId)!.push(matchup);
        });
      }
    });

    return teamMatchups;
  };

  const handleSimulation = async () => {
    if (selectedYear === null) {
      setError("Please select a year first");
      return;
    }

    setSimulating(true);
    setResults(null);

    try {
      // Fetch the schedule data for simulation for the selected year
      const schedule = await fetchScheduleDataForYear(selectedYear);

      // Parse the start week value
      const startWeekNum = parseInt(startWeek);

      // Create and run the simulator with the new constructor
      const sim = new Simulator(schedule, startWeekNum);

      // Run the specified number of simulations
      for (let i = 0; i < iterations; i++) {
        sim.step();
      }

      // Update state with results
      setSimulationResults(sim.getTeamScoringData());
      setSimulator(sim); // Store the simulator instance
      setFilteredResults(sim.getTeamScoringData()); // Initialize with all results
      setMatchingSimCount(iterations); // Initialize with total iterations
      setResults(
        `Simulation completed for ${selectedYear} season with ${iterations.toLocaleString()} iterations starting from week ${startWeekNum} (ε = ${sim.epsilon.toFixed(
          6
        )})`
      );

      // Extract remaining matchups for interactive visualization
      const matchupsByTeam = extractRemainingMatchups(schedule, startWeekNum);
      setRemainingMatchups(matchupsByTeam);
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
                  htmlFor="year"
                  className="block text-sm font-medium mb-1"
                >
                  Season Year
                </label>
                <select
                  id="year"
                  value={selectedYear || ""}
                  onChange={(e) => setSelectedYear(Number(e.target.value))}
                  disabled={availableYears.length === 0}
                  className="w-full md:w-64 px-3 py-2 border dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {availableYears.map((year) => (
                    <option key={year} value={year}>
                      {year}
                    </option>
                  ))}
                </select>
                {availableYears.length === 0 && (
                  <p className="text-xs text-gray-500 mt-1">
                    Loading available years...
                  </p>
                )}
              </div>

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

              {/* Interactive "Choose Your Own Results" Section */}
              {remainingMatchups.size > 0 &&
                (() => {
                  // Extract all unique weeks from remaining matchups and sort them
                  const allWeeksSet = new Set<number>();
                  remainingMatchups.forEach((matchups) => {
                    matchups.forEach((matchup) => {
                      allWeeksSet.add(matchup.week);
                    });
                  });
                  const sortedWeeks = Array.from(allWeeksSet).sort(
                    (a, b) => a - b
                  );

                  // Calculate matching simulations
                  const matchStats = calculateMatchingSimulations();

                  return (
                    <div>
                      <div className="flex items-center justify-between mb-4">
                        <h3 className="text-lg font-medium">
                          Choose Your Own Results
                        </h3>
                        {selectedResults.size > 0 && (
                          <button
                            onClick={handleResetSelections}
                            className="px-4 py-2 text-sm font-medium text-red-700 dark:text-red-300 bg-red-100 dark:bg-red-900/30 border border-red-300 dark:border-red-700 rounded-md hover:bg-red-200 dark:hover:bg-red-900/50 transition-colors"
                          >
                            Reset All
                          </button>
                        )}
                      </div>
                      <p className="text-sm text-gray-600 dark:text-gray-400 mb-2">
                        Click on matchups to select winners and see how results
                        affect playoff odds
                      </p>
                      <div className="mb-4 p-3 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-md">
                        <p className="text-sm font-medium text-blue-900 dark:text-blue-100">
                          Matching Simulations:{" "}
                          {matchStats.matching.toLocaleString()}/
                          {matchStats.total.toLocaleString()} (
                          {matchStats.percentage.toFixed(1)}%)
                        </p>
                        {selectedResults.size > 0 && (
                          <p className="text-xs text-blue-700 dark:text-blue-300 mt-1">
                            {selectedResults.size} game
                            {selectedResults.size !== 1 ? "s" : ""} selected
                          </p>
                        )}
                      </div>

                      {matchStats.percentage < 0.5 &&
                        selectedResults.size > 0 && (
                          <div className="mb-4 p-4 bg-gradient-to-r from-orange-50 to-red-50 dark:from-orange-900/20 dark:to-red-900/20 border-2 border-orange-400 dark:border-orange-600 rounded-lg shadow-md">
                            <div className="flex items-center">
                              <span className="text-2xl mr-3">🎲</span>
                              <div>
                                <p className="text-sm font-bold text-orange-900 dark:text-orange-200">
                                  This is a very unlikely scenario, keep
                                  dreaming partner!
                                </p>
                                <p className="text-xs text-orange-700 dark:text-orange-300 mt-1">
                                  Less than 1 in 200 simulations match your
                                  picks. Might want to reconsider your
                                  strategy...
                                </p>
                              </div>
                            </div>
                          </div>
                        )}

                      <div className="overflow-x-auto">
                        <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
                          <thead className="bg-gray-50 dark:bg-gray-800">
                            <tr>
                              <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                                Team
                              </th>
                              {sortedWeeks.map((week) => (
                                <th
                                  key={week}
                                  className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider w-32"
                                >
                                  Week {week}
                                </th>
                              ))}
                              <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                                Playoff Odds
                                <br />
                                <span className="text-[10px] font-normal">
                                  (Default)
                                </span>
                              </th>
                              <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                                Playoff Odds
                                <br />
                                <span className="text-[10px] font-normal">
                                  (New)
                                </span>
                              </th>
                              <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                                Last Place Odds
                                <br />
                                <span className="text-[10px] font-normal">
                                  (Default)
                                </span>
                              </th>
                              <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                                Last Place Odds
                                <br />
                                <span className="text-[10px] font-normal">
                                  (New)
                                </span>
                              </th>
                            </tr>
                          </thead>
                          <tbody className="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
                            {simulationResults
                              .filter(
                                (team) =>
                                  team.teamName !== "League Average" &&
                                  team.id !== -1
                              )
                              .sort((a, b) => b.playoffOdds - a.playoffOdds)
                              .map((team, index) => {
                                const teamMatchups =
                                  remainingMatchups.get(team.id) || [];

                                // Find the filtered result for this team
                                const filteredTeam = filteredResults.find(
                                  (t) => t.id === team.id
                                );

                                return (
                                  <tr
                                    key={team.id}
                                    className={
                                      index % 2 === 0
                                        ? "bg-white dark:bg-gray-800"
                                        : "bg-gray-50 dark:bg-gray-700"
                                    }
                                  >
                                    {/* Team Name */}
                                    <td className="py-3 px-4 whitespace-nowrap font-medium">
                                      {team.teamName}
                                    </td>

                                    {/* Remaining Matchups - one cell per week */}
                                    {sortedWeeks.map((week) => {
                                      // Find the matchup for this team in this week
                                      const matchup = teamMatchups.find(
                                        (m) => m.week === week
                                      );

                                      if (!matchup) {
                                        return (
                                          <td
                                            key={week}
                                            className="py-3 px-4 text-center w-32"
                                          >
                                            <div className="text-gray-400 text-xs">
                                              -
                                            </div>
                                          </td>
                                        );
                                      }

                                      const isHomeTeam =
                                        matchup.homeTeamESPNID === team.id;
                                      const opponentId = isHomeTeam
                                        ? matchup.awayTeamESPNID
                                        : matchup.homeTeamESPNID;
                                      const opponent = isHomeTeam
                                        ? matchup.awayTeamName
                                        : matchup.homeTeamName;

                                      const state = getMatchupState(
                                        matchup,
                                        team.id
                                      );

                                      // Determine button styling based on state
                                      let buttonClasses =
                                        "w-full px-2 py-2 rounded-md transition-colors cursor-pointer border ";
                                      let textClasses = "text-xs";

                                      if (state === "win") {
                                        buttonClasses +=
                                          "bg-green-100 dark:bg-green-900/30 border-green-300 dark:border-green-700 hover:bg-green-200 dark:hover:bg-green-900/50";
                                        textClasses +=
                                          " text-green-800 dark:text-green-200";
                                      } else if (state === "loss") {
                                        buttonClasses +=
                                          "bg-red-100 dark:bg-red-900/30 border-red-300 dark:border-red-700 hover:bg-red-200 dark:hover:bg-red-900/50";
                                        textClasses +=
                                          " text-red-800 dark:text-red-200";
                                      } else {
                                        buttonClasses +=
                                          "bg-blue-100 dark:bg-blue-900/30 border-blue-300 dark:border-blue-700 hover:bg-blue-200 dark:hover:bg-blue-900/50";
                                        textClasses +=
                                          " text-gray-700 dark:text-gray-300";
                                      }

                                      return (
                                        <td
                                          key={week}
                                          className="py-3 px-4 text-center w-32"
                                        >
                                          <button
                                            onClick={() =>
                                              handleMatchupClick(
                                                matchup,
                                                team.id,
                                                opponentId
                                              )
                                            }
                                            className={buttonClasses}
                                          >
                                            <div className={textClasses}>
                                              <div className="font-semibold">
                                                Week {matchup.week}
                                              </div>
                                              <div className="truncate">
                                                {isHomeTeam ? "vs" : "@"}{" "}
                                                {opponent}
                                              </div>
                                            </div>
                                          </button>
                                        </td>
                                      );
                                    })}

                                    {/* Default Playoff Odds */}
                                    <td className="py-3 px-4 text-center">
                                      <span className="font-medium">
                                        {(team.playoffOdds * 100).toFixed(1)}%
                                      </span>
                                    </td>

                                    {/* New Playoff Odds */}
                                    <td className="py-3 px-4 text-center">
                                      <span className="font-medium">
                                        {filteredTeam
                                          ? (
                                              filteredTeam.playoffOdds * 100
                                            ).toFixed(1)
                                          : "0.0"}
                                        %
                                      </span>
                                    </td>

                                    {/* Default Last Place Odds */}
                                    <td className="py-3 px-4 text-center">
                                      <span className="font-medium">
                                        {(team.lastPlaceOdds * 100).toFixed(1)}%
                                      </span>
                                    </td>

                                    {/* New Last Place Odds */}
                                    <td className="py-3 px-4 text-center">
                                      <span className="font-medium">
                                        {filteredTeam
                                          ? (
                                              filteredTeam.lastPlaceOdds * 100
                                            ).toFixed(1)
                                          : "0.0"}
                                        %
                                      </span>
                                    </td>
                                  </tr>
                                );
                              })}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  );
                })()}

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
                          Champion %
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
                                  team.playoffResult.length > 0 &&
                                  team.playoffResult[0] > 0.15
                                    ? "text-green-600 dark:text-green-400"
                                    : team.playoffResult.length > 0 &&
                                      team.playoffResult[0] > 0.05
                                    ? "text-yellow-600 dark:text-yellow-400"
                                    : "text-gray-600 dark:text-gray-400"
                                }`}
                              >
                                {team.playoffResult.length > 0
                                  ? (team.playoffResult[0] * 100).toFixed(1) +
                                    "%"
                                  : "0.0%"}
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

              {/* Championship and Playoff Results */}
              <div>
                <h3 className="text-lg font-medium mb-2">Championship Odds</h3>
                <div className="overflow-x-auto">
                  <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
                    <thead className="bg-gray-50 dark:bg-gray-800">
                      <tr>
                        <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          Team
                        </th>
                        <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          Champion
                        </th>
                        <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          Runner-up
                        </th>
                        <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          3rd Place
                        </th>
                        <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          4th Place
                        </th>
                      </tr>
                    </thead>
                    <tbody className="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
                      {simulationResults
                        .filter(
                          (team) =>
                            team.teamName !== "League Average" && team.id !== -1
                        )
                        .sort((a, b) => {
                          // Sort by championship odds (1st place in playoffs)
                          const aChampOdds =
                            a.playoffResult.length > 0 ? a.playoffResult[0] : 0;
                          const bChampOdds =
                            b.playoffResult.length > 0 ? b.playoffResult[0] : 0;
                          return bChampOdds - aChampOdds;
                        })
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
                              <span
                                className={`font-medium ${
                                  team.playoffResult.length > 0 &&
                                  team.playoffResult[0] > 0.2
                                    ? "text-green-600 dark:text-green-400"
                                    : team.playoffResult.length > 0 &&
                                      team.playoffResult[0] > 0.1
                                    ? "text-yellow-600 dark:text-yellow-400"
                                    : "text-gray-600 dark:text-gray-400"
                                }`}
                              >
                                {team.playoffResult.length > 0
                                  ? (team.playoffResult[0] * 100).toFixed(1) +
                                    "%"
                                  : "0.0%"}
                              </span>
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap text-center">
                              {team.playoffResult.length > 1
                                ? (team.playoffResult[1] * 100).toFixed(1) + "%"
                                : "0.0%"}
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap text-center">
                              {team.playoffResult.length > 2
                                ? (team.playoffResult[2] * 100).toFixed(1) + "%"
                                : "0.0%"}
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap text-center">
                              {team.playoffResult.length > 3
                                ? (team.playoffResult[3] * 100).toFixed(1) + "%"
                                : "0.0%"}
                            </td>
                          </tr>
                        ))}
                    </tbody>
                  </table>
                </div>
              </div>

              {/* Regular Season Finish */}
              <div>
                <h3 className="text-lg font-medium mb-2">
                  Regular Season Finish
                </h3>
                <div className="overflow-x-auto">
                  <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
                    <thead className="bg-gray-50 dark:bg-gray-800">
                      <tr>
                        <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          Team
                        </th>
                        <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          1st Seed
                        </th>
                        <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          2nd Seed
                        </th>
                        <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          3rd Seed
                        </th>
                        <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          4th Seed
                        </th>
                        <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          5th Seed
                        </th>
                        <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          6th Seed
                        </th>
                        <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                          Last Place
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
                              <span
                                className={`font-medium ${
                                  team.regularSeasonResult.length > 0 &&
                                  team.regularSeasonResult[0] > 0.2
                                    ? "text-green-600 dark:text-green-400"
                                    : team.regularSeasonResult.length > 0 &&
                                      team.regularSeasonResult[0] > 0.1
                                    ? "text-yellow-600 dark:text-yellow-400"
                                    : "text-gray-600 dark:text-gray-400"
                                }`}
                              >
                                {team.regularSeasonResult.length > 0
                                  ? (team.regularSeasonResult[0] * 100).toFixed(
                                      1
                                    ) + "%"
                                  : "0.0%"}
                              </span>
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap text-center">
                              <span
                                className={`font-medium ${
                                  team.regularSeasonResult.length > 1 &&
                                  team.regularSeasonResult[1] > 0.15
                                    ? "text-green-600 dark:text-green-400"
                                    : team.regularSeasonResult.length > 1 &&
                                      team.regularSeasonResult[1] > 0.08
                                    ? "text-yellow-600 dark:text-yellow-400"
                                    : "text-gray-600 dark:text-gray-400"
                                }`}
                              >
                                {team.regularSeasonResult.length > 1
                                  ? (team.regularSeasonResult[1] * 100).toFixed(
                                      1
                                    ) + "%"
                                  : "0.0%"}
                              </span>
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap text-center">
                              {team.regularSeasonResult.length > 2
                                ? (team.regularSeasonResult[2] * 100).toFixed(
                                    1
                                  ) + "%"
                                : "0.0%"}
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap text-center">
                              {team.regularSeasonResult.length > 3
                                ? (team.regularSeasonResult[3] * 100).toFixed(
                                    1
                                  ) + "%"
                                : "0.0%"}
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap text-center">
                              {team.regularSeasonResult.length > 4
                                ? (team.regularSeasonResult[4] * 100).toFixed(
                                    1
                                  ) + "%"
                                : "0.0%"}
                            </td>
                            <td className="py-2 px-4 whitespace-nowrap text-center">
                              {team.regularSeasonResult.length > 5
                                ? (team.regularSeasonResult[5] * 100).toFixed(
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
