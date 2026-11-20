import { useEffect, useMemo, useState } from "react";
import { draftRewindService } from "@/services/draftRewindService";

type DraftRequestResult<T> = { data: T; error?: never } | { data?: never; error: true };

function useDraftRequest<T>(request: (() => Promise<T>) | null) {
  const [revision, setRevision] = useState(0);
  const [settled, setSettled] = useState<{
    request: () => Promise<T>;
    revision: number;
    result: DraftRequestResult<T>;
  }>();

  useEffect(() => {
    if (!request) return;
    let active = true;
    const settle = (result: DraftRequestResult<T>) => {
      if (active) setSettled({ request, revision, result });
    };
    request().then((data) => settle({ data }), () => settle({ error: true }));
    return () => { active = false; };
  }, [request, revision]);

  const result = settled && settled.request === request && settled.revision === revision
    ? settled.result
    : undefined;
  return {
    data: result?.data,
    error: result?.error,
    loading: Boolean(request && !result),
    retry: () => setRevision((attempt) => attempt + 1),
  };
}

export function useDraftRewind(leagueId: number | undefined, requestedYear: number | undefined) {
  const loadSeasons = useMemo(() => leagueId ? () => draftRewindService.getSeasons(leagueId) : null, [leagueId]);
  const seasons = useDraftRequest(loadSeasons);
  const year = requestedYear ?? seasons.data?.years[0];
  const loadReport = useMemo(() => leagueId && year ? () => draftRewindService.getReport(leagueId, year) : null, [leagueId, year]);
  const report = useDraftRequest(loadReport);
  return { year, seasons, report };
}
