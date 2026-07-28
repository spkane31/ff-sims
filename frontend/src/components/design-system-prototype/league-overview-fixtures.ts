/**
 * Local mock data for the league-overview prototype page (Task 6).
 *
 * Everything here is a plausible, hand-authored fixture — there is no API
 * call backing this page. Content categories mirror the production league
 * page's information architecture (featured matchup, summary metrics,
 * scoring chart, standings) without reusing any of its styling.
 *
 * This file intentionally lives outside `src/pages/` rather than alongside
 * `league-overview.tsx`: Next.js's pages router treats every module under
 * `pages/` as a route and fails the production build with "page without a
 * React Component as default export" for any file there that doesn't
 * default-export a component (confirmed locally before writing this file).
 */

export interface FeaturedMatchupFixture {
  week: number;
  status: 'live' | 'final';
  homeTeam: { name: string; record: string; score: number };
  awayTeam: { name: string; record: string; score: number };
  homeWinProbability: number;
}

export const FEATURED_MATCHUP: FeaturedMatchupFixture = {
  week: 14,
  status: 'live',
  homeTeam: { name: 'Gridiron Gremlins', record: '9-4', score: 102.4 },
  awayTeam: { name: 'End Zone Enigmas', record: '8-5', score: 96.8 },
  homeWinProbability: 63,
};

export interface SummaryMetricFixture {
  id: string;
  label: string;
  value: string;
  helpText: string;
}

export const SUMMARY_METRICS: SummaryMetricFixture[] = [
  {
    id: 'total-points',
    label: 'Total points scored',
    value: '1,284.6',
    helpText: 'League-wide, week 14',
  },
  {
    id: 'avg-margin',
    label: 'Average margin',
    value: '18.4',
    helpText: 'Points per matchup, week 14',
  },
  {
    id: 'completed-games',
    label: 'Completed games',
    value: '5 of 6',
    helpText: '1 matchup still live',
  },
  {
    id: 'highest-score',
    label: 'Highest score',
    value: '142.1',
    helpText: 'Gridiron Gremlins, week 12',
  },
];

export interface WeeklyMarginFixture {
  week: string;
  margin: number;
}

/** Weekly scoring margin (points for minus points against) for the mock
 * user's team — a single-series chart, colored by sign using
 * `--chart-positive`/`--chart-negative` rather than a categorical series
 * palette. */
export const WEEKLY_MARGIN: WeeklyMarginFixture[] = [
  { week: 'W7', margin: 12.4 },
  { week: 'W8', margin: -6.1 },
  { week: 'W9', margin: 24.8 },
  { week: 'W10', margin: -14.2 },
  { week: 'W11', margin: 8.6 },
  { week: 'W12', margin: 31.5 },
  { week: 'W13', margin: -3.4 },
  { week: 'W14', margin: 5.6 },
];

export interface StandingsRowFixture {
  rank: number;
  team: string;
  wins: number;
  losses: number;
  pointsFor: number;
}

export const STANDINGS: StandingsRowFixture[] = [
  { rank: 1, team: 'Gridiron Gremlins', wins: 9, losses: 4, pointsFor: 1487.2 },
  { rank: 2, team: 'End Zone Enigmas', wins: 8, losses: 5, pointsFor: 1452.9 },
  { rank: 3, team: 'Waiver Wire Wizards', wins: 8, losses: 5, pointsFor: 1418.3 },
  { rank: 4, team: 'Blitz Brigade', wins: 7, losses: 6, pointsFor: 1390.6 },
  { rank: 5, team: 'Hail Mary Heroes', wins: 6, losses: 7, pointsFor: 1355.1 },
  { rank: 6, team: 'Red Zone Renegades', wins: 5, losses: 8, pointsFor: 1301.7 },
  { rank: 7, team: 'Fumble Recovery Unit', wins: 3, losses: 10, pointsFor: 1204.4 },
];
