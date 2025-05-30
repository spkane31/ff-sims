package handlers

import (
	"log/slog"
	"net/http"

	"backend/internal/models"

	"github.com/gin-gonic/gin"
)

type GetTeamsResponse struct {
	Teams []TeamResponse `json:"teams"`
}

type TeamResponse struct {
	ID            int        `json:"id"`
	Name          string     `json:"name"`
	OwnerName     string     `json:"owner"`
	TeamRecord    TeamRecord `json:"record"`
	Points        TeamPoints `json:"points"`
	Rank          int        `json:"rank"`
	PlayoffChance float64    `json:"playoffChance"`
}

type TeamRecord struct {
	Wins   int `json:"wins"`
	Losses int `json:"losses"`
	Ties   int `json:"ties"`
}

type TeamPoints struct {
	Scored  float64 `json:"scored"`
	Against float64 `json:"against"`
}

// GetTeams returns all teams
func GetTeams(c *gin.Context) {
	c.JSON(http.StatusOK, GetTeamsResponse{
		Teams: []TeamResponse{
			{ID: 1, Name: "Tua Deez Nuts", OwnerName: "Kyle Burns", Rank: 1, TeamRecord: TeamRecord{Wins: 10, Losses: 3}, PlayoffChance: 100 * 0.95, Points: TeamPoints{Scored: 135, Against: 120}},
			{ID: 2, Name: "Christian Mingle", OwnerName: "Nick Toth", Rank: 2, TeamRecord: TeamRecord{Wins: 9, Losses: 4}, PlayoffChance: 100 * 0.90, Points: TeamPoints{Scored: 135, Against: 120}},
			{ID: 3, Name: "Bake Show", OwnerName: "Connor Brand", Rank: 3, TeamRecord: TeamRecord{Wins: 8, Losses: 5}, PlayoffChance: 100 * 0.85, Points: TeamPoints{Scored: 135, Against: 120}},
			{ID: 4, Name: "Omaha Audibles", OwnerName: "Kevin Dailey", Rank: 4, TeamRecord: TeamRecord{Wins: 7, Losses: 6}, PlayoffChance: 100 * 0.80, Points: TeamPoints{Scored: 135, Against: 120}},
			{ID: 5, Name: "The Nut Dumper", OwnerName: "Sean Kane", Rank: 5, TeamRecord: TeamRecord{Wins: 6, Losses: 7}, PlayoffChance: 100 * 0.75, Points: TeamPoints{Scored: 135, Against: 120}},
			{ID: 6, Name: "Daddy Doepker", OwnerName: "Josh Doepker", Rank: 6, TeamRecord: TeamRecord{Wins: 5, Losses: 8}, PlayoffChance: 100 * 0.70, Points: TeamPoints{Scored: 135, Against: 120}},
			{ID: 7, Name: "Chef Hans", OwnerName: "Mitch Lichtinger", Rank: 7, TeamRecord: TeamRecord{Wins: 4, Losses: 9}, PlayoffChance: 100 * 0.65, Points: TeamPoints{Scored: 135, Against: 120}},
			{ID: 8, Name: "Walker Texas Nutter", OwnerName: "Jack Aldridge", Rank: 8, TeamRecord: TeamRecord{Wins: 3, Losses: 10}, PlayoffChance: 100 * 0.60, Points: TeamPoints{Scored: 135, Against: 120}},
			{ID: 9, Name: "BbwBearcats", OwnerName: "Nick DeHaven", Rank: 9, TeamRecord: TeamRecord{Wins: 2, Losses: 11}, PlayoffChance: 100 * 0.55, Points: TeamPoints{Scored: 135, Against: 120}},
			{ID: 10, Name: "Brock'd Up", OwnerName: "Ethan Moran", Rank: 10, TeamRecord: TeamRecord{Wins: 1, Losses: 12}, PlayoffChance: 100 * 0.50, Points: TeamPoints{Scored: 135, Against: 120}},
		},
	})
}

// GetTeamByID returns a team by its ID
func GetTeamByID(c *gin.Context) {
	id := c.Param("id")

	slog.Info("Fetching team by ID", "id", id)

	team := models.Team{
		ID:        1,
		Name:      "Touchdown Terrors",
		OwnerName: "John Doe",
		Wins:      8,
		Losses:    3,
	}

	c.JSON(http.StatusOK, team)
}

// CreateTeam creates a new team
func CreateTeam(c *gin.Context) {
	var team models.Team
	if err := c.ShouldBindJSON(&team); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// In a real implementation, you would save to database
	team.ID = 3 // Mocked ID assignment

	c.JSON(http.StatusCreated, team)
}

// UpdateTeam updates an existing team
func UpdateTeam(c *gin.Context) {
	id := c.Param("id")

	slog.Info("Updating team", "id", id)

	var team models.Team

	if err := c.ShouldBindJSON(&team); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// In a real implementation, you would update in database
	c.JSON(http.StatusOK, team)
}

// DeleteTeam deletes a team
func DeleteTeam(c *gin.Context) {
	id := c.Param("id")

	slog.Info("Deleting team", "id", id)

	c.Status(http.StatusNoContent)
}
