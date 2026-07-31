import { useState, useEffect } from "react";
import { Simulator } from "../utils/simulator";
import { Schedule, Matchup, TeamScoringData } from "../types/simulation";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

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

type MatchupState = "win" | "loss" | "none";

// Static class strings (not template-built) so Tailwind's content scanner can
// find these arbitrary-value utilities even though the state key is picked at
// runtime.
const MATCHUP_HOVER_CLASSES: Record<MatchupState, string> = {
  win: "hover:bg-[color-mix(in_oklch,var(--status-success-bg),var(--status-success-fg)_20%)]",
  loss: "hover:bg-[color-mix(in_oklch,var(--status-danger-bg),var(--status-danger-fg)_20%)]",
  none: "hover:bg-[color-mix(in_oklch,var(--surface-sunken),var(--text-primary)_8%)]",
};

const MATCHUP_STYLES: Record<
  MatchupState,
  { backgroundColor: string; borderColor: string; color: string }
> = {
  win: {
    backgroundColor: "var(--status-success-bg)",
    borderColor: "var(--status-success-fg)",
    color: "var(--status-success-fg)",
  },
  loss: {
    backgroundColor: "var(--status-danger-bg)",
    borderColor: "var(--status-danger-fg)",
    color: "var(--status-danger-fg)",
  },
  none: {
    backgroundColor: "var(--surface-sunken)",
    borderColor: "var(--border-subtle)",
    color: "var(--text-secondary)",
  },
};

export default function InteractiveSimulation({
  schedule,
  startWeek,
  iterations = 10000,
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
  ): MatchupState => {
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

  const getDefaultOddsColor = (percentage: number): string | undefined => {
    if (percentage < 30) {
      return "var(--status-danger-fg)";
    } else if (percentage > 70) {
      return "var(--status-success-fg)";
    }
    return undefined;
  };

  // Inverted color logic for last place odds (low = good, high = bad)
  const getDefaultLastPlaceOddsColor = (percentage: number): string | undefined => {
    if (percentage < 5) {
      return "var(--status-success-fg)";
    } else if (percentage >= 5 && percentage <= 20) {
      return "var(--status-warning-fg)";
    } else {
      return "var(--status-danger-fg)";
    }
  };

  const getNewOddsColor = (
    newPercentage: number,
    defaultPercentage: number
  ): string | undefined => {
    const diff = newPercentage - defaultPercentage;
    if (Math.abs(diff) <= 3) {
      return undefined; // No color for minimal change
    } else if (diff > 3) {
      return "var(--status-success-fg)";
    } else {
      return "var(--status-danger-fg)";
    }
  };

  // Inverted color logic for last place odds (decrease = good, increase = bad)
  const getNewLastPlaceOddsColor = (
    newPercentage: number,
    defaultPercentage: number
  ): string | undefined => {
    const diff = newPercentage - defaultPercentage;
    if (Math.abs(diff) <= 3) {
      return undefined; // No color for minimal change
    } else if (diff > 3) {
      return "var(--status-danger-fg)"; // Increase is bad
    } else {
      return "var(--status-success-fg)"; // Decrease is good
    }
  };

  if (isSimulating) {
    return (
      <div className="p-8">
        <div className="mb-4 flex items-center justify-center">
          <Skeleton className="h-5 w-48" />
        </div>
        <div className="space-y-2">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
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
        <h3
          className="text-lg font-medium"
          style={{ color: "var(--text-primary)" }}
        >
          Choose Your Own Results
        </h3>
        {selectedResults.size > 0 && (
          <Button variant="destructive" size="sm" onClick={handleResetSelections}>
            Reset All
          </Button>
        )}
      </div>
      <p className="text-sm mb-2" style={{ color: "var(--text-muted)" }}>
        Click on matchups to select winners and see how results affect playoff
        odds
      </p>
      <div
        className="mb-4 p-3 rounded-md border"
        style={{
          backgroundColor: "var(--status-info-bg)",
          borderColor: "var(--status-info-fg)",
        }}
      >
        <p
          className="text-sm font-medium"
          style={{ color: "var(--status-info-fg)" }}
        >
          Matching Simulations: {matchStats.matching.toLocaleString()}/
          {matchStats.total.toLocaleString()} (
          {matchStats.percentage.toFixed(1)}
          %)
        </p>
        {selectedResults.size > 0 && (
          <p
            className="text-xs mt-1"
            style={{ color: "var(--status-info-fg)" }}
          >
            {selectedResults.size} game{selectedResults.size !== 1 ? "s" : ""}{" "}
            selected
          </p>
        )}
      </div>

      {matchStats.percentage < 0.5 && selectedResults.size > 0 && (
        <Card
          className="mb-4 border-l-4"
          style={{ borderLeftColor: "var(--status-warning-fg)" }}
        >
          <CardContent className="p-4">
            <div className="flex items-center">
              <span className="text-2xl mr-3">🎲</span>
              <div>
                <p
                  className="text-sm font-bold"
                  style={{ color: "var(--status-warning-fg)" }}
                >
                  This is a very unlikely scenario, keep dreaming partner!
                </p>
                <p className="text-xs mt-1" style={{ color: "var(--text-muted)" }}>
                  Less than 1 in 200 simulations match your picks. Might want to
                  reconsider your strategy...
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      <div className="overflow-x-auto">
        <table className="min-w-full border-collapse">
          <thead style={{ backgroundColor: "var(--surface-sunken)" }}>
            <tr style={{ borderBottom: "1px solid var(--border-subtle)" }}>
              <th
                className="py-3 px-4 text-left text-xs font-medium uppercase tracking-wider"
                style={{ color: "var(--text-muted)" }}
              >
                Team
              </th>
              {sortedWeeks.map((week) => (
                <th
                  key={week}
                  className="py-3 px-4 text-center text-xs font-medium uppercase tracking-wider w-32"
                  style={{ color: "var(--text-muted)" }}
                >
                  Week {week}
                </th>
              ))}
              <th
                className="py-3 px-4 text-center text-xs font-medium uppercase tracking-wider"
                style={{ color: "var(--text-muted)" }}
              >
                Playoff Odds
                <br />
                <span className="text-[10px] font-normal">(Default)</span>
              </th>
              <th
                className="py-3 px-4 text-center text-xs font-medium uppercase tracking-wider"
                style={{ color: "var(--text-muted)" }}
              >
                Playoff Odds
                <br />
                <span className="text-[10px] font-normal">(New)</span>
              </th>
              <th
                className="py-3 px-4 text-center text-xs font-medium uppercase tracking-wider"
                style={{ color: "var(--text-muted)" }}
              >
                Last Place Odds
                <br />
                <span className="text-[10px] font-normal">(Default)</span>
              </th>
              <th
                className="py-3 px-4 text-center text-xs font-medium uppercase tracking-wider"
                style={{ color: "var(--text-muted)" }}
              >
                Last Place Odds
                <br />
                <span className="text-[10px] font-normal">(New)</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {simulationResults
              .filter(
                (team) => team.teamName !== "League Average" && team.id !== -1
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
                    style={{
                      backgroundColor:
                        index % 2 === 0
                          ? "var(--surface-raised)"
                          : "var(--surface-sunken)",
                      borderBottom: "1px solid var(--border-subtle)",
                    }}
                  >
                    <td
                      className="py-3 px-4 whitespace-nowrap font-medium"
                      style={{ color: "var(--text-primary)" }}
                    >
                      {team.teamName}
                    </td>

                    {sortedWeeks.map((week) => {
                      const matchup = teamMatchups.find((m) => m.week === week);

                      if (!matchup) {
                        return (
                          <td key={week} className="py-3 px-4 text-center w-32">
                            <div
                              className="text-xs"
                              style={{ color: "var(--text-muted)" }}
                            >
                              -
                            </div>
                          </td>
                        );
                      }

                      const isHomeTeam = matchup.homeTeamESPNID === team.id;
                      const opponentId = isHomeTeam
                        ? matchup.awayTeamESPNID
                        : matchup.homeTeamESPNID;
                      const opponent = isHomeTeam
                        ? matchup.awayTeamName
                        : matchup.homeTeamName;

                      const state = getMatchupState(matchup, team.id);
                      const stateStyle = MATCHUP_STYLES[state];

                      return (
                        <td key={week} className="py-3 px-4 text-center w-32">
                          <button
                            onClick={() =>
                              handleMatchupClick(matchup, team.id, opponentId)
                            }
                            className={`w-full px-2 py-2 rounded-md transition-colors cursor-pointer border ${MATCHUP_HOVER_CLASSES[state]}`}
                            style={{
                              backgroundColor: stateStyle.backgroundColor,
                              borderColor: stateStyle.borderColor,
                            }}
                          >
                            <div
                              className="text-xs"
                              style={{ color: stateStyle.color }}
                            >
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
                        className="font-medium"
                        style={{ color: getDefaultOddsColor(team.playoffOdds * 100) }}
                      >
                        {(team.playoffOdds * 100).toFixed(1)}%
                      </span>
                    </td>

                    <td className="py-3 px-4 text-center">
                      <span
                        className="font-medium"
                        style={{
                          color: filteredTeam
                            ? getNewOddsColor(
                                filteredTeam.playoffOdds * 100,
                                team.playoffOdds * 100
                              )
                            : undefined,
                        }}
                      >
                        {filteredTeam
                          ? (filteredTeam.playoffOdds * 100).toFixed(1)
                          : "0.0"}
                        %
                      </span>
                    </td>

                    <td className="py-3 px-4 text-center">
                      <span
                        className="font-medium"
                        style={{
                          color: getDefaultLastPlaceOddsColor(
                            team.lastPlaceOdds * 100
                          ),
                        }}
                      >
                        {(team.lastPlaceOdds * 100).toFixed(1)}%
                      </span>
                    </td>

                    <td className="py-3 px-4 text-center">
                      <span
                        className="font-medium"
                        style={{
                          color: filteredTeam
                            ? getNewLastPlaceOddsColor(
                                filteredTeam.lastPlaceOdds * 100,
                                team.lastPlaceOdds * 100
                              )
                            : undefined,
                        }}
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
