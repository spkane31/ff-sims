import { useState, useEffect } from 'react';
import { Game, scheduleService } from '../services/scheduleService';

/**
 * Hook for fetching the full schedule
 */
export function useSchedule() {
  const [schedule, setSchedule] = useState<Game[][]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchSchedule = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await scheduleService.getFullSchedule();
      setSchedule(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('An error occurred while fetching schedule'));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchSchedule();
  }, []);

  return { schedule, isLoading, error, refetch: fetchSchedule };
}

/**
 * Hook for fetching a team's schedule
 */
export function useTeamSchedule(teamId: number) {
  const [teamSchedule, setTeamSchedule] = useState<Game[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchTeamSchedule = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await scheduleService.getTeamSchedule(teamId);
      setTeamSchedule(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('An error occurred while fetching team schedule'));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    if (teamId) {
      fetchTeamSchedule();
    }
  }, [teamId]);

  return { teamSchedule, isLoading, error, refetch: fetchTeamSchedule };
}