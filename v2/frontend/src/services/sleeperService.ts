import { apiClient } from './apiClient';
import { SleeperStats, SleeperTradesResponse, SleeperDraftsResponse } from '../types/models';

export const sleeperService = {
  getStats: (): Promise<SleeperStats> =>
    apiClient.get<SleeperStats>('/sleeper/stats'),

  getTrades: (page = 1, limit = 25): Promise<SleeperTradesResponse> =>
    apiClient.get<SleeperTradesResponse>(`/sleeper/trades?page=${page}&limit=${limit}`),

  getDrafts: (page = 1, limit = 25): Promise<SleeperDraftsResponse> =>
    apiClient.get<SleeperDraftsResponse>(`/sleeper/drafts?page=${page}&limit=${limit}`),
};
