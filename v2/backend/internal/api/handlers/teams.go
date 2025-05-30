package handlers

import (
	"log/slog"
	"net/http"

	"backend/internal/models"

	"github.com/gin-gonic/gin"
)

// GetTeams returns all teams
func GetTeams(c *gin.Context) {
	teams := []models.Team{
		{ID: 1, Name: "Touchdown Terrors", OwnerName: "John Doe", Wins: 8, Losses: 3},
		{ID: 2, Name: "Gridiron Giants", OwnerName: "Jane Smith", Wins: 6, Losses: 5},
		// Add more mock data as needed
	}

	c.JSON(http.StatusOK, gin.H{
		"teams": teams,
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
