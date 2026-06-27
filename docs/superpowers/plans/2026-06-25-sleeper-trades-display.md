# Sleeper Trades Display — Per-Roster Player Names

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the broken "Players Added / Players Dropped" columns on `/sleeper/trades` with "Side A / Side B" columns showing the player names (and positions) each roster received.

**Architecture:** Backend enriches raw `adds` JSONB into typed per-roster sides by batch-querying `sleeper_players`; a pure `buildTradeSides` helper is extracted so it can be unit-tested independently of Gin and the DB. Frontend removes the broken `playerList()` helper and renders the new `sides` field directly.

**Tech Stack:** Go 1.25, GORM, Gin, Next.js/TypeScript, SQLite in-memory for tests.

## Global Constraints

- Work on a new issue branch off `feature/multi-league`; PR targets `feature/multi-league`.
- All Go code lives under `v2/backend/`; all frontend code under `v2/frontend/`.
- No new dependencies — `sort` is stdlib, already available.
- Players absent from `sleeper_players` fall back to `name: player_id, position: ""`.

---

### Task 1: Backend — new types, `buildTradeSides` helper, updated handler

**Files:**
- Modify: `v2/backend/internal/api/handlers/sleeper.go`
- Create: `v2/backend/internal/api/handlers/sleeper_test.go`

**Interfaces:**
- Produces: `TradeSidePlayer{ID, Name, Position}`, `TradeSide{RosterID, Players}`, updated `SleeperTradeItem.Sides []TradeSide` (removes `Adds`/`Drops`).
- Consumed by: Task 2 (frontend TypeScript types mirror this shape).

- [ ] **Step 1: Write the failing tests**

Create `v2/backend/internal/api/handlers/sleeper_test.go`:

```go
package handlers

import (
	"testing"
)

func TestBuildTradeSides_TwoRosters(t *testing.T) {
	adds := map[string]int{
		"6797": 7,
		"8146": 7,
		"6904": 8,
	}
	players := map[string]TradeSidePlayer{
		"6797": {ID: "6797", Name: "Justin Jefferson", Position: "WR"},
		"8146": {ID: "8146", Name: "Davante Adams", Position: "WR"},
		"6904": {ID: "6904", Name: "Travis Kelce", Position: "TE"},
	}

	sides := buildTradeSides(adds, players)

	if len(sides) != 2 {
		t.Fatalf("expected 2 sides, got %d", len(sides))
	}
	if sides[0].RosterID != 7 {
		t.Errorf("expected first side roster_id=7, got %d", sides[0].RosterID)
	}
	if len(sides[0].Players) != 2 {
		t.Errorf("expected 2 players on side 7, got %d", len(sides[0].Players))
	}
	if sides[1].RosterID != 8 {
		t.Errorf("expected second side roster_id=8, got %d", sides[1].RosterID)
	}
	if len(sides[1].Players) != 1 {
		t.Errorf("expected 1 player on side 8, got %d", len(sides[1].Players))
	}
}

func TestBuildTradeSides_MissingPlayer(t *testing.T) {
	adds := map[string]int{"9999": 3}
	players := map[string]TradeSidePlayer{}

	sides := buildTradeSides(adds, players)

	if len(sides) != 1 {
		t.Fatalf("expected 1 side, got %d", len(sides))
	}
	if sides[0].Players[0].ID != "9999" {
		t.Errorf("expected fallback ID '9999', got %q", sides[0].Players[0].ID)
	}
	if sides[0].Players[0].Name != "9999" {
		t.Errorf("expected fallback Name '9999', got %q", sides[0].Players[0].Name)
	}
}

func TestBuildTradeSides_EmptyAdds(t *testing.T) {
	sides := buildTradeSides(map[string]int{}, map[string]TradeSidePlayer{})
	if len(sides) != 0 {
		t.Fatalf("expected 0 sides for empty adds, got %d", len(sides))
	}
}

func TestBuildTradeSides_SortedByRosterID(t *testing.T) {
	adds := map[string]int{"p1": 10, "p2": 2}
	players := map[string]TradeSidePlayer{}

	sides := buildTradeSides(adds, players)

	if sides[0].RosterID != 2 || sides[1].RosterID != 10 {
		t.Errorf("expected sides sorted by roster_id asc, got %d, %d", sides[0].RosterID, sides[1].RosterID)
	}
}
```

- [ ] **Step 2: Run tests — expect compile failure (types not defined yet)**

```bash
cd v2/backend && go test ./internal/api/handlers/... -v -run TestBuildTradeSides
```

Expected: `undefined: TradeSidePlayer` / `undefined: buildTradeSides`

- [ ] **Step 3: Update `sleeper.go` — new types, helper, updated handler**

Replace the contents of `v2/backend/internal/api/handlers/sleeper.go` with:

```go
package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"

	"backend/internal/database"
	"backend/internal/models"
)

// SleeperStatsResponse is the response for GET /api/v1/sleeper/stats.
type SleeperStatsResponse struct {
	LeagueCount int64 `json:"league_count"`
	TradeCount  int64 `json:"trade_count"`
	DraftCount  int64 `json:"draft_count"`
}

// TradeSidePlayer is a single player in one side of a trade.
type TradeSidePlayer struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Position string `json:"position"`
}

// TradeSide groups the players received by one roster in a trade.
type TradeSide struct {
	RosterID int               `json:"roster_id"`
	Players  []TradeSidePlayer `json:"players"`
}

// SleeperTradeItem is a single row in the trades list.
type SleeperTradeItem struct {
	ID         string      `json:"id"`
	LeagueID   string      `json:"league_id"`
	LeagueName string      `json:"league_name"`
	Season     string      `json:"season"`
	Status     string      `json:"status"`
	Sides      []TradeSide `json:"sides"`
	CreatedAt  int64       `json:"created_at"`
}

// SleeperTradesResponse is the paginated response for GET /api/v1/sleeper/trades.
type SleeperTradesResponse struct {
	Trades     []SleeperTradeItem `json:"trades"`
	Total      int64              `json:"total"`
	Page       int                `json:"page"`
	Limit      int                `json:"limit"`
	TotalPages int                `json:"total_pages"`
}

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

// buildTradeSides groups the adds map (player_id → roster_id) into per-roster
// slices with player names resolved from the lookup. Players missing from the
// lookup fall back to name=player_id, position="". Sides are sorted by
// roster_id ascending; players within each side are sorted by name ascending.
func buildTradeSides(adds map[string]int, players map[string]TradeSidePlayer) []TradeSide {
	sideMap := map[int][]TradeSidePlayer{}
	for playerID, rosterID := range adds {
		p, ok := players[playerID]
		if !ok {
			p = TradeSidePlayer{ID: playerID, Name: playerID}
		}
		sideMap[rosterID] = append(sideMap[rosterID], p)
	}
	rosterIDs := make([]int, 0, len(sideMap))
	for id := range sideMap {
		rosterIDs = append(rosterIDs, id)
	}
	sort.Ints(rosterIDs)
	sides := make([]TradeSide, len(rosterIDs))
	for i, rid := range rosterIDs {
		ps := sideMap[rid]
		sort.Slice(ps, func(a, b int) bool { return ps[a].Name < ps[b].Name })
		sides[i] = TradeSide{RosterID: rid, Players: ps}
	}
	return sides
}

// GetSleeperStats returns counts of leagues, trades, and completed drafts in the Sleeper DB.
func GetSleeperStats(c *gin.Context) {
	var leagueCount, tradeCount, draftCount int64

	database.DB.Model(&models.SleeperLeague{}).
		Where("last_fetched_at IS NOT NULL").
		Count(&leagueCount)

	database.DB.Model(&models.SleeperTransaction{}).
		Where("type = ? AND status = ?", "trade", "complete").
		Count(&tradeCount)

	database.DB.Model(&models.SleeperDraft{}).
		Where("status = ?", "complete").
		Count(&draftCount)

	c.JSON(http.StatusOK, SleeperStatsResponse{
		LeagueCount: leagueCount,
		TradeCount:  tradeCount,
		DraftCount:  draftCount,
	})
}

// GetSleeperTrades returns a paginated list of Sleeper trades ordered by recency,
// with each trade's adds grouped by roster into named sides.
func GetSleeperTrades(c *gin.Context) {
	page, limit := parsePagination(c)
	offset := (page - 1) * limit

	type tradeRow struct {
		SleeperTransactionID string          `gorm:"column:sleeper_transaction_id"`
		SleeperLeagueID      string          `gorm:"column:sleeper_league_id"`
		LeagueName           string          `gorm:"column:league_name"`
		Season               string          `gorm:"column:season"`
		Status               string          `gorm:"column:status"`
		Adds                 json.RawMessage `gorm:"column:adds"`
		CreatedAtSleeper     int64           `gorm:"column:created_at_sleeper"`
	}

	var rows []tradeRow
	var total int64

	db := database.DB.Table("sleeper_transactions t").
		Select("t.sleeper_transaction_id, t.sleeper_league_id, l.name as league_name, l.season, t.status, t.adds, t.created_at_sleeper").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = t.sleeper_league_id").
		Where("t.type = ? AND t.status = ?", "trade", "complete")

	db.Count(&total)
	db.Order("t.created_at_sleeper DESC").Limit(limit).Offset(offset).Scan(&rows)

	// Decode adds and collect all unique player IDs on this page.
	addsPerRow := make([]map[string]int, len(rows))
	playerIDSet := map[string]struct{}{}
	for i, r := range rows {
		var adds map[string]int
		if len(r.Adds) > 0 {
			_ = json.Unmarshal(r.Adds, &adds)
		}
		addsPerRow[i] = adds
		for pid := range adds {
			playerIDSet[pid] = struct{}{}
		}
	}

	// Batch-fetch player names for all players on this page.
	playerLookup := map[string]TradeSidePlayer{}
	if len(playerIDSet) > 0 {
		ids := make([]string, 0, len(playerIDSet))
		for id := range playerIDSet {
			ids = append(ids, id)
		}
		var players []models.SleeperPlayer
		database.DB.Where("sleeper_player_id IN ?", ids).Find(&players)
		for _, p := range players {
			playerLookup[p.SleeperPlayerID] = TradeSidePlayer{
				ID:       p.SleeperPlayerID,
				Name:     p.FullName,
				Position: p.Position,
			}
		}
	}

	items := make([]SleeperTradeItem, len(rows))
	for i, r := range rows {
		items[i] = SleeperTradeItem{
			ID:         r.SleeperTransactionID,
			LeagueID:   r.SleeperLeagueID,
			LeagueName: r.LeagueName,
			Season:     r.Season,
			Status:     r.Status,
			Sides:      buildTradeSides(addsPerRow[i], playerLookup),
			CreatedAt:  r.CreatedAtSleeper,
		}
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	c.JSON(http.StatusOK, SleeperTradesResponse{
		Trades:     items,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

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

	database.DB.Table("sleeper_drafts").Where("status = ?", "complete").Count(&total)
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

func parsePagination(c *gin.Context) (page, limit int) {
	page = 1
	limit = 25
	if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && p > 0 {
		page = p
	}
	if l, err := strconv.Atoi(c.DefaultQuery("limit", "25")); err == nil && l > 0 && l <= 100 {
		limit = l
	}
	return
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
cd v2/backend && go test ./internal/api/handlers/... -v -run TestBuildTradeSides
```

Expected output:
```
--- PASS: TestBuildTradeSides_TwoRosters
--- PASS: TestBuildTradeSides_MissingPlayer
--- PASS: TestBuildTradeSides_EmptyAdds
--- PASS: TestBuildTradeSides_SortedByRosterID
PASS
```

- [ ] **Step 5: Verify full build**

```bash
cd v2/backend && go build ./...
```

Expected: no output (clean build).

- [ ] **Step 6: Commit**

```bash
git add v2/backend/internal/api/handlers/sleeper.go \
        v2/backend/internal/api/handlers/sleeper_test.go
git commit -m "feat(handlers): enrich trade sides with player names from sleeper_players"
```

---

### Task 2: Frontend — updated types and table columns

**Files:**
- Modify: `v2/frontend/src/types/models.ts` (lines 217–226 — the `SleeperTrade` interface)
- Modify: `v2/frontend/src/pages/sleeper/trades.tsx`

**Interfaces:**
- Consumes: `sides: { roster_id: number; players: { id, name, position } }[]` produced by Task 1.

- [ ] **Step 1: Update `SleeperTrade` in `models.ts`**

Replace the `SleeperTrade` interface (currently lines 217–226) with:

```typescript
export interface TradeSidePlayer {
  id: string;
  name: string;
  position: string;
}

export interface TradeSide {
  roster_id: number;
  players: TradeSidePlayer[];
}

export interface SleeperTrade {
  id: string;
  league_id: string;
  league_name: string;
  season: string;
  status: string;
  sides: TradeSide[];
  created_at: number;
}
```

- [ ] **Step 2: Update `trades.tsx`**

Replace the full contents of `v2/frontend/src/pages/sleeper/trades.tsx` with:

```tsx
import { useState } from "react";
import Layout from "../../components/Layout";
import { useSleeperTrades } from "../../hooks/useSleeperData";
import { SleeperTrade } from "../../types/models";

const LIMIT = 25;

function formatDate(unixMs: number): string {
  if (!unixMs) return "—";
  return new Date(unixMs).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function sideLabel(side: SleeperTrade["sides"][number] | undefined): string {
  if (!side || side.players.length === 0) return "—";
  return side.players
    .map((p) => (p.position ? `${p.name} (${p.position})` : p.name))
    .join(", ");
}

export default function SleeperTradesPage() {
  const [page, setPage] = useState(1);
  const { items, total, totalPages, isLoading, error } = useSleeperTrades(page, LIMIT);

  return (
    <Layout>
      <div className="space-y-6">
        <div>
          <h1 className="text-3xl font-bold text-blue-600">Sleeper Trades</h1>
          <p className="text-gray-600 dark:text-gray-300 mt-1">
            {isLoading ? "Loading…" : `${total.toLocaleString()} completed trades`}
          </p>
        </div>

        {error && (
          <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4 text-red-700 dark:text-red-300">
            Failed to load trades: {error.message}
          </div>
        )}

        <div className="overflow-x-auto bg-white dark:bg-gray-800 rounded-lg shadow">
          <table className="w-full">
            <thead className="bg-gray-50 dark:bg-gray-700">
              <tr>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Date</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">League</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Season</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Side A</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300">Side B</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-600">
              {isLoading ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                    <div className="flex justify-center items-center space-x-2">
                      <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                      <span>Loading trades…</span>
                    </div>
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                    No trades found. Data may still be syncing.
                  </td>
                </tr>
              ) : (
                items.map((trade) => (
                  <tr key={trade.id} className="hover:bg-gray-50 dark:hover:bg-gray-700">
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300 whitespace-nowrap">
                      {formatDate(trade.created_at)}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-900 dark:text-gray-100 max-w-xs truncate">
                      {trade.league_name}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{trade.season}</td>
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300 max-w-xs truncate">
                      {sideLabel(trade.sides?.[0])}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-300 max-w-xs truncate">
                      {sideLabel(trade.sides?.[1])}
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
              onClick={() => setPage((p) => p - 1)}
              disabled={page <= 1 || isLoading}
            >
              Previous
            </button>
            <span className="text-sm text-gray-600 dark:text-gray-300">
              Page {page} of {totalPages}
            </span>
            <button
              className="px-4 py-2 text-sm bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-md disabled:opacity-40 hover:bg-gray-50 dark:hover:bg-gray-600 transition-colors"
              onClick={() => setPage((p) => p + 1)}
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

- [ ] **Step 3: TypeScript build check**

```bash
cd v2/frontend && npm run build 2>&1 | tail -20
```

Expected: build completes with no TypeScript errors. (Next.js may emit warnings about image optimisation or similar — those are fine; type errors are not.)

- [ ] **Step 4: Commit**

```bash
git add v2/frontend/src/types/models.ts \
        v2/frontend/src/pages/sleeper/trades.tsx
git commit -m "feat(frontend): display trade sides with player names on /sleeper/trades"
```
