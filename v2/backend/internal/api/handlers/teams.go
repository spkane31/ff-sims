package handlers

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"sync"

	"backend/internal/database"
	"backend/internal/logging"
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

	for _, matchup := range fullSchedule {
		// Add to resp
		for i, team := range resp.Teams {
			// slog.Info("Checking team against matchup", "team_id", team.ESPNID, "home_team_espn_id", matchup.HomeTeamESPNID, "away_team_espn_id", matchup.AwayTeamESPNID)
			// Add total points scored and against
			if team.ID == fmt.Sprintf("%d", matchup.HomeTeamID) {
				resp.Teams[i].Points.Scored += matchup.HomeTeamFinalScore
				resp.Teams[i].Points.Against += matchup.AwayTeamFinalScore
			} else if team.ID == fmt.Sprintf("%d", matchup.AwayTeamID) {
				resp.Teams[i].Points.Scored += matchup.AwayTeamFinalScore
				resp.Teams[i].Points.Against += matchup.HomeTeamFinalScore
			}

			// Add the wins and losses
			if matchup.HomeTeamFinalScore > matchup.AwayTeamFinalScore {
				if team.ID == fmt.Sprintf("%d", matchup.HomeTeamID) {
					resp.Teams[i].TeamRecord.Wins++
				} else if team.ID == fmt.Sprintf("%d", matchup.AwayTeamID) {
					resp.Teams[i].TeamRecord.Losses++
				}
			} else if matchup.HomeTeamFinalScore < matchup.AwayTeamFinalScore {
				if team.ID == fmt.Sprintf("%d", matchup.AwayTeamID) {
					resp.Teams[i].TeamRecord.Wins++
				} else if team.ID == fmt.Sprintf("%d", matchup.HomeTeamID) {
					resp.Teams[i].TeamRecord.Losses++
				}
			} else {
				resp.Teams[i].TeamRecord.Ties++
			}
		}
	}

	// Sort teams by wins, then by points scored
	slices.SortStableFunc(resp.Teams, func(a, b TeamResponse) int {
		if a.TeamRecord.Wins != b.TeamRecord.Wins {
			return b.TeamRecord.Wins - a.TeamRecord.Wins
		}
		if a.Points.Scored != b.Points.Scored {
			if a.Points.Scored < b.Points.Scored {
				return 1
			}
			return -1
		}
		return 0
	})

	for i := range resp.Teams {
		resp.Teams[i].Rank = i + 1
	}

	c.JSON(http.StatusOK, resp)
}

// GetTeamByID returns detailed information about a team including schedule, players, draft, and transactions
func GetTeamByID(c *gin.Context) {
	id := c.Param("id")

	slog.Info("Fetching team by ID", "id", id)

	teamMap, err := database.GetTeamsIDMap()
	if err != nil {
		slog.Error("Failed to fetch teams ID map", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch team data"})
		return
	}

	// Fetch the team
	var team models.Team
	if err := database.DB.Where("espn_id = ?", id).First(&team).Error; err != nil {
		slog.Error("Failed to fetch team from database", "error", err, "id", id)
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	// Fetch team's schedule (all matchups for this team)
	var schedule []models.Matchup
	if err := database.DB.Where("home_team_id = ? OR away_team_id = ?", team.ID, team.ID).
		Order("year desc, week asc").Find(&schedule).Error; err != nil {
		slog.Error("Failed to fetch team schedule", "error", err, "team_id", team.ID)
	}

	for _, matchup := range schedule {
		logging.Infof("Matchup: Week %d, Year %d, Home Team: %s (%d), Away Team: %s (%d), Home Score: %.2f, Away Score: %.2f",
			matchup.Week, matchup.Year,
			teamMap[matchup.HomeTeamID].Owner, matchup.HomeTeamID,
			teamMap[matchup.AwayTeamID].Owner, matchup.AwayTeamID,
			matchup.HomeTeamFinalScore, matchup.AwayTeamFinalScore)
	}

	// Fetch team's draft picks from DraftSelection table
	var draftSelections []models.DraftSelection
	if err := database.DB.Where("team_id = ?", team.ID).
		Order("year desc, round asc, pick asc").Find(&draftSelections).Error; err != nil {
		slog.Error("Failed to fetch team draft picks", "error", err, "team_id", team.ID)
	}

	// TODO: Fetch current and past players once team-player relationships are implemented
	// This would require a join table or foreign keys between teams and players
	currentPlayers := []PlayerResponse{
		{
			ID:            "1",
			Name:          "Patrick Mahomes",
			Position:      "QB",
			Team:          "KC",
			Status:        "Active",
			FantasyPoints: 287.5,
			Stats: PlayerStatsResponse{
				PassingYards: 4183,
				PassingTDs:   27,
				RushingTDs:   1,
			},
		},
		{
			ID:            "2",
			Name:          "Travis Kelce",
			Position:      "TE",
			Team:          "KC",
			Status:        "Active",
			FantasyPoints: 201.3,
			Stats: PlayerStatsResponse{
				Receptions:     93,
				ReceivingYards: 984,
				ReceivingTDs:   5,
			},
		},
	}

	// Transform draft picks data
	var draftPicks []DraftPickResponse
	for _, selection := range draftSelections {
		draftPicks = append(draftPicks, DraftPickResponse{
			Round:    int(selection.Round),
			Pick:     int(selection.Pick),
			Player:   selection.PlayerName,
			Position: selection.PlayerPosition,
			Team:     "", // TODO: Add NFL team field to DraftSelection model
			Year:     int(selection.Year),
		})

		log.Printf("Draft Pick: Round %d, Pick %d, Player %s, Position %s, Year %d",
			selection.Round, selection.Pick, selection.PlayerName, selection.PlayerPosition, selection.Year)
	}

	// TODO: Fetch transactions once transaction data is stored in database
	// This would require a Transaction model with trade/waiver/free agent data
	transactions := []TransactionResponse{
		{
			ID:            "1",
			Type:          "Trade",
			Date:          "2024-10-15",
			Description:   "Traded DeAndre Hopkins for DJ Moore",
			PlayersGained: []string{"DJ Moore"},
			PlayersLost:   []string{"DeAndre Hopkins"},
			Week:          7,
		},
		{
			ID:            "2",
			Type:          "Waiver",
			Date:          "2024-11-01",
			Description:   "Claimed Jayden Reed off waivers",
			PlayersGained: []string{"Jayden Reed"},
			PlayersLost:   []string{},
			Week:          9,
		},
	}

	// Transform schedule data
	scheduleResponse := make([]ScheduleGameResponse, len(schedule))
	for i, matchup := range schedule {
		var opponent string
		var teamScore, opponentScore float64
		var isHome bool

		if matchup.HomeTeamID == team.ID {
			// This team is home
			isHome = true
			teamScore = matchup.HomeTeamFinalScore
			opponentScore = matchup.AwayTeamFinalScore
			opponent = teamMap[matchup.AwayTeamID].Owner
		} else {
			// This team is away
			isHome = false
			teamScore = matchup.AwayTeamFinalScore
			opponentScore = matchup.HomeTeamFinalScore
			opponent = teamMap[matchup.HomeTeamID].Owner
		}

		var result string
		if matchup.Completed {
			if teamScore > opponentScore {
				result = "W"
			} else if teamScore < opponentScore {
				result = "L"
			} else {
				result = "T"
			}
		} else {
			result = "Upcoming"
		}

		scheduleResponse[i] = ScheduleGameResponse{
			Week:          int(matchup.Week),
			Year:          int(matchup.Year),
			Opponent:      opponent,
			IsHome:        isHome,
			TeamScore:     teamScore,
			OpponentScore: opponentScore,
			Result:        result,
			Completed:     matchup.Completed,
			IsPlayoff:     matchup.IsPlayoff,
		}
	}

	response := TeamDetailResponse{
		ID:     fmt.Sprintf("%d", team.ID),
		ESPNID: fmt.Sprintf("%d", team.ESPNID),
		Name:   team.Name,
		Owner:  team.Owner,
		Record: TeamRecord{
			Wins:   team.Wins,
			Losses: team.Losses,
			Ties:   team.Ties,
		},
		Points: TeamPoints{
			Scored:  team.Points, // TODO: Calculate from actual matchup data
			Against: 0.0,         // TODO: Calculate from actual matchup data
		},
		Schedule:       scheduleResponse,
		CurrentPlayers: currentPlayers,
		DraftPicks:     draftPicks,
		Transactions:   transactions,
	}

	c.JSON(http.StatusOK, response)
}

// Response types for detailed team information
type TeamDetailResponse struct {
	ID             string                 `json:"id"`
	ESPNID         string                 `json:"espnId"`
	Name           string                 `json:"name"`
	Owner          string                 `json:"owner"`
	Record         TeamRecord             `json:"record"`
	Points         TeamPoints             `json:"points"`
	Schedule       []ScheduleGameResponse `json:"schedule"`
	CurrentPlayers []PlayerResponse       `json:"currentPlayers"`
	DraftPicks     []DraftPickResponse    `json:"draftPicks"`
	Transactions   []TransactionResponse  `json:"transactions"`
}

type ScheduleGameResponse struct {
	Week          int     `json:"week"`
	Year          int     `json:"year"`
	Opponent      string  `json:"opponent"`
	IsHome        bool    `json:"isHome"`
	TeamScore     float64 `json:"teamScore"`
	OpponentScore float64 `json:"opponentScore"`
	Result        string  `json:"result"` // "W", "L", "T", or "Upcoming"
	Completed     bool    `json:"completed"`
	IsPlayoff     bool    `json:"isPlayoff"`
}

type PlayerResponse struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Position      string              `json:"position"`
	Team          string              `json:"team"`
	Status        string              `json:"status"`
	FantasyPoints float64             `json:"fantasyPoints"`
	Stats         PlayerStatsResponse `json:"stats"`
}

type PlayerStatsResponse struct {
	PassingYards   int `json:"passingYards"`
	PassingTDs     int `json:"passingTDs"`
	Interceptions  int `json:"interceptions"`
	RushingYards   int `json:"rushingYards"`
	RushingTDs     int `json:"rushingTDs"`
	Receptions     int `json:"receptions"`
	ReceivingYards int `json:"receivingYards"`
	ReceivingTDs   int `json:"receivingTDs"`
	Fumbles        int `json:"fumbles"`
	FieldGoals     int `json:"fieldGoals"`
	ExtraPoints    int `json:"extraPoints"`
}

type DraftPickResponse struct {
	Round    int    `json:"round"`
	Pick     int    `json:"pick"`
	Player   string `json:"player"`
	Position string `json:"position"`
	Team     string `json:"team"`
	Year     int    `json:"year"`
}

type TransactionResponse struct {
	ID            string   `json:"id"`
	Type          string   `json:"type"` // "Trade", "Waiver", "Free Agent"
	Date          string   `json:"date"`
	Description   string   `json:"description"`
	PlayersGained []string `json:"playersGained"`
	PlayersLost   []string `json:"playersLost"`
	Week          int      `json:"week"`
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
