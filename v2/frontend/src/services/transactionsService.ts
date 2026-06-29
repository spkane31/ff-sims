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

export interface DraftPicksPagedResponse {
  draft_picks: DraftPick[];
  total: number;
  page: number;
  limit: number;
  total_pages: number;
}

export interface Transaction {
  id: number;
  date: string;
  type: 'draft' | 'trade' | 'waiver';
  teams: string[];
  players: {
    id: string;
    name: string;
    position: string;
    team: string;
    points: number;
  }[];
}

export interface TransactionsPagedResponse {
  transactions: Transaction[];
  total: number;
  page: number;
  limit: number;
  total_pages: number;
}

/**
 * Transactions API service
 */
export const transactionsService = {
  getDraftPicks: async (
    leagueId: number,
    year: number = 2024,
    page = 1,
    limit = 25
  ): Promise<DraftPicksPagedResponse> => {
    return apiClient.get<DraftPicksPagedResponse>(
      `/leagues/${leagueId}/transactions/draft-picks?year=${year}&page=${page}&limit=${limit}`
    );
  },

  getAllTransactions: async (
    leagueId: number,
    page = 1,
    limit = 25,
    year?: number
  ): Promise<TransactionsPagedResponse> => {
    const params = new URLSearchParams({ page: String(page), limit: String(limit) });
    if (year) params.set('year', String(year));
    return apiClient.get<TransactionsPagedResponse>(`/leagues/${leagueId}/transactions?${params}`);
  },
};
