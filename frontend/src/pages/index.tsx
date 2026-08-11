import Link from "next/link";
import { useLeagues } from "../hooks/useLeagues";
import { useSleeperStats } from "../hooks/useSleeperData";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import EmptyState from "@/components/design-system/EmptyState";
import ErrorState from "@/components/design-system/ErrorState";

export default function Home() {
  const { leagues, isLoading, error } = useLeagues();
  const { stats: sleeperStats, isLoading: sleeperLoading } = useSleeperStats();

  return (
    <div className="space-y-8">
      <section className="text-center md:text-left">
        <h1
          className="text-4xl md:text-5xl font-bold mb-4"
          style={{ color: "var(--text-primary)" }}
        >
          Fantasy Football Simulations
        </h1>
        <p className="text-lg max-w-3xl" style={{ color: "var(--text-secondary)" }}>
          Select a league to view standings, schedule, simulations, and more.
        </p>
      </section>

      <section>
        <h2
          className="text-2xl font-semibold mb-6"
          style={{ color: "var(--text-primary)" }}
        >
          Your Leagues
        </h2>
        {isLoading && (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            <Skeleton className="h-32 w-full" />
            <Skeleton className="h-32 w-full" />
            <Skeleton className="h-32 w-full" />
          </div>
        )}
        {!isLoading && error && <ErrorState message="Failed to load leagues." />}
        {!isLoading && !error && leagues.length === 0 && (
          <EmptyState title="No leagues found." />
        )}
        {!isLoading && !error && leagues.length > 0 && (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {leagues.map((league) => (
              <Link key={league.id} href={`/league/${league.id}`} className="block">
                <Card className="transition-shadow hover:shadow-lg">
                  <CardContent className="p-6">
                    <h3
                      className="text-xl font-semibold mb-2"
                      style={{ color: "var(--text-primary)" }}
                    >
                      {league.name}
                    </h3>
                    <div
                      className="text-sm space-y-1"
                      style={{ color: "var(--text-muted)" }}
                    >
                      <p>Platform: {league.platform || "—"}</p>
                      {league.current_week > 0 && (
                        <p>
                          Week {league.current_week} of {league.total_weeks}
                        </p>
                      )}
                    </div>
                    <p
                      className="mt-4 text-sm font-medium"
                      style={{ color: "var(--action-primary)" }}
                    >
                      Open league →
                    </p>
                  </CardContent>
                </Card>
              </Link>
            ))}
          </div>
        )}
      </section>

      <section className="py-6">
        <h2 className="text-2xl font-semibold mb-2" style={{ color: "var(--text-primary)" }}>
          Sleeper Data
        </h2>
        <p className="mb-6 text-sm" style={{ color: "var(--text-secondary)" }}>
          Trade and draft data collected from Sleeper leagues across the network.
        </p>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          {[
            {
              label: "Leagues",
              count: sleeperStats?.leagues_expanded ?? null,
              link: "/drafts",
              description: "Total leagues tracked",
            },
            {
              label: "Trades",
              count: sleeperStats?.trade_count ?? null,
              link: "/trades?page=1&league_size=10&scoring_format=ppr&superflex=true&league_type=redraft",
              description: "Completed trades recorded",
            },
            {
              label: "Drafts",
              count: sleeperStats?.draft_count ?? null,
              link: "/drafts",
              description: "Completed drafts with picks",
            },
          ].map((card) => (
            <Link key={card.label} href={card.link} className="block">
              <Card className="cursor-pointer transition-shadow hover:shadow-lg">
                <CardContent className="p-6">
                  <div
                    className="text-3xl font-bold mb-1"
                    style={{ color: "var(--text-primary)" }}
                  >
                    {sleeperLoading ? (
                      <span style={{ color: "var(--text-muted)" }}>—</span>
                    ) : (
                      (card.count ?? 0).toLocaleString()
                    )}
                  </div>
                  <div
                    className="text-lg font-medium"
                    style={{ color: "var(--text-primary)" }}
                  >
                    {card.label}
                  </div>
                  <div className="text-sm mt-1" style={{ color: "var(--text-muted)" }}>
                    {card.description}
                  </div>
                  <div
                    className="mt-4 text-sm font-medium"
                    style={{ color: "var(--action-primary)" }}
                  >
                    Explore →
                  </div>
                </CardContent>
              </Card>
            </Link>
          ))}
        </div>
      </section>
    </div>
  );
}
