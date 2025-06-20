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
	ID                 string           `json:"id"`
	Year               uint             `json:"year"`
	Week               uint             `json:"week"`
	HomeTeamESPNID     uint             `json:"home_team_espn_id"`
	AwayTeamESPNID     uint             `json:"away_team_espn_id"`
	HomeTeamName       string           `json:"home_team_name"`
	AwayTeamName       string           `json:"away_team_name"`
	HomeScore          float64          `json:"home_score"`
	AwayScore          float64          `json:"away_score"`
	HomeProjectedScore float64          `json:"home_projected_score"`
	AwayProjectedScore float64          `json:"away_projected_score"`
	HomePlayers        []BoxScorePlayer `json:"home_players"`
	AwayPlayers        []BoxScorePlayer `json:"away_players"`
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
			if scheduleErr = database.DB.Model(&models.Matchup{}).Find(&schedule).Error; scheduleErr != nil {
				slog.Error("Failed to fetch schedules from database", "error", scheduleErr)
			}
		} else {
			if scheduleErr = database.DB.Model(&models.Matchup{}).Where("year = ?", year).Find(&schedule).Error; scheduleErr != nil {
				slog.Error("Failed to fetch schedules from database", "error", scheduleErr)
			}
		}
		slog.Info("Fetched schedules from database", "count", len(schedule))
	}()

	teams, teamsErr := []models.Team{}, error(nil)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if teamsErr = database.DB.Model(&models.Team{}).Select("id, owner").Find(&teams).Error; teamsErr != nil {
			slog.Error("Failed to fetch teams from database", "error", teamsErr)
			return
		}
		slog.Info("Fetched teams from database", "count", len(teams))
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
			HomeTeamESPNID:     matchup.HomeTeamID,
			AwayTeamESPNID:     matchup.AwayTeamID,
			HomeScore:          matchup.HomeTeamFinalScore,
			AwayScore:          matchup.AwayTeamFinalScore,
			HomeProjectedScore: matchup.HomeTeamESPNProjectedScore,
			AwayProjectedScore: matchup.AwayTeamESPNProjectedScore,
		}

		for _, team := range teams {
			if team.ID == matchup.HomeTeamID {
				resp.Data.Matchups[i].HomeTeamName = team.Owner
			}
			if team.ID == matchup.AwayTeamID {
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

type GetMatchupResponse struct {
	Data SingleMatchup `json:"data"`
}

type SingleMatchup struct {
	ID                string            `json:"id"`
	Year              uint              `json:"year"`
	Week              uint              `json:"week"`
	HomeTeam          TeamMatchup       `json:"homeTeam"`
	AwayTeam          TeamMatchup       `json:"awayTeam"`
	MatchupStatistics MatchupStatistics `json:"matchupStatistics"`
}

type TeamMatchup struct {
	ESPNID         string           `json:"id"`
	Name           string           `json:"name"`
	Score          float64          `json:"score"`
	ProjectedScore float64          `json:"projectedScore"`
	Players        []BoxScorePlayer `json:"players"`
}

type MatchupStatistics struct {
	PointDifferential  float64 `json:"pointDifferential"`
	AccuracyPercentage float64 `json:"accuracyPercentage"`
	PlayoffImplication string  `json:"playoffImplication"`
	WinProbability     float64 `json:"winProbability"`
}

type BoxScorePlayer struct {
	ID              string  `json:"id"`
	PlayerName      string  `json:"playerName"`
	PlayerPosition  string  `json:"playerPosition"`
	Status          string  `json:"status"`
	Team            string  `json:"team"`
	Points          float64 `json:"points"`
	ProjectedPoints float64 `json:"projectedPoints"`
	IsStarter       bool    `json:"isStarter"`
}

func GetMatchup(c *gin.Context) {
	id := c.Param("id")
	slog.Info("Fetching matchup", "id", id)

	wg := sync.WaitGroup{}
	wg.Add(1)

	matchups, matchupsErr := []models.Matchup{}, error(nil)
	go func() {
		defer wg.Done()
		if matchupsErr = database.DB.Model(&models.Matchup{}).Where("id = ?", id).Find(&matchups).Error; matchupsErr != nil {
			slog.Error("Failed to fetch matchup from database", "error", matchupsErr)
		}
	}()

	wg.Wait()

	if matchupsErr != nil {
		slog.Error("Error fetching matchup", "error", matchupsErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch matchup"})
		return
	}

	if len(matchups) != 1 {
		slog.Error("Matchup not found", "id", id, "count", len(matchups))
		if len(matchups) == 0 {
			// No matchup found
			c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
			return
		}
		// More than one matchup found, which is unexpected
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Multiple matchups found with the same ID"})
		return
	}

	resp := GetMatchupResponse{
		Data: SingleMatchup{
			ID:   id,
			Year: matchups[0].Year,
			Week: matchups[0].Week,
			HomeTeam: TeamMatchup{
				ESPNID:         fmt.Sprintf("%d", matchups[0].HomeTeamID),
				Score:          matchups[0].HomeTeamFinalScore,
				ProjectedScore: matchups[0].HomeTeamESPNProjectedScore,
				Name:           "Team Alpha", // Placeholder, should be fetched from teams
				Players: []BoxScorePlayer{
					{ID: "101", PlayerName: "Patrick Mahomes", PlayerPosition: "QB", Team: "KC", Status: "Active", ProjectedPoints: 24.5, Points: 27.8, IsStarter: true},
					{ID: "110", PlayerName: "DK Metcalf", PlayerPosition: "WR", Team: "SEA", Status: "Active", ProjectedPoints: 13.7, Points: 6.4, IsStarter: false},
				},
			},
			AwayTeam: TeamMatchup{
				ESPNID:         fmt.Sprintf("%d", matchups[0].AwayTeamID),
				Score:          matchups[0].AwayTeamFinalScore,
				ProjectedScore: matchups[0].AwayTeamESPNProjectedScore,
				Name:           "Team Omega", // Placeholder, should be fetched from teams
				Players: []BoxScorePlayer{
					{ID: "201", PlayerName: "Josh Allen", PlayerPosition: "QB", Team: "BUF", Status: "Active", ProjectedPoints: 23.8, Points: 20.2, IsStarter: true},
				},
			},
			MatchupStatistics: MatchupStatistics{
				PointDifferential:  matchups[0].HomeTeamFinalScore - matchups[0].AwayTeamFinalScore,
				AccuracyPercentage: 85.0,                             // Placeholder value
				PlayoffImplication: "High - every game is important", // Placeholder value
				WinProbability:     0.75,                             // Placeholder value
			},
		},
	}

	c.JSON(http.StatusOK, resp)
}
