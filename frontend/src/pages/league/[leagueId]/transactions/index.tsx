import { useEffect, useState } from "react";
import { useRouter } from "next/router";
import Link from "next/link";
import { useDraftPicks, useTransactions } from "@/hooks/useTransactions";
import { useLeagueYears } from "@/hooks/useLeagues";
import type { DraftPick, Transaction } from "@/services/transactionsService";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import EmptyState from "@/components/design-system/EmptyState";
import ErrorState from "@/components/design-system/ErrorState";
import DataTable, { type DataTableColumn } from "@/components/design-system/DataTable";

const LIMIT = 25;

type Tab = "transactions" | "draft-picks";

function txTypeLabel(type: string): string {
  switch (type) {
    case "trade": return "Trade";
    case "waiver": return "Waiver";
    case "draft": return "Draft";
    default: return type;
  }
}

// Colorblind-safe qualitative palette (--chart-series-*) — transaction type is
// an unordered category (trade vs. waiver vs. everything else), not a status,
// so the semantic --status-* tokens don't apply here.
function txTypeColor(type: string): string {
  switch (type) {
    case "trade": return "var(--chart-series-1)";
    case "waiver": return "var(--chart-series-2)";
    default: return "var(--chart-series-6)";
  }
}

// Same qualitative-palette reasoning as txTypeColor: roster position is an
// unordered category label, not a status.
function positionColor(position: string): string {
  switch (position) {
    case "QB": return "var(--chart-series-1)";
    case "RB": return "var(--chart-series-2)";
    case "WR": return "var(--chart-series-3)";
    case "TE": return "var(--chart-series-4)";
    case "K": return "var(--chart-series-5)";
    case "DEF": return "var(--chart-series-6)";
    default: return "var(--chart-series-6)";
  }
}

export default function Transactions() {
  const router = useRouter();
  const leagueId = Number(router.query.leagueId);
  const [tab, setTab] = useState<Tab>("transactions");
  const [selectedYear, setSelectedYear] = useState<number>(2024);
  const [page, setPage] = useState(1);
  const [hasInitialized, setHasInitialized] = useState(false);

  const { years: availableYears, isLoading: yearsLoading } = useLeagueYears(leagueId);

  const {
    transactions,
    total: txTotal,
    totalPages: txTotalPages,
    isLoading: txLoading,
    error: txError,
  } = useTransactions(leagueId, page, LIMIT, tab === "transactions" ? selectedYear : undefined);

  const {
    draftPicks,
    total: dpTotal,
    totalPages: dpTotalPages,
    isLoading: dpLoading,
    error: dpError,
  } = useDraftPicks(leagueId, selectedYear, page, LIMIT);

  useEffect(() => {
    if (!router.isReady) return;
    if (availableYears && availableYears.length > 0 && !hasInitialized) {
      const urlYear = parseInt(router.query.year as string);
      setSelectedYear(urlYear > 0 ? urlYear : availableYears[0]);
      const urlPage = parseInt(router.query.page as string);
      if (urlPage > 0) setPage(urlPage);
      const urlTab = router.query.tab as string;
      if (urlTab === "draft-picks" || urlTab === "transactions") setTab(urlTab);
      setHasInitialized(true);
    }
  }, [router.isReady, router.query, availableYears, hasInitialized]);

  function changeTab(next: Tab) {
    setTab(next);
    setPage(1);
    router.push(
      { pathname: router.pathname, query: { ...router.query, tab: next, page: "1" } },
      undefined,
      { shallow: true }
    );
  }

  function changeYear(year: number) {
    setSelectedYear(year);
    setPage(1);
    router.push(
      { pathname: router.pathname, query: { ...router.query, year: String(year), page: "1" } },
      undefined,
      { shallow: true }
    );
  }

  function goToPage(p: number) {
    setPage(p);
    router.push(
      { pathname: router.pathname, query: { ...router.query, page: String(p) } },
      undefined,
      { shallow: true }
    );
  }

  const isLoading = tab === "transactions" ? txLoading : dpLoading;
  const total = tab === "transactions" ? txTotal : dpTotal;
  const error = tab === "transactions" ? txError : dpError;

  const transactionColumns: DataTableColumn<Transaction>[] = [
    {
      id: "date",
      header: "Date",
      cell: (tx) => <span style={{ color: "var(--text-secondary)" }}>{tx.date}</span>,
    },
    {
      id: "type",
      header: "Type",
      cell: (tx) => (
        <Badge
          variant="outline"
          style={{ color: txTypeColor(tx.type), borderColor: txTypeColor(tx.type) }}
        >
          {txTypeLabel(tx.type)}
        </Badge>
      ),
    },
    {
      id: "team",
      header: "Team",
      cell: (tx) => <span style={{ color: "var(--text-primary)" }}>{tx.teams?.[0] ?? "—"}</span>,
    },
    {
      id: "players",
      header: "Players",
      cell: (tx) => (
        <span style={{ color: "var(--text-secondary)" }}>
          {tx.players?.map((p) => p.name).join(", ") || "—"}
        </span>
      ),
    },
  ];

  const draftPickColumns: DataTableColumn<DraftPick>[] = [
    {
      id: "round",
      header: "Round",
      cell: (pick) => <span style={{ color: "var(--text-primary)" }}>{pick.round}</span>,
    },
    {
      id: "pickNumber",
      header: "Pick #",
      cell: (pick) => <span style={{ color: "var(--text-primary)" }}>{pick.pick}</span>,
    },
    {
      id: "owner",
      header: "Drafting Owner",
      cell: (pick) => (
        <Link
          href={`/league/${leagueId}/teams/${pick.team_id}`}
          className="font-medium hover:underline"
          style={{ color: "var(--action-primary)" }}
        >
          {pick.owner}
        </Link>
      ),
    },
    {
      id: "player",
      header: "Player",
      cell: (pick) => (
        <Link
          href={`/players/${pick.player_id}`}
          className="font-medium hover:underline"
          style={{ color: "var(--action-primary)" }}
        >
          {pick.player}
        </Link>
      ),
    },
    {
      id: "position",
      header: "Position",
      cell: (pick) => (
        <Badge
          variant="outline"
          style={{ color: positionColor(pick.position), borderColor: positionColor(pick.position) }}
        >
          {pick.position}
        </Badge>
      ),
    },
  ];

  return (
    <div className="space-y-8">
      <section>
        <h1 className="text-3xl md:text-4xl font-bold mb-6" style={{ color: "var(--text-primary)" }}>
          Transactions
        </h1>

        <Tabs value={tab} onValueChange={(v) => changeTab(v as Tab)}>
          <TabsList variant="line" className="mb-6">
            <TabsTrigger value="transactions" className="px-4">
              Transactions
            </TabsTrigger>
            <TabsTrigger value="draft-picks" className="px-4">
              Draft Picks
            </TabsTrigger>
          </TabsList>

          {/* Year filter */}
          <Card className="mb-6">
            <CardContent className="p-4">
              <div className="flex items-center gap-4">
                <span className="text-sm font-medium" style={{ color: "var(--text-primary)" }}>
                  Season
                </span>
                {yearsLoading ? (
                  <Skeleton className="h-8 w-32" />
                ) : (
                  <Select
                    value={String(selectedYear)}
                    onValueChange={(v) => changeYear(parseInt(v))}
                  >
                    <SelectTrigger aria-label="Season" className="w-32">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {availableYears.map((year) => (
                        <SelectItem key={year} value={String(year)}>
                          {year}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
                {!isLoading && total > 0 && (
                  <span className="text-sm" style={{ color: "var(--text-muted)" }}>
                    {total.toLocaleString()} {tab === "transactions" ? "transactions" : "picks"}
                  </span>
                )}
              </div>
            </CardContent>
          </Card>

          {error && <ErrorState message={error.message} />}

          {/* Transactions tab */}
          <TabsContent value="transactions" className="space-y-4">
            <Card>
              <CardContent className="p-6">
                {txLoading ? (
                  <div className="space-y-2">
                    <Skeleton className="h-10 w-full" />
                    <Skeleton className="h-10 w-full" />
                    <Skeleton className="h-10 w-full" />
                    <Skeleton className="h-10 w-full" />
                    <Skeleton className="h-10 w-full" />
                  </div>
                ) : !transactions || transactions.length === 0 ? (
                  <EmptyState title={`No transactions found for ${selectedYear}`} />
                ) : (
                  <DataTable
                    columns={transactionColumns}
                    rows={transactions}
                    rowKey={(tx) => String(tx.id)}
                  />
                )}
              </CardContent>
            </Card>

            {txTotalPages > 1 && (
              <div className="flex items-center justify-between">
                <Button
                  variant="outline"
                  onClick={() => goToPage(page - 1)}
                  disabled={page <= 1 || txLoading}
                >
                  Previous
                </Button>
                <span className="text-sm" style={{ color: "var(--text-secondary)" }}>
                  Page {page} of {txTotalPages}
                </span>
                <Button
                  variant="outline"
                  onClick={() => goToPage(page + 1)}
                  disabled={page >= txTotalPages || txLoading}
                >
                  Next
                </Button>
              </div>
            )}
          </TabsContent>

          {/* Draft Picks tab */}
          <TabsContent value="draft-picks" className="space-y-4">
            <Card>
              <CardContent className="p-6">
                {dpLoading ? (
                  <div className="space-y-2">
                    <Skeleton className="h-10 w-full" />
                    <Skeleton className="h-10 w-full" />
                    <Skeleton className="h-10 w-full" />
                    <Skeleton className="h-10 w-full" />
                    <Skeleton className="h-10 w-full" />
                  </div>
                ) : !draftPicks || draftPicks.length === 0 ? (
                  <EmptyState
                    title="No draft picks found"
                    description="Try changing the year to see draft results"
                  />
                ) : (
                  <DataTable
                    columns={draftPickColumns}
                    rows={draftPicks}
                    rowKey={(pick) => `${pick.round}-${pick.pick}`}
                  />
                )}
              </CardContent>
            </Card>

            {dpTotalPages > 1 && (
              <div className="flex items-center justify-between">
                <Button
                  variant="outline"
                  onClick={() => goToPage(page - 1)}
                  disabled={page <= 1 || dpLoading}
                >
                  Previous
                </Button>
                <span className="text-sm" style={{ color: "var(--text-secondary)" }}>
                  Page {page} of {dpTotalPages}
                </span>
                <Button
                  variant="outline"
                  onClick={() => goToPage(page + 1)}
                  disabled={page >= dpTotalPages || dpLoading}
                >
                  Next
                </Button>
              </div>
            )}
          </TabsContent>
        </Tabs>
      </section>
    </div>
  );
}
