import { apiClient } from './apiClient';
import {
  SleeperStats,
  SleeperTradesResponse,
  SleeperDraftsResponse,
  SleeperTransactionsResponse,
  SleeperLeagueFilters,
} from '../types/models';

function buildQuery(params: Record<string, string | number | undefined>): string {
  const parts = Object.entries(params)
    .filter(([, v]) => v !== undefined && v !== '')
    .map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`);
  return parts.length > 0 ? `?${parts.join('&')}` : '';
}

export const sleeperService = {
  getStats: (): Promise<SleeperStats> =>
    apiClient.get<SleeperStats>('/sleeper/stats'),

  getTrades: (
    page = 1,
    limit = 25,
    filters: SleeperLeagueFilters = {}
  ): Promise<SleeperTradesResponse> =>
    apiClient.get<SleeperTradesResponse>(
      `/sleeper/trades${buildQuery({ page, limit, ...filters })}`
    ),

  getTransactions: (
    page = 1,
    limit = 25,
    txType = '',
    filters: SleeperLeagueFilters = {}
  ): Promise<SleeperTransactionsResponse> =>
    apiClient.get<SleeperTransactionsResponse>(
      `/sleeper/transactions${buildQuery({ page, limit, type: txType || undefined, ...filters })}`
    ),

  getDrafts: (page = 1, limit = 25): Promise<SleeperDraftsResponse> =>
    apiClient.get<SleeperDraftsResponse>(`/sleeper/drafts?page=${page}&limit=${limit}`),
};
