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
 * current route carries a `leagueId`. League-scoped navigation keeps a
 * persistent Home destination so users can return to the global routes.
 * `/admin` and `/transactions` remain reachable by direct URL only.
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
  exact?: boolean;
}

export interface MoreMenuItem {
  label: string;
  href: (leagueId?: string) => string;
}

export const HOME_NAV_ITEM: ShellNavItem = {
  id: "home",
  label: "Home",
  href: () => "/",
  exact: true,
};

// League-scoped context (a leagueId is present in the route).
export const LEAGUE_NAV_ITEMS: ShellNavItem[] = [
  HOME_NAV_ITEM,
  { id: "overview", label: "Overview", href: (id) => `/league/${id}`, exact: true },
  { id: "schedule", label: "Schedule", href: (id) => `/league/${id}/schedule` },
  { id: "teams", label: "Teams", href: (id) => `/league/${id}/teams` },
  { id: "players", label: "Players", href: () => "/players" },
];

export const LEAGUE_MORE_ITEMS: MoreMenuItem[] = [
  { label: "Simulations", href: (id) => `/league/${id}/simulations` },
  { label: "Transactions", href: (id) => `/league/${id}/transactions` },
];

// Global context (no leagueId in the route: home, /players, /admin, /trades, /drafts).
// Only 4 real destinations exist here, so all 4 are shown directly with no
// "More" overflow needed (unlike the league-scoped context, which also
// exposes simulations and transactions).
export const GLOBAL_NAV_ITEMS: ShellNavItem[] = [
  HOME_NAV_ITEM,
  { id: "players", label: "Players", href: () => "/players" },
  { id: "trades", label: "Trade Data", href: () => "/trades" },
  { id: "drafts", label: "Draft Data", href: () => "/drafts" },
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
