import { useState, useEffect } from 'react';
import { Player, playerService } from '../services/playerService';

/**
 * Hook for fetching all players
 */
export function usePlayers(year?: string | number) {
  const [players, setPlayers] = useState<Player[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchPlayers = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await playerService.getAllPlayers(year);
      setPlayers(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('An error occurred while fetching players'));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchPlayers();
  }, [year]);

  return { players, isLoading, error, refetch: fetchPlayers };
}

/**
 * Hook for fetching a team's players
 */
export function useTeamPlayers(teamId: number) {
  const [players, setPlayers] = useState<Player[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchTeamPlayers = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await playerService.getTeamPlayers(teamId);
      setPlayers(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('An error occurred while fetching team players'));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    if (teamId) {
      fetchTeamPlayers();
    }
  }, [teamId]);

  return { players, isLoading, error, refetch: fetchTeamPlayers };
}

/**
 * Hook for fetching draft data
 */
export function useDraft(year?: number) {
  const [draftData, setDraftData] = useState<Player[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchDraft = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await playerService.getDraft(year);
      setDraftData(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('An error occurred while fetching draft data'));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchDraft();
  }, [year]);

  return { draftData, isLoading, error, refetch: fetchDraft };
}