// Base interfaces
export interface TeamStats {
  average: number;
  std_dev: number;
}

export interface LeagueStats {
  average: number;
  std_dev: number;
}

export interface TeamAverage {
  id: number;
  owner: string;
  averageScore: number;
  stddevScore: number;
}

// Matchup interfaces
export interface Matchup {
  home_team_espn_id: number;
  away_team_espn_id: number;
  home_team_final_score: number;
  away_team_final_score: number;
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
  std_dev: number;
  wins: number;
  losses: number;
  pointsFor: number;
  pointsAgainst: number;
  playoff_odds: number;
  last_place_odds: number;
  regular_season_result: number[];
  playoff_result: number[];
}

// Simulation parameters
export interface SimulationParams {
  iterations: number;
  startWeek: string | number;
  useActualResults: boolean;
}
