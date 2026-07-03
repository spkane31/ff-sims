import { apiClient } from './apiClient';

export interface AdminBacklogResponse {
  season: string;
  total_leagues: number;
  never_fetched_count: number;
  oldest_transactions_fetched_at: string | null;
}

export const adminService = {
  getBacklog: async (): Promise<AdminBacklogResponse> => {
    return apiClient.get<AdminBacklogResponse>('/admin/backlog');
  },
};
