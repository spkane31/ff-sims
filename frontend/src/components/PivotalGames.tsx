interface PivotalGame {
  week: number;
  homeTeamId: number;
  awayTeamId: number;
  homeTeamName: string;
  awayTeamName: string;
  totalSwing: number;
  homeTeamWinScenario: {
    homePlayoffOdds: number;
    awayPlayoffOdds: number;
    homeLastPlaceOdds: number;
    awayLastPlaceOdds: number;
  };
  awayTeamWinScenario: {
    homePlayoffOdds: number;
    awayPlayoffOdds: number;
    homeLastPlaceOdds: number;
    awayLastPlaceOdds: number;
  };
  defaultOdds: {
    homePlayoffOdds: number;
    awayPlayoffOdds: number;
    homeLastPlaceOdds: number;
    awayLastPlaceOdds: number;
  };
}

interface PivotalGamesProps {
  games: PivotalGame[];
}

export default function PivotalGames({ games }: PivotalGamesProps) {
  if (games.length === 0) {
    return null;
  }

  const formatOdds = (odds: number) => (odds * 100).toFixed(1);

  const getOddsChange = (newOdds: number, defaultOdds: number) => {
    const change = (newOdds - defaultOdds) * 100;
    return change;
  };

  const getChangeColor = (change: number) => {
    if (Math.abs(change) < 1) return "text-gray-600 dark:text-gray-400";
    return change > 0
      ? "text-green-600 dark:text-green-400"
      : "text-red-600 dark:text-red-400";
  };

  const formatChange = (change: number) => {
    const sign = change > 0 ? "+" : "";
    return `${sign}${change.toFixed(1)}%`;
  };

  return (
    <section className="py-6">
      <div className="bg-gradient-to-r from-orange-50 to-yellow-50 dark:from-orange-900/20 dark:to-yellow-900/20 rounded-lg shadow-md p-6 border-2 border-orange-200 dark:border-orange-700">
        <div className="flex items-center mb-4">
          <span className="text-3xl mr-3">🔥</span>
          <div>
            <h2 className="text-2xl font-bold text-orange-800 dark:text-orange-200">
              Pivotal Games
            </h2>
            <p className="text-sm text-orange-700 dark:text-orange-300">
              The {games.length} most important upcoming matchups that will
              decide playoff and last place races
            </p>
          </div>
        </div>

        <div className="space-y-4">
          {games.map((game, index) => {
            const homePlayoffChangeIfHomeWins = getOddsChange(
              game.homeTeamWinScenario.homePlayoffOdds,
              game.defaultOdds.homePlayoffOdds
            );
            const awayPlayoffChangeIfHomeWins = getOddsChange(
              game.homeTeamWinScenario.awayPlayoffOdds,
              game.defaultOdds.awayPlayoffOdds
            );
            const homePlayoffChangeIfAwayWins = getOddsChange(
              game.awayTeamWinScenario.homePlayoffOdds,
              game.defaultOdds.homePlayoffOdds
            );
            const awayPlayoffChangeIfAwayWins = getOddsChange(
              game.awayTeamWinScenario.awayPlayoffOdds,
              game.defaultOdds.awayPlayoffOdds
            );

            const homeLastPlaceChangeIfHomeWins = getOddsChange(
              game.homeTeamWinScenario.homeLastPlaceOdds,
              game.defaultOdds.homeLastPlaceOdds
            );
            const awayLastPlaceChangeIfHomeWins = getOddsChange(
              game.homeTeamWinScenario.awayLastPlaceOdds,
              game.defaultOdds.awayLastPlaceOdds
            );
            const homeLastPlaceChangeIfAwayWins = getOddsChange(
              game.awayTeamWinScenario.homeLastPlaceOdds,
              game.defaultOdds.homeLastPlaceOdds
            );
            const awayLastPlaceChangeIfAwayWins = getOddsChange(
              game.awayTeamWinScenario.awayLastPlaceOdds,
              game.defaultOdds.awayLastPlaceOdds
            );

            return (
              <div
                key={`${game.week}-${game.homeTeamId}-${game.awayTeamId}`}
                className={`bg-white dark:bg-gray-800 rounded-lg p-4 shadow border-l-4 ${
                  index === 0
                    ? "border-orange-500"
                    : index === 1
                    ? "border-orange-400"
                    : "border-orange-300"
                }`}
              >
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center space-x-2">
                    <span className="text-2xl font-bold text-orange-600 dark:text-orange-400">
                      #{index + 1}
                    </span>
                    <div>
                      <div className="text-sm font-medium text-gray-500 dark:text-gray-400">
                        Week {game.week}
                      </div>
                      <div className="text-xs text-gray-400 dark:text-gray-500">
                        Impact Score: {game.totalSwing.toFixed(3)}
                      </div>
                    </div>
                  </div>
                </div>

                <div className="text-center mb-4">
                  <div className="text-lg font-bold text-gray-900 dark:text-gray-100">
                    {game.homeTeamName} vs {game.awayTeamName}
                  </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  {/* Scenario 1: Home Team Wins */}
                  <div className="bg-green-50 dark:bg-green-900/20 rounded-lg p-3 border border-green-200 dark:border-green-800">
                    <div className="text-sm font-semibold text-green-800 dark:text-green-300 mb-2">
                      If {game.homeTeamName} Wins
                    </div>
                    <div className="space-y-1 text-xs">
                      <div className="flex justify-between items-center">
                        <span className="text-gray-700 dark:text-gray-300">
                          {game.homeTeamName} Playoff:
                        </span>
                        <span
                          className={`font-medium ${getChangeColor(
                            homePlayoffChangeIfHomeWins
                          )}`}
                        >
                          {formatOdds(game.defaultOdds.homePlayoffOdds)}%{" "}
                          {formatChange(homePlayoffChangeIfHomeWins)}
                        </span>
                      </div>
                      <div className="flex justify-between items-center">
                        <span className="text-gray-700 dark:text-gray-300">
                          {game.awayTeamName} Playoff:
                        </span>
                        <span
                          className={`font-medium ${getChangeColor(
                            awayPlayoffChangeIfHomeWins
                          )}`}
                        >
                          {formatOdds(game.defaultOdds.awayPlayoffOdds)}%{" "}
                          {formatChange(awayPlayoffChangeIfHomeWins)}
                        </span>
                      </div>
                      <div className="flex justify-between items-center">
                        <span className="text-gray-700 dark:text-gray-300">
                          {game.homeTeamName} Last Place:
                        </span>
                        <span
                          className={`font-medium ${getChangeColor(
                            -homeLastPlaceChangeIfHomeWins
                          )}`}
                        >
                          {formatOdds(game.defaultOdds.homeLastPlaceOdds)}%{" "}
                          {formatChange(homeLastPlaceChangeIfHomeWins)}
                        </span>
                      </div>
                      <div className="flex justify-between items-center">
                        <span className="text-gray-700 dark:text-gray-300">
                          {game.awayTeamName} Last Place:
                        </span>
                        <span
                          className={`font-medium ${getChangeColor(
                            -awayLastPlaceChangeIfHomeWins
                          )}`}
                        >
                          {formatOdds(game.defaultOdds.awayLastPlaceOdds)}%{" "}
                          {formatChange(awayLastPlaceChangeIfHomeWins)}
                        </span>
                      </div>
                    </div>
                  </div>

                  {/* Scenario 2: Away Team Wins */}
                  <div className="bg-blue-50 dark:bg-blue-900/20 rounded-lg p-3 border border-blue-200 dark:border-blue-800">
                    <div className="text-sm font-semibold text-blue-800 dark:text-blue-300 mb-2">
                      If {game.awayTeamName} Wins
                    </div>
                    <div className="space-y-1 text-xs">
                      <div className="flex justify-between items-center">
                        <span className="text-gray-700 dark:text-gray-300">
                          {game.homeTeamName} Playoff:
                        </span>
                        <span
                          className={`font-medium ${getChangeColor(
                            homePlayoffChangeIfAwayWins
                          )}`}
                        >
                          {formatOdds(game.defaultOdds.homePlayoffOdds)}%{" "}
                          {formatChange(homePlayoffChangeIfAwayWins)}
                        </span>
                      </div>
                      <div className="flex justify-between items-center">
                        <span className="text-gray-700 dark:text-gray-300">
                          {game.awayTeamName} Playoff:
                        </span>
                        <span
                          className={`font-medium ${getChangeColor(
                            awayPlayoffChangeIfAwayWins
                          )}`}
                        >
                          {formatOdds(game.defaultOdds.awayPlayoffOdds)}%{" "}
                          {formatChange(awayPlayoffChangeIfAwayWins)}
                        </span>
                      </div>
                      <div className="flex justify-between items-center">
                        <span className="text-gray-700 dark:text-gray-300">
                          {game.homeTeamName} Last Place:
                        </span>
                        <span
                          className={`font-medium ${getChangeColor(
                            -homeLastPlaceChangeIfAwayWins
                          )}`}
                        >
                          {formatOdds(game.defaultOdds.homeLastPlaceOdds)}%{" "}
                          {formatChange(homeLastPlaceChangeIfAwayWins)}
                        </span>
                      </div>
                      <div className="flex justify-between items-center">
                        <span className="text-gray-700 dark:text-gray-300">
                          {game.awayTeamName} Last Place:
                        </span>
                        <span
                          className={`font-medium ${getChangeColor(
                            -awayLastPlaceChangeIfAwayWins
                          )}`}
                        >
                          {formatOdds(game.defaultOdds.awayLastPlaceOdds)}%{" "}
                          {formatChange(awayLastPlaceChangeIfAwayWins)}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}
