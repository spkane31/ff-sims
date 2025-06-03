package handlers

import (
	"backend/internal/database"
	"backend/internal/models"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"

	"github.com/gin-gonic/gin"
)

type GetSchedulesResponse struct {
	Data Schedule `json:"data"`
}

type Schedule struct {
	Matchups []Matchup `json:"matchups"`
}

type Matchup struct {
	ID                 string  `json:"id"`
	Year               uint    `json:"year"`
	Week               uint    `json:"week"`
	HomeTeamESPNID     uint    `json:"homeTeamESPNId"`
	AwayTeamESPNID     uint    `json:"awayTeamESPNId"`
	HomeTeamName       string  `json:"homeTeamName"`
	AwayTeamName       string  `json:"awayTeamName"`
	HomeScore          float64 `json:"homeScore"`
	AwayScore          float64 `json:"awayScore"`
	HomeProjectedScore float64 `json:"homeProjectedScore"`
	AwayProjectedScore float64 `json:"awayProjectedScore"`
}

// GetPlayers returns all players with optional filtering
func GetSchedules(c *gin.Context) {
	year := c.Query("year")

	slog.Info("Fetching schedules", "year", year)

	wg := sync.WaitGroup{}
	wg.Add(1)

	schedule, scheduleErr := []models.Matchup{}, error(nil)
	go func() {
		defer wg.Done()
		if year == "" {
			if scheduleErr := database.DB.Model(&models.Matchup{}).Find(&schedule).Error; scheduleErr != nil {
				slog.Error("Failed to fetch schedules from database", "error", scheduleErr)
				return
			}
		} else {
			if scheduleErr := database.DB.Model(&models.Matchup{}).Where("year = ?", year).Find(&schedule).Error; scheduleErr != nil {
				slog.Error("Failed to fetch schedules from database", "error", scheduleErr)
				return
			}
		}
	}()

	teams, teamsErr := []models.Team{}, error(nil)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if teamsErr = database.DB.Model(&models.Team{}).Select("espn_id, owner").Find(&teams).Error; teamsErr != nil {
			slog.Error("Failed to fetch teams from database", "error", teamsErr)
			return
		}
		slog.Info("Fetched teams from database", "count", len(teams))
		for _, team := range teams {
			slog.Info("Team", "espn_id", team.ESPNID, "owner", team.Owner)
		}
	}()

	wg.Wait()

	if teamsErr != nil {
		slog.Error("Error fetching teams", "error", teamsErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch teams"})
		return
	}
	if scheduleErr != nil {
		slog.Error("Error fetching schedules", "error", scheduleErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch schedules"})
		return
	}

	resp := GetSchedulesResponse{}
	resp.Data.Matchups = make([]Matchup, len(schedule))
	for i, matchup := range schedule {
		resp.Data.Matchups[i] = Matchup{
			ID:                 fmt.Sprintf("%d", matchup.ID),
			Year:               matchup.Year,
			Week:               matchup.Week,
			HomeTeamESPNID:     matchup.HomeTeamESPNID,
			AwayTeamESPNID:     matchup.AwayTeamESPNID,
			HomeScore:          matchup.HomeTeamFinalScore,
			AwayScore:          matchup.AwayTeamFinalScore,
			HomeProjectedScore: matchup.HomeTeamESPNProjectedScore,
			AwayProjectedScore: matchup.AwayTeamESPNProjectedScore,
		}

		for _, team := range teams {
			if team.ESPNID == matchup.HomeTeamESPNID {
				resp.Data.Matchups[i].HomeTeamName = team.Owner
			}
			if team.ESPNID == matchup.AwayTeamESPNID {
				resp.Data.Matchups[i].AwayTeamName = team.Owner
			}
		}
	}

	// Sort in reverse order by year and week
	slices.SortStableFunc(resp.Data.Matchups, func(a, b Matchup) int {
		if a.Year != b.Year {
			return int(b.Year - a.Year) // Reverse order by year
		}
		return int(b.Week - a.Week) // Reverse order by week
	})

	c.JSON(http.StatusOK, resp)
}
