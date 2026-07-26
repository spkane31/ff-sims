import {
  adminService,
  AdminDiscoveryFrontierResponse,
} from "../services/adminService";
import { useAsyncQuery } from "./useAsyncQuery";

export function useAdminDiscoveryFrontier() {
  const {
    data: frontier,
    isLoading,
    error,
  } = useAsyncQuery<AdminDiscoveryFrontierResponse | null>(
    adminService.getDiscoveryFrontier,
    {
      initialData: null,
      errorMessage: "Failed to fetch admin discovery frontier",
    }
  );

  return { frontier, isLoading, error };
}
