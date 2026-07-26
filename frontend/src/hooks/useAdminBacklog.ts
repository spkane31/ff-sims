import { adminService, AdminBacklogResponse } from "../services/adminService";
import { useAsyncQuery } from "./useAsyncQuery";

export function useAdminBacklog() {
  const {
    data: backlog,
    isLoading,
    error,
  } = useAsyncQuery<AdminBacklogResponse | null>(adminService.getBacklog, {
    initialData: null,
    errorMessage: "Failed to fetch admin backlog",
  });

  return { backlog, isLoading, error };
}
