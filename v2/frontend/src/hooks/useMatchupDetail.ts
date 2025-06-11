import { useState, useEffect } from 'react';
import { GetMatchupDetailResponse, scheduleService } from '../services/scheduleService';

/**
 * Hook for fetching details of a specific matchup
 */
export function useMatchupDetail(matchupId: string) {
  const [matchup, setMatchup] = useState<GetMatchupDetailResponse | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchMatchupDetail = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await scheduleService.getMatchupById(matchupId);
      setMatchup(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('An error occurred while fetching matchup details'));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    if (matchupId) {
      fetchMatchupDetail();
    }
  }, [matchupId]);

  return { matchup, isLoading, error, refetch: fetchMatchupDetail };
}