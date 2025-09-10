import { useState, useEffect, useCallback } from "react";
import { Team, teamsService } from "../services/teamsService";

interface UseTeamsReturn {
  teams: Team[];
  isLoading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

/**
 * Hook for fetching teams data
 */
export function useTeams(): UseTeamsReturn {
  const [teams, setTeams] = useState<Team[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchTeams = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await teamsService.getAllTeams();
      setTeams(data.teams);
    } catch (err) {
      setError(
        err instanceof Error
          ? err
          : new Error("An error occurred while fetching teams")
      );
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchTeams();
  }, [fetchTeams]);

  return { teams, isLoading, error, refetch: fetchTeams };
}

/**
 * Hook for fetching a single team by ID
 */
export function useTeam(teamId: number) {
  const [team, setTeam] = useState<Team | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchTeam = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await teamsService.getTeamById(teamId);
      setTeam(data);
    } catch (err) {
      setError(
        err instanceof Error
          ? err
          : new Error("An error occurred while fetching team")
      );
    } finally {
      setIsLoading(false);
    }
  }, [teamId]);

  useEffect(() => {
    if (teamId) {
      fetchTeam();
    }
  }, [teamId, fetchTeam]);

  return { team, isLoading, error, refetch: fetchTeam };
}
