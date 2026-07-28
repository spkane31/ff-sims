import { useState } from 'react';
import Head from 'next/head';
import AppShell from '@/components/design-system-prototype/AppShell';
import FeaturedMatchupCard from '@/components/design-system-prototype/FeaturedMatchupCard';
import SummaryMetricsRow, {
  type MetricsViewState,
} from '@/components/design-system-prototype/SummaryMetricsRow';
import ScoringMarginChart from '@/components/design-system-prototype/ScoringMarginChart';
import StandingsList from '@/components/design-system-prototype/StandingsList';
import {
  FEATURED_MATCHUP,
  SUMMARY_METRICS,
  WEEKLY_MARGIN,
  STANDINGS,
} from '@/components/design-system-prototype/league-overview-fixtures';

const FOCUS_RING =
  'outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]';

const VIEW_STATE_OPTIONS: { id: MetricsViewState; label: string }[] = [
  { id: 'default', label: 'Default' },
  { id: 'loading', label: 'Loading' },
  { id: 'empty', label: 'Empty' },
  { id: 'error', label: 'Error' },
];

function SectionHeading({ children }: { children: React.ReactNode }) {
  return (
    <h2 className="mb-2 text-sm font-semibold" style={{ color: 'var(--text-primary)' }}>
      {children}
    </h2>
  );
}

/**
 * League overview prototype page (Phase 1, Task 6).
 *
 * Renders inside `AppShell` using only local mock data — no API calls, per
 * task-6-brief.md. Content categories (featured matchup, summary metrics,
 * a single-series chart, a standings list) mirror the production league
 * page's information architecture only; none of its styling is reused.
 */
export default function LeagueOverviewPage() {
  const [metricsViewState, setMetricsViewState] = useState<MetricsViewState>('default');

  return (
    <>
      <Head>
        <title>League overview — Design system prototype</title>
      </Head>
      <AppShell activeNavId="overview">
        <div className="mx-auto flex max-w-3xl flex-col gap-6">
          <div>
            <h1 className="text-2xl font-semibold" style={{ color: 'var(--text-primary)' }}>
              League overview
            </h1>
            <p className="mt-1 text-sm" style={{ color: 'var(--text-secondary)' }}>
              Prototype page built on the Task 5 app shell, using local mock
              data only.
            </p>
          </div>

          <FeaturedMatchupCard matchup={FEATURED_MATCHUP} />

          <section aria-label="Summary metrics">
            <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
              <SectionHeading>Summary metrics</SectionHeading>
              <div
                role="group"
                aria-label="Preview summary metrics states (prototype toggle only)"
                className="flex gap-1"
              >
                {VIEW_STATE_OPTIONS.map((option) => {
                  const active = option.id === metricsViewState;
                  return (
                    <button
                      key={option.id}
                      type="button"
                      aria-pressed={active}
                      onClick={() => setMetricsViewState(option.id)}
                      className={`min-h-11 rounded-md border px-2.5 text-xs ${FOCUS_RING}`}
                      style={{
                        borderColor: active ? 'var(--action-primary)' : 'var(--border-subtle)',
                        backgroundColor: active ? 'var(--action-primary)' : 'var(--surface-raised)',
                        color: active ? 'var(--action-on-primary)' : 'var(--text-secondary)',
                        fontWeight: active ? 600 : 500,
                      }}
                    >
                      {option.label}
                    </button>
                  );
                })}
              </div>
            </div>
            <p className="mb-2 text-xs" style={{ color: 'var(--text-muted)' }}>
              These buttons swap a local mock <code>viewState</code> — there
              is no real network request behind them.
            </p>
            <SummaryMetricsRow
              metrics={SUMMARY_METRICS}
              viewState={metricsViewState}
              onRetry={() => setMetricsViewState('default')}
            />
          </section>

          <section aria-label="Scoring trend">
            <SectionHeading>Scoring margin trend</SectionHeading>
            <div
              className="rounded-lg border p-4"
              style={{ borderColor: 'var(--border-subtle)', backgroundColor: 'var(--surface-raised)' }}
            >
              <ScoringMarginChart data={WEEKLY_MARGIN} />
            </div>
          </section>

          <div>
            <SectionHeading>Standings</SectionHeading>
            <StandingsList standings={STANDINGS} />
          </div>
        </div>
      </AppShell>
    </>
  );
}
