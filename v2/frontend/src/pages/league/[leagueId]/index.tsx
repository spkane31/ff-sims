import { useRouter } from "next/router";
import Link from "next/link";
import Layout from "@/components/Layout";
import { leaguesService, League } from "@/services/leaguesService";
import { useState, useEffect } from "react";

export default function LeagueDashboard() {
  const router = useRouter();
  const { leagueId } = router.query;
  const id = Number(leagueId);

  const [league, setLeague] = useState<League | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    if (!id) return;
    setIsLoading(true);
    leaguesService.getLeague(id)
      .then(setLeague)
      .catch(setError)
      .finally(() => setIsLoading(false));
  }, [id]);

  const navItems = [
    { name: "Teams", path: `/league/${id}/teams`, description: "Standings, head-to-head records, and league stats" },
    { name: "Schedule", path: `/league/${id}/schedule`, description: "Matchups, results, and strength of schedule" },
    { name: "Simulations", path: `/league/${id}/simulations`, description: "Playoff odds and season projections" },
    { name: "Transactions", path: `/league/${id}/transactions`, description: "Draft picks and waiver wire activity" },
    { name: "Players", path: `/league/${id}/players`, description: "Player stats and game logs" },
  ];

  return (
    <Layout>
      <div className="space-y-8">
        <section>
          {isLoading ? (
            <div className="flex items-center space-x-2">
              <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin" />
              <p>Loading league...</p>
            </div>
          ) : error ? (
            <p className="text-red-600">Failed to load league.</p>
          ) : league ? (
            <>
              <h1 className="text-4xl md:text-5xl font-bold text-blue-600 mb-2">{league.name}</h1>
              <p className="text-gray-500 dark:text-gray-400 text-sm">
                {league.platform && <span>Platform: {league.platform} · </span>}
                {league.current_week > 0 && <span>Week {league.current_week} of {league.total_weeks}</span>}
              </p>
            </>
          ) : null}
        </section>

        <section>
          <h2 className="text-2xl font-semibold mb-4">Navigate</h2>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {navItems.map((item) => (
              <Link
                key={item.path}
                href={item.path}
                className="block bg-white dark:bg-gray-800 rounded-lg shadow-md hover:shadow-lg transition-shadow p-6 border border-gray-200 dark:border-gray-700"
              >
                <h3 className="text-lg font-semibold text-blue-600 mb-1">{item.name}</h3>
                <p className="text-sm text-gray-500 dark:text-gray-400">{item.description}</p>
              </Link>
            ))}
          </div>
        </section>

        <section>
          <Link href="/" className="text-blue-600 hover:text-blue-800 dark:hover:text-blue-400 text-sm">
            ← All Leagues
          </Link>
        </section>
      </div>
    </Layout>
  );
}
