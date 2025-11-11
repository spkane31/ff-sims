import { useState, useEffect } from "react";
import { Simulator } from "../utils/simulator";
import { Schedule, Matchup, TeamScoringData } from "../types/simulation";

interface PivotalGame {
  week: number;
  homeTeamId: number;
  awayTeamId: number;
  homeTeamName: string;
  awayTeamName: string;
  totalSwing: number;
  homeTeamWinScenario: {
    homePlayoffOdds: number;
    awayPlayoffOdds: number;
    homeLastPlaceOdds: number;
    awayLastPlaceOdds: number;
  };
  awayTeamWinScenario: {
    homePlayoffOdds: number;
    awayPlayoffOdds: number;
    homeLastPlaceOdds: number;
    awayLastPlaceOdds: number;
  };
  defaultOdds: {
    homePlayoffOdds: number;
    awayPlayoffOdds: number;
    homeLastPlaceOdds: number;
    awayLastPlaceOdds: number;
  };
}

interface InteractiveSimulationProps {
  schedule: Schedule;
  startWeek: number;
  iterations?: number;
  autoRun?: boolean;
  onPivotalGamesCalculated?: (games: PivotalGame[]) => void;
}

export default function InteractiveSimulation({
  schedule,
  startWeek,
  iterations = 5000,
  autoRun = false,
  onPivotalGamesCalculated,
}: InteractiveSimulationProps) {
  const [simulator, setSimulator] = useState<Simulator | null>(null);
  const [simulationResults, setSimulationResults] = useState<TeamScoringData[]>(
    []
  );
  const [filteredResults, setFilteredResults] = useState<TeamScoringData[]>([]);
  const [matchingSimCount, setMatchingSimCount] = useState<number>(iterations);
  const [remainingMatchups, setRemainingMatchups] = useState<
    Map<number, Matchup[]>
  >(new Map());
  const [selectedResults, setSelectedResults] = useState<Map<string, number>>(
    new Map()
  );
  const [isSimulating, setIsSimulating] = useState(false);
  const [simulationComplete, setSimulationComplete] = useState(false);

  useEffect(() => {
    if (autoRun && schedule.length > 0 && !simulationComplete) {
      runSimulation();
    }
  }, [autoRun, schedule, simulationComplete]);

  const runSimulation = () => {
    setIsSimulating(true);
    try {
      // Create and run simulator
      const sim = new Simulator(schedule, startWeek);

      for (let i = 0; i < iterations; i++) {
        sim.step();
      }

      const results = sim.getTeamScoringData();
      setSimulationResults(results);
      setSimulator(sim);
      setFilteredResults(results);
      setMatchingSimCount(iterations);

      // Extract remaining matchups
      const matchupsByTeam = extractRemainingMatchups(schedule, startWeek);
      setRemainingMatchups(matchupsByTeam);

      // Calculate pivotal games
      if (onPivotalGamesCalculated) {
        const pivotalGames = sim.getMostImportantMatchups(3);
        onPivotalGamesCalculated(pivotalGames);
      }

      setSimulationComplete(true);
    } catch (error) {
      console.error("Simulation error:", error);
    } finally {
      setIsSimulating(false);
    }
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

      // Only include matchups from startWeek up to 4 weeks ahead
      if (currentWeek >= startWeek && currentWeek <= endWeek) {
        week.forEach((matchup) => {
          if (matchup.gameType !== "NONE") {
            return;
          }

          const homeTeamId = matchup.homeTeamESPNID;
          const awayTeamId = matchup.awayTeamESPNID;

          if (!teamMatchups.has(homeTeamId)) {
            teamMatchups.set(homeTeamId, []);
          }
          teamMatchups.get(homeTeamId)!.push(matchup);

          if (!teamMatchups.has(awayTeamId)) {
            teamMatchups.set(awayTeamId, []);
          }
          teamMatchups.get(awayTeamId)!.push(matchup);
        });
      }
    });

    return teamMatchups;
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

      if (currentWinner === undefined) {
        newResults.set(matchupKey, teamId);
      } else if (currentWinner === teamId) {
        newResults.set(matchupKey, opponentId);
      } else {
        newResults.delete(matchupKey);
      }

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

  const handleResetSelections = () => {
    setSelectedResults(new Map());
    setFilteredResults(simulationResults);
    setMatchingSimCount(iterations);
  };

  const calculateMatchingSimulations = () => {
    const percentage = (matchingSimCount / iterations) * 100;
    return {
      matching: matchingSimCount,
      total: iterations,
      percentage: percentage,
    };
  };

  const getDefaultOddsColor = (percentage: number): string => {
    if (percentage < 30) {
      return "text-red-600 dark:text-red-400";
    } else if (percentage > 70) {
      return "text-green-600 dark:text-green-400";
    }
    return "";
  };

  // Inverted color logic for last place odds (low = good, high = bad)
  const getDefaultLastPlaceOddsColor = (percentage: number): string => {
    if (percentage < 30) {
      return "text-green-600 dark:text-green-400";
    } else if (percentage > 70) {
      return "text-red-600 dark:text-red-400";
    }
    return "";
  };

  const getNewOddsColor = (
    newPercentage: number,
    defaultPercentage: number
  ): string => {
    const diff = newPercentage - defaultPercentage;
    if (Math.abs(diff) <= 3) {
      return ""; // No color for minimal change
    } else if (diff > 3) {
      return "text-green-600 dark:text-green-400";
    } else {
      return "text-red-600 dark:text-red-400";
    }
  };

  // Inverted color logic for last place odds (decrease = good, increase = bad)
  const getNewLastPlaceOddsColor = (
    newPercentage: number,
    defaultPercentage: number
  ): string => {
    const diff = newPercentage - defaultPercentage;
    if (Math.abs(diff) <= 3) {
      return ""; // No color for minimal change
    } else if (diff > 3) {
      return "text-red-600 dark:text-red-400"; // Increase is bad
    } else {
      return "text-green-600 dark:text-green-400"; // Decrease is good
    }
  };

  if (isSimulating) {
    return (
      <div className="p-8 text-center">
        <div className="inline-block w-8 h-8 border-4 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
        <p className="mt-4 text-gray-600 dark:text-gray-300">
          Running simulation...
        </p>
      </div>
    );
  }

  if (!simulationComplete || remainingMatchups.size === 0) {
    return null;
  }

  // Extract all unique weeks from remaining matchups and sort them
  const allWeeksSet = new Set<number>();
  remainingMatchups.forEach((matchups) => {
    matchups.forEach((matchup) => {
      allWeeksSet.add(matchup.week);
    });
  });
  const sortedWeeks = Array.from(allWeeksSet).sort((a, b) => a - b);

  const matchStats = calculateMatchingSimulations();

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-lg font-medium">Choose Your Own Results</h3>
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
        Click on matchups to select winners and see how results affect playoff
        odds
      </p>
      <div className="mb-4 p-3 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-md">
        <p className="text-sm font-medium text-blue-900 dark:text-blue-100">
          Matching Simulations: {matchStats.matching.toLocaleString()}/
          {matchStats.total.toLocaleString()} ({matchStats.percentage.toFixed(1)}
          %)
        </p>
        {selectedResults.size > 0 && (
          <p className="text-xs text-blue-700 dark:text-blue-300 mt-1">
            {selectedResults.size} game{selectedResults.size !== 1 ? "s" : ""}{" "}
            selected
          </p>
        )}
      </div>

      {matchStats.percentage < 0.5 && selectedResults.size > 0 && (
        <div className="mb-4 p-4 bg-gradient-to-r from-orange-50 to-red-50 dark:from-orange-900/20 dark:to-red-900/20 border-2 border-orange-400 dark:border-orange-600 rounded-lg shadow-md">
          <div className="flex items-center">
            <span className="text-2xl mr-3">🎲</span>
            <div>
              <p className="text-sm font-bold text-orange-900 dark:text-orange-200">
                This is a very unlikely scenario, keep dreaming partner!
              </p>
              <p className="text-xs text-orange-700 dark:text-orange-300 mt-1">
                Less than 1 in 200 simulations match your picks. Might want to
                reconsider your strategy...
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
                <span className="text-[10px] font-normal">(Default)</span>
              </th>
              <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                Playoff Odds
                <br />
                <span className="text-[10px] font-normal">(New)</span>
              </th>
              <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                Last Place Odds
                <br />
                <span className="text-[10px] font-normal">(Default)</span>
              </th>
              <th className="py-3 px-4 text-center text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                Last Place Odds
                <br />
                <span className="text-[10px] font-normal">(New)</span>
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
              .map((team, index) => {
                const teamMatchups = remainingMatchups.get(team.id) || [];
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
                    <td className="py-3 px-4 whitespace-nowrap font-medium">
                      {team.teamName}
                    </td>

                    {sortedWeeks.map((week) => {
                      const matchup = teamMatchups.find(
                        (m) => m.week === week
                      );

                      if (!matchup) {
                        return (
                          <td
                            key={week}
                            className="py-3 px-4 text-center w-32"
                          >
                            <div className="text-gray-400 text-xs">-</div>
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

                      const state = getMatchupState(matchup, team.id);

                      let buttonClasses =
                        "w-full px-2 py-2 rounded-md transition-colors cursor-pointer border ";
                      let textClasses = "text-xs";

                      if (state === "win") {
                        buttonClasses +=
                          "bg-green-100 dark:bg-green-900/30 border-green-300 dark:border-green-700 hover:bg-green-200 dark:hover:bg-green-900/50";
                        textClasses += " text-green-800 dark:text-green-200";
                      } else if (state === "loss") {
                        buttonClasses +=
                          "bg-red-100 dark:bg-red-900/30 border-red-300 dark:border-red-700 hover:bg-red-200 dark:hover:bg-red-900/50";
                        textClasses += " text-red-800 dark:text-red-200";
                      } else {
                        buttonClasses +=
                          "bg-blue-100 dark:bg-blue-900/30 border-blue-300 dark:border-blue-700 hover:bg-blue-200 dark:hover:bg-blue-900/50";
                        textClasses += " text-gray-700 dark:text-gray-300";
                      }

                      return (
                        <td key={week} className="py-3 px-4 text-center w-32">
                          <button
                            onClick={() =>
                              handleMatchupClick(matchup, team.id, opponentId)
                            }
                            className={buttonClasses}
                          >
                            <div className={textClasses}>
                              <div className="font-semibold">
                                Week {matchup.week}
                              </div>
                              <div className="truncate">
                                {isHomeTeam ? "vs" : "@"} {opponent}
                              </div>
                            </div>
                          </button>
                        </td>
                      );
                    })}

                    <td className="py-3 px-4 text-center">
                      <span
                        className={`font-medium ${getDefaultOddsColor(
                          team.playoffOdds * 100
                        )}`}
                      >
                        {(team.playoffOdds * 100).toFixed(1)}%
                      </span>
                    </td>

                    <td className="py-3 px-4 text-center">
                      <span
                        className={`font-medium ${
                          filteredTeam
                            ? getNewOddsColor(
                                filteredTeam.playoffOdds * 100,
                                team.playoffOdds * 100
                              )
                            : ""
                        }`}
                      >
                        {filteredTeam
                          ? (filteredTeam.playoffOdds * 100).toFixed(1)
                          : "0.0"}
                        %
                      </span>
                    </td>

                    <td className="py-3 px-4 text-center">
                      <span
                        className={`font-medium ${getDefaultLastPlaceOddsColor(
                          team.lastPlaceOdds * 100
                        )}`}
                      >
                        {(team.lastPlaceOdds * 100).toFixed(1)}%
                      </span>
                    </td>

                    <td className="py-3 px-4 text-center">
                      <span
                        className={`font-medium ${
                          filteredTeam
                            ? getNewLastPlaceOddsColor(
                                filteredTeam.lastPlaceOdds * 100,
                                team.lastPlaceOdds * 100
                              )
                            : ""
                        }`}
                      >
                        {filteredTeam
                          ? (filteredTeam.lastPlaceOdds * 100).toFixed(1)
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
}
