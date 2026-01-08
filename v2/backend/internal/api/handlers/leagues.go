package handlers

import (
	"backend/internal/api/middleware"
	"backend/internal/database"
	"net/http"

	"github.com/gin-gonic/gin"
)

type GetLeaguesRequest struct {
	Limit  int `form:"limit"`
	Offset int `form:"offset"`
}

type GetLeaguesResponse struct {
	Leagues []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"leagues"`
}

func GetLeagues(c *gin.Context) {
	// Get limit and offset query parameters
	var req GetLeaguesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid query parameters"})
		return
	}

	// Placeholder response
	resp := GetLeaguesResponse{
		Leagues: []struct {
			ID   string "json:\"id\""
			Name string "json:\"name\""
		}{},
	}

	err := database.DB.Table("leagues").
		Select("id, name").
		Order("id DESC").
		Scan(&resp.Leagues).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch league years"})
		return
	}

	c.JSON(http.StatusOK, resp)

}

type GetLeagueYearsResponse struct {
	Years []uint `json:"years"`
}

// GetLeagueYears returns all years that a league has been active based on matchup data
func GetLeagueYears(c *gin.Context) {
	leagueID := middleware.GetLeagueID(c)

	// Query distinct years from matchups table
	var years []uint
	err := database.DB.Table("matchups").
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
