import { useState, type ReactNode } from 'react';
import TopBar from './TopBar';
import BottomNav from './BottomNav';
import type { ThemeMode } from './ThemeToggle';
import { type ShellNavId } from './nav-items';

export type { ShellNavId } from './nav-items';
export { SHELL_NAV_ITEMS } from './nav-items';
export type { ThemeMode } from './ThemeToggle';

export interface AppShellProps {
  /** Which of the five nav destinations is current, for the nav's
   * `aria-current`/active styling in both the top bar and bottom nav. */
  activeNavId: ShellNavId;
  /** Mock league name shown in the top bar's league switcher. */
  leagueName?: string;
  /** Page content, rendered in the shell's main content area. */
  children: ReactNode;
}

/**
 * Design-system prototype app shell.
 *
 * Composes the top bar (league switcher, filter entry-point, primary nav
 * at `lg`+, theme toggle) and the persistent five-item bottom nav (below
 * `lg`), per task-5-brief.md and responsive-rules.md Section 2.
 *
 * Theme toggle wiring: AppShell owns the `system | light | dark` state
 * and applies the resulting `light`/`dark` class (or no class, for
 * "system") to its own outermost `<div>` — NOT to `document.documentElement`
 * (`<html>`). This keeps the manual theme override scoped to the
 * `/design-system` prototype tree, since the rest of the app has no theme
 * toggle and should keep following OS preference untouched. Task 6 (or any
 * page under `/design-system`) should render its content as `children` of
 * this component to inherit the token values; it does not need to manage
 * theme state itself.
 */
export default function AppShell({
  activeNavId,
  leagueName = 'Dynasty Warriors',
  children,
}: AppShellProps) {
  const [themeMode, setThemeMode] = useState<ThemeMode>('system');
  const themeClass =
    themeMode === 'system' ? '' : themeMode === 'light' ? 'light' : 'dark';

  return (
    <div
      className={`min-h-screen ${themeClass}`}
      style={{
        backgroundColor: 'var(--surface-canvas)',
        color: 'var(--text-primary)',
        colorScheme: themeMode === 'system' ? undefined : themeMode,
      }}
    >
      <div className="flex min-h-screen flex-col">
        <TopBar
          leagueName={leagueName}
          activeNavId={activeNavId}
          themeMode={themeMode}
          onThemeChange={setThemeMode}
        />

        <main className="flex-1 px-3 pb-20 pt-4 sm:px-4 lg:pb-6">
          {children}
        </main>

        <BottomNav activeNavId={activeNavId} />
      </div>
    </div>
  );
}
