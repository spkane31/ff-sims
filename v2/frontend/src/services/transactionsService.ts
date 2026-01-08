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
  /**
   * Get all draft picks for a specific league
   */
  getDraftPicks: async (leagueId: string, year: number = 2024): Promise<DraftPicksResponse> => {
    return apiClient.get<DraftPicksResponse>(`/league/${leagueId}/transactions/draft-picks?year=${year}`);
  },

  /**
   * Get all transactions for a specific league
   */
  getAllTransactions: async (leagueId: string): Promise<TransactionsResponse> => {
    return apiClient.get<TransactionsResponse>(`/league/${leagueId}/transactions`);
  },

  /**
   * Get a single transaction by ID for a specific league
   */
  getTransactionById: async (leagueId: string, transactionId: number): Promise<TransactionsResponse> => {
    return apiClient.get<TransactionsResponse>(`/league/${leagueId}/transactions/${transactionId}`);
  },

};