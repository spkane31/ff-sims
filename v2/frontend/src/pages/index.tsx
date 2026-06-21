import Layout from "../components/Layout";
import Link from "next/link";
import { useLeagues } from "../hooks/useLeagues";

export default function Home() {
  const { leagues, isLoading, error } = useLeagues();

  return (
    <Layout>
      <div className="space-y-8">
        <section className="text-center md:text-left">
          <h1 className="text-4xl md:text-5xl font-bold text-blue-600 mb-4">
            Fantasy Football Simulations
          </h1>
          <p className="text-lg text-gray-600 dark:text-gray-300 max-w-3xl">
            Select a league to view standings, schedule, simulations, and more.
          </p>
        </section>

        <section>
          <h2 className="text-2xl font-semibold mb-6">Your Leagues</h2>
          {isLoading && (
            <div className="flex items-center space-x-2">
              <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin" />
              <p>Loading leagues...</p>
            </div>
          )}
          {error && (
            <p className="text-red-600">Failed to load leagues.</p>
          )}
          {!isLoading && !error && leagues.length === 0 && (
            <p className="text-gray-500">No leagues found.</p>
          )}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {leagues.map((league) => (
              <Link
                key={league.id}
                href={`/league/${league.id}`}
                className="block bg-white dark:bg-gray-800 rounded-lg shadow-md hover:shadow-lg transition-shadow p-6 border border-gray-200 dark:border-gray-700"
              >
                <h3 className="text-xl font-semibold text-blue-600 mb-2">{league.name}</h3>
                <div className="text-sm text-gray-500 dark:text-gray-400 space-y-1">
                  <p>Platform: {league.platform || "—"}</p>
                  {league.current_week > 0 && (
                    <p>Week {league.current_week} of {league.total_weeks}</p>
                  )}
                </div>
                <p className="mt-4 text-blue-600 font-medium text-sm">Open league →</p>
              </Link>
            ))}
          </div>
        </section>
      </div>
    </Layout>
  );
}
