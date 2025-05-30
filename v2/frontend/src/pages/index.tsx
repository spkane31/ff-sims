import { useEffect, useState } from "react";
import Layout from "../components/Layout";
import Link from "next/link";

export default function Home() {
  const [teamsData, setTeamsData] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    async function fetchTeamsData() {
      try {
        setIsLoading(true);
        const response = await fetch("http://localhost:8080/api/teams");
        const data = await response.text();
        setTeamsData(data);
      } catch (error) {
        console.error("Error fetching teams data:", error);
        setTeamsData("Failed to fetch teams data.");
      } finally {
        setIsLoading(false);
      }
    }

    fetchTeamsData();
  }, []);

  return (
    <Layout>
      <div className="space-y-8">
        {/* Hero Section */}
        <section className="text-center md:text-left">
          <h1 className="text-4xl md:text-5xl font-bold text-blue-600 mb-4">
            Fantasy Football Simulations
          </h1>
          <p className="text-lg md:text-xl text-gray-600 dark:text-gray-300 max-w-3xl mb-8">
            Analyze, simulate, and optimize your fantasy football strategy with
            data-driven insights.
          </p>
          <div className="flex flex-wrap gap-4 justify-center md:justify-start">
            <Link
              href="/simulations"
              className="bg-blue-600 hover:bg-blue-700 text-white px-6 py-3 rounded-md font-medium transition-colors"
            >
              Run Simulation
            </Link>
            <Link
              href="/teams"
              className="border border-blue-600 text-blue-600 hover:bg-blue-50 dark:hover:bg-blue-900/20 px-6 py-3 rounded-md font-medium transition-colors"
            >
              View Teams
            </Link>
          </div>
        </section>

        {/* Features Section */}
        <section className="py-6">
          <h2 className="text-2xl font-semibold mb-6">Features</h2>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            {[
              {
                title: "Team Simulations",
                description: "Simulate matchups and project season outcomes.",
                link: "/simulations",
              },
              {
                title: "Schedule Analysis",
                description: "Analyze strength of schedule and key matchups.",
                link: "/schedule",
              },
              {
                title: "Team Management",
                description: "View and compare team rosters and performance.",
                link: "/teams",
              },
            ].map((feature, index) => (
              <div
                key={index}
                className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md hover:shadow-lg transition-shadow"
              >
                <h3 className="text-xl font-medium text-blue-600 mb-2">
                  {feature.title}
                </h3>
                <p className="mb-4 text-gray-600 dark:text-gray-300">
                  {feature.description}
                </p>
                <Link
                  href={feature.link}
                  className="text-blue-600 hover:text-blue-800 dark:hover:text-blue-400 font-medium"
                >
                  Learn more →
                </Link>
              </div>
            ))}
          </div>
        </section>

        {/* API Status Section */}
        <section className="bg-gray-100 dark:bg-gray-700 rounded-lg p-6">
          <h2 className="text-xl font-semibold mb-4">API Status</h2>
          {isLoading ? (
            <div className="flex items-center space-x-2">
              <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
              <p>Loading API status...</p>
            </div>
          ) : (
            <div>
              <p className="text-gray-600 dark:text-gray-300 mb-2">
                Teams API Response:
              </p>
              <pre className="bg-gray-200 dark:bg-gray-800 p-4 rounded-md overflow-x-auto text-sm">
                {teamsData}
              </pre>
            </div>
          )}
        </section>
      </div>
    </Layout>
  );
}