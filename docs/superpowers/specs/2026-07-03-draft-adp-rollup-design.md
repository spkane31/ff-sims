# Draft ADP rollup — design

Date: 2026-07-03
Status: approved, ready for implementation plan

## Problem

`/sleeper/drafts` currently lists individual completed Sleeper drafts (league,
type, season, pick count) with no aggregate signal — not useful for actually
answering "where do players typically get drafted?" This replaces that page
with an Average Draft Position (ADP) report: players ranked by average pick
number, filterable by league size, scoring format, and superflex, computed
from real Sleeper draft data already being synced.

## Scope decisions

- **Draft scope**: only `snake`/`linear` drafts (`sleeper_drafts.type`) from
  `redraft` leagues (`sleeper_leagues.league_type`) count toward ADP.
  - Auction drafts are excluded: `pick_no` there reflects nomination order,
    not draft slot value, so it isn't comparable to snake pick order.
  - Dynasty/keeper drafts are excluded: different player pools and draft
    incentives (rookies, keeper compensation picks) make them incomparable
    to redraft startup pools.
  - Follow-up ideas for auction average-cost and dynasty/keeper ADP are
    tracked in [issue #131](https://github.com/spkane31/ff-sims/issues/131)
    rather than scoped here.
- **Segments**: full cross product of league_size `{8, 10, 12, 14+}` ×
  scoring `{standard, half_ppr, ppr}` × superflex `{yes, no}` = 24 segments.
  Leagues with `total_rosters` outside `{8,10,12,14+}` are excluded from ADP
  entirely (no "Other" bucket). This is a distinct segment key space from
  the existing valuation model's `knownValuationSegments` (which is
  PPR+superflex only) — the two are not unified.
- **Minimum sample size**: a player needs at least 20 qualifying drafts in a
  segment to appear in that segment's ADP list. Enforced at **read time**
  (API query), not at rollup time, so the threshold can change later without
  a recompute.
- **Freshness model**: current-value only. Each daily run upserts one row
  per (segment, player), overwriting the previous value. No dated history /
  trend tracking in this iteration.
- **Partial filters**: the frontend always sends all three filter values.
  Defaults are `league_size=12`, `scoring_format=ppr`, `superflex=true`
  (segment `12-ppr-sf`) — the same "primary" shape the valuation model
  already treats as default. There is no cross-segment aggregation.

## Data model

New migration `backend/migrations/016_draft_adp.sql`, one table:

```sql
CREATE TABLE draft_adp (
    segment           TEXT NOT NULL,
    sleeper_player_id TEXT NOT NULL,
    avg_pick_no       NUMERIC NOT NULL,
    pick_count        INTEGER NOT NULL,
    min_pick_no       INTEGER NOT NULL,
    max_pick_no       INTEGER NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (segment, sleeper_player_id)
);
CREATE INDEX idx_draft_adp_segment_avg_pick ON draft_adp (segment, avg_pick_no);
```

New GORM model `backend/internal/models/draft_adp.go`:

```go
type DraftADP struct {
    Segment         string    `gorm:"primaryKey;column:segment"`
    SleeperPlayerID string    `gorm:"primaryKey;column:sleeper_player_id"`
    AvgPickNo       float64   `gorm:"column:avg_pick_no"`
    PickCount       int       `gorm:"column:pick_count"`
    MinPickNo       int       `gorm:"column:min_pick_no"`
    MaxPickNo       int       `gorm:"column:max_pick_no"`
    UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"`
}
func (DraftADP) TableName() string { return "draft_adp" }
```

Segment key format: `{league_size}-{scoring}-{sf|1qb}`, e.g. `12-ppr-sf`,
`10-half_ppr-1qb`, `8-standard-sf`. Defined as a fixed list `adpSegments` of
24 `(leagueSize, scoring, superflex)` tuples — a sibling to, not a reuse of,
`knownValuationSegments`.

## Rollup computation

Per segment, the rollup query is:

```sql
SELECT p.sleeper_player_id,
       AVG(p.pick_no) AS avg_pick_no,
       COUNT(*)       AS pick_count,
       MIN(p.pick_no) AS min_pick_no,
       MAX(p.pick_no) AS max_pick_no
FROM sleeper_draft_picks p
JOIN sleeper_drafts d  ON d.sleeper_draft_id = p.sleeper_draft_id
JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id
WHERE d.status = 'complete'
  AND d.type IN ('snake', 'linear')
  AND l.league_type = 'redraft'
  AND <league_size bucket predicate>
  AND <scoring bucket predicate>
  AND <superflex predicate>
  AND p.sleeper_player_id != ''  -- skip empty/kept picks if any
GROUP BY p.sleeper_player_id
```

Results are upserted into `draft_adp` via `clause.OnConflict` on
`(segment, sleeper_player_id)`, `AssignmentColumns` on the four computed
fields + `updated_at` — same upsert pattern used elsewhere in
`internal/activities`. Rows for players who drop out of a segment's
qualifying draft pool are **not** deleted in this iteration (stale rows just
age out of relevance since pick_count won't grow) — acceptable for a v1,
noted as a nit rather than a blocker.

## Temporal worker

Follows the existing dispatcher → per-unit-child pattern used by
`DraftSyncDispatcher` / `LeagueDraftSyncWorkflow`:

- `backend/internal/activities/adp_rollup.go`
  - `ADPRollupActivities{DB *gorm.DB}` struct (matches the `*Activities{DB,
    Sleeper}` convention).
  - `ComputeSegmentADP(ctx, params ComputeSegmentADPParams) error` — runs
    the query above for one segment and upserts.
- `backend/internal/workflows/adp_rollup.go`
  - `ADPRollupDispatcher(ctx) error` — zero-arg, iterates the 24 segment
    tuples in-workflow (no activity needed to list them, they're a
    constant), fires one `SegmentADPRollupWorkflow` child per segment with
    `ParentClosePolicy: ABANDON` (matches `DraftSyncDispatcher`).
  - `SegmentADPRollupWorkflow(ctx, params SegmentADPParams) error` — thin
    wrapper: one `ComputeSegmentADP` activity call. Logs+continues past a
    single segment's failure rather than failing the whole dispatch (mirrors
    `LeagueDraftSyncWorkflow`'s per-pick warn+continue).
- `backend/internal/workflows/helpers.go`: add `TaskQueueADP =
  "sleeper-adp"`.
- `backend/cmd/worker/main.go`: register the new task queue's worker
  (workflow + activity), following the existing per-queue registration
  block.
- `backend/schedules/register.go`: add `sleeper-adp-rollup-schedule`, a
  daily `Calendars` schedule at a fixed off-peak hour (pattern-matched to
  the existing week-stats/player-sync daily schedules), targeting
  `ADPRollupDispatcher` on `TaskQueueADP`.

## API

New handler `GetSleeperADP` on `GET /api/v1/sleeper/adp`, in a new
`backend/internal/api/handlers/draft_adp.go` — `sleeper.go` is already 657
lines, so this keeps the new endpoint's segment-bucketing helpers isolated
rather than growing that file further.

Query params:
- `league_size` (one of `8`,`10`,`12`,`14+`; default `12`)
- `scoring_format` (one of `standard`,`half_ppr`,`ppr`; default `ppr`)
- `superflex` (`true`/`false`; default `true`)
- `min_drafts` (int; default `20`)
- `page`, `limit` (existing pagination convention)

Behavior: build the segment key from the three filters, query `draft_adp`
where `segment = ? AND pick_count >= ?`, join `sleeper_players` for name /
position / nfl_team, order by `avg_pick_no ASC`, paginate. Response shape
mirrors the existing paginated list responses (`SleeperADPResponse` with
`Players`, `Total`, `Page`, `Limit`, `TotalPages`).

Route registered in `backend/internal/api/routes.go` next to the other
`/sleeper/*` routes.

## Frontend

Repurposes `/sleeper/drafts` in place (same URL/nav entry — this *replaces*
the old draft-list page per the request, it doesn't add a new route):

- `frontend/src/types/models.ts`: new `SleeperADPFilters` (`league_size`,
  `scoring_format`, `superflex`) and `SleeperADPResponse`/`SleeperADPItem`
  types.
- `frontend/src/services/sleeperService.ts`: new `getADP(page, limit,
  filters)` following the existing `getDrafts` pattern.
- `frontend/src/hooks/useSleeperData.ts`: new `useSleeperADP` hook mirroring
  `useSleeperDrafts`.
- `frontend/src/pages/sleeper/drafts.tsx`: table columns become Rank,
  Player, Pos/Team, Avg Pick, Drafts (sample size). Default sort is the
  API's `avg_pick_no ASC` (best/earliest picks first) — no separate
  client-side sort control needed since the API already orders correctly
  per filter combination.
- New filter control (three pill groups: league size, scoring, superflex)
  — a variant of `LeagueFilterBar`'s pill styling, but its own component
  since `draft_type`/`league_type`/`exclude_picks` don't apply to ADP and
  `superflex` is boolean rather than one of `LeagueFilterBar`'s
  string-valued pill groups.

## Testing

- Activities: extend the existing in-memory sqlite activity test pattern
  (`internal/activities/discovery_test.go`'s `newTestDB(t)` helper) — add
  `DraftADP` to its `AutoMigrate` call, seed `sleeper_drafts` /
  `sleeper_leagues` / `sleeper_draft_picks` fixtures covering: qualifying
  redraft/snake picks, a non-qualifying auction draft, a non-qualifying
  dynasty league, and a player just under/over the 20-pick threshold (to
  confirm the threshold is *not* applied at write time — sub-threshold rows
  should still be upserted).
- Workflow: `go.temporal.io/sdk/testsuite`, verifying the dispatcher fires
  one child per of the 24 segments and that one child's activity failure
  doesn't abort the others.
- API handler: table-driven test hitting `GetSleeperADP` with the sqlite
  test DB, verifying segment defaulting, `min_drafts` filtering, and sort
  order.

## Out of scope (tracked in #131)

- Auction average-cost rollups.
- Dynasty/keeper ADP.
- Historical ADP trend/snapshots.
- "Other" league-size/scoring buckets.
