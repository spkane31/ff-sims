import { useState, useEffect, useCallback } from "react";
import {
  Transaction,
  DraftPick,
  transactionsService,
} from "../services/transactionsService";

interface UseTransactionsReturn {
  transactions: Transaction[];
  isLoading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

/**
 * Hook for fetching transactions data
 */
export function useTransactions(): UseTransactionsReturn {
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchTransactions = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await transactionsService.getAllTransactions();
      setTransactions(data.transactions);
    } catch (err) {
      setError(
        err instanceof Error
          ? err
          : new Error("An error occurred while fetching transactions")
      );
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchTransactions();
  }, [fetchTransactions]);

  return { transactions, isLoading, error, refetch: fetchTransactions };
}

/**
 * Hook for fetching a single transaction by ID
 */
export function useTransaction(transactionId: number) {
  const [transaction, setTransaction] = useState<Transaction | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchTransaction = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await transactionsService.getTransactionById(transactionId);
      setTransaction(data.transactions[0] || null); // Assuming the API returns an array
    } catch (err) {
      setError(
        err instanceof Error
          ? err
          : new Error("An error occurred while fetching transaction")
      );
    } finally {
      setIsLoading(false);
    }
  }, [transactionId]);

  useEffect(() => {
    if (transactionId) {
      fetchTransaction();
    }
  }, [transactionId, fetchTransaction]);

  return { transaction, isLoading, error, refetch: fetchTransaction };
}

interface UseDraftPicksReturn {
  draftPicks: DraftPick[];
  isLoading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

/**
 * Hook for fetching draft picks data
 */
export function useDraftPicks(year: number = 2024): UseDraftPicksReturn {
  const [draftPicks, setDraftPicks] = useState<DraftPick[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchDraftPicks = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await transactionsService.getDraftPicks(year);
      setDraftPicks(data.draft_picks || []);
    } catch (err) {
      setError(
        err instanceof Error
          ? err
          : new Error("An error occurred while fetching draft picks")
      );
      setDraftPicks([]); // Set empty array on error
    } finally {
      setIsLoading(false);
    }
  }, [year]);

  useEffect(() => {
    fetchDraftPicks();
  }, [fetchDraftPicks]);

  return { draftPicks, isLoading, error, refetch: fetchDraftPicks };
}
