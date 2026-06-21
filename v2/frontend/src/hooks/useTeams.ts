import { useState, useEffect, useCallback } from "react";
import { Team, teamsService } from "../services/teamsService";

interface UseTeamsReturn {
  teams: Team[];
  isLoading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

export function useTeams(leagueId: number): UseTeamsReturn {
  const [teams, setTeams] = useState<Team[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchTeams = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await teamsService.getAllTeams(leagueId);
      setTeams(data.teams);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("An error occurred while fetching teams"));
    } finally {
      setIsLoading(false);
    }
  }, [leagueId]);

  useEffect(() => {
    fetchTeams();
  }, [fetchTeams]);

  return { teams, isLoading, error, refetch: fetchTeams };
}

export function useTeam(leagueId: number, teamId: number) {
  const [team, setTeam] = useState<Team | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchTeam = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await teamsService.getTeamById(leagueId, teamId);
      setTeam(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("An error occurred while fetching team"));
    } finally {
      setIsLoading(false);
    }
  }, [leagueId, teamId]);

  useEffect(() => {
    if (teamId) {
      fetchTeam();
    }
  }, [teamId, fetchTeam]);

  return { team, isLoading, error, refetch: fetchTeam };
}
