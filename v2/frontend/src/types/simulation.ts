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
  homeTeamEspnId: number;
  awayTeamEspnId: number;
  homeTeamFinalScore: number;
  awayTeamFinalScore: number;
  completed: boolean;
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
  playoff_odds: number;
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
