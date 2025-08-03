package handlers

import (
	"backend/internal/database"
	"backend/internal/logging"
	"backend/internal/models"
	"fmt"
	"math"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
)

type GetStatsRequest struct{}

type GetStatsResponse struct {
	TeamStats []TeamStats `json:"teamStats"`
}

type TeamStats struct {
	TeamID        string  `json:"teamId"`
	TeamOwner     string  `json:"teamOwner"`
	AveragePoints float64 `json:"averagePoints"`
	StdDevPoints  float64 `json:"stdDevPoints"`
}

func GetStats(c *gin.Context) {
	// Handler logic for getting simulation stats

	type Stats struct {
		TeamID        string  `json:"team_id"`
		AveragePoints float64 `json:"average_points"`
		StdDevPoints  float64 `json:"std_dev_points"`
	}

	var matchups []models.Matchup

	err := database.DB.Model(&models.Matchup{}).Select([]string{
		"home_team_id",
		"away_team_id",
		"home_team_final_score",
		"away_team_final_score",
	}).
		Where("season = ? AND completed = ?", 2024, true).
		Scan(&matchups).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get stats"})
		return
	}

	logging.Infof("Retrieved stats for %d teams", len(matchups))

	allTeams, err := database.GetTeamsIDMap()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get teams"})
		return
	}

	resp := GetStatsResponse{
		TeamStats: []TeamStats{},
	}

	teamScoresMap := make(map[uint][]float64)

	for _, matchup := range matchups {
		// homeTeamID := fmt.Sprintf("%d", matchup.HomeTeamID)
		// awayTeamID := fmt.Sprintf("%d", matchup.AwayTeamID)

		if _, exists := teamScoresMap[matchup.HomeTeamID]; !exists {
			teamScoresMap[matchup.HomeTeamID] = []float64{matchup.HomeTeamFinalScore}
		} else {
			teamScoresMap[matchup.HomeTeamID] = append(teamScoresMap[matchup.HomeTeamID], matchup.HomeTeamFinalScore)
		}
		if _, exists := teamScoresMap[matchup.AwayTeamID]; !exists {
			teamScoresMap[matchup.AwayTeamID] = []float64{matchup.AwayTeamFinalScore}
		} else {
			teamScoresMap[matchup.AwayTeamID] = append(teamScoresMap[matchup.AwayTeamID], matchup.AwayTeamFinalScore)
		}
	}

	for teamID, scores := range teamScoresMap {
		if len(scores) == 0 {
			continue
		}

		var total float64
		for _, score := range scores {
			total += score
		}
		average := total / float64(len(scores))

		var variance float64
		for _, score := range scores {
			variance += (score - average) * (score - average)
		}
		variance /= float64(len(scores))

		resp.TeamStats = append(resp.TeamStats, TeamStats{
			TeamID:        fmt.Sprintf("%d", teamID),
			TeamOwner:     allTeams[teamID].Name,
			AveragePoints: average,
			StdDevPoints:  math.Sqrt(variance),
		})
	}

	// Add the league average and standard deviation
	if len(resp.TeamStats) == 0 {
		c.JSON(200, resp)
		return
	}

	var leagueTotal float64
	var leagueCount int
	for _, scores := range teamScoresMap {
		for _, score := range scores {
			leagueTotal += score
			leagueCount++
		}
	}
	leagueAverage := leagueTotal / float64(leagueCount)

	var leagueVariance float64
	for _, scores := range teamScoresMap {
		for _, score := range scores {
			leagueVariance += (score - leagueAverage) * (score - leagueAverage)
		}
	}
	leagueVariance /= float64(leagueCount)
	leagueStdDev := math.Sqrt(leagueVariance)

	sort.Slice(resp.TeamStats, func(i, j int) bool {
		return resp.TeamStats[i].AveragePoints > resp.TeamStats[j].AveragePoints
	})

	resp.TeamStats = append(resp.TeamStats, TeamStats{
		TeamID:        "league_average",
		TeamOwner:     "League Average",
		AveragePoints: leagueAverage,
		StdDevPoints:  leagueStdDev,
	})

	c.JSON(200, resp)
}
