# Sleeper Weekly NFL Player Stats Scraper Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Sleeper weekly NFL player stats scraper (client → migration → Temporal activity → workflow → schedule) so weekly fantasy points for all NFL players land in `sleeper_player_week_stats`, unblocking the player-valuation model.

**Architecture:** Follows the existing `backend/internal/sleeper` + `backend/internal/activities` + `backend/internal/workflows` + `backend/schedules` pattern used by drafts/transactions sync. A single `WeekStatsActivities` struct wraps DB + Sleeper client. `SyncWeekStats(season)` loops weeks 1–18 in-workflow, skipping weeks already `finalized`. A zero-arg `WeekStatsSyncDispatcher` resolves the current season via Sleeper's NFL state and delegates to `SyncWeekStats`, matching the other `...Dispatcher` schedule entry points. `SyncWeekStats` itself stays directly invocable (via `temporal workflow start`) for the 2025 backfill.

**Tech Stack:** Go, GORM (Postgres in prod, sqlite in-memory for tests), Temporal Go SDK, goose migrations.

## Global Constraints

- Migration file MUST be numbered `013_sleeper_week_stats.sql` exactly — `014` is reserved by `feat/player-valuation`.
- `sleeper_week_stat_fetches` upserts must overwrite (`DO UPDATE`), not `DoNothing` — in-season refetches change `pts_*`/`stats`/`finalized`.
- Filter weekly stats to fantasy positions only: QB, RB, WR, TE, K, DEF — join against `sleeper_players.position`; players missing from `sleeper_players` are skipped.
- `finalized` = `season < current_season` OR (`season == current_season` AND `week < current_week`), per Sleeper's `/v1/state/nfl`.
- Follow existing conventions exactly: `backend/internal/sleeper/client.go`'s `get()` helper (base URL injection, `NotFoundError`, retry/backoff) and `backend/internal/activities/data_fetch.go`'s upsert style (`clause.OnConflict` + `clause.AssignmentColumns`).
- All new Go code must pass `gofmt` and `go test ./...` from `backend/`.

---

## File Structure

- **Modify** `backend/internal/sleeper/types.go` — add `NFLState` type.
- **Modify** `backend/internal/sleeper/client.go` — add `GetWeekStats`, `GetNFLState`.
- **Modify** `backend/internal/sleeper/client_test.go` — tests for the two new client methods.
- **Create** `backend/migrations/013_sleeper_week_stats.sql` — the two tables (exact SQL given below).
- **Modify** `backend/internal/models/sleeper.go` — add `SleeperPlayerWeekStat`, `SleeperWeekStatFetch` GORM models.
- **Create** `backend/internal/activities/week_stats.go` — `WeekStatsActivities` with `FetchWeekStats`, `GetFinalizedWeeks`, `GetCurrentSeason`.
- **Modify** `backend/internal/activities/params.go` — add `FetchWeekStatsParams`, `GetFinalizedWeeksParams`.
- **Create** `backend/internal/activities/week_stats_test.go` — activity tests.
- **Modify** `backend/internal/activities/discovery_test.go` — add the two new models to `newTestDB`'s `AutoMigrate` call so `week_stats_test.go` can use it.
- **Create** `backend/internal/workflows/week_stats_sync.go` — `SyncWeekStats`, `WeekStatsSyncDispatcher`.
- **Modify** `backend/internal/workflows/helpers.go` — add `TaskQueueWeekStats` constant.
- **Modify** `backend/internal/workflows/workflows_test.go` — append workflow tests.
- **Modify** `backend/schedules/register.go` — register `WeekStatsSyncDispatcher` on a schedule.
- **Modify** `backend/cmd/worker/main.go` — build `WeekStatsActivities`, register a new worker on `TaskQueueWeekStats`.

---

## Task 1: Sleeper client — `GetWeekStats` and `GetNFLState`

**Files:**
- Modify: `backend/internal/sleeper/types.go`
- Modify: `backend/internal/sleeper/client.go`
- Test: `backend/internal/sleeper/client_test.go`

**Interfaces:**
- Produces: `sleeper.NFLState{Season, SeasonType string; Week int}`; `(*Client) GetWeekStats(ctx, season string, week int) (map[string]json.RawMessage, error)`; `(*Client) GetNFLState(ctx) (*NFLState, error)`. Both use the existing `NotFoundError` on 404, same as every other client method.

- [ ] **Step 1: Write the failing tests**

Append to `backend/internal/sleeper/client_test.go` (needs `"errors"` already imported):

```go
func TestGetWeekStats_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/stats/nfl/regular/2025/3" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"421":{"pts_ppr":24.06,"pts_half_ppr":20.56,"pts_std":17.06,"rec":5},"999":{"pts_ppr":0}}`))
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	stats, err := c.GetWeekStats(context.Background(), "2025", 3)
	if err != nil {
		t.Fatalf("GetWeekStats error: %v", err)
	}
	if len(stats) != 2 {
		t.Errorf("got %d players, want 2", len(stats))
	}
	var p421 struct {
		PtsPPR float64 `json:"pts_ppr"`
	}
	if err := json.Unmarshal(stats["421"], &p421); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p421.PtsPPR != 24.06 {
		t.Errorf("got PtsPPR %v, want 24.06", p421.PtsPPR)
	}
}

func TestGetWeekStats_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	_, err := c.GetWeekStats(context.Background(), "2025", 25)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	var nfe *sleeper.NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}

func TestGetNFLState_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/state/nfl" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(sleeper.NFLState{Season: "2025", SeasonType: "regular", Week: 5})
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	state, err := c.GetNFLState(context.Background())
	if err != nil {
		t.Fatalf("GetNFLState error: %v", err)
	}
	if state.Season != "2025" || state.Week != 5 {
		t.Errorf("got %+v, want season=2025 week=5", state)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (compile error — methods don't exist yet)**

Run: `cd backend && go test ./internal/sleeper/... -run 'TestGetWeekStats|TestGetNFLState' -v`
Expected: FAIL — `c.GetWeekStats undefined` / `c.GetNFLState undefined` / `sleeper.NFLState undefined`

- [ ] **Step 3: Add `NFLState` type**

In `backend/internal/sleeper/types.go`, after the `Player` struct and before `NotFoundError`:

```go
// NFLState is returned by GET /v1/state/nfl and describes the current NFL week/season,
// used to decide whether a fetched week's stats are final.
type NFLState struct {
	Season     string `json:"season"`
	SeasonType string `json:"season_type"`
	Week       int    `json:"week"`
}
```

- [ ] **Step 4: Add `GetWeekStats` and `GetNFLState` client methods**

In `backend/internal/sleeper/client.go`, after `GetAllPlayers`:

```go
// GetWeekStats fetches per-player weekly stats for season/week. The map key is the
// sleeper_player_id; each value is the raw stat object (includes pts_ppr, pts_half_ppr,
// pts_std among many other fields, decoded further by callers).
func (c *Client) GetWeekStats(ctx context.Context, season string, week int) (map[string]json.RawMessage, error) {
	var stats map[string]json.RawMessage
	path := fmt.Sprintf("/v1/stats/nfl/regular/%s/%d", season, week)
	if err := c.get(ctx, path, &stats); err != nil {
		return nil, err
	}
	return stats, nil
}

// GetNFLState fetches the current NFL season/week/season_type.
func (c *Client) GetNFLState(ctx context.Context) (*NFLState, error) {
	var s NFLState
	if err := c.get(ctx, "/v1/state/nfl", &s); err != nil {
		return nil, err
	}
	return &s, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd backend && go test ./internal/sleeper/... -v`
Expected: PASS (all tests, including pre-existing ones)

- [ ] **Step 6: Commit**

```bash
git add backend/internal/sleeper/types.go backend/internal/sleeper/client.go backend/internal/sleeper/client_test.go
git commit -m "feat: add Sleeper GetWeekStats and GetNFLState client methods"
```

---

## Task 2: Migration + GORM models

**Files:**
- Create: `backend/migrations/013_sleeper_week_stats.sql`
- Modify: `backend/internal/models/sleeper.go`

**Interfaces:**
- Produces: `models.SleeperPlayerWeekStat{Season, SleeperPlayerID string; Week int; PtsPPR, PtsHalfPPR, PtsStd *float64; Stats json.RawMessage; CreatedAt, UpdatedAt time.Time}` (table `sleeper_player_week_stats`, PK `season,week,sleeper_player_id`); `models.SleeperWeekStatFetch{Season string; Week int; LastFetchedAt *time.Time; Finalized bool}` (table `sleeper_week_stat_fetches`, PK `season,week`).

- [ ] **Step 1: Create the migration**

Create `backend/migrations/013_sleeper_week_stats.sql`:

```sql
-- +goose Up

CREATE TABLE sleeper_player_week_stats (
    season             TEXT NOT NULL,
    week               INT  NOT NULL,
    sleeper_player_id  TEXT NOT NULL,
    pts_ppr            FLOAT,
    pts_half_ppr       FLOAT,
    pts_std            FLOAT,
    stats              JSONB,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (season, week, sleeper_player_id)
);

CREATE TABLE sleeper_week_stat_fetches (
    season           TEXT NOT NULL,
    week             INT  NOT NULL,
    last_fetched_at  TIMESTAMPTZ,
    finalized        BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (season, week)
);

-- +goose Down

DROP TABLE IF EXISTS sleeper_week_stat_fetches;
DROP TABLE IF EXISTS sleeper_player_week_stats;
```

- [ ] **Step 2: Add GORM models**

In `backend/internal/models/sleeper.go`, append after `SleeperTransaction`:

```go
type SleeperPlayerWeekStat struct {
	Season          string          `gorm:"primaryKey;column:season"`
	Week            int             `gorm:"primaryKey;column:week"`
	SleeperPlayerID string          `gorm:"primaryKey;column:sleeper_player_id"`
	PtsPPR          *float64        `gorm:"column:pts_ppr"`
	PtsHalfPPR      *float64        `gorm:"column:pts_half_ppr"`
	PtsStd          *float64        `gorm:"column:pts_std"`
	Stats           json.RawMessage `gorm:"column:stats;type:jsonb"`
	CreatedAt       time.Time       `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time       `gorm:"column:updated_at;autoUpdateTime"`
}

func (SleeperPlayerWeekStat) TableName() string { return "sleeper_player_week_stats" }

type SleeperWeekStatFetch struct {
	Season        string     `gorm:"primaryKey;column:season"`
	Week          int        `gorm:"primaryKey;column:week"`
	LastFetchedAt *time.Time `gorm:"column:last_fetched_at"`
	Finalized     bool       `gorm:"column:finalized"`
}

func (SleeperWeekStatFetch) TableName() string { return "sleeper_week_stat_fetches" }
```

- [ ] **Step 3: Verify it builds and goose parses the migration**

Run: `cd backend && go build ./... && go run ./cmd/migrate status`
Expected: build succeeds; `migrate status` either connects and lists `013_sleeper_week_stats.sql` as pending, or fails only on DB connectivity (no `DATABASE_URL`) — NOT on SQL/goose parse errors. If there's a reachable dev DB, confirm the migration is listed.

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/013_sleeper_week_stats.sql backend/internal/models/sleeper.go
git commit -m "feat: add sleeper_player_week_stats and sleeper_week_stat_fetches tables"
```

---

## Task 3: `FetchWeekStats`, `GetFinalizedWeeks`, `GetCurrentSeason` activities

**Files:**
- Create: `backend/internal/activities/week_stats.go`
- Modify: `backend/internal/activities/params.go`
- Modify: `backend/internal/activities/discovery_test.go` (extend `newTestDB`)
- Test: `backend/internal/activities/week_stats_test.go`

**Consumes:** `sleeper.Client.GetWeekStats`, `sleeper.Client.GetNFLState` (Task 1); `models.SleeperPlayer`, `models.SleeperPlayerWeekStat`, `models.SleeperWeekStatFetch` (Task 2).

**Produces:** `activities.WeekStatsActivities{DB *gorm.DB; Sleeper *sleeper.Client}` with methods `FetchWeekStats(ctx, FetchWeekStatsParams{Season string; Week int}) error`, `GetFinalizedWeeks(ctx, GetFinalizedWeeksParams{Season string}) ([]int, error)`, `GetCurrentSeason(ctx) (string, error)` — consumed by Task 4's workflow.

- [ ] **Step 1: Extend `newTestDB` to migrate the new tables**

In `backend/internal/activities/discovery_test.go`, in `newTestDB`'s `AutoMigrate` call, add the two new models:

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
		t.Fatalf("automigrate: %v", err)
	}
```

- [ ] **Step 2: Add params structs**

In `backend/internal/activities/params.go`, append:

```go
type FetchWeekStatsParams struct {
	Season string
	Week   int
}

type GetFinalizedWeeksParams struct {
	Season string
}
```

- [ ] **Step 3: Write the failing tests**

Create `backend/internal/activities/week_stats_test.go`:

```go
package activities_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/sleeper"
)

func weekStatsServer(t *testing.T, statsBody string, nflWeek int, nflSeason string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/state/nfl":
			json.NewEncoder(w).Encode(sleeper.NFLState{Season: nflSeason, SeasonType: "regular", Week: nflWeek})
		default:
			if statsBody == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Write([]byte(statsBody))
		}
	}))
}

func TestFetchWeekStats_FiltersToFantasyPositionsAndUpserts(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperPlayer{SleeperPlayerID: "421", Position: "RB"})
	db.Create(&models.SleeperPlayer{SleeperPlayerID: "999", Position: "DL"}) // not fantasy-relevant
	// "555" is absent from sleeper_players entirely — must be skipped too.

	body := `{"421":{"pts_ppr":24.06,"pts_half_ppr":20.56,"pts_std":17.06},"999":{"pts_ppr":5},"555":{"pts_ppr":3}}`
	srv := weekStatsServer(t, body, 10, "2025")
	defer srv.Close()

	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 3})
	if err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}

	var rows []models.SleeperPlayerWeekStat
	db.Find(&rows)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (only fantasy position kept), got %d: %+v", len(rows), rows)
	}
	if rows[0].SleeperPlayerID != "421" || rows[0].PtsPPR == nil || *rows[0].PtsPPR != 24.06 {
		t.Errorf("unexpected row: %+v", rows[0])
	}
}

func TestFetchWeekStats_RefetchOverwrites(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperPlayer{SleeperPlayerID: "421", Position: "RB"})

	srv1 := weekStatsServer(t, `{"421":{"pts_ppr":10}}`, 10, "2025")
	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv1.URL)}
	if err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 3}); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	srv1.Close()

	srv2 := weekStatsServer(t, `{"421":{"pts_ppr":15.5}}`, 10, "2025")
	defer srv2.Close()
	wsa2 := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv2.URL)}
	if err := wsa2.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 3}); err != nil {
		t.Fatalf("second fetch: %v", err)
	}

	var row models.SleeperPlayerWeekStat
	db.First(&row)
	if row.PtsPPR == nil || *row.PtsPPR != 15.5 {
		t.Errorf("expected overwritten PtsPPR 15.5, got %+v", row.PtsPPR)
	}
	var count int64
	db.Model(&models.SleeperPlayerWeekStat{}).Count(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 row after refetch, got %d", count)
	}
}

func TestFetchWeekStats_MarksFinalized_PastWeek(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperPlayer{SleeperPlayerID: "421", Position: "RB"})
	srv := weekStatsServer(t, `{"421":{"pts_ppr":10}}`, 10, "2025") // current week is 10
	defer srv.Close()

	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 3}); err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}

	var fetch models.SleeperWeekStatFetch
	db.First(&fetch)
	if !fetch.Finalized {
		t.Errorf("expected week 3 finalized (current week is 10), got %+v", fetch)
	}
}

func TestFetchWeekStats_NotFinalized_CurrentWeek(t *testing.T) {
	db := newTestDB(t)
	srv := weekStatsServer(t, `{}`, 10, "2025") // current week is 10, fetching week 10
	defer srv.Close()

	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 10}); err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}

	var fetch models.SleeperWeekStatFetch
	db.First(&fetch)
	if fetch.Finalized {
		t.Errorf("expected current week not finalized, got %+v", fetch)
	}
}

func TestFetchWeekStats_PastSeasonAlwaysFinalized(t *testing.T) {
	db := newTestDB(t)
	srv := weekStatsServer(t, `{}`, 3, "2026") // NFL is now in season 2026
	defer srv.Close()

	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 18}); err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}

	var fetch models.SleeperWeekStatFetch
	db.First(&fetch)
	if !fetch.Finalized {
		t.Errorf("expected past-season week finalized, got %+v", fetch)
	}
}

func TestFetchWeekStats_EmptyWeek404_NoRowsButFetchStamped(t *testing.T) {
	db := newTestDB(t)
	srv := weekStatsServer(t, "", 10, "2025") // stats endpoint 404s
	defer srv.Close()

	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 20}); err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}

	var statCount int64
	db.Model(&models.SleeperPlayerWeekStat{}).Count(&statCount)
	if statCount != 0 {
		t.Errorf("expected no stat rows for 404 week, got %d", statCount)
	}
	var fetchCount int64
	db.Model(&models.SleeperWeekStatFetch{}).Count(&fetchCount)
	if fetchCount != 1 {
		t.Errorf("expected fetch row still stamped for 404 week, got %d", fetchCount)
	}
}

func TestGetFinalizedWeeks_ReturnsOnlyFinalized(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperWeekStatFetch{Season: "2025", Week: 1, Finalized: true})
	db.Create(&models.SleeperWeekStatFetch{Season: "2025", Week: 2, Finalized: true})
	db.Create(&models.SleeperWeekStatFetch{Season: "2025", Week: 3, Finalized: false})
	db.Create(&models.SleeperWeekStatFetch{Season: "2024", Week: 1, Finalized: true}) // different season

	wsa := &activities.WeekStatsActivities{DB: db}
	weeks, err := wsa.GetFinalizedWeeks(context.Background(), activities.GetFinalizedWeeksParams{Season: "2025"})
	if err != nil {
		t.Fatalf("GetFinalizedWeeks error: %v", err)
	}
	if len(weeks) != 2 {
		t.Fatalf("expected 2 finalized weeks, got %v", weeks)
	}
}

func TestGetCurrentSeason_ReturnsSleeperState(t *testing.T) {
	srv := weekStatsServer(t, "", 5, "2025")
	defer srv.Close()

	wsa := &activities.WeekStatsActivities{Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	season, err := wsa.GetCurrentSeason(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentSeason error: %v", err)
	}
	if season != "2025" {
		t.Errorf("got season %q, want 2025", season)
	}
}
```

- [ ] **Step 4: Run tests to verify they fail (compile error — activities file doesn't exist yet)**

Run: `cd backend && go test ./internal/activities/... -run WeekStats -v`
Expected: FAIL — `activities.WeekStatsActivities undefined` (and similar)

- [ ] **Step 5: Implement the activities**

Create `backend/internal/activities/week_stats.go`:

```go
package activities

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/models"
	"backend/internal/sleeper"
)

// WeekStatsActivities holds dependencies for the weekly NFL player stats scraper.
type WeekStatsActivities struct {
	DB      *gorm.DB
	Sleeper *sleeper.Client
}

// fantasyPositions are the positions kept from Sleeper's weekly stats response;
// everything else (IDP, etc.) is filtered out.
var fantasyPositions = []string{"QB", "RB", "WR", "TE", "K", "DEF"}

type weekStatPoints struct {
	PtsPPR     *float64 `json:"pts_ppr"`
	PtsHalfPPR *float64 `json:"pts_half_ppr"`
	PtsStd     *float64 `json:"pts_std"`
}

// FetchWeekStats fetches one week of Sleeper stats, filters to fantasy-relevant
// positions, upserts sleeper_player_week_stats (overwriting on refetch so in-season
// corrections land), and stamps sleeper_week_stat_fetches — including whether the
// week is finalized per Sleeper's current NFL state.
func (a *WeekStatsActivities) FetchWeekStats(ctx context.Context, params FetchWeekStatsParams) error {
	raw, err := a.Sleeper.GetWeekStats(ctx, params.Season, params.Week)
	if err != nil {
		var nfe *sleeper.NotFoundError
		if !errors.As(err, &nfe) {
			return err
		}
		raw = nil // no stats published for this week yet
	}

	if len(raw) > 0 {
		var players []models.SleeperPlayer
		if err := a.DB.WithContext(ctx).
			Where("position IN ?", fantasyPositions).
			Find(&players).Error; err != nil {
			return err
		}
		fantasyIDs := make(map[string]struct{}, len(players))
		for _, p := range players {
			fantasyIDs[p.SleeperPlayerID] = struct{}{}
		}

		for id, statBytes := range raw {
			if _, ok := fantasyIDs[id]; !ok {
				continue
			}
			var pts weekStatPoints
			if err := json.Unmarshal(statBytes, &pts); err != nil {
				return err
			}
			row := models.SleeperPlayerWeekStat{
				Season:          params.Season,
				Week:            params.Week,
				SleeperPlayerID: id,
				PtsPPR:          pts.PtsPPR,
				PtsHalfPPR:      pts.PtsHalfPPR,
				PtsStd:          pts.PtsStd,
				Stats:           json.RawMessage(statBytes),
			}
			if err := a.DB.WithContext(ctx).Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "season"}, {Name: "week"}, {Name: "sleeper_player_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"pts_ppr", "pts_half_ppr", "pts_std", "stats", "updated_at",
				}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
	}

	state, err := a.Sleeper.GetNFLState(ctx)
	if err != nil {
		return err
	}
	finalized := params.Season < state.Season || (params.Season == state.Season && params.Week < state.Week)

	now := time.Now().UTC()
	fetchRow := models.SleeperWeekStatFetch{
		Season:        params.Season,
		Week:          params.Week,
		LastFetchedAt: &now,
		Finalized:     finalized,
	}
	return a.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "season"}, {Name: "week"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_fetched_at", "finalized"}),
	}).Create(&fetchRow).Error
}

// GetFinalizedWeeks returns the weeks already marked finalized for season, so
// SyncWeekStats can skip re-fetching them.
func (a *WeekStatsActivities) GetFinalizedWeeks(ctx context.Context, params GetFinalizedWeeksParams) ([]int, error) {
	var rows []models.SleeperWeekStatFetch
	if err := a.DB.WithContext(ctx).
		Where("season = ? AND finalized = ?", params.Season, true).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	weeks := make([]int, len(rows))
	for i, r := range rows {
		weeks[i] = r.Week
	}
	return weeks, nil
}

// GetCurrentSeason returns the current NFL season per Sleeper's state endpoint,
// used by the schedule dispatcher to sync the in-progress season automatically.
func (a *WeekStatsActivities) GetCurrentSeason(ctx context.Context) (string, error) {
	state, err := a.Sleeper.GetNFLState(ctx)
	if err != nil {
		return "", err
	}
	return state.Season, nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd backend && go test ./internal/activities/... -v`
Expected: PASS (all tests, including pre-existing ones)

- [ ] **Step 7: Commit**

```bash
git add backend/internal/activities/week_stats.go backend/internal/activities/params.go \
        backend/internal/activities/week_stats_test.go backend/internal/activities/discovery_test.go
git commit -m "feat: add FetchWeekStats, GetFinalizedWeeks, GetCurrentSeason activities"
```

---

## Task 4: `SyncWeekStats` workflow + `WeekStatsSyncDispatcher`

**Files:**
- Modify: `backend/internal/workflows/helpers.go`
- Create: `backend/internal/workflows/week_stats_sync.go`
- Modify: `backend/internal/workflows/workflows_test.go`

**Consumes:** `activities.WeekStatsActivities{}` methods `GetFinalizedWeeks`, `FetchWeekStats`, `GetCurrentSeason` (Task 3); `defaultActivityOptions` (existing, `helpers.go`).

**Produces:** `workflows.SyncWeekStats(ctx workflow.Context, season string) error` (directly invocable via `temporal workflow start --type SyncWeekStats` for backfills); `workflows.WeekStatsSyncDispatcher(ctx workflow.Context) error` (zero-arg, for the schedule — resolves current season then calls `SyncWeekStats`); `workflows.TaskQueueWeekStats` constant (Task 5 registers workers/schedules on this).

- [ ] **Step 1: Add the task queue constant**

In `backend/internal/workflows/helpers.go`, add to the `const` block:

```go
	TaskQueueWeekStats    = "sleeper-week-stats"
```

(Place it alongside the other `TaskQueue*` constants.)

- [ ] **Step 2: Write the failing tests**

Append to `backend/internal/workflows/workflows_test.go`:

```go
// ---- SyncWeekStats ----

func TestSyncWeekStats_SkipsFinalizedWeeks(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	wsa := &activities.WeekStatsActivities{}
	// Weeks 1 and 2 already finalized — only weeks 3-18 should be fetched.
	env.OnActivity(wsa.GetFinalizedWeeks, mock.Anything, activities.GetFinalizedWeeksParams{Season: "2025"}).
		Return([]int{1, 2}, nil)
	for week := 3; week <= 18; week++ {
		env.OnActivity(wsa.FetchWeekStats, mock.Anything, activities.FetchWeekStatsParams{Season: "2025", Week: week}).Return(nil)
	}

	env.ExecuteWorkflow(workflows.SyncWeekStats, "2025")

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestSyncWeekStats_AllWeeksFinalized_NoFetchCalls(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	allWeeks := make([]int, 0, 18)
	for w := 1; w <= 18; w++ {
		allWeeks = append(allWeeks, w)
	}

	wsa := &activities.WeekStatsActivities{}
	env.OnActivity(wsa.GetFinalizedWeeks, mock.Anything, activities.GetFinalizedWeeksParams{Season: "2025"}).
		Return(allWeeks, nil)

	env.ExecuteWorkflow(workflows.SyncWeekStats, "2025")

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

// ---- WeekStatsSyncDispatcher ----

func TestWeekStatsSyncDispatcher_ResolvesSeasonAndSyncs(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	wsa := &activities.WeekStatsActivities{}
	env.OnActivity(wsa.GetCurrentSeason, mock.Anything).Return("2025", nil)
	env.OnActivity(wsa.GetFinalizedWeeks, mock.Anything, activities.GetFinalizedWeeksParams{Season: "2025"}).
		Return([]int{}, nil)
	for week := 1; week <= 18; week++ {
		env.OnActivity(wsa.FetchWeekStats, mock.Anything, activities.FetchWeekStatsParams{Season: "2025", Week: week}).Return(nil)
	}

	env.ExecuteWorkflow(workflows.WeekStatsSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}
```

- [ ] **Step 3: Run tests to verify they fail (compile error — workflow doesn't exist yet)**

Run: `cd backend && go test ./internal/workflows/... -run 'SyncWeekStats|WeekStatsSyncDispatcher' -v`
Expected: FAIL — `workflows.SyncWeekStats undefined` / `workflows.WeekStatsSyncDispatcher undefined`

- [ ] **Step 4: Implement the workflows**

Create `backend/internal/workflows/week_stats_sync.go`:

```go
package workflows

import (
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// lastFantasyWeek is the last fantasy-relevant regular season week.
const lastFantasyWeek = 18

// SyncWeekStats fetches weekly Sleeper stats for every week 1-18 of season that
// isn't already finalized. Directly invocable (e.g. via `temporal workflow start
// --type SyncWeekStats --input '"2025"'`) for backfills, and delegated to by
// WeekStatsSyncDispatcher for the in-season schedule.
func SyncWeekStats(ctx workflow.Context, season string) error {
	wsa := &activities.WeekStatsActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var finalizedWeeks []int
	if err := workflow.ExecuteActivity(actCtx, wsa.GetFinalizedWeeks, activities.GetFinalizedWeeksParams{Season: season}).Get(ctx, &finalizedWeeks); err != nil {
		return err
	}
	finalized := make(map[int]bool, len(finalizedWeeks))
	for _, w := range finalizedWeeks {
		finalized[w] = true
	}

	for week := 1; week <= lastFantasyWeek; week++ {
		if finalized[week] {
			continue
		}
		if err := workflow.ExecuteActivity(actCtx, wsa.FetchWeekStats, activities.FetchWeekStatsParams{Season: season, Week: week}).Get(ctx, nil); err != nil {
			return err
		}
	}
	return nil
}

// WeekStatsSyncDispatcher is the scheduled entry point: it resolves the current NFL
// season via Sleeper's state endpoint, then runs SyncWeekStats for it.
func WeekStatsSyncDispatcher(ctx workflow.Context) error {
	wsa := &activities.WeekStatsActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var season string
	if err := workflow.ExecuteActivity(actCtx, wsa.GetCurrentSeason).Get(ctx, &season); err != nil {
		return err
	}
	return SyncWeekStats(ctx, season)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd backend && go test ./internal/workflows/... -v`
Expected: PASS (all tests, including pre-existing ones)

- [ ] **Step 6: Commit**

```bash
git add backend/internal/workflows/helpers.go backend/internal/workflows/week_stats_sync.go \
        backend/internal/workflows/workflows_test.go
git commit -m "feat: add SyncWeekStats workflow and WeekStatsSyncDispatcher"
```

---

## Task 5: Schedule registration + worker wiring

**Files:**
- Modify: `backend/schedules/register.go`
- Modify: `backend/cmd/worker/main.go`

**Consumes:** `workflows.WeekStatsSyncDispatcher`, `workflows.SyncWeekStats`, `workflows.TaskQueueWeekStats` (Task 4); `activities.WeekStatsActivities{DB, Sleeper}` (Task 3).

- [ ] **Step 1: Register the schedule**

In `backend/schedules/register.go`, replace the final `return upsert(...)` for the player-sync schedule so it's no longer the last statement, then add the week-stats schedule as the new final return:

```go
	if err := upsert(ctx, c, client.ScheduleOptions{
		ID: "sleeper-player-sync-schedule",
		Spec: client.ScheduleSpec{
			Calendars: []client.ScheduleCalendarSpec{
				{
					DayOfWeek: []client.ScheduleRange{{Start: 2}}, // Tuesday
					Hour:      []client.ScheduleRange{{Start: 8}}, // 03:00 EST (UTC-5)
					Minute:    []client.ScheduleRange{{Start: 0}},
				},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			Workflow:  workflows.PlayerDatabaseSyncWorkflow,
			TaskQueue: workflows.TaskQueuePlayerSync,
		},
	}); err != nil {
		return err
	}

	return upsert(ctx, c, client.ScheduleOptions{
		ID: "sleeper-week-stats-schedule",
		Spec: client.ScheduleSpec{
			Intervals: []client.ScheduleIntervalSpec{
				{Every: 30 * time.Minute},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			Workflow:  workflows.WeekStatsSyncDispatcher,
			TaskQueue: workflows.TaskQueueWeekStats,
		},
	})
```

(This just changes the previous single `return upsert(...)` into `if err := upsert(...); err != nil { return err }` followed by a new final `return upsert(...)` block — the player-sync schedule body itself is unchanged.)

- [ ] **Step 2: Register the worker**

In `backend/cmd/worker/main.go`:

Add to the activities construction block (after `psa := &activities.PlayerSyncActivities{...}`):

```go
	wsa := &activities.WeekStatsActivities{DB: database.DB, Sleeper: sc}
```

Add a new worker block (after the player-sync worker block, before the `workers := []worker.Worker{...}` line):

```go
	// Week stats worker: WeekStatsSyncDispatcher + SyncWeekStats
	wsw := worker.New(c, workflows.TaskQueueWeekStats, worker.Options{})
	wsw.RegisterWorkflow(workflows.WeekStatsSyncDispatcher)
	wsw.RegisterWorkflow(workflows.SyncWeekStats)
	wsw.RegisterActivity(wsa)
```

Update the `workers := []worker.Worker{...}` line to include it:

```go
	workers := []worker.Worker{dw, draftsw, transactionsw, psw, wsw}
```

- [ ] **Step 3: Verify it builds**

Run: `cd backend && go build ./...`
Expected: success, no errors

- [ ] **Step 4: Run the full test suite**

Run: `cd backend && go test ./... `
Expected: PASS across all packages

- [ ] **Step 5: Commit**

```bash
git add backend/schedules/register.go backend/cmd/worker/main.go
git commit -m "feat: register sleeper-week-stats schedule and worker"
```

---

## Task 6: 2025 backfill

**Files:** none (operational step + PR documentation only).

- [ ] **Step 1: Document and (if a reachable Temporal + Postgres dev/prod environment is available) run the backfill**

One-liner using the Temporal CLI, targeting the worker's task queue so a running `cmd/worker` process picks it up:

```bash
temporal workflow start \
  --task-queue sleeper-week-stats \
  --type SyncWeekStats \
  --workflow-id sleeper-week-stats-backfill-2025 \
  --input '"2025"'
```

This requires: `cmd/worker` running against the target Temporal namespace (so `SyncWeekStats`/`WeekStatsSyncDispatcher` are registered on task queue `sleeper-week-stats`), and `sleeper_players` already populated (existing player-sync schedule) so position filtering has data to join against.

- [ ] **Step 2: Verify backfill results**

```sql
SELECT count(*) FROM sleeper_player_week_stats WHERE season = '2025';
SELECT week, finalized FROM sleeper_week_stat_fetches WHERE season = '2025' ORDER BY week;
```

Expected: rows for season 2025 across weeks 1–18, and all 18 `sleeper_week_stat_fetches` rows for season 2025 have `finalized = true` (2025 is a past season by the time this runs in 2026).

- [ ] **Step 3: Note the outcome in the PR description**

If actually executed: paste the row counts. If not executed (no reachable environment from the dev sandbox): note the one-liner above as the documented trigger, per the issue's acceptance criteria ("2025 backfill executed (or documented one-liner to trigger it)").

---

## Self-Review Notes

- **Spec coverage:** client methods (Task 1) ✓; migration `013` exact SQL + models (Task 2) ✓; activity upsert/filter/finalize (Task 3) ✓; workflow skip-finalized + schedule registration (Task 4, 5) ✓; tests for upsert-overwrite, position filtering, finalized marking (past/current week), 404/empty handling (Task 3 tests) ✓; 2025 backfill or documented one-liner (Task 6) ✓.
- **Migration number:** confirmed `013`, not `014` (reserved by `feat/player-valuation`).
- **Type consistency:** `FetchWeekStatsParams{Season string; Week int}` and `GetFinalizedWeeksParams{Season string}` used identically across Task 3 (definition), Task 3 tests, and Task 4 (workflow call sites). `WeekStatsActivities` field names (`DB`, `Sleeper`) match the `DataFetchActivities`/`PlayerSyncActivities` convention used in `cmd/worker/main.go`.
