import type { FeaturedMatchupFixture } from './league-overview-fixtures';

interface FeaturedMatchupCardProps {
  matchup: FeaturedMatchupFixture;
}

const CARD_STYLE = {
  backgroundColor: 'var(--surface-raised)',
  borderColor: 'var(--border-subtle)',
};

/**
 * Current/featured matchup score card for the league-overview prototype.
 *
 * The "which team is ahead" cue pairs a font-weight change with an explicit
 * "Leading" text label (responsive-rules.md Section 6: never color alone),
 * and the win-probability bar prints the percentages as text rather than
 * relying on the fill width/color alone.
 */
export default function FeaturedMatchupCard({ matchup }: FeaturedMatchupCardProps) {
  const { week, status, homeTeam, awayTeam, homeWinProbability } = matchup;
  const homeLeading = homeTeam.score > awayTeam.score;
  const awayLeading = awayTeam.score > homeTeam.score;

  return (
    <section
      aria-label="Featured matchup"
      className="rounded-lg border p-4"
      style={CARD_STYLE}
    >
      <div className="mb-3 flex items-center justify-between gap-2">
        <h2 className="text-sm font-semibold" style={{ color: 'var(--text-primary)' }}>
          Featured matchup — Week {week}
        </h2>
        {status === 'live' ? (
          <span
            className="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-semibold"
            style={{ color: 'var(--status-info-fg)', backgroundColor: 'var(--status-info-bg)' }}
          >
            <span
              aria-hidden="true"
              className="h-1.5 w-1.5 rounded-full motion-safe:animate-pulse motion-reduce:animate-none"
              style={{ backgroundColor: 'var(--status-info-fg)' }}
            />
            Live
          </span>
        ) : (
          <span
            className="rounded-full border px-2.5 py-1 text-xs font-semibold"
            style={{ color: 'var(--text-secondary)', borderColor: 'var(--border-subtle)' }}
          >
            Final
          </span>
        )}
      </div>

      <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        {[
          { team: homeTeam, leading: homeLeading },
          { team: awayTeam, leading: awayLeading },
        ].map(({ team, leading }) => (
          <div
            key={team.name}
            className="flex items-center justify-between gap-3 rounded-md p-3"
            style={{ backgroundColor: 'var(--surface-sunken)' }}
          >
            <div className="min-w-0">
              <dt
                className="truncate text-sm"
                style={{
                  color: 'var(--text-primary)',
                  fontWeight: leading ? 700 : 500,
                }}
              >
                {team.name}
              </dt>
              <dd className="mt-0.5 flex items-center gap-2 text-xs" style={{ color: 'var(--text-secondary)' }}>
                <span>{team.record}</span>
                {leading && (
                  <span className="font-semibold" style={{ color: 'var(--action-primary)' }}>
                    Leading
                  </span>
                )}
              </dd>
            </div>
            <dd
              className="shrink-0 tabular-nums"
              style={{
                color: 'var(--text-primary)',
                fontSize: '1.5rem',
                fontWeight: leading ? 700 : 500,
              }}
            >
              {team.score.toFixed(1)}
            </dd>
          </div>
        ))}
      </dl>

      <div className="mt-3">
        <div
          className="flex h-2 w-full overflow-hidden rounded-full"
          style={{ backgroundColor: 'var(--surface-sunken)' }}
          role="img"
          aria-label={`Win probability: ${homeTeam.name} ${homeWinProbability}%, ${awayTeam.name} ${100 - homeWinProbability}%`}
        >
          <div
            style={{
              width: `${homeWinProbability}%`,
              backgroundColor: 'var(--action-primary)',
            }}
          />
        </div>
        <div className="mt-1 flex justify-between text-xs" style={{ color: 'var(--text-muted)' }}>
          <span>{homeTeam.name} {homeWinProbability}%</span>
          <span>{awayTeam.name} {100 - homeWinProbability}%</span>
        </div>
      </div>
    </section>
  );
}
