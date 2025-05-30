import { useState } from "react";
import Layout from "../../components/Layout";

export default function Simulations() {
  const [simulating, setSimulating] = useState(false);
  const [results, setResults] = useState<string | null>(null);

  const handleSimulation = () => {
    setSimulating(true);
    // Simulate API call
    setTimeout(() => {
      setResults("Simulation completed with 1000 iterations.");
      setSimulating(false);
    }, 2000);
  };

  return (
    <Layout>
      <div className="space-y-8">
        <section>
          <h1 className="text-3xl md:text-4xl font-bold text-blue-600 mb-6">
            Run Fantasy Football Simulations
          </h1>
          <p className="text-lg text-gray-600 dark:text-gray-300 mb-8 max-w-3xl">
            Simulate the rest of your fantasy season to see projections for final standings,
            playoff odds, and championship probabilities.
          </p>

          <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
            <h2 className="text-xl font-semibold mb-4">Simulation Parameters</h2>

            <div className="space-y-4 mb-6">
              <div>
                <label htmlFor="iterations" className="block text-sm font-medium mb-1">
                  Number of Iterations
                </label>
                <input
                  type="number"
                  id="iterations"
                  defaultValue={1000}
                  className="w-full md:w-64 px-3 py-2 border dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                />
              </div>

              <div>
                <label htmlFor="startWeek" className="block text-sm font-medium mb-1">
                  Start Week
                </label>
                <select
                  id="startWeek"
                  className="w-full md:w-64 px-3 py-2 border dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                >
                  <option value="current">Current (Week 10)</option>
                  <option value="11">Week 11</option>
                  <option value="12">Week 12</option>
                  <option value="13">Week 13</option>
                </select>
              </div>

              <div className="flex items-center">
                <input
                  type="checkbox"
                  id="useActualResults"
                  className="h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
                  defaultChecked
                />
                <label htmlFor="useActualResults" className="ml-2 block text-sm">
                  Use actual results for completed weeks
                </label>
              </div>
            </div>

            <button
              onClick={handleSimulation}
              disabled={simulating}
              className={`${
                simulating
                  ? "bg-blue-400 cursor-not-allowed"
                  : "bg-blue-600 hover:bg-blue-700"
              } text-white px-6 py-2 rounded-md font-medium transition-colors flex items-center`}
            >
              {simulating ? (
                <>
                  <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin mr-2"></div>
                  Simulating...
                </>
              ) : (
                "Run Simulation"
              )}
            </button>
          </div>
        </section>

        {results && (
          <section className="bg-gray-100 dark:bg-gray-700 p-6 rounded-lg">
            <h2 className="text-xl font-semibold mb-4">Simulation Results</h2>

            <div className="space-y-6">
              <p className="text-green-600 font-medium">{results}</p>

              <div>
                <h3 className="text-lg font-medium mb-2">Playoff Odds</h3>
                <div className="bg-white dark:bg-gray-800 p-4 rounded-md shadow-sm">
                  <p className="text-sm text-gray-500 mb-4">Probability of making playoffs</p>

                  <div className="space-y-3">
                    {["Team A", "Team B", "Team C", "Team D"].map((team, i) => (
                      <div key={team} className="flex items-center">
                        <span className="w-20 text-sm">{team}</span>
                        <div className="flex-1 h-5 bg-gray-200 dark:bg-gray-600 rounded-full overflow-hidden">
                          <div
                            className="h-full bg-blue-600"
                            style={{ width: `${90 - i * 20}%` }}
                          ></div>
                        </div>
                        <span className="w-14 text-right text-sm">{90 - i * 20}%</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>

              <div>
                <h3 className="text-lg font-medium mb-2">Projected Final Standings</h3>
                <div className="overflow-x-auto">
                  <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
                    <thead className="bg-gray-50 dark:bg-gray-800">
                      <tr>
                        <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Team</th>
                        <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Wins</th>
                        <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Losses</th>
                        <th className="py-3 px-4 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Points For</th>
                      </tr>
                    </thead>
                    <tbody className="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
                      {["Team A", "Team B", "Team C", "Team D"].map((team, i) => (
                        <tr key={team} className={i % 2 === 0 ? "bg-white dark:bg-gray-800" : "bg-gray-50 dark:bg-gray-700"}>
                          <td className="py-2 px-4 whitespace-nowrap">{team}</td>
                          <td className="py-2 px-4 whitespace-nowrap">{12 - i}</td>
                          <td className="py-2 px-4 whitespace-nowrap">{2 + i}</td>
                          <td className="py-2 px-4 whitespace-nowrap">{1800 - i * 75}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          </section>
        )}
      </div>
    </Layout>
  );
}