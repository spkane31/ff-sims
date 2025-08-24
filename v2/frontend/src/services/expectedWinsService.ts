import { apiClient } from "./apiClient";

export interface AllTimeExpectedWins {
  team_id: number;
  team_name: string;
  owner: string;
  total_expected_wins: number;
  total_expected_losses: number;
  total_actual_wins: number;
  total_actual_losses: number;
  total_win_luck: number;
  seasons_played: number;
}

export interface AllTimeExpectedWinsResponse {
  data: AllTimeExpectedWins[];
}

/**
 * Expected Wins API service
 */
export const expectedWinsService = {
  /**
   * Get all-time expected wins for all teams
   */
  getAllTimeExpectedWins: async (): Promise<AllTimeExpectedWinsResponse> => {
    return apiClient.get<AllTimeExpectedWinsResponse>("/teams/all-time-expected-wins");
  },
};