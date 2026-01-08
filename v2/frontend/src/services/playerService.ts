import { apiClient } from './apiClient';

// Define interfaces for player data
export interface Player {
  id: number;
  name: string;
  position: string;
  team: string;
  owner?: number;
  ownerName?: string;
  totalProjectedPoints: number;
  totalActualPoints: number;
  difference: number;
  positionRank: number;
  vorp: number; // Value Over Replacement Player
}

/**
 * Player API service
 */
export const playerService = {
  /**
   * Get all players with stats for a specific league
   */
  getAllPlayers: async (leagueId: string, year?: string | number): Promise<Player[]> => {
    const endpoint = year ? `/league/${leagueId}/boxscoreplayers?year=${year}` : `/league/${leagueId}/boxscoreplayers`;
    const response = await apiClient.get<{ data: Player[], total: number, page_size: number }>(endpoint);
    return response.data;
  },

  /**
   * Get players for a specific team in a league
   */
  getTeamPlayers: async (leagueId: string, teamId: number): Promise<Player[]> => {
    return apiClient.get<Player[]>(`/league/${leagueId}/teams/${teamId}/players`);
  },

  /**
   * Get players by position for a specific league
   */
  getPlayersByPosition: async (leagueId: string, position: string): Promise<Player[]> => {
    return apiClient.get<Player[]>(`/league/${leagueId}/boxscoreplayers?position=${position}`);
  },

  /**
   * Get draft data for a specific league
   */
  getDraft: async (leagueId: string, year?: number): Promise<Player[]> => {
    const endpoint = year ? `/league/${leagueId}/draft?year=${year}` : `/league/${leagueId}/draft`;
    return apiClient.get<Player[]>(endpoint);
  }
};