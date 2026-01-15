import { useEffect, useState } from "react";
import { useRouter } from "next/router";
import Layout from "../components/Layout";
import { leaguesService } from "../services/leaguesService";
import { League } from "../types/models";

export default function LeagueSelector() {
  const router = useRouter();
  const [leagues, setLeagues] = useState<League[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function fetchLeagues() {
      try {
        setLoading(true);
        const response = await leaguesService.getAllLeagues();
        setLeagues(response.leagues || []);
      } catch (err) {
        console.error("Error fetching leagues:", err);
        setError("Failed to load leagues");
      } finally {
        setLoading(false);
      }
    }

    fetchLeagues();
  }, []);

  const handleLeagueClick = (leagueId: string) => {
    router.push(`/league/${leagueId}`);
  };

  return (
    <Layout>
      <div className="container mx-auto px-4 py-8">
        <div className="max-w-4xl mx-auto">
          <h1 className="text-4xl font-bold text-gray-900 mb-2">
            Fantasy Football Leagues
          </h1>
          <p className="text-gray-600 mb-8">
            Select a league to view standings, schedules, and simulations
          </p>

          {loading && (
            <div className="flex justify-center items-center py-12">
              <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
            </div>
          )}

          {error && (
            <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded mb-4">
              {error}
            </div>
          )}

          {!loading && !error && leagues.length === 0 && (
            <div className="bg-gray-50 border border-gray-200 text-gray-700 px-4 py-3 rounded">
              No leagues found
            </div>
          )}

          {!loading && !error && leagues.length > 0 && (
            <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
              {leagues.map((league) => (
                <button
                  key={league.id}
                  onClick={() => handleLeagueClick(String(league.id))}
                  className="bg-white border border-gray-200 rounded-lg p-6 hover:shadow-lg hover:border-blue-500 transition-all duration-200 text-left"
                >
                  <h2 className="text-xl font-semibold text-gray-900 mb-2">
                    {league.name}
                  </h2>
                  <p className="text-sm text-gray-500">League ID: {league.id}</p>
                  <div className="mt-4 text-blue-600 font-medium flex items-center">
                    View League
                    <svg
                      className="w-4 h-4 ml-2"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M9 5l7 7-7 7"
                      />
                    </svg>
                  </div>
                </button>
              ))}
            </div>
          )}
        </div>
      </div>
    </Layout>
  );
}
