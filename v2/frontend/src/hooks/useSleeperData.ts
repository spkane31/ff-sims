import { useState, useEffect, useCallback } from 'react';
import { SleeperStats, SleeperTrade, SleeperDraft } from '../types/models';
import { sleeperService } from '../services/sleeperService';

export function useSleeperStats() {
  const [stats, setStats] = useState<SleeperStats | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetch = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      setStats(await sleeperService.getStats());
    } catch (err) {
      setError(err instanceof Error ? err : new Error('Failed to fetch Sleeper stats'));
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => { fetch(); }, [fetch]);
  return { stats, isLoading, error, refetch: fetch };
}

interface PaginatedState<T> {
  items: T[];
  total: number;
  totalPages: number;
  isLoading: boolean;
  error: Error | null;
}

export function useSleeperTrades(page: number, limit: number) {
  const [state, setState] = useState<PaginatedState<SleeperTrade>>({
    items: [], total: 0, totalPages: 0, isLoading: true, error: null,
  });

  const fetch = useCallback(async () => {
    setState(s => ({ ...s, isLoading: true, error: null }));
    try {
      const data = await sleeperService.getTrades(page, limit);
      setState({ items: data.trades, total: data.total, totalPages: data.total_pages, isLoading: false, error: null });
    } catch (err) {
      setState(s => ({ ...s, isLoading: false, error: err instanceof Error ? err : new Error('Failed to fetch trades') }));
    }
  }, [page, limit]);

  useEffect(() => { fetch(); }, [fetch]);
  return { ...state, refetch: fetch };
}

export function useSleeperDrafts(page: number, limit: number) {
  const [state, setState] = useState<PaginatedState<SleeperDraft>>({
    items: [], total: 0, totalPages: 0, isLoading: true, error: null,
  });

  const fetch = useCallback(async () => {
    setState(s => ({ ...s, isLoading: true, error: null }));
    try {
      const data = await sleeperService.getDrafts(page, limit);
      setState({ items: data.drafts, total: data.total, totalPages: data.total_pages, isLoading: false, error: null });
    } catch (err) {
      setState(s => ({ ...s, isLoading: false, error: err instanceof Error ? err : new Error('Failed to fetch drafts') }));
    }
  }, [page, limit]);

  useEffect(() => { fetch(); }, [fetch]);
  return { ...state, refetch: fetch };
}
