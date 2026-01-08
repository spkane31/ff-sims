import { useState, useEffect, useCallback } from "react";
import { Player, playerService } from "../services/playerService";

/**
 * Hook for fetching all players
 */
export function usePlayers(leagueId?: string, year?: string | number) {
  const [players, setPlayers] = useState<Player[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchPlayers = useCallback(async () => {
    if (!leagueId) {
      setIsLoading(false);
      return;
    }

    try {
      setIsLoading(true);
      setError(null);
      const data = await playerService.getAllPlayers(leagueId, year);
      setPlayers(data);
    } catch (err) {
      setError(
        err instanceof Error
          ? err
          : new Error("An error occurred while fetching players")
      );
    } finally {
      setIsLoading(false);
    }
  }, [leagueId, year]);

  useEffect(() => {
    fetchPlayers();
  }, [fetchPlayers]);

  return { players, isLoading, error, refetch: fetchPlayers };
}

/**
 * Hook for fetching a team's players
 */
export function useTeamPlayers(leagueId: string | undefined, teamId: number) {
  const [players, setPlayers] = useState<Player[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchTeamPlayers = useCallback(async () => {
    if (!leagueId || !teamId) {
      setIsLoading(false);
      return;
    }

    try {
      setIsLoading(true);
      setError(null);
      const data = await playerService.getTeamPlayers(leagueId, teamId);
      setPlayers(data);
    } catch (err) {
      setError(
        err instanceof Error
          ? err
          : new Error("An error occurred while fetching team players")
      );
    } finally {
      setIsLoading(false);
    }
  }, [leagueId, teamId]);

  useEffect(() => {
    if (leagueId && teamId) {
      fetchTeamPlayers();
    }
  }, [leagueId, teamId, fetchTeamPlayers]);

  return { players, isLoading, error, refetch: fetchTeamPlayers };
}

/**
 * Hook for fetching draft data
 */
export function useDraft(leagueId?: string, year?: number) {
  const [draftData, setDraftData] = useState<Player[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchDraft = useCallback(async () => {
    if (!leagueId) {
      setIsLoading(false);
      return;
    }

    try {
      setIsLoading(true);
      setError(null);
      const data = await playerService.getDraft(leagueId, year);
      setDraftData(data);
    } catch (err) {
      setError(
        err instanceof Error
          ? err
          : new Error("An error occurred while fetching draft data")
      );
    } finally {
      setIsLoading(false);
    }
  }, [leagueId, year]);

  useEffect(() => {
    fetchDraft();
  }, [fetchDraft]);

  return { draftData, isLoading, error, refetch: fetchDraft };
}
