import { useState, useEffect } from "react";
import { useRouter } from "next/router";
import Link from "next/link";
import { Simulator } from "@/utils/simulator";
import { TeamScoringData, Schedule, Matchup } from "@/types/simulation";
import { scheduleService } from "@/services/scheduleService";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import ErrorState from "@/components/design-system/ErrorState";
import DataTable, {
  type DataTableColumn,
} from "@/components/design-system/DataTable";
import { FOCUS_RING } from "@/components/design-system/focus-ring";

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

/** `values[index]`, or undefined when the array is too short. */
function oddsAt(values: number[], index: number): number | undefined {
  return values.length > index ? values[index] : undefined;
}

/** Renders a 0..1 probability as a percentage, "0.0%" when absent. */
function formatOdds(value: number | undefined): string {
  return value !== undefined ? (value * 100).toFixed(1) + "%" : "0.0%";
}

/**
 * Graduated color for a "higher is better" finish-probability cell, falling back
 * to muted (not danger) when the probability is low or missing. Thresholds vary
 * per column, so every call site passes its own explicitly.
 */
function finishOddsColor(
  value: number | undefined,
  successAbove: number,
  warningAbove: number
): string {
  if (value !== undefined && value > successAbove) {
    return "var(--status-success-fg)";
  }
  if (value !== undefined && value > warningAbove) {
    return "var(--status-warning-fg)";
  }
  return "var(--text-muted)";
}

/** Projected Final Standings "Playoff %" column. */
function playoffOddsColor(odds: number): string {
  if (odds > 0.5) return "var(--status-success-fg)";
  if (odds > 0.25) return "var(--status-warning-fg)";
  return "var(--status-danger-fg)";
}

/** Playoff-odds bar fill (its own thresholds, distinct from the table column). */
function playoffOddsBarColor(odds: number): string {
  if (odds > 0.67) return "var(--status-success-fg)";
  if (odds > 0.33) return "var(--status-warning-fg)";
  return "var(--status-danger-fg)";
}

/** Projected Final Standings "Last Place %" column (higher odds = worse). */
function standingsLastPlaceColor(odds: number): string {
  if (odds > 0.5) return "var(--status-danger-fg)";
  if (odds > 0.25) return "var(--status-warning-fg)";
  return "var(--status-success-fg)";
}

/**
 * Regular Season Finish "Last Place" column. Deliberately separate from
 * standingsLastPlaceColor: this column's thresholds and comparison operators
 * differ (< 0.1 / <= 0.3).
 */
function regularSeasonLastPlaceColor(odds: number): string {
  if (odds < 0.1) return "var(--status-success-fg)";
  if (odds <= 0.3) return "var(--status-warning-fg)";
  return "var(--status-danger-fg)";
}

interface StandingsRow {
  team: TeamScoringData;
  rank: number;
}

export default function Simulations() {
  const router = useRouter();
  const leagueId = Number(router.query.leagueId);
  const [simulating, setSimulating] = useState(false);
  const [results, setResults] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [iterations, setIterations] = useState(10000);
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
    if (!leagueId || isNaN(leagueId)) return;
    const loadScheduleInfo = async () => {
      try {
        // Get all schedule data to determine available years
        const response = await scheduleService.getFullSchedule(leagueId);

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
  }, [leagueId]);

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
      const response = await scheduleService.getFullSchedule(leagueId);

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
  ): MatchupState => {
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

  // Each list below re-runs its own `.filter()` before sorting so the sort never
  // mutates `simulationResults` (or another list's ordering) — same as the
  // per-table filter/sort chains this markup replaced.
  const teamsByPlayoffOdds = simulationResults
    .filter((team) => team.teamName !== "League Average" && team.id !== -1)
    .sort((a, b) => b.playoffOdds - a.playoffOdds);

  const standingsRows: StandingsRow[] = simulationResults
    .filter((team) => team.teamName !== "League Average" && team.id !== -1)
    .sort((a, b) => b.wins - a.wins)
    .map((team, index) => ({ team, rank: index + 1 }));

  const teamsByChampionshipOdds = simulationResults
    .filter((team) => team.teamName !== "League Average" && team.id !== -1)
    .sort((a, b) => {
      // Sort by championship odds (1st place in playoffs)
      const aChampOdds = a.playoffResult.length > 0 ? a.playoffResult[0] : 0;
      const bChampOdds = b.playoffResult.length > 0 ? b.playoffResult[0] : 0;
      return bChampOdds - aChampOdds;
    });

  const standingsColumns: DataTableColumn<StandingsRow>[] = [
    {
      id: "rank",
      header: "Rank",
      cell: ({ rank }) => <span className="font-medium">{rank}</span>,
    },
    {
      id: "team",
      header: "Team",
      cell: ({ team }) => (
        <Link
          href={`/league/${leagueId}/teams/${team.id}`}
          className="hover:underline"
          style={{ color: "var(--action-primary)" }}
        >
          {team.teamName}
        </Link>
      ),
    },
    {
      id: "wins",
      header: "Avg Wins",
      cell: ({ team }) =>
        team.wins.toLocaleString(undefined, {
          minimumFractionDigits: 2,
          maximumFractionDigits: 2,
        }),
    },
    {
      id: "losses",
      header: "Avg Losses",
      cell: ({ team }) =>
        team.losses.toLocaleString(undefined, {
          minimumFractionDigits: 2,
          maximumFractionDigits: 2,
        }),
    },
    {
      id: "pointsFor",
      header: "Avg Points For",
      cell: ({ team }) =>
        team.pointsFor.toLocaleString(undefined, {
          minimumFractionDigits: 2,
          maximumFractionDigits: 2,
        }),
    },
    {
      id: "playoffOdds",
      header: "Playoff %",
      cell: ({ team }) => (
        <span
          className="font-medium"
          style={{ color: playoffOddsColor(team.playoffOdds) }}
        >
          {(team.playoffOdds * 100).toFixed(1)}%
        </span>
      ),
    },
    {
      id: "championOdds",
      header: "Champion %",
      cell: ({ team }) => (
        <span
          className="font-medium"
          style={{
            color: finishOddsColor(oddsAt(team.playoffResult, 0), 0.15, 0.05),
          }}
        >
          {formatOdds(oddsAt(team.playoffResult, 0))}
        </span>
      ),
    },
    {
      id: "lastPlaceOdds",
      header: "Last Place %",
      cell: ({ team }) => (
        <span
          className="font-medium"
          style={{ color: standingsLastPlaceColor(team.lastPlaceOdds) }}
        >
          {(team.lastPlaceOdds * 100).toFixed(1)}%
        </span>
      ),
    },
  ];

  const championshipColumns: DataTableColumn<TeamScoringData>[] = [
    {
      id: "team",
      header: "Team",
      cell: (team) => team.teamName,
    },
    {
      id: "champion",
      header: "Champion",
      align: "center",
      cell: (team) => (
        <span
          className="font-medium"
          style={{
            color: finishOddsColor(oddsAt(team.playoffResult, 0), 0.2, 0.1),
          }}
        >
          {formatOdds(oddsAt(team.playoffResult, 0))}
        </span>
      ),
    },
    {
      id: "runnerUp",
      header: "Runner-up",
      align: "center",
      cell: (team) => formatOdds(oddsAt(team.playoffResult, 1)),
    },
    {
      id: "third",
      header: "3rd Place",
      align: "center",
      cell: (team) => formatOdds(oddsAt(team.playoffResult, 2)),
    },
    {
      id: "fourth",
      header: "4th Place",
      align: "center",
      cell: (team) => formatOdds(oddsAt(team.playoffResult, 3)),
    },
  ];

  const seedColumn = (
    id: string,
    header: string,
    index: number,
    successAbove: number,
    warningAbove: number
  ): DataTableColumn<TeamScoringData> => ({
    id,
    header,
    align: "center",
    cell: (team) => (
      <span
        className="font-medium"
        style={{
          color: finishOddsColor(
            oddsAt(team.regularSeasonResult, index),
            successAbove,
            warningAbove
          ),
        }}
      >
        {formatOdds(oddsAt(team.regularSeasonResult, index))}
      </span>
    ),
  });

  const regularSeasonColumns: DataTableColumn<TeamScoringData>[] = [
    {
      id: "team",
      header: "Team",
      cell: (team) => team.teamName,
    },
    seedColumn("seed1", "1st Seed", 0, 0.2, 0.1),
    seedColumn("seed2", "2nd Seed", 1, 0.15, 0.08),
    seedColumn("seed3", "3rd Seed", 2, 0.12, 0.06),
    seedColumn("seed4", "4th Seed", 3, 0.1, 0.05),
    seedColumn("seed5", "5th Seed", 4, 0.1, 0.05),
    seedColumn("seed6", "6th Seed", 5, 0.1, 0.05),
    {
      id: "lastPlace",
      header: "Last Place",
      align: "center",
      cell: (team) => (
        <span
          className="font-medium"
          style={{ color: regularSeasonLastPlaceColor(team.lastPlaceOdds) }}
        >
          {(team.lastPlaceOdds * 100).toFixed(1)}%
        </span>
      ),
    },
  ];

  return (
    <div className="space-y-8">
      <section>
        <h1
          className="text-3xl md:text-4xl font-bold mb-6"
          style={{ color: "var(--text-primary)" }}
        >
          Run Fantasy Football Simulations
        </h1>
        <p
          className="text-lg mb-8 max-w-3xl"
          style={{ color: "var(--text-muted)" }}
        >
          Simulate the rest of your fantasy season to see projections for
          final standings, playoff odds, and championship probabilities.
        </p>

        {/* Simulation Parameters */}
        <Card>
          <CardContent className="p-6">
            <h2
              className="text-xl font-semibold mb-4"
              style={{ color: "var(--text-primary)" }}
            >
              Simulation Parameters
            </h2>

            <div className="space-y-4 mb-6">
              <div>
                <label
                  htmlFor="year"
                  className="block text-sm font-medium mb-1"
                  style={{ color: "var(--text-primary)" }}
                >
                  Season Year
                </label>
                <Select
                  value={selectedYear !== null ? String(selectedYear) : ""}
                  onValueChange={(v) => setSelectedYear(Number(v))}
                  disabled={availableYears.length === 0}
                >
                  <SelectTrigger
                    id="year"
                    aria-label="Season Year"
                    className="w-full md:w-64"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {availableYears.map((year) => (
                      <SelectItem key={year} value={String(year)}>
                        {year}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {availableYears.length === 0 && (
                  <p
                    className="text-xs mt-1"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Loading available years...
                  </p>
                )}
              </div>

              <div>
                <label
                  htmlFor="iterations"
                  className="block text-sm font-medium mb-1"
                  style={{ color: "var(--text-primary)" }}
                >
                  Number of Iterations
                </label>
                <input
                  type="number"
                  id="iterations"
                  value={iterations}
                  onChange={(e) => setIterations(Number(e.target.value))}
                  className={`w-full md:w-64 rounded-md border bg-transparent px-3 py-2 text-sm ${FOCUS_RING}`}
                  style={{
                    borderColor: "var(--border-subtle)",
                    color: "var(--text-primary)",
                  }}
                />
              </div>

              <div>
                <label
                  htmlFor="startWeek"
                  className="block text-sm font-medium mb-1"
                  style={{ color: "var(--text-primary)" }}
                >
                  Start Week
                  {scheduleLoaded && (
                    <span
                      className="text-xs ml-2"
                      style={{ color: "var(--text-muted)" }}
                    >
                      (Current: Week {currentWeek})
                    </span>
                  )}
                </label>
                <Select
                  value={startWeek}
                  onValueChange={(v) => setStartWeek(v)}
                  disabled={!scheduleLoaded}
                >
                  <SelectTrigger
                    id="startWeek"
                    aria-label="Start Week"
                    className="w-full md:w-64"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {availableWeeks.map((week) => (
                      <SelectItem key={week} value={week.toString()}>
                        Week {week}
                        {week === currentWeek ? " (Current)" : ""}
                        {week < currentWeek ? " (Past)" : ""}
                        {week > currentWeek ? " (Future)" : ""}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {!scheduleLoaded && (
                  <p
                    className="text-xs mt-1"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Loading schedule data...
                  </p>
                )}
              </div>
            </div>

            <Button size="lg" onClick={handleSimulation} disabled={simulating}>
              {simulating ? (
                <>
                  <Skeleton className="mr-2 size-4 rounded-full" />
                  Simulating...
                </>
              ) : (
                "Run Simulation"
              )}
            </Button>
          </CardContent>
        </Card>

        {/* Error Display */}
        {error && (
          <div className="mt-6 space-y-2">
            <ErrorState message={error} />
            <Button
              variant="outline"
              size="sm"
              onClick={() => setError(null)}
            >
              Dismiss
            </Button>
          </div>
        )}
      </section>

      {results && simulationResults.length > 0 && (
        <section
          className="p-6 rounded-lg"
          style={{ backgroundColor: "var(--surface-sunken)" }}
        >
          <h2
            className="text-xl font-semibold mb-4"
            style={{ color: "var(--text-primary)" }}
          >
            Simulation Results
          </h2>

          <div className="space-y-6">
            <p
              className="font-medium"
              style={{ color: "var(--status-success-fg)" }}
            >
              {results}
            </p>

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
                      <h3
                        className="text-lg font-medium"
                        style={{ color: "var(--text-primary)" }}
                      >
                        Choose Your Own Results
                      </h3>
                      {selectedResults.size > 0 && (
                        <Button
                          variant="destructive"
                          size="sm"
                          onClick={handleResetSelections}
                        >
                          Reset All
                        </Button>
                      )}
                    </div>
                    <p
                      className="text-sm mb-2"
                      style={{ color: "var(--text-muted)" }}
                    >
                      Click on matchups to select winners and see how results
                      affect playoff odds
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
                        Matching Simulations:{" "}
                        {matchStats.matching.toLocaleString()}/
                        {matchStats.total.toLocaleString()} (
                        {matchStats.percentage.toFixed(1)}%)
                      </p>
                      {selectedResults.size > 0 && (
                        <p
                          className="text-xs mt-1"
                          style={{ color: "var(--status-info-fg)" }}
                        >
                          {selectedResults.size} game
                          {selectedResults.size !== 1 ? "s" : ""} selected
                        </p>
                      )}
                    </div>

                    {matchStats.percentage < 0.5 &&
                      selectedResults.size > 0 && (
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
                                  This is a very unlikely scenario, keep
                                  dreaming partner!
                                </p>
                                <p
                                  className="text-xs mt-1"
                                  style={{ color: "var(--text-muted)" }}
                                >
                                  Less than 1 in 200 simulations match your
                                  picks. Might want to reconsider your
                                  strategy...
                                </p>
                              </div>
                            </div>
                          </CardContent>
                        </Card>
                      )}

                    <div className="overflow-x-auto">
                      <table className="min-w-full border-collapse">
                        <thead
                          style={{
                            backgroundColor: "var(--surface-sunken)",
                          }}
                        >
                          <tr
                            style={{
                              borderBottom: "1px solid var(--border-subtle)",
                            }}
                          >
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
                              <span className="text-[10px] font-normal">
                                (Default)
                              </span>
                            </th>
                            <th
                              className="py-3 px-4 text-center text-xs font-medium uppercase tracking-wider"
                              style={{ color: "var(--text-muted)" }}
                            >
                              Playoff Odds
                              <br />
                              <span className="text-[10px] font-normal">
                                (New)
                              </span>
                            </th>
                            <th
                              className="py-3 px-4 text-center text-xs font-medium uppercase tracking-wider"
                              style={{ color: "var(--text-muted)" }}
                            >
                              Last Place Odds
                              <br />
                              <span className="text-[10px] font-normal">
                                (Default)
                              </span>
                            </th>
                            <th
                              className="py-3 px-4 text-center text-xs font-medium uppercase tracking-wider"
                              style={{ color: "var(--text-muted)" }}
                            >
                              Last Place Odds
                              <br />
                              <span className="text-[10px] font-normal">
                                (New)
                              </span>
                            </th>
                          </tr>
                        </thead>
                        <tbody>
                          {teamsByPlayoffOdds.map((team, index) => {
                            const teamMatchups =
                              remainingMatchups.get(team.id) || [];

                            // Find the filtered result for this team
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
                                  borderBottom:
                                    "1px solid var(--border-subtle)",
                                }}
                              >
                                {/* Team Name */}
                                <td
                                  className="py-3 px-4 whitespace-nowrap font-medium"
                                  style={{ color: "var(--text-primary)" }}
                                >
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
                                        <div
                                          className="text-xs"
                                          style={{ color: "var(--text-muted)" }}
                                        >
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
                                  const stateStyle = MATCHUP_STYLES[state];

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
                                        className={`w-full px-2 py-2 rounded-md transition-colors cursor-pointer border ${MATCHUP_HOVER_CLASSES[state]}`}
                                        style={{
                                          backgroundColor:
                                            stateStyle.backgroundColor,
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
                                            {isHomeTeam ? "vs" : "@"}{" "}
                                            {opponent}
                                          </div>
                                        </div>
                                      </button>
                                    </td>
                                  );
                                })}

                                {/* Default Playoff Odds */}
                                <td
                                  className="py-3 px-4 text-center"
                                  style={{ color: "var(--text-primary)" }}
                                >
                                  <span className="font-medium">
                                    {(team.playoffOdds * 100).toFixed(1)}%
                                  </span>
                                </td>

                                {/* New Playoff Odds */}
                                <td
                                  className="py-3 px-4 text-center"
                                  style={{ color: "var(--text-primary)" }}
                                >
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
                                <td
                                  className="py-3 px-4 text-center"
                                  style={{ color: "var(--text-primary)" }}
                                >
                                  <span className="font-medium">
                                    {(team.lastPlaceOdds * 100).toFixed(1)}%
                                  </span>
                                </td>

                                {/* New Last Place Odds */}
                                <td
                                  className="py-3 px-4 text-center"
                                  style={{ color: "var(--text-primary)" }}
                                >
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
              <h3
                className="text-lg font-medium mb-2"
                style={{ color: "var(--text-primary)" }}
              >
                Playoff Odds
              </h3>
              <Card>
                <CardContent className="p-4">
                  <p
                    className="text-sm mb-4"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Probability of making playoffs
                  </p>

                  <div className="space-y-3">
                    {teamsByPlayoffOdds.map((team) => (
                      <div key={team.id} className="flex items-center">
                        <span
                          className="w-32 text-sm truncate"
                          title={team.teamName}
                        >
                          {team.teamName}
                        </span>
                        <div
                          className="flex-1 h-5 rounded-full overflow-hidden mx-3"
                          style={{
                            backgroundColor: "var(--surface-sunken)",
                          }}
                        >
                          <div
                            className="h-full"
                            style={{
                              width: `${(team.playoffOdds * 100).toFixed(1)}%`,
                              backgroundColor: playoffOddsBarColor(
                                team.playoffOdds
                              ),
                            }}
                          ></div>
                        </div>
                        <span className="w-14 text-right text-sm">
                          {(team.playoffOdds * 100).toFixed(1)}%
                        </span>
                      </div>
                    ))}
                  </div>
                </CardContent>
              </Card>
            </div>

            <div>
              <h3
                className="text-lg font-medium mb-2"
                style={{ color: "var(--text-primary)" }}
              >
                Projected Final Standings
              </h3>
              <DataTable
                columns={standingsColumns}
                rows={standingsRows}
                rowKey={({ team }) => String(team.id)}
              />
            </div>

            {/* Championship and Playoff Results */}
            <div>
              <h3
                className="text-lg font-medium mb-2"
                style={{ color: "var(--text-primary)" }}
              >
                Championship Odds
              </h3>
              <DataTable
                columns={championshipColumns}
                rows={teamsByChampionshipOdds}
                rowKey={(team) => String(team.id)}
              />
            </div>

            {/* Regular Season Finish */}
            <div>
              <h3
                className="text-lg font-medium mb-2"
                style={{ color: "var(--text-primary)" }}
              >
                Regular Season Finish
              </h3>
              <DataTable
                columns={regularSeasonColumns}
                rows={teamsByPlayoffOdds}
                rowKey={(team) => String(team.id)}
              />
            </div>
          </div>
        </section>
      )}
    </div>
  );
}
