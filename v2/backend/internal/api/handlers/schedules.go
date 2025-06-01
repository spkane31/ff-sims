package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type GetSchedulesResponse struct {
	Data Schedule `json:"data"`
}

type Schedule struct {
	Matchups []Matchup `json:"matchups"`
}

type Matchup struct {
	Year               int    `json:"year"`
	Week               int    `json:"week"`
	HomeTeamID         string `json:"homeTeamId"`
	AwayTeamID         string `json:"awayTeamId"`
	HomeTeamName       string `json:"homeTeamName"`
	AwayTeamName       string `json:"awayTeamName"`
	HomeScore          int    `json:"homeScore"`
	AwayScore          int    `json:"awayScore"`
	HomeProjectedScore int    `json:"homeProjectedScore"`
	AwayProjectedScore int    `json:"awayProjectedScore"`
}

// GetPlayers returns all players with optional filtering
func GetSchedules(c *gin.Context) {
	year := c.Query("year")

	log.Printf("GetSchedules called with year: %s\n", year)

	c.JSON(http.StatusOK, GetSchedulesResponse{
		Data: Schedule{
			Matchups: []Matchup{
				{Year: 2024, Week: 1, HomeTeamID: "team1", AwayTeamID: "team2", HomeTeamName: "Team A", AwayTeamName: "Team B", HomeScore: 24, AwayScore: 17, HomeProjectedScore: 25, AwayProjectedScore: 20},
				{Year: 2024, Week: 1, HomeTeamID: "team3", AwayTeamID: "team4", HomeTeamName: "Team C", AwayTeamName: "Team D", HomeScore: 30, AwayScore: 21, HomeProjectedScore: 28, AwayProjectedScore: 22},
				{Year: 2024, Week: 1, HomeTeamID: "team5", AwayTeamID: "team6", HomeTeamName: "Team E", AwayTeamName: "Team F", HomeScore: 14, AwayScore: 28, HomeProjectedScore: 18, AwayProjectedScore: 26},
				{Year: 2024, Week: 1, HomeTeamID: "team7", AwayTeamID: "team8", HomeTeamName: "Team G", AwayTeamName: "Team H", HomeScore: 21, AwayScore: 24, HomeProjectedScore: 22, AwayProjectedScore: 23},
				{Year: 2024, Week: 1, HomeTeamID: "team9", AwayTeamID: "team10", HomeTeamName: "Team I", AwayTeamName: "Team J", HomeScore: 27, AwayScore: 30, HomeProjectedScore: 26, AwayProjectedScore: 29},
				{Year: 2024, Week: 2, HomeTeamID: "team1", AwayTeamID: "team3", HomeTeamName: "Team A", AwayTeamName: "Team C", HomeScore: 20, AwayScore: 24, HomeProjectedScore: 22, AwayProjectedScore: 25},
				{Year: 2024, Week: 2, HomeTeamID: "team2", AwayTeamID: "team4", HomeTeamName: "Team B", AwayTeamName: "Team D", HomeScore: 17, AwayScore: 21, HomeProjectedScore: 19, AwayProjectedScore: 23},
				{Year: 2024, Week: 2, HomeTeamID: "team5", AwayTeamID: "team7", HomeTeamName: "Team E", AwayTeamName: "Team G", HomeScore: 30, AwayScore: 28, HomeProjectedScore: 29, AwayProjectedScore: 27},
				{Year: 2024, Week: 2, HomeTeamID: "team6", AwayTeamID: "team8", HomeTeamName: "Team F", AwayTeamName: "Team H", HomeScore: 21, AwayScore: 30, HomeProjectedScore: 20, AwayProjectedScore: 31},
				{Year: 2024, Week: 2, HomeTeamID: "team9", AwayTeamID: "team10", HomeTeamName: "Team I", AwayTeamName: "Team J", HomeScore: 24, AwayScore: 27, HomeProjectedScore: 25, AwayProjectedScore: 26},
			},
		},
	})
}
