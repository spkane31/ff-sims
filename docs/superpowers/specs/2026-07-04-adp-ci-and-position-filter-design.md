# Spec: ADP Min/Max/95% CI Display + Client-Side Position Filter

**Date:** 2026-07-04
**Status:** Approved

## Context

`/sleeper/drafts` (file: `frontend/src/pages/sleeper/drafts.tsx`) is actually the "Average Draft
Position" report — it shows a paginated, ranked table of players by average Sleeper draft pick
number, per (league_size, scoring_format, superflex, season) segment. The underlying rollup
(`draft_adp` table, populated daily by `ADPRollupDispatcher` /
`ComputeSegmentSeasonADP` in `backend/internal/activities/adp_rollup.go`) already computes and
stores `min_pick_no`/`max_pick_no`, and both are already typed on the frontend
(`SleeperADPItem.min_pick_no`/`max_pick_no`) — but the table doesn't render them.

A single average can be misleading when one outlier draft picks a player unusually early or late
(a "fluke" pick). The goal of this change is twofold:

1. Show the existing min/max range, plus a proper 95% confidence interval (percentile-based, not
   min/max) so a single fluke draft doesn't visually dominate the "typical" range.
2. Add a position filter (QB/RB/WR/TE/K/DEF) to the table, filtered entirely client-side.

## Part 1 — Backend: 95% confidence interval columns

### Migration

New file `backend/migrations/017_draft_adp_ci.sql`:

```sql
-- +goose Up

ALTER TABLE draft_adp
    ADD COLUMN ci_low_pick_no  NUMERIC NOT NULL DEFAULT 0,
    ADD COLUMN ci_high_pick_no NUMERIC NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE draft_adp
    DROP COLUMN IF EXISTS ci_low_pick_no,
    DROP COLUMN IF EXISTS ci_high_pick_no;
```

`DEFAULT 0` lets the migration run without backfilling; the next scheduled `ADPRollupDispatcher`
run overwrites every row with real values within one rollup cycle (same as any other column
added to this upsert-driven table).

### Model

`backend/internal/models/draft_adp.go` — add two fields to `DraftADP`:

```go
type DraftADP struct {
	Segment         string    `gorm:"primaryKey;column:segment"`
	Season          string    `gorm:"primaryKey;column:season"`
	SleeperPlayerID string    `gorm:"primaryKey;column:sleeper_player_id"`
	AvgPickNo       float64   `gorm:"column:avg_pick_no"`
	PickCount       int       `gorm:"column:pick_count"`
	MinPickNo       int       `gorm:"column:min_pick_no"`
	MaxPickNo       int       `gorm:"column:max_pick_no"`
	CILowPickNo     float64   `gorm:"column:ci_low_pick_no"`
	CIHighPickNo    float64   `gorm:"column:ci_high_pick_no"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"`
}
```

### Rollup activity

`backend/internal/activities/adp_rollup.go`, `ComputeSegmentSeasonADP`:

- Extend `adpRow` with `CILowPickNo`, `CIHighPickNo float64` (columns `ci_low_pick_no`,
  `ci_high_pick_no`).
- Extend the `Select(...)` to add, using Postgres's native ordered-set aggregate:
  ```sql
  PERCENTILE_CONT(0.025) WITHIN GROUP (ORDER BY p.pick_no) AS ci_low_pick_no,
  PERCENTILE_CONT(0.975) WITHIN GROUP (ORDER BY p.pick_no) AS ci_high_pick_no
  ```
  This runs in the same query that already computes `AVG`/`MIN`/`MAX`, grouped by
  `p.sleeper_player_id`, so no new joins or data sources are needed.
- Populate the two new fields in the `records[i] = models.DraftADP{...}` construction.
- Add `"ci_low_pick_no", "ci_high_pick_no"` to the `DoUpdates` column list in the batched
  `clause.OnConflict` upsert.

### API handler

`backend/internal/api/handlers/draft_adp.go`:

- `adpItemRow`: add `CILowPickNo`, `CIHighPickNo float64` (columns `ci_low_pick_no`,
  `ci_high_pick_no`), included in the existing `Select(...)` in `GetSleeperADP`.
- `SleeperADPItem`: add `CILowPickNo float64 \`json:"ci_low_pick_no"\`` and
  `CIHighPickNo float64 \`json:"ci_high_pick_no"\``.
- Copy both fields through in the `items[i] = SleeperADPItem{...}` loop.

### Tests

Update the three existing test files to cover the new columns — no new test files:

- `backend/internal/models/draft_adp_test.go` — any struct/round-trip assertions covering
  `DraftADP` fields.
- `backend/internal/activities/adp_rollup_test.go` — assert `ci_low_pick_no`/`ci_high_pick_no`
  are populated correctly for a known set of seeded picks (e.g. picks `[3, 5, 8, 12, 40]` for one
  player should produce a low/high band close to the 2.5th/97.5th percentile of that sample).
- `backend/internal/api/handlers/draft_adp_test.go` — assert the new fields round-trip through
  `GET /sleeper/adp`.

## Part 2 — Frontend: display new columns

`frontend/src/types/models.ts`, `SleeperADPItem`:

```ts
export interface SleeperADPItem {
  sleeper_player_id: string;
  name: string;
  position: string;
  nfl_team: string;
  avg_pick_no: number;
  pick_count: number;
  min_pick_no: number;
  max_pick_no: number;
  ci_low_pick_no: number;
  ci_high_pick_no: number;
}
```

`frontend/src/pages/sleeper/drafts.tsx` table: add two columns after "Avg Pick":

- **Range** — `{min_pick_no}–{max_pick_no}` (integers, no decimals, matching existing min/max
  semantics).
- **95% CI** — `{ci_low_pick_no.toFixed(1)}–{ci_high_pick_no.toFixed(1)}` (percentiles are
  fractional).

Table header row and `colSpan` values (currently `6`, for loading/empty states) update to `8`.

## Part 3 — Frontend: client-side position filter

### Why "fetch all" is required

The table is server-paginated: `useSleeperADP(page, limit, filters)` requests one 25-row page at
a time, and the backend caps `limit` at 100 (`parsePagination`,
`backend/internal/api/handlers/sleeper.go:567`). A position filter applied only to whatever page
happens to be loaded would look broken — e.g. filtering to "QB" against a page dominated by
RBs/WRs could show 1–2 rows while the pager still reads "Page 1 of 20". To filter and paginate
correctly client-side, the full qualifying set for the current
(league_size, scoring_format, superflex, season) segment must be loaded into the browser first.

Segment result sets are bounded: only players with `pick_count >= min_drafts` (default 20)
qualify, which in practice is a few hundred rows per segment/season — safe to fetch in full.

### Data fetching change

`frontend/src/hooks/useSleeperData.ts` — add a new hook (leaving `useSleeperADP` untouched for
any other future paginated use):

```ts
export function useSleeperADPAll(filters: SleeperADPFilters = {}) {
  const [state, setState] = useState<{
    items: SleeperADPItem[];
    season: string;
    availableSeasons: string[];
    isLoading: boolean;
    error: Error | null;
  }>({ items: [], season: '', availableSeasons: [], isLoading: true, error: null });

  const filtersKey = JSON.stringify(filters);

  const fetch = useCallback(async () => {
    setState(s => ({ ...s, isLoading: true, error: null }));
    try {
      const first = await sleeperService.getADP(1, 100, filters);
      let items = first.players;
      for (let page = 2; page <= first.total_pages; page++) {
        const next = await sleeperService.getADP(page, 100, filters);
        items = items.concat(next.players);
      }
      setState({
        items,
        season: first.season,
        availableSeasons: first.available_seasons,
        isLoading: false,
        error: null,
      });
    } catch (err) {
      setState(s => ({ ...s, isLoading: false, error: err instanceof Error ? err : new Error('Failed to fetch ADP') }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filtersKey]);

  useEffect(() => { fetch(); }, [fetch]);
  return { ...state, refetch: fetch };
}
```

Sequential `for` loop (not `Promise.all`) to keep load on the backend predictable and preserve
page order without needing to re-sort; a few hundred rows is at most 2-4 requests.

### Page component changes

`frontend/src/pages/sleeper/drafts.tsx`:

- Replace `useSleeperADP(page, LIMIT, filters)` with `useSleeperADPAll(filters)`.
- Add local state `const [position, setPosition] = useState<string>('');` (empty = all positions),
  read from/written to the `position` query param the same way other filters are, for
  bookmarkable/shareable URLs — but this param is only ever consumed client-side, never sent to
  `sleeperService.getADP`.
- Derive the filtered + paginated slice in the component:
  ```ts
  const filtered = position ? all.items.filter(p => p.position === position) : all.items;
  const totalPages = Math.ceil(filtered.length / LIMIT) || 1;
  const pageItems = filtered.slice((page - 1) * LIMIT, page * LIMIT);
  ```
- Changing `position` resets `page` to 1 (same pattern as `applyFilters` for the existing
  segment/season filters).
- The "N players" header count reflects `filtered.length`, not the unfiltered total.

### Filter UI

`frontend/src/components/ADPFilterBar.tsx` — add a `Position` `PillGroup`:

```ts
const POSITIONS = [
  { value: '', label: 'All' },
  { value: 'QB', label: 'QB' },
  { value: 'RB', label: 'RB' },
  { value: 'WR', label: 'WR' },
  { value: 'TE', label: 'TE' },
  { value: 'K', label: 'K' },
  { value: 'DEF', label: 'DEF' },
];
```

Rendered as a new `PillGroup` alongside the existing Size/Scoring/Format/Season groups. Single-
select, consistent with the existing pills. Since `ADPFilterBar` currently only knows about
`SleeperADPFilters` (server-side filters), `position` is passed/handled as a sibling prop rather
than folded into that type, to keep clear which filters hit the backend and which are purely
client-side:

```ts
interface ADPFilterBarProps {
  filters: SleeperADPFilters;
  onChange: (filters: SleeperADPFilters) => void;
  availableSeasons: string[];
  position: string;
  onPositionChange: (position: string) => void;
}
```

## Non-goals

- No toggle between 95%/99% CI — 95% only, per decision above.
- No backend position filter query param — filtering is client-side only, per explicit request.
- No change to the daily rollup schedule/cadence — the new CI columns ride along in the existing
  `ADPRollupDispatcher` run.
