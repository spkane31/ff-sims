package handlers

import (
	"backend/internal/database"
	"backend/internal/models"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type GetTransactionsResponse struct {
	Transactions []Transaction `json:"transactions"`
}

type TransactionType string

const (
	TransactionTypeDraft  TransactionType = "draft"
	TransactionTypeTrade  TransactionType = "trade"
	TransactionTypeWaiver TransactionType = "waiver"
)

type Transaction struct {
	ID          string              `json:"id"`
	Date        string              `json:"date"`
	Type        TransactionType     `json:"type"`
	Description string              `json:"description"`
	Teams       []string            `json:"teams"`
	Players     []PlayerTransaction `json:"players"`
}

type PlayerTransaction struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Position string  `json:"position"`
	Team     string  `json:"team"`
	Points   float64 `json:"points,omitempty"`
}

type GetDraftPicksResponse struct {
	DraftPicks []DraftPickResponse `json:"draft_picks"`
}

func GetDraftPicks(c *gin.Context) {
	// Get year parameter, default to 2024
	yearStr := c.DefaultQuery("year", "2024")
	year, err := strconv.Atoi(yearStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid year parameter"})
		return
	}

	// Get league ID parameter, default to 345674
	leagueIDStr := c.DefaultQuery("league_id", "345674")
	leagueID, err := strconv.Atoi(leagueIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid league_id parameter"})
		return
	}

	// Fetch draft selections from database
	draftSelections, err := models.GetLeagueDraftSelections(database.DB, uint(leagueID), uint(year))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch draft selections"})
		return
	}

	// Transform to response format
	var draftPicks []DraftPickResponse
	for _, selection := range draftSelections {
		teamOwner := ""
		teamID := 0
		if selection.Team != nil {
			teamOwner = selection.Team.Owner
			teamID = int(selection.Team.ESPNID)
		}

		draftPicks = append(draftPicks, DraftPickResponse{
			PlayerID: fmt.Sprintf("%d", selection.PlayerID),
			Round:    int(selection.Round),
			Pick:     int(selection.Pick),
			Player:   selection.PlayerName,
			Position: selection.PlayerPosition,
			TeamID:   teamID,
			Owner:    teamOwner,
			Year:     int(selection.Year),
		})
	}

	c.JSON(http.StatusOK, GetDraftPicksResponse{
		DraftPicks: draftPicks,
	})
}

func GetTransactions(c *gin.Context) {
	c.JSON(200, GetTransactionsResponse{
		Transactions: []Transaction{
			{
				ID:          "1",
				Date:        "Aug 25, 2024",
				Type:        TransactionTypeDraft,
				Description: "Team A drafted Cooper Kupp with the 8th overall pick.",
				Teams:       []string{"Team A"},
				Players: []PlayerTransaction{
					{
						ID:       "1",
						Name:     "Cooper Kupp",
						Position: "WR",
						Team:     "Los Angeles Rams",
						Points:   256.5,
					},
				},
			},
		},
	})
}
