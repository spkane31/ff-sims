import { apiClient } from './apiClient';

export interface DraftPick {
  player_id: string;
  round: number;
  pick: number;
  player: string;
  position: string;
  team_id: number;
  owner: string;
  year: number;
}

export interface DraftPicksResponse {
  draft_picks: DraftPick[];
}

export interface Transaction {
  id: number;
  date: string;
  type: 'draft' | 'trade' | 'waiver';
  description: string;
  teams: string[];
  players: {
    id: string;
    name: string;
    position: string;
    team: string;
    points: number;
  }[];
}

export interface TransactionsResponse {
  transactions: Transaction[];
}

/**
 * Transactions API service
 */
export const transactionsService = {
  getDraftPicks: async (leagueId: number, year: number = 2024): Promise<DraftPicksResponse> => {
    return apiClient.get<DraftPicksResponse>(`/leagues/${leagueId}/transactions/draft-picks?year=${year}`);
  },

  getAllTransactions: async (leagueId: number): Promise<TransactionsResponse> => {
    return apiClient.get<TransactionsResponse>(`/leagues/${leagueId}/transactions`);
  },
};