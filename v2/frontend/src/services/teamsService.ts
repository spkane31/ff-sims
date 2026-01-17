import { apiClient } from "./apiClient";

// Define TypeScript interfaces for your team data
export interface TeamRecord {
  wins: number;
  losses: number;
  ties: number;
}

export interface TeamPoints {
  scored: number;
  against: number;
}

export interface TeamExpectedWins {
  expectedWins: number;
  expectedLosses: number;
  winLuck: number;
  seasonsPlayed: number;
}

export interface Team {
  id: string;
  teamId: string;
  name: string;
  owner: string;
  record: TeamRecord;
  playoffRecord: TeamRecord;
  points: TeamPoints;
  expectedWins?: TeamExpectedWins;
  rank: number;
  playoffChance: number;
}

export interface TeamResponse {
  teams: Team[];
}

// Extended interfaces for detailed team view
export interface PlayerStats {
  passingYards: number;
  passingTDs: number;
  interceptions: number;
  rushingYards: number;
  rushingTDs: number;
  receptions: number;
  receivingYards: number;
  receivingTDs: number;
  fumbles: number;
  fieldGoals: number;
  extraPoints: number;
}

export interface Player {
  id: string;
  name: string;
  position: string;
  team: string;
  status: string;
  fantasyPoints: number;
  stats: PlayerStats;
}

export interface DraftPick {
  round: number;
  pick: number;
  player: string;
  position: string;
  team: string;
  year: number;
}

export interface Transaction {
  id: string;
  type: string;
  date: string;
  year: number;
  week: number;
  description: string;
  playersGained: {
    id: string;
    name: string;
  }[];
  playersLost: {
    id: string;
    name: string;
  }[];
}

export interface ScheduleGame {
  week: number;
  year: number;
  opponent: string;
  opponentTeamID: string; // Platform-specific team ID (ESPN/Sleeper)
  opponentInternalID: string; // Internal database ID for linking
  isHome: boolean;
  teamScore: number;
  opponentScore: number;
  result: string; // "W", "L", "T", or "Upcoming"
  completed: boolean;
  isPlayoff: boolean;
  matchupId?: string; // Add matchup ID for linking to schedule detail page
}

export interface TeamDetail {
  id: string;
  teamId: string;
  name: string;
  owner: string;
  record: TeamRecord;
  points: TeamPoints;
  schedule: ScheduleGame[];
  currentPlayers: Player[];
  draftPicks: DraftPick[];
  transactions: Transaction[];
}

/**
 * Teams API service
 */
export const teamsService = {
  /**
   * Get all teams for a specific league
   */
  getAllTeams: async (leagueId: string, season: number): Promise<TeamResponse> => {
    return apiClient.get<TeamResponse>(`/league/${leagueId}/teams?season=${season}`);
  },

  /**
   * Get a single team by ID for a specific league
   */
  getTeamById: async (leagueId: string, teamId: number, season: number): Promise<Team> => {
    return apiClient.get<Team>(`/league/${leagueId}/teams/${teamId}?season=${season}`);
  },

  /**
   * Get detailed team information including schedule, players, draft picks, and transactions
   */
  getTeamDetail: async (leagueId: string, teamId: string | number, season: number): Promise<TeamDetail> => {
    return apiClient.get<TeamDetail>(`/league/${leagueId}/teams/${teamId}?season=${season}`);
  },

  /**
   * Get team standings for a specific league
   */
  getTeamStandings: async (leagueId: string): Promise<Team[]> => {
    return apiClient.get<Team[]>(`/league/${leagueId}/teams/standings`);
  },
};
