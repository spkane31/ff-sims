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
   * Get all draft picks
   */
  getDraftPicks: async (year: number = 2024, leagueId: number = 345674): Promise<DraftPicksResponse> => {
    return apiClient.get<DraftPicksResponse>(`/transactions/draft-picks?year=${year}&league_id=${leagueId}`);
  },

  /**
   * Get all transactions
   */
  getAllTransactions: async (): Promise<TransactionsResponse> => {
    return apiClient.get<TransactionsResponse>('/transactions');
  },

  /**
   * Get a single transaction by ID
   */
  getTransactionById: async (transactionId: number): Promise<TransactionsResponse> => {
    return apiClient.get<TransactionsResponse>(`/transactions/${transactionId}`);
  },

};