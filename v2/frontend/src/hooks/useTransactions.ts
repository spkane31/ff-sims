import { useState, useEffect, useCallback } from "react";
import { Transaction, DraftPick, transactionsService } from "../services/transactionsService";

interface UseTransactionsReturn {
  transactions: Transaction[];
  isLoading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

export function useTransactions(leagueId: number): UseTransactionsReturn {
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchTransactions = useCallback(async () => {
    if (!leagueId) return;
    try {
      setIsLoading(true);
      setError(null);
      const data = await transactionsService.getAllTransactions(leagueId);
      setTransactions(data.transactions);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("An error occurred while fetching transactions"));
    } finally {
      setIsLoading(false);
    }
  }, [leagueId]);

  useEffect(() => {
    fetchTransactions();
  }, [fetchTransactions]);

  return { transactions, isLoading, error, refetch: fetchTransactions };
}

interface UseDraftPicksReturn {
  draftPicks: DraftPick[];
  isLoading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

export function useDraftPicks(leagueId: number, year: number = 2024): UseDraftPicksReturn {
  const [draftPicks, setDraftPicks] = useState<DraftPick[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchDraftPicks = useCallback(async () => {
    if (!leagueId) return;
    try {
      setIsLoading(true);
      setError(null);
      const data = await transactionsService.getDraftPicks(leagueId, year);
      setDraftPicks(data.draft_picks || []);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("An error occurred while fetching draft picks"));
      setDraftPicks([]);
    } finally {
      setIsLoading(false);
    }
  }, [leagueId, year]);

  useEffect(() => {
    fetchDraftPicks();
  }, [fetchDraftPicks]);

  return { draftPicks, isLoading, error, refetch: fetchDraftPicks };
}
