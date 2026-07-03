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
