package handlers

import (
	"backend/internal/database"
	"backend/internal/models"
	"backend/internal/simulation"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Expected Wins Response Types

type WeeklyExpectedWinsWithLuck struct {
	models.WeeklyExpectedWins
	WinLuck float64 `json:"win_luck"`
}

type GetWeeklyExpectedWinsResponse struct {
	Data []WeeklyExpectedWinsWithLuck `json:"data"`
}

type SeasonExpectedWinsWithLuck struct {
	models.SeasonExpectedWins
	WinLuck float64 `json:"win_luck"`
}

type GetSeasonExpectedWinsResponse struct {
	Data []SeasonExpectedWinsWithLuck `json:"data"`
}

type GetSeasonRankingsResponse struct {
	Data *simulation.SeasonRankings `json:"data"`
}

type GetLuckDistributionResponse struct {
	Data *simulation.LuckDistribution `json:"data"`
}

type GetTeamProgressionResponse struct {
	Data []WeeklyExpectedWinsWithLuck `json:"data"`
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

		// Convert to response format with calculated win_luck
		responseData := make([]WeeklyExpectedWinsWithLuck, len(data))
		for i, weekly := range data {
			responseData[i] = WeeklyExpectedWinsWithLuck{
				WeeklyExpectedWins: weekly,
				WinLuck:            weekly.WinLuck(),
			}
		}

		c.JSON(http.StatusOK, GetWeeklyExpectedWinsResponse{Data: responseData})
	} else {
		// Get all weeks for season progression
		data, err := models.GetAllWeeklyExpectedWins(database.DB, leagueID, year)
		if err != nil {
			slog.Error("Failed to fetch all weekly expected wins", "error", err, "league", leagueID, "year", year)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch weekly expected wins"})
			return
		}

		// Convert to response format with calculated win_luck
		responseData := make([]WeeklyExpectedWinsWithLuck, len(data))
		for i, weekly := range data {
			responseData[i] = WeeklyExpectedWinsWithLuck{
				WeeklyExpectedWins: weekly,
				WinLuck:            weekly.WinLuck(),
			}
		}

		c.JSON(http.StatusOK, GetWeeklyExpectedWinsResponse{Data: responseData})
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

	// Convert to response format with calculated win_luck
	responseData := make([]SeasonExpectedWinsWithLuck, len(data))
	for i, season := range data {
		responseData[i] = SeasonExpectedWinsWithLuck{
			SeasonExpectedWins: season,
			WinLuck:            season.WinLuck(),
		}
	}

	c.JSON(http.StatusOK, GetSeasonExpectedWinsResponse{Data: responseData})
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

	// Convert to response format with calculated win_luck
	responseData := make([]WeeklyExpectedWinsWithLuck, len(data))
	for i, weekly := range data {
		responseData[i] = WeeklyExpectedWinsWithLuck{
			WeeklyExpectedWins: weekly,
			WinLuck:            weekly.WinLuck(),
		}
	}

	c.JSON(http.StatusOK, GetTeamProgressionResponse{Data: responseData})
}

// NOTE: Management/Admin endpoints for recalculation have been removed
// Expected wins recalculation is now only available via ETL scripts to prevent server stress
// Use: backend/cmd/etl/main.go --calculate-expected-wins for recalculation

// GetAllTimeExpectedWins returns all-time expected wins totals for all teams
// GET /api/teams/all-time-expected-wins
func GetAllTimeExpectedWins(c *gin.Context) {
	db := database.DB

	// Query to aggregate expected wins from season_expected_wins (regular season only)
	// This table contains final season totals calculated from regular season games only
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

	err := db.Raw(`
		SELECT
			season_expected_wins.team_id,
			teams.name as team_name,
			teams.owner,
			SUM(season_expected_wins.expected_wins) as total_expected_wins,
			SUM(season_expected_wins.expected_losses) as total_expected_losses,
			SUM(season_expected_wins.actual_wins) as total_actual_wins,
			SUM(season_expected_wins.actual_losses) as total_actual_losses,
			SUM(season_expected_wins.actual_wins) - SUM(season_expected_wins.expected_wins) as total_win_luck,
			COUNT(season_expected_wins.year) as seasons_played
		FROM season_expected_wins
		JOIN teams ON teams.id = season_expected_wins.team_id
		GROUP BY season_expected_wins.team_id, teams.name, teams.owner
		ORDER BY total_expected_wins DESC
	`).Scan(&results).Error

	fmt.Println(results)

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
