# ADP 95% CI + Position Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show min/max pick range and a 95% percentile-based confidence interval on the `/sleeper/drafts` (ADP) table, plus a client-side position filter, without touching the backend for the filter.

**Architecture:** The `draft_adp` rollup table gains two columns (`ci_low_pick_no`, `ci_high_pick_no`) populated by the existing daily `ComputeSegmentSeasonADP` activity. That activity is refactored from a single SQL aggregate query to fetching raw `(sleeper_player_id, pick_no)` rows and aggregating in Go — **this deviates from the design spec's "PERCENTILE_CONT in SQL" approach**: the activity's existing test suite (`adp_rollup_test.go`) runs against an in-memory SQLite database (`mattn/go-sqlite3` via `newTestDB`), and SQLite has no `PERCENTILE_CONT`/ordered-set aggregate support. Computing the percentile in Go with the same linear-interpolation formula Postgres's `PERCENTILE_CONT` uses produces numerically identical results while keeping the activity portable and testable under the existing harness. Avg/min/max/count move to the same Go computation for a single query round-trip; behavior for those three is unchanged. The API handler and frontend types/table pick up the two new fields, and the frontend adds a client-only position filter by fetching every page for the current segment/season (bounded to a few hundred rows by the existing `pick_count >= 20` qualifying threshold) and filtering/paginating in the browser.

**Tech Stack:** Go (Gin, GORM, Postgres in prod / SQLite in tests), Next.js/React/TypeScript, goose migrations.

## Global Constraints

- 95% CI only (2.5th/97.5th percentile) — no 99% or user-selectable CI level.
- Position filter is client-side only — no new backend query parameter.
- No change to the ADP rollup schedule/cadence — new columns ride along in the existing `ADPRollupDispatcher` run.
- Migration file: `backend/migrations/017_draft_adp_ci.sql` (next number after `016_draft_adp.sql`).
- Frontend has no test framework (`frontend/package.json` has no jest/vitest and no `*.test.*` files exist) — frontend tasks are verified via `npm run lint`, `npm run build`, and manual check in the browser, not automated tests.

---

### Task 1: Migration + `DraftADP` model fields for the CI columns

**Files:**
- Create: `backend/migrations/017_draft_adp_ci.sql`
- Modify: `backend/internal/models/draft_adp.go`
- Test: `backend/internal/models/draft_adp_test.go`

**Interfaces:**
- Produces: `models.DraftADP.CILowPickNo float64` (column `ci_low_pick_no`), `models.DraftADP.CIHighPickNo float64` (column `ci_high_pick_no`) — consumed by Task 2 (rollup activity) and Task 3 (API handler).

- [ ] **Step 1: Write the migration**

Create `backend/migrations/017_draft_adp_ci.sql`:

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

- [ ] **Step 2: Write the failing test**

Add to `backend/internal/models/draft_adp_test.go` (add `"gorm.io/driver/sqlite"` and `"gorm.io/gorm"` to the import block):

```go
func TestDraftADP_CIFieldsRoundTrip(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.DraftADP{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	row := models.DraftADP{
		Segment:         "12-ppr-sf",
		Season:          "2025",
		SleeperPlayerID: "p1",
		AvgPickNo:       10.0,
		PickCount:       25,
		MinPickNo:       1,
		MaxPickNo:       30,
		CILowPickNo:     2.5,
		CIHighPickNo:    22.5,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got models.DraftADP
	if err := db.First(&got, "sleeper_player_id = ?", "p1").Error; err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got.CILowPickNo != 2.5 || got.CIHighPickNo != 22.5 {
		t.Errorf("expected ci_low=2.5 ci_high=22.5, got ci_low=%v ci_high=%v", got.CILowPickNo, got.CIHighPickNo)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd backend && go test ./internal/models/... -run TestDraftADP_CIFieldsRoundTrip -v`
Expected: FAIL — `models.DraftADP` has no field `CILowPickNo`/`CIHighPickNo` (compile error).

- [ ] **Step 4: Add the fields to the model**

In `backend/internal/models/draft_adp.go`, update the `DraftADP` struct:

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

- [ ] **Step 5: Run test to verify it passes**

Run: `cd backend && go test ./internal/models/... -run TestDraftADP_CIFieldsRoundTrip -v`
Expected: PASS

- [ ] **Step 6: Run the full models package test suite**

Run: `cd backend && go test ./internal/models/... -v`
Expected: all tests PASS (including the pre-existing `TestADPSegmentKey` and `TestAllADPSegments_Has24UniqueKeys`).

- [ ] **Step 7: Commit**

```bash
git add backend/migrations/017_draft_adp_ci.sql backend/internal/models/draft_adp.go backend/internal/models/draft_adp_test.go
git commit -m "Add ci_low_pick_no/ci_high_pick_no columns to draft_adp"
```

---

### Task 2: Compute the 95% CI in the ADP rollup activity

**Files:**
- Modify: `backend/internal/activities/adp_rollup.go`
- Test: `backend/internal/activities/adp_rollup_test.go`

**Interfaces:**
- Consumes: `models.DraftADP.CILowPickNo/CIHighPickNo` (Task 1).
- Produces: `percentileCont(sorted []int, p float64) float64` (package-private helper, used only within `adp_rollup.go`); `ADPRollupActivities.ComputeSegmentSeasonADP` now also populates `CILowPickNo`/`CIHighPickNo` on every upserted row — consumed by Task 3 (handler reads `draft_adp.ci_low_pick_no`/`ci_high_pick_no` from the DB, not from this activity directly).

- [ ] **Step 1: Write the failing test**

Add to `backend/internal/activities/adp_rollup_test.go`:

```go
func TestComputeSegmentSeasonADP_ComputesPercentileCI(t *testing.T) {
	db := newTestDB(t)
	seedADPLeague(t, db, "lg1", 12, 1.0, true, "redraft")
	for i, pickNo := range []int{1, 2, 3, 4, 5} {
		draftID := fmt.Sprintf("d%d", i+1)
		seedADPDraft(t, db, draftID, "lg1", "snake", "complete", "2024")
		seedADPPick(t, db, draftID, 1, pickNo, "p1")
	}

	a := &activities.ADPRollupActivities{DB: db}
	if err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	}); err != nil {
		t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
	}

	var row models.DraftADP
	if err := db.Where("segment = ? AND season = ? AND sleeper_player_id = ?", "12-ppr-sf", "2024", "p1").First(&row).Error; err != nil {
		t.Fatalf("fetch p1 row: %v", err)
	}
	// Picks [1,2,3,4,5]: rank = p*(n-1). p=0.025 -> rank=0.1 -> 1 + 0.1*(2-1) = 1.1.
	// p=0.975 -> rank=3.9 -> 4 + 0.9*(5-4) = 4.9.
	const epsilon = 1e-9
	if math.Abs(row.CILowPickNo-1.1) > epsilon {
		t.Errorf("expected ci_low_pick_no ~= 1.1, got %v", row.CILowPickNo)
	}
	if math.Abs(row.CIHighPickNo-4.9) > epsilon {
		t.Errorf("expected ci_high_pick_no ~= 4.9, got %v", row.CIHighPickNo)
	}
	if row.AvgPickNo != 3 || row.PickCount != 5 || row.MinPickNo != 1 || row.MaxPickNo != 5 {
		t.Errorf("expected avg=3 count=5 min=1 max=5, got avg=%v count=%v min=%v max=%v", row.AvgPickNo, row.PickCount, row.MinPickNo, row.MaxPickNo)
	}
}
```

Add `"fmt"` and `"math"` to the import block of `adp_rollup_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/activities/... -run TestComputeSegmentSeasonADP_ComputesPercentileCI -v`
Expected: FAIL — `row.CILowPickNo`/`row.CIHighPickNo` are `0` (zero value), not `1.1`/`4.9`, since the rollup doesn't populate them yet.

- [ ] **Step 3: Rewrite `ComputeSegmentSeasonADP` to compute stats in Go**

Replace the full contents of `backend/internal/activities/adp_rollup.go` with:

```go
package activities

import (
	"context"
	"math"
	"sort"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/models"
)

// qualifyingDraftTypes are the Sleeper draft types comparable to a snake
// pick order. Auction pick_no reflects nomination order, not draft slot
// value, so auction drafts are excluded from ADP.
var qualifyingDraftTypes = []string{"snake", "linear"}

// ADPRollupActivities holds dependencies for the daily ADP rollup worker.
type ADPRollupActivities struct {
	DB *gorm.DB
}

// ListADPSeasons returns the distinct seasons with at least one qualifying
// (complete, snake/linear, redraft) draft, so the dispatcher doesn't need a
// hardcoded season list.
func (a *ADPRollupActivities) ListADPSeasons(ctx context.Context) ([]string, error) {
	var seasons []string
	err := a.DB.WithContext(ctx).
		Table("sleeper_drafts d").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id").
		Where("d.status = ? AND d.type IN ? AND l.league_type = ?", "complete", qualifyingDraftTypes, "redraft").
		Distinct("d.season").
		Pluck("d.season", &seasons).Error
	return seasons, err
}

type pickRow struct {
	SleeperPlayerID string `gorm:"column:sleeper_player_id"`
	PickNo          int    `gorm:"column:pick_no"`
}

// percentileCont returns the p-th percentile (0 <= p <= 1) of sorted using
// linear interpolation between closest ranks — the same algorithm Postgres's
// PERCENTILE_CONT implements. sorted must already be sorted ascending and
// non-empty.
func percentileCont(sorted []int, p float64) float64 {
	n := len(sorted)
	if n == 1 {
		return float64(sorted[0])
	}
	rank := p * float64(n-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return float64(sorted[lo])
	}
	frac := rank - float64(lo)
	return float64(sorted[lo]) + frac*(float64(sorted[hi])-float64(sorted[lo]))
}

// ComputeSegmentSeasonADP computes ADP for every player picked in qualifying
// drafts matching params.Segment and params.Season, then upserts one
// draft_adp row per player. The 20-draft minimum sample size is enforced at
// API read time, not here — every player who appears at least once is
// upserted.
//
// Stats (avg/min/max/count/95% CI) are aggregated in Go rather than SQL: the
// 95% CI needs an ordered-set aggregate (Postgres's PERCENTILE_CONT), which
// has no SQLite equivalent, and this activity's test suite runs against an
// in-memory SQLite DB. Computing in Go with the same linear-interpolation
// formula PERCENTILE_CONT uses keeps results identical while staying
// portable and testable.
func (a *ADPRollupActivities) ComputeSegmentSeasonADP(ctx context.Context, params ComputeSegmentSeasonADPParams) error {
	db := a.DB.WithContext(ctx).
		Table("sleeper_draft_picks p").
		Select("p.sleeper_player_id, p.pick_no").
		Joins("JOIN sleeper_drafts d ON d.sleeper_draft_id = p.sleeper_draft_id").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id").
		Where("d.status = ? AND d.type IN ? AND l.league_type = ? AND d.season = ?",
			"complete", qualifyingDraftTypes, "redraft", params.Season).
		Where("p.sleeper_player_id != ''")
	db = applySegmentPredicate(db, params.Segment)

	var picks []pickRow
	if err := db.Scan(&picks).Error; err != nil {
		return err
	}
	if len(picks) == 0 {
		return nil
	}

	byPlayer := make(map[string][]int, len(picks))
	for _, p := range picks {
		byPlayer[p.SleeperPlayerID] = append(byPlayer[p.SleeperPlayerID], p.PickNo)
	}

	segmentKey := params.Segment.Key()
	records := make([]models.DraftADP, 0, len(byPlayer))
	for playerID, pickNos := range byPlayer {
		sort.Ints(pickNos)
		sum := 0
		for _, v := range pickNos {
			sum += v
		}
		records = append(records, models.DraftADP{
			Segment:         segmentKey,
			Season:          params.Season,
			SleeperPlayerID: playerID,
			AvgPickNo:       float64(sum) / float64(len(pickNos)),
			PickCount:       len(pickNos),
			MinPickNo:       pickNos[0],
			MaxPickNo:       pickNos[len(pickNos)-1],
			CILowPickNo:     percentileCont(pickNos, 0.025),
			CIHighPickNo:    percentileCont(pickNos, 0.975),
		})
	}

	// One batched upsert instead of one round-trip per player: with a large
	// qualifying draft pool (hundreds of distinct players), a per-row loop
	// could exceed the activity's StartToCloseTimeout partway through,
	// leaving only whichever players were reached upserted for that
	// segment/season, with no rollback. A single batched statement is both
	// atomic and one round trip instead of hundreds.
	return a.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "segment"}, {Name: "season"}, {Name: "sleeper_player_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"avg_pick_no", "pick_count", "min_pick_no", "max_pick_no", "ci_low_pick_no", "ci_high_pick_no", "updated_at",
		}),
	}).CreateInBatches(&records, 500).Error
}

// applySegmentPredicate appends WHERE conditions for one ADP segment's
// league_size/scoring_format/superflex bucket onto a query already joined to
// sleeper_leagues as "l".
func applySegmentPredicate(db *gorm.DB, seg models.ADPSegment) *gorm.DB {
	if seg.LeagueSize == "14+" {
		db = db.Where("l.total_rosters >= ?", 14)
	} else if n, err := strconv.Atoi(seg.LeagueSize); err == nil {
		db = db.Where("l.total_rosters = ?", n)
	}
	switch seg.ScoringFormat {
	case "standard":
		db = db.Where("l.ppr = ?", 0)
	case "half_ppr":
		db = db.Where("l.ppr = ?", 0.5)
	case "ppr":
		db = db.Where("l.ppr = ?", 1)
	}
	return db.Where("l.is_superflex = ?", seg.Superflex)
}
```

- [ ] **Step 4: Run the new test to verify it passes**

Run: `cd backend && go test ./internal/activities/... -run TestComputeSegmentSeasonADP_ComputesPercentileCI -v`
Expected: PASS

- [ ] **Step 5: Run the full activities package test suite**

Run: `cd backend && go test ./internal/activities/... -v`
Expected: all tests PASS, including the pre-existing `TestComputeSegmentSeasonADP_ComputesAverages`,
`TestComputeSegmentSeasonADP_ExcludesAuctionAndNonRedraft`,
`TestComputeSegmentSeasonADP_NoMinDraftsThresholdAtWriteTime`,
`TestComputeSegmentSeasonADP_UpsertOverwritesPreviousRun`, and `TestListADPSeasons_ReturnsOnlyQualifyingSeasons` — none of their assertions reference the new CI fields, so they must keep passing unmodified.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/activities/adp_rollup.go backend/internal/activities/adp_rollup_test.go
git commit -m "Compute 95% percentile CI in ADP rollup activity"
```

---

### Task 3: Surface the CI fields through the `/sleeper/adp` API

**Files:**
- Modify: `backend/internal/api/handlers/draft_adp.go`
- Test: `backend/internal/api/handlers/draft_adp_test.go`

**Interfaces:**
- Consumes: `draft_adp.ci_low_pick_no`/`ci_high_pick_no` columns (Task 1/2).
- Produces: `SleeperADPItem.CILowPickNo/CIHighPickNo float64` (JSON `ci_low_pick_no`/`ci_high_pick_no`) — consumed by the frontend (Task 4).

- [ ] **Step 1: Write the failing test**

In `backend/internal/api/handlers/draft_adp_test.go`, update `seedADPRow` to accept and set CI values, and add a new test. Replace the existing `seedADPRow` function with:

```go
func seedADPRow(t *testing.T, db *gorm.DB, segment, season, playerID string, avgPick float64, pickCount int) {
	t.Helper()
	if err := db.Create(&models.DraftADP{
		Segment:         segment,
		Season:          season,
		SleeperPlayerID: playerID,
		AvgPickNo:       avgPick,
		PickCount:       pickCount,
		MinPickNo:       int(avgPick),
		MaxPickNo:       int(avgPick),
		CILowPickNo:     avgPick,
		CIHighPickNo:    avgPick,
	}).Error; err != nil {
		t.Fatalf("seed adp row %s/%s/%s: %v", segment, season, playerID, err)
	}
}
```

Then add:

```go
func TestGetSleeperADP_IncludesCIFields(t *testing.T) {
	db := newDraftADPTestDB(t)
	withDraftADPTestDB(t, db)

	seedADPPlayer(t, db, "p1", "Player One", "RB", "KC")
	if err := db.Create(&models.DraftADP{
		Segment:         "12-ppr-sf",
		Season:          "2025",
		SleeperPlayerID: "p1",
		AvgPickNo:       10.0,
		PickCount:       25,
		MinPickNo:       2,
		MaxPickNo:       28,
		CILowPickNo:     4.5,
		CIHighPickNo:    20.5,
	}).Error; err != nil {
		t.Fatalf("seed row: %v", err)
	}

	_, resp := performGetSleeperADP(t, "?season=2025")
	if len(resp.Players) != 1 {
		t.Fatalf("expected 1 player, got %d", len(resp.Players))
	}
	got := resp.Players[0]
	if got.MinPickNo != 2 || got.MaxPickNo != 28 {
		t.Errorf("expected min=2 max=28, got min=%v max=%v", got.MinPickNo, got.MaxPickNo)
	}
	if got.CILowPickNo != 4.5 || got.CIHighPickNo != 20.5 {
		t.Errorf("expected ci_low=4.5 ci_high=20.5, got ci_low=%v ci_high=%v", got.CILowPickNo, got.CIHighPickNo)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/api/handlers/... -run TestGetSleeperADP_IncludesCIFields -v`
Expected: FAIL — `SleeperADPItem` has no `CILowPickNo`/`CIHighPickNo` field (compile error).

- [ ] **Step 3: Add the fields to the handler**

In `backend/internal/api/handlers/draft_adp.go`:

Update `SleeperADPItem`:

```go
type SleeperADPItem struct {
	SleeperPlayerID string  `json:"sleeper_player_id"`
	Name            string  `json:"name"`
	Position        string  `json:"position"`
	NflTeam         string  `json:"nfl_team"`
	AvgPickNo       float64 `json:"avg_pick_no"`
	PickCount       int     `json:"pick_count"`
	MinPickNo       int     `json:"min_pick_no"`
	MaxPickNo       int     `json:"max_pick_no"`
	CILowPickNo     float64 `json:"ci_low_pick_no"`
	CIHighPickNo    float64 `json:"ci_high_pick_no"`
}
```

Update `adpItemRow`:

```go
type adpItemRow struct {
	SleeperPlayerID string  `gorm:"column:sleeper_player_id"`
	Name            string  `gorm:"column:full_name"`
	Position        string  `gorm:"column:position"`
	NflTeam         string  `gorm:"column:nfl_team"`
	AvgPickNo       float64 `gorm:"column:avg_pick_no"`
	PickCount       int     `gorm:"column:pick_count"`
	MinPickNo       int     `gorm:"column:min_pick_no"`
	MaxPickNo       int     `gorm:"column:max_pick_no"`
	CILowPickNo     float64 `gorm:"column:ci_low_pick_no"`
	CIHighPickNo    float64 `gorm:"column:ci_high_pick_no"`
}
```

Update the `Select(...)` call in `GetSleeperADP`:

```go
	var rows []adpItemRow
	database.DB.Table("draft_adp a").
		Select("a.sleeper_player_id, p.full_name, p.position, p.nfl_team, a.avg_pick_no, a.pick_count, a.min_pick_no, a.max_pick_no, a.ci_low_pick_no, a.ci_high_pick_no").
		Joins("JOIN sleeper_players p ON p.sleeper_player_id = a.sleeper_player_id").
		Where("a.segment = ? AND a.season = ? AND a.pick_count >= ?", segment, season, minDrafts).
		Order("a.avg_pick_no ASC").
		Limit(limit).Offset(offset).
		Scan(&rows)
```

Update the `items[i] = SleeperADPItem{...}` construction:

```go
	items := make([]SleeperADPItem, len(rows))
	for i, r := range rows {
		items[i] = SleeperADPItem{
			SleeperPlayerID: r.SleeperPlayerID,
			Name:            r.Name,
			Position:        r.Position,
			NflTeam:         r.NflTeam,
			AvgPickNo:       r.AvgPickNo,
			PickCount:       r.PickCount,
			MinPickNo:       r.MinPickNo,
			MaxPickNo:       r.MaxPickNo,
			CILowPickNo:     r.CILowPickNo,
			CIHighPickNo:    r.CIHighPickNo,
		}
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/api/handlers/... -run TestGetSleeperADP_IncludesCIFields -v`
Expected: PASS

- [ ] **Step 5: Run the full handlers package test suite**

Run: `cd backend && go test ./internal/api/handlers/... -v`
Expected: all tests PASS, including the pre-existing `TestGetSleeperADP_DefaultsAndOrdering`, `TestGetSleeperADP_MinDraftsFiltersLowSampleSize`, `TestGetSleeperADP_ExplicitFiltersBuildSegmentKey`, `TestGetSleeperADP_SeasonListIsHardcoded`, `TestGetSleeperADP_EmptyTable`.

- [ ] **Step 6: Run the full backend test suite**

Run: `cd backend && go build ./... && go test ./...`
Expected: build succeeds, all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/api/handlers/draft_adp.go backend/internal/api/handlers/draft_adp_test.go
git commit -m "Return ci_low_pick_no/ci_high_pick_no from GET /sleeper/adp"
```

---

### Task 4: Frontend types + Range/95% CI table columns

**Files:**
- Modify: `frontend/src/types/models.ts`
- Modify: `frontend/src/pages/sleeper/drafts.tsx`

**Interfaces:**
- Consumes: `ci_low_pick_no`/`ci_high_pick_no` JSON fields from `GET /sleeper/adp` (Task 3).
- Produces: `SleeperADPItem.ci_low_pick_no`/`ci_high_pick_no: number` — consumed by Task 5 (position filter reads `SleeperADPItem.position` on the same type, already present).

- [ ] **Step 1: Add the fields to the TypeScript type**

In `frontend/src/types/models.ts`, update `SleeperADPItem`:

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

- [ ] **Step 2: Add the two columns to the table**

In `frontend/src/pages/sleeper/drafts.tsx`, update the `<thead>` row (adding two `<th>` after "Avg Pick"):

```tsx
              <tr>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Rank</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Player</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Pos</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Team</th>
                <th className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300">Avg Pick</th>
                <th className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300">Range</th>
                <th className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300">95% CI</th>
                <th className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300">Drafts</th>
              </tr>
```

Update both `colSpan={6}` occurrences (loading row and empty-state row) to `colSpan={8}`.

Update the row-rendering `<tr>` body, adding two `<td>` after the Avg Pick `<td>`:

```tsx
                    <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                      {player.avg_pick_no.toFixed(1)}
                    </td>
                    <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                      {player.min_pick_no}–{player.max_pick_no}
                    </td>
                    <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                      {player.ci_low_pick_no.toFixed(1)}–{player.ci_high_pick_no.toFixed(1)}
                    </td>
                    <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                      {player.pick_count}
                    </td>
```

- [ ] **Step 3: Verify the frontend builds and lints clean**

Run: `cd frontend && npm run lint && npm run build`
Expected: both succeed with no new errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/types/models.ts frontend/src/pages/sleeper/drafts.tsx
git commit -m "Display pick range and 95% CI columns on the ADP table"
```

---

### Task 5: Client-side position filter with correct pagination

**Files:**
- Modify: `frontend/src/hooks/useSleeperData.ts`
- Modify: `frontend/src/components/ADPFilterBar.tsx`
- Modify: `frontend/src/pages/sleeper/drafts.tsx`

**Interfaces:**
- Consumes: `sleeperService.getADP(page, limit, filters)` (existing, `frontend/src/services/sleeperService.ts`), `SleeperADPItem` (Task 4).
- Produces: `useSleeperADPAll(filters: SleeperADPFilters)` hook (new, alongside the untouched `useSleeperADP`) returning `{ items, season, availableSeasons, isLoading, error, refetch }` with every qualifying row for the segment/season already loaded.

- [ ] **Step 1: Add the `useSleeperADPAll` hook**

In `frontend/src/hooks/useSleeperData.ts`, add after the existing `useSleeperADP` function:

```ts
interface ADPAllState {
  items: SleeperADPItem[];
  season: string;
  availableSeasons: string[];
  isLoading: boolean;
  error: Error | null;
}

export function useSleeperADPAll(filters: SleeperADPFilters = {}) {
  const [state, setState] = useState<ADPAllState>({
    items: [], season: '', availableSeasons: [], isLoading: true, error: null,
  });

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

- [ ] **Step 2: Verify the hook compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: no type errors.

- [ ] **Step 3: Commit the hook**

```bash
git add frontend/src/hooks/useSleeperData.ts
git commit -m "Add useSleeperADPAll hook for full-segment client-side pagination"
```

- [ ] **Step 4: Add the Position pill group to the filter bar**

In `frontend/src/components/ADPFilterBar.tsx`, add the position options constant near the other option arrays:

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

Update the component's props and body to accept and render the position filter as a sibling to `filters` (it's client-only, not part of `SleeperADPFilters`):

```tsx
interface ADPFilterBarProps {
  filters: SleeperADPFilters;
  onChange: (filters: SleeperADPFilters) => void;
  availableSeasons: string[];
  position: string;
  onPositionChange: (position: string) => void;
}

export default function ADPFilterBar({ filters, onChange, availableSeasons, position, onPositionChange }: ADPFilterBarProps) {
  function set(key: keyof SleeperADPFilters, value: string) {
    onChange({ ...filters, [key]: value });
  }

  const seasonOptions = availableSeasons.map(s => ({ value: s, label: s }));
  const currentSeason = filters.season ?? seasonOptions[0]?.value ?? '';

  return (
    <div className="flex flex-col gap-2.5 bg-gray-50 dark:bg-gray-800/50 border border-gray-200 dark:border-gray-700 rounded-lg px-4 py-3">
      <div className="flex flex-wrap gap-x-6 gap-y-2">
        <PillGroup
          label="Size"
          options={LEAGUE_SIZES}
          value={filters.league_size ?? '12'}
          onChange={v => set('league_size', v)}
        />

        <PillGroup
          label="Scoring"
          options={SCORING_FORMATS}
          value={filters.scoring_format ?? 'ppr'}
          onChange={v => set('scoring_format', v)}
        />

        <PillGroup
          label="Format"
          options={SUPERFLEX_OPTIONS}
          value={filters.superflex ?? 'true'}
          onChange={v => set('superflex', v)}
        />

        {seasonOptions.length > 0 && (
          <PillGroup
            label="Season"
            options={seasonOptions}
            value={currentSeason}
            onChange={v => set('season', v)}
          />
        )}

        <PillGroup
          label="Position"
          options={POSITIONS}
          value={position}
          onChange={onPositionChange}
        />
      </div>
    </div>
  );
}
```

- [ ] **Step 5: Wire the position filter and fetch-all pagination into the page**

Replace the full contents of `frontend/src/pages/sleeper/drafts.tsx` with:

```tsx
import { useEffect, useState } from "react";
import { useRouter } from "next/router";
import Layout from "../../components/Layout";
import ADPFilterBar from "../../components/ADPFilterBar";
import { useSleeperADPAll } from "../../hooks/useSleeperData";
import { SleeperADPFilters } from "../../types/models";

const LIMIT = 25;

function filtersFromQuery(query: Record<string, string | string[] | undefined>): SleeperADPFilters {
  return {
    league_size: typeof query.league_size === "string" ? query.league_size : undefined,
    scoring_format: typeof query.scoring_format === "string" ? query.scoring_format : undefined,
    superflex: typeof query.superflex === "string" ? query.superflex : undefined,
    season: typeof query.season === "string" ? query.season : undefined,
  };
}

export default function SleeperADPPage() {
  const router = useRouter();
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState<SleeperADPFilters>({});
  const [position, setPosition] = useState("");
  const [ready, setReady] = useState(false);

  useEffect(() => {
    if (!router.isReady) return;
    setFilters(filtersFromQuery(router.query));
    setPosition(typeof router.query.position === "string" ? router.query.position : "");
    const p = parseInt(router.query.page as string);
    if (p > 0) setPage(p);
    setReady(true);
  }, [router.isReady, router.query]);

  const { items: allItems, season, availableSeasons, isLoading, error } = useSleeperADPAll(
    ready ? filters : {}
  );

  const filtered = position ? allItems.filter(p => p.position === position) : allItems;
  const total = filtered.length;
  const totalPages = Math.max(1, Math.ceil(total / LIMIT));
  const items = filtered.slice((page - 1) * LIMIT, page * LIMIT);

  function applyFilters(next: SleeperADPFilters) {
    setFilters(next);
    setPage(1);
    const q: Record<string, string> = { page: "1" };
    if (next.league_size) q.league_size = next.league_size;
    if (next.scoring_format) q.scoring_format = next.scoring_format;
    if (next.superflex) q.superflex = next.superflex;
    if (next.season) q.season = next.season;
    if (position) q.position = position;
    router.push({ pathname: router.pathname, query: q }, undefined, { shallow: true });
  }

  function applyPosition(next: string) {
    setPosition(next);
    setPage(1);
    const q: Record<string, string> = { ...(router.query as Record<string, string>), page: "1" };
    if (next) {
      q.position = next;
    } else {
      delete q.position;
    }
    router.push({ pathname: router.pathname, query: q }, undefined, { shallow: true });
  }

  function goToPage(p: number) {
    setPage(p);
    const q: Record<string, string> = { ...router.query as Record<string, string>, page: String(p) };
    router.push({ pathname: router.pathname, query: q }, undefined, { shallow: true });
  }

  return (
    <Layout>
      <div className="space-y-6">
        <div>
          <h1 className="text-3xl font-bold text-blue-600">Average Draft Position</h1>
          <p className="text-gray-600 dark:text-gray-300 mt-1">
            {isLoading ? "Loading…" : `${total.toLocaleString()} players${season ? ` — ${season} season` : ""}`}
          </p>
        </div>

        <ADPFilterBar
          filters={filters}
          onChange={applyFilters}
          availableSeasons={availableSeasons}
          position={position}
          onPositionChange={applyPosition}
        />

        {error && (
          <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4 text-red-700 dark:text-red-300">
            Failed to load ADP: {error.message}
          </div>
        )}

        <div className="overflow-x-auto bg-white dark:bg-gray-800 rounded-lg shadow">
          <table className="w-full">
            <thead className="bg-gray-50 dark:bg-gray-700">
              <tr>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Rank</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Player</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Pos</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Team</th>
                <th className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300">Avg Pick</th>
                <th className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300">Range</th>
                <th className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300">95% CI</th>
                <th className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300">Drafts</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
              {isLoading ? (
                <tr>
                  <td colSpan={8} className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                    <div className="flex justify-center items-center space-x-2">
                      <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                      <span>Loading ADP…</span>
                    </div>
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td colSpan={8} className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                    No players found for this filter combination.
                  </td>
                </tr>
              ) : (
                items.map((player, i) => (
                  <tr key={player.sleeper_player_id} className="hover:bg-gray-50 dark:hover:bg-gray-700">
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">
                      {(page - 1) * LIMIT + i + 1}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-900 dark:text-gray-100 max-w-xs truncate">
                      {player.name}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{player.position}</td>
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{player.nfl_team}</td>
                    <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                      {player.avg_pick_no.toFixed(1)}
                    </td>
                    <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                      {player.min_pick_no}–{player.max_pick_no}
                    </td>
                    <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                      {player.ci_low_pick_no.toFixed(1)}–{player.ci_high_pick_no.toFixed(1)}
                    </td>
                    <td className="px-4 py-3 text-sm text-center text-gray-600 dark:text-gray-300">
                      {player.pick_count}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        {totalPages > 1 && (
          <div className="flex items-center justify-between">
            <button
              className="px-4 py-2 text-sm bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-md disabled:opacity-40 hover:bg-gray-50 dark:hover:bg-gray-600 transition-colors"
              onClick={() => goToPage(page - 1)}
              disabled={page <= 1 || isLoading}
            >
              Previous
            </button>
            <span className="text-sm text-gray-600 dark:text-gray-300">
              Page {page} of {totalPages}
            </span>
            <button
              className="px-4 py-2 text-sm bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-md disabled:opacity-40 hover:bg-gray-50 dark:hover:bg-gray-600 transition-colors"
              onClick={() => goToPage(page + 1)}
              disabled={page >= totalPages || isLoading}
            >
              Next
            </button>
          </div>
        )}
      </div>
    </Layout>
  );
}
```

- [ ] **Step 6: Verify the frontend builds and lints clean**

Run: `cd frontend && npm run lint && npm run build`
Expected: both succeed with no new errors.

- [ ] **Step 7: Manual verification in the browser**

Run: `cd frontend && npm run dev` (backend must also be running — see `backend/README.md` or `make run` from `/backend`).

In the browser at `/sleeper/drafts`:
- Confirm the Range and 95% CI columns render plausible values (CI band narrower than or equal to min–max range).
- Click each Position pill (QB/RB/WR/TE/K/DEF/All) and confirm the table filters to only that position, the "N players" count matches, and pagination (`Page X of Y`) reflects the filtered count, not the unfiltered total.
- Change a segment filter (e.g. league size) and confirm the position filter and page reset to 1 and "All".

- [ ] **Step 8: Commit**

```bash
git add frontend/src/components/ADPFilterBar.tsx frontend/src/pages/sleeper/drafts.tsx
git commit -m "Add client-side position filter to the ADP table"
```

---

### Task 6: Update the design spec to match the implemented approach, then open the PR

**Files:**
- Modify: `docs/superpowers/specs/2026-07-04-adp-ci-and-position-filter-design.md`

- [ ] **Step 1: Patch the spec's rollup section to reflect the Go-side percentile computation**

In `docs/superpowers/specs/2026-07-04-adp-ci-and-position-filter-design.md`, under "### Rollup activity", add a note directly under the heading (before the bullet list):

```markdown
**Implementation note (added during planning):** the percentile computation was moved from
SQL (`PERCENTILE_CONT`) to Go, because `adp_rollup_test.go` runs against an in-memory SQLite
database which has no ordered-set aggregate support. The activity now fetches raw
`(sleeper_player_id, pick_no)` rows and aggregates avg/min/max/count/percentiles in Go using
the same linear-interpolation formula `PERCENTILE_CONT` implements, producing identical
results.
```

- [ ] **Step 2: Commit the spec update**

```bash
git add docs/superpowers/specs/2026-07-04-adp-ci-and-position-filter-design.md
git commit -m "Note Go-side percentile computation in the ADP CI design spec"
```

- [ ] **Step 3: Run the full verification suite one more time**

Run: `cd backend && go build ./... && go test ./...`
Run: `cd frontend && npm run lint && npm run build`
Expected: everything passes — this is the final gate before opening the PR.

- [ ] **Step 4: Push the branch and open the PR**

```bash
git push -u origin HEAD
gh pr create --title "Show ADP pick range, 95% CI, and add position filter" --body "$(cat <<'EOF'
## Summary
- Adds `ci_low_pick_no`/`ci_high_pick_no` (95% percentile confidence interval) to the `draft_adp` rollup, computed alongside the existing avg/min/max/count in `ComputeSegmentSeasonADP`.
- Displays the existing min/max range and the new 95% CI as two new columns on `/sleeper/drafts`.
- Adds a client-side position filter (QB/RB/WR/TE/K/DEF) — fetches every qualifying row for the current segment/season and filters/paginates entirely in the browser, since the position filter never touches the backend.

## Test plan
- [x] `go test ./...` passes in `backend/`
- [x] `npm run lint` and `npm run build` pass in `frontend/`
- [x] Manually verified in the browser: Range/95% CI columns render, each position pill filters correctly with accurate pagination counts, changing a segment filter resets position/page

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Report the returned PR URL back to the user.

## Self-Review Notes

- **Spec coverage:** Part 1 (backend CI columns) → Tasks 1-3. Part 2 (frontend display) → Task 4. Part 3 (client-side position filter) → Task 5. Non-goals (no 99%/toggle, no backend filter param, no schedule change) are respected throughout — confirmed no task adds a `position` query param to the Go handler or a CI-level toggle.
- **Deviation from spec:** the design spec describes computing the CI via Postgres `PERCENTILE_CONT` directly in SQL. Task 2 implements this in Go instead, for SQLite test-suite compatibility (discovered during planning). Task 6 patches the spec doc to document this so the two stay consistent. The observable behavior (95% percentile CI, same rollup cadence, same columns) is unchanged.
- **Type consistency:** `CILowPickNo`/`CIHighPickNo` (Go) and `ci_low_pick_no`/`ci_high_pick_no` (JSON/SQL) are used identically across `models.DraftADP` (Task 1), `activities.ADPRollupActivities` (Task 2), `handlers.SleeperADPItem`/`adpItemRow` (Task 3), and `SleeperADPItem` (Task 4) — verified no naming drift between tasks.
