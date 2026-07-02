import { useState, useEffect, useCallback } from 'react';
import {
  SleeperStats,
  SleeperTrade,
  SleeperDraft,
  SleeperTransaction,
  SleeperLeagueFilters,
} from '../types/models';
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

export function useSleeperTrades(page: number, limit: number, filters: SleeperLeagueFilters = {}) {
  const [state, setState] = useState<PaginatedState<SleeperTrade>>({
    items: [], total: 0, totalPages: 0, isLoading: true, error: null,
  });

  const filtersKey = JSON.stringify(filters);

  const fetch = useCallback(async () => {
    setState(s => ({ ...s, isLoading: true, error: null }));
    try {
      const data = await sleeperService.getTrades(page, limit, filters);
      setState({ items: data.trades, total: data.total, totalPages: data.total_pages, isLoading: false, error: null });
    } catch (err) {
      setState(s => ({ ...s, isLoading: false, error: err instanceof Error ? err : new Error('Failed to fetch trades') }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, limit, filtersKey]);

  useEffect(() => { fetch(); }, [fetch]);
  return { ...state, refetch: fetch };
}

export function useSleeperTransactions(
  page: number,
  limit: number,
  txType: string,
  filters: SleeperLeagueFilters = {}
) {
  const [state, setState] = useState<PaginatedState<SleeperTransaction>>({
    items: [], total: 0, totalPages: 0, isLoading: true, error: null,
  });

  const filtersKey = JSON.stringify(filters);

  const fetch = useCallback(async () => {
    setState(s => ({ ...s, isLoading: true, error: null }));
    try {
      const data = await sleeperService.getTransactions(page, limit, txType, filters);
      setState({ items: data.transactions, total: data.total, totalPages: data.total_pages, isLoading: false, error: null });
    } catch (err) {
      setState(s => ({ ...s, isLoading: false, error: err instanceof Error ? err : new Error('Failed to fetch transactions') }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, limit, txType, filtersKey]);

  useEffect(() => { fetch(); }, [fetch]);
  return { ...state, refetch: fetch };
}

export function useSleeperDrafts(page: number, limit: number, filters: SleeperLeagueFilters = {}) {
  const [state, setState] = useState<PaginatedState<SleeperDraft>>({
    items: [], total: 0, totalPages: 0, isLoading: true, error: null,
  });

  const filtersKey = JSON.stringify(filters);

  const fetch = useCallback(async () => {
    setState(s => ({ ...s, isLoading: true, error: null }));
    try {
      const data = await sleeperService.getDrafts(page, limit, filters);
      setState({ items: data.drafts, total: data.total, totalPages: data.total_pages, isLoading: false, error: null });
    } catch (err) {
      setState(s => ({ ...s, isLoading: false, error: err instanceof Error ? err : new Error('Failed to fetch drafts') }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, limit, filtersKey]);

  useEffect(() => { fetch(); }, [fetch]);
  return { ...state, refetch: fetch };
}
