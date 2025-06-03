package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"backend/internal/database"
	"backend/internal/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/exp/slices"
)

type GetTeamsResponse struct {
	Teams []TeamResponse `json:"teams"`
}

type TeamResponse struct {
	ID            string     `json:"id"`
	ESPNID        string     `json:"espnId"`
	Name          string     `json:"name"`
	OwnerName     string     `json:"owner"`
	TeamRecord    TeamRecord `json:"record"`
	Points        TeamPoints `json:"points"`
	Rank          int        `json:"rank"`
	PlayoffChance float64    `json:"playoffChance"`
}

type TeamRecord struct {
	Wins   int `json:"wins"`
	Losses int `json:"losses"`
	Ties   int `json:"ties"`
}

type TeamPoints struct {
	Scored  float64 `json:"scored"`
	Against float64 `json:"against"`
}

// GetTeams returns all teams
func GetTeams(c *gin.Context) {
	allTeams, teamsErr := []models.Team{}, error(nil)
	fullSchedule, scheduleErr := []models.Matchup{}, error(nil)

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if teamsErr = database.DB.Model(&models.Team{}).Find(&allTeams).Error; teamsErr != nil {
			slog.Error("Failed to fetch teams from database", "error", teamsErr)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if scheduleErr = database.DB.Model(&models.Matchup{}).Where("completed = true").Find(&fullSchedule).Error; scheduleErr != nil {
			slog.Error("Failed to fetch full schedule from database", "error", scheduleErr)
		}
	}()

	wg.Wait()

	if teamsErr != nil {
		slog.Error("Error fetching teams", "error", teamsErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch teams"})
		return
	}
	if scheduleErr != nil {
		slog.Error("Error fetching full schedule", "error", scheduleErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch full schedule"})
		return
	}

	slog.Info("Fetched teams from database", "count", len(allTeams))
	slog.Info("Fetched full schedule from database", "count", len(fullSchedule))

	resp := GetTeamsResponse{}

	for _, team := range allTeams {
		resp.Teams = append(resp.Teams, TeamResponse{
			ID:        fmt.Sprintf("%d", team.ID),
			ESPNID:    fmt.Sprintf("%d", team.ESPNID),
			Name:      team.Name,
			OwnerName: team.Owner,
			TeamRecord: TeamRecord{
				Wins:   team.Wins,
				Losses: team.Losses,
				Ties:   team.Ties,
			},
		})
	}

	for idx, matchup := range fullSchedule {
		_ = idx
		// if idx > 1 {
		// 	break
		// }
		// slog.Info("Processing matchup", "home_team_espn_id", matchup.HomeTeamESPNID, "away_team_espn_id", matchup.AwayTeamESPNID)
		// Add to resp
		for i, team := range resp.Teams {
			// slog.Info("Checking team against matchup", "team_id", team.ESPNID, "home_team_espn_id", matchup.HomeTeamESPNID, "away_team_espn_id", matchup.AwayTeamESPNID)
			// Add total points scored and against
			if team.ESPNID == fmt.Sprintf("%d", matchup.HomeTeamESPNID) {
				resp.Teams[i].Points.Scored += matchup.HomeTeamFinalScore
				resp.Teams[i].Points.Against += matchup.AwayTeamFinalScore
			} else if team.ESPNID == fmt.Sprintf("%d", matchup.AwayTeamESPNID) {
				resp.Teams[i].Points.Scored += matchup.AwayTeamFinalScore
				resp.Teams[i].Points.Against += matchup.HomeTeamFinalScore
			}

			// Add the wins and losses
			if matchup.Completed {
				if matchup.HomeTeamFinalScore > matchup.AwayTeamFinalScore {
					if team.ESPNID == fmt.Sprintf("%d", matchup.HomeTeamESPNID) {
						resp.Teams[i].TeamRecord.Wins++
					} else if team.ESPNID == fmt.Sprintf("%d", matchup.AwayTeamESPNID) {
						resp.Teams[i].TeamRecord.Losses++
					}
				} else if matchup.HomeTeamFinalScore < matchup.AwayTeamFinalScore {
					if team.ESPNID == fmt.Sprintf("%d", matchup.AwayTeamESPNID) {
						resp.Teams[i].TeamRecord.Wins++
					} else if team.ESPNID == fmt.Sprintf("%d", matchup.HomeTeamESPNID) {
						resp.Teams[i].TeamRecord.Losses++
					}
				} else {
					resp.Teams[i].TeamRecord.Ties++
				}
			}
		}
	}

	// Sort teams by wins, then by points scored
	slices.SortStableFunc(resp.Teams, func(a, b TeamResponse) int {
		if a.TeamRecord.Wins != b.TeamRecord.Wins {
			return b.TeamRecord.Wins - a.TeamRecord.Wins // Sort by wins descending
		}
		if a.Points.Scored != b.Points.Scored {
			if a.Points.Scored < b.Points.Scored {
				return 1 // Sort by points scored descending
			}
			return -1
		}
		return 0 // Equal, maintain order
	})

	// Assign ranks based on sorted order
	for i := range resp.Teams {
		resp.Teams[i].Rank = i + 1 // Rank starts from 1
	}

	c.JSON(http.StatusOK, resp)
}

// GetTeamByID returns a team by its ID
func GetTeamByID(c *gin.Context) {
	id := c.Param("id")

	slog.Info("Fetching team by ID", "id", id)

	team := models.Team{
		ID:     1,
		Name:   "Touchdown Terrors",
		Owner:  "John Doe",
		Wins:   8,
		Losses: 3,
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
