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
