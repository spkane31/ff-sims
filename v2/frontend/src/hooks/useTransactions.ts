import { useState, useEffect, useCallback } from "react";
import { Transaction, DraftPick, transactionsService } from "../services/transactionsService";

interface UseTransactionsReturn {
  transactions: Transaction[];
  total: number;
  totalPages: number;
  isLoading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

export function useTransactions(
  leagueId: number,
  page = 1,
  limit = 25,
  year?: number
): UseTransactionsReturn {
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(0);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchTransactions = useCallback(async () => {
    if (!leagueId) return;
    try {
      setIsLoading(true);
      setError(null);
      const data = await transactionsService.getAllTransactions(leagueId, page, limit, year);
      setTransactions(data.transactions);
      setTotal(data.total);
      setTotalPages(data.total_pages);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("An error occurred while fetching transactions"));
    } finally {
      setIsLoading(false);
    }
  }, [leagueId, page, limit, year]);

  useEffect(() => {
    fetchTransactions();
  }, [fetchTransactions]);

  return { transactions, total, totalPages, isLoading, error, refetch: fetchTransactions };
}

interface UseDraftPicksReturn {
  draftPicks: DraftPick[];
  total: number;
  totalPages: number;
  isLoading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

export function useDraftPicks(
  leagueId: number,
  year: number = 2024,
  page = 1,
  limit = 25
): UseDraftPicksReturn {
  const [draftPicks, setDraftPicks] = useState<DraftPick[]>([]);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(0);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchDraftPicks = useCallback(async () => {
    if (!leagueId) return;
    try {
      setIsLoading(true);
      setError(null);
      const data = await transactionsService.getDraftPicks(leagueId, year, page, limit);
      setDraftPicks(data.draft_picks || []);
      setTotal(data.total);
      setTotalPages(data.total_pages);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("An error occurred while fetching draft picks"));
      setDraftPicks([]);
    } finally {
      setIsLoading(false);
    }
  }, [leagueId, year, page, limit]);

  useEffect(() => {
    fetchDraftPicks();
  }, [fetchDraftPicks]);

  return { draftPicks, total, totalPages, isLoading, error, refetch: fetchDraftPicks };
}
