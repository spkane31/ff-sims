import { apiClient } from './apiClient';

export interface LeagueYearsResponse {
  years: number[];
}

/**
 * Leagues API service
 */
export const leaguesService = {
  /**
   * Get all years the league has been active
   */
  getLeagueYears: async (leagueId: number = 345674): Promise<LeagueYearsResponse> => {
    return apiClient.get<LeagueYearsResponse>(`/leagues/years?league_id=${leagueId}`);
  },
};