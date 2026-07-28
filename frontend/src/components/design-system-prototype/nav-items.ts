import type { ComponentType } from 'react';
import {
  type IconProps,
  OverviewIcon,
  ScheduleIcon,
  PlayersIcon,
  TeamsIcon,
  MoreIcon,
} from './icons';

/**
 * Shared nav destination list for the design-system shell prototype.
 *
 * Five items, per task-5-brief.md's "persistent five-item bottom
 * navigation". The same list drives both the bottom nav (below `lg`) and
 * the top bar's primary nav (`lg` and up) — see responsive-rules.md
 * Section 2, which specifies a single shell breakpoint governing both.
 *
 * Only `/design-system/league-overview` (Task 6) is a real route today.
 * Schedule/Players/Teams/More all point at it too for now — they'll get
 * their own prototype pages in a later phase; pointing them at their
 * "real" (but nonexistent) paths would 404.
 */

export type ShellNavId = 'overview' | 'schedule' | 'players' | 'teams' | 'more';

export interface ShellNavItem {
  id: ShellNavId;
  label: string;
  href: string;
}

export const SHELL_NAV_ITEMS: ShellNavItem[] = [
  { id: 'overview', label: 'Overview', href: '/design-system/league-overview' },
  { id: 'schedule', label: 'Schedule', href: '/design-system/league-overview' },
  { id: 'players', label: 'Players', href: '/design-system/league-overview' },
  { id: 'teams', label: 'Teams', href: '/design-system/league-overview' },
  { id: 'more', label: 'More', href: '/design-system/league-overview' },
];

/**
 * Nav icon components, keyed by nav id, shared between TopBar (desktop
 * primary nav) and BottomNav (mobile primary nav) — previously duplicated
 * byte-for-byte in both files.
 */
export const NAV_ICONS: Record<ShellNavId, ComponentType<IconProps>> = {
  overview: OverviewIcon,
  schedule: ScheduleIcon,
  players: PlayersIcon,
  teams: TeamsIcon,
  more: MoreIcon,
};
