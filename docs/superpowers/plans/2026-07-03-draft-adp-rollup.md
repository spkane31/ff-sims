# Draft ADP Rollup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `/sleeper/drafts` (a raw list of completed Sleeper drafts) with an Average Draft Position (ADP) report — players ranked by average pick number, filterable by league size, scoring format, superflex, and season — computed by a new daily Temporal rollup worker from existing synced Sleeper draft data.

**Architecture:** A new Postgres table (`draft_adp`) is upserted daily by a Temporal dispatcher → per-(segment, season)-child-workflow pair, following the existing `DraftSyncDispatcher`/`LeagueDraftSyncWorkflow` pattern in `backend/internal/workflows`. A new read-only API endpoint (`GET /api/v1/sleeper/adp`) serves the ranked, filtered, paginated list. The frontend's existing `/sleeper/drafts` page is rewritten in place to call the new endpoint with a new filter bar.

**Tech Stack:** Go (Gin, GORM, Temporal Go SDK), Next.js/TypeScript/React, PostgreSQL, goose migrations.

## Global Constraints

- Only `snake`/`linear` drafts (`sleeper_drafts.type`) from `redraft` leagues (`sleeper_leagues.league_type`) count toward ADP. Auction and dynasty/keeper drafts are excluded (tracked as follow-ups in [issue #131](https://github.com/spkane31/ff-sims/issues/131)).
- Segments are the full cross product of league_size `{8, 10, 12, 14+}` × scoring `{standard, half_ppr, ppr}` × superflex `{true, false}` = 24 segments. Segment key format: `{league_size}-{scoring}-{sf|1qb}` (e.g. `12-ppr-sf`, `10-half_ppr-1qb`).
- ADP is computed **per season** — `(segment, season, sleeper_player_id)` is the storage key. Seasons are discovered at run time (no hardcoded list).
- Minimum sample size (20 qualifying drafts) is enforced at **API read time**, not at rollup write time.
- Current-value only: each daily run upserts and overwrites; no historical snapshots.
- Default filters when unset: `league_size=12`, `scoring_format=ppr`, `superflex=true`, `season=`(most recent season with data for the resolved segment).
- Full design: `docs/superpowers/specs/2026-07-03-draft-adp-rollup-design.md`.

---

### Task 1: `draft_adp` migration

**Files:**
- Create: `backend/migrations/016_draft_adp.sql`

**Interfaces:**
- Produces: `draft_adp` table — columns `segment TEXT`, `season TEXT`, `sleeper_player_id TEXT REFERENCES sleeper_players`, `avg_pick_no NUMERIC`, `pick_count INTEGER`, `min_pick_no INTEGER`, `max_pick_no INTEGER`, `updated_at TIMESTAMPTZ`, `PRIMARY KEY (segment, season, sleeper_player_id)`. Consumed by Task 2's GORM model and Task 3's rollup activity.

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up

CREATE TABLE draft_adp (
    segment           TEXT NOT NULL,
    season            TEXT NOT NULL,
    sleeper_player_id TEXT NOT NULL REFERENCES sleeper_players(sleeper_player_id),
    avg_pick_no       NUMERIC NOT NULL,
    pick_count        INTEGER NOT NULL,
    min_pick_no       INTEGER NOT NULL,
    max_pick_no       INTEGER NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (segment, season, sleeper_player_id)
);

CREATE INDEX idx_draft_adp_segment_season_avg_pick
    ON draft_adp (segment, season, avg_pick_no);

-- +goose Down

DROP TABLE IF EXISTS draft_adp;
```

Save to `backend/migrations/016_draft_adp.sql`.

- [ ] **Step 2: Verify the migrate binary still builds with the new file embedded**

Run: `cd backend && go build ./cmd/migrate/...`
Expected: no output, exit 0 (the migration isn't applied to any live database by this step — that requires `DATABASE_URL`/`.env` configured against a real Postgres instance; run `go run ./cmd/migrate up` yourself once you're ready to apply it to a dev database).

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/016_draft_adp.sql
git commit -m "feat(sleeper): add draft_adp rollup table migration"
```

---

### Task 2: GORM model and segment helpers

**Files:**
- Create: `backend/internal/models/draft_adp.go`
- Test: `backend/internal/models/draft_adp_test.go`

**Interfaces:**
- Consumes: nothing (leaf package).
- Produces:
  - `models.DraftADP{Segment, Season, SleeperPlayerID string; AvgPickNo float64; PickCount, MinPickNo, MaxPickNo int; UpdatedAt time.Time}` — GORM model, `TableName() string` returns `"draft_adp"`.
  - `models.ADPSegment{LeagueSize, ScoringFormat string; Superflex bool}` with method `(s ADPSegment) Key() string`.
  - `models.ADPSegmentKey(leagueSize, scoringFormat string, superflex bool) string`.
  - `models.AllADPSegments() []ADPSegment` — 24 entries.
  - `models.ADPLeagueSizes []string` (`{"8","10","12","14+"}`), `models.ADPScoringFormats []string` (`{"standard","half_ppr","ppr"}`).
  - Consumed by Task 3 (activities), Task 4 (workflows), Task 6 (API handler), and their tests.

- [ ] **Step 1: Write the failing test**

```go
// backend/internal/models/draft_adp_test.go
package models_test

import (
	"testing"

	"backend/internal/models"
)

func TestADPSegmentKey(t *testing.T) {
	cases := []struct {
		leagueSize    string
		scoringFormat string
		superflex     bool
		want          string
	}{
		{"12", "ppr", true, "12-ppr-sf"},
		{"10", "half_ppr", false, "10-half_ppr-1qb"},
		{"14+", "standard", true, "14+-standard-sf"},
	}
	for _, c := range cases {
		if got := models.ADPSegmentKey(c.leagueSize, c.scoringFormat, c.superflex); got != c.want {
			t.Errorf("ADPSegmentKey(%q, %q, %v) = %q, want %q", c.leagueSize, c.scoringFormat, c.superflex, got, c.want)
		}
	}
}

func TestAllADPSegments_Has24UniqueKeys(t *testing.T) {
	segments := models.AllADPSegments()
	if len(segments) != 24 {
		t.Fatalf("expected 24 segments, got %d", len(segments))
	}
	seen := make(map[string]bool, 24)
	for _, s := range segments {
		key := s.Key()
		if seen[key] {
			t.Errorf("duplicate segment key %q", key)
		}
		seen[key] = true
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/models/... -run 'TestADPSegmentKey|TestAllADPSegments' -v`
Expected: FAIL — compile error, `undefined: models.ADPSegmentKey` (or similar).

- [ ] **Step 3: Write the implementation**

```go
// backend/internal/models/draft_adp.go
package models

import "time"

// DraftADP is one player's average-draft-position rollup for a single
// (segment, season) — upserted daily by the ADP rollup Temporal worker from
// completed snake/linear redraft Sleeper drafts.
type DraftADP struct {
	Segment         string    `gorm:"primaryKey;column:segment"`
	Season          string    `gorm:"primaryKey;column:season"`
	SleeperPlayerID string    `gorm:"primaryKey;column:sleeper_player_id"`
	AvgPickNo       float64   `gorm:"column:avg_pick_no"`
	PickCount       int       `gorm:"column:pick_count"`
	MinPickNo       int       `gorm:"column:min_pick_no"`
	MaxPickNo       int       `gorm:"column:max_pick_no"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (DraftADP) TableName() string { return "draft_adp" }

// ADPLeagueSizes are the league_size filter/bucket values ADP is computed
// for. "14+" buckets every league with total_rosters >= 14.
var ADPLeagueSizes = []string{"8", "10", "12", "14+"}

// ADPScoringFormats are the scoring_format filter/bucket values ADP is
// computed for, matching sleeper_leagues.ppr: 0, 0.5, 1.
var ADPScoringFormats = []string{"standard", "half_ppr", "ppr"}

// ADPSegment is one (league_size, scoring_format, superflex) combination.
type ADPSegment struct {
	LeagueSize    string
	ScoringFormat string
	Superflex     bool
}

// Key returns the segment's storage/lookup key, e.g. "12-ppr-sf" or "10-half_ppr-1qb".
func (s ADPSegment) Key() string {
	return ADPSegmentKey(s.LeagueSize, s.ScoringFormat, s.Superflex)
}

// ADPSegmentKey builds a segment key from bucketed filter values.
func ADPSegmentKey(leagueSize, scoringFormat string, superflex bool) string {
	sf := "1qb"
	if superflex {
		sf = "sf"
	}
	return leagueSize + "-" + scoringFormat + "-" + sf
}

// AllADPSegments enumerates every ADP segment: the full cross product of
// ADPLeagueSizes x ADPScoringFormats x {superflex, 1qb} (24 segments).
func AllADPSegments() []ADPSegment {
	segments := make([]ADPSegment, 0, len(ADPLeagueSizes)*len(ADPScoringFormats)*2)
	for _, size := range ADPLeagueSizes {
		for _, scoring := range ADPScoringFormats {
			for _, superflex := range []bool{true, false} {
				segments = append(segments, ADPSegment{
					LeagueSize:    size,
					ScoringFormat: scoring,
					Superflex:     superflex,
				})
			}
		}
	}
	return segments
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/models/... -v`
Expected: PASS (`TestADPSegmentKey`, `TestAllADPSegments_Has24UniqueKeys`).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/models/draft_adp.go backend/internal/models/draft_adp_test.go
git commit -m "feat(sleeper): add DraftADP model and 24-segment key builder"
```

---

### Task 3: ADP rollup activities

**Files:**
- Modify: `backend/internal/activities/params.go`
- Create: `backend/internal/activities/adp_rollup.go`
- Modify: `backend/internal/activities/discovery_test.go` (extend `newTestDB`'s `AutoMigrate` call)
- Test: `backend/internal/activities/adp_rollup_test.go`

**Interfaces:**
- Consumes: `models.DraftADP`, `models.ADPSegment`, `models.ADPSegmentKey` (Task 2); existing `models.SleeperDraft`, `models.SleeperDraftPick`, `models.SleeperLeague`.
- Produces:
  - `activities.ADPRollupActivities{DB *gorm.DB}`.
  - `(a *ADPRollupActivities) ListADPSeasons(ctx context.Context) ([]string, error)`.
  - `(a *ADPRollupActivities) ComputeSegmentSeasonADP(ctx context.Context, params ComputeSegmentSeasonADPParams) error`.
  - `activities.ComputeSegmentSeasonADPParams{Segment models.ADPSegment; Season string}`.
  - Consumed by Task 4 (workflows) and Task 5 (worker registration).

- [ ] **Step 1: Extend the shared test DB helper**

In `backend/internal/activities/discovery_test.go`, add `&models.DraftADP{}` and `&models.SleeperPlayer{}` is already present — only `DraftADP` needs adding. Find:

```go
	if err := db.AutoMigrate(
		&models.SleeperUser{},
		&models.SleeperLeague{},
		&models.SleeperLeagueUser{},
		&models.SleeperDraft{},
		&models.SleeperDraftPick{},
		&models.SleeperTransaction{},
		&models.SleeperPlayer{},
		&models.SleeperPlayerWeekStat{},
		&models.SleeperWeekStatFetch{},
	); err != nil {
```

Replace with:

```go
	if err := db.AutoMigrate(
		&models.SleeperUser{},
		&models.SleeperLeague{},
		&models.SleeperLeagueUser{},
		&models.SleeperDraft{},
		&models.SleeperDraftPick{},
		&models.SleeperTransaction{},
		&models.SleeperPlayer{},
		&models.SleeperPlayerWeekStat{},
		&models.SleeperWeekStatFetch{},
		&models.DraftADP{},
	); err != nil {
```

- [ ] **Step 2: Write the failing tests**

```go
// backend/internal/activities/adp_rollup_test.go
package activities_test

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"backend/internal/activities"
	"backend/internal/models"
)

func floatPtr(v float64) *float64 { return &v }
func boolPtr(v bool) *bool        { return &v }

func seedADPLeague(t *testing.T, db *gorm.DB, id string, totalRosters int, ppr float64, superflex bool, leagueType string) {
	t.Helper()
	if err := db.Create(&models.SleeperLeague{
		SleeperLeagueID: id,
		TotalRosters:    totalRosters,
		PPR:             floatPtr(ppr),
		IsSuperflex:     boolPtr(superflex),
		LeagueType:      leagueType,
	}).Error; err != nil {
		t.Fatalf("seed league %s: %v", id, err)
	}
}

func seedADPDraft(t *testing.T, db *gorm.DB, id, leagueID, draftType, status, season string) {
	t.Helper()
	if err := db.Create(&models.SleeperDraft{
		SleeperDraftID:  id,
		SleeperLeagueID: leagueID,
		Type:            draftType,
		Status:          status,
		Season:          season,
	}).Error; err != nil {
		t.Fatalf("seed draft %s: %v", id, err)
	}
}

func seedADPPick(t *testing.T, db *gorm.DB, draftID string, round, pickNo int, playerID string) {
	t.Helper()
	if err := db.Create(&models.SleeperDraftPick{
		SleeperDraftID:  draftID,
		Round:           round,
		PickNo:          pickNo,
		SleeperPlayerID: playerID,
	}).Error; err != nil {
		t.Fatalf("seed pick %s/%d: %v", draftID, pickNo, err)
	}
}

var adpTestSegment = models.ADPSegment{LeagueSize: "12", ScoringFormat: "ppr", Superflex: true}

func TestListADPSeasons_ReturnsOnlyQualifyingSeasons(t *testing.T) {
	db := newTestDB(t)
	seedADPLeague(t, db, "lg1", 12, 1.0, true, "redraft")
	seedADPDraft(t, db, "d1", "lg1", "snake", "complete", "2024")   // qualifying
	seedADPDraft(t, db, "d2", "lg1", "auction", "complete", "2025") // wrong draft type
	seedADPLeague(t, db, "lg2", 12, 1.0, true, "dynasty")
	seedADPDraft(t, db, "d3", "lg2", "snake", "complete", "2026") // wrong league type

	a := &activities.ADPRollupActivities{DB: db}
	seasons, err := a.ListADPSeasons(context.Background())
	if err != nil {
		t.Fatalf("ListADPSeasons error: %v", err)
	}
	if len(seasons) != 1 || seasons[0] != "2024" {
		t.Errorf("expected [2024], got %v", seasons)
	}
}

func TestComputeSegmentSeasonADP_ComputesAverages(t *testing.T) {
	db := newTestDB(t)
	seedADPLeague(t, db, "lg1", 12, 1.0, true, "redraft")
	seedADPDraft(t, db, "d1", "lg1", "snake", "complete", "2024")
	seedADPDraft(t, db, "d2", "lg1", "snake", "complete", "2024")
	seedADPPick(t, db, "d1", 1, 1, "p1")
	seedADPPick(t, db, "d1", 1, 2, "p2")
	seedADPPick(t, db, "d2", 1, 3, "p1")
	seedADPPick(t, db, "d2", 1, 4, "p2")

	a := &activities.ADPRollupActivities{DB: db}
	if err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	}); err != nil {
		t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
	}

	var p1, p2 models.DraftADP
	if err := db.Where("segment = ? AND season = ? AND sleeper_player_id = ?", "12-ppr-sf", "2024", "p1").First(&p1).Error; err != nil {
		t.Fatalf("fetch p1 row: %v", err)
	}
	if p1.AvgPickNo != 2 || p1.PickCount != 2 || p1.MinPickNo != 1 || p1.MaxPickNo != 3 {
		t.Errorf("p1: got avg=%v count=%v min=%v max=%v", p1.AvgPickNo, p1.PickCount, p1.MinPickNo, p1.MaxPickNo)
	}

	if err := db.Where("segment = ? AND season = ? AND sleeper_player_id = ?", "12-ppr-sf", "2024", "p2").First(&p2).Error; err != nil {
		t.Fatalf("fetch p2 row: %v", err)
	}
	if p2.AvgPickNo != 3 || p2.PickCount != 2 || p2.MinPickNo != 2 || p2.MaxPickNo != 4 {
		t.Errorf("p2: got avg=%v count=%v min=%v max=%v", p2.AvgPickNo, p2.PickCount, p2.MinPickNo, p2.MaxPickNo)
	}
}

func TestComputeSegmentSeasonADP_ExcludesAuctionAndNonRedraft(t *testing.T) {
	db := newTestDB(t)
	seedADPLeague(t, db, "lg1", 12, 1.0, true, "redraft")
	seedADPDraft(t, db, "d-auction", "lg1", "auction", "complete", "2024")
	seedADPPick(t, db, "d-auction", 1, 1, "p-auction")

	seedADPLeague(t, db, "lg2", 12, 1.0, true, "dynasty")
	seedADPDraft(t, db, "d-dynasty", "lg2", "snake", "complete", "2024")
	seedADPPick(t, db, "d-dynasty", 1, 1, "p-dynasty")

	a := &activities.ADPRollupActivities{DB: db}
	if err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	}); err != nil {
		t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
	}

	var count int64
	db.Model(&models.DraftADP{}).Count(&count)
	if count != 0 {
		t.Errorf("expected no rows (auction/dynasty excluded), got %d", count)
	}
}

func TestComputeSegmentSeasonADP_NoMinDraftsThresholdAtWriteTime(t *testing.T) {
	db := newTestDB(t)
	seedADPLeague(t, db, "lg1", 12, 1.0, true, "redraft")
	seedADPDraft(t, db, "d1", "lg1", "snake", "complete", "2024")
	seedADPPick(t, db, "d1", 1, 1, "p1") // only 1 qualifying draft, well under the API's 20-draft threshold

	a := &activities.ADPRollupActivities{DB: db}
	if err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	}); err != nil {
		t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
	}

	var row models.DraftADP
	if err := db.Where("segment = ? AND season = ? AND sleeper_player_id = ?", "12-ppr-sf", "2024", "p1").First(&row).Error; err != nil {
		t.Fatalf("expected sub-threshold row to still be upserted: %v", err)
	}
	if row.PickCount != 1 {
		t.Errorf("expected pick_count 1, got %d", row.PickCount)
	}
}

func TestComputeSegmentSeasonADP_UpsertOverwritesPreviousRun(t *testing.T) {
	db := newTestDB(t)
	seedADPLeague(t, db, "lg1", 12, 1.0, true, "redraft")
	seedADPDraft(t, db, "d1", "lg1", "snake", "complete", "2024")
	seedADPPick(t, db, "d1", 1, 1, "p1")

	a := &activities.ADPRollupActivities{DB: db}
	run := func() {
		if err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
			Segment: adpTestSegment,
			Season:  "2024",
		}); err != nil {
			t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
		}
	}
	run() // first run: p1 picks [1] -> avg=1, count=1

	seedADPDraft(t, db, "d2", "lg1", "snake", "complete", "2024")
	seedADPPick(t, db, "d2", 1, 5, "p1")
	run() // second run: p1 picks [1,5] -> avg=3, count=2

	var rows []models.DraftADP
	db.Where("segment = ? AND season = ? AND sleeper_player_id = ?", "12-ppr-sf", "2024", "p1").Find(&rows)
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 row after upsert, got %d", len(rows))
	}
	if rows[0].AvgPickNo != 3 || rows[0].PickCount != 2 {
		t.Errorf("expected updated avg=3 count=2, got avg=%v count=%v", rows[0].AvgPickNo, rows[0].PickCount)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd backend && go test ./internal/activities/... -run 'ListADPSeasons|ComputeSegmentSeasonADP' -v`
Expected: FAIL — compile error, `undefined: activities.ADPRollupActivities`.

- [ ] **Step 4: Add `ComputeSegmentSeasonADPParams` to params.go**

In `backend/internal/activities/params.go`, find:

```go
package activities

type GetStaleUsersParams struct {
```

Replace with:

```go
package activities

import "backend/internal/models"

type GetStaleUsersParams struct {
```

Find (end of file):

```go
type GetFinalizedWeeksParams struct {
	Season string
}
```

Replace with:

```go
type GetFinalizedWeeksParams struct {
	Season string
}

type ComputeSegmentSeasonADPParams struct {
	Segment models.ADPSegment
	Season  string
}
```

- [ ] **Step 5: Write the implementation**

```go
// backend/internal/activities/adp_rollup.go
package activities

import (
	"context"
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

type adpRow struct {
	SleeperPlayerID string  `gorm:"column:sleeper_player_id"`
	AvgPickNo       float64 `gorm:"column:avg_pick_no"`
	PickCount       int     `gorm:"column:pick_count"`
	MinPickNo       int     `gorm:"column:min_pick_no"`
	MaxPickNo       int     `gorm:"column:max_pick_no"`
}

// ComputeSegmentSeasonADP computes ADP for every player picked in qualifying
// drafts matching params.Segment and params.Season, then upserts one
// draft_adp row per player. The 20-draft minimum sample size is enforced at
// API read time, not here — every player who appears at least once is
// upserted.
func (a *ADPRollupActivities) ComputeSegmentSeasonADP(ctx context.Context, params ComputeSegmentSeasonADPParams) error {
	db := a.DB.WithContext(ctx).
		Table("sleeper_draft_picks p").
		Select("p.sleeper_player_id, AVG(p.pick_no) AS avg_pick_no, COUNT(*) AS pick_count, MIN(p.pick_no) AS min_pick_no, MAX(p.pick_no) AS max_pick_no").
		Joins("JOIN sleeper_drafts d ON d.sleeper_draft_id = p.sleeper_draft_id").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id").
		Where("d.status = ? AND d.type IN ? AND l.league_type = ? AND d.season = ?",
			"complete", qualifyingDraftTypes, "redraft", params.Season).
		Where("p.sleeper_player_id != ''")
	db = applySegmentPredicate(db, params.Segment)

	var rows []adpRow
	if err := db.Group("p.sleeper_player_id").Scan(&rows).Error; err != nil {
		return err
	}

	segmentKey := params.Segment.Key()
	for _, r := range rows {
		record := models.DraftADP{
			Segment:         segmentKey,
			Season:          params.Season,
			SleeperPlayerID: r.SleeperPlayerID,
			AvgPickNo:       r.AvgPickNo,
			PickCount:       r.PickCount,
			MinPickNo:       r.MinPickNo,
			MaxPickNo:       r.MaxPickNo,
		}
		if err := a.DB.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "segment"}, {Name: "season"}, {Name: "sleeper_player_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"avg_pick_no", "pick_count", "min_pick_no", "max_pick_no", "updated_at",
			}),
		}).Create(&record).Error; err != nil {
			return err
		}
	}
	return nil
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

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd backend && go test ./internal/activities/... -v`
Expected: PASS — all new tests plus the existing `discovery_test.go` suite (confirms the `AutoMigrate` change didn't break anything).

- [ ] **Step 7: Commit**

```bash
git add backend/internal/activities/adp_rollup.go backend/internal/activities/adp_rollup_test.go backend/internal/activities/discovery_test.go backend/internal/activities/params.go
git commit -m "feat(sleeper): add ADP rollup activities (ListADPSeasons, ComputeSegmentSeasonADP)"
```

---

### Task 4: ADP rollup workflows

**Files:**
- Modify: `backend/internal/workflows/helpers.go`
- Modify: `backend/internal/workflows/params.go`
- Create: `backend/internal/workflows/adp_rollup.go`
- Modify: `backend/internal/workflows/workflows_test.go`

**Interfaces:**
- Consumes: `activities.ADPRollupActivities`, `activities.ComputeSegmentSeasonADPParams` (Task 3); `models.AllADPSegments()`, `models.ADPSegment` (Task 2).
- Produces:
  - `workflows.TaskQueueADP = "sleeper-adp"`.
  - `workflows.SegmentSeasonADPParams{Segment models.ADPSegment; Season string}`.
  - `workflows.ADPRollupDispatcher(ctx workflow.Context) error`.
  - `workflows.SegmentSeasonADPRollupWorkflow(ctx workflow.Context, params SegmentSeasonADPParams) error`.
  - Consumed by Task 5 (worker + schedule registration).

- [ ] **Step 1: Write the failing tests**

In `backend/internal/workflows/workflows_test.go`, find the import block:

```go
import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"backend/internal/activities"
	"backend/internal/workflows"
)
```

Replace with:

```go
import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/workflows"
)
```

Then append to the end of the file:

```go
// ---- ADPRollupDispatcher ----

func TestADPRollupDispatcher_SpawnsChildPerSeasonSegment(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	ara := &activities.ADPRollupActivities{}
	env.OnActivity(ara.ListADPSeasons, mock.Anything).Return([]string{"2024"}, nil)

	env.RegisterWorkflow(workflows.SegmentSeasonADPRollupWorkflow)
	segments := models.AllADPSegments()
	if len(segments) != 24 {
		t.Fatalf("expected 24 segments, got %d", len(segments))
	}
	for _, seg := range segments {
		env.OnWorkflow(workflows.SegmentSeasonADPRollupWorkflow, mock.Anything, workflows.SegmentSeasonADPParams{
			Segment: seg,
			Season:  "2024",
		}).Return(nil)
	}

	env.ExecuteWorkflow(workflows.ADPRollupDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestADPRollupDispatcher_NoSeasons_NoChildren(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	ara := &activities.ADPRollupActivities{}
	env.OnActivity(ara.ListADPSeasons, mock.Anything).Return([]string{}, nil)

	env.ExecuteWorkflow(workflows.ADPRollupDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}

// ---- SegmentSeasonADPRollupWorkflow ----

func TestSegmentSeasonADPRollupWorkflow_CallsComputeActivity(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	seg := models.ADPSegment{LeagueSize: "12", ScoringFormat: "ppr", Superflex: true}
	ara := &activities.ADPRollupActivities{}
	env.OnActivity(ara.ComputeSegmentSeasonADP, mock.Anything, activities.ComputeSegmentSeasonADPParams{
		Segment: seg,
		Season:  "2024",
	}).Return(nil)

	env.ExecuteWorkflow(workflows.SegmentSeasonADPRollupWorkflow, workflows.SegmentSeasonADPParams{
		Segment: seg,
		Season:  "2024",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestSegmentSeasonADPRollupWorkflow_ActivityFailure_WorkflowStillSucceeds(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	seg := models.ADPSegment{LeagueSize: "12", ScoringFormat: "ppr", Superflex: true}
	ara := &activities.ADPRollupActivities{}
	env.OnActivity(ara.ComputeSegmentSeasonADP, mock.Anything, activities.ComputeSegmentSeasonADPParams{
		Segment: seg,
		Season:  "2024",
	}).Return(temporal.NewApplicationError("db error", "DB_ERROR", nil))

	env.ExecuteWorkflow(workflows.SegmentSeasonADPRollupWorkflow, workflows.SegmentSeasonADPParams{
		Segment: seg,
		Season:  "2024",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError()) // logged and swallowed, not propagated
	env.AssertExpectations(t)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/workflows/... -run 'ADPRollupDispatcher|SegmentSeasonADPRollupWorkflow' -v`
Expected: FAIL — compile error, `undefined: workflows.ADPRollupDispatcher`.

- [ ] **Step 3: Add `TaskQueueADP` to helpers.go**

In `backend/internal/workflows/helpers.go`, find:

```go
const (
	TaskQueueDiscovery    = "sleeper-discovery"
	TaskQueueDrafts       = "sleeper-drafts"
	TaskQueueTransactions = "sleeper-transactions"
	TaskQueuePlayerSync   = "sleeper-player-sync"
	TaskQueueWeekStats    = "sleeper-week-stats"
	BatchSize             = 10
	SyncBatchSize         = 400
)
```

Replace with:

```go
const (
	TaskQueueDiscovery    = "sleeper-discovery"
	TaskQueueDrafts       = "sleeper-drafts"
	TaskQueueTransactions = "sleeper-transactions"
	TaskQueuePlayerSync   = "sleeper-player-sync"
	TaskQueueWeekStats    = "sleeper-week-stats"
	TaskQueueADP          = "sleeper-adp"
	BatchSize             = 10
	SyncBatchSize         = 400
)
```

- [ ] **Step 4: Add `SegmentSeasonADPParams` to params.go**

In `backend/internal/workflows/params.go`, find:

```go
package workflows

type UserDiscoveryParams struct {
	UserID string
}
```

Replace with:

```go
package workflows

import "backend/internal/models"

type UserDiscoveryParams struct {
	UserID string
}
```

Find (end of file):

```go
type SyncWeekStatsParams struct {
	Season string
}
```

Replace with:

```go
type SyncWeekStatsParams struct {
	Season string
}

type SegmentSeasonADPParams struct {
	Segment models.ADPSegment
	Season  string
}
```

- [ ] **Step 5: Write the implementation**

```go
// backend/internal/workflows/adp_rollup.go
package workflows

import (
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
	"backend/internal/models"
)

// ADPRollupDispatcher is a scheduled workflow that discovers every season
// with qualifying draft data, crosses it with the fixed set of 24 ADP
// segments, and spawns one SegmentSeasonADPRollupWorkflow child per
// (season, segment) pair (fire-and-forget).
func ADPRollupDispatcher(ctx workflow.Context) error {
	ara := &activities.ADPRollupActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var seasons []string
	if err := workflow.ExecuteActivity(actCtx, ara.ListADPSeasons).Get(ctx, &seasons); err != nil {
		return err
	}

	for _, season := range seasons {
		for _, seg := range models.AllADPSegments() {
			cwo := workflow.ChildWorkflowOptions{
				TaskQueue:         TaskQueueADP,
				ParentClosePolicy: enumspb.PARENT_CLOSE_POLICY_ABANDON,
			}
			params := SegmentSeasonADPParams{Segment: seg, Season: season}
			f := workflow.ExecuteChildWorkflow(workflow.WithChildOptions(ctx, cwo), SegmentSeasonADPRollupWorkflow, params)
			if err := f.GetChildWorkflowExecution().Get(ctx, nil); err != nil {
				workflow.GetLogger(ctx).Warn("failed to start SegmentSeasonADPRollupWorkflow",
					"segment", seg.Key(), "season", season, "error", err)
			}
		}
	}
	return nil
}

// SegmentSeasonADPRollupWorkflow computes and upserts ADP for one
// (segment, season) pair. A compute failure is logged rather than returned,
// so one bad segment/season doesn't surface as a workflow failure.
func SegmentSeasonADPRollupWorkflow(ctx workflow.Context, params SegmentSeasonADPParams) error {
	ara := &activities.ADPRollupActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	if err := workflow.ExecuteActivity(actCtx, ara.ComputeSegmentSeasonADP, activities.ComputeSegmentSeasonADPParams{
		Segment: params.Segment,
		Season:  params.Season,
	}).Get(ctx, nil); err != nil {
		workflow.GetLogger(ctx).Warn("ComputeSegmentSeasonADP failed",
			"segment", params.Segment.Key(), "season", params.Season, "error", err)
	}
	return nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd backend && go test ./internal/workflows/... -v`
Expected: PASS — all new tests plus the existing dispatcher/workflow suite.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/workflows/adp_rollup.go backend/internal/workflows/helpers.go backend/internal/workflows/params.go backend/internal/workflows/workflows_test.go
git commit -m "feat(sleeper): add ADPRollupDispatcher and SegmentSeasonADPRollupWorkflow"
```

---

### Task 5: Worker registration and daily schedule

**Files:**
- Modify: `backend/cmd/worker/main.go`
- Modify: `backend/schedules/register.go`

**Interfaces:**
- Consumes: `workflows.TaskQueueADP`, `workflows.ADPRollupDispatcher`, `workflows.SegmentSeasonADPRollupWorkflow`, `activities.ADPRollupActivities` (Tasks 3, 4).
- Produces: a running `worker.Worker` on `sleeper-adp` and a registered Temporal schedule `sleeper-adp-rollup-schedule`. No new Go symbols consumed by later tasks — this is wiring only, verified by build.

- [ ] **Step 1: Register the ADP worker in cmd/worker/main.go**

Find:

```go
	sc := sleeper.New()
	da := &activities.DiscoveryActivities{DB: database.DB, Sleeper: sc}
	dfa := &activities.DataFetchActivities{DB: database.DB, Sleeper: sc}
	psa := &activities.PlayerSyncActivities{DB: database.DB, Sleeper: sc}
	wsa := &activities.WeekStatsActivities{DB: database.DB, Sleeper: sc}
```

Replace with:

```go
	sc := sleeper.New()
	da := &activities.DiscoveryActivities{DB: database.DB, Sleeper: sc}
	dfa := &activities.DataFetchActivities{DB: database.DB, Sleeper: sc}
	psa := &activities.PlayerSyncActivities{DB: database.DB, Sleeper: sc}
	wsa := &activities.WeekStatsActivities{DB: database.DB, Sleeper: sc}
	aa := &activities.ADPRollupActivities{DB: database.DB}
```

Find:

```go
	// Week stats worker: WeekStatsSyncDispatcher + SyncWeekStats
	wsw := worker.New(c, workflows.TaskQueueWeekStats, worker.Options{})
	wsw.RegisterWorkflow(workflows.WeekStatsSyncDispatcher)
	wsw.RegisterWorkflow(workflows.SyncWeekStats)
	wsw.RegisterActivity(wsa)

	workers := []worker.Worker{dw, draftsw, transactionsw, psw, wsw}
```

Replace with:

```go
	// Week stats worker: WeekStatsSyncDispatcher + SyncWeekStats
	wsw := worker.New(c, workflows.TaskQueueWeekStats, worker.Options{})
	wsw.RegisterWorkflow(workflows.WeekStatsSyncDispatcher)
	wsw.RegisterWorkflow(workflows.SyncWeekStats)
	wsw.RegisterActivity(wsa)

	// ADP worker: ADPRollupDispatcher + SegmentSeasonADPRollupWorkflow
	adpw := worker.New(c, workflows.TaskQueueADP, worker.Options{
		MaxConcurrentActivityExecutionSize: 50,
		MaxConcurrentWorkflowTaskPollers:   10,
	})
	adpw.RegisterWorkflow(workflows.ADPRollupDispatcher)
	adpw.RegisterWorkflow(workflows.SegmentSeasonADPRollupWorkflow)
	adpw.RegisterActivity(aa)

	workers := []worker.Worker{dw, draftsw, transactionsw, psw, wsw, adpw}
```

- [ ] **Step 2: Add the daily schedule in schedules/register.go**

Find:

```go
	return upsert(ctx, c, client.ScheduleOptions{
		ID: "sleeper-week-stats-schedule",
		Spec: client.ScheduleSpec{
			Calendars: []client.ScheduleCalendarSpec{
				{
					Hour:   []client.ScheduleRange{{Start: 9}}, // 04:00 EST (UTC-5)
					Minute: []client.ScheduleRange{{Start: 0}},
				},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			Workflow:  workflows.WeekStatsSyncDispatcher,
			TaskQueue: workflows.TaskQueueWeekStats,
		},
	})
}
```

Replace with:

```go
	if err := upsert(ctx, c, client.ScheduleOptions{
		ID: "sleeper-week-stats-schedule",
		Spec: client.ScheduleSpec{
			Calendars: []client.ScheduleCalendarSpec{
				{
					Hour:   []client.ScheduleRange{{Start: 9}}, // 04:00 EST (UTC-5)
					Minute: []client.ScheduleRange{{Start: 0}},
				},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			Workflow:  workflows.WeekStatsSyncDispatcher,
			TaskQueue: workflows.TaskQueueWeekStats,
		},
	}); err != nil {
		return err
	}

	return upsert(ctx, c, client.ScheduleOptions{
		ID: "sleeper-adp-rollup-schedule",
		Spec: client.ScheduleSpec{
			Calendars: []client.ScheduleCalendarSpec{
				{
					Hour:   []client.ScheduleRange{{Start: 11}}, // 06:00 EST (UTC-5)
					Minute: []client.ScheduleRange{{Start: 0}},
				},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			Workflow:  workflows.ADPRollupDispatcher,
			TaskQueue: workflows.TaskQueueADP,
		},
	})
}
```

(This schedule runs after week-stats, once a day, so ADP always rolls up from that day's freshest draft-sync data.)

- [ ] **Step 3: Verify it builds**

Run: `cd backend && go build ./...`
Expected: no output, exit 0.

- [ ] **Step 4: Commit**

```bash
git add backend/cmd/worker/main.go backend/schedules/register.go
git commit -m "feat(sleeper): register ADP rollup worker and daily schedule"
```

---

### Task 6: `GET /api/v1/sleeper/adp` handler

**Files:**
- Create: `backend/internal/api/handlers/draft_adp.go`
- Modify: `backend/internal/api/routes.go`
- Test: `backend/internal/api/handlers/draft_adp_test.go`

**Interfaces:**
- Consumes: `database.DB` (package global), `models.DraftADP`, `models.ADPSegmentKey`, `models.SleeperPlayer`, the package-private `parsePagination(c *gin.Context) (page, limit int)` already defined in `sleeper.go`.
- Produces:
  - `handlers.SleeperADPItem{SleeperPlayerID, Name, Position, NflTeam string; AvgPickNo float64; PickCount, MinPickNo, MaxPickNo int}`.
  - `handlers.SleeperADPResponse{Players []SleeperADPItem; Season string; AvailableSeasons []string; Total int64; Page, Limit, TotalPages int}`.
  - `handlers.GetSleeperADP(c *gin.Context)`.
  - Route `GET /api/v1/sleeper/adp`. Consumed by Task 7 (frontend service).

- [ ] **Step 1: Write the failing tests**

```go
// backend/internal/api/handlers/draft_adp_test.go
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"backend/internal/database"
	"backend/internal/models"
)

func newDraftADPTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.DraftADP{}, &models.SleeperPlayer{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func withDraftADPTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })
}

func seedADPPlayer(t *testing.T, db *gorm.DB, id, name, position, team string) {
	t.Helper()
	if err := db.Create(&models.SleeperPlayer{
		SleeperPlayerID: id,
		FullName:        name,
		Position:        position,
		NflTeam:         team,
	}).Error; err != nil {
		t.Fatalf("seed player %s: %v", id, err)
	}
}

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
	}).Error; err != nil {
		t.Fatalf("seed adp row %s/%s/%s: %v", segment, season, playerID, err)
	}
}

func performGetSleeperADP(t *testing.T, query string) (*httptest.ResponseRecorder, SleeperADPResponse) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/sleeper/adp", GetSleeperADP)

	req := httptest.NewRequest(http.MethodGet, "/sleeper/adp"+query, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp SleeperADPResponse
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
	}
	return w, resp
}

func TestGetSleeperADP_DefaultsAndOrdering(t *testing.T) {
	db := newDraftADPTestDB(t)
	withDraftADPTestDB(t, db)

	seedADPPlayer(t, db, "p1", "Player One", "RB", "KC")
	seedADPPlayer(t, db, "p2", "Player Two", "WR", "SF")
	seedADPRow(t, db, "12-ppr-sf", "2024", "p1", 5.0, 25)
	seedADPRow(t, db, "12-ppr-sf", "2024", "p2", 2.0, 30)

	w, resp := performGetSleeperADP(t, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(resp.Players) != 2 {
		t.Fatalf("expected 2 players, got %d", len(resp.Players))
	}
	if resp.Players[0].SleeperPlayerID != "p2" {
		t.Errorf("expected p2 (avg 2.0) ranked first, got %s", resp.Players[0].SleeperPlayerID)
	}
	if resp.Season != "2024" {
		t.Errorf("expected default season 2024, got %q", resp.Season)
	}
}

func TestGetSleeperADP_MinDraftsFiltersLowSampleSize(t *testing.T) {
	db := newDraftADPTestDB(t)
	withDraftADPTestDB(t, db)

	seedADPPlayer(t, db, "p1", "Under Threshold", "RB", "KC")
	seedADPPlayer(t, db, "p2", "Over Threshold", "WR", "SF")
	seedADPRow(t, db, "12-ppr-sf", "2024", "p1", 5.0, 19) // below default min_drafts=20
	seedADPRow(t, db, "12-ppr-sf", "2024", "p2", 2.0, 20)

	_, resp := performGetSleeperADP(t, "")
	if len(resp.Players) != 1 || resp.Players[0].SleeperPlayerID != "p2" {
		t.Errorf("expected only p2 (pick_count >= 20), got %+v", resp.Players)
	}
}

func TestGetSleeperADP_ExplicitFiltersBuildSegmentKey(t *testing.T) {
	db := newDraftADPTestDB(t)
	withDraftADPTestDB(t, db)

	seedADPPlayer(t, db, "p1", "Standard 10 Team", "QB", "BUF")
	seedADPRow(t, db, "10-standard-1qb", "2023", "p1", 3.0, 25)

	_, resp := performGetSleeperADP(t, "?league_size=10&scoring_format=standard&superflex=false&season=2023")
	if len(resp.Players) != 1 || resp.Players[0].SleeperPlayerID != "p1" {
		t.Errorf("expected p1 from 10-standard-1qb/2023, got %+v", resp.Players)
	}
}

func TestGetSleeperADP_SeasonDefaultsToMostRecent(t *testing.T) {
	db := newDraftADPTestDB(t)
	withDraftADPTestDB(t, db)

	seedADPPlayer(t, db, "p-old", "Old Season", "RB", "KC")
	seedADPPlayer(t, db, "p-new", "New Season", "RB", "KC")
	seedADPRow(t, db, "12-ppr-sf", "2023", "p-old", 5.0, 25)
	seedADPRow(t, db, "12-ppr-sf", "2024", "p-new", 5.0, 25)

	_, resp := performGetSleeperADP(t, "")
	if resp.Season != "2024" {
		t.Errorf("expected default season 2024 (most recent), got %q", resp.Season)
	}
	if len(resp.Players) != 1 || resp.Players[0].SleeperPlayerID != "p-new" {
		t.Errorf("expected only 2024's player, got %+v", resp.Players)
	}
	if len(resp.AvailableSeasons) != 2 || resp.AvailableSeasons[0] != "2024" || resp.AvailableSeasons[1] != "2023" {
		t.Errorf("expected available_seasons [2024, 2023], got %v", resp.AvailableSeasons)
	}
}

func TestGetSleeperADP_EmptyTable(t *testing.T) {
	db := newDraftADPTestDB(t)
	withDraftADPTestDB(t, db)

	w, resp := performGetSleeperADP(t, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(resp.Players) != 0 || resp.Total != 0 {
		t.Errorf("expected empty result, got %+v", resp)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/api/handlers/... -run TestGetSleeperADP -v`
Expected: FAIL — compile error, `undefined: GetSleeperADP` / `undefined: SleeperADPResponse`.

- [ ] **Step 3: Write the implementation**

```go
// backend/internal/api/handlers/draft_adp.go
package handlers

import (
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"backend/internal/database"
	"backend/internal/models"
)

// SleeperADPItem is a single player's ADP row in the ranked list.
type SleeperADPItem struct {
	SleeperPlayerID string  `json:"sleeper_player_id"`
	Name            string  `json:"name"`
	Position        string  `json:"position"`
	NflTeam         string  `json:"nfl_team"`
	AvgPickNo       float64 `json:"avg_pick_no"`
	PickCount       int     `json:"pick_count"`
	MinPickNo       int     `json:"min_pick_no"`
	MaxPickNo       int     `json:"max_pick_no"`
}

// SleeperADPResponse is the paginated response for GET /api/v1/sleeper/adp.
type SleeperADPResponse struct {
	Players          []SleeperADPItem `json:"players"`
	Season           string           `json:"season"`
	AvailableSeasons []string         `json:"available_seasons"`
	Total            int64            `json:"total"`
	Page             int              `json:"page"`
	Limit            int              `json:"limit"`
	TotalPages       int              `json:"total_pages"`
}

// defaultADPMinDrafts is the minimum number of qualifying drafts a player
// must appear in for a segment/season before showing up in the ADP list.
const defaultADPMinDrafts = 20

type adpItemRow struct {
	SleeperPlayerID string  `gorm:"column:sleeper_player_id"`
	Name            string  `gorm:"column:full_name"`
	Position        string  `gorm:"column:position"`
	NflTeam         string  `gorm:"column:nfl_team"`
	AvgPickNo       float64 `gorm:"column:avg_pick_no"`
	PickCount       int     `gorm:"column:pick_count"`
	MinPickNo       int     `gorm:"column:min_pick_no"`
	MaxPickNo       int     `gorm:"column:max_pick_no"`
}

// GetSleeperADP returns a paginated, ADP-ranked player list for one
// (league_size, scoring_format, superflex, season) combination, populated by
// the daily ADP rollup worker.
// Supports query filters: league_size (8|10|12|14+, default 12),
// scoring_format (standard|half_ppr|ppr, default ppr), superflex
// (true|false, default true), season (defaults to the most recent season
// with data for the resolved segment), min_drafts (default 20).
func GetSleeperADP(c *gin.Context) {
	page, limit := parsePagination(c)
	offset := (page - 1) * limit

	leagueSize := c.DefaultQuery("league_size", "12")
	scoringFormat := c.DefaultQuery("scoring_format", "ppr")
	superflex := c.DefaultQuery("superflex", "true") == "true"
	segment := models.ADPSegmentKey(leagueSize, scoringFormat, superflex)

	minDrafts := defaultADPMinDrafts
	if v, err := strconv.Atoi(c.Query("min_drafts")); err == nil && v >= 0 {
		minDrafts = v
	}

	var availableSeasons []string
	database.DB.Model(&models.DraftADP{}).
		Where("segment = ?", segment).
		Distinct("season").
		Order("season DESC").
		Pluck("season", &availableSeasons)

	season := c.Query("season")
	if season == "" && len(availableSeasons) > 0 {
		season = availableSeasons[0]
	}

	var total int64
	database.DB.Table("draft_adp a").
		Where("a.segment = ? AND a.season = ? AND a.pick_count >= ?", segment, season, minDrafts).
		Count(&total)

	var rows []adpItemRow
	database.DB.Table("draft_adp a").
		Select("a.sleeper_player_id, p.full_name, p.position, p.nfl_team, a.avg_pick_no, a.pick_count, a.min_pick_no, a.max_pick_no").
		Joins("JOIN sleeper_players p ON p.sleeper_player_id = a.sleeper_player_id").
		Where("a.segment = ? AND a.season = ? AND a.pick_count >= ?", segment, season, minDrafts).
		Order("a.avg_pick_no ASC").
		Limit(limit).Offset(offset).
		Scan(&rows)

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
		}
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	c.JSON(http.StatusOK, SleeperADPResponse{
		Players:          items,
		Season:           season,
		AvailableSeasons: availableSeasons,
		Total:            total,
		Page:             page,
		Limit:            limit,
		TotalPages:       totalPages,
	})
}
```

- [ ] **Step 4: Register the route**

In `backend/internal/api/routes.go`, find:

```go
	sleeper := v1.Group("/sleeper")
	sleeper.GET("/stats", handlers.GetSleeperStats)
	sleeper.GET("/trades", handlers.GetSleeperTrades)
	sleeper.GET("/transactions", handlers.GetSleeperTransactions)
	sleeper.GET("/drafts", handlers.GetSleeperDrafts)
```

Replace with:

```go
	sleeper := v1.Group("/sleeper")
	sleeper.GET("/stats", handlers.GetSleeperStats)
	sleeper.GET("/trades", handlers.GetSleeperTrades)
	sleeper.GET("/transactions", handlers.GetSleeperTransactions)
	sleeper.GET("/drafts", handlers.GetSleeperDrafts)
	sleeper.GET("/adp", handlers.GetSleeperADP)
```

(The old `/drafts` route is removed in Task 10, once the frontend no longer needs it.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd backend && go test ./internal/api/handlers/... -v`
Expected: PASS — all new `TestGetSleeperADP_*` tests plus the existing handler test suite (confirms nothing broke).

- [ ] **Step 6: Commit**

```bash
git add backend/internal/api/handlers/draft_adp.go backend/internal/api/handlers/draft_adp_test.go backend/internal/api/routes.go
git commit -m "feat(sleeper): add GET /api/v1/sleeper/adp endpoint"
```

---

### Task 7: Frontend data layer (types, service, hook)

**Files:**
- Modify: `frontend/src/types/models.ts`
- Modify: `frontend/src/services/sleeperService.ts`
- Modify: `frontend/src/hooks/useSleeperData.ts`

**Interfaces:**
- Consumes: `GET /api/v1/sleeper/adp` response shape from Task 6 (`players`, `season`, `available_seasons`, `total`, `page`, `limit`, `total_pages`).
- Produces:
  - `SleeperADPItem`, `SleeperADPResponse`, `SleeperADPFilters` types.
  - `sleeperService.getADP(page, limit, filters): Promise<SleeperADPResponse>`.
  - `useSleeperADP(page, limit, filters)` hook returning `{ items, season, availableSeasons, total, totalPages, isLoading, error, refetch }`.
  - Consumed by Task 8 (filter bar) and Task 9 (page).
- Also removes now-dead code: the `SleeperDraft`/`SleeperDraftsResponse` types, `sleeperService.getDrafts`, and `useSleeperDrafts` — nothing outside `pages/sleeper/drafts.tsx` references them (confirmed by repo-wide search), and that page is rewritten in Task 9.

- [ ] **Step 1: Update types/models.ts**

Find:

```ts
export interface SleeperDraft {
  id: string;
  league_id: string;
  league_name: string;
  type: string;
  status: string;
  season: string;
  pick_count: number;
}

export interface SleeperTradesResponse {
```

Replace with:

```ts
export interface SleeperTradesResponse {
```

Find:

```ts
export interface SleeperDraftsResponse {
  drafts: SleeperDraft[];
  total: number;
  page: number;
  limit: number;
  total_pages: number;
}

export interface SleeperTransaction {
```

Replace with:

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
}

export interface SleeperADPResponse {
  players: SleeperADPItem[];
  season: string;
  available_seasons: string[];
  total: number;
  page: number;
  limit: number;
  total_pages: number;
}

export interface SleeperTransaction {
```

Find:

```ts
export interface SleeperLeagueFilters {
  league_size?: string;
  scoring_format?: string;
  draft_type?: string;
  league_type?: string;
  exclude_picks?: string;
}
```

Replace with:

```ts
export interface SleeperLeagueFilters {
  league_size?: string;
  scoring_format?: string;
  draft_type?: string;
  league_type?: string;
  exclude_picks?: string;
}

export interface SleeperADPFilters {
  league_size?: string;
  scoring_format?: string;
  superflex?: string;
  season?: string;
}
```

- [ ] **Step 2: Update sleeperService.ts**

Find:

```ts
import { apiClient } from './apiClient';
import {
  SleeperStats,
  SleeperTradesResponse,
  SleeperDraftsResponse,
  SleeperTransactionsResponse,
  SleeperLeagueFilters,
} from '../types/models';
```

Replace with:

```ts
import { apiClient } from './apiClient';
import {
  SleeperStats,
  SleeperTradesResponse,
  SleeperADPResponse,
  SleeperTransactionsResponse,
  SleeperLeagueFilters,
  SleeperADPFilters,
} from '../types/models';
```

Find:

```ts
  getDrafts: (
    page = 1,
    limit = 25,
    filters: SleeperLeagueFilters = {}
  ): Promise<SleeperDraftsResponse> =>
    apiClient.get<SleeperDraftsResponse>(
      `/sleeper/drafts${buildQuery({ page, limit, ...filters })}`
    ),
```

Replace with:

```ts
  getADP: (
    page = 1,
    limit = 25,
    filters: SleeperADPFilters = {}
  ): Promise<SleeperADPResponse> =>
    apiClient.get<SleeperADPResponse>(
      `/sleeper/adp${buildQuery({ page, limit, ...filters })}`
    ),
```

- [ ] **Step 3: Update useSleeperData.ts**

Find:

```ts
import { useState, useEffect, useCallback } from 'react';
import {
  SleeperStats,
  SleeperTrade,
  SleeperDraft,
  SleeperTransaction,
  SleeperLeagueFilters,
} from '../types/models';
import { sleeperService } from '../services/sleeperService';
```

Replace with:

```ts
import { useState, useEffect, useCallback } from 'react';
import {
  SleeperStats,
  SleeperTrade,
  SleeperADPItem,
  SleeperTransaction,
  SleeperLeagueFilters,
  SleeperADPFilters,
} from '../types/models';
import { sleeperService } from '../services/sleeperService';
```

Find (the whole `useSleeperDrafts` function, currently the last function in the file):

```ts
export function useSleeperDrafts(page: number, limit: number, filters: SleeperLeagueFilters = {}) {
  const [state, setState] = useState<PaginatedState<SleeperDraft>>({
    items: [], total: 0, totalPages: 0, isLoading: true, error: null,
  });

  const filtersKey = JSON.stringify(filters);

  const fetch = useCallback(async () => {
    setState(s => ({ ...s, isLoading: true, error: null }));
    try {
      const data = await sleeperService.getDrafts(page, limit, filters);
      setState({ items: data.drafts, total: data.total, totalPages: data.total_pages, isLoading: false, error: null });
    } catch (err) {
      setState(s => ({ ...s, isLoading: false, error: err instanceof Error ? err : new Error('Failed to fetch drafts') }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, limit, filtersKey]);

  useEffect(() => { fetch(); }, [fetch]);
  return { ...state, refetch: fetch };
}
```

Replace with:

```ts
interface ADPState {
  items: SleeperADPItem[];
  season: string;
  availableSeasons: string[];
  total: number;
  totalPages: number;
  isLoading: boolean;
  error: Error | null;
}

export function useSleeperADP(page: number, limit: number, filters: SleeperADPFilters = {}) {
  const [state, setState] = useState<ADPState>({
    items: [], season: '', availableSeasons: [], total: 0, totalPages: 0, isLoading: true, error: null,
  });

  const filtersKey = JSON.stringify(filters);

  const fetch = useCallback(async () => {
    setState(s => ({ ...s, isLoading: true, error: null }));
    try {
      const data = await sleeperService.getADP(page, limit, filters);
      setState({
        items: data.players,
        season: data.season,
        availableSeasons: data.available_seasons,
        total: data.total,
        totalPages: data.total_pages,
        isLoading: false,
        error: null,
      });
    } catch (err) {
      setState(s => ({ ...s, isLoading: false, error: err instanceof Error ? err : new Error('Failed to fetch ADP') }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, limit, filtersKey]);

  useEffect(() => { fetch(); }, [fetch]);
  return { ...state, refetch: fetch };
}
```

- [ ] **Step 4: Verify it type-checks**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors. (`drafts.tsx` will still reference the now-removed `useSleeperDrafts`/`SleeperDraft` at this point — that's expected and fixed in Task 9. If `tsc --noEmit` fails only on `pages/sleeper/drafts.tsx`, that's the known, temporary breakage; any other file failing is a real bug to fix here.)

- [ ] **Step 5: Commit**

```bash
git add frontend/src/types/models.ts frontend/src/services/sleeperService.ts frontend/src/hooks/useSleeperData.ts
git commit -m "feat(sleeper): add ADP types/service/hook, remove dead drafts-list ones"
```

---

### Task 8: ADP filter bar component

**Files:**
- Modify: `frontend/src/components/LeagueFilterBar.tsx`
- Create: `frontend/src/components/ADPFilterBar.tsx`

**Interfaces:**
- Consumes: `SleeperADPFilters` (Task 7); exported `PillGroup` from `LeagueFilterBar.tsx`.
- Produces: `ADPFilterBar` (default export) — props `{ filters: SleeperADPFilters; onChange: (f: SleeperADPFilters) => void; availableSeasons: string[] }`. Consumed by Task 9.

- [ ] **Step 1: Export `PillGroup` from LeagueFilterBar.tsx for reuse**

In `frontend/src/components/LeagueFilterBar.tsx`, find:

```tsx
interface PillGroupProps {
  label: string;
  options: { value: string; label: string }[];
  value: string;
  onChange: (v: string) => void;
}

function PillGroup({ label, options, value, onChange }: PillGroupProps) {
```

Replace with:

```tsx
export interface PillGroupProps {
  label: string;
  options: { value: string; label: string }[];
  value: string;
  onChange: (v: string) => void;
}

export function PillGroup({ label, options, value, onChange }: PillGroupProps) {
```

- [ ] **Step 2: Write ADPFilterBar.tsx**

```tsx
// frontend/src/components/ADPFilterBar.tsx
import { PillGroup } from './LeagueFilterBar';
import { SleeperADPFilters } from '../types/models';

interface ADPFilterBarProps {
  filters: SleeperADPFilters;
  onChange: (filters: SleeperADPFilters) => void;
  availableSeasons: string[];
}

const LEAGUE_SIZES = [
  { value: '8', label: '8' },
  { value: '10', label: '10' },
  { value: '12', label: '12' },
  { value: '14+', label: '14+' },
];
const SCORING_FORMATS = [
  { value: 'standard', label: 'Standard' },
  { value: 'half_ppr', label: 'Half-PPR' },
  { value: 'ppr', label: 'PPR' },
];
const SUPERFLEX_OPTIONS = [
  { value: 'true', label: 'Superflex' },
  { value: 'false', label: '1QB' },
];

export default function ADPFilterBar({ filters, onChange, availableSeasons }: ADPFilterBarProps) {
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
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Verify it type-checks**

Run: `cd frontend && npx tsc --noEmit`
Expected: no new errors introduced by these two files (the pre-existing `drafts.tsx` breakage from Task 7 is still expected here — fixed next).

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/LeagueFilterBar.tsx frontend/src/components/ADPFilterBar.tsx
git commit -m "feat(sleeper): add ADPFilterBar component"
```

---

### Task 9: Rewrite the `/sleeper/drafts` page as the ADP report

**Files:**
- Modify: `frontend/src/pages/sleeper/drafts.tsx`

**Interfaces:**
- Consumes: `useSleeperADP` (Task 7), `ADPFilterBar` (Task 8), `SleeperADPFilters` (Task 7).
- Produces: the rendered `/sleeper/drafts` page (URL unchanged — this replaces its content, per the request; it doesn't add a new route).

- [ ] **Step 1: Replace the page**

Overwrite `frontend/src/pages/sleeper/drafts.tsx`:

```tsx
import { useEffect, useState } from "react";
import { useRouter } from "next/router";
import Layout from "../../components/Layout";
import ADPFilterBar from "../../components/ADPFilterBar";
import { useSleeperADP } from "../../hooks/useSleeperData";
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
  const [ready, setReady] = useState(false);

  useEffect(() => {
    if (!router.isReady) return;
    setFilters(filtersFromQuery(router.query));
    const p = parseInt(router.query.page as string);
    if (p > 0) setPage(p);
    setReady(true);
  }, [router.isReady, router.query]);

  const { items, season, availableSeasons, total, totalPages, isLoading, error } = useSleeperADP(
    ready ? page : 1,
    LIMIT,
    ready ? filters : {}
  );

  function applyFilters(next: SleeperADPFilters) {
    setFilters(next);
    setPage(1);
    const q: Record<string, string> = { page: "1" };
    if (next.league_size) q.league_size = next.league_size;
    if (next.scoring_format) q.scoring_format = next.scoring_format;
    if (next.superflex) q.superflex = next.superflex;
    if (next.season) q.season = next.season;
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

        <ADPFilterBar filters={filters} onChange={applyFilters} availableSeasons={availableSeasons} />

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
                <th className="px-4 py-3 text-center text-sm font-medium text-gray-700 dark:text-gray-300">Drafts</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
              {isLoading ? (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                    <div className="flex justify-center items-center space-x-2">
                      <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                      <span>Loading ADP…</span>
                    </div>
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
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

- [ ] **Step 2: Verify it type-checks and builds**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: no errors (this is the point where the Task 7 `drafts.tsx` breakage resolves — the whole frontend should now be clean).

- [ ] **Step 3: Manually verify in the browser**

Run: `cd backend && make run` (in one terminal) and `cd frontend && npm run dev` (in another). This requires `DATABASE_URL` pointed at a Postgres instance with the Task 1 migration applied and at least one `draft_adp` row seeded — if no backend/DB is available in this environment, skip this step and note it in the task handoff instead of claiming it was verified.

Visit `http://localhost:3000/sleeper/drafts` and confirm:
- The page loads without a console error, showing "Average Draft Position" as the heading.
- The filter bar renders Size/Scoring/Format/Season pill groups.
- Changing a filter updates the URL query string and re-fetches.
- With no `draft_adp` rows for the selected segment/season, the page shows "No players found for this filter combination." rather than crashing.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/pages/sleeper/drafts.tsx
git commit -m "feat(sleeper): replace drafts list page with ADP report"
```

---

### Task 10: Remove the old drafts-list backend endpoint

**Files:**
- Modify: `backend/internal/api/handlers/sleeper.go`
- Modify: `backend/internal/api/routes.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: nothing new — this is dead-code removal now that Task 9 means nothing calls `GET /api/v1/sleeper/drafts` anymore. `applyLeagueFilters`/`hasLeagueFilters` (used by `GetSleeperTrades` and `GetSleeperTransactions` too) are left in place.

- [ ] **Step 1: Remove the route**

In `backend/internal/api/routes.go`, find:

```go
	sleeper := v1.Group("/sleeper")
	sleeper.GET("/stats", handlers.GetSleeperStats)
	sleeper.GET("/trades", handlers.GetSleeperTrades)
	sleeper.GET("/transactions", handlers.GetSleeperTransactions)
	sleeper.GET("/drafts", handlers.GetSleeperDrafts)
	sleeper.GET("/adp", handlers.GetSleeperADP)
```

Replace with:

```go
	sleeper := v1.Group("/sleeper")
	sleeper.GET("/stats", handlers.GetSleeperStats)
	sleeper.GET("/trades", handlers.GetSleeperTrades)
	sleeper.GET("/transactions", handlers.GetSleeperTransactions)
	sleeper.GET("/adp", handlers.GetSleeperADP)
```

- [ ] **Step 2: Remove the handler and its types**

In `backend/internal/api/handlers/sleeper.go`, find:

```go
// SleeperDraftItem is a single row in the drafts list.
type SleeperDraftItem struct {
	ID         string `json:"id"`
	LeagueID   string `json:"league_id"`
	LeagueName string `json:"league_name"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	Season     string `json:"season"`
	PickCount  int64  `json:"pick_count"`
}

// SleeperDraftsResponse is the paginated response for GET /api/v1/sleeper/drafts.
type SleeperDraftsResponse struct {
	Drafts     []SleeperDraftItem `json:"drafts"`
	Total      int64              `json:"total"`
	Page       int                `json:"page"`
	Limit      int                `json:"limit"`
	TotalPages int                `json:"total_pages"`
}
```

Delete this block entirely (no replacement).

Find:

```go
// GetSleeperDrafts returns a paginated list of completed Sleeper drafts with pick counts.
func GetSleeperDrafts(c *gin.Context) {
	page, limit := parsePagination(c)
	offset := (page - 1) * limit

	type draftRow struct {
		SleeperDraftID  string `gorm:"column:sleeper_draft_id"`
		SleeperLeagueID string `gorm:"column:sleeper_league_id"`
		LeagueName      string `gorm:"column:league_name"`
		Type            string `gorm:"column:type"`
		Status          string `gorm:"column:status"`
		Season          string `gorm:"column:season"`
		PickCount       int64  `gorm:"column:pick_count"`
	}

	var rows []draftRow
	var total int64

	db := database.DB.Table("sleeper_drafts d").
		Select("d.sleeper_draft_id, d.sleeper_league_id, l.name as league_name, d.type, d.status, d.season, COUNT(p.pick_no) as pick_count").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id").
		Joins("LEFT JOIN sleeper_draft_picks p ON p.sleeper_draft_id = d.sleeper_draft_id").
		Where("d.status = ?", "complete").
		Group("d.sleeper_draft_id, d.sleeper_league_id, l.name, d.type, d.status, d.season")
	db = applyLeagueFilters(db, c, "l")

	if hasLeagueFilters(c) {
		countDB := database.DB.Table("sleeper_drafts d").
			Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id").
			Where("d.status = ?", "complete")
		countDB = applyLeagueFilters(countDB, c, "l")
		countDB.Count(&total)
	} else {
		database.DB.Table("sleeper_drafts").Where("status = ?", "complete").Count(&total)
	}
	db.Order("d.season DESC, d.created_at DESC").Limit(limit).Offset(offset).Scan(&rows)

	items := make([]SleeperDraftItem, len(rows))
	for i, r := range rows {
		items[i] = SleeperDraftItem{
			ID:         r.SleeperDraftID,
			LeagueID:   r.SleeperLeagueID,
			LeagueName: r.LeagueName,
			Type:       r.Type,
			Status:     r.Status,
			Season:     r.Season,
			PickCount:  r.PickCount,
		}
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	c.JSON(http.StatusOK, SleeperDraftsResponse{
		Drafts:     items,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}
```

Delete this block entirely (no replacement).

- [ ] **Step 3: Run the full backend test suite**

Run: `cd backend && go build ./... && go test ./...`
Expected: build succeeds, all tests PASS (confirms no other code referenced `GetSleeperDrafts`/`SleeperDraftItem`/`SleeperDraftsResponse`, matching the repo-wide search done during planning).

- [ ] **Step 4: Commit**

```bash
git add backend/internal/api/handlers/sleeper.go backend/internal/api/routes.go
git commit -m "chore(sleeper): remove unused drafts-list endpoint"
```
