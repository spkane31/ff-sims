import { useState, useEffect, useCallback } from "react";
import { leaguesService, League } from "../services/leaguesService";

export function useLeagues() {
  const [leagues, setLeagues] = useState<League[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchLeagues = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await leaguesService.getLeagues();
      setLeagues(data.leagues || []);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to fetch leagues"));
      setLeagues([]);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchLeagues();
  }, [fetchLeagues]);

  return { leagues, isLoading, error };
}

export function useLeagueYears(leagueId: number) {
  const [years, setYears] = useState<number[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchLeagueYears = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await leaguesService.getLeagueYears(leagueId);
      setYears(data.years || []);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to fetch league years"));
      setYears([]);
    } finally {
      setIsLoading(false);
    }
  }, [leagueId]);

  useEffect(() => {
    fetchLeagueYears();
  }, [fetchLeagueYears]);

  return { years, isLoading, error };
}

export function useLeague(leagueId: number | undefined) {
  const [league, setLeague] = useState<League | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    if (leagueId === undefined) {
      setLeague(null);
      setIsLoading(false);
      setError(null);
      return;
    }
    setIsLoading(true);
    setError(null);
    leaguesService
      .getLeague(leagueId)
      .then(setLeague)
      .catch((err) =>
        setError(err instanceof Error ? err : new Error("Failed to fetch league"))
      )
      .finally(() => setIsLoading(false));
  }, [leagueId]);

  return { league, isLoading, error };
}
