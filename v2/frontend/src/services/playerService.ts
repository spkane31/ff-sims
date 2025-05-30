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
   * Get all players with stats
   */
  getAllPlayers: async (year?: string | number): Promise<Player[]> => {
    const endpoint = year ? `/boxscoreplayers?year=${year}` : '/boxscoreplayers';
    const response = await apiClient.get<{ data: Player[], total: number, page_size: number }>(endpoint);
    return response.data;
  },

  /**
   * Get players for a specific team
   */
  getTeamPlayers: async (teamId: number): Promise<Player[]> => {
    return apiClient.get<Player[]>(`/teams/${teamId}/players`);
  },

  /**
   * Get players by position
   */
  getPlayersByPosition: async (position: string): Promise<Player[]> => {
    return apiClient.get<Player[]>(`/boxscoreplayers?position=${position}`);
  },

  /**
   * Get draft data
   */
  getDraft: async (year?: number): Promise<Player[]> => {
    const endpoint = year ? `/draft?year=${year}` : '/draft';
    return apiClient.get<Player[]>(endpoint);
  }
};