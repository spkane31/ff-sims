import { apiClient } from './apiClient';

export interface League {
  id: string;
  name: string;
}

export interface LeaguesResponse {
  leagues: League[];
}

export interface LeagueYearsResponse {
  years: number[];
}

/**
 * Leagues API service
 */
export const leaguesService = {
  /**
   * Get all leagues
   */
  getAllLeagues: async (): Promise<LeaguesResponse> => {
    return apiClient.get<LeaguesResponse>("/leagues");
  },

  /**
   * Get all years that a league has been active
   */
  getLeagueYears: async (leagueId: string): Promise<LeagueYearsResponse> => {
    return apiClient.get<LeagueYearsResponse>(`/league/${leagueId}/leagues/years`);
  },
};