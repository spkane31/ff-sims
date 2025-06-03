import { apiClient } from './apiClient';

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

/**
 * Teams API service
 */
export const teamsService = {
  /**
   * Get all teams
   */
  getAllTeams: async (): Promise<TeamResponse> => {
    return apiClient.get<TeamResponse>('/teams');
  },

  /**
   * Get a single team by ID
   */
  getTeamById: async (teamId: number): Promise<Team> => {
    return apiClient.get<Team>(`/teams/${teamId}`);
  },

  /**
   * Get team standings
   */
  getTeamStandings: async (): Promise<Team[]> => {
    return apiClient.get<Team[]>('/teams/standings');
  },
};