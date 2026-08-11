import { useEffect, useState, ReactNode } from "react";
import { useRouter } from "next/router";
import Link from "next/link";
import LeagueFilterBar from "../components/LeagueFilterBar";
import { useSleeperTrades } from "../hooks/useSleeperData";
import { SleeperLeagueFilters, SleeperTrade, TradeSidePlayer } from "../types/models";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import EmptyState from "@/components/design-system/EmptyState";
import ErrorState from "@/components/design-system/ErrorState";

const LIMIT = 25;

function formatDate(unixMs: number): string {
  if (!unixMs) return "—";
  return new Date(unixMs).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    second: "2-digit",
  });
}

// Renders a trade-side player as a link to its /players/:id page when the
// Sleeper player could be resolved to an internal (ESPN-sourced) player;
// otherwise falls back to plain text.
function SidePlayer({ player }: { player: TradeSidePlayer }) {
  const label = player.position ? `${player.name} (${player.position})` : player.name;
  if (!player.player_id) return <>{label}</>;
  return (
    <Link href={`/players/${player.player_id}`} className="hover:underline" style={{ color: "var(--action-primary)" }}>
      {label}
    </Link>
  );
}

function sideItems(side: SleeperTrade["sides"][number] | undefined): ReactNode[] {
  if (!side) return [];
  return [
    ...(side.players ?? []).map((p, i) => <SidePlayer key={`player-${i}`} player={p} />),
    ...(side.picks ?? []).map((pick, i) => <span key={`pick-${i}`}>{pick}</span>),
  ];
}

function formatValue(total: number | null | undefined): string {
  if (total === null || total === undefined) return "—";
  return Math.round(total).toLocaleString();
}

// Index of the side the model thinks won (higher total value), or null when
// either side has no valuation or the totals tie.
function winningSide(trade: SleeperTrade): number | null {
  const a = trade.sides?.[0]?.total_value;
  const b = trade.sides?.[1]?.total_value;
  if (a === null || a === undefined || b === null || b === undefined || a === b) return null;
  return a > b ? 0 : 1;
}

// Defaults match the user's own league (10-team, PPR, superflex) so the page
// opens pre-filtered to the trades most relevant to them.
const DEFAULT_FILTERS: SleeperLeagueFilters = {
  league_size: "10",
  scoring_format: "ppr",
  superflex: "true",
};

// "any" is an explicit marker written to the URL when the user clears a
// defaulted filter back to "Any" — distinct from the param being absent
// entirely, which means "not yet touched, use the default".
function resolveDefaulted(v: string | string[] | undefined, fallback: string | undefined): string | undefined {
  if (typeof v !== "string") return fallback;
  return v === "any" ? undefined : v;
}

function filtersFromQuery(query: Record<string, string | string[] | undefined>): SleeperLeagueFilters {
  return {
    league_size: resolveDefaulted(query.league_size, DEFAULT_FILTERS.league_size),
    scoring_format: resolveDefaulted(query.scoring_format, DEFAULT_FILTERS.scoring_format),
    draft_type: typeof query.draft_type === "string" ? query.draft_type : undefined,
    league_type: typeof query.league_type === "string" ? query.league_type : undefined,
    superflex: resolveDefaulted(query.superflex, DEFAULT_FILTERS.superflex),
  };
}

export default function SleeperTradesPage() {
  const router = useRouter();
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState<SleeperLeagueFilters>(DEFAULT_FILTERS);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    if (!router.isReady) return;
    setFilters(filtersFromQuery(router.query));
    const p = parseInt(router.query.page as string);
    if (p > 0) setPage(p);
    setReady(true);
  }, [router.isReady, router.query]);

  const { items, total, totalPages, isLoading, error } = useSleeperTrades(
    ready ? page : 1,
    LIMIT,
    ready ? filters : {}
  );

  function applyFilters(next: SleeperLeagueFilters) {
    setFilters(next);
    setPage(1);
    const q: Record<string, string> = { page: "1" };
    q.league_size = next.league_size ?? "any";
    q.scoring_format = next.scoring_format ?? "any";
    q.superflex = next.superflex ?? "any";
    if (next.draft_type) q.draft_type = next.draft_type;
    if (next.league_type) q.league_type = next.league_type;
    router.push({ pathname: router.pathname, query: q }, undefined, { shallow: true });
  }

  function goToPage(p: number) {
    setPage(p);
    const q: Record<string, string> = { ...router.query as Record<string, string>, page: String(p) };
    router.push({ pathname: router.pathname, query: q }, undefined, { shallow: true });
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold" style={{ color: "var(--text-primary)" }}>
          Sleeper Trades
        </h1>
        <p className="mt-1" style={{ color: "var(--text-secondary)" }}>
          {isLoading ? "Loading…" : `${total.toLocaleString()} completed trades`}
        </p>
        <p className="text-sm mt-1" style={{ color: "var(--text-muted)" }}>
          Values are the model&apos;s player valuations as of the trade date, using the valuation
          segment matching the trade&apos;s league format (full-PPR superflex redraft, 8/10/12-team);
          trades from other formats show &quot;—&quot;. The highlighted side is the one the model
          favored. Draft picks are not valued.
        </p>
      </div>

      <LeagueFilterBar filters={filters} onChange={applyFilters} showSuperflexFilter />

      {error && <ErrorState message={`Failed to load trades: ${error.message}`} />}

      <Card>
        <CardContent className="p-6">
          {isLoading ? (
            <div className="space-y-2">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          ) : items.length === 0 ? (
            <EmptyState title="No trades found." />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr style={{ borderBottom: "1px solid var(--border-subtle)" }}>
                    <th className="px-4 py-3 text-left text-sm font-medium" style={{ color: "var(--text-muted)" }}>Date &amp; Time</th>
                    <th className="px-4 py-3 text-left text-sm font-medium" style={{ color: "var(--text-muted)" }}>PPR</th>
                    <th className="px-4 py-3 text-left text-sm font-medium" style={{ color: "var(--text-muted)" }}>SuperFlex</th>
                    <th className="px-4 py-3 text-left text-sm font-medium" style={{ color: "var(--text-muted)" }}>League Size</th>
                    <th className="px-4 py-3 text-left text-sm font-medium" style={{ color: "var(--text-muted)" }}>Side A</th>
                    <th className="px-4 py-3 text-right text-sm font-medium" style={{ color: "var(--text-muted)" }}>Value A</th>
                    <th className="px-4 py-3 text-left text-sm font-medium" style={{ color: "var(--text-muted)" }}>Side B</th>
                    <th className="px-4 py-3 text-right text-sm font-medium" style={{ color: "var(--text-muted)" }}>Value B</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((trade) => {
                    const winner = winningSide(trade);
                    return (
                      <tr
                        key={trade.id}
                        className="hover:bg-[var(--surface-sunken)] transition-colors"
                        style={{ borderBottom: "1px solid var(--border-subtle)" }}
                      >
                        <td className="px-4 py-3 text-sm whitespace-nowrap" style={{ color: "var(--text-secondary)" }}>
                          {formatDate(trade.created_at)}
                        </td>
                        <td className="px-4 py-3 text-sm whitespace-nowrap" style={{ color: "var(--text-secondary)" }}>
                          {trade.scoring}
                        </td>
                        <td className="px-4 py-3 text-sm whitespace-nowrap" style={{ color: "var(--text-secondary)" }}>
                          {trade.superflex ? "Yes" : "No"}
                        </td>
                        <td className="px-4 py-3 text-sm whitespace-nowrap" style={{ color: "var(--text-secondary)" }}>
                          {trade.league_size}
                        </td>
                        <td
                          className="px-4 py-3 text-sm align-top max-w-xs"
                          style={{
                            color: "var(--text-secondary)",
                            backgroundColor: winner === 0 ? "var(--status-success-bg)" : undefined,
                          }}
                        >
                          {sideItems(trade.sides?.[0]).length > 0 ? (
                            <ul className="space-y-0.5">
                              {sideItems(trade.sides?.[0]).map((item, i) => (
                                <li key={i}>{item}</li>
                              ))}
                            </ul>
                          ) : (
                            "—"
                          )}
                        </td>
                        <td
                          className="px-4 py-3 text-sm text-right align-top whitespace-nowrap"
                          style={{
                            color: winner === 0 ? "var(--status-success-fg)" : "var(--text-secondary)",
                            backgroundColor: winner === 0 ? "var(--status-success-bg)" : undefined,
                            fontWeight: winner === 0 ? 600 : undefined,
                          }}
                        >
                          {formatValue(trade.sides?.[0]?.total_value)}
                        </td>
                        <td
                          className="px-4 py-3 text-sm align-top max-w-xs"
                          style={{
                            color: "var(--text-secondary)",
                            backgroundColor: winner === 1 ? "var(--status-success-bg)" : undefined,
                          }}
                        >
                          {sideItems(trade.sides?.[1]).length > 0 ? (
                            <ul className="space-y-0.5">
                              {sideItems(trade.sides?.[1]).map((item, i) => (
                                <li key={i}>{item}</li>
                              ))}
                            </ul>
                          ) : (
                            "—"
                          )}
                        </td>
                        <td
                          className="px-4 py-3 text-sm text-right align-top whitespace-nowrap"
                          style={{
                            color: winner === 1 ? "var(--status-success-fg)" : "var(--text-secondary)",
                            backgroundColor: winner === 1 ? "var(--status-success-bg)" : undefined,
                            fontWeight: winner === 1 ? 600 : undefined,
                          }}
                        >
                          {formatValue(trade.sides?.[1]?.total_value)}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      {totalPages > 1 && (
        <div className="flex items-center justify-between">
          <Button
            variant="outline"
            onClick={() => goToPage(page - 1)}
            disabled={page <= 1 || isLoading}
          >
            Previous
          </Button>
          <span className="text-sm" style={{ color: "var(--text-secondary)" }}>
            Page {page} of {totalPages}
          </span>
          <Button
            variant="outline"
            onClick={() => goToPage(page + 1)}
            disabled={page >= totalPages || isLoading}
          >
            Next
          </Button>
        </div>
      )}
    </div>
  );
}
