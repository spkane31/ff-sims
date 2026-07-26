import { useCallback } from "react";
import {
  expectedWinsService,
  WeeklyExpectedWins,
  SeasonExpectedWins,
} from "../services/expectedWinsService";
import { useAsyncQuery } from "./useAsyncQuery";

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
export function useWeeklyExpectedWins(
  leagueId: number,
  year: number,
  week?: number
): UseWeeklyExpectedWinsReturn {
  const query = useCallback(
    () =>
      expectedWinsService
        .getWeeklyExpectedWins(leagueId, year, week)
        .then((response) => response.data),
    [leagueId, year, week]
  );
  const {
    data: weeklyData,
    isLoading,
    error,
    refetch,
  } = useAsyncQuery(query, {
    initialData: [],
    enabled: Boolean(leagueId && year),
    errorMessage: "An error occurred while fetching weekly expected wins",
  });

  return { weeklyData, isLoading, error, refetch };
}

/**
 * Hook for fetching season expected wins data
 */
export function useSeasonExpectedWins(
  leagueId: number,
  year: number
): UseSeasonExpectedWinsReturn {
  const query = useCallback(
    () =>
      expectedWinsService
        .getSeasonExpectedWins(leagueId, year)
        .then((response) => response.data),
    [leagueId, year]
  );
  const {
    data: seasonData,
    isLoading,
    error,
    refetch,
  } = useAsyncQuery(query, {
    initialData: [],
    enabled: Boolean(leagueId && year),
    errorMessage: "An error occurred while fetching season expected wins",
  });

  return { seasonData, isLoading, error, refetch };
}

/**
 * Hook for fetching team progression data
 */
export function useTeamProgression(
  leagueId: number,
  teamId: number,
  year: number
): UseTeamProgressionReturn {
  const query = useCallback(
    () =>
      expectedWinsService
        .getTeamProgression(leagueId, teamId, year)
        .then((response) => response.data),
    [leagueId, teamId, year]
  );
  const {
    data: progressionData,
    isLoading,
    error,
    refetch,
  } = useAsyncQuery(query, {
    initialData: [],
    enabled: Boolean(leagueId && teamId && year),
    errorMessage: "An error occurred while fetching team progression",
  });

  return { progressionData, isLoading, error, refetch };
}
