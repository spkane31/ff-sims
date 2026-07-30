import { apiClient } from './apiClient';

export interface AdminBacklogBucketRow {
  label: string;
  leagues: number;
}

export interface AdminBacklogResponse {
  season: string;
  total_leagues: number;
  never_fetched_count: number;
  oldest_transactions_fetched_at: string | null;
  buckets: AdminBacklogBucketRow[];
}

export interface AdminTransactionFetchAgeHistorySnapshot {
  snapshot_at: string;
  never_fetched: number;
  fetched_within_four_hours: number;
  fetched_four_to_eight_hours: number;
  fetched_eight_to_twelve_hours: number;
  fetched_twelve_to_sixteen_hours: number;
  fetched_sixteen_to_twenty_hours: number;
  fetched_twenty_to_twenty_four_hours: number;
  fetched_twenty_four_or_more_hours: number;
}

export interface AdminTransactionFetchAgeHistoryResponse {
  season: string;
  snapshots: AdminTransactionFetchAgeHistorySnapshot[];
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

export interface AdminDiscoveryCounts {
  total: number;
  expanded: number;
  pending: number;
  skipped: number;
}

export interface AdminDiscoveryLeagueSeasonRow extends AdminDiscoveryCounts {
  season: string;
}

export interface AdminDiscoveryFrontierResponse {
  users: AdminDiscoveryCounts;
  leagues_by_season: AdminDiscoveryLeagueSeasonRow[];
}

export const adminService = {
  getBacklog: async (): Promise<AdminBacklogResponse> => {
    return apiClient.get<AdminBacklogResponse>('/admin/backlog');
  },

  getTransactionFetchAgeHistory: async (): Promise<AdminTransactionFetchAgeHistoryResponse> => {
    return apiClient.get<AdminTransactionFetchAgeHistoryResponse>('/admin/transaction-fetch-age-history');
  },

  getSegments: async (): Promise<AdminSegmentsResponse> => {
    return apiClient.get<AdminSegmentsResponse>('/admin/segments');
  },

  getDatabaseSize: async (): Promise<AdminDatabaseSizeResponse> => {
    return apiClient.get<AdminDatabaseSizeResponse>('/admin/database-size');
  },

  getDiscoveryFrontier: async (): Promise<AdminDiscoveryFrontierResponse> => {
    return apiClient.get<AdminDiscoveryFrontierResponse>('/admin/discovery-frontier');
  },
};
