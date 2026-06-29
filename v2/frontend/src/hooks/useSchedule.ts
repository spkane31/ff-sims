import { useState, useEffect, useCallback } from "react";
import { GetScheduleResponse, scheduleService } from "../services/scheduleService";

export function useSchedule(leagueId: number, options?: { gameType?: string }) {
  const [schedule, setSchedule] = useState<GetScheduleResponse>({ data: { matchups: [] } });
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchSchedule = useCallback(async () => {
    if (!leagueId) return;
    try {
      setIsLoading(true);
      setError(null);
      const data = await scheduleService.getFullSchedule(leagueId, options?.gameType);
      setSchedule(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("An error occurred while fetching schedule"));
    } finally {
      setIsLoading(false);
    }
  }, [leagueId, options?.gameType]);

  useEffect(() => {
    fetchSchedule();
  }, [fetchSchedule]);

  return { schedule, isLoading, error, refetch: fetchSchedule };
}
