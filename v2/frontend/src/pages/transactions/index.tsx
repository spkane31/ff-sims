import { useState } from "react";
import Layout from "../../components/Layout";
import { useTransactions } from "@/hooks/useTransactions";

// Transaction type is imported from the useTransactions hook

export default function Transactions() {
  const { transactions, isLoading, error } = useTransactions();

  // const [transactions, setTransactions] = useState<Transaction[]>([
  //   {
  //     id: '1',
  //     date: 'Aug 25, 2025',
  //     type: 'DRAFT',
  //     description: 'Team A drafted Cooper Kupp with the 8th overall pick.',
  //     teams: ['Team A'],
  //     players: [
  //       { name: 'Cooper Kupp', position: 'WR', team: 'LAR', points: 256 }
  //     ]
  //   },
  // ]);
  // const [isLoading, setIsLoading] = useState(true);
  // const [error, setError] = useState<Error | null>(null);
  const [activeTab, setActiveTab] = useState<'all' | 'draft' | 'waiver' | 'trade'>('all');
  const [selectedYear, setSelectedYear] = useState<string>('2025');

  // useEffect(() => {
  //   async function fetchTransactions() {
  //     try {
  //       setIsLoading(true);
  //       // In a real implementation, this would fetch from your API
  //       const response = await fetch("http://localhost:8080/api/transactions");
  //       const data = await response.json();
  //       setTransactions(data || mockTransactions);
  //     } catch (error) {
  //       console.error("Error fetching transaction data:", error);
  //       setError(error instanceof Error ? error : new Error("Failed to fetch transactions"));
  //       // Using mock data for display purposes
  //       setTransactions(mockTransactions);
  //     } finally {
  //       setIsLoading(false);
  //     }
  //   }
  //   setIsLoading(false);
  //   // fetchTransactions();
  // }, []);

  const filteredTransactions = transactions.filter(transaction => {
    if (activeTab === 'all') return true;
    return transaction.type.toLowerCase() === activeTab;
  });

  return (
    <Layout>
      <div className="space-y-8">
        <section>
          <h1 className="text-3xl md:text-4xl font-bold text-blue-600 mb-6">
            League Transactions
          </h1>
          <p className="text-lg text-gray-600 dark:text-gray-300 mb-8 max-w-3xl">
            Track and analyze all league transactions including draft picks, waiver wire selections, and trades throughout the season.
          </p>

          {/* Filters */}
          <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md mb-8">
            <div className="flex flex-col md:flex-row justify-between items-start md:items-center gap-4 mb-6">
              <h2 className="text-xl font-semibold">Filters</h2>

              <div className="flex flex-wrap gap-4">
                <div>
                  <label htmlFor="yearFilter" className="block text-sm font-medium mb-1">
                    Season
                  </label>
                  <select
                    id="yearFilter"
                    value={selectedYear}
                    onChange={(e) => setSelectedYear(e.target.value)}
                    className="w-full px-3 py-2 border dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                  >
                    <option value="2025">2025</option>
                    <option value="2024">2024</option>
                    <option value="2023">2023</option>
                  </select>
                </div>
              </div>
            </div>

            <div className="border-b border-gray-200 dark:border-gray-600">
              <nav className="flex flex-wrap -mb-px">
                {[
                  { id: 'all', name: 'All Transactions' },
                  { id: 'draft', name: 'Draft Picks' },
                  { id: 'waiver', name: 'Waiver Wire' },
                  { id: 'trade', name: 'Trades' }
                ].map((tab) => (
                  <button
                    key={tab.id}
                    onClick={() => setActiveTab(tab.id as 'all' | 'draft' | 'waiver' | 'trade')}
                    className={`py-3 px-4 border-b-2 font-medium text-sm ${
                      activeTab === tab.id
                        ? 'border-blue-600 text-blue-600'
                        : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
                    }`}
                  >
                    {tab.name}
                  </button>
                ))}
              </nav>
            </div>
          </div>

          {/* Transactions List */}
          <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
            <h2 className="text-xl font-semibold mb-6">
              {activeTab === 'all' ? 'All Transactions' :
               activeTab === 'draft' ? 'Draft Picks' :
               activeTab === 'waiver' ? 'Waiver Wire Selections' : 'Trades'}
            </h2>

            {isLoading ? (
              <div className="flex items-center justify-center h-40">
                <div className="w-6 h-6 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                <span className="ml-2">Loading transactions...</span>
              </div>
            ) : error ? (
              <div className="bg-red-100 dark:bg-red-900 p-4 rounded-lg text-red-700 dark:text-red-200">
                <h3 className="text-lg font-semibold">Error loading transactions</h3>
                <p>{error.message}</p>
              </div>
            ) : filteredTransactions.length === 0 ? (
              <div className="text-center py-10 text-gray-500 dark:text-gray-400">
                <p className="mb-2">No transactions found</p>
                <p className="text-sm">Try changing your filters to see more results</p>
              </div>
            ) : (
              <div className="space-y-6">
                {filteredTransactions.map((transaction) => (
                  <div key={transaction.id} className="border border-gray-200 dark:border-gray-600 rounded-lg overflow-hidden">
                    <div className="bg-gray-50 dark:bg-gray-800 px-4 py-3 flex justify-between items-center">
                      <div>
                        <span className={`px-2 py-1 text-xs rounded-full ${
                          transaction.type === 'draft' ? 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200' :
                          transaction.type === 'waiver' ? 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200' :
                          'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200'
                        }`}>
                          {transaction.type}
                        </span>
                        <span className="ml-3 text-gray-600 dark:text-gray-400 text-sm">{transaction.date}</span>
                      </div>
                      <div className="text-sm">
                        {transaction.teams.join(' | ')}
                      </div>
                    </div>
                    <div className="p-4">
                      <p className="mb-4">{transaction.description}</p>

                      <h4 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">Players Involved:</h4>
                      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                        {transaction.players.map((player, idx) => (
                          <div key={idx} className="bg-gray-50 dark:bg-gray-800 p-3 rounded-md">
                            <div className="font-medium">{player.name}</div>
                            <div className="text-sm text-gray-500 dark:text-gray-400">
                              {player.position} | {player.team}
                              {player.points !== undefined && (
                                <span className="ml-2 font-medium text-blue-600">{player.points} pts</span>
                              )}
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </section>

        {/* Statistics Section */}
        <section className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
            <h2 className="text-lg font-semibold mb-4">Transaction Insights</h2>
            <div className="space-y-4">
              <div>
                <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">
                  Most Active Teams
                </h3>
                <div className="space-y-2">
                  {['Team A', 'Team B', 'Team C'].map((team, i) => (
                    <div key={i} className="flex justify-between items-center">
                      <span className="font-medium">{team}</span>
                      <span>{12 - i * 3} transactions</span>
                    </div>
                  ))}
                </div>
              </div>

              <div className="pt-4 border-t border-gray-200 dark:border-gray-600">
                <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">
                  Most Traded Players
                </h3>
                <div className="space-y-2">
                  {['Player X', 'Player Y', 'Player Z'].map((player, i) => (
                    <div key={i} className="flex justify-between items-center">
                      <span className="font-medium">{player}</span>
                      <span>{3 - i} trades</span>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>

          <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
            <h2 className="text-lg font-semibold mb-4">Value Summary</h2>
            <div className="space-y-4">
              <div>
                <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">
                  Best Draft Picks
                </h3>
                <div className="space-y-2">
                  {['QB A - Round 8', 'RB B - Round 10', 'WR C - Round 9'].map((pick, i) => (
                    <div key={i} className="flex justify-between items-center">
                      <span className="font-medium">{pick}</span>
                      <span className="text-green-600">+{(5 - i) * 35} pts over projection</span>
                    </div>
                  ))}
                </div>
              </div>

              <div className="pt-4 border-t border-gray-200 dark:border-gray-600">
                <h3 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">
                  Best Waiver Pickups
                </h3>
                <div className="space-y-2">
                  {['WR D - Week 3', 'RB E - Week 5', 'TE F - Week 2'].map((pickup, i) => (
                    <div key={i} className="flex justify-between items-center">
                      <span className="font-medium">{pickup}</span>
                      <span className="text-green-600">{(8 - i) * 12} total points</span>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>
        </section>
      </div>
    </Layout>
  );
}

