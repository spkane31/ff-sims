import type { StandingsRowFixture } from './league-overview-fixtures';

interface StandingsListProps {
  standings: StandingsRowFixture[];
}

const FOCUS_RING =
  'outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]';

function formatPointsFor(value: number): string {
  return value.toLocaleString(undefined, { minimumFractionDigits: 1, maximumFractionDigits: 1 });
}

/**
 * Compact standings list.
 *
 * Follows responsive-rules.md Section 3's treatment for the league-overview
 * standings table ("rank, team, W-L, points for"): ranked-list-row below
 * `md` (768px), a standard compact `<table>` at `md` and above. Both
 * renderings exist in the DOM and are toggled with Tailwind's `md:`
 * prefix, matching the shell's existing bottom-nav/top-nav swap pattern.
 */
export default function StandingsList({ standings }: StandingsListProps) {
  return (
    <section aria-label="Standings">
      {/* Below md: ranked-list-row layout — one row per team, headline
          values (W-L, PF) inline rather than stacked as labelled fields. */}
      <ol className="flex flex-col gap-2 md:hidden">
        {standings.map((row) => (
          <li
            key={row.team}
            className="flex items-center gap-3 rounded-md border p-3"
            style={{ borderColor: 'var(--border-subtle)', backgroundColor: 'var(--surface-raised)' }}
          >
            <span
              className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold"
              style={{ backgroundColor: 'var(--surface-sunken)', color: 'var(--text-secondary)' }}
            >
              {row.rank}
            </span>
            <span className="min-w-0 flex-1 truncate text-sm font-medium" style={{ color: 'var(--text-primary)' }}>
              {row.team}
            </span>
            <span className="shrink-0 text-xs tabular-nums" style={{ color: 'var(--text-secondary)' }}>
              {row.wins}-{row.losses}
            </span>
            <span className="shrink-0 text-xs tabular-nums" style={{ color: 'var(--text-muted)' }}>
              {formatPointsFor(row.pointsFor)} PF
            </span>
          </li>
        ))}
      </ol>

      {/* md and up: standard compact table. */}
      <table className="hidden w-full text-sm md:table">
        <thead>
          <tr className="border-b" style={{ borderColor: 'var(--border-subtle)' }}>
            <th scope="col" className="px-2 py-2 text-left font-medium" style={{ color: 'var(--text-secondary)' }}>
              Rank
            </th>
            <th scope="col" className="px-2 py-2 text-left font-medium" style={{ color: 'var(--text-secondary)' }}>
              Team
            </th>
            <th scope="col" className="px-2 py-2 text-right font-medium" style={{ color: 'var(--text-secondary)' }}>
              W-L
            </th>
            <th scope="col" className="px-2 py-2 text-right font-medium" style={{ color: 'var(--text-secondary)' }}>
              PF
            </th>
          </tr>
        </thead>
        <tbody>
          {standings.map((row) => (
            <tr key={row.team} className="border-b last:border-0" style={{ borderColor: 'var(--border-subtle)' }}>
              <td className="px-2 py-2" style={{ color: 'var(--text-secondary)' }}>
                {row.rank}
              </td>
              <td className="px-2 py-2 font-medium" style={{ color: 'var(--text-primary)' }}>
                {row.team}
              </td>
              <td className="px-2 py-2 text-right tabular-nums" style={{ color: 'var(--text-secondary)' }}>
                {row.wins}-{row.losses}
              </td>
              <td className="px-2 py-2 text-right tabular-nums" style={{ color: 'var(--text-muted)' }}>
                {formatPointsFor(row.pointsFor)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <button
        type="button"
        aria-disabled="true"
        onClick={(event) => event.preventDefault()}
        title="Full standings view isn't part of this prototype yet"
        className={`mt-3 inline-flex min-h-11 items-center rounded-md border px-3 text-sm font-medium ${FOCUS_RING}`}
        style={{
          borderColor: 'var(--border-subtle)',
          color: 'var(--text-muted)',
          backgroundColor: 'var(--surface-sunken)',
        }}
      >
        View all standings
      </button>
    </section>
  );
}
