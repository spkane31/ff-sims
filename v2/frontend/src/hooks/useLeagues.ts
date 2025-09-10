import { useState, useEffect, useCallback } from "react";
import { leaguesService } from "../services/leaguesService";

interface UseLeagueYearsReturn {
  years: number[];
  isLoading: boolean;
  error: Error | null;
}

/**
 * Hook for fetching league years data
 */
export function useLeagueYears(
  leagueId: number = 345674
): UseLeagueYearsReturn {
  const [years, setYears] = useState<number[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchLeagueYears = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await leaguesService.getLeagueYears(leagueId);
      setYears(data.years || []);
    } catch (err) {
      setError(
        err instanceof Error
          ? err
          : new Error("An error occurred while fetching league years")
      );
      setYears([]);
    } finally {
      setIsLoading(false);
    }
  }, [leagueId]);

  useEffect(() => {
    fetchLeagueYears();
  }, [fetchLeagueYears]);

  return { years, isLoading, error };
}
