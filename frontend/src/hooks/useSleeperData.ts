import { useState, useEffect, useCallback } from 'react';
import {
  SleeperStats,
  SleeperTrade,
  SleeperADPItem,
  SleeperTransaction,
  SleeperLeagueFilters,
  SleeperADPFilters,
} from '../types/models';
import { sleeperService } from '../services/sleeperService';

// useSleeperStats returns just the latest snapshot (limit=1), for the home
// page's current totals. For a growth-over-time series (e.g. an /admin
// chart), use useSleeperStatsHistory instead.
export function useSleeperStats() {
  const [stats, setStats] = useState<SleeperStats | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetch = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await sleeperService.getStats(1);
      setStats(data.snapshots[0] ?? null);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('Failed to fetch Sleeper stats'));
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => { fetch(); }, [fetch]);
  return { stats, isLoading, error, refetch: fetch };
}

// useSleeperStatsHistory returns the hourly snapshot series (most recent
// first) backing an eventual /admin growth-over-time chart.
export function useSleeperStatsHistory(limit = 168, skip = 0) {
  const [snapshots, setSnapshots] = useState<SleeperStats[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetch = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await sleeperService.getStats(limit, skip);
      setSnapshots(data.snapshots);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('Failed to fetch Sleeper stats history'));
    } finally {
      setIsLoading(false);
    }
  }, [limit, skip]);

  useEffect(() => { fetch(); }, [fetch]);
  return { snapshots, isLoading, error, refetch: fetch };
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

interface ADPState {
  items: SleeperADPItem[];
  season: string;
  availableSeasons: string[];
  total: number;
  totalPages: number;
  isLoading: boolean;
  error: Error | null;
}

export function useSleeperADP(page: number, limit: number, filters: SleeperADPFilters = {}) {
  const [state, setState] = useState<ADPState>({
    items: [], season: '', availableSeasons: [], total: 0, totalPages: 0, isLoading: true, error: null,
  });

  const filtersKey = JSON.stringify(filters);

  const fetch = useCallback(async () => {
    setState(s => ({ ...s, isLoading: true, error: null }));
    try {
      const data = await sleeperService.getADP(page, limit, filters);
      setState({
        items: data.players,
        season: data.season,
        availableSeasons: data.available_seasons,
        total: data.total,
        totalPages: data.total_pages,
        isLoading: false,
        error: null,
      });
    } catch (err) {
      setState(s => ({ ...s, isLoading: false, error: err instanceof Error ? err : new Error('Failed to fetch ADP') }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, limit, filtersKey]);

  useEffect(() => { fetch(); }, [fetch]);
  return { ...state, refetch: fetch };
}

interface ADPAllState {
  items: SleeperADPItem[];
  season: string;
  availableSeasons: string[];
  isLoading: boolean;
  error: Error | null;
}

export function useSleeperADPAll(filters: SleeperADPFilters = {}) {
  const [state, setState] = useState<ADPAllState>({
    items: [], season: '', availableSeasons: [], isLoading: true, error: null,
  });

  const filtersKey = JSON.stringify(filters);

  const fetch = useCallback(async () => {
    setState(s => ({ ...s, isLoading: true, error: null }));
    try {
      const first = await sleeperService.getADP(1, 100, filters);
      let items = first.players;
      for (let page = 2; page <= first.total_pages; page++) {
        const next = await sleeperService.getADP(page, 100, filters);
        items = items.concat(next.players);
      }
      setState({
        items,
        season: first.season,
        availableSeasons: first.available_seasons,
        isLoading: false,
        error: null,
      });
    } catch (err) {
      setState(s => ({ ...s, isLoading: false, error: err instanceof Error ? err : new Error('Failed to fetch ADP') }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filtersKey]);

  useEffect(() => { fetch(); }, [fetch]);
  return { ...state, refetch: fetch };
}
