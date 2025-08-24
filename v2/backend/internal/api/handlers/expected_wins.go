package handlers

import (
	"backend/internal/database"
	"backend/internal/models"
	"backend/internal/simulation"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Expected Wins Response Types

type GetWeeklyExpectedWinsResponse struct {
	Data []models.WeeklyExpectedWins `json:"data"`
}

type GetSeasonExpectedWinsResponse struct {
	Data []models.SeasonExpectedWins `json:"data"`
}

type GetSeasonRankingsResponse struct {
	Data *simulation.SeasonRankings `json:"data"`
}

type GetLuckDistributionResponse struct {
	Data *simulation.LuckDistribution `json:"data"`
}

type GetTeamProgressionResponse struct {
	Data []models.WeeklyExpectedWins `json:"data"`
}

type AllTimeExpectedWins struct {
	TeamID             uint    `json:"team_id"`
	TeamName           string  `json:"team_name"`
	Owner              string  `json:"owner"`
	TotalExpectedWins  float64 `json:"total_expected_wins"`
	TotalExpectedLoses float64 `json:"total_expected_losses"`
	TotalActualWins    int     `json:"total_actual_wins"`
	TotalActualLosses  int     `json:"total_actual_losses"`
	TotalWinLuck       float64 `json:"total_win_luck"`
	SeasonsPlayed      int     `json:"seasons_played"`
}

type GetAllTimeExpectedWinsResponse struct {
	Data []AllTimeExpectedWins `json:"data"`
}

// API Handlers

// GetWeeklyExpectedWins returns weekly expected wins data
// GET /api/v1/leagues/{id}/expected-wins/weekly/{year}?week={week}
func GetWeeklyExpectedWins(c *gin.Context) {
	leagueID, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid league ID"})
		return
	}

	year, err := parseUintParam(c, "year")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid year"})
		return
	}

	weekParam := c.Query("week")
	if weekParam != "" {
		// Get specific week
		week, err := strconv.ParseUint(weekParam, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid week parameter"})
			return
		}

		data, err := models.GetWeeklyExpectedWinsData(database.DB, leagueID, year, uint(week))
		if err != nil {
			slog.Error("Failed to fetch weekly expected wins", "error", err, "league", leagueID, "year", year, "week", week)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch weekly expected wins"})
			return
		}

		c.JSON(http.StatusOK, GetWeeklyExpectedWinsResponse{Data: data})
	} else {
		// Get all weeks for season progression
		data, err := models.GetAllWeeklyExpectedWins(database.DB, leagueID, year)
		if err != nil {
			slog.Error("Failed to fetch all weekly expected wins", "error", err, "league", leagueID, "year", year)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch weekly expected wins"})
			return
		}

		c.JSON(http.StatusOK, GetWeeklyExpectedWinsResponse{Data: data})
	}
}

// GetSeasonExpectedWins returns season expected wins data
// GET /api/v1/leagues/{id}/expected-wins/season/{year}
func GetSeasonExpectedWins(c *gin.Context) {
	leagueID, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid league ID"})
		return
	}

	year, err := parseUintParam(c, "year")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid year"})
		return
	}

	data, err := models.GetSeasonExpectedWinsData(database.DB, leagueID, year)
	if err != nil {
		slog.Error("Failed to fetch season expected wins", "error", err, "league", leagueID, "year", year)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch season expected wins"})
		return
	}

	c.JSON(http.StatusOK, GetSeasonExpectedWinsResponse{Data: data})
}

// GetSeasonRankings returns teams ranked by various expected wins metrics
// GET /api/v1/leagues/{id}/expected-wins/rankings/{year}
func GetSeasonRankings(c *gin.Context) {
	leagueID, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid league ID"})
		return
	}

	year, err := parseUintParam(c, "year")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid year"})
		return
	}

	data, err := simulation.GetSeasonExpectedWinsRankings(leagueID, year)
	if err != nil {
		slog.Error("Failed to fetch season rankings", "error", err, "league", leagueID, "year", year)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch season rankings"})
		return
	}

	c.JSON(http.StatusOK, GetSeasonRankingsResponse{Data: data})
}

// GetLuckDistribution returns luck distribution analysis for a season
// GET /api/v1/leagues/{id}/expected-wins/luck/{year}
func GetLuckDistribution(c *gin.Context) {
	leagueID, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid league ID"})
		return
	}

	year, err := parseUintParam(c, "year")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid year"})
		return
	}

	data, err := simulation.CalculateLeagueLuckDistribution(leagueID, year)
	if err != nil {
		slog.Error("Failed to calculate luck distribution", "error", err, "league", leagueID, "year", year)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to calculate luck distribution"})
		return
	}

	c.JSON(http.StatusOK, GetLuckDistributionResponse{Data: data})
}

// GetTeamProgression returns weekly progression for a specific team
// GET /api/v1/teams/{id}/expected-wins/{year}
func GetTeamProgression(c *gin.Context) {
	teamID, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	year, err := parseUintParam(c, "year")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid year"})
		return
	}

	data, err := models.GetTeamWeeklyProgression(database.DB, teamID, year)
	if err != nil {
		slog.Error("Failed to fetch team progression", "error", err, "team", teamID, "year", year)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch team progression"})
		return
	}

	c.JSON(http.StatusOK, GetTeamProgressionResponse{Data: data})
}

// NOTE: Management/Admin endpoints for recalculation have been removed
// Expected wins recalculation is now only available via ETL scripts to prevent server stress
// Use: backend/cmd/etl/main.go --calculate-expected-wins for recalculation

// GetAllTimeExpectedWins returns all-time expected wins totals for all teams
// GET /api/teams/all-time-expected-wins
func GetAllTimeExpectedWins(c *gin.Context) {
	db := database.DB

	// Query to aggregate expected wins across all seasons for each team
	var results []struct {
		TeamID              uint    `json:"team_id"`
		TeamName            string  `json:"team_name"`
		Owner               string  `json:"owner"`
		TotalExpectedWins   float64 `json:"total_expected_wins"`
		TotalExpectedLosses float64 `json:"total_expected_losses"`
		TotalActualWins     int64   `json:"total_actual_wins"`
		TotalActualLosses   int64   `json:"total_actual_losses"`
		TotalWinLuck        float64 `json:"total_win_luck"`
		SeasonsPlayed       int64   `json:"seasons_played"`
	}

	err := db.Table("season_expected_wins").
		Select(`
			season_expected_wins.team_id,
			teams.name as team_name,
			teams.owner,
			COALESCE(SUM(season_expected_wins.expected_wins), 0) as total_expected_wins,
			COALESCE(SUM(season_expected_wins.expected_losses), 0) as total_expected_losses,
			COALESCE(SUM(season_expected_wins.actual_wins), 0) as total_actual_wins,
			COALESCE(SUM(season_expected_wins.actual_losses), 0) as total_actual_losses,
			COALESCE(SUM(season_expected_wins.win_luck), 0) as total_win_luck,
			COUNT(season_expected_wins.year) as seasons_played
		`).
		Joins("JOIN teams ON teams.id = season_expected_wins.team_id").
		Where("season_expected_wins.deleted_at IS NULL AND teams.deleted_at IS NULL").
		Group("season_expected_wins.team_id, teams.name, teams.owner").
		Order("total_expected_wins DESC").
		Scan(&results).Error

	if err != nil {
		slog.Error("Failed to fetch all-time expected wins", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch all-time expected wins"})
		return
	}

	// Convert to response format
	data := make([]AllTimeExpectedWins, len(results))
	for i, result := range results {
		data[i] = AllTimeExpectedWins{
			TeamID:             result.TeamID,
			TeamName:           result.TeamName,
			Owner:              result.Owner,
			TotalExpectedWins:  result.TotalExpectedWins,
			TotalExpectedLoses: result.TotalExpectedLosses,
			TotalActualWins:    int(result.TotalActualWins),
			TotalActualLosses:  int(result.TotalActualLosses),
			TotalWinLuck:       result.TotalWinLuck,
			SeasonsPlayed:      int(result.SeasonsPlayed),
		}
	}

	c.JSON(http.StatusOK, GetAllTimeExpectedWinsResponse{Data: data})
}

// Helper functions

func parseUintParam(c *gin.Context, param string) (uint, error) {
	value, err := strconv.ParseUint(c.Param(param), 10, 32)
	return uint(value), err
}
