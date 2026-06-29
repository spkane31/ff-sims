# Hall of Fame / Wall of Shame & All-Time Team Records

**Date:** 2026-06-20  
**Status:** Approved

## Overview

Port two historical-stats sections from the old single-league home page (`v2/frontend/src/pages/index.tsx`) to the new per-league home page (`v2/frontend/src/pages/league/[leagueId]/index.tsx`), making them work for any league.

## Components

### 1. `HallOfFameWallOfShame.tsx`

**Location:** `v2/frontend/src/components/HallOfFameWallOfShame.tsx`

**Props:**
```ts
interface Props {
  leagueId: number;
  schedule: ScheduleResponse | null;   // already loaded by parent via useSchedule(id)
  isLoading: boolean;
}
```

**Logic:** Ports `calculateWinnersAndLosers` from `index.tsx` verbatim with two changes:
- `currentYear` becomes `new Date().getFullYear()` (was hardcoded `2025`)
- Team links use `/league/${leagueId}/teams/${espnId}` (was `/teams/${espnId}`)

**Champion detection:** Team with the most `WINNERS_BRACKET` playoff wins per year.  
**Last-place detection:** Team with the most regular-season losses per year (tiebreak: fewest points scored).  
**Completed season filter:** Excludes the current year and any year with incomplete regular-season games.

**Renders:** Two side-by-side cards (gold / red gradient) — Hall of Fame (🏆) on the left, Wall of Shame (💩) on the right — each listing one entry per completed year sorted newest-first. Shows a loading placeholder row while `isLoading` is true.

---

### 2. `AllTimeRecordsTable.tsx`

**Location:** `v2/frontend/src/components/AllTimeRecordsTable.tsx`

**Props:**
```ts
interface Props {
  leagueId: number;
}
```

**Data fetching:** Fetches its own data on mount via `Promise.all`:
- `teamsService.getAllTeams(leagueId)` — provides `record`, `playoffRecord`, `points`
- `expectedWinsService.getAllTimeExpectedWins(leagueId)` — provides `expectedWins`, `expectedLosses`, `winLuck`

Merges the two by matching `team_id` or `owner` name, same pattern as the old page.

**No ESPN-ID filtering.** The old page filtered out IDs "2" and "8" as a single-league workaround; the new component is already league-scoped so all teams are shown.

**Columns (all sortable):** Owner · Regular Season Record · Playoff Record · Points For · Points Against · Expected Record · Luck  
**Default sort:** Regular Season Record descending (most wins first).  
**Luck coloring:** green if > 0, red if < 0, gray if 0 or unavailable.  
**Team links:** `/league/${leagueId}/teams/${espnId}`

---

## Integration

Both components are added to `league/[leagueId]/index.tsx` below the existing `AllTimeMatchupsGrid`, in this order:

1. `<HallOfFameWallOfShame>` — passes the already-loaded `schedule` and `isLoading` from `useSchedule(id)`
2. `<AllTimeRecordsTable>` — fetches its own data

No new API endpoints needed. All data is available from existing routes.

## Out of Scope

- No changes to backend
- No changes to existing hooks or services
- No changes to any other page
