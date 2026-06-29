package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

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

// TradeSide groups the assets received by one roster in a trade.
type TradeSide struct {
	RosterID int               `json:"roster_id"`
	Players  []TradeSidePlayer `json:"players"`
	Picks    []string          `json:"picks"`
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

// tradePick is the shape of one entry in the draft_picks JSON array on a Sleeper transaction.
type tradePick struct {
	Season  string `json:"season"`
	Round   int    `json:"round"`
	OwnerID int    `json:"owner_id"` // roster receiving the pick
}

// buildTradeSides groups adds (player_id → roster_id) and draft picks into
// per-roster sides. Sides are sorted by roster_id; players within each side
// are sorted by name.
func buildTradeSides(adds map[string]int, players map[string]TradeSidePlayer, rawPicks []byte) []TradeSide {
	sideMap := map[int]*TradeSide{}

	ensureSide := func(rosterID int) *TradeSide {
		if _, ok := sideMap[rosterID]; !ok {
			sideMap[rosterID] = &TradeSide{RosterID: rosterID, Players: []TradeSidePlayer{}, Picks: []string{}}
		}
		return sideMap[rosterID]
	}

	for playerID, rosterID := range adds {
		p, ok := players[playerID]
		if !ok {
			p = TradeSidePlayer{ID: playerID, Name: playerID}
		}
		s := ensureSide(rosterID)
		s.Players = append(s.Players, p)
	}

	if len(rawPicks) > 0 {
		var picks []tradePick
		if err := json.Unmarshal(rawPicks, &picks); err == nil {
			for _, pk := range picks {
				if pk.OwnerID == 0 {
					continue
				}
				label := fmt.Sprintf("%s Round %d pick", pk.Season, pk.Round)
				s := ensureSide(pk.OwnerID)
				s.Picks = append(s.Picks, label)
			}
		}
	}

	rosterIDs := make([]int, 0, len(sideMap))
	for id := range sideMap {
		rosterIDs = append(rosterIDs, id)
	}
	sort.Ints(rosterIDs)
	sides := make([]TradeSide, len(rosterIDs))
	for i, rid := range rosterIDs {
		s := sideMap[rid]
		sort.Slice(s.Players, func(a, b int) bool { return s.Players[a].Name < s.Players[b].Name })
		sort.Strings(s.Picks)
		sides[i] = *s
	}
	return sides
}

// GetSleeperStats returns counts of leagues, trades, and completed drafts in the Sleeper DB.
func GetSleeperStats(c *gin.Context) {
	var leagueCount, tradeCount, draftCount int64

	ctx, cleanup := context.WithTimeout(context.Background(), 20*time.Second)
	defer cleanup()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return database.DB.WithContext(ctx).Model(&models.SleeperLeague{}).
			Where("last_fetched_at IS NOT NULL").
			Count(&leagueCount).Error
	})

	g.Go(func() error {
		return database.DB.WithContext(ctx).Model(&models.SleeperTransaction{}).
			Where("type = ? AND status = ?", "trade", "complete").
			Count(&tradeCount).Error
	})

	g.Go(func() error {
		return database.DB.WithContext(ctx).Model(&models.SleeperDraft{}).
			Where("status = ?", "complete").
			Count(&draftCount).Error
	})

	if err := g.Wait(); err != nil {
		if ctx.Err() != nil {
			c.JSON(http.StatusRequestTimeout, map[string]string{"msg": "request timed out"})
			return
		}
		c.JSON(http.StatusInternalServerError, map[string]string{"msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, SleeperStatsResponse{
		LeagueCount: leagueCount,
		TradeCount:  tradeCount,
		DraftCount:  draftCount,
	})
}

// applyLeagueFilters appends league-level filter conditions to a GORM query.
// leagueAlias is the SQL alias used for sleeper_leagues (e.g. "l").
func applyLeagueFilters(db *gorm.DB, c *gin.Context, leagueAlias string) *gorm.DB {
	if v := c.Query("league_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			db = db.Where(leagueAlias+".total_rosters = ?", n)
		}
	}
	if v := c.Query("scoring_format"); v != "" {
		switch v {
		case "standard":
			db = db.Where(leagueAlias+".ppr = ?", 0)
		case "half_ppr":
			db = db.Where(leagueAlias+".ppr = ?", 0.5)
		case "ppr":
			db = db.Where(leagueAlias+".ppr = ?", 1)
		}
	}
	if v := c.Query("draft_type"); v != "" {
		db = db.Where(leagueAlias+".draft_type = ?", v)
	}
	if v := c.Query("league_type"); v != "" {
		db = db.Where(leagueAlias+".league_type = ?", v)
	}
	return db
}

// GetSleeperTrades returns a paginated list of Sleeper trades ordered by recency,
// with each trade's adds grouped by roster into named sides.
// Supports query filters: league_size (int), scoring_format (standard|half_ppr|ppr), draft_type (snake|auction|linear), league_type (redraft|keeper|dynasty).
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
		DraftPicks           json.RawMessage `gorm:"column:draft_picks"`
		CreatedAtSleeper     int64           `gorm:"column:created_at_sleeper"`
	}

	var rows []tradeRow
	var total int64

	db := database.DB.Table("sleeper_transactions t").
		Select("t.sleeper_transaction_id, t.sleeper_league_id, l.name as league_name, l.season, t.status, t.adds, t.draft_picks, t.created_at_sleeper").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = t.sleeper_league_id").
		Where("t.type = ? AND t.status = ?", "trade", "complete")
	db = applyLeagueFilters(db, c, "l")

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
			Sides:      buildTradeSides(addsPerRow[i], playerLookup, r.DraftPicks),
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

// SleeperTransactionItem is a single row in the transactions list.
type SleeperTransactionItem struct {
	ID          string `json:"id"`
	LeagueID    string `json:"league_id"`
	LeagueName  string `json:"league_name"`
	Season      string `json:"season"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	CreatedAt   int64  `json:"created_at"`
	PlayerCount int    `json:"player_count"`
}

// SleeperTransactionsResponse is the paginated response for GET /api/v1/sleeper/transactions.
type SleeperTransactionsResponse struct {
	Transactions []SleeperTransactionItem `json:"transactions"`
	Total        int64                    `json:"total"`
	Page         int                      `json:"page"`
	Limit        int                      `json:"limit"`
	TotalPages   int                      `json:"total_pages"`
}

// GetSleeperTransactions returns a paginated list of all Sleeper transactions.
// Supports query filters: type (trade|waiver|free_agent), league_size, scoring_format, draft_type, league_type (redraft|keeper|dynasty).
func GetSleeperTransactions(c *gin.Context) {
	page, limit := parsePagination(c)
	offset := (page - 1) * limit

	type txRow struct {
		SleeperTransactionID string          `gorm:"column:sleeper_transaction_id"`
		SleeperLeagueID      string          `gorm:"column:sleeper_league_id"`
		LeagueName           string          `gorm:"column:league_name"`
		Season               string          `gorm:"column:season"`
		Type                 string          `gorm:"column:type"`
		Status               string          `gorm:"column:status"`
		CreatedAtSleeper     int64           `gorm:"column:created_at_sleeper"`
		Adds                 json.RawMessage `gorm:"column:adds"`
	}

	var rows []txRow
	var total int64

	db := database.DB.Table("sleeper_transactions t").
		Select("t.sleeper_transaction_id, t.sleeper_league_id, l.name as league_name, l.season, t.type, t.status, t.created_at_sleeper, t.adds").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = t.sleeper_league_id").
		Where("t.status = ?", "complete")

	if txType := c.Query("type"); txType != "" {
		db = db.Where("t.type = ?", txType)
	}
	db = applyLeagueFilters(db, c, "l")

	db.Count(&total)
	db.Order("t.created_at_sleeper DESC").Limit(limit).Offset(offset).Scan(&rows)

	items := make([]SleeperTransactionItem, len(rows))
	for i, r := range rows {
		var adds map[string]int
		if len(r.Adds) > 0 {
			_ = json.Unmarshal(r.Adds, &adds)
		}
		items[i] = SleeperTransactionItem{
			ID:          r.SleeperTransactionID,
			LeagueID:    r.SleeperLeagueID,
			LeagueName:  r.LeagueName,
			Season:      r.Season,
			Type:        r.Type,
			Status:      r.Status,
			CreatedAt:   r.CreatedAtSleeper,
			PlayerCount: len(adds),
		}
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	c.JSON(http.StatusOK, SleeperTransactionsResponse{
		Transactions: items,
		Total:        total,
		Page:         page,
		Limit:        limit,
		TotalPages:   totalPages,
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
