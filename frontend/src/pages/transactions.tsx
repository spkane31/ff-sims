import { useEffect, useState } from "react";
import { useRouter } from "next/router";
import LeagueFilterBar from "../components/LeagueFilterBar";
import { useSleeperTransactions } from "../hooks/useSleeperData";
import { SleeperLeagueFilters } from "../types/models";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import EmptyState from "@/components/design-system/EmptyState";
import ErrorState from "@/components/design-system/ErrorState";

const LIMIT = 25;

function formatDate(unixMs: number): string {
  if (!unixMs) return "—";
  return new Date(unixMs).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function txTypeLabel(type: string): string {
  switch (type) {
    case "trade": return "Trade";
    case "waiver": return "Waiver";
    case "free_agent": return "Free agent";
    default: return type;
  }
}

// Colorblind-safe qualitative palette (--chart-series-*) used to give each
// transaction type a distinct, scannable color — this is a category
// distinction, not a status, so the semantic --status-* tokens don't apply.
function txTypeColor(type: string): string {
  switch (type) {
    case "trade": return "var(--chart-series-1)";
    case "waiver": return "var(--chart-series-2)";
    case "free_agent": return "var(--chart-series-3)";
    default: return "var(--chart-series-6)";
  }
}

function filtersFromQuery(query: Record<string, string | string[] | undefined>): SleeperLeagueFilters {
  return {
    league_size: typeof query.league_size === "string" ? query.league_size : undefined,
    scoring_format: typeof query.scoring_format === "string" ? query.scoring_format : undefined,
    draft_type: typeof query.draft_type === "string" ? query.draft_type : undefined,
    league_type: typeof query.league_type === "string" ? query.league_type : undefined,
  };
}

export default function SleeperTransactionsPage() {
  const router = useRouter();
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState<SleeperLeagueFilters>({});
  const [txType, setTxType] = useState("");
  const [ready, setReady] = useState(false);

  useEffect(() => {
    if (!router.isReady) return;
    setFilters(filtersFromQuery(router.query));
    setTxType(typeof router.query.type === "string" ? router.query.type : "");
    const p = parseInt(router.query.page as string);
    if (p > 0) setPage(p);
    setReady(true);
  }, [router.isReady, router.query]);

  const { items, total, totalPages, isLoading, error } = useSleeperTransactions(
    ready ? page : 1,
    LIMIT,
    ready ? txType : "",
    ready ? filters : {}
  );

  function buildQuery(nextFilters: SleeperLeagueFilters, nextType: string, nextPage: number) {
    const q: Record<string, string> = { page: String(nextPage) };
    if (nextType) q.type = nextType;
    if (nextFilters.league_size) q.league_size = nextFilters.league_size;
    if (nextFilters.scoring_format) q.scoring_format = nextFilters.scoring_format;
    if (nextFilters.draft_type) q.draft_type = nextFilters.draft_type;
    if (nextFilters.league_type) q.league_type = nextFilters.league_type;
    return q;
  }

  function applyFilters(next: SleeperLeagueFilters) {
    setFilters(next);
    setPage(1);
    router.push({ pathname: router.pathname, query: buildQuery(next, txType, 1) }, undefined, { shallow: true });
  }

  function applyTxType(next: string) {
    setTxType(next);
    setPage(1);
    router.push({ pathname: router.pathname, query: buildQuery(filters, next, 1) }, undefined, { shallow: true });
  }

  function goToPage(p: number) {
    setPage(p);
    router.push({ pathname: router.pathname, query: buildQuery(filters, txType, p) }, undefined, { shallow: true });
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold" style={{ color: "var(--text-primary)" }}>
          Sleeper Transactions
        </h1>
        <p className="mt-1" style={{ color: "var(--text-secondary)" }}>
          {isLoading ? "Loading…" : `${total.toLocaleString()} transactions`}
        </p>
      </div>

      <LeagueFilterBar
        filters={filters}
        onChange={applyFilters}
        txType={txType}
        onTxTypeChange={applyTxType}
      />

      {error && <ErrorState message={`Failed to load transactions: ${error.message}`} />}

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
            <EmptyState title="No transactions found." />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr style={{ borderBottom: "1px solid var(--border-subtle)" }}>
                    <th className="px-4 py-3 text-left text-sm font-medium" style={{ color: "var(--text-muted)" }}>Date</th>
                    <th className="px-4 py-3 text-left text-sm font-medium" style={{ color: "var(--text-muted)" }}>Type</th>
                    <th className="px-4 py-3 text-left text-sm font-medium" style={{ color: "var(--text-muted)" }}>League</th>
                    <th className="px-4 py-3 text-left text-sm font-medium" style={{ color: "var(--text-muted)" }}>Season</th>
                    <th className="px-4 py-3 text-center text-sm font-medium" style={{ color: "var(--text-muted)" }}>Players</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((tx) => (
                    <tr
                      key={tx.id}
                      className="hover:bg-[var(--surface-sunken)] transition-colors"
                      style={{ borderBottom: "1px solid var(--border-subtle)" }}
                    >
                      <td className="px-4 py-3 text-sm whitespace-nowrap" style={{ color: "var(--text-secondary)" }}>
                        {formatDate(tx.created_at)}
                      </td>
                      <td className="px-4 py-3 text-sm">
                        <Badge
                          variant="outline"
                          style={{ color: txTypeColor(tx.type), borderColor: txTypeColor(tx.type) }}
                        >
                          {txTypeLabel(tx.type)}
                        </Badge>
                      </td>
                      <td className="px-4 py-3 text-sm max-w-xs truncate" style={{ color: "var(--text-primary)" }}>
                        {tx.league_name}
                      </td>
                      <td className="px-4 py-3 text-sm" style={{ color: "var(--text-secondary)" }}>{tx.season}</td>
                      <td className="px-4 py-3 text-sm text-center" style={{ color: "var(--text-secondary)" }}>
                        {tx.player_count}
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
