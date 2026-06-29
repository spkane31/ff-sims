package handlers

import (
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"backend/internal/database"
	"backend/internal/models"
)

type TransactionType string

const (
	TransactionTypeDraft  TransactionType = "draft"
	TransactionTypeTrade  TransactionType = "trade"
	TransactionTypeWaiver TransactionType = "waiver"
)

type Transaction struct {
	ID      string              `json:"id"`
	Date    string              `json:"date"`
	Type    TransactionType     `json:"type"`
	Teams   []string            `json:"teams"`
	Players []PlayerTransaction `json:"players"`
}

type PlayerTransaction struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Position string  `json:"position"`
	Team     string  `json:"team"`
	Points   float64 `json:"points,omitempty"`
}

type GetTransactionsPagedResponse struct {
	Transactions []Transaction `json:"transactions"`
	Total        int64         `json:"total"`
	Page         int           `json:"page"`
	Limit        int           `json:"limit"`
	TotalPages   int           `json:"total_pages"`
}

type GetDraftPicksPagedResponse struct {
	DraftPicks []DraftPickResponse `json:"draft_picks"`
	Total      int64               `json:"total"`
	Page       int                 `json:"page"`
	Limit      int                 `json:"limit"`
	TotalPages int                 `json:"total_pages"`
}

func txTypeFromModel(t string) TransactionType {
	switch t {
	case "TRADED":
		return TransactionTypeTrade
	case "ADDED", "DROPPED":
		return TransactionTypeWaiver
	default:
		return TransactionTypeDraft
	}
}

func GetDraftPicks(c *gin.Context) {
	leagueID, ok := parseLeagueID(c)
	if !ok {
		return
	}

	yearStr := c.DefaultQuery("year", "2024")
	year, err := strconv.Atoi(yearStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid year parameter"})
		return
	}

	page, limit := parsePagination(c)
	offset := (page - 1) * limit

	var total int64
	database.DB.Model(&models.DraftSelection{}).
		Where("league_id = ? AND year = ?", leagueID, uint(year)).
		Count(&total)

	var draftSelections []models.DraftSelection
	if err := database.DB.
		Where("league_id = ? AND year = ?", leagueID, uint(year)).
		Order("round asc, pick asc").
		Preload("Team").
		Preload("Player").
		Limit(limit).Offset(offset).
		Find(&draftSelections).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch draft selections"})
		return
	}

	draftPicks := make([]DraftPickResponse, 0, len(draftSelections))
	for _, selection := range draftSelections {
		teamOwner := ""
		teamID := 0
		if selection.Team != nil {
			teamOwner = selection.Team.Owner
			teamID = int(selection.Team.ESPNID)
		}
		draftPicks = append(draftPicks, DraftPickResponse{
			PlayerID: strconv.FormatUint(uint64(selection.PlayerID), 10),
			Round:    int(selection.Round),
			Pick:     int(selection.Pick),
			Player:   selection.PlayerName,
			Position: selection.PlayerPosition,
			TeamID:   teamID,
			Owner:    teamOwner,
			Year:     int(selection.Year),
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	c.JSON(http.StatusOK, GetDraftPicksPagedResponse{
		DraftPicks: draftPicks,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

func GetTransactions(c *gin.Context) {
	page, limit := parsePagination(c)
	offset := (page - 1) * limit

	db := database.DB.Model(&models.Transaction{})

	// leagueId path param is optional; omit filter when not present or not numeric.
	if raw := c.Param("leagueId"); raw != "" {
		if id, err := strconv.ParseUint(raw, 10, 32); err == nil && id > 0 {
			db = db.Where("league_id = ?", id)
		}
	}

	if year := c.Query("year"); year != "" {
		if y, err := strconv.Atoi(year); err == nil {
			db = db.Where("year = ?", y)
		}
	}

	var total int64
	db.Count(&total)

	var txs []models.Transaction
	db.Preload("Team").Preload("Player").
		Order("date desc").
		Limit(limit).Offset(offset).
		Find(&txs)

	items := make([]Transaction, len(txs))
	for i, tx := range txs {
		teamName := ""
		if tx.Team != nil {
			teamName = tx.Team.Owner
		}
		playerPos := ""
		playerTeam := ""
		if tx.Player != nil {
			playerPos = tx.Player.Position
			playerTeam = tx.Player.Team
		}
		items[i] = Transaction{
			ID:   strconv.FormatUint(uint64(tx.ID), 10),
			Date: tx.Date.Format("Jan 02, 2006"),
			Type: txTypeFromModel(tx.TransactionType),
			Teams: []string{teamName},
			Players: []PlayerTransaction{{
				ID:       strconv.FormatUint(uint64(tx.PlayerID), 10),
				Name:     tx.PlayerName,
				Position: playerPos,
				Team:     playerTeam,
			}},
		}
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	c.JSON(http.StatusOK, GetTransactionsPagedResponse{
		Transactions: items,
		Total:        total,
		Page:         page,
		Limit:        limit,
		TotalPages:   totalPages,
	})
}
