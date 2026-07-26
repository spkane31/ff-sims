import { adminService, AdminDatabaseSizeResponse } from "../services/adminService";
import { useAsyncQuery } from "./useAsyncQuery";

export function useAdminDatabaseSize() {
  const { data: databaseSize, isLoading, error } = useAsyncQuery<AdminDatabaseSizeResponse | null>(
    adminService.getDatabaseSize,
    { initialData: null, errorMessage: "Failed to fetch admin database size" }
  );

  return { databaseSize, isLoading, error };
}
