# Multi-League Migration Design

**Date:** 2026-06-17
**Status:** Approved
**Branch strategy:** Feature branch → multiple PRs → merge to main when validated against dev Postgres

## Overview

Migrate the existing single-league fantasy football site to support multiple leagues. The schema already has `league_id` on most tables; the work is fixing a constraint, adding two fields, cleaning up redundancy, parameterizing the ETL, and restructuring the API and frontend routing.

Development happens against a fresh Postgres instance (drop/recreate as needed — no incremental migration scripts during feature work). The CockroachDB export + Postgres import script is only needed at production cutover.

## Phase 1: Schema + Model Changes

### Required for multi-league

**`League` table — add two fields:**
- `platform varchar` — "ESPN", "Sleeper", "Yahoo". Stored as a string, validated by a Go enum/constant. Not a DB enum so it's easy to extend.
- `external_id varchar` — the platform-assigned league ID (e.g. `"345674"` for the existing ESPN league). Replaces the hardcoded `const leagueID = 345674` in `etl/upload.go`.

**`Team` table — fix the unique constraint:**
- Drop `idx_teams_espn_id` (global unique on `espn_id` alone).
- Add composite unique on `(espn_id, league_id)`. ESPN assigns team IDs per-league, not globally; two leagues can share the same ESPN team ID.

`Player.espn_id` keeps its global unique — NFL players are shared across all leagues.

### Schema cleanup (future growth)

**Remove redundant `season` field** from `Matchup` and `BoxScore`. Both models carry `year uint` and `season int` with the same value. The `season` column is dropped; `year` is the canonical field.

**Add `Manager` model** — `owner` is currently a denormalized string on `Team`, so the same person appears as disconnected records across seasons. A `Manager` table (`id`, `name`, `email optional`) with a FK `Team.manager_id` enables cross-season owner analytics (all-time records, head-to-head history). The existing `owner` string column is kept as a display name but `manager_id` becomes the join key.

**Add `player_season_stats` table** — already designed in `TODO.md`. Aggregates box score data per player per season with rankings, average points, and consistency metrics. A background job populates it weekly.

## Phase 2: CockroachDB Export + Postgres Import (production cutover only)

Two-step script committed to `scripts/`:

1. Export from CockroachDB using `pg_dump` (CockroachDB is wire-compatible).
2. Transform + import to Postgres:
   - Replay the dump into the new Postgres instance.
   - One-time SQL fixup: set `platform = 'ESPN'` and `external_id = '345674'` on the existing league row.
   - Drop `idx_teams_espn_id`, create composite unique `(espn_id, league_id)`.
   - Drop the `season` column from `matchups` and `box_scores` after confirming `year` is populated.

This runs alongside Phase 1 validation — the seeded Postgres data is used to verify the API and frontend changes render correctly before cutover is declared done.

## Phase 3: API Restructuring

All endpoints become league-scoped. Flat routes are removed — no backwards-compat shims.

```
/api/v1/leagues                           # list all leagues
/api/v1/leagues/{leagueId}                # single league metadata
/api/v1/leagues/{leagueId}/teams
/api/v1/leagues/{leagueId}/teams/{teamId}
/api/v1/leagues/{leagueId}/schedules
/api/v1/leagues/{leagueId}/players
/api/v1/leagues/{leagueId}/players/{playerId}
/api/v1/leagues/{leagueId}/simulations
/api/v1/leagues/{leagueId}/transactions
/api/v1/leagues/{leagueId}/expected-wins
```

`leagueId` is extracted from the URL path parameter in each handler. The global `leagueID` constant in the ETL and any handler-level hardcoding are removed.

## Phase 4: Frontend Routing

Pages move from root level into `src/pages/league/[leagueId]/`.

```
/                                     # League selector home page (new)
/league/[leagueId]                    # League dashboard
/league/[leagueId]/teams
/league/[leagueId]/teams/[teamId]
/league/[leagueId]/schedule
/league/[leagueId]/players
/league/[leagueId]/players/[playerId]
/league/[leagueId]/simulations
/league/[leagueId]/transactions
```

**Home page (`/`):** Fetches `/api/v1/leagues`, renders a card per league showing name, platform, current season, and a link into that league's dashboard.

**Threading:** `leagueId` is passed through each page's data-fetching hooks and `apiClient.ts` service calls. No page logic changes — route restructuring and parameter threading only.

**UX improvements (fold in during this phase):**
- League name and platform shown in the nav/header when inside a league.
- Back-to-home navigation from any league page.

## Phase 5: ETL Parameterization + Temporal Path

### Now — CLI flag

```
./etl --league-id=345674 --platform=ESPN
```

`cmd/etl/main.go` parses these flags and passes them into ETL functions. The ETL looks up the league by `(external_id, platform)` rather than by internal DB ID. The hardcoded `const leagueID = 345674` is removed.

### Future — Temporal (TODO)

Each league becomes a Temporal workflow on a cron schedule. A single activity handles the data pull, parameterized by `external_id` and `platform`. Adding a new league means registering a new scheduled workflow — no code changes.

The CLI flag implementation maps cleanly onto Temporal activity inputs; the refactor is mechanical. A `// TODO(temporal): migrate to Temporal workflow — see GitHub issue #X` comment is left on the ETL entrypoint.

## GitHub Issues

Issues to be created in `spkane31/ff-sims`:

1. **[Schema] Add `platform` and `external_id` to League; fix Team unique constraint to `(espn_id, league_id)`**
2. **[Schema] Remove redundant `season` field from Matchup and BoxScore**
3. **[Schema] Add Manager model for cross-season owner tracking**
4. **[Schema] Add player_season_stats table with background job**
5. **[ETL] Parameterize ETL with `--league-id` and `--platform` CLI flags**
6. **[ETL] TODO: Migrate ETL to Temporal workflows/activities on a per-league schedule**
7. **[API] Restructure all routes to `/api/v1/leagues/{leagueId}/...`**
8. **[Frontend] Add league selector home page and move all pages under `/league/[leagueId]/`**
9. **[Data] Write CockroachDB export + Postgres import script for production cutover**
10. **[Frontend] UX: league context in nav header and back-to-home navigation**

## Constraints + Decisions

- No backwards-compat API shims — internal app, no external consumers.
- Dev environment: drop/recreate Postgres freely; no incremental migration scripts during feature work.
- `Player.espn_id` global unique is correct — NFL players are shared across leagues.
- Platform stored as `varchar`, not a DB enum, for easy extensibility.
- `year` is the canonical season field; `season` is redundant and will be removed.
- Temporal is the end-goal for ETL scheduling; CLI flag is the bridge.
