/**
 * Shared nav destination list for the design-system shell prototype.
 *
 * Five items, per task-5-brief.md's "persistent five-item bottom
 * navigation". The same list drives both the bottom nav (below `lg`) and
 * the top bar's primary nav (`lg` and up) — see responsive-rules.md
 * Section 2, which specifies a single shell breakpoint governing both.
 *
 * Only `/design-system/league-overview` (Task 6) is expected to exist as a
 * real route right now; the other hrefs are placeholders for future
 * prototype pages and will 404 until built.
 */

export type ShellNavId = 'overview' | 'schedule' | 'players' | 'teams' | 'more';

export interface ShellNavItem {
  id: ShellNavId;
  label: string;
  href: string;
}

export const SHELL_NAV_ITEMS: ShellNavItem[] = [
  { id: 'overview', label: 'Overview', href: '/design-system/league-overview' },
  { id: 'schedule', label: 'Schedule', href: '/design-system/schedule' },
  { id: 'players', label: 'Players', href: '/design-system/players' },
  { id: 'teams', label: 'Teams', href: '/design-system/teams' },
  { id: 'more', label: 'More', href: '/design-system/more' },
];
