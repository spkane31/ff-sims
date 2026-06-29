import { apiClient } from './apiClient';

export interface League {
  id: number;
  name: string;
  platform: string;
  external_id: string;
  current_week: number;
  total_weeks: number;
}

export interface GetLeaguesResponse {
  leagues: League[];
}

export interface LeagueYearsResponse {
  years: number[];
}

export const leaguesService = {
  getLeagues: async (): Promise<GetLeaguesResponse> => {
    return apiClient.get<GetLeaguesResponse>('/leagues');
  },

  getLeague: async (leagueId: number): Promise<League> => {
    return apiClient.get<League>(`/leagues/${leagueId}`);
  },

  getLeagueYears: async (leagueId: number): Promise<LeagueYearsResponse> => {
    return apiClient.get<LeagueYearsResponse>(`/leagues/${leagueId}/years`);
  },
};
