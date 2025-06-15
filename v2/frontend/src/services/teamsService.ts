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

export interface Team {
  id: string;
  espnId: string;
  name: string;
  owner: string;
  record: TeamRecord;
  points: TeamPoints;
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
  description: string;
  playersGained: {
    id: string;
    name: string;
  }[];
  playersLost: {
    id: string;
    name: string;
  }[];
  week: number;
}

export interface ScheduleGame {
  week: number;
  year: number;
  opponent: string;
  isHome: boolean;
  teamScore: number;
  opponentScore: number;
  result: string; // "W", "L", "T", or "Upcoming"
  completed: boolean;
  isPlayoff: boolean;
}

export interface TeamDetail {
  id: string;
  espnId: string;
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
   * Get all teams
   */
  getAllTeams: async (): Promise<TeamResponse> => {
    return apiClient.get<TeamResponse>("/teams");
  },

  /**
   * Get a single team by ID
   */
  getTeamById: async (teamId: number): Promise<Team> => {
    return apiClient.get<Team>(`/teams/${teamId}`);
  },

  /**
   * Get detailed team information including schedule, players, draft picks, and transactions
   */
  getTeamDetail: async (teamId: string | number): Promise<TeamDetail> => {
    return apiClient.get<TeamDetail>(`/teams/${teamId}`);
  },

  /**
   * Get team standings
   */
  getTeamStandings: async (): Promise<Team[]> => {
    return apiClient.get<Team[]>("/teams/standings");
  },
};
