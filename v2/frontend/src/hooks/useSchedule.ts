import { useState, useEffect, useCallback } from "react";
import {
  Game,
  GetScheduleResponse,
  scheduleService,
} from "../services/scheduleService";

/**
 * Hook for fetching the full schedule
 */
export function useSchedule(leagueId?: string, options?: { gameType?: string }) {
  const [schedule, setSchedule] = useState<GetScheduleResponse>({
    data: { matchups: [] },
  });
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchSchedule = useCallback(async () => {
    if (!leagueId) {
      setIsLoading(false);
      return;
    }

    try {
      setIsLoading(true);
      setError(null);
      const data = await scheduleService.getFullSchedule(leagueId, options?.gameType);
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
  }, [leagueId, options?.gameType]);

  useEffect(() => {
    fetchSchedule();
  }, [fetchSchedule]);

  return { schedule, isLoading, error, refetch: fetchSchedule };
}

/**
 * Hook for fetching a team's schedule
 */
export function useTeamSchedule(leagueId: string | undefined, teamId: number) {
  const [teamSchedule, setTeamSchedule] = useState<Game[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchTeamSchedule = useCallback(async () => {
    if (!leagueId || !teamId) {
      setIsLoading(false);
      return;
    }

    try {
      setIsLoading(true);
      setError(null);
      const data = await scheduleService.getTeamSchedule(leagueId, teamId);
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
  }, [leagueId, teamId]);

  useEffect(() => {
    if (leagueId && teamId) {
      fetchTeamSchedule();
    }
  }, [leagueId, teamId, fetchTeamSchedule]);

  return { teamSchedule, isLoading, error, refetch: fetchTeamSchedule };
}
