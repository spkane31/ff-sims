package handlers

import (
	"backend/internal/database"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type GetLeagueYearsResponse struct {
	Years []uint `json:"years"`
}

// GetLeagueYears returns all years that a league has been active based on matchup data
func GetLeagueYears(c *gin.Context) {
	// Get league ID parameter, default to 345674
	leagueIDStr := c.DefaultQuery("league_id", "345674")
	leagueID, err := strconv.Atoi(leagueIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid league_id parameter"})
		return
	}

	// Query distinct years from matchups table
	var years []uint
	err = database.DB.Table("matchups").
		Select("DISTINCT year").
		Where("league_id = ?", leagueID).
		Order("year DESC").
		Pluck("year", &years).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch league years"})
		return
	}

	c.JSON(http.StatusOK, GetLeagueYearsResponse{
		Years: years,
	})
}