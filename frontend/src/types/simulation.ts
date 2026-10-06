// Base interfaces
export interface TeamStats {
  average: number;
  std_dev: number;
}

export interface LeagueStats {
  mean: number;
  stdDev: number;
}

export interface TeamAverage {
  id: number;
  owner: string;
  averageScore: number;
  stddevScore: number;
}

// Matchup interfaces
export interface Matchup {
  homeTeamName: string;
  awayTeamName: string;
  homeTeamESPNID: number;
  awayTeamESPNID: number;
  homeTeamFinalScore: number;
  awayTeamFinalScore: number;
  completed: boolean;
  gameType: string;
  week: number;
}

export type Schedule = Matchup[][];

// Single team result for one simulation
export interface SingleTeamResult {
  id: number;
  wins: number;
  losses: number;
  pointsFor: number;
  pointsAgainst: number;
  madePlayoffs: boolean;
  lastPlace: boolean;
  regularSeasonResult: number;
  playoffResult: number;
}

// Aggregated results across all simulations
export interface TeamResult {
  wins: number;
  losses: number;
  pointsFor: number;
  pointsAgainst: number;
  madePlayoffs: number;
  lastPlace: number;
  regularSeasonResult: number[];
  playoffResult: number[];
}

// Team scoring data output
export interface TeamScoringData {
  id: number;
  teamName: string;
  average: number;
  stdDev: number;
  wins: number;
  losses: number;
  pointsFor: number;
  pointsAgainst: number;
  playoffOdds: number;
  lastPlaceOdds: number;
  regularSeasonResult: number[];
  playoffResult: number[];
}

// Simulation parameters
export interface SimulationParams {
  iterations: number;
  startWeek: string | number;
  useActualResults: boolean;
}

// Store matchup outcome for a single iteration
export interface MatchupOutcome {
  week: number;
  homeTeamId: number;
  awayTeamId: number;
  winnerId: number;
}

// Store all matchup outcomes for a single simulation iteration
export interface SimulationIteration {
  matchupOutcomes: MatchupOutcome[];
  teamResults: Map<number, SingleTeamResult>;
}

/** Probabilities (0–1) across sampled combinations of one week's winners. */
export interface WeeklyOddsRange {
  baseline: number;
  min: number;
  max: number;
}

export interface WeeklyTeamLeverage {
  teamId: number;
  teamName: string;
  playoff: WeeklyOddsRange;
  lastPlace: WeeklyOddsRange;
}

export interface WeeklyLeverage {
  week: number;
  scenarioCount: number;
  possibleScenarioCount: number;
  smallestScenarioCount: number;
  teams: WeeklyTeamLeverage[];
}

// One team's playoff/last-place odds under each outcome of a single game, plus
// how far those outcomes move it away from its baseline odds.
export interface BigGameTeamOdds {
  teamId: number;
  teamName: string;
  baselinePlayoffOdds: number;
  baselineLastPlaceOdds: number;
  homeWinPlayoffOdds: number;
  homeWinLastPlaceOdds: number;
  awayWinPlayoffOdds: number;
  awayWinLastPlaceOdds: number;
  /**
   * |homeWin − baseline| + |awayWin − baseline| across both playoff and
   * last-place odds. Because baseline is the convex combination of the two
   * outcomes, this also equals |homeWin − awayWin| summed over both metrics.
   */
  swing: number;
}

// An upcoming game ranked by how much its outcome reshuffles the league.
export interface BigGame {
  week: number;
  homeTeamId: number;
  awayTeamId: number;
  homeTeamName: string;
  awayTeamName: string;
  /** Every team's swing added together, so it can exceed 1. */
  totalSwing: number;
  /** All teams, ordered by their own swing descending. */
  teams: BigGameTeamOdds[];
}
