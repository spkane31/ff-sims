import type { WeeklyLeverage, WeeklyTeamLeverage } from "@/types/simulation";
import { Card, CardContent } from "@/components/ui/card";

const percent = (value: number) => `${(value * 100).toFixed(1)}%`;

function OddsChart({
  title,
  metric,
  teams,
}: {
  title: string;
  metric: "playoff" | "lastPlace";
  teams: WeeklyTeamLeverage[];
}) {
  const lowerColor = metric === "playoff" ? "var(--status-danger-fg)" : "var(--status-success-fg)";
  const higherColor = metric === "playoff" ? "var(--status-success-fg)" : "var(--status-danger-fg)";

  return (
    <Card>
      <CardContent className="p-4 sm:p-6">
        <h4 className="text-base font-semibold" style={{ color: "var(--text-primary)" }}>
          {title}
        </h4>
        <div className="relative mx-2 mb-5 mt-4 h-4 text-xs" style={{ color: "var(--text-muted)" }} aria-hidden="true">
          {[0, 25, 50, 75, 100].map((tick) => (
            <span key={tick} className="absolute" style={{
              left: `${tick}%`,
              transform: tick === 0 ? undefined : tick === 100 ? "translateX(-100%)" : "translateX(-50%)",
            }}>
              {tick}%
            </span>
          ))}
        </div>
        <ul className="space-y-5">
          {teams.map((team) => {
            const odds = team[metric];
            const description = `${team.teamName}: current ${percent(odds.baseline)}, weekly minimum ${percent(odds.min)}, weekly maximum ${percent(odds.max)}.`;
            return (
              <li key={team.teamId}>
                <div className="flex items-baseline justify-between gap-3 text-sm" style={{ color: "var(--text-primary)" }}>
                  <span className="min-w-0 break-words font-medium">{team.teamName}</span>
                  <span className="shrink-0 font-semibold tabular-nums">{percent(odds.baseline)}</span>
                </div>
                <div role="img" aria-label={description} title={description} className="relative mx-2 my-1 h-6">
                  <div className="absolute inset-x-0 top-1/2 h-px" style={{ backgroundColor: "var(--border-strong)" }} />
                  {[0, 25, 50, 75, 100].map((tick) => (
                    <div key={tick} className="absolute top-1/2 h-2 w-px -translate-y-1/2" style={{ left: `${tick}%`, backgroundColor: "var(--border-subtle)" }} />
                  ))}
                  {odds.min < odds.baseline && (
                    <>
                      <div className="absolute top-1/2 h-0.5 -translate-y-1/2" style={{ left: `${odds.min * 100}%`, width: `${(odds.baseline - odds.min) * 100}%`, backgroundColor: lowerColor }} />
                      <div className="absolute top-1/2 h-3 w-2 -translate-y-1/2" style={{ left: `${odds.min * 100}%`, backgroundColor: lowerColor, clipPath: "polygon(0 50%, 100% 0, 100% 100%)" }} />
                    </>
                  )}
                  {odds.max > odds.baseline && (
                    <>
                      <div className="absolute top-1/2 h-0.5 -translate-y-1/2" style={{ left: `${odds.baseline * 100}%`, width: `${(odds.max - odds.baseline) * 100}%`, backgroundColor: higherColor }} />
                      <div className="absolute top-1/2 h-3 w-2 -translate-x-full -translate-y-1/2" style={{ left: `${odds.max * 100}%`, backgroundColor: higherColor, clipPath: "polygon(100% 50%, 0 0, 0 100%)" }} />
                    </>
                  )}
                  <div className="absolute top-1/2 h-4 w-4 -translate-x-1/2 -translate-y-1/2 rounded-full border-2" style={{ left: `${odds.baseline * 100}%`, backgroundColor: "var(--text-primary)", borderColor: "var(--surface-raised)" }} />
                </div>
                <div className="flex justify-between gap-2 text-xs tabular-nums" style={{ color: "var(--text-muted)" }}>
                  <span>Min {percent(odds.min)}</span>
                  <span>Max {percent(odds.max)}</span>
                </div>
              </li>
            );
          })}
        </ul>
      </CardContent>
    </Card>
  );
}

export default function WeeklyLeverageCharts({ leverage }: { leverage: WeeklyLeverage | null }) {
  if (!leverage) return null;

  return (
    <section aria-labelledby="weekly-leverage-title" className="space-y-3">
      <h3 id="weekly-leverage-title" className="text-lg font-medium" style={{ color: "var(--text-primary)" }}>
        Week {leverage.week} Leverage
      </h3>
      <p className="text-sm" style={{ color: "var(--text-secondary)" }}>
        The dot shows current odds. Arrows show the lowest and highest odds across
        sampled combinations of every game’s winners in week {leverage.week},
        with later weeks still simulated. These charts use the full simulation,
        independent of your selected winners below. Last place means the regular-season finish.
      </p>
      <p className="text-xs" style={{ color: "var(--text-muted)" }}>
        {leverage.scenarioCount.toLocaleString()} of {leverage.possibleScenarioCount.toLocaleString()} winner
        combinations sampled · Smallest sample: {leverage.smallestScenarioCount.toLocaleString()} simulations
      </p>
      {(leverage.scenarioCount < leverage.possibleScenarioCount || leverage.smallestScenarioCount < 30) && (
        <p className="text-xs" style={{ color: "var(--status-warning-fg)" }}>
          Rare outcomes have few or no samples, so these ranges may be noisy or
          incomplete. Run more iterations for more reliable estimates.
        </p>
      )}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <OddsChart title="Making the Playoffs" metric="playoff" teams={leverage.teams} />
        <OddsChart title="Finishing Last" metric="lastPlace" teams={leverage.teams} />
      </div>
    </section>
  );
}
