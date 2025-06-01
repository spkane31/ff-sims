import { apiClient } from './apiClient';


export interface Transaction {
  id: number;
  date: string;
  type: 'draft' | 'trade' | 'waiver';
  description: string;
  teams: string[];
  players: {
    id: string;
    name: string;
    position: string;
    team: string;
    points: number;
  }[];
}

export interface TransactionsResponse {
  transactions: Transaction[];
}

/**
 * Transactions API service
 */
export const transactionsService = {
  /**
   * Get all transactions
   */
  getAllTransactions: async (): Promise<TransactionsResponse> => {
    return apiClient.get<TransactionsResponse>('/transactions');
  },

  /**
   * Get a single transaction by ID
   */
  getTransactionById: async (transactionId: number): Promise<TransactionsResponse> => {
    return apiClient.get<TransactionsResponse>(`/transactions/${transactionId}`);
  },

};