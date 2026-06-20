package handlers

import (
	"backend/internal/database"
	"backend/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type LeagueResponse struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Platform    string `json:"platform"`
	ExternalID  string `json:"external_id"`
	CurrentWeek int    `json:"current_week"`
	TotalWeeks  int    `json:"total_weeks"`
}

type GetLeaguesResponse struct {
	Leagues []LeagueResponse `json:"leagues"`
}

type GetLeagueYearsResponse struct {
	Years []uint `json:"years"`
}

func toLeagueResponse(l models.League) LeagueResponse {
	return LeagueResponse{
		ID:          l.ID,
		Name:        l.Name,
		Platform:    l.Platform,
		ExternalID:  l.ExternalID,
		CurrentWeek: l.CurrentWeek,
		TotalWeeks:  l.TotalWeeks,
	}
}

// GetLeagues returns all leagues ordered by ID.
func GetLeagues(c *gin.Context) {
	var leagues []models.League
	if err := database.DB.Order("id asc").Find(&leagues).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch leagues"})
		return
	}
	resp := GetLeaguesResponse{Leagues: make([]LeagueResponse, len(leagues))}
	for i, l := range leagues {
		resp.Leagues[i] = toLeagueResponse(l)
	}
	c.JSON(http.StatusOK, resp)
}

// GetLeague returns a single league by internal ID.
func GetLeague(c *gin.Context) {
	leagueID, ok := parseLeagueID(c)
	if !ok {
		return
	}
	var league models.League
	if err := database.DB.First(&league, leagueID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "league not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch league"})
		return
	}
	c.JSON(http.StatusOK, toLeagueResponse(league))
}

// GetLeagueYears returns all years with matchup data for a league.
func GetLeagueYears(c *gin.Context) {
	leagueID, ok := parseLeagueID(c)
	if !ok {
		return
	}
	var years []uint
	err := database.DB.Table("matchups").
		Select("DISTINCT year").
		Where("league_id = ?", leagueID).
		Order("year DESC").
		Pluck("year", &years).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch league years"})
		return
	}
	c.JSON(http.StatusOK, GetLeagueYearsResponse{Years: years})
}
