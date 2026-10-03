import { useState } from "react";
import { Card, CardContent } from "@/components/ui/card";
import EmptyState from "@/components/design-system/EmptyState";
import { FOCUS_RING } from "@/components/design-system/focus-ring";
import type { BigGame, BigGameTeamOdds } from "@/types/simulation";

/**
 * A team's odds have to move at least this much under one of the two outcomes
 * before the expanded panel bothers listing it.
 */
const MOVER_THRESHOLD = 0.005;

interface BigGamesProps {
  games: BigGame[];
  /** How many weeks ahead the games were drawn from, for the subtitle. */
  weekWindow: number;
}

/** One line of the expanded panel: a team, a metric, and both outcomes. */
interface MoverRow {
  teamId: number;
  teamName: string;
  metric: "Playoff" | "Last place";
  baseline: number;
  ifHomeWins: number;
  ifAwayWins: number;
  /** True when a rising number is bad news, i.e. the last-place metric. */
  higherIsWorse: boolean;
}

function formatOdds(value: number): string {
  return `${(value * 100).toFixed(1)}%`;
}

function deltaColor(
  baseline: number,
  outcome: number,
  higherIsWorse: boolean
): string {
  const delta = outcome - baseline;
  if (Math.abs(delta) < MOVER_THRESHOLD) return "var(--text-muted)";
  const good = higherIsWorse ? delta < 0 : delta > 0;
  return good ? "var(--status-success-fg)" : "var(--status-danger-fg)";
}

function gameKey(game: BigGame): string {
  return `${game.week}-${game.homeTeamId}-${game.awayTeamId}`;
}

function moved(team: BigGameTeamOdds, metric: MoverRow["metric"]): boolean {
  const [baseline, ifHome, ifAway] =
    metric === "Playoff"
      ? [
          team.baselinePlayoffOdds,
          team.homeWinPlayoffOdds,
          team.awayWinPlayoffOdds,
        ]
      : [
          team.baselineLastPlaceOdds,
          team.homeWinLastPlaceOdds,
          team.awayWinLastPlaceOdds,
        ];
  return (
    Math.abs(ifHome - baseline) >= MOVER_THRESHOLD ||
    Math.abs(ifAway - baseline) >= MOVER_THRESHOLD
  );
}

/**
 * The teams and metrics worth showing for one game: anything that actually
 * moves, ordered by the swing ranking the simulator already applied.
 */
function moverRows(game: BigGame): MoverRow[] {
  const rows: MoverRow[] = [];

  for (const team of game.teams) {
    if (moved(team, "Playoff")) {
      rows.push({
        teamId: team.teamId,
        teamName: team.teamName,
        metric: "Playoff",
        baseline: team.baselinePlayoffOdds,
        ifHomeWins: team.homeWinPlayoffOdds,
        ifAwayWins: team.awayWinPlayoffOdds,
        higherIsWorse: false,
      });
    }
    if (moved(team, "Last place")) {
      rows.push({
        teamId: team.teamId,
        teamName: team.teamName,
        metric: "Last place",
        baseline: team.baselineLastPlaceOdds,
        ifHomeWins: team.homeWinLastPlaceOdds,
        ifAwayWins: team.awayWinLastPlaceOdds,
        higherIsWorse: true,
      });
    }
  }

  return rows;
}

function OutcomeCell({
  baseline,
  outcome,
  higherIsWorse,
}: {
  baseline: number;
  outcome: number;
  higherIsWorse: boolean;
}) {
  return (
    <span className="whitespace-nowrap">
      <span style={{ color: "var(--text-muted)" }}>{formatOdds(baseline)}</span>
      <span style={{ color: "var(--text-muted)" }}> → </span>
      <span
        className="font-medium"
        style={{ color: deltaColor(baseline, outcome, higherIsWorse) }}
      >
        {formatOdds(outcome)}
      </span>
    </span>
  );
}

function ExpandedPanel({ game }: { game: BigGame }) {
  const rows = moverRows(game);

  if (rows.length === 0) {
    return (
      <p className="px-4 pb-4 text-sm" style={{ color: "var(--text-muted)" }}>
        No team&apos;s odds move by more than half a point either way.
      </p>
    );
  }

  return (
    <div className="overflow-x-auto px-4 pb-4">
      <table className="min-w-full border-collapse text-sm">
        <thead>
          <tr style={{ borderBottom: "1px solid var(--border-subtle)" }}>
            <th
              className="py-2 pr-4 text-left font-medium"
              style={{ color: "var(--text-secondary)" }}
            >
              Team
            </th>
            <th
              className="py-2 pr-4 text-left font-medium"
              style={{ color: "var(--text-secondary)" }}
            >
              Odds
            </th>
            <th
              className="py-2 pr-4 text-left font-medium"
              style={{ color: "var(--text-secondary)" }}
            >
              If {game.homeTeamName} wins
            </th>
            <th
              className="py-2 text-left font-medium"
              style={{ color: "var(--text-secondary)" }}
            >
              If {game.awayTeamName} wins
            </th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr
              key={`${row.teamId}-${row.metric}`}
              style={{ borderBottom: "1px solid var(--border-subtle)" }}
            >
              <td
                className="py-2 pr-4 whitespace-nowrap"
                style={{ color: "var(--text-primary)" }}
              >
                {row.teamName}
              </td>
              <td
                className="py-2 pr-4 whitespace-nowrap"
                style={{ color: "var(--text-muted)" }}
              >
                {row.metric}
              </td>
              <td className="py-2 pr-4">
                <OutcomeCell
                  baseline={row.baseline}
                  outcome={row.ifHomeWins}
                  higherIsWorse={row.higherIsWorse}
                />
              </td>
              <td className="py-2">
                <OutcomeCell
                  baseline={row.baseline}
                  outcome={row.ifAwayWins}
                  higherIsWorse={row.higherIsWorse}
                />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export default function BigGames({ games, weekWindow }: BigGamesProps) {
  const [expandedKey, setExpandedKey] = useState<string | null>(null);

  return (
    <div>
      <h3
        className="text-lg font-medium mb-2"
        style={{ color: "var(--text-primary)" }}
      >
        Big Games
      </h3>
      <p className="text-sm mb-4" style={{ color: "var(--text-muted)" }}>
        The games over the next {weekWindow} week
        {weekWindow === 1 ? "" : "s"} whose outcome moves the playoff and
        last-place odds the most. Odds swing adds up every team&apos;s change
        under both results, so it can run past 100%.
      </p>

      {games.length === 0 ? (
        <EmptyState
          title="No big games coming up"
          description="No game in this window shifts the league's playoff or last-place odds by more than 2%."
        />
      ) : (
        <Card>
          <CardContent className="p-0">
            {games.map((game) => {
              const key = gameKey(game);
              const expanded = expandedKey === key;

              return (
                <div
                  key={key}
                  style={{ borderBottom: "1px solid var(--border-subtle)" }}
                >
                  <button
                    type="button"
                    onClick={() => setExpandedKey(expanded ? null : key)}
                    aria-expanded={expanded}
                    className={`flex w-full items-center gap-4 px-4 py-3 text-left ${FOCUS_RING}`}
                  >
                    <span
                      className="w-12 shrink-0 text-sm font-medium"
                      style={{ color: "var(--text-muted)" }}
                    >
                      Wk {game.week}
                    </span>
                    <span
                      className="flex-1 truncate text-sm font-medium"
                      style={{ color: "var(--text-primary)" }}
                    >
                      {game.homeTeamName} vs {game.awayTeamName}
                    </span>
                    <span
                      className="shrink-0 text-sm font-semibold tabular-nums"
                      style={{ color: "var(--text-primary)" }}
                      title="Total odds swing across the league"
                    >
                      {formatOdds(game.totalSwing)}
                    </span>
                    <span
                      className="shrink-0 text-xs"
                      style={{ color: "var(--text-muted)" }}
                      aria-hidden="true"
                    >
                      {expanded ? "▾" : "▸"}
                    </span>
                  </button>
                  {expanded && <ExpandedPanel game={game} />}
                </div>
              );
            })}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
