import { apiClient } from './apiClient';

export interface AdminBacklogResponse {
  season: string;
  total_leagues: number;
  never_fetched_count: number;
  oldest_transactions_fetched_at: string | null;
}

export interface AdminSegmentRow {
  scoring: string;
  superflex: boolean;
  league_size: string;
  leagues: number;
}

export interface AdminSegmentsResponse {
  total_leagues: number;
  segments: AdminSegmentRow[];
}

export const adminService = {
  getBacklog: async (): Promise<AdminBacklogResponse> => {
    return apiClient.get<AdminBacklogResponse>('/admin/backlog');
  },

  getSegments: async (): Promise<AdminSegmentsResponse> => {
    return apiClient.get<AdminSegmentsResponse>('/admin/segments');
  },
};
