import Link from 'next/link';
import { SHELL_NAV_ITEMS, type ShellNavId } from './nav-items';
import LeagueSwitcher from './LeagueSwitcher';
import FilterEntryPoint from './FilterEntryPoint';
import ThemeToggle, { type ThemeMode } from './ThemeToggle';
import {
  OverviewIcon,
  ScheduleIcon,
  PlayersIcon,
  TeamsIcon,
  MoreIcon,
} from './icons';

const NAV_ICONS: Record<ShellNavId, typeof OverviewIcon> = {
  overview: OverviewIcon,
  schedule: ScheduleIcon,
  players: PlayersIcon,
  teams: TeamsIcon,
  more: MoreIcon,
};

const FOCUS_RING =
  'outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]';

interface TopBarProps {
  leagueName: string;
  activeNavId: ShellNavId;
  themeMode: ThemeMode;
  onThemeChange: (mode: ThemeMode) => void;
}

/**
 * Top bar: league switcher + filter entry-point at every width (base
 * through 2xl), plus the primary nav link group which is hidden below
 * `lg` (1024px) and shown from `lg` up — responsive-rules.md Section 2's
 * "Implementation shape for Task 5: ... the top bar's primary-nav link
 * group uses `hidden lg:flex`." Below `lg`, BottomNav.tsx is the primary
 * nav instead.
 */
export default function TopBar({
  leagueName,
  activeNavId,
  themeMode,
  onThemeChange,
}: TopBarProps) {
  return (
    <header
      className="sticky top-0 z-30 flex items-center justify-between gap-3 border-b px-3 py-2 sm:px-4"
      style={{
        backgroundColor: 'var(--surface-raised)',
        borderColor: 'var(--border-subtle)',
      }}
    >
      <div className="flex items-center gap-3">
        <LeagueSwitcher leagueName={leagueName} />
      </div>

      <nav
        aria-label="Primary"
        className="hidden flex-1 items-center justify-center gap-1 lg:flex"
      >
        {SHELL_NAV_ITEMS.map((item) => {
          const Icon = NAV_ICONS[item.id];
          const active = item.id === activeNavId;

          return (
            <Link
              key={item.id}
              href={item.href}
              aria-current={active ? 'page' : undefined}
              className={`flex min-h-11 items-center gap-2 rounded-md border-b-2 px-3 text-sm ${FOCUS_RING}`}
              style={{
                borderColor: active ? 'var(--action-primary)' : 'transparent',
                color: active ? 'var(--text-primary)' : 'var(--text-secondary)',
                fontWeight: active ? 600 : 500,
              }}
            >
              <Icon className="h-4 w-4" active={active} />
              {item.label}
            </Link>
          );
        })}
      </nav>

      <div className="flex items-center gap-2">
        <FilterEntryPoint />
        <ThemeToggle mode={themeMode} onChange={onThemeChange} />
      </div>
    </header>
  );
}
