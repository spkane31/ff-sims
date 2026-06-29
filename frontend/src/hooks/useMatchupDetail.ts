import { useState, useEffect, useCallback } from "react";
import { GetMatchupDetailResponse, scheduleService } from "../services/scheduleService";

export function useMatchupDetail(leagueId: number, matchupId: string) {
  const [matchup, setMatchup] = useState<GetMatchupDetailResponse | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchMatchupDetail = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await scheduleService.getMatchupById(leagueId, matchupId);
      setMatchup(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("An error occurred while fetching matchup details"));
    } finally {
      setIsLoading(false);
    }
  }, [leagueId, matchupId]);

  useEffect(() => {
    if (matchupId) {
      fetchMatchupDetail();
    }
  }, [matchupId, fetchMatchupDetail]);

  return { matchup, isLoading, error, refetch: fetchMatchupDetail };
}
