import type { ComponentType } from "react";
import {
  type IconProps,
  OverviewIcon,
  ScheduleIcon,
  PlayersIcon,
  TeamsIcon,
  MoreIcon,
} from "./icons";

/**
 * Real nav destinations, corrected against the app's existing
 * `Header.tsx` (which this replaces): nav content depends on whether the
 * current route carries a `leagueId`. No destination is added or removed
 * relative to what `Header.tsx` already links to today — in particular,
 * `/admin` and `/sleeper/transactions` are reachable by direct URL only,
 * not from nav, and that stays true here.
 */

export type ShellNavId =
  | "overview"
  | "schedule"
  | "players"
  | "teams"
  | "more"
  | "home"
  | "trades"
  | "drafts";

export interface ShellNavItem {
  id: ShellNavId;
  label: string;
  href: (leagueId?: string) => string;
}

export interface MoreMenuItem {
  label: string;
  href: (leagueId?: string) => string;
}

// League-scoped context (a leagueId is present in the route).
export const LEAGUE_NAV_ITEMS: ShellNavItem[] = [
  { id: "overview", label: "Overview", href: (id) => `/league/${id}` },
  { id: "schedule", label: "Schedule", href: (id) => `/league/${id}/schedule` },
  { id: "teams", label: "Teams", href: (id) => `/league/${id}/teams` },
  { id: "players", label: "Players", href: () => "/players" },
];

export const LEAGUE_MORE_ITEMS: MoreMenuItem[] = [
  { label: "Simulations", href: (id) => `/league/${id}/simulations` },
  { label: "Transactions", href: (id) => `/league/${id}/transactions` },
  { label: "All Leagues", href: () => "/" },
];

// Global context (no leagueId in the route: home, /players, /admin, /sleeper/*).
// Only 4 real destinations exist here, so all 4 are shown directly with no
// "More" overflow needed (unlike the league-scoped context's 4 primary + 3
// overflow, which doesn't fit in 5 slots without one).
export const GLOBAL_NAV_ITEMS: ShellNavItem[] = [
  { id: "home", label: "Home", href: () => "/" },
  { id: "players", label: "Players", href: () => "/players" },
  { id: "trades", label: "Trade Data", href: () => "/sleeper/trades" },
  { id: "drafts", label: "Draft Data", href: () => "/sleeper/drafts" },
];

export const NAV_ICONS: Record<ShellNavId, ComponentType<IconProps>> = {
  overview: OverviewIcon,
  schedule: ScheduleIcon,
  players: PlayersIcon,
  teams: TeamsIcon,
  more: MoreIcon,
  home: OverviewIcon,
  trades: ScheduleIcon,
  drafts: TeamsIcon,
};
