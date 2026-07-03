import { useState, useEffect, useCallback } from "react";
import { adminService, AdminSegmentsResponse } from "../services/adminService";

export function useAdminSegments() {
  const [segments, setSegments] = useState<AdminSegmentsResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchSegments = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await adminService.getSegments();
      setSegments(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to fetch admin segments"));
      setSegments(null);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchSegments();
  }, [fetchSegments]);

  return { segments, isLoading, error };
}
