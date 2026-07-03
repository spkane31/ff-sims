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
  transactions: number;
}

export interface AdminSegmentsResponse {
  total_leagues: number;
  total_transactions: number;
  segments: AdminSegmentRow[];
}

export interface AdminTableSizeRow {
  table_name: string;
  size_bytes: number;
  row_estimate: number;
}

export interface AdminDatabaseSizeResponse {
  total_bytes: number;
  tables: AdminTableSizeRow[];
}

export const adminService = {
  getBacklog: async (): Promise<AdminBacklogResponse> => {
    return apiClient.get<AdminBacklogResponse>('/admin/backlog');
  },

  getSegments: async (): Promise<AdminSegmentsResponse> => {
    return apiClient.get<AdminSegmentsResponse>('/admin/segments');
  },

  getDatabaseSize: async (): Promise<AdminDatabaseSizeResponse> => {
    return apiClient.get<AdminDatabaseSizeResponse>('/admin/database-size');
  },
};
