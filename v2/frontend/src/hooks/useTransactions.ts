import { useState, useEffect } from 'react';
import { Transaction, transactionsService } from '../services/transactionsService';

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

  const fetchTransactions = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await transactionsService.getAllTransactions();
      setTransactions(data.transactions);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('An error occurred while fetching transactions'));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchTransactions();
  }, []);

  return { transactions, isLoading, error, refetch: fetchTransactions };
}

/**
 * Hook for fetching a single transaction by ID
 */
export function useTransaction(transactionId: number) {
  const [transaction, setTransaction] = useState<Transaction | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchTransaction = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await transactionsService.getTransactionById(transactionId);
      setTransaction(data.transactions[0] || null); // Assuming the API returns an array
    } catch (err) {
      setError(err instanceof Error ? err : new Error('An error occurred while fetching transaction'));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    if (transactionId) {
      fetchTransaction();
    }
  }, [transactionId]);

  return { transaction, isLoading, error, refetch: fetchTransaction };
}