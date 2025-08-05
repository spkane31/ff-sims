/**
 * TypeScript interfaces matching backend Go models
 * Generated from v2/backend/internal/models/*.go
 */

// Base model with common fields
export interface BaseModel {
  id: number;
  createdAt: string;
  updatedAt: string;
}

// Matchup model
export interface Matchup extends BaseModel {
  leagueId: number;
  week: number;
  year: number;
  season: number;
  homeTeamId: number;
  awayTeamId: number;
  homeTeamName: string;
  awayTeamName: string;
  homeTeamEspnId: number;
  awayTeamEspnId: number;
  homeScore: number;
  awayScore: number;
  homeProjectedScore: number;
  awayProjectedScore: number;
  completed: boolean;
  isPlayoff: boolean;
  homeTeam?: Team;
  awayTeam?: Team;
}

// Player stats model
export interface PlayerStats {
  passing_yards: number;
  passing_tds: number;
  interceptions: number;
  rushing_yards: number;
  rushing_tds: number;
  receptions: number;
  receiving_yards: number;
  receiving_tds: number;
  fumbles: number;
  field_goals: number;
  extra_points: number;
}

// Player game stats model
export interface PlayerGameStats extends BaseModel {
  player_id: number;
  player_name: string;
  week: number;
  season: number;
  game_stats: PlayerStats;
  fantasy_points: number;
}

// Player model
export interface Player extends BaseModel {
  name: string;
  position: string; // QB, RB, WR, TE, K, DEF
  team: string; // NFL team abbreviation
  fantasy_points: number;
  status: string; // Active, Injured, etc.
  stats: PlayerStats;
  game_stats?: PlayerGameStats[];
}

// Roster settings model
export interface RosterSettings {
  qb: number;
  rb: number;
  wr: number;
  te: number;
  flex: number;
  k: number;
  dst: number;
  bn: number;
  ir: number;
}

// Scoring settings model
export interface ScoringSettings {
  passing_yards: number;
  passing_td: number;
  interception: number;
  rushing_yards: number;
  rushing_td: number;
  reception: number;
  receiving_yards: number;
  receiving_td: number;
  fumble: number;
  field_goal_0to39: number;
  field_goal_40to49: number;
  field_goal_50plus: number;
  extra_point: number;
}

// League model
export interface League extends BaseModel {
  name: string;
  description: string;
  scoring_type: string; // Standard, PPR, Half-PPR
  teams?: Team[];
  season: number;
  current_week: number;
  total_weeks: number;
  playoff_weeks: number;
  roster_settings: RosterSettings;
  scoring_settings: ScoringSettings;
  matchups?: Matchup[];
}

// Team model
export interface Team extends BaseModel {
  name: string;
  owner_name: string;
  espn_id: number;
  league_id: number;
  wins: number;
  losses: number;
  ties: number;
  points: number;
  players?: Player[];
  league?: League;
}

// Simulation model
export interface Simulation extends BaseModel {
  league_id: number;
  name: string;
  description: string;
  season: number;
  start_week: number;
  end_week: number;
  num_simulations: number;
  completed: boolean;
  var_factor: number;
  results?: SimResult[];
  team_results?: SimTeamResult[];
}

// Simulation result model
export interface SimResult extends BaseModel {
  simulation_id: number;
  matchup_id: number;
  team_id: number;
  opponent_id: number;
  score: number;
  opponent_score: number;
  win: boolean;
  sim_run: number;
}

// Simulation team result model
export interface SimTeamResult extends BaseModel {
  simulation_id: number;
  team_id: number;
  wins: number;
  losses: number;
  playoff_odds: number;
  championship_odds: number;
  avg_points: number;
}

// Response types for API calls
export interface GetTeamsResponse {
  data: Team[];
}

export interface GetTeamResponse {
  data: Team;
}

export interface GetLeagueResponse {
  data: League;
}

export interface GetMatchupsResponse {
  data: {
    matchups: Matchup[];
  };
}

export interface GetMatchupResponse {
  data: Matchup;
}

export interface GetPlayersResponse {
  data: Player[];
}

export interface GetPlayerResponse {
  data: Player;
}

export interface GetSimulationsResponse {
  data: Simulation[];
}

export interface GetSimulationResponse {
  data: Simulation;
}
