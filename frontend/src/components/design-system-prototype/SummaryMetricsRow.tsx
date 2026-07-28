import type { SummaryMetricFixture } from './league-overview-fixtures';
import { FOCUS_RING } from './focus-ring';

export type MetricsViewState = 'default' | 'loading' | 'empty' | 'error';

interface SummaryMetricsRowProps {
  metrics: SummaryMetricFixture[];
  viewState: MetricsViewState;
  onRetry: () => void;
}

const TILE_STYLE = {
  backgroundColor: 'var(--surface-raised)',
  borderColor: 'var(--border-subtle)',
};

function LoadingTiles() {
  return (
    <div className="flex gap-3 overflow-x-auto pb-1" aria-hidden="true">
      {Array.from({ length: 4 }).map((_, index) => (
        <div
          key={index}
          className="min-w-[9.5rem] shrink-0 snap-start rounded-lg border p-4"
          style={TILE_STYLE}
        >
          <div
            className="h-3 w-20 rounded motion-safe:animate-pulse motion-reduce:animate-none"
            style={{ backgroundColor: 'var(--surface-sunken)' }}
          />
          <div
            className="mt-3 h-6 w-16 rounded motion-safe:animate-pulse motion-reduce:animate-none"
            style={{ backgroundColor: 'var(--surface-sunken)' }}
          />
          <div
            className="mt-2 h-2.5 w-24 rounded motion-safe:animate-pulse motion-reduce:animate-none"
            style={{ backgroundColor: 'var(--surface-sunken)' }}
          />
        </div>
      ))}
    </div>
  );
}

function EmptyPanel() {
  return (
    <div
      className="rounded-lg border border-dashed p-6 text-center"
      style={{ borderColor: 'var(--border-subtle)' }}
    >
      <p className="text-sm font-medium" style={{ color: 'var(--text-secondary)' }}>
        No metrics available yet
      </p>
      <p className="mt-1 text-xs" style={{ color: 'var(--text-muted)' }}>
        Metrics appear once this week&apos;s matchups have scores to summarize.
      </p>
    </div>
  );
}

function ErrorPanel({ onRetry }: { onRetry: () => void }) {
  return (
    <div
      role="alert"
      className="flex flex-col items-start gap-2 rounded-lg border p-4 sm:flex-row sm:items-center sm:justify-between"
      style={{ borderColor: 'var(--status-danger-fg)', backgroundColor: 'var(--status-danger-bg)' }}
    >
      <p className="text-sm font-medium" style={{ color: 'var(--status-danger-fg)' }}>
        Couldn&apos;t load summary metrics. Check your connection and try again.
      </p>
      <button
        type="button"
        onClick={onRetry}
        className={`inline-flex min-h-11 items-center rounded-md border px-3 text-sm font-semibold ${FOCUS_RING}`}
        style={{ borderColor: 'var(--status-danger-fg)', color: 'var(--status-danger-fg)' }}
      >
        Retry
      </button>
    </div>
  );
}

/**
 * Horizontally-scrollable row of summary metric tiles, with default/
 * loading/empty/error variants driven by the page's mock `viewState`
 * toggle (task-6-brief.md: "reachable via a simple local state toggle...
 * not a real async flow").
 *
 * The whole region is one `aria-live="polite"` container so switching
 * viewState is announced to assistive tech the same way a real fetch
 * transition would be (component-state-matrix.md: loading sets
 * `aria-busy`/`aria-live=polite`; error uses `role=alert`).
 */
export default function SummaryMetricsRow({ metrics, viewState, onRetry }: SummaryMetricsRowProps) {
  return (
    <div aria-live="polite" aria-busy={viewState === 'loading'}>
      {viewState === 'loading' && <LoadingTiles />}
      {viewState === 'empty' && <EmptyPanel />}
      {viewState === 'error' && <ErrorPanel onRetry={onRetry} />}
      {viewState === 'default' && (
        <div className="flex snap-x gap-3 overflow-x-auto pb-1">
          {metrics.map((metric) => (
            <div
              key={metric.id}
              className="min-w-[9.5rem] shrink-0 snap-start rounded-lg border p-4"
              style={TILE_STYLE}
            >
              <p className="text-xs font-medium" style={{ color: 'var(--text-secondary)' }}>
                {metric.label}
              </p>
              <p
                className="mt-1 text-2xl font-semibold tabular-nums"
                style={{ color: 'var(--text-primary)' }}
              >
                {metric.value}
              </p>
              <p className="mt-1 text-xs" style={{ color: 'var(--text-muted)' }}>
                {metric.helpText}
              </p>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
