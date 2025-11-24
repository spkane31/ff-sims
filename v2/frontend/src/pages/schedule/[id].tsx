import { useRouter } from "next/router";
import Layout from "../../components/Layout";
import { useMatchupDetail } from "../../hooks/useMatchupDetail";
import Link from "next/link";
import { Player } from "../../services/scheduleService";

// Helper function to find better lineup decisions
function findBetterLineupDecisions(players: Player[]) {
  const starters = players.filter(
    (p) =>
      p.slotPosition !== "BE" &&
      p.slotPosition !== "IR" &&
      p.slotPosition !== ""
  );
  const benchPlayers = players.filter(
    (p) => p.slotPosition === "BE" || p.slotPosition === "IR"
  );

  const betterDecisions: Array<{
    benchPlayer: Player;
    starterPlayer: Player;
    pointsGained: number;
  }> = [];

  benchPlayers.forEach((benchPlayer) => {
    // Find starters with matching positions who scored lower
    const eligibleStarters = starters.filter((starter) => {
      // Direct position match
      if (benchPlayer.playerPosition === starter.playerPosition) {
        return true;
      }

      // FLEX eligibility - RB/WR/TE can be slotted into FLEX
      if (
        starter.slotPosition === "RB/WR/TE" &&
        (benchPlayer.playerPosition === "RB" ||
          benchPlayer.playerPosition === "WR" ||
          benchPlayer.playerPosition === "TE")
      ) {
        return true;
      }

      return false;
    });

    // Find the lowest scoring eligible starter
    const worstStarter = eligibleStarters.sort(
      (a, b) => a.points - b.points
    )[0];

    if (worstStarter && benchPlayer.points > worstStarter.points) {
      betterDecisions.push({
        benchPlayer,
        starterPlayer: worstStarter,
        pointsGained: benchPlayer.points - worstStarter.points,
      });
    }
  });

  return betterDecisions.sort((a, b) => b.pointsGained - a.pointsGained);
}

export default function MatchupDetail() {
  const router = useRouter();
  const { id } = router.query;
  const { matchup, isLoading, error } = useMatchupDetail(id as string);

  if (isLoading || matchup === null) {
    return (
      <Layout>
        <div className="flex items-center justify-center min-h-screen">
          <div className="w-8 h-8 border-4 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
          <span className="ml-2 text-lg">Loading matchup details...</span>
        </div>
      </Layout>
    );
  }

  if (error) {
    return (
      <Layout>
        <div className="bg-red-100 dark:bg-red-900 p-6 rounded-lg text-red-700 dark:text-red-200 max-w-4xl mx-auto my-8">
          <h2 className="text-xl font-semibold mb-2">
            Error loading matchup details
          </h2>
          <p>{error.message}</p>
          <Link
            href="/schedule"
            className="mt-4 inline-block px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700"
          >
            Return to Schedule
          </Link>
        </div>
      </Layout>
    );
  }

  const matchupData = matchup;

  const { data } = matchupData;
  const { homeTeam, awayTeam, year, week, homeTeamESPNID, awayTeamESPNID } =
    data;

  // Calculate winner
  const homeWon = homeTeam.score > awayTeam.score;
  const awayWon = awayTeam.score > homeTeam.score;

  // Calculate if game is completed
  const isCompleted = homeTeam.score > 0 || awayTeam.score > 0;

  // Calculate projected vs actual differential
  const homeProjectedDiff = homeTeam.score - homeTeam.projectedScore;
  const awayProjectedDiff = awayTeam.score - awayTeam.projectedScore;

  return (
    <Layout>
      <div className="max-w-7xl mx-auto px-4 py-8">
        {/* Breadcrumbs and title */}
        <div className="mb-6">
          <div className="flex items-center text-gray-500 dark:text-gray-400 mb-2">
            <Link href="/schedule" className="hover:text-blue-600">
              Schedule
            </Link>
            <span className="mx-2">›</span>
            <span>
              Year {year}, Week {week}
            </span>
          </div>
          <h1 className="text-3xl md:text-4xl font-bold text-blue-600 mb-2">
            {homeTeam.name} vs {awayTeam.name}
          </h1>
          <div className="text-lg text-gray-600 dark:text-gray-400">
            Week {week}, Year {year}
          </div>
        </div>

        {/* Matchup summary card */}
        <div className="bg-white dark:bg-gray-800 shadow-md rounded-lg p-6 mb-8">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-8 items-center">
            {/* Home Team */}
            <div
              className={`text-center ${
                homeWon ? "bg-green-50 dark:bg-green-900/20 p-4 rounded-lg" : ""
              }`}
            >
              <div className="text-xl font-semibold mb-1">
                <Link
                  href={`/teams/${homeTeamESPNID}`}
                  className="hover:text-blue-600 transition-colors"
                >
                  {homeTeam.name}
                </Link>
              </div>
              <div
                className={`text-4xl font-bold mb-2 ${
                  homeWon ? "text-green-600 dark:text-green-400" : ""
                }`}
              >
                {homeTeam.score.toFixed(1)}
              </div>
              <div className="text-sm text-gray-500 dark:text-gray-400">
                Projected: {homeTeam.projectedScore.toFixed(1)}
                {homeProjectedDiff !== 0 && (
                  <span
                    className={`ml-2 ${
                      homeProjectedDiff > 0 ? "text-green-600" : "text-red-600"
                    }`}
                  >
                    ({homeProjectedDiff > 0 ? "+" : ""}
                    {homeProjectedDiff.toFixed(1)})
                  </span>
                )}
              </div>
            </div>

            {/* Matchup Status */}
            <div className="flex flex-col items-center justify-center text-center">
              <div className="text-lg mb-2">
                {isCompleted ? (
                  <span className="px-3 py-1 bg-green-100 dark:bg-green-800 text-green-800 dark:text-green-100 rounded-full text-sm font-medium">
                    Final
                  </span>
                ) : (
                  <span className="px-3 py-1 bg-blue-100 dark:bg-blue-800 text-blue-800 dark:text-blue-100 rounded-full text-sm font-medium">
                    Upcoming
                  </span>
                )}
              </div>

              <div className="text-2xl font-bold my-2">vs</div>

              {isCompleted && (
                <div className="text-sm text-gray-500 dark:text-gray-400 mb-2">
                  Point Differential:{" "}
                  <span className="font-medium">
                    {(homeTeam.score - awayTeam.score).toFixed(2)}
                  </span>
                </div>
              )}
            </div>

            {/* Away Team */}
            <div
              className={`text-center ${
                awayWon ? "bg-green-50 dark:bg-green-900/20 p-4 rounded-lg" : ""
              }`}
            >
              <div className="text-xl font-semibold mb-1">
                <Link
                  href={`/teams/${awayTeamESPNID}`}
                  className="hover:text-blue-600 transition-colors"
                >
                  {awayTeam.name}
                </Link>
              </div>
              <div
                className={`text-4xl font-bold mb-2 ${
                  awayWon ? "text-green-600 dark:text-green-400" : ""
                }`}
              >
                {awayTeam.score.toFixed(1)}
              </div>
              <div className="text-sm text-gray-500 dark:text-gray-400">
                Projected: {awayTeam.projectedScore.toFixed(1)}
                {awayProjectedDiff !== 0 && (
                  <span
                    className={`ml-2 ${
                      awayProjectedDiff > 0 ? "text-green-600" : "text-red-600"
                    }`}
                  >
                    ({awayProjectedDiff > 0 ? "+" : ""}
                    {awayProjectedDiff.toFixed(1)})
                  </span>
                )}
              </div>
            </div>
          </div>
        </div>

        {/* Team Lineups */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-8 mb-8">
          {/* Home Team Lineup */}
          <div className="bg-white dark:bg-gray-800 shadow-md rounded-lg p-6">
            <h2 className="text-xl font-semibold mb-4">
              {homeTeam.name} Lineup
            </h2>

            <div className="overflow-x-auto">
              <table className="min-w-full">
                <thead>
                  <tr className="border-b border-gray-200 dark:border-gray-700">
                    <th className="py-2 px-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                      Player
                    </th>
                    <th className="py-2 px-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                      NFL Pos
                    </th>
                    <th className="py-2 px-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                      Proj
                    </th>
                    <th className="py-2 px-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                      Actual
                    </th>
                    <th className="py-2 px-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                      Diff
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                  {homeTeam.players.map((player) => {
                    const diff = player.points - player.projectedPoints;
                    return (
                      <tr
                        key={player.id}
                        className={`${
                          player.slotPosition === "BE" ||
                          player.slotPosition === "IR"
                            ? "text-gray-500 dark:text-gray-400 bg-gray-50 dark:bg-gray-700/50"
                            : ""
                        }`}
                      >
                        <td className="py-2 px-3">
                          <div className="flex items-center">
                            <span className="inline-block w-8 mr-2 text-xs font-medium text-gray-500">
                              {player.slotPosition || player.playerPosition}
                            </span>
                            <Link
                              href={`/players/${player.id}`}
                              className="hover:text-blue-600 transition-colors"
                            >
                              {player.playerName}
                            </Link>
                          </div>
                        </td>
                        <td className="py-2 px-3">{player.playerPosition}</td>
                        <td className="py-2 px-3 text-right">
                          {player.projectedPoints.toFixed(1)}
                        </td>
                        <td className="py-2 px-3 text-right">
                          {player.points.toFixed(1)}
                        </td>
                        <td
                          className={`py-2 px-3 text-right ${
                            diff > 0
                              ? "text-green-600 dark:text-green-400"
                              : diff < 0
                              ? "text-red-600 dark:text-red-400"
                              : ""
                          }`}
                        >
                          {diff > 0 && "+"}
                          {diff.toFixed(1)}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
                <tfoot>
                  <tr className="border-t border-gray-300 dark:border-gray-600 font-semibold">
                    <td colSpan={2} className="py-2 px-3 text-left">
                      Total
                    </td>
                    <td className="py-2 px-3 text-right">
                      {homeTeam.projectedScore.toFixed(1)}
                    </td>
                    <td className="py-2 px-3 text-right">
                      {homeTeam.score.toFixed(1)}
                    </td>
                    <td
                      className={`py-2 px-3 text-right ${
                        homeProjectedDiff > 0
                          ? "text-green-600 dark:text-green-400"
                          : homeProjectedDiff < 0
                          ? "text-red-600 dark:text-red-400"
                          : ""
                      }`}
                    >
                      {homeProjectedDiff > 0 && "+"}
                      {homeProjectedDiff.toFixed(1)}
                    </td>
                  </tr>
                </tfoot>
              </table>
            </div>
          </div>

          {/* Away Team Lineup */}
          <div className="bg-white dark:bg-gray-800 shadow-md rounded-lg p-6">
            <h2 className="text-xl font-semibold mb-4">
              {awayTeam.name} Lineup
            </h2>

            <div className="overflow-x-auto">
              <table className="min-w-full">
                <thead>
                  <tr className="border-b border-gray-200 dark:border-gray-700">
                    <th className="py-2 px-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                      Player
                    </th>
                    <th className="py-2 px-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                      NFL Pos
                    </th>
                    <th className="py-2 px-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                      Proj
                    </th>
                    <th className="py-2 px-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                      Actual
                    </th>
                    <th className="py-2 px-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                      Diff
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                  {awayTeam.players.map((player) => {
                    const diff = player.points - player.projectedPoints;
                    return (
                      <tr
                        key={player.id}
                        className={`${
                          player.slotPosition === "BE" ||
                          player.slotPosition === "IR"
                            ? "text-gray-500 dark:text-gray-400 bg-gray-50 dark:bg-gray-700/50"
                            : ""
                        }`}
                      >
                        <td className="py-2 px-3">
                          <div className="flex items-center">
                            <span className="inline-block w-8 mr-2 text-xs font-medium text-gray-500">
                              {player.slotPosition || player.playerPosition}
                            </span>
                            <Link
                              href={`/players/${player.id}`}
                              className="hover:text-blue-600 transition-colors"
                            >
                              {player.playerName}
                            </Link>
                          </div>
                        </td>
                        <td className="py-2 px-3">{player.playerPosition}</td>
                        <td className="py-2 px-3 text-right">
                          {player.projectedPoints.toFixed(1)}
                        </td>
                        <td className="py-2 px-3 text-right">
                          {player.points.toFixed(1)}
                        </td>
                        <td
                          className={`py-2 px-3 text-right ${
                            diff > 0
                              ? "text-green-600 dark:text-green-400"
                              : diff < 0
                              ? "text-red-600 dark:text-red-400"
                              : ""
                          }`}
                        >
                          {diff > 0 && "+"}
                          {diff.toFixed(1)}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
                <tfoot>
                  <tr className="border-t border-gray-300 dark:border-gray-600 font-semibold">
                    <td colSpan={2} className="py-2 px-3 text-left">
                      Total
                    </td>
                    <td className="py-2 px-3 text-right">
                      {awayTeam.projectedScore.toFixed(1)}
                    </td>
                    <td className="py-2 px-3 text-right">
                      {awayTeam.score.toFixed(1)}
                    </td>
                    <td
                      className={`py-2 px-3 text-right ${
                        awayProjectedDiff > 0
                          ? "text-green-600 dark:text-green-400"
                          : awayProjectedDiff < 0
                          ? "text-red-600 dark:text-red-400"
                          : ""
                      }`}
                    >
                      {awayProjectedDiff > 0 && "+"}
                      {awayProjectedDiff.toFixed(1)}
                    </td>
                  </tr>
                </tfoot>
              </table>
            </div>
          </div>
        </div>

        {/* Key Performances */}
        <div className="bg-white dark:bg-gray-800 shadow-md rounded-lg p-6 mb-8">
          <h2 className="text-xl font-semibold mb-4 pb-2 border-b border-gray-200 dark:border-gray-700">
            Key Performances
          </h2>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            {/* Best Performers */}
            <div>
              <h3 className="text-lg font-medium mb-3">Top Performers</h3>
              <ul className="space-y-3">
                {[...homeTeam.players, ...awayTeam.players]
                  .filter(
                    (p) =>
                      p.slotPosition !== "BE" &&
                      p.slotPosition !== "IR" &&
                      p.slotPosition !== ""
                  )
                  .sort((a, b) => b.points - a.points)
                  .slice(0, 3)
                  .map((player) => (
                    <li
                      key={player.id}
                      className="flex items-center justify-between bg-gray-50 dark:bg-gray-700 p-3 rounded-lg"
                    >
                      <div>
                        <div className="font-medium">
                          <Link
                            href={`/players/${player.id}`}
                            className="hover:text-blue-600 transition-colors"
                          >
                            {player.playerName}
                          </Link>
                        </div>
                        <div className="text-sm text-gray-500 dark:text-gray-400">
                          {player.playerPosition} · {player.team}
                        </div>
                      </div>
                      <div className="text-xl font-semibold">
                        {player.points.toFixed(1)}
                      </div>
                    </li>
                  ))}
              </ul>
            </div>

            {/* Underperformers */}
            <div>
              <h3 className="text-lg font-medium mb-3">
                Biggest Underperformers
              </h3>
              <ul className="space-y-3">
                {[...homeTeam.players, ...awayTeam.players]
                  .filter(
                    (p) =>
                      p.slotPosition !== "BE" &&
                      p.slotPosition !== "IR" &&
                      p.slotPosition !== ""
                  )
                  .sort(
                    (a, b) =>
                      a.points -
                      a.projectedPoints -
                      (b.points - b.projectedPoints)
                  )
                  .slice(0, 3)
                  .map((player) => {
                    const diff = player.points - player.projectedPoints;
                    return (
                      <li
                        key={player.id}
                        className="flex items-center justify-between bg-gray-50 dark:bg-gray-700 p-3 rounded-lg"
                      >
                        <div>
                          <div className="font-medium">
                            <Link
                              href={`/players/${player.id}`}
                              className="hover:text-blue-600 transition-colors"
                            >
                              {player.playerName}
                            </Link>
                          </div>
                          <div className="text-sm text-gray-500 dark:text-gray-400">
                            {player.playerPosition} · {player.team}
                          </div>
                        </div>
                        <div className="text-right">
                          <div className="text-xl font-semibold">
                            {player.points.toFixed(1)}
                          </div>
                          <div className="text-sm text-red-600">
                            {diff > 0 ? "+" : ""}
                            {diff.toFixed(1)} vs proj
                          </div>
                        </div>
                      </li>
                    );
                  })}
              </ul>
            </div>
          </div>
        </div>

        {/* Points Left on Bench */}
        <div className="bg-white dark:bg-gray-800 shadow-md rounded-lg p-6">
          <h2 className="text-xl font-semibold mb-4 pb-2 border-b border-gray-200 dark:border-gray-700">
            Bench Analysis
          </h2>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
            {/* Home Team Bench */}
            <div>
              <h3 className="text-lg font-medium mb-3">
                {homeTeam.name} Bench
              </h3>

              {homeTeam.players.filter(
                (p) => p.slotPosition === "BE" || p.slotPosition === "IR"
              ).length > 0 ? (
                <>
                  <div className="mb-4">
                    <div className="text-sm text-gray-500 dark:text-gray-400">
                      Bench Scoring
                    </div>
                    <div className="text-2xl font-semibold">
                      {homeTeam.players
                        .filter(
                          (p) =>
                            p.slotPosition === "BE" || p.slotPosition === "IR"
                        )
                        .reduce((sum, p) => sum + p.points, 0)
                        .toFixed(1)}
                    </div>
                  </div>

                  <ul className="space-y-2">
                    {homeTeam.players
                      .filter(
                        (p) =>
                          p.slotPosition === "BE" || p.slotPosition === "IR"
                      )
                      .sort((a, b) => b.points - a.points)
                      .map((player) => (
                        <li
                          key={player.id}
                          className="flex items-center justify-between"
                        >
                          <div>
                            <span className="font-medium">
                              <Link
                                href={`/players/${player.id}`}
                                className="hover:text-blue-600 transition-colors"
                              >
                                {player.playerName}
                              </Link>
                            </span>
                            <span className="text-sm text-gray-500 dark:text-gray-400 ml-2">
                              {player.playerPosition}
                            </span>
                          </div>
                          <div className="font-medium">
                            {player.points.toFixed(1)}
                          </div>
                        </li>
                      ))}
                  </ul>
                </>
              ) : (
                <div className="text-gray-500 dark:text-gray-400">
                  No bench players
                </div>
              )}
            </div>

            {/* Away Team Bench */}
            <div>
              <h3 className="text-lg font-medium mb-3">
                {awayTeam.name} Bench
              </h3>

              {awayTeam.players.filter(
                (p) => p.slotPosition === "BE" || p.slotPosition === "IR"
              ).length > 0 ? (
                <>
                  <div className="mb-4">
                    <div className="text-sm text-gray-500 dark:text-gray-400">
                      Bench Scoring
                    </div>
                    <div className="text-2xl font-semibold">
                      {awayTeam.players
                        .filter(
                          (p) =>
                            p.slotPosition === "BE" || p.slotPosition === "IR"
                        )
                        .reduce((sum, p) => sum + p.points, 0)
                        .toFixed(1)}
                    </div>
                  </div>

                  <ul className="space-y-2">
                    {awayTeam.players
                      .filter(
                        (p) =>
                          p.slotPosition === "BE" || p.slotPosition === "IR"
                      )
                      .sort((a, b) => b.points - a.points)
                      .map((player) => (
                        <li
                          key={player.id}
                          className="flex items-center justify-between"
                        >
                          <div>
                            <span className="font-medium">
                              <Link
                                href={`/players/${player.id}`}
                                className="hover:text-blue-600 transition-colors"
                              >
                                {player.playerName}
                              </Link>
                            </span>
                            <span className="text-sm text-gray-500 dark:text-gray-400 ml-2">
                              {player.playerPosition}
                            </span>
                          </div>
                          <div className="font-medium">
                            {player.points.toFixed(1)}
                          </div>
                        </li>
                      ))}
                  </ul>
                </>
              ) : (
                <div className="text-gray-500 dark:text-gray-400">
                  No bench players
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Better Lineup Decisions */}
        <div className="bg-white dark:bg-gray-800 shadow-md rounded-lg p-6">
          <h2 className="text-xl font-semibold mb-4 pb-2 border-b border-gray-200 dark:border-gray-700">
            Better Lineup Decisions
          </h2>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
            {/* Home Team Better Decisions */}
            <div>
              <h3 className="text-lg font-medium mb-3">
                {homeTeam.name} Missed Opportunities
              </h3>

              {(() => {
                const betterDecisions = findBetterLineupDecisions(
                  homeTeam.players
                );
                return betterDecisions.length > 0 ? (
                  <>
                    <div className="mb-4">
                      <div className="text-sm text-gray-500 dark:text-gray-400">
                        Total Points Left on Table
                      </div>
                      <div className="text-2xl font-semibold text-red-600">
                        {betterDecisions
                          .reduce(
                            (sum, decision) => sum + decision.pointsGained,
                            0
                          )
                          .toFixed(1)}
                      </div>
                    </div>

                    <ul className="space-y-3">
                      {betterDecisions.map((decision) => (
                        <li
                          key={`${decision.benchPlayer.id}-${decision.starterPlayer.id}`}
                          className="bg-red-50 dark:bg-red-900/20 p-3 rounded-lg border border-red-200 dark:border-red-800"
                        >
                          <div className="flex items-center justify-between">
                            <div className="flex-1">
                              <div className="text-sm font-medium">
                                Start{" "}
                                <Link
                                  href={`/players/${decision.benchPlayer.id}`}
                                  className="text-green-600 font-semibold hover:text-green-700 transition-colors"
                                >
                                  {decision.benchPlayer.playerName}
                                </Link>{" "}
                                ({decision.benchPlayer.playerPosition})
                              </div>
                              <div className="text-sm text-gray-600 dark:text-gray-400">
                                Instead of{" "}
                                <Link
                                  href={`/players/${decision.starterPlayer.id}`}
                                  className="text-red-600 font-semibold hover:text-red-700 transition-colors"
                                >
                                  {decision.starterPlayer.playerName}
                                </Link>{" "}
                                ({decision.starterPlayer.slotPosition})
                              </div>
                              <div className="text-xs text-gray-500 mt-1">
                                {decision.benchPlayer.points.toFixed(1)} vs{" "}
                                {decision.starterPlayer.points.toFixed(1)} pts
                              </div>
                            </div>
                            <div className="text-right">
                              <div className="text-lg font-semibold text-green-600">
                                +{decision.pointsGained.toFixed(1)}
                              </div>
                              <div className="text-xs text-gray-500">
                                points
                              </div>
                            </div>
                          </div>
                        </li>
                      ))}
                    </ul>
                  </>
                ) : (
                  <div className="text-gray-500 dark:text-gray-400 bg-green-50 dark:bg-green-900/20 p-4 rounded-lg border border-green-200 dark:border-green-800">
                    Perfect lineup! No better decisions available.
                  </div>
                );
              })()}
            </div>

            {/* Away Team Better Decisions */}
            <div>
              <h3 className="text-lg font-medium mb-3">
                {awayTeam.name} Missed Opportunities
              </h3>

              {(() => {
                const betterDecisions = findBetterLineupDecisions(
                  awayTeam.players
                );
                return betterDecisions.length > 0 ? (
                  <>
                    <div className="mb-4">
                      <div className="text-sm text-gray-500 dark:text-gray-400">
                        Total Points Left on Table
                      </div>
                      <div className="text-2xl font-semibold text-red-600">
                        {betterDecisions
                          .reduce(
                            (sum, decision) => sum + decision.pointsGained,
                            0
                          )
                          .toFixed(1)}
                      </div>
                    </div>

                    <ul className="space-y-3">
                      {betterDecisions.map((decision) => (
                        <li
                          key={`${decision.benchPlayer.id}-${decision.starterPlayer.id}`}
                          className="bg-red-50 dark:bg-red-900/20 p-3 rounded-lg border border-red-200 dark:border-red-800"
                        >
                          <div className="flex items-center justify-between">
                            <div className="flex-1">
                              <div className="text-sm font-medium">
                                Start{" "}
                                <Link
                                  href={`/players/${decision.benchPlayer.id}`}
                                  className="text-green-600 font-semibold hover:text-green-700 transition-colors"
                                >
                                  {decision.benchPlayer.playerName}
                                </Link>{" "}
                                ({decision.benchPlayer.playerPosition})
                              </div>
                              <div className="text-sm text-gray-600 dark:text-gray-400">
                                Instead of{" "}
                                <Link
                                  href={`/players/${decision.starterPlayer.id}`}
                                  className="text-red-600 font-semibold hover:text-red-700 transition-colors"
                                >
                                  {decision.starterPlayer.playerName}
                                </Link>{" "}
                                ({decision.starterPlayer.slotPosition})
                              </div>
                              <div className="text-xs text-gray-500 mt-1">
                                {decision.benchPlayer.points.toFixed(1)} vs{" "}
                                {decision.starterPlayer.points.toFixed(1)} pts
                              </div>
                            </div>
                            <div className="text-right">
                              <div className="text-lg font-semibold text-green-600">
                                +{decision.pointsGained.toFixed(1)}
                              </div>
                              <div className="text-xs text-gray-500">
                                points
                              </div>
                            </div>
                          </div>
                        </li>
                      ))}
                    </ul>
                  </>
                ) : (
                  <div className="text-gray-500 dark:text-gray-400 bg-green-50 dark:bg-green-900/20 p-4 rounded-lg border border-green-200 dark:border-green-800">
                    Perfect lineup! No better decisions available.
                  </div>
                );
              })()}
            </div>
          </div>
        </div>
      </div>
    </Layout>
  );
}
