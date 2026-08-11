# Trade Valuation Totals Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist a per-side valuation total on every `ppr-sf-10` trade at sync time (instead of computing it live on every `/trades` request), backfilling existing trades via the same mechanism, with zero added latency to trade ingestion.

**Architecture:** A new `backend/internal/valuation` package holds pure segment/valuation-lookup logic (moved out of the trades handler). `transactioncron` calls it twice per 10-minute tick: once at insert time for newly-synced trades, and once via a bounded, timed-out `ReconcileTradeValues` sweep that runs concurrently with (not after) the existing fetch → flush pipeline and doubles as the backfill for pre-existing null rows. `GetSleeperTrades` drops its live computation and just reads the persisted `sleeper_transactions.trade_values` column.

**Tech Stack:** Go (Gin, GORM, PostgreSQL/SQLite for tests), goose migrations, systemd timers, Next.js/TypeScript frontend (read-only type change).

## Global Constraints

- A side's `TotalValue` is populated only when **every** player on that side has a valuation within 24h of the trade's timestamp (`valuation.FreshnessWindow`) — never a partial sum.
- Draft picks never count toward or block a side's valuation (no pick-valuation model exists).
- No new systemd unit for the linking work — it lives inside the existing `ff-sims-transactions.service` (`transactioncron`).
- `ReconcileTradeValues` must never add blocking latency to trade ingestion: it runs concurrently with fetch → flush, under its own 5s timeout, capped at 200 rows per tick.
- `trade_values` is cloud-only (`sleeper_transactions`); the archive DB copy does not get this column.
- Design spec: `docs/superpowers/specs/2026-08-10-trade-valuation-totals-design.md`.

---

### Task 1: Schema — `trade_values` column

**Files:**
- Create: `backend/migrations/032_trade_values.sql`
- Modify: `backend/internal/models/sleeper.go:98-112` (`SleeperTransaction`)

**Interfaces:**
- Produces: `models.SleeperTransaction.TradeValues json.RawMessage` (column `trade_values`), consumed by Tasks 3-7.

- [ ] **Step 1: Write the migration**

Create `backend/migrations/032_trade_values.sql`:

```sql
-- +goose Up
-- +goose NO TRANSACTION

-- Persists each trade's per-side valuation total, computed and written by
-- transactioncron (internal/valuation.ComputeTradeValues) at sync time and
-- backfilled by its reconcile sweep — see
-- docs/superpowers/specs/2026-08-10-trade-valuation-totals-design.md.
-- Shape: {"<roster_id>": <float>, ...}, present only for roster IDs where
-- every player on that side has a fresh valuation. Cloud-only; the archive
-- DB copy of sleeper_transactions does not get this column.
ALTER TABLE sleeper_transactions ADD COLUMN IF NOT EXISTS trade_values JSONB;

-- Keeps the reconcile sweep's `WHERE trade_values IS NULL` cheap regardless
-- of table size — mirrors idx_sleeper_transactions_trade_complete's
-- partial-index pattern for the same table.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_transactions_trade_values_null
  ON sleeper_transactions (created_at_sleeper)
  WHERE type = 'trade' AND status = 'complete' AND trade_values IS NULL;

-- +goose Down
-- +goose NO TRANSACTION

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_transactions_trade_values_null;
ALTER TABLE sleeper_transactions DROP COLUMN IF EXISTS trade_values;
```

`CREATE INDEX CONCURRENTLY` cannot run inside goose's default transaction wrapper, hence `-- +goose NO TRANSACTION` on both Up and Down — matches the exact pattern already used in `backend/migrations/028_transaction_claimable_stale_index.sql:1-2,21-22`.

- [ ] **Step 2: Add the GORM field**

In `backend/internal/models/sleeper.go`, add `TradeValues` to `SleeperTransaction` (around line 108, alongside the other `json.RawMessage` columns):

```go
type SleeperTransaction struct {
	SleeperTransactionID string          `gorm:"primaryKey;column:sleeper_transaction_id"`
	SleeperLeagueID      string          `gorm:"column:sleeper_league_id"`
	Type                 string          `gorm:"column:type"`
	Status               string          `gorm:"column:status"`
	CreatedAtSleeper     int64           `gorm:"column:created_at_sleeper"`
	Leg                  int             `gorm:"column:leg"`
	Adds                 json.RawMessage `gorm:"column:adds;type:jsonb"`
	Drops                json.RawMessage `gorm:"column:drops;type:jsonb"`
	DraftPicks           json.RawMessage `gorm:"column:draft_picks;type:jsonb"`
	WaiverBudget         json.RawMessage `gorm:"column:waiver_budget;type:jsonb"`
	TradeValues          json.RawMessage `gorm:"column:trade_values;type:jsonb"`
	CreatedAt            time.Time       `gorm:"column:created_at;autoCreateTime"`
}
```

- [ ] **Step 3: Verify the backend still builds**

Run: `cd backend && go build ./...`
Expected: no errors (this is a pure additive struct field change).

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/032_trade_values.sql backend/internal/models/sleeper.go
git commit -m "feat(db): add sleeper_transactions.trade_values column"
```

---

### Task 2: `internal/valuation` package — segment resolution and trade valuation math

**Files:**
- Create: `backend/internal/valuation/valuation.go`
- Create: `backend/internal/valuation/valuation_test.go`

**Interfaces:**
- Produces:
  - `valuation.Snapshot{Segment, SleeperPlayerID, ValuationDate, Value}` (GORM model for `player_valuations`, read-only in Go)
  - `valuation.FreshnessWindow time.Duration` (24h)
  - `valuation.KnownSegments map[string]struct{}`
  - `valuation.SegmentKeyForLeague(ppr *float64, isSuperflex *bool, totalRosters int, leagueType string) string`
  - `valuation.LoadSnapshotHistory(db *gorm.DB, segment string, playerIDs []string, upTo time.Time) map[string][]Snapshot`
  - `valuation.ValueAsOf(snaps []Snapshot, ts time.Time, maxAge time.Duration) (float64, bool)`
  - `valuation.GroupPlayersByRoster(adds map[string]int) map[int][]string`
  - `valuation.ComputeTradeValues(rosterPlayers map[int][]string, tradeTime time.Time, history map[string][]Snapshot) (json.RawMessage, bool)`
- Consumed by: Task 3 (`transactioncron`).

- [ ] **Step 1: Write the failing tests**

Create `backend/internal/valuation/valuation_test.go`:

```go
package valuation_test

import (
	"encoding/json"
	"testing"
	"time"

	"backend/internal/valuation"
)

func TestSegmentKeyForLeague(t *testing.T) {
	ppr, half := 1.0, 0.5
	sf, oneQB := true, false

	cases := []struct {
		name       string
		ppr        *float64
		superflex  *bool
		rosters    int
		leagueType string
		want       string
	}{
		{"ppr superflex 12 redraft", &ppr, &sf, 12, "redraft", "ppr-sf-12"},
		{"ppr superflex 10 redraft", &ppr, &sf, 10, "redraft", "ppr-sf-10"},
		{"ppr superflex 8 redraft", &ppr, &sf, 8, "redraft", "ppr-sf-8"},
		{"unsupported size", &ppr, &sf, 14, "redraft", ""},
		{"half ppr", &half, &sf, 12, "redraft", ""},
		{"one qb", &ppr, &oneQB, 12, "redraft", ""},
		{"dynasty", &ppr, &sf, 12, "dynasty", ""},
		{"nil ppr", nil, &sf, 12, "redraft", ""},
		{"nil superflex", &ppr, nil, 12, "redraft", ""},
	}
	for _, c := range cases {
		if got := valuation.SegmentKeyForLeague(c.ppr, c.superflex, c.rosters, c.leagueType); got != c.want {
			t.Errorf("%s: expected %q, got %q", c.name, c.want, got)
		}
	}
}

func TestValueAsOf(t *testing.T) {
	d := func(day int) time.Time { return time.Date(2025, 9, day, 0, 0, 0, 0, time.UTC) }
	snaps := []valuation.Snapshot{
		{ValuationDate: d(8), Value: 1000},
		{ValuationDate: d(15), Value: 1200},
		{ValuationDate: d(22), Value: 900},
	}

	if _, ok := valuation.ValueAsOf(snaps, d(7), valuation.FreshnessWindow); ok {
		t.Error("expected no value before first snapshot")
	}
	if v, ok := valuation.ValueAsOf(snaps, d(15).Add(14*time.Hour), valuation.FreshnessWindow); !ok || v != 1200 {
		t.Errorf("expected 1200 between snapshots (within freshness window), got %v ok=%v", v, ok)
	}
	if v, ok := valuation.ValueAsOf(snaps, d(8), valuation.FreshnessWindow); !ok || v != 1000 {
		t.Errorf("expected same-day snapshot 1000, got %v ok=%v", v, ok)
	}
	// d(30) is 8 days after the latest snapshot d(22) — beyond the 24h
	// freshness window, so it must be treated as absent even though it's
	// the latest-known value chronologically.
	if _, ok := valuation.ValueAsOf(snaps, d(30), valuation.FreshnessWindow); ok {
		t.Error("expected no value once the latest snapshot is beyond the freshness window")
	}
	// Same snapshot set, just after it was published (a few hours later,
	// same UTC day) — within the freshness window.
	if v, ok := valuation.ValueAsOf(snaps, d(22).Add(6*time.Hour), valuation.FreshnessWindow); !ok || v != 900 {
		t.Errorf("expected 900 within the freshness window of the latest snapshot, got %v ok=%v", v, ok)
	}
	if _, ok := valuation.ValueAsOf(nil, d(30), valuation.FreshnessWindow); ok {
		t.Error("expected no value for player with no snapshots")
	}
}

func TestGroupPlayersByRoster(t *testing.T) {
	adds := map[string]int{"p1": 7, "p2": 7, "p3": 8}
	got := valuation.GroupPlayersByRoster(adds)
	if len(got[7]) != 2 || len(got[8]) != 1 {
		t.Fatalf("expected roster 7 with 2 players and roster 8 with 1, got %+v", got)
	}
	if len(got) != 2 {
		t.Errorf("expected exactly 2 rosters, got %d", len(got))
	}
}

func TestComputeTradeValues_FullCoverage(t *testing.T) {
	now := time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)
	rosterPlayers := map[int][]string{
		7: {"p1", "p2"},
		8: {"p3"},
	}
	history := map[string][]valuation.Snapshot{
		"p1": {{ValuationDate: now.Add(-6 * time.Hour), Value: 5000}},
		"p2": {{ValuationDate: now.Add(-6 * time.Hour), Value: 1500}},
		"p3": {{ValuationDate: now.Add(-6 * time.Hour), Value: 7000}},
	}

	raw, ok := valuation.ComputeTradeValues(rosterPlayers, now, history)
	if !ok {
		t.Fatal("expected a computed result")
	}
	var totals map[string]float64
	if err := json.Unmarshal(raw, &totals); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if totals["7"] != 6500 {
		t.Errorf("expected roster 7 total 6500, got %v", totals["7"])
	}
	if totals["8"] != 7000 {
		t.Errorf("expected roster 8 total 7000, got %v", totals["8"])
	}
}

func TestComputeTradeValues_PartialSideStaysAbsent(t *testing.T) {
	now := time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)
	rosterPlayers := map[int][]string{
		7: {"p1", "unvalued"},
		8: {"p3"},
	}
	history := map[string][]valuation.Snapshot{
		"p1": {{ValuationDate: now.Add(-6 * time.Hour), Value: 5000}},
		"p3": {{ValuationDate: now.Add(-6 * time.Hour), Value: 7000}},
	}

	raw, ok := valuation.ComputeTradeValues(rosterPlayers, now, history)
	if !ok {
		t.Fatal("expected a computed result (roster 8 is fully valued)")
	}
	var totals map[string]float64
	if err := json.Unmarshal(raw, &totals); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := totals["7"]; present {
		t.Errorf("expected roster 7 absent (one unvalued player), got %v", totals["7"])
	}
	if totals["8"] != 7000 {
		t.Errorf("expected roster 8 total 7000, got %v", totals["8"])
	}
}

func TestComputeTradeValues_StaleSnapshotTreatedAsAbsent(t *testing.T) {
	now := time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)
	rosterPlayers := map[int][]string{7: {"p1"}}
	history := map[string][]valuation.Snapshot{
		"p1": {{ValuationDate: now.Add(-30 * time.Hour), Value: 5000}},
	}

	if _, ok := valuation.ComputeTradeValues(rosterPlayers, now, history); ok {
		t.Error("expected no result — the only snapshot is 30h stale, beyond FreshnessWindow")
	}
}

func TestComputeTradeValues_PicksOnlySideStaysAbsent(t *testing.T) {
	now := time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)
	// Roster 9 traded only a pick (not represented in rosterPlayers at all,
	// since GroupPlayersByRoster only sees `adds`), roster 7 traded a valued
	// player.
	rosterPlayers := map[int][]string{7: {"p1"}}
	history := map[string][]valuation.Snapshot{
		"p1": {{ValuationDate: now.Add(-6 * time.Hour), Value: 5000}},
	}

	raw, ok := valuation.ComputeTradeValues(rosterPlayers, now, history)
	if !ok {
		t.Fatal("expected a computed result for roster 7")
	}
	var totals map[string]float64
	json.Unmarshal(raw, &totals)
	if len(totals) != 1 || totals["7"] != 5000 {
		t.Errorf("expected only roster 7 valued at 5000, got %+v", totals)
	}
}

func TestComputeTradeValues_NoneValuedReturnsFalse(t *testing.T) {
	now := time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)
	rosterPlayers := map[int][]string{7: {"unvalued"}}
	if _, ok := valuation.ComputeTradeValues(rosterPlayers, now, map[string][]valuation.Snapshot{}); ok {
		t.Error("expected no result when nothing is valued")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/valuation/... -v`
Expected: FAIL — `package valuation is not in std` / build errors, since `valuation.go` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

Create `backend/internal/valuation/valuation.go`:

```go
// Package valuation resolves a league's model-valuation segment and computes
// per-side trade totals from player_valuations snapshots. It is used by
// transactioncron to persist sleeper_transactions.trade_values at sync time
// — see docs/superpowers/specs/2026-08-10-trade-valuation-totals-design.md.
package valuation

import (
	"encoding/json"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// FreshnessWindow is how stale a snapshot may be, relative to the timestamp
// it's being valued as-of, before it's treated as absent rather than used.
const FreshnessWindow = 24 * time.Hour

// KnownSegments mirrors SEGMENTS in analysis/src/config.py — the league
// formats the valuation model runs on. Only ppr-sf-10 has a systemd job
// actually producing snapshots today; the other two are supported here so
// adding their replay job later doesn't require a code change.
var KnownSegments = map[string]struct{}{
	"ppr-sf-12": {},
	"ppr-sf-10": {},
	"ppr-sf-8":  {},
}

// Snapshot is one dated model valuation for a player, read from
// player_valuations (written by analysis/main.py, never by Go in
// production — this model exists for reads and test fixtures only).
type Snapshot struct {
	Segment         string    `gorm:"column:segment"`
	SleeperPlayerID string    `gorm:"column:sleeper_player_id"`
	ValuationDate   time.Time `gorm:"column:valuation_date"`
	Value           float64   `gorm:"column:value"`
}

func (Snapshot) TableName() string { return "player_valuations" }

// SegmentKeyForLeague maps a league's settings to its valuation segment key,
// or "" when no segment covers that format.
func SegmentKeyForLeague(ppr *float64, isSuperflex *bool, totalRosters int, leagueType string) string {
	if ppr == nil || *ppr != 1.0 || isSuperflex == nil || !*isSuperflex || leagueType != "redraft" {
		return ""
	}
	key := "ppr-sf-" + strconv.Itoa(totalRosters)
	if _, ok := KnownSegments[key]; !ok {
		return ""
	}
	return key
}

// LoadSnapshotHistory fetches one segment's valuation snapshots up to upTo
// for the given players, grouped per player and sorted by date ascending.
func LoadSnapshotHistory(db *gorm.DB, segment string, playerIDs []string, upTo time.Time) map[string][]Snapshot {
	history := map[string][]Snapshot{}
	if segment == "" || len(playerIDs) == 0 {
		return history
	}
	var snaps []Snapshot
	db.Table("player_valuations").
		Select("sleeper_player_id, valuation_date, value").
		Where("segment = ? AND sleeper_player_id IN ? AND valuation_date <= ?", segment, playerIDs, upTo).
		Order("sleeper_player_id, valuation_date ASC").
		Scan(&snaps)
	for _, s := range snaps {
		history[s.SleeperPlayerID] = append(history[s.SleeperPlayerID], s)
	}
	return history
}

// ValueAsOf returns the latest snapshot value at or before ts, provided it's
// within maxAge of ts — a snapshot older than that is treated the same as no
// snapshot at all. snaps must be sorted by date ascending.
func ValueAsOf(snaps []Snapshot, ts time.Time, maxAge time.Duration) (float64, bool) {
	for i := len(snaps) - 1; i >= 0; i-- {
		if !snaps[i].ValuationDate.After(ts) {
			if ts.Sub(snaps[i].ValuationDate) > maxAge {
				return 0, false
			}
			return snaps[i].Value, true
		}
	}
	return 0, false
}

// GroupPlayersByRoster inverts a trade's `adds` map (player_id -> roster_id,
// the shape of sleeper_transactions.adds) into roster_id -> player IDs.
// Draft picks are never included — there is no pick-valuation model, and
// picks live in a separate column (draft_picks) this function never sees.
func GroupPlayersByRoster(adds map[string]int) map[int][]string {
	rosters := map[int][]string{}
	for playerID, rosterID := range adds {
		rosters[rosterID] = append(rosters[rosterID], playerID)
	}
	return rosters
}

// ComputeTradeValues sums each roster's players into a total, requiring
// every player on a roster to resolve via ValueAsOf before that roster's
// total is included — a roster with any unvalued player, or with no players
// at all (a picks-only side), is simply omitted. Returns (nil, false) when
// no roster is fully valued, so callers can store SQL NULL rather than {}.
func ComputeTradeValues(rosterPlayers map[int][]string, tradeTime time.Time, history map[string][]Snapshot) (json.RawMessage, bool) {
	totals := map[string]float64{}
	for rosterID, playerIDs := range rosterPlayers {
		if len(playerIDs) == 0 {
			continue
		}
		var total float64
		complete := true
		for _, pid := range playerIDs {
			v, ok := ValueAsOf(history[pid], tradeTime, FreshnessWindow)
			if !ok {
				complete = false
				break
			}
			total += v
		}
		if complete {
			totals[strconv.Itoa(rosterID)] = total
		}
	}
	if len(totals) == 0 {
		return nil, false
	}
	raw, err := json.Marshal(totals)
	if err != nil {
		return nil, false
	}
	return raw, true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/valuation/... -v`
Expected: PASS (all cases in Step 1)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/valuation/
git commit -m "feat(valuation): add internal/valuation package for trade-side computation"
```

---

### Task 3: `transactioncron` shared batching helper

**Files:**
- Create: `backend/internal/transactioncron/valuation.go`
- Create: `backend/internal/transactioncron/valuation_test.go`

**Interfaces:**
- Consumes: `valuation.SegmentKeyForLeague`, `valuation.LoadSnapshotHistory`, `valuation.GroupPlayersByRoster`, `valuation.ComputeTradeValues` (Task 2)
- Produces: `tradeValuationInput{ID string, TradeTime time.Time, Adds map[string]int, Segment string}` and `computeTradeValuesForRows(ctx context.Context, db *gorm.DB, inputs []tradeValuationInput) map[string]json.RawMessage`, consumed by Task 4 (`FlushLeagueTransactions`) and Task 5 (`ReconcileTradeValues`).

- [ ] **Step 1: Write the failing test**

Create `backend/internal/transactioncron/valuation_test.go`:

```go
package transactioncron

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/valuation"
)

func newValuationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&valuation.Snapshot{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestComputeTradeValuesForRows(t *testing.T) {
	db := newValuationTestDB(t)
	now := time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)
	db.Create(&valuation.Snapshot{Segment: "ppr-sf-10", SleeperPlayerID: "p1", ValuationDate: now.Add(-6 * time.Hour), Value: 5000})
	db.Create(&valuation.Snapshot{Segment: "ppr-sf-10", SleeperPlayerID: "p2", ValuationDate: now.Add(-6 * time.Hour), Value: 1500})
	// p3 has no snapshot at all.

	inputs := []tradeValuationInput{
		{ID: "tx-full", TradeTime: now, Adds: map[string]int{"p1": 7, "p2": 8}, Segment: "ppr-sf-10"},
		{ID: "tx-partial", TradeTime: now, Adds: map[string]int{"p1": 7, "p3": 8}, Segment: "ppr-sf-10"},
		{ID: "tx-uncovered", TradeTime: now, Adds: map[string]int{"p1": 7}, Segment: ""},
	}

	got := computeTradeValuesForRows(context.Background(), db, inputs)

	if _, ok := got["tx-full"]; !ok {
		t.Fatal("expected tx-full to have computed values")
	}
	var full map[string]float64
	json.Unmarshal(got["tx-full"], &full)
	if full["7"] != 5000 || full["8"] != 1500 {
		t.Errorf("expected {7:5000, 8:1500}, got %+v", full)
	}

	if raw, ok := got["tx-partial"]; ok {
		var partial map[string]float64
		json.Unmarshal(raw, &partial)
		if _, present := partial["8"]; present {
			t.Errorf("expected roster 8 absent for tx-partial (p3 unvalued), got %+v", partial)
		}
		if partial["7"] != 5000 {
			t.Errorf("expected roster 7 = 5000 for tx-partial, got %+v", partial)
		}
	} else {
		t.Error("expected tx-partial to still have a partial result (roster 7 is valued)")
	}

	if _, ok := got["tx-uncovered"]; ok {
		t.Error("expected tx-uncovered (empty segment) to be skipped entirely")
	}
}

func TestComputeTradeValuesForRows_EmptyInputsReturnsEmptyMap(t *testing.T) {
	db := newValuationTestDB(t)
	got := computeTradeValuesForRows(context.Background(), db, nil)
	if len(got) != 0 {
		t.Errorf("expected empty result, got %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/transactioncron/... -run TestComputeTradeValuesForRows -v`
Expected: FAIL — `undefined: tradeValuationInput` / `undefined: computeTradeValuesForRows`

- [ ] **Step 3: Write the implementation**

Create `backend/internal/transactioncron/valuation.go`:

```go
package transactioncron

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"backend/internal/valuation"
)

// tradeValuationInput is one trade candidate for value computation — either
// a freshly-fetched row (FlushLeagueTransactions) or one read back from the
// DB with a null trade_values column (ReconcileTradeValues). Segment is
// resolved by the caller (it needs the trade's league settings, which this
// package's two call sites fetch differently) and is "" for trades outside
// the model's covered segments — computeTradeValuesForRows skips those
// without touching the database.
type tradeValuationInput struct {
	ID        string
	TradeTime time.Time
	Adds      map[string]int
	Segment   string
}

// computeTradeValuesForRows batch-loads player_valuations once per distinct
// segment present in inputs (not once per trade), then computes each
// trade's per-side totals. Mirrors the batching handlers.GetSleeperTrades
// used to do inline before this package took over — see
// docs/superpowers/specs/2026-08-10-trade-valuation-totals-design.md.
// Returns a map from trade ID to its computed trade_values JSON; a trade
// with an empty Segment or no fully-valued side is simply absent from the
// result, not present with a nil/empty value.
func computeTradeValuesForRows(ctx context.Context, db *gorm.DB, inputs []tradeValuationInput) map[string]json.RawMessage {
	result := map[string]json.RawMessage{}

	playersBySegment := map[string]map[string]struct{}{}
	var maxTime time.Time
	for _, in := range inputs {
		if in.Segment == "" {
			continue
		}
		if in.TradeTime.After(maxTime) {
			maxTime = in.TradeTime
		}
		if playersBySegment[in.Segment] == nil {
			playersBySegment[in.Segment] = map[string]struct{}{}
		}
		for pid := range in.Adds {
			playersBySegment[in.Segment][pid] = struct{}{}
		}
	}
	if len(playersBySegment) == 0 {
		return result
	}

	historyBySegment := map[string]map[string][]valuation.Snapshot{}
	for seg, idSet := range playersBySegment {
		ids := make([]string, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		historyBySegment[seg] = valuation.LoadSnapshotHistory(db.WithContext(ctx), seg, ids, maxTime)
	}

	for _, in := range inputs {
		if in.Segment == "" {
			continue
		}
		rosterPlayers := valuation.GroupPlayersByRoster(in.Adds)
		if tv, ok := valuation.ComputeTradeValues(rosterPlayers, in.TradeTime, historyBySegment[in.Segment]); ok {
			result[in.ID] = tv
		}
	}
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/transactioncron/... -run TestComputeTradeValuesForRows -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/transactioncron/valuation.go backend/internal/transactioncron/valuation_test.go
git commit -m "feat(transactioncron): add batched trade-value computation helper"
```

---

### Task 4: Insert-time trade valuation in `FlushLeagueTransactions`

**Files:**
- Modify: `backend/internal/transactioncron/fetch.go:196-252` (`FlushLeagueTransactions`)
- Modify: `backend/internal/transactioncron/fetch_test.go` (`newTestDB`, plus new test)

**Interfaces:**
- Consumes: `computeTradeValuesForRows` (Task 3), `valuation.SegmentKeyForLeague` (Task 2)
- Produces: `FlushLeagueTransactions` now sets `TradeValues` on qualifying rows before insert — no change to its exported signature.

- [ ] **Step 1: Extend the test DB fixture and write the failing test**

In `backend/internal/transactioncron/fetch_test.go`, add `&valuation.Snapshot{}` to `newTestDB`'s `AutoMigrate` call and the import:

```go
import (
	// ...existing imports...
	"backend/internal/valuation"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("unwrap sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.SleeperLeague{}, &models.SleeperTransaction{}, &valuation.Snapshot{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}
```

Add to `backend/internal/transactioncron/fetch_test.go`:

```go
func ppr10League(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	ppr := 1.0
	sf := true
	now := time.Now().UTC()
	l := models.SleeperLeague{
		SleeperLeagueID: id, Season: "2025", LastFetchedAt: &now, ClaimedAt: &now,
		PPR: &ppr, IsSuperflex: &sf, TotalRosters: 10, LeagueType: "redraft",
	}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
}

func TestFlushLeagueTransactions_ComputesTradeValuesForCoveredLeague(t *testing.T) {
	db := newTestDB(t)
	ppr10League(t, db, "lg1")

	tradeTime := time.Now().UTC()
	db.Create(&valuation.Snapshot{Segment: "ppr-sf-10", SleeperPlayerID: "p1", ValuationDate: tradeTime.Add(-6 * time.Hour), Value: 5000})
	db.Create(&valuation.Snapshot{Segment: "ppr-sf-10", SleeperPlayerID: "p2", ValuationDate: tradeTime.Add(-6 * time.Hour), Value: 1500})

	batch := []transactioncron.LeagueTransactionFetchResult{{
		LeagueID: "lg1",
		CloudRows: []models.SleeperTransaction{{
			SleeperTransactionID: "tx1", SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
			CreatedAtSleeper: tradeTime.UnixMilli(),
			Adds:             json.RawMessage(`{"p1": 7, "p2": 8}`),
		}},
	}}
	dfa := &activities.DataFetchActivities{DB: db}
	if err := transactioncron.FlushLeagueTransactions(context.Background(), dfa, db, batch); err != nil {
		t.Fatalf("FlushLeagueTransactions error: %v", err)
	}

	var tx models.SleeperTransaction
	if err := db.First(&tx, "sleeper_transaction_id = ?", "tx1").Error; err != nil {
		t.Fatalf("lookup transaction: %v", err)
	}
	if len(tx.TradeValues) == 0 {
		t.Fatal("expected trade_values to be populated")
	}
	var totals map[string]float64
	json.Unmarshal(tx.TradeValues, &totals)
	if totals["7"] != 5000 || totals["8"] != 1500 {
		t.Errorf("expected {7:5000, 8:1500}, got %+v", totals)
	}
}

func TestFlushLeagueTransactions_LeavesTradeValuesNullWhenUnvalued(t *testing.T) {
	db := newTestDB(t)
	ppr10League(t, db, "lg1")

	batch := []transactioncron.LeagueTransactionFetchResult{{
		LeagueID: "lg1",
		CloudRows: []models.SleeperTransaction{{
			SleeperTransactionID: "tx1", SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
			CreatedAtSleeper: time.Now().UTC().UnixMilli(),
			Adds:             json.RawMessage(`{"p1": 7}`), // no player_valuations rows seeded
		}},
	}}
	dfa := &activities.DataFetchActivities{DB: db}
	if err := transactioncron.FlushLeagueTransactions(context.Background(), dfa, db, batch); err != nil {
		t.Fatalf("FlushLeagueTransactions error: %v", err)
	}

	var tx models.SleeperTransaction
	db.First(&tx, "sleeper_transaction_id = ?", "tx1")
	if len(tx.TradeValues) != 0 {
		t.Errorf("expected trade_values to stay null, got %s", tx.TradeValues)
	}
}
```

Note: `transactioncron` is used both unqualified (this file is `package transactioncron`, per Task 3) and qualified as `transactioncron.X` in `fetch_test.go` (`package transactioncron_test`, an external test package) — match whichever package declaration already exists at the top of `fetch_test.go`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/transactioncron/... -run TestFlushLeagueTransactions_ComputesTradeValues -v`
Expected: FAIL — `tx.TradeValues` empty (no computation wired in yet)

- [ ] **Step 3: Wire the computation into `FlushLeagueTransactions`**

In `backend/internal/transactioncron/fetch.go`, add the import and a new `leagueValuationSettings` type + `attachTradeValues` function, then call it from `FlushLeagueTransactions`:

```go
import (
	// ...existing imports...
	"backend/internal/valuation"
)
```

```go
// leagueValuationSettings is the subset of a league's settings needed to
// resolve its valuation segment (valuation.SegmentKeyForLeague).
type leagueValuationSettings struct {
	SleeperLeagueID string   `gorm:"column:sleeper_league_id"`
	PPR             *float64 `gorm:"column:ppr"`
	IsSuperflex     *bool    `gorm:"column:is_superflex"`
	TotalRosters    int      `gorm:"column:total_rosters"`
	LeagueType      string   `gorm:"column:league_type"`
}

// attachTradeValues sets TradeValues on each complete trade row in rows
// whose league is in a covered valuation segment and whose players all have
// a fresh-enough valuation. Best-effort: any lookup failure just leaves
// TradeValues nil for ReconcileTradeValues to retry later — it must never
// fail the insert this is called from.
func attachTradeValues(ctx context.Context, tx *gorm.DB, leagueIDs []string, rows []models.SleeperTransaction) {
	var leagues []leagueValuationSettings
	if err := tx.WithContext(ctx).Table("sleeper_leagues").
		Select("sleeper_league_id, ppr, is_superflex, total_rosters, league_type").
		Where("sleeper_league_id IN ?", leagueIDs).
		Scan(&leagues).Error; err != nil {
		return
	}
	settingsByLeague := make(map[string]leagueValuationSettings, len(leagues))
	for _, l := range leagues {
		settingsByLeague[l.SleeperLeagueID] = l
	}

	inputs := make([]tradeValuationInput, 0, len(rows))
	rowIndexByID := make(map[string]int, len(rows))
	for i, r := range rows {
		if r.Type != "trade" || r.Status != "complete" {
			continue
		}
		ls, ok := settingsByLeague[r.SleeperLeagueID]
		if !ok {
			continue
		}
		seg := valuation.SegmentKeyForLeague(ls.PPR, ls.IsSuperflex, ls.TotalRosters, ls.LeagueType)
		if seg == "" {
			continue
		}
		var adds map[string]int
		if len(r.Adds) > 0 {
			if err := json.Unmarshal(r.Adds, &adds); err != nil {
				continue
			}
		}
		inputs = append(inputs, tradeValuationInput{
			ID: r.SleeperTransactionID, TradeTime: time.UnixMilli(r.CreatedAtSleeper).UTC(),
			Adds: adds, Segment: seg,
		})
		rowIndexByID[r.SleeperTransactionID] = i
	}
	if len(inputs) == 0 {
		return
	}

	for id, tv := range computeTradeValuesForRows(ctx, tx, inputs) {
		rows[rowIndexByID[id]].TradeValues = tv
	}
}
```

Then in `FlushLeagueTransactions`, call `attachTradeValues` right after `cloudRows` is fully assembled, before the insert:

```go
	if len(cloudRows) > 0 {
		attachTradeValues(ctx, tx, leagueIDs, cloudRows)
		if err := tx.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(cloudRows, 500).Error; err != nil {
			return fmt.Errorf("cloud upsert: %w", err)
		}
	}
```

(`json` needs to already be imported in `fetch.go` — check the existing import block; add `"encoding/json"` if it isn't there yet.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/transactioncron/... -v`
Expected: PASS — both new tests, and all pre-existing `fetch_test.go` tests still pass (the `newTestDB` AutoMigrate change is additive).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/transactioncron/fetch.go backend/internal/transactioncron/fetch_test.go
git commit -m "feat(transactioncron): compute trade_values at insert time"
```

---

### Task 5: `ReconcileTradeValues` sweep

**Files:**
- Create: `backend/internal/transactioncron/reconcile.go`
- Create: `backend/internal/transactioncron/reconcile_test.go`

**Interfaces:**
- Consumes: `computeTradeValuesForRows` (Task 3), `valuation.SegmentKeyForLeague` (Task 2), `newTestDB`/`ppr10League` (Task 4, reused from `fetch_test.go` — same test package)
- Produces: `ReconcileTradeValues(ctx context.Context, db *gorm.DB, limit int) error`, consumed by Task 6 (`RunTransactionSync`).

- [ ] **Step 1: Write the failing test**

Create `backend/internal/transactioncron/reconcile_test.go`:

```go
package transactioncron_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/transactioncron"
	"backend/internal/valuation"
)

func TestReconcileTradeValues_FillsNullRowWhenValuationExists(t *testing.T) {
	db := newTestDB(t)
	ppr10League(t, db, "lg1")

	tradeTime := time.Now().UTC().Add(-time.Hour)
	db.Create(&models.SleeperTransaction{
		SleeperTransactionID: "tx1", SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
		CreatedAtSleeper: tradeTime.UnixMilli(), Adds: json.RawMessage(`{"p1": 7}`),
	})
	db.Create(&valuation.Snapshot{Segment: "ppr-sf-10", SleeperPlayerID: "p1", ValuationDate: tradeTime.Add(-6 * time.Hour), Value: 4200})

	dfa := &activities.DataFetchActivities{DB: db}
	_ = dfa
	if err := transactioncron.ReconcileTradeValues(context.Background(), db, 200); err != nil {
		t.Fatalf("ReconcileTradeValues error: %v", err)
	}

	var tx models.SleeperTransaction
	db.First(&tx, "sleeper_transaction_id = ?", "tx1")
	if len(tx.TradeValues) == 0 {
		t.Fatal("expected trade_values to be filled in")
	}
	var totals map[string]float64
	json.Unmarshal(tx.TradeValues, &totals)
	if totals["7"] != 4200 {
		t.Errorf("expected {7:4200}, got %+v", totals)
	}
}

func TestReconcileTradeValues_LeavesRowNullWhenStillUnvalued(t *testing.T) {
	db := newTestDB(t)
	ppr10League(t, db, "lg1")
	db.Create(&models.SleeperTransaction{
		SleeperTransactionID: "tx1", SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
		CreatedAtSleeper: time.Now().UTC().UnixMilli(), Adds: json.RawMessage(`{"p1": 7}`),
	})

	if err := transactioncron.ReconcileTradeValues(context.Background(), db, 200); err != nil {
		t.Fatalf("ReconcileTradeValues error: %v", err)
	}

	var tx models.SleeperTransaction
	db.First(&tx, "sleeper_transaction_id = ?", "tx1")
	if len(tx.TradeValues) != 0 {
		t.Errorf("expected trade_values to stay null, got %s", tx.TradeValues)
	}
}

func TestReconcileTradeValues_SkipsAlreadyValuedRows(t *testing.T) {
	db := newTestDB(t)
	ppr10League(t, db, "lg1")
	existing := json.RawMessage(`{"7": 999}`)
	db.Create(&models.SleeperTransaction{
		SleeperTransactionID: "tx1", SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
		CreatedAtSleeper: time.Now().UTC().UnixMilli(), Adds: json.RawMessage(`{"p1": 7}`),
		TradeValues: existing,
	})
	db.Create(&valuation.Snapshot{Segment: "ppr-sf-10", SleeperPlayerID: "p1", ValuationDate: time.Now().UTC(), Value: 4200})

	if err := transactioncron.ReconcileTradeValues(context.Background(), db, 200); err != nil {
		t.Fatalf("ReconcileTradeValues error: %v", err)
	}

	var tx models.SleeperTransaction
	db.First(&tx, "sleeper_transaction_id = ?", "tx1")
	var totals map[string]float64
	json.Unmarshal(tx.TradeValues, &totals)
	if totals["7"] != 999 {
		t.Errorf("expected the pre-existing value 999 left untouched, got %+v", totals)
	}
}

func TestReconcileTradeValues_RespectsLimit(t *testing.T) {
	db := newTestDB(t)
	ppr10League(t, db, "lg1")
	base := time.Now().UTC()
	for i := 0; i < 3; i++ {
		db.Create(&models.SleeperTransaction{
			SleeperTransactionID: "tx" + string(rune('1'+i)), SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
			CreatedAtSleeper: base.Add(-time.Duration(i) * time.Hour).UnixMilli(),
			Adds:             json.RawMessage(`{"p1": 7}`),
		})
	}
	db.Create(&valuation.Snapshot{Segment: "ppr-sf-10", SleeperPlayerID: "p1", ValuationDate: base.Add(-6 * time.Hour), Value: 4200})

	if err := transactioncron.ReconcileTradeValues(context.Background(), db, 1); err != nil {
		t.Fatalf("ReconcileTradeValues error: %v", err)
	}

	var filled int64
	db.Model(&models.SleeperTransaction{}).Where("trade_values IS NOT NULL").Count(&filled)
	if filled != 1 {
		t.Errorf("expected exactly 1 row filled with limit=1, got %d", filled)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/transactioncron/... -run TestReconcileTradeValues -v`
Expected: FAIL — `undefined: transactioncron.ReconcileTradeValues`

- [ ] **Step 3: Write the implementation**

Create `backend/internal/transactioncron/reconcile.go`:

```go
package transactioncron

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"backend/internal/models"
	"backend/internal/valuation"
)

// ReconcileTradeValues fills in trade_values for up to limit trades whose
// value is still null, newest first, restricted at the query level to
// leagues in a covered valuation format (matching
// valuation.SegmentKeyForLeague's own conditions) so a permanently-uncovered
// league's trades can never crowd the LIMIT window and starve reconcilable
// ones. This is the sole backfill mechanism for pre-existing trades — see
// docs/superpowers/specs/2026-08-10-trade-valuation-totals-design.md — and
// must stay cheap: RunTransactionSync runs it concurrently with, not after,
// each tick's fetch/flush, under its own short deadline, so it can never
// delay new-trade ingestion.
func ReconcileTradeValues(ctx context.Context, db *gorm.DB, limit int) error {
	type row struct {
		SleeperTransactionID string          `gorm:"column:sleeper_transaction_id"`
		Adds                 json.RawMessage `gorm:"column:adds"`
		CreatedAtSleeper     int64           `gorm:"column:created_at_sleeper"`
		PPR                  *float64        `gorm:"column:ppr"`
		IsSuperflex          *bool           `gorm:"column:is_superflex"`
		TotalRosters         int             `gorm:"column:total_rosters"`
		LeagueType           string          `gorm:"column:league_type"`
	}
	var rows []row
	if err := db.WithContext(ctx).Table("sleeper_transactions t").
		Select("t.sleeper_transaction_id, t.adds, t.created_at_sleeper, l.ppr, l.is_superflex, l.total_rosters, l.league_type").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = t.sleeper_league_id").
		Where("t.type = ? AND t.status = ? AND t.trade_values IS NULL AND l.ppr = ? AND l.is_superflex = ? AND l.league_type = ?",
			"trade", "complete", 1.0, true, "redraft").
		Order("t.created_at_sleeper DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return fmt.Errorf("select null trade_values: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	inputs := make([]tradeValuationInput, 0, len(rows))
	for _, r := range rows {
		seg := valuation.SegmentKeyForLeague(r.PPR, r.IsSuperflex, r.TotalRosters, r.LeagueType)
		if seg == "" {
			continue
		}
		var adds map[string]int
		if len(r.Adds) > 0 {
			if err := json.Unmarshal(r.Adds, &adds); err != nil {
				continue
			}
		}
		inputs = append(inputs, tradeValuationInput{
			ID: r.SleeperTransactionID, TradeTime: time.UnixMilli(r.CreatedAtSleeper).UTC(),
			Adds: adds, Segment: seg,
		})
	}
	if len(inputs) == 0 {
		return nil
	}

	for id, tv := range computeTradeValuesForRows(ctx, db, inputs) {
		if err := db.WithContext(ctx).Model(&models.SleeperTransaction{}).
			Where("sleeper_transaction_id = ? AND trade_values IS NULL", id).
			Update("trade_values", tv).Error; err != nil {
			return fmt.Errorf("update trade_values for %s: %w", id, err)
		}
	}
	return nil
}
```

Note: the `l.ppr = ? AND l.is_superflex = ? AND l.league_type = ?` predicate intentionally only narrows to *redraft, PPR, superflex* leagues of any roster count — `valuation.SegmentKeyForLeague` is still the source of truth for which roster counts are actually covered (8/10/12), applied per-row in the loop above. This keeps the SQL predicate simple while still excluding the large majority of never-coverable trades from the LIMIT window.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/transactioncron/... -run TestReconcileTradeValues -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/transactioncron/reconcile.go backend/internal/transactioncron/reconcile_test.go
git commit -m "feat(transactioncron): add ReconcileTradeValues backfill sweep"
```

---

### Task 6: Wire the reconcile sweep concurrently into `RunTransactionSync`

**Files:**
- Modify: `backend/internal/transactioncron/transactioncron.go`
- Modify: `backend/internal/transactioncron/transactioncron_test.go`

**Interfaces:**
- Consumes: `ReconcileTradeValues` (Task 5)
- Produces: `Config.ReconcileLimit int`, `Config.ReconcileTimeout time.Duration` (new fields); `RunTransactionSync`'s exported signature and `Report` shape are unchanged.

- [ ] **Step 1: Write the failing test**

Add to `backend/internal/transactioncron/transactioncron_test.go` (same file as the existing Postgres-gated `TestRunTransactionSync_ProcessesLeaguesToCompletion` — copy its skip/setup pattern):

```go
// TestRunTransactionSync_ReconcilesTradeValuesConcurrently seeds a trade
// with trade_values already null (as if a previous flush ran before its
// players had valuations) and a matching player_valuations snapshot, then
// asserts a single RunTransactionSync call reconciles it — proving the
// sweep is actually wired in and awaited before the run returns, not just
// unit-testable in isolation (Task 5 already covers the sweep's own logic).
func TestRunTransactionSync_ReconcilesTradeValuesConcurrently(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; RunTransactionSync needs Postgres (FOR UPDATE SKIP LOCKED claim query)")
	}
	scopedDSN := testutil.NewPGSchema(t, dsn, "transactioncron_reconcile_test")
	db := testutil.OpenGORM(t, scopedDSN)
	if err := db.AutoMigrate(&models.SleeperLeague{}, &models.SleeperTransaction{}, &valuation.Snapshot{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	now := time.Now().UTC()
	ppr := 1.0
	sf := true
	// last_transactions_fetched_at already set so this league is not
	// re-claimed for a fetch — this test only cares about the reconcile
	// sweep, not the claim/fetch pipeline (already covered elsewhere).
	db.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg1", Season: "2026", LastFetchedAt: &now, LastTransactionsFetchedAt: &now,
		PPR: &ppr, IsSuperflex: &sf, TotalRosters: 10, LeagueType: "redraft",
	})
	tradeTime := now.Add(-time.Hour)
	db.Create(&models.SleeperTransaction{
		SleeperTransactionID: "tx1", SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
		CreatedAtSleeper: tradeTime.UnixMilli(), Adds: json.RawMessage(`{"p1": 7}`),
	})
	db.Create(&valuation.Snapshot{Segment: "ppr-sf-10", SleeperPlayerID: "p1", ValuationDate: tradeTime.Add(-6 * time.Hour), Value: 3300})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/state/nfl" {
			json.NewEncoder(w).Encode(sleeper.NFLState{Season: "2026", Week: 3})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	cfg := transactioncron.Config{
		PoolSize: 2, RefillBatch: 1, BatchSize: 5, BatchFlushInterval: 5 * time.Second,
		ReconcileLimit: 200, ReconcileTimeout: 5 * time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	if _, err := transactioncron.RunTransactionSync(ctx, dfa, cfg); err != nil {
		t.Fatalf("RunTransactionSync error: %v", err)
	}

	var tx models.SleeperTransaction
	db.First(&tx, "sleeper_transaction_id = ?", "tx1")
	if len(tx.TradeValues) == 0 {
		t.Fatal("expected trade_values to be reconciled by the end of RunTransactionSync")
	}
}
```

Add `"backend/internal/valuation"` to this file's import block if not already present (Task 4/5's SQLite tests live in `fetch_test.go`/`reconcile_test.go`, not this file, so it may not be imported here yet).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && TEST_DATABASE_URL=<a real Postgres DSN, e.g. postgres://localhost:5432/ff_sims_test?sslmode=disable> go test ./internal/transactioncron/... -run TestRunTransactionSync_ReconcilesTradeValuesConcurrently -v`

This test needs a real, reachable Postgres instance — `ClaimLeaguesForTransactions` (exercised by `fdb.RunPool` inside `RunTransactionSync`) uses `FOR UPDATE SKIP LOCKED`, which SQLite's parser rejects outright. If no `TEST_DATABASE_URL` is available in this environment, skip running this specific test locally (it will `t.Skip()` cleanly without the env var — confirm that skip happens rather than a compile error) and rely on CI to exercise it; all other steps in this task do not require Postgres.

Expected (with `TEST_DATABASE_URL` set): FAIL — `cfg.ReconcileLimit`/`cfg.ReconcileTimeout` undefined (Config fields don't exist yet)

- [ ] **Step 3: Add Config fields and wire the goroutine**

In `backend/internal/transactioncron/transactioncron.go`, add two fields to `Config`:

```go
type Config struct {
	PoolSize    int `env:"CRON_TXN_POOL_SIZE,default=80,min=1"`
	RefillBatch int `env:"CRON_TXN_REFILL_BATCH,default=40,min=1"`
	BatchSize int `env:"CRON_TXN_BATCH_SIZE,default=160,min=1"`
	BatchFlushInterval time.Duration `env:"CRON_TXN_BATCH_FLUSH_INTERVAL_DURATION,default=5s,min=1s"`
	ShutdownGracePeriod time.Duration `env:"CRON_TXN_SHUTDOWN_GRACE_PERIOD_DURATION,default=30s,min=0s"`
	// ReconcileLimit caps how many null-valued trades ReconcileTradeValues
	// attempts per tick — bounds its worst-case cost regardless of backlog
	// size.
	ReconcileLimit int `env:"CRON_TXN_RECONCILE_LIMIT,default=200,min=1"`
	// ReconcileTimeout is the sweep's own deadline, independent of the run's
	// overall -max-duration — it must never be the reason a tick runs long.
	ReconcileTimeout time.Duration `env:"CRON_TXN_RECONCILE_TIMEOUT_DURATION,default=5s,min=1s"`
}
```

(Keep the existing field comments already in the file — only the two new fields and their comments are additions.)

Then in `RunTransactionSync`, launch the sweep before `fdb.RunPool` and wait for it after:

```go
func RunTransactionSync(ctx context.Context, dfa *activities.DataFetchActivities, cfg Config) (Report, error) {
	logger := newStdLogger()
	logger.Info("transaction sync cron starting", "poolSize", cfg.PoolSize, "refillBatch", cfg.RefillBatch,
		"batchSize", cfg.BatchSize, "batchFlushInterval", cfg.BatchFlushInterval,
		"shutdownGracePeriod", cfg.ShutdownGracePeriod)
	start := time.Now()

	// Launched concurrently with (not after) fetch -> flush below, so its
	// own short deadline overlaps the real wall-clock time fetch -> flush
	// already spends on Sleeper API calls, rather than adding to the tick's
	// total duration in the common case. See docs/superpowers/specs/
	// 2026-08-10-trade-valuation-totals-design.md.
	reconcileCtx, reconcileCancel := context.WithTimeout(ctx, cfg.ReconcileTimeout)
	defer reconcileCancel()
	reconcileErrCh := make(chan error, 1)
	go func() {
		reconcileErrCh <- ReconcileTradeValues(reconcileCtx, dfa.DB, cfg.ReconcileLimit)
	}()

	state, err := dfa.Sleeper.GetNFLState(ctx)
	if err != nil {
		logger.Warn("GetNFLState failed; falling back to full 18-leg sweep", "error", err)
		state = nil
	}

	result := fdb.RunPool(ctx, dfa.DB,
		// ...unchanged fdb.RunPool call...
	)

	if err := <-reconcileErrCh; err != nil {
		logger.Warn("trade value reconciliation failed", "error", err)
	}

	report := Report{
		// ...unchanged...
	}
	// ...unchanged remainder...
}
```

Only the two blocks shown (the `reconcileCtx`/goroutine launch before `GetNFLState`, and the `<-reconcileErrCh` wait after `fdb.RunPool`) are new — everything else in the function body stays exactly as it is today.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go build ./... && go test ./internal/transactioncron/... -v`
Expected: PASS for all tests in the package, including the new Postgres-gated one (or a clean SKIP if `TEST_DATABASE_URL` isn't set — either is acceptable for this step, but if you have a way to set it, confirm the PASS case at least once).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/transactioncron/transactioncron.go backend/internal/transactioncron/transactioncron_test.go
git commit -m "feat(transactioncron): run ReconcileTradeValues concurrently every tick"
```

---

### Task 7: Read path — `GetSleeperTrades` reads the persisted column

**Files:**
- Modify: `backend/internal/api/handlers/sleeper.go`
- Modify: `backend/internal/api/handlers/sleeper_test.go`
- Modify: `frontend/src/types/models.ts`

**Interfaces:**
- Consumes: `models.SleeperTransaction.TradeValues` (Task 1)
- Produces: `GetSleeperTrades`'s JSON response shape is unchanged except `TradeSidePlayer.value` (per-player) is always omitted now — `TradeSide.total_value` (per-side) is unchanged in shape and semantics from the caller's perspective, just sourced differently.

- [ ] **Step 1: Update the failing/changing tests first**

In `backend/internal/api/handlers/sleeper_test.go`:

1. Delete `TestSegmentKeyForLeague` (lines ~16-40) and `TestValueAsOf` (~ the `func TestValueAsOf` block) — both moved to `internal/valuation/valuation_test.go` in Task 2.
2. Delete `TestApplySideValues` and `TestApplySideValues_NoValuations` — `applySideValues` is being deleted; the equivalent coverage lives in `TestComputeTradeValues_*` (Task 2, `internal/valuation`) and `TestComputeTradeValuesForRows` (Task 3, `internal/transactioncron`).
3. Keep `TestFormatScoring` and `TestFormatLeagueSize` — unchanged, those functions aren't moving.
4. Update `TestGetSleeperTrades_FiltersPlayerToRecentWindowAndPaginates` (and any other `GetSleeperTrades` test) to seed `TradeValues` directly on the fixture rows instead of relying on live computation — e.g. wherever the existing test creates a `models.SleeperTransaction{...}` fixture that's meant to end up valued, add `TradeValues: json.RawMessage(`{"1": 1000}`)` (matching whatever roster ID that fixture's `adds` uses) directly to the literal.

Add one new test proving the handler reads the column rather than computing:

```go
func TestGetSleeperTrades_ReadsPersistedTradeValues(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	ppr := 1.0
	sf := true
	db.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg1", Name: "Test League", Season: "2025",
		PPR: &ppr, IsSuperflex: &sf, TotalRosters: 10, LeagueType: "redraft",
	})
	db.Create(&models.SleeperTransaction{
		SleeperTransactionID: "tx1", SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
		CreatedAtSleeper: time.Now().UTC().UnixMilli(),
		Adds:             json.RawMessage(`{"p1": 7, "p2": 8}`),
		TradeValues:      json.RawMessage(`{"7": 5000}`), // roster 8 intentionally absent (unvalued)
	})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/sleeper/trades", GetSleeperTrades)
	req := httptest.NewRequest(http.MethodGet, "/sleeper/trades", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var response SleeperTradesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(response.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(response.Trades))
	}
	var side7, side8 *TradeSide
	for i := range response.Trades[0].Sides {
		s := &response.Trades[0].Sides[i]
		switch s.RosterID {
		case 7:
			side7 = s
		case 8:
			side8 = s
		}
	}
	if side7 == nil || side7.TotalValue == nil || *side7.TotalValue != 5000 {
		t.Errorf("expected roster 7 total_value 5000, got %+v", side7)
	}
	if side8 == nil || side8.TotalValue != nil {
		t.Errorf("expected roster 8 total_value nil (absent from persisted trade_values), got %+v", side8)
	}
}
```

- [ ] **Step 2: Run tests to verify the new one fails**

Run: `cd backend && go test ./internal/api/handlers/... -run TestGetSleeperTrades_ReadsPersistedTradeValues -v`
Expected: FAIL — `TradeValues` field doesn't exist on the handler's local `tradeRow` yet, so it won't compile / side totals stay nil.

- [ ] **Step 3: Update `sleeper.go`**

Replace the `TradeSidePlayer`/`TradeSide` structs (drop the per-player `Value` field, update the `TradeSide` doc comment):

```go
type TradeSidePlayer struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Position string  `json:"position"`
	PlayerID *string `json:"player_id,omitempty"`
}

// TradeSide groups the assets received by one roster in a trade. TotalValue
// is set only when every player on the side has a persisted valuation
// (sleeper_transactions.trade_values, written by transactioncron at sync
// time — see internal/valuation.ComputeTradeValues); nil otherwise, whether
// because the league's format isn't covered by the model or a player on the
// side hasn't gotten a fresh-enough valuation yet.
type TradeSide struct {
	RosterID   int               `json:"roster_id"`
	Players    []TradeSidePlayer `json:"players"`
	Picks      []string          `json:"picks"`
	TotalValue *float64          `json:"total_value"`
}
```

Delete these definitions entirely (they moved to `internal/valuation` in Task 2): `knownValuationSegments`, `segmentKeyForLeague`, `valuationSnap`, `loadValuationHistory`, `valueAsOf`, `applySideValues`. Keep `formatScoring` and `formatLeagueSize` exactly where they are — they're pure display formatting, unrelated to valuation.

In `GetSleeperTrades`, update the `tradeRow` struct — drop `LeagueType` (only ever used for segment resolution), add `TradeValues`:

```go
	type tradeRow struct {
		SleeperTransactionID string          `gorm:"column:sleeper_transaction_id"`
		SleeperLeagueID      string          `gorm:"column:sleeper_league_id"`
		LeagueName           string          `gorm:"column:league_name"`
		Season               string          `gorm:"column:season"`
		Status               string          `gorm:"column:status"`
		Adds                 json.RawMessage `gorm:"column:adds"`
		DraftPicks           json.RawMessage `gorm:"column:draft_picks"`
		CreatedAtSleeper     int64           `gorm:"column:created_at_sleeper"`
		PPR                  *float64        `gorm:"column:ppr"`
		IsSuperflex          *bool           `gorm:"column:is_superflex"`
		TotalRosters         int             `gorm:"column:total_rosters"`
		TradeValues          json.RawMessage `gorm:"column:trade_values"`
	}
```

Update the `Select(...)` call feeding it (drop `l.league_type`, add `t.trade_values`):

```go
	db := database.DB.Table("sleeper_transactions t").
		Select("t.sleeper_transaction_id, t.sleeper_league_id, l.name as league_name, l.season, t.status, t.adds, t.draft_picks, t.created_at_sleeper, l.ppr, l.is_superflex, l.total_rosters, t.trade_values").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = t.sleeper_league_id").
		Where("t.type = ? AND t.status = ?", "trade", "complete")
```

Delete the whole segment/valuation batching block that currently sits between the player-lookup section and the row-building loop (the block computing `maxCreated`, `segmentPerRow`, `playersBySegment`, `historyBySegment`). Replace the row-building loop's per-row valuation application with a read of the persisted column:

```go
	items := make([]SleeperTradeItem, len(rows))
	for i, r := range rows {
		sides := buildTradeSides(addsPerRow[i], playerLookup, r.DraftPicks)
		if len(r.TradeValues) > 0 {
			var totals map[string]float64
			if err := json.Unmarshal(r.TradeValues, &totals); err == nil {
				for j := range sides {
					if v, ok := totals[strconv.Itoa(sides[j].RosterID)]; ok {
						val := v
						sides[j].TotalValue = &val
					}
				}
			}
		}
		items[i] = SleeperTradeItem{
			ID:         r.SleeperTransactionID,
			LeagueID:   r.SleeperLeagueID,
			LeagueName: r.LeagueName,
			Season:     r.Season,
			Scoring:    formatScoring(r.PPR),
			Superflex:  r.IsSuperflex != nil && *r.IsSuperflex,
			LeagueSize: formatLeagueSize(r.TotalRosters),
			Status:     r.Status,
			Sides:      sides,
			CreatedAt:  r.CreatedAtSleeper,
		}
	}
```

`strconv` should already be imported in this file (used elsewhere, e.g. `parsePagination`) — confirm rather than assume, and add it if missing.

- [ ] **Step 4: Update the frontend type and run all tests**

In `frontend/src/types/models.ts`, remove the now-always-absent per-player field from `TradeSidePlayer` (around line 237-243):

```typescript
export interface TradeSidePlayer {
  id: string;
  name: string;
  position: string;
  player_id?: string;
}
```

(Drop the `value?: number;` line. `TradeSide.total_value` on the next interface is unchanged — still `number | null`.)

Run: `cd backend && go build ./... && go test ./internal/api/handlers/... -v`
Expected: PASS — all `sleeper_test.go` tests, including the new `TestGetSleeperTrades_ReadsPersistedTradeValues`.

Run: `cd frontend && npm run lint`
Expected: no new errors (this is a type-only removal of a field nothing consumes — verified in the design phase that `frontend/src/pages/trades.tsx` never reads `player.value`).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/api/handlers/sleeper.go backend/internal/api/handlers/sleeper_test.go frontend/src/types/models.ts
git commit -m "feat(api): read trade_values from sleeper_transactions instead of computing live"
```

---

### Task 8: Systemd timer cadence (inert prep, per design addendum)

**Files:**
- Modify: `deploy/worker-host/ff-sims-player-valuations.timer`
- Modify: `deploy/worker-host/ff-sims-player-valuations.service`

**Interfaces:** None (deploy-only config; no Go code touches these files).

- [ ] **Step 1: Update the timer**

Edit `deploy/worker-host/ff-sims-player-valuations.timer`:

```ini
[Unit]
Description=Run the ff-sims player valuation replay twice daily, 06:00 and 18:00 UTC

[Timer]
# 06:00 and 18:00 UTC. NOTE: analysis/main.py's default_end() is date-only
# (utc_today(), no time-of-day) — two runs on the same calendar day compute
# an identical window and produce byte-identical output. This cadence bump
# does not improve trade-valuation freshness today; it ships as inert prep
# for a later change to make the replay's end boundary time-aware. See the
# "Update (2026-08-10, plan phase)" note in
# docs/superpowers/specs/2026-08-10-trade-valuation-totals-design.md.
OnCalendar=*-*-* 06,18:00:00 UTC
# No OnBootSec and no Persistent=true, deliberately: enabling or starting this
# timer must not kick off a full replay. The first production run is invoked
# explicitly (systemctl start ff-sims-player-valuations.service) so it can be
# watched. setup.sh's disable_sleep already keeps the host awake for the tick.
Unit=ff-sims-player-valuations.service

[Install]
WantedBy=timers.target
```

- [ ] **Step 2: Update the service description**

In `deploy/worker-host/ff-sims-player-valuations.service`, update only the `Description=` line (everything else — `ExecStart`, `TimeoutStartSec`, env — is unchanged, since neither the replay window nor its per-run duration depends on cadence):

```ini
Description=ff-sims player valuation replay (ppr-sf-10, season 2025), twice daily
```

- [ ] **Step 3: Sanity-check the unit files**

Run: `systemd-analyze verify deploy/worker-host/ff-sims-player-valuations.timer deploy/worker-host/ff-sims-player-valuations.service 2>&1 | grep -v "Failed to prepare filesystem"` (the `{{SERVICE_USER}}`/`{{REPO_DIR}}` template placeholders in the `.service` file mean full verification isn't possible outside the deploy templating step — this just catches outright syntax errors like a malformed `OnCalendar`).

If `systemd-analyze` isn't available on this machine (macOS dev environment), skip this step and instead visually diff against the working `OnCalendar` syntax already used successfully in `ff-sims-discovery.timer`/`ff-sims-transactions.timer` (both already deployed and running) to confirm the `06,18:00:00` comma-list form matches systemd's `OnCalendar` grammar.

- [ ] **Step 4: Commit**

```bash
git add deploy/worker-host/ff-sims-player-valuations.timer deploy/worker-host/ff-sims-player-valuations.service
git commit -m "chore(deploy): bump player-valuations timer to twice-daily (06:00/18:00 UTC)"
```

---

## Final verification

After all 8 tasks:

- [ ] `cd backend && go build ./... && go vet ./... && gofmt -l .` — expect a clean build, no vet warnings, no unformatted files.
- [ ] `cd backend && go test ./...` — expect all tests to pass (Postgres-gated ones will `SKIP` without `TEST_DATABASE_URL`, which is fine).
- [ ] `cd frontend && npm run lint` — expect no new errors.
- [ ] Read back through `docs/superpowers/specs/2026-08-10-trade-valuation-totals-design.md` once more and confirm every section (Architecture, Data model, Write path, Read path, Error handling, Testing, Deployment) has a corresponding completed task above.
