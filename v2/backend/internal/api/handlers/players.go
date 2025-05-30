package handlers

import (
	"log/slog"
	"net/http"

	"backend/internal/models"

	"github.com/gin-gonic/gin"
)

// GetPlayers returns all players with optional filtering
func GetPlayers(c *gin.Context) {
	position := c.Query("position") // Filter by position if provided
	slog.Info("Fetching players", "position", position)

	// In a real implementation, you would query the database with filters
	players := []models.Player{
		{ID: 1, Name: "Tom Brady", Position: "QB", Team: "TB", FantasyPoints: 289.5},
		{ID: 2, Name: "Derrick Henry", Position: "RB", Team: "TEN", FantasyPoints: 312.8},
		{ID: 3, Name: "Davante Adams", Position: "WR", Team: "LV", FantasyPoints: 274.2},
		// Add more mock data as needed
	}

	c.JSON(http.StatusOK, gin.H{
		"players": players,
	})
}

// GetPlayerByID returns a player by ID
func GetPlayerByID(c *gin.Context) {
	id := c.Param("id")

	slog.Info("Fetching player by ID", "id", id)

	player := models.Player{
		ID:            1,
		Name:          "Tom Brady",
		Position:      "QB",
		Team:          "TB",
		FantasyPoints: 289.5,
		Stats: models.PlayerStats{
			PassingYards:   4500,
			PassingTDs:     35,
			RushingYards:   50,
			RushingTDs:     3,
			Receptions:     0,
			ReceivingYards: 0,
			ReceivingTDs:   0,
		},
	}

	c.JSON(http.StatusOK, player)
}

// GetPlayerStats returns player statistics
func GetPlayerStats(c *gin.Context) {
	// In a real implementation, you would query the database
	// Optionally filter by week, season, etc.
	week := c.DefaultQuery("week", "all")
	season := c.DefaultQuery("season", "2023")

	stats := []models.PlayerGameStats{
		{
			PlayerID:   1,
			PlayerName: "Tom Brady",
			Week:       1,
			Season:     2023,
			GameStats: models.PlayerStats{
				PassingYards:   312,
				PassingTDs:     3,
				RushingYards:   5,
				RushingTDs:     0,
				Receptions:     0,
				ReceivingYards: 0,
				ReceivingTDs:   0,
			},
			FantasyPoints: 25.7,
		},
		// Add more mock data
	}

	c.JSON(http.StatusOK, gin.H{
		"stats": stats,
		"filters": gin.H{
			"week":   week,
			"season": season,
		},
	})
}
