import Link from 'next/link';
import { SHELL_NAV_ITEMS, type ShellNavId } from './nav-items';
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

interface BottomNavProps {
  activeNavId: ShellNavId;
}

/**
 * Persistent five-item bottom navigation.
 *
 * Per responsive-rules.md Section 2, this is removed from the layout (not
 * just visually hidden) at `lg` (1024px) and up via `lg:hidden`, so it
 * also leaves the tab order at that width — TopBar's primary nav takes
 * over. Each item keeps a 44px min hit area per Section 5, and the
 * current item pairs a color change with `aria-current="page"` plus a
 * font-weight + top-border change (Section 6: never color alone).
 */
export default function BottomNav({ activeNavId }: BottomNavProps) {
  return (
    <nav
      aria-label="Primary"
      className="fixed inset-x-0 bottom-0 z-30 flex border-t lg:hidden"
      style={{
        backgroundColor: 'var(--surface-raised)',
        borderColor: 'var(--border-subtle)',
      }}
    >
      {SHELL_NAV_ITEMS.map((item) => {
        const Icon = NAV_ICONS[item.id];
        const active = item.id === activeNavId;

        return (
          <Link
            key={item.id}
            href={item.href}
            aria-current={active ? 'page' : undefined}
            className={`flex min-h-11 flex-1 flex-col items-center justify-center gap-0.5 border-t-2 py-1.5 text-xs ${FOCUS_RING}`}
            style={{
              borderColor: active ? 'var(--action-primary)' : 'transparent',
              color: active ? 'var(--action-primary)' : 'var(--text-muted)',
              fontWeight: active ? 600 : 500,
            }}
          >
            <Icon className="h-5 w-5" active={active} />
            {item.label}
          </Link>
        );
      })}
    </nav>
  );
}
