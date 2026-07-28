import Link from 'next/link';
import Head from 'next/head';
import AppShell from '@/components/design-system-prototype/AppShell';

const FOCUS_RING =
  'outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]';

/**
 * Landing page for the isolated `/design-system` prototype tree (Phase 1,
 * Task 5). This tree exists to prototype the redesigned UI in isolation
 * from production pages/components — nothing under this route is wired
 * to real data yet.
 */
export default function DesignSystemIndex() {
  return (
    <>
      <Head>
        <title>Design system prototype</title>
      </Head>
      <AppShell activeNavId="overview">
        <div className="mx-auto max-w-2xl">
          <h1 className="text-2xl font-semibold" style={{ color: 'var(--text-primary)' }}>
            Design system prototype
          </h1>
          <p className="mt-2 text-sm" style={{ color: 'var(--text-secondary)' }}>
            Isolated prototype tree for the app-shell redesign (Phase 1,
            Task 5). Pages here render inside <code>AppShell</code> and use
            only the semantic tokens from <code>globals.css</code> — no
            hardcoded colors. Try the theme toggle in the top bar, and
            resize the window across 1024px to see the shell switch
            between bottom nav (mobile) and top nav (desktop).
          </p>

          <ul className="mt-6 space-y-3">
            <li
              className="rounded-lg border p-4"
              style={{
                borderColor: 'var(--border-subtle)',
                backgroundColor: 'var(--surface-raised)',
              }}
            >
              <Link
                href="/design-system/league-overview"
                className={`text-base font-medium underline-offset-2 hover:underline ${FOCUS_RING}`}
                style={{ color: 'var(--action-primary)' }}
              >
                League overview prototype
              </Link>
              <p className="mt-1 text-sm" style={{ color: 'var(--text-muted)' }}>
                Task 6 — compact league overview page built on this shell.
              </p>
            </li>
          </ul>
        </div>
      </AppShell>
    </>
  );
}
