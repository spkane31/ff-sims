import Head from "next/head";
import Link from "next/link";
import { useRouter } from "next/router";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import EmptyState from "@/components/design-system/EmptyState";
import ErrorState from "@/components/design-system/ErrorState";
import { FOCUS_RING } from "@/components/design-system/focus-ring";
import DraftBoard, { type DraftRankMode } from "@/components/draft/DraftBoard";
import DraftFinishChart from "@/components/draft/DraftFinishChart";
import DraftResults from "@/components/draft/DraftResults";
import { useDraftRewind } from "@/hooks/useDraftRewind";
import type { DraftRewindPick } from "@/services/draftRewindService";
import {
  draftProjectionDifference, draftScoreReason, formatDraftDifference, formatDraftPoints, formatDraftRank, formatDraftSlot,
} from "@/utils/draftRewind";

function positiveID(value: string | string[] | undefined) {
  if (typeof value !== "string" || !/^\d+$/.test(value)) return undefined;
  const number = Number(value);
  return Number.isSafeInteger(number) && number > 0 ? number : undefined;
}

function PickDetails({ pick, team, throughWeek }: { pick: DraftRewindPick; team: string; throughWeek: number }) {
  return (
    <section
      className="grid gap-3 rounded-xl border border-[var(--border-subtle)] bg-[var(--surface-raised)] px-4 py-3 sm:grid-cols-[minmax(0,1fr)_2fr]"
      aria-label="Selected pick"
      aria-live="polite"
    >
      <div className="min-w-0">
        <div className="text-xs font-medium text-[var(--text-secondary)]">Pick {formatDraftSlot(pick)} · {team}</div>
        <h2 className="text-lg font-semibold">
          {pick.player_id > 0 ? (
            <Link href={`/players/${pick.player_id}`} className={`rounded-sm text-[var(--action-primary)] hover:underline ${FOCUS_RING}`}>
              {pick.player}
            </Link>
          ) : pick.player}
          {pick.position && <span className="ml-2 text-xs font-medium text-[var(--text-secondary)]">{pick.position}</span>}
        </h2>
        <p className="text-xs text-[var(--text-secondary)]">
          {pick.actual_points === null ? draftScoreReason(pick) : `${pick.observed_weeks} of ${throughWeek} weeks with team roster scores`}
        </p>
      </div>
      <dl className="grid grid-cols-3 items-center gap-3 text-sm tabular-nums">
        <div><dt className="text-xs text-[var(--text-secondary)]">Round rank</dt><dd className="font-semibold">{formatDraftRank(pick.round_rank)}</dd></div>
        <div><dt className="text-xs text-[var(--text-secondary)]">Overall rank</dt><dd className="font-semibold">{formatDraftRank(pick.overall_rank)}</dd></div>
        <div><dt className="text-xs text-[var(--text-secondary)]">Actual points</dt><dd className="font-semibold">{formatDraftPoints(pick.actual_points)}</dd></div>
        {pick.projected_points !== null && (
          <div className="col-span-3 border-t border-[var(--border-subtle)] pt-2 text-xs text-[var(--text-secondary)]">
            <dt className="inline font-medium">{pick.projection_weeks} matched projection {pick.projection_weeks === 1 ? "week" : "weeks"} </dt>
            <dd className="inline">· {formatDraftPoints(pick.projected_points)} projected · {formatDraftDifference(draftProjectionDifference(pick))} vs. projection</dd>
          </div>
        )}
      </dl>
    </section>
  );
}

export default function DraftRewind() {
  const router = useRouter();
  const leagueId = positiveID(router.query.leagueId);
  const requestedYear = positiveID(router.query.year);
  const { year, seasons, report } = useDraftRewind(leagueId, requestedYear);
  const view = router.query.view === "finish" || router.query.view === "results" ? router.query.view : "board";
  const rankMode: DraftRankMode = router.query.rank === "overall" ? "overall" : "round";
  const leagueYears = seasons.data?.years ?? [];
  const years = year && !leagueYears.includes(year) ? [year, ...leagueYears] : leagueYears;
  const data = report.data;
  const requestedPick = positiveID(router.query.pick);
  const selected = data?.picks.find((pick) => pick.id === requestedPick);
  const team = data?.teams.find((entry) => entry.id === selected?.team_id);
  const selectingYear = !router.isReady || seasons.loading;

  function updateQuery(changes: Record<string, string | undefined>) {
    const query: Record<string, string | string[] | undefined> = { ...router.query, year: year?.toString(), ...changes };
    for (const key of Object.keys(query)) {
      if (query[key] === undefined) delete query[key];
    }
    void router.push({ pathname: router.pathname, query }, undefined, { shallow: true, scroll: false });
  }

  function selectPick(id: number) {
    if (id !== requestedPick) updateQuery({ pick: String(id) });
  }

  return (
    <div className="space-y-4 sm:space-y-5">
      <Head><title>Draft Rewind{year ? ` · ${year}` : ""} | FF Sims</title></Head>
      <header className="flex flex-wrap items-end justify-between gap-4 border-b border-[var(--border-subtle)] pb-4">
        <div className="space-y-1">
          <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">Draft Rewind</h1>
          <p className="text-sm text-[var(--text-secondary)]">
            {data?.through_week
              ? `Through Week ${data.through_week} of the regular season · ${data.ranked_picks} of ${data.total_picks} picks ranked`
              : "See how your league’s draft picks performed."}
          </p>
        </div>
        {years.length > 0 && (
          <label className="flex flex-col gap-1 text-xs font-medium text-[var(--text-secondary)]">
            Season
            <select
              className={`min-w-24 rounded-md border border-[var(--border-strong)] bg-[var(--surface-raised)] px-3 py-1.5 text-sm text-[var(--text-primary)] ${FOCUS_RING}`}
              value={year}
              onChange={(event) => updateQuery({ year: event.target.value, pick: undefined })}
            >
              {years.map((season) => <option key={season}>{season}</option>)}
            </select>
          </label>
        )}
      </header>

      {selectingYear || report.loading ? (
        <div className="space-y-3" aria-label="Loading draft" role="status">
          <Skeleton className="h-10 w-56" />
          <Skeleton className="h-64 w-full" />
        </div>
      ) : seasons.error ? (
        <ErrorState message="Draft seasons could not be loaded." onRetry={seasons.retry} />
      ) : report.error ? (
        <ErrorState message="Draft results could not be loaded." onRetry={report.retry} />
      ) : !data?.picks.length ? (
        <EmptyState title="No draft picks found" description={year ? `There are no imported picks for the ${year} season.` : "Drafts appear here once the league has imported picks."} />
      ) : (
        <Tabs key={data.year} value={view} onValueChange={(next) => updateQuery({ view: next })} className="gap-4">
          <TabsList aria-label="Draft Rewind views" className="h-10 sm:h-10">
            <TabsTrigger value="board" className="px-2.5 sm:px-3">Board</TabsTrigger>
            <TabsTrigger value="finish" className="px-2.5 sm:px-3">Pick vs. Finish</TabsTrigger>
            <TabsTrigger value="results" className="px-2.5 sm:px-3">Results</TabsTrigger>
          </TabsList>
          {selected && <PickDetails pick={selected} team={team?.name ?? "Unknown team"} throughWeek={data.through_week} />}
          <TabsContent value="board" className="space-y-3">
            <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
              <div className="flex flex-wrap items-center gap-2" role="group" aria-label="Rank picks">
                <span className="text-sm font-medium">Rank</span>
                <Button type="button" size="sm" variant={rankMode === "round" ? "default" : "outline"} aria-pressed={rankMode === "round"} onClick={() => updateQuery({ rank: "round" })}>By Round</Button>
                <Button type="button" size="sm" variant={rankMode === "overall" ? "default" : "outline"} aria-pressed={rankMode === "overall"} onClick={() => updateQuery({ rank: "overall" })}>Overall</Button>
              </div>
              {data.ranked_picks > 0 && (
                <div className="flex items-center gap-2 text-xs text-[var(--text-secondary)]">
                  <span>Lower finish</span>
                  <span className="h-2 w-16 rounded-full" style={{ background: "linear-gradient(90deg, var(--surface-sunken), color-mix(in oklab, var(--chart-series-1) 37%, var(--surface-sunken)))" }} aria-hidden="true" />
                  <span>Higher finish</span>
                </div>
              )}
            </div>
            {data.ranked_picks === 0 && (
              <p className="rounded-md bg-[var(--surface-sunken)] p-3 text-sm text-[var(--text-secondary)]">
                {data.through_week ? "This season has draft picks but no usable player scores." : "Ranks become available when completed regular-season player scores are synced."}
              </p>
            )}
            <DraftBoard report={data} mode={rankMode} selectedId={selected?.id} onSelect={selectPick} />
          </TabsContent>
          <TabsContent value="finish">
            <DraftFinishChart report={data} selectedId={selected?.id} onSelect={selectPick} />
          </TabsContent>
          <TabsContent value="results">
            <DraftResults report={data} selectedId={selected?.id} onSelect={selectPick} />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}
