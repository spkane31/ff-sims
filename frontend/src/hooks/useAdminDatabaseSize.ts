import { useState, useEffect, useCallback } from "react";
import { adminService, AdminDatabaseSizeResponse } from "../services/adminService";

export function useAdminDatabaseSize() {
  const [databaseSize, setDatabaseSize] = useState<AdminDatabaseSizeResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchDatabaseSize = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await adminService.getDatabaseSize();
      setDatabaseSize(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to fetch admin database size"));
      setDatabaseSize(null);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchDatabaseSize();
  }, [fetchDatabaseSize]);

  return { databaseSize, isLoading, error };
}
