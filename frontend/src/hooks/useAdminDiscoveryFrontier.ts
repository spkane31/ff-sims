import { useState, useEffect, useCallback } from "react";
import { adminService, AdminDiscoveryFrontierResponse } from "../services/adminService";

export function useAdminDiscoveryFrontier() {
  const [frontier, setFrontier] = useState<AdminDiscoveryFrontierResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchFrontier = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await adminService.getDiscoveryFrontier();
      setFrontier(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to fetch admin discovery frontier"));
      setFrontier(null);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchFrontier();
  }, [fetchFrontier]);

  return { frontier, isLoading, error };
}
