import { useState, useEffect, useCallback } from "react";
import {
  Game,
  GetScheduleResponse,
  scheduleService,
} from "../services/scheduleService";

/**
 * Hook for fetching the full schedule
 */
export function useSchedule(options?: { gameType?: string }) {
  const [schedule, setSchedule] = useState<GetScheduleResponse>({
    data: { matchups: [] },
  });
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchSchedule = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await scheduleService.getFullSchedule(options?.gameType);
      setSchedule(data);
    } catch (err) {
      setError(
        err instanceof Error
          ? err
          : new Error("An error occurred while fetching schedule")
      );
    } finally {
      setIsLoading(false);
    }
  }, [options?.gameType]);

  useEffect(() => {
    fetchSchedule();
  }, [fetchSchedule]);

  return { schedule, isLoading, error, refetch: fetchSchedule };
}

/**
 * Hook for fetching a team's schedule
 */
export function useTeamSchedule(teamId: number) {
  const [teamSchedule, setTeamSchedule] = useState<Game[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchTeamSchedule = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await scheduleService.getTeamSchedule(teamId);
      setTeamSchedule(data);
    } catch (err) {
      setError(
        err instanceof Error
          ? err
          : new Error("An error occurred while fetching team schedule")
      );
    } finally {
      setIsLoading(false);
    }
  }, [teamId]);

  useEffect(() => {
    if (teamId) {
      fetchTeamSchedule();
    }
  }, [teamId, fetchTeamSchedule]);

  return { teamSchedule, isLoading, error, refetch: fetchTeamSchedule };
}
