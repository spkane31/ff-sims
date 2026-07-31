import { useEffect, useState } from "react";
import { useRouter } from "next/router";
import Link from "next/link";
import ADPFilterBar from "../../components/ADPFilterBar";
import { useSleeperADPAll } from "../../hooks/useSleeperData";
import { SleeperADPFilters } from "../../types/models";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import EmptyState from "@/components/design-system/EmptyState";
import ErrorState from "@/components/design-system/ErrorState";

const LIMIT = 25;

function filtersFromQuery(query: Record<string, string | string[] | undefined>): SleeperADPFilters {
  return {
    league_size: typeof query.league_size === "string" ? query.league_size : undefined,
    scoring_format: typeof query.scoring_format === "string" ? query.scoring_format : undefined,
    superflex: typeof query.superflex === "string" ? query.superflex : undefined,
    season: typeof query.season === "string" ? query.season : undefined,
  };
}

export default function SleeperADPPage() {
  const router = useRouter();
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState<SleeperADPFilters>({});
  const [position, setPosition] = useState("");
  const [ready, setReady] = useState(false);

  useEffect(() => {
    if (!router.isReady) return;
    setFilters(filtersFromQuery(router.query));
    setPosition(typeof router.query.position === "string" ? router.query.position : "");
    const p = parseInt(router.query.page as string);
    if (p > 0) setPage(p);
    setReady(true);
  }, [router.isReady, router.query]);

  const { items: allItems, season, availableSeasons, isLoading, error } = useSleeperADPAll(
    ready ? filters : {}
  );

  const filtered = position ? allItems.filter(p => p.position === position) : allItems;
  const total = filtered.length;
  const totalPages = Math.max(1, Math.ceil(total / LIMIT));
  const items = filtered.slice((page - 1) * LIMIT, page * LIMIT);

  function applyFilters(next: SleeperADPFilters) {
    setFilters(next);
    setPage(1);
    const q: Record<string, string> = { page: "1" };
    if (next.league_size) q.league_size = next.league_size;
    if (next.scoring_format) q.scoring_format = next.scoring_format;
    if (next.superflex) q.superflex = next.superflex;
    if (next.season) q.season = next.season;
    if (position) q.position = position;
    router.push({ pathname: router.pathname, query: q }, undefined, { shallow: true });
  }

  function applyPosition(next: string) {
    setPosition(next);
    setPage(1);
    const q: Record<string, string> = { ...(router.query as Record<string, string>), page: "1" };
    if (next) {
      q.position = next;
    } else {
      delete q.position;
    }
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
          Average Draft Position
        </h1>
        <p className="mt-1" style={{ color: "var(--text-secondary)" }}>
          {isLoading ? "Loading…" : `${total.toLocaleString()} players${season ? ` — ${season} season` : ""}`}
        </p>
      </div>

      <ADPFilterBar
        filters={filters}
        onChange={applyFilters}
        availableSeasons={availableSeasons}
        position={position}
        onPositionChange={applyPosition}
      />

      {error && <ErrorState message={`Failed to load ADP: ${error.message}`} />}

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
            <EmptyState title="No players found for this filter combination." />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr style={{ borderBottom: "1px solid var(--border-subtle)" }}>
                    <th className="px-4 py-3 text-left text-sm font-medium" style={{ color: "var(--text-muted)" }}>Rank</th>
                    <th className="px-4 py-3 text-left text-sm font-medium" style={{ color: "var(--text-muted)" }}>Player</th>
                    <th className="px-4 py-3 text-left text-sm font-medium" style={{ color: "var(--text-muted)" }}>Pos</th>
                    <th className="px-4 py-3 text-left text-sm font-medium" style={{ color: "var(--text-muted)" }}>Team</th>
                    <th className="px-4 py-3 text-center text-sm font-medium" style={{ color: "var(--text-muted)" }}>Avg Pick</th>
                    <th className="px-4 py-3 text-center text-sm font-medium" style={{ color: "var(--text-muted)" }}>95% CI</th>
                    <th className="px-4 py-3 text-center text-sm font-medium" style={{ color: "var(--text-muted)" }}>Drafts</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((player, i) => (
                    <tr
                      key={player.sleeper_player_id}
                      className="hover:bg-[var(--surface-sunken)] transition-colors"
                      style={{ borderBottom: "1px solid var(--border-subtle)" }}
                    >
                      <td className="px-4 py-3 text-sm" style={{ color: "var(--text-secondary)" }}>
                        {(page - 1) * LIMIT + i + 1}
                      </td>
                      <td className="px-4 py-3 text-sm max-w-xs truncate" style={{ color: "var(--text-primary)" }}>
                        {player.player_id ? (
                          <Link
                            href={`/players/${player.player_id}`}
                            className="hover:underline"
                            style={{ color: "var(--action-primary)" }}
                          >
                            {player.name}
                          </Link>
                        ) : (
                          player.name
                        )}
                      </td>
                      <td className="px-4 py-3 text-sm" style={{ color: "var(--text-secondary)" }}>{player.position}</td>
                      <td className="px-4 py-3 text-sm" style={{ color: "var(--text-secondary)" }}>{player.nfl_team}</td>
                      <td className="px-4 py-3 text-sm text-center" style={{ color: "var(--text-secondary)" }}>
                        {player.avg_pick_no.toFixed(1)}
                      </td>
                      <td className="px-4 py-3 text-sm text-center" style={{ color: "var(--text-secondary)" }}>
                        {player.ci_low_pick_no.toFixed(1)}–{player.ci_high_pick_no.toFixed(1)}
                      </td>
                      <td className="px-4 py-3 text-sm text-center" style={{ color: "var(--text-secondary)" }}>
                        {player.pick_count}
                      </td>
                    </tr>
                  ))}
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
