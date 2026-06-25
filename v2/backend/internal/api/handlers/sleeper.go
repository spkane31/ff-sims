package handlers

import (
	"encoding/json"
	"math"
	"net/http"
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

// SleeperTradeItem is a single row in the trades list.
type SleeperTradeItem struct {
	ID         string          `json:"id"`
	LeagueID   string          `json:"league_id"`
	LeagueName string          `json:"league_name"`
	Season     string          `json:"season"`
	Status     string          `json:"status"`
	Adds       json.RawMessage `json:"adds"`
	Drops      json.RawMessage `json:"drops"`
	CreatedAt  int64           `json:"created_at"`
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

// GetSleeperTrades returns a paginated list of Sleeper trades ordered by recency.
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
		Drops                json.RawMessage `gorm:"column:drops"`
		CreatedAtSleeper     int64           `gorm:"column:created_at_sleeper"`
	}

	var rows []tradeRow
	var total int64

	db := database.DB.Table("sleeper_transactions t").
		Select("t.sleeper_transaction_id, t.sleeper_league_id, l.name as league_name, l.season, t.status, t.adds, t.drops, t.created_at_sleeper").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = t.sleeper_league_id").
		Where("t.type = ? AND t.status = ?", "trade", "complete")

	db.Count(&total)
	db.Order("t.created_at_sleeper DESC").Limit(limit).Offset(offset).Scan(&rows)

	items := make([]SleeperTradeItem, len(rows))
	for i, r := range rows {
		items[i] = SleeperTradeItem{
			ID:         r.SleeperTransactionID,
			LeagueID:   r.SleeperLeagueID,
			LeagueName: r.LeagueName,
			Season:     r.Season,
			Status:     r.Status,
			Adds:       r.Adds,
			Drops:      r.Drops,
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
