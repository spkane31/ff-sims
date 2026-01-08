import { useRouter } from 'next/router';

export function useLeague() {
  const router = useRouter();
  const { leagueId } = router.query;

  return {
    leagueId: leagueId as string | undefined,
    isLeagueContext: !!leagueId,
  };
}
