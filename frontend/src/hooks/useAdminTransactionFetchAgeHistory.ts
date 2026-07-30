import {
  adminService,
  AdminTransactionFetchAgeHistoryResponse,
} from "../services/adminService";
import { useAsyncQuery } from "./useAsyncQuery";

export function useAdminTransactionFetchAgeHistory() {
  const {
    data: history,
    isLoading,
    error,
  } = useAsyncQuery<AdminTransactionFetchAgeHistoryResponse | null>(
    adminService.getTransactionFetchAgeHistory,
    {
      initialData: null,
      errorMessage: "Failed to fetch transaction fetch-age history",
    }
  );

  return { history, isLoading, error };
}
