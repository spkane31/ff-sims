import { adminService, AdminSegmentsResponse } from "../services/adminService";
import { useAsyncQuery } from "./useAsyncQuery";

export function useAdminSegments() {
  const {
    data: segments,
    isLoading,
    error,
  } = useAsyncQuery<AdminSegmentsResponse | null>(adminService.getSegments, {
    initialData: null,
    errorMessage: "Failed to fetch admin segments",
  });

  return { segments, isLoading, error };
}
