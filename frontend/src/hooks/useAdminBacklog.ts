import { useState, useEffect, useCallback } from "react";
import { adminService, AdminBacklogResponse } from "../services/adminService";

export function useAdminBacklog() {
  const [backlog, setBacklog] = useState<AdminBacklogResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchBacklog = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await adminService.getBacklog();
      setBacklog(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to fetch admin backlog"));
      setBacklog(null);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchBacklog();
  }, [fetchBacklog]);

  return { backlog, isLoading, error };
}
