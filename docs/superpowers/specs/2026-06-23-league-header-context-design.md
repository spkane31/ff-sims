# League Context in Nav Header

**Issue:** #56  
**Date:** 2026-06-23  
**Status:** Approved

## Context

Part of the multi-league migration. With routing now under `/league/[leagueId]/`, the header needs to surface which league the user is viewing and let them return to the league selector.

The "All Leagues" back-link already exists in `Header.tsx`. What's missing is the league name and platform badge replacing the generic "The League" app title when inside a league.

## Design

### Scope

Two files change:

1. `frontend/src/hooks/useLeagues.ts` — add `useLeague(leagueId)` hook
2. `frontend/src/components/Header.tsx` — consume the hook, swap the title

No other files change.

### Data Layer

Add `useLeague(leagueId: number | undefined)` to `useLeagues.ts`:

- When `leagueId` is `undefined`, returns `{ league: null, isLoading: false, error: null }` immediately (no fetch).
- When `leagueId` is defined, calls `leaguesService.getLeague(leagueId)` and manages the standard `{ league, isLoading, error }` state.
- Re-fetches if `leagueId` changes.

### Header Title Behavior

| State | Title area renders |
|---|---|
| No `leagueId` in route | `"The League"` linked to `/` |
| `leagueId` present, loading | Placeholder (e.g., `"Loading…"` or a short gray bar) |
| `leagueId` present, loaded | League name linked to `/league/${lid}` + platform badge |
| `leagueId` present, error | Falls back to `"The League"` silently |

### Platform Badge

- Small rounded pill (`<span>`) rendered inline after the league name.
- Text: platform value uppercased (e.g., `espn` → `ESPN`).
- Colors: `espn` → red background (`bg-red-600`); any other platform → gray (`bg-gray-500`).
- Size: `text-xs px-2 py-0.5 rounded-full font-semibold`.

### What Stays the Same

- "All Leagues" link in desktop and mobile nav (already present, no changes).
- League-scoped nav items (League, Simulations, Schedule, Teams, Players, Transactions).
- Mobile hamburger menu behavior.
