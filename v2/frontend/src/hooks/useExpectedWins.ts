import { useState, useEffect } from 'react';
import { 
  expectedWinsService, 
  WeeklyExpectedWins, 
  SeasonExpectedWins 
} from '../services/expectedWinsService';

interface UseWeeklyExpectedWinsReturn {
  weeklyData: WeeklyExpectedWins[];
  isLoading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

interface UseSeasonExpectedWinsReturn {
  seasonData: SeasonExpectedWins[];
  isLoading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

interface UseTeamProgressionReturn {
  progressionData: WeeklyExpectedWins[];
  isLoading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

/**
 * Hook for fetching weekly expected wins data
 */
export function useWeeklyExpectedWins(leagueId: number, year: number, week?: number): UseWeeklyExpectedWinsReturn {
  const [weeklyData, setWeeklyData] = useState<WeeklyExpectedWins[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchWeeklyData = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const response = await expectedWinsService.getWeeklyExpectedWins(leagueId, year, week);
      setWeeklyData(response.data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('An error occurred while fetching weekly expected wins'));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    if (leagueId && year) {
      fetchWeeklyData();
    }
  }, [leagueId, year, week]);

  return { weeklyData, isLoading, error, refetch: fetchWeeklyData };
}

/**
 * Hook for fetching season expected wins data
 */
export function useSeasonExpectedWins(leagueId: number, year: number): UseSeasonExpectedWinsReturn {
  const [seasonData, setSeasonData] = useState<SeasonExpectedWins[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchSeasonData = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const response = await expectedWinsService.getSeasonExpectedWins(leagueId, year);
      setSeasonData(response.data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('An error occurred while fetching season expected wins'));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    if (leagueId && year) {
      fetchSeasonData();
    }
  }, [leagueId, year]);

  return { seasonData, isLoading, error, refetch: fetchSeasonData };
}

/**
 * Hook for fetching team progression data
 */
export function useTeamProgression(teamId: number, year: number): UseTeamProgressionReturn {
  const [progressionData, setProgressionData] = useState<WeeklyExpectedWins[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchProgressionData = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const response = await expectedWinsService.getTeamProgression(teamId, year);
      setProgressionData(response.data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('An error occurred while fetching team progression'));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    if (teamId && year) {
      fetchProgressionData();
    }
  }, [teamId, year]);

  return { progressionData, isLoading, error, refetch: fetchProgressionData };
}