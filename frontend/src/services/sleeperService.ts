import { apiClient } from './apiClient';
import {
  SleeperStatsResponse,
  SleeperTradesResponse,
  SleeperADPResponse,
  SleeperTransactionsResponse,
  SleeperLeagueFilters,
  SleeperADPFilters,
} from '../types/models';

function buildQuery(params: Record<string, string | number | undefined>): string {
  const parts = Object.entries(params)
    .filter(([, v]) => v !== undefined && v !== '')
    .map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`);
  return parts.length > 0 ? `?${parts.join('&')}` : '';
}

export const sleeperService = {
  // limit/skip page through sleeper_lifetime_counts' hourly history, most
  // recent first; omit both for just the latest point (limit defaults to
  // 100 server-side, so pass limit=1 when only the latest is needed).
  getStats: (limit?: number, skip?: number): Promise<SleeperStatsResponse> =>
    apiClient.get<SleeperStatsResponse>(`/sleeper/stats${buildQuery({ limit, skip })}`),

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

  getADP: (
    page = 1,
    limit = 25,
    filters: SleeperADPFilters = {}
  ): Promise<SleeperADPResponse> =>
    apiClient.get<SleeperADPResponse>(
      `/sleeper/adp${buildQuery({ page, limit, ...filters })}`
    ),
};
