import { useRouter } from "next/router";
import { useMatchupDetail } from "@/hooks/useMatchupDetail";
import Link from "next/link";
import { Player } from "@/services/scheduleService";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import ErrorState from "@/components/design-system/ErrorState";

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
  const { matchupId, leagueId } = router.query;
  const leagueIdNum = Number(leagueId);
  const { matchup, isLoading, error } = useMatchupDetail(leagueIdNum, matchupId as string);

  if (isLoading || matchup === null) {
    return (
      <div className="space-y-8">
        <div className="space-y-2">
          <Skeleton className="h-4 w-40" />
          <Skeleton className="h-10 w-96 max-w-full" />
        </div>
        <Card>
          <CardContent className="p-6">
            <Skeleton className="h-32 w-full" />
          </CardContent>
        </Card>
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
          <Card>
            <CardContent className="space-y-2 p-6">
              {Array.from({ length: 6 }).map((_, i) => (
                <Skeleton key={i} className="h-8 w-full" />
              ))}
            </CardContent>
          </Card>
          <Card>
            <CardContent className="space-y-2 p-6">
              {Array.from({ length: 6 }).map((_, i) => (
                <Skeleton key={i} className="h-8 w-full" />
              ))}
            </CardContent>
          </Card>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-4">
        <ErrorState message={error.message} />
        <Button asChild>
          <Link href={`/league/${leagueIdNum}/schedule`}>
            Return to Schedule
          </Link>
        </Button>
      </div>
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
    <div className="space-y-8">
      {/* Breadcrumbs and title */}
      <div>
        <div
          className="flex items-center mb-2"
          style={{ color: "var(--text-muted)" }}
        >
          <Link
            href={`/league/${leagueIdNum}/schedule`}
            className="hover:underline"
            style={{ color: "var(--text-muted)" }}
          >
            Schedule
          </Link>
          <span className="mx-2">›</span>
          <span>
            Year {year}, Week {week}
          </span>
        </div>
        <h1
          className="text-3xl md:text-4xl font-bold mb-2"
          style={{ color: "var(--text-primary)" }}
        >
          {homeTeam.name} vs {awayTeam.name}
        </h1>
        <div className="text-lg" style={{ color: "var(--text-muted)" }}>
          Week {week}, Year {year}
        </div>
      </div>

      {/* Matchup summary card */}
      <Card>
        <CardContent className="p-6">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-8 items-center">
            {/* Home Team */}
            <div
              className={`text-center ${homeWon ? "p-4 rounded-lg" : ""}`}
              style={
                homeWon
                  ? { backgroundColor: "var(--status-success-bg)" }
                  : undefined
              }
            >
              <div className="text-xl font-semibold mb-1">
                <Link
                  href={`/league/${leagueIdNum}/teams/${homeTeamESPNID}`}
                  className="hover:underline"
                  style={{ color: "var(--text-primary)" }}
                >
                  {homeTeam.name}
                </Link>
              </div>
              <div
                className="text-4xl font-bold mb-2"
                style={{
                  color: homeWon
                    ? "var(--status-success-fg)"
                    : "var(--text-primary)",
                }}
              >
                {homeTeam.score.toFixed(1)}
              </div>
              <div className="text-sm" style={{ color: "var(--text-muted)" }}>
                Projected: {homeTeam.projectedScore.toFixed(1)}
                {homeProjectedDiff !== 0 && (
                  <span
                    className="ml-2"
                    style={{
                      color:
                        homeProjectedDiff > 0
                          ? "var(--status-success-fg)"
                          : "var(--status-danger-fg)",
                    }}
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
                  <Badge
                    variant="outline"
                    className="rounded-full"
                    style={{
                      color: "var(--status-success-fg)",
                      borderColor: "var(--status-success-fg)",
                      backgroundColor: "var(--status-success-bg)",
                    }}
                  >
                    Final
                  </Badge>
                ) : (
                  <Badge
                    variant="outline"
                    className="rounded-full"
                    style={{
                      color: "var(--status-info-fg)",
                      borderColor: "var(--status-info-fg)",
                      backgroundColor: "var(--status-info-bg)",
                    }}
                  >
                    Upcoming
                  </Badge>
                )}
              </div>

              <div
                className="text-2xl font-bold my-2"
                style={{ color: "var(--text-primary)" }}
              >
                vs
              </div>

              {isCompleted && (
                <div
                  className="text-sm mb-2"
                  style={{ color: "var(--text-muted)" }}
                >
                  Point Differential:{" "}
                  <span className="font-medium">
                    {(homeTeam.score - awayTeam.score).toFixed(2)}
                  </span>
                </div>
              )}
            </div>

            {/* Away Team */}
            <div
              className={`text-center ${awayWon ? "p-4 rounded-lg" : ""}`}
              style={
                awayWon
                  ? { backgroundColor: "var(--status-success-bg)" }
                  : undefined
              }
            >
              <div className="text-xl font-semibold mb-1">
                <Link
                  href={`/league/${leagueIdNum}/teams/${awayTeamESPNID}`}
                  className="hover:underline"
                  style={{ color: "var(--text-primary)" }}
                >
                  {awayTeam.name}
                </Link>
              </div>
              <div
                className="text-4xl font-bold mb-2"
                style={{
                  color: awayWon
                    ? "var(--status-success-fg)"
                    : "var(--text-primary)",
                }}
              >
                {awayTeam.score.toFixed(1)}
              </div>
              <div className="text-sm" style={{ color: "var(--text-muted)" }}>
                Projected: {awayTeam.projectedScore.toFixed(1)}
                {awayProjectedDiff !== 0 && (
                  <span
                    className="ml-2"
                    style={{
                      color:
                        awayProjectedDiff > 0
                          ? "var(--status-success-fg)"
                          : "var(--status-danger-fg)",
                    }}
                  >
                    ({awayProjectedDiff > 0 ? "+" : ""}
                    {awayProjectedDiff.toFixed(1)})
                  </span>
                )}
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Team Lineups */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        {/* Home Team Lineup */}
        <Card>
          <CardContent className="p-6">
            <h2
              className="text-xl font-semibold mb-4"
              style={{ color: "var(--text-primary)" }}
            >
              {homeTeam.name} Lineup
            </h2>

            <div className="overflow-x-auto">
              <table className="min-w-full">
                <thead>
                  <tr style={{ borderBottom: "1px solid var(--border-subtle)" }}>
                    <th
                      className="py-2 px-3 text-left text-xs font-medium uppercase tracking-wider"
                      style={{ color: "var(--text-muted)" }}
                    >
                      Player
                    </th>
                    <th
                      className="py-2 px-3 text-left text-xs font-medium uppercase tracking-wider"
                      style={{ color: "var(--text-muted)" }}
                    >
                      NFL Pos
                    </th>
                    <th
                      className="py-2 px-3 text-right text-xs font-medium uppercase tracking-wider"
                      style={{ color: "var(--text-muted)" }}
                    >
                      Proj
                    </th>
                    <th
                      className="py-2 px-3 text-right text-xs font-medium uppercase tracking-wider"
                      style={{ color: "var(--text-muted)" }}
                    >
                      Actual
                    </th>
                    <th
                      className="py-2 px-3 text-right text-xs font-medium uppercase tracking-wider"
                      style={{ color: "var(--text-muted)" }}
                    >
                      Diff
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {homeTeam.players.map((player, playerIdx) => {
                    const diff = player.points - player.projectedPoints;
                    const isBench =
                      player.slotPosition === "BE" ||
                      player.slotPosition === "IR";
                    const isLastRow =
                      playerIdx === homeTeam.players.length - 1;
                    return (
                      <tr
                        key={player.id}
                        style={{
                          borderBottom: isLastRow
                            ? undefined
                            : "1px solid var(--border-subtle)",
                          color: isBench ? "var(--text-muted)" : undefined,
                          backgroundColor: isBench
                            ? "var(--surface-sunken)"
                            : undefined,
                        }}
                      >
                        <td className="py-2 px-3">
                          <div className="flex items-center">
                            <span
                              className="inline-block w-8 mr-2 text-xs font-medium"
                              style={{ color: "var(--text-muted)" }}
                            >
                              {player.slotPosition || player.playerPosition}
                            </span>
                            <Link
                              href={`/players/${player.id}`}
                              className="hover:underline"
                              style={{
                                color: isBench
                                  ? "var(--text-muted)"
                                  : "var(--text-primary)",
                              }}
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
                          className="py-2 px-3 text-right"
                          style={{
                            color:
                              diff > 0
                                ? "var(--status-success-fg)"
                                : diff < 0
                                ? "var(--status-danger-fg)"
                                : undefined,
                          }}
                        >
                          {diff > 0 && "+"}
                          {diff.toFixed(1)}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
                <tfoot>
                  <tr
                    className="font-semibold"
                    style={{ borderTop: "1px solid var(--border-strong)" }}
                  >
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
                      className="py-2 px-3 text-right"
                      style={{
                        color:
                          homeProjectedDiff > 0
                            ? "var(--status-success-fg)"
                            : homeProjectedDiff < 0
                            ? "var(--status-danger-fg)"
                            : undefined,
                      }}
                    >
                      {homeProjectedDiff > 0 && "+"}
                      {homeProjectedDiff.toFixed(1)}
                    </td>
                  </tr>
                </tfoot>
              </table>
            </div>
          </CardContent>
        </Card>

        {/* Away Team Lineup */}
        <Card>
          <CardContent className="p-6">
            <h2
              className="text-xl font-semibold mb-4"
              style={{ color: "var(--text-primary)" }}
            >
              {awayTeam.name} Lineup
            </h2>

            <div className="overflow-x-auto">
              <table className="min-w-full">
                <thead>
                  <tr style={{ borderBottom: "1px solid var(--border-subtle)" }}>
                    <th
                      className="py-2 px-3 text-left text-xs font-medium uppercase tracking-wider"
                      style={{ color: "var(--text-muted)" }}
                    >
                      Player
                    </th>
                    <th
                      className="py-2 px-3 text-left text-xs font-medium uppercase tracking-wider"
                      style={{ color: "var(--text-muted)" }}
                    >
                      NFL Pos
                    </th>
                    <th
                      className="py-2 px-3 text-right text-xs font-medium uppercase tracking-wider"
                      style={{ color: "var(--text-muted)" }}
                    >
                      Proj
                    </th>
                    <th
                      className="py-2 px-3 text-right text-xs font-medium uppercase tracking-wider"
                      style={{ color: "var(--text-muted)" }}
                    >
                      Actual
                    </th>
                    <th
                      className="py-2 px-3 text-right text-xs font-medium uppercase tracking-wider"
                      style={{ color: "var(--text-muted)" }}
                    >
                      Diff
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {awayTeam.players.map((player, playerIdx) => {
                    const diff = player.points - player.projectedPoints;
                    const isBench =
                      player.slotPosition === "BE" ||
                      player.slotPosition === "IR";
                    const isLastRow =
                      playerIdx === awayTeam.players.length - 1;
                    return (
                      <tr
                        key={player.id}
                        style={{
                          borderBottom: isLastRow
                            ? undefined
                            : "1px solid var(--border-subtle)",
                          color: isBench ? "var(--text-muted)" : undefined,
                          backgroundColor: isBench
                            ? "var(--surface-sunken)"
                            : undefined,
                        }}
                      >
                        <td className="py-2 px-3">
                          <div className="flex items-center">
                            <span
                              className="inline-block w-8 mr-2 text-xs font-medium"
                              style={{ color: "var(--text-muted)" }}
                            >
                              {player.slotPosition || player.playerPosition}
                            </span>
                            <Link
                              href={`/players/${player.id}`}
                              className="hover:underline"
                              style={{
                                color: isBench
                                  ? "var(--text-muted)"
                                  : "var(--text-primary)",
                              }}
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
                          className="py-2 px-3 text-right"
                          style={{
                            color:
                              diff > 0
                                ? "var(--status-success-fg)"
                                : diff < 0
                                ? "var(--status-danger-fg)"
                                : undefined,
                          }}
                        >
                          {diff > 0 && "+"}
                          {diff.toFixed(1)}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
                <tfoot>
                  <tr
                    className="font-semibold"
                    style={{ borderTop: "1px solid var(--border-strong)" }}
                  >
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
                      className="py-2 px-3 text-right"
                      style={{
                        color:
                          awayProjectedDiff > 0
                            ? "var(--status-success-fg)"
                            : awayProjectedDiff < 0
                            ? "var(--status-danger-fg)"
                            : undefined,
                      }}
                    >
                      {awayProjectedDiff > 0 && "+"}
                      {awayProjectedDiff.toFixed(1)}
                    </td>
                  </tr>
                </tfoot>
              </table>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Key Performances */}
      <Card>
        <CardContent className="p-6">
          <h2
            className="text-xl font-semibold mb-4 pb-2"
            style={{
              color: "var(--text-primary)",
              borderBottom: "1px solid var(--border-subtle)",
            }}
          >
            Key Performances
          </h2>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            {/* Best Performers */}
            <div>
              <h3
                className="text-lg font-medium mb-3"
                style={{ color: "var(--text-primary)" }}
              >
                Top Performers
              </h3>
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
                      className="flex items-center justify-between p-3 rounded-lg"
                      style={{ backgroundColor: "var(--surface-sunken)" }}
                    >
                      <div>
                        <div className="font-medium">
                          <Link
                            href={`/players/${player.id}`}
                            className="hover:underline"
                            style={{ color: "var(--text-primary)" }}
                          >
                            {player.playerName}
                          </Link>
                        </div>
                        <div
                          className="text-sm"
                          style={{ color: "var(--text-muted)" }}
                        >
                          {player.playerPosition} · {player.team}
                        </div>
                      </div>
                      <div
                        className="text-xl font-semibold"
                        style={{ color: "var(--text-primary)" }}
                      >
                        {player.points.toFixed(1)}
                      </div>
                    </li>
                  ))}
              </ul>
            </div>

            {/* Underperformers */}
            <div>
              <h3
                className="text-lg font-medium mb-3"
                style={{ color: "var(--text-primary)" }}
              >
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
                        className="flex items-center justify-between p-3 rounded-lg"
                        style={{ backgroundColor: "var(--surface-sunken)" }}
                      >
                        <div>
                          <div className="font-medium">
                            <Link
                              href={`/players/${player.id}`}
                              className="hover:underline"
                              style={{ color: "var(--text-primary)" }}
                            >
                              {player.playerName}
                            </Link>
                          </div>
                          <div
                            className="text-sm"
                            style={{ color: "var(--text-muted)" }}
                          >
                            {player.playerPosition} · {player.team}
                          </div>
                        </div>
                        <div className="text-right">
                          <div
                            className="text-xl font-semibold"
                            style={{ color: "var(--text-primary)" }}
                          >
                            {player.points.toFixed(1)}
                          </div>
                          <div
                            className="text-sm"
                            style={{ color: "var(--status-danger-fg)" }}
                          >
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
        </CardContent>
      </Card>

      {/* Points Left on Bench */}
      <Card>
        <CardContent className="p-6">
          <h2
            className="text-xl font-semibold mb-4 pb-2"
            style={{
              color: "var(--text-primary)",
              borderBottom: "1px solid var(--border-subtle)",
            }}
          >
            Bench Analysis
          </h2>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
            {/* Home Team Bench */}
            <div>
              <h3
                className="text-lg font-medium mb-3"
                style={{ color: "var(--text-primary)" }}
              >
                {homeTeam.name} Bench
              </h3>

              {homeTeam.players.filter(
                (p) => p.slotPosition === "BE" || p.slotPosition === "IR"
              ).length > 0 ? (
                <>
                  <div className="mb-4">
                    <div
                      className="text-sm"
                      style={{ color: "var(--text-muted)" }}
                    >
                      Bench Scoring
                    </div>
                    <div
                      className="text-2xl font-semibold"
                      style={{ color: "var(--text-primary)" }}
                    >
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
                                className="hover:underline"
                                style={{ color: "var(--text-primary)" }}
                              >
                                {player.playerName}
                              </Link>
                            </span>
                            <span
                              className="text-sm ml-2"
                              style={{ color: "var(--text-muted)" }}
                            >
                              {player.playerPosition}
                            </span>
                          </div>
                          <div
                            className="font-medium"
                            style={{ color: "var(--text-primary)" }}
                          >
                            {player.points.toFixed(1)}
                          </div>
                        </li>
                      ))}
                  </ul>
                </>
              ) : (
                <div style={{ color: "var(--text-muted)" }}>
                  No bench players
                </div>
              )}
            </div>

            {/* Away Team Bench */}
            <div>
              <h3
                className="text-lg font-medium mb-3"
                style={{ color: "var(--text-primary)" }}
              >
                {awayTeam.name} Bench
              </h3>

              {awayTeam.players.filter(
                (p) => p.slotPosition === "BE" || p.slotPosition === "IR"
              ).length > 0 ? (
                <>
                  <div className="mb-4">
                    <div
                      className="text-sm"
                      style={{ color: "var(--text-muted)" }}
                    >
                      Bench Scoring
                    </div>
                    <div
                      className="text-2xl font-semibold"
                      style={{ color: "var(--text-primary)" }}
                    >
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
                                className="hover:underline"
                                style={{ color: "var(--text-primary)" }}
                              >
                                {player.playerName}
                              </Link>
                            </span>
                            <span
                              className="text-sm ml-2"
                              style={{ color: "var(--text-muted)" }}
                            >
                              {player.playerPosition}
                            </span>
                          </div>
                          <div
                            className="font-medium"
                            style={{ color: "var(--text-primary)" }}
                          >
                            {player.points.toFixed(1)}
                          </div>
                        </li>
                      ))}
                  </ul>
                </>
              ) : (
                <div style={{ color: "var(--text-muted)" }}>
                  No bench players
                </div>
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Better Lineup Decisions */}
      <Card>
        <CardContent className="p-6">
          <h2
            className="text-xl font-semibold mb-4 pb-2"
            style={{
              color: "var(--text-primary)",
              borderBottom: "1px solid var(--border-subtle)",
            }}
          >
            Better Lineup Decisions
          </h2>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
            {/* Home Team Better Decisions */}
            <div>
              <h3
                className="text-lg font-medium mb-3"
                style={{ color: "var(--text-primary)" }}
              >
                {homeTeam.name} Missed Opportunities
              </h3>

              {(() => {
                const betterDecisions = findBetterLineupDecisions(
                  homeTeam.players
                );
                return betterDecisions.length > 0 ? (
                  <>
                    <div className="mb-4">
                      <div
                        className="text-sm"
                        style={{ color: "var(--text-muted)" }}
                      >
                        Total Points Left on Table
                      </div>
                      <div
                        className="text-2xl font-semibold"
                        style={{ color: "var(--status-danger-fg)" }}
                      >
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
                          className="p-3 rounded-lg border-l-4"
                          style={{
                            backgroundColor: "var(--surface-sunken)",
                            borderLeftColor: "var(--status-danger-fg)",
                          }}
                        >
                          <div className="flex items-center justify-between">
                            <div className="flex-1">
                              <div className="text-sm font-medium">
                                Start{" "}
                                <Link
                                  href={`/players/${decision.benchPlayer.id}`}
                                  className="font-semibold hover:underline"
                                  style={{ color: "var(--status-success-fg)" }}
                                >
                                  {decision.benchPlayer.playerName}
                                </Link>{" "}
                                ({decision.benchPlayer.playerPosition})
                              </div>
                              <div
                                className="text-sm"
                                style={{ color: "var(--text-secondary)" }}
                              >
                                Instead of{" "}
                                <Link
                                  href={`/players/${decision.starterPlayer.id}`}
                                  className="font-semibold hover:underline"
                                  style={{ color: "var(--status-danger-fg)" }}
                                >
                                  {decision.starterPlayer.playerName}
                                </Link>{" "}
                                ({decision.starterPlayer.slotPosition})
                              </div>
                              <div
                                className="text-xs mt-1"
                                style={{ color: "var(--text-muted)" }}
                              >
                                {decision.benchPlayer.points.toFixed(1)} vs{" "}
                                {decision.starterPlayer.points.toFixed(1)} pts
                              </div>
                            </div>
                            <div className="text-right">
                              <div
                                className="text-lg font-semibold"
                                style={{ color: "var(--status-success-fg)" }}
                              >
                                +{decision.pointsGained.toFixed(1)}
                              </div>
                              <div
                                className="text-xs"
                                style={{ color: "var(--text-muted)" }}
                              >
                                points
                              </div>
                            </div>
                          </div>
                        </li>
                      ))}
                    </ul>
                  </>
                ) : (
                  <div
                    className="p-4 rounded-lg border-l-4"
                    style={{
                      color: "var(--text-muted)",
                      backgroundColor: "var(--surface-sunken)",
                      borderLeftColor: "var(--status-success-fg)",
                    }}
                  >
                    Perfect lineup! No better decisions available.
                  </div>
                );
              })()}
            </div>

            {/* Away Team Better Decisions */}
            <div>
              <h3
                className="text-lg font-medium mb-3"
                style={{ color: "var(--text-primary)" }}
              >
                {awayTeam.name} Missed Opportunities
              </h3>

              {(() => {
                const betterDecisions = findBetterLineupDecisions(
                  awayTeam.players
                );
                return betterDecisions.length > 0 ? (
                  <>
                    <div className="mb-4">
                      <div
                        className="text-sm"
                        style={{ color: "var(--text-muted)" }}
                      >
                        Total Points Left on Table
                      </div>
                      <div
                        className="text-2xl font-semibold"
                        style={{ color: "var(--status-danger-fg)" }}
                      >
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
                          className="p-3 rounded-lg border-l-4"
                          style={{
                            backgroundColor: "var(--surface-sunken)",
                            borderLeftColor: "var(--status-danger-fg)",
                          }}
                        >
                          <div className="flex items-center justify-between">
                            <div className="flex-1">
                              <div className="text-sm font-medium">
                                Start{" "}
                                <Link
                                  href={`/players/${decision.benchPlayer.id}`}
                                  className="font-semibold hover:underline"
                                  style={{ color: "var(--status-success-fg)" }}
                                >
                                  {decision.benchPlayer.playerName}
                                </Link>{" "}
                                ({decision.benchPlayer.playerPosition})
                              </div>
                              <div
                                className="text-sm"
                                style={{ color: "var(--text-secondary)" }}
                              >
                                Instead of{" "}
                                <Link
                                  href={`/players/${decision.starterPlayer.id}`}
                                  className="font-semibold hover:underline"
                                  style={{ color: "var(--status-danger-fg)" }}
                                >
                                  {decision.starterPlayer.playerName}
                                </Link>{" "}
                                ({decision.starterPlayer.slotPosition})
                              </div>
                              <div
                                className="text-xs mt-1"
                                style={{ color: "var(--text-muted)" }}
                              >
                                {decision.benchPlayer.points.toFixed(1)} vs{" "}
                                {decision.starterPlayer.points.toFixed(1)} pts
                              </div>
                            </div>
                            <div className="text-right">
                              <div
                                className="text-lg font-semibold"
                                style={{ color: "var(--status-success-fg)" }}
                              >
                                +{decision.pointsGained.toFixed(1)}
                              </div>
                              <div
                                className="text-xs"
                                style={{ color: "var(--text-muted)" }}
                              >
                                points
                              </div>
                            </div>
                          </div>
                        </li>
                      ))}
                    </ul>
                  </>
                ) : (
                  <div
                    className="p-4 rounded-lg border-l-4"
                    style={{
                      color: "var(--text-muted)",
                      backgroundColor: "var(--surface-sunken)",
                      borderLeftColor: "var(--status-success-fg)",
                    }}
                  >
                    Perfect lineup! No better decisions available.
                  </div>
                );
              })()}
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
