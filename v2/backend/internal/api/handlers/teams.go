package handlers

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"backend/internal/database"
	"backend/internal/logging"
	"backend/internal/models"
	"backend/internal/utils"

	"github.com/gin-gonic/gin"
	"golang.org/x/exp/slices"
)

// Removed isThirdPlaceGame function - now using utils.GetPlayoffGameType

type GetTeamsResponse struct {
	Teams []TeamResponse `json:"teams"`
}

type TeamResponse struct {
	ID                  string     `json:"id"`
	ESPNID              string     `json:"espnId"`
	Name                string     `json:"name"`
	OwnerName           string     `json:"owner"`
	RegularSeasonRecord TeamRecord `json:"record"`
	PlayoffsRecord      TeamRecord `json:"playoffRecord"`
	Points              TeamPoints `json:"points"`
	Rank                int        `json:"rank"`
	PlayoffChance       float64    `json:"playoffChance"`
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
		if teamsErr = database.DB.Model(&models.Team{}).Preload("NameHistory").Find(&allTeams).Error; teamsErr != nil {
			slog.Error("Failed to fetch teams from database", "error", teamsErr)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if scheduleErr = database.DB.Model(&models.Matchup{}).
			Where("completed = true").
			Find(&fullSchedule).Error; scheduleErr != nil {
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
			RegularSeasonRecord: TeamRecord{
				Wins:   team.Wins,
				Losses: team.Losses,
				Ties:   team.Ties,
			},
		})
	}

	for _, matchup := range fullSchedule {
		if !utils.ShouldIncludeInRecord(matchup, fullSchedule) {
			continue
		}

		// Add to resp
		for i, team := range resp.Teams {
			// Add total points scored and against
			if team.ID == fmt.Sprintf("%d", matchup.HomeTeamID) {
				resp.Teams[i].Points.Scored += matchup.HomeTeamFinalScore
				resp.Teams[i].Points.Against += matchup.AwayTeamFinalScore
			} else if team.ID == fmt.Sprintf("%d", matchup.AwayTeamID) {
				resp.Teams[i].Points.Scored += matchup.AwayTeamFinalScore
				resp.Teams[i].Points.Against += matchup.HomeTeamFinalScore
			}

			// Use the new playoff utility to determine game type
			if utils.ShouldIncludeInPlayoffRecord(matchup, fullSchedule) {
				// For playoff games, we need to track playoff records separately
				if matchup.HomeTeamFinalScore > matchup.AwayTeamFinalScore {
					if team.ID == fmt.Sprintf("%d", matchup.HomeTeamID) {
						resp.Teams[i].PlayoffsRecord.Wins++
					} else if team.ID == fmt.Sprintf("%d", matchup.AwayTeamID) {
						resp.Teams[i].PlayoffsRecord.Losses++
					}
				} else if matchup.HomeTeamFinalScore < matchup.AwayTeamFinalScore {
					if team.ID == fmt.Sprintf("%d", matchup.AwayTeamID) {
						resp.Teams[i].PlayoffsRecord.Wins++
					} else if team.ID == fmt.Sprintf("%d", matchup.HomeTeamID) {
						resp.Teams[i].PlayoffsRecord.Losses++
					}
				} else {
					resp.Teams[i].PlayoffsRecord.Ties++
				}
			} else if matchup.GameType == "NONE" {
				// Add the wins and losses for regular season games
				if matchup.HomeTeamFinalScore > matchup.AwayTeamFinalScore {
					if team.ID == fmt.Sprintf("%d", matchup.HomeTeamID) {
						resp.Teams[i].RegularSeasonRecord.Wins++
					} else if team.ID == fmt.Sprintf("%d", matchup.AwayTeamID) {
						resp.Teams[i].RegularSeasonRecord.Losses++
					}
				} else if matchup.HomeTeamFinalScore < matchup.AwayTeamFinalScore {
					if team.ID == fmt.Sprintf("%d", matchup.AwayTeamID) {
						resp.Teams[i].RegularSeasonRecord.Wins++
					} else if team.ID == fmt.Sprintf("%d", matchup.HomeTeamID) {
						resp.Teams[i].RegularSeasonRecord.Losses++
					}
				} else {
					resp.Teams[i].RegularSeasonRecord.Ties++
				}
			}
		}
	}

	// Sort teams by wins, then by points scored
	slices.SortStableFunc(resp.Teams, func(a, b TeamResponse) int {
		if a.RegularSeasonRecord.Wins != b.RegularSeasonRecord.Wins {
			return b.RegularSeasonRecord.Wins - a.RegularSeasonRecord.Wins
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
	team.Wins, team.Losses = 0, 0

	// Fetch team's schedule (all matchups for this team, including incomplete games for display)
	var schedule []models.Matchup
	if err := database.DB.Where("(home_team_id = ? OR away_team_id = ?) AND completed = true", team.ID, team.ID).
		Order("year desc, week asc").Find(&schedule).Error; err != nil {
		slog.Error("Failed to fetch team schedule", "error", err, "team_id", team.ID)
	}

	// Fetch full schedule for playoff detection (only completed games needed for playoff logic)
	var fullSchedule []models.Matchup
	if err := database.DB.Model(&models.Matchup{}).Where("completed = true").Find(&fullSchedule).Error; err != nil {
		slog.Error("Failed to fetch full schedule for playoff detection", "error", err)
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

	// Transform draft picks data
	var draftPicks []DraftPickResponse
	for _, selection := range draftSelections {
		teamOwner := ""
		teamID := 0
		if selection.Team != nil {
			teamOwner = selection.Team.Owner
			teamID = int(selection.Team.ESPNID)
		}

		draftPicks = append(draftPicks, DraftPickResponse{
			PlayerID: fmt.Sprintf("%d", selection.PlayerID),
			Round:    int(selection.Round),
			Pick:     int(selection.Pick),
			Player:   selection.PlayerName,
			Position: selection.PlayerPosition,
			TeamID:   teamID,
			Owner:    teamOwner,
			Year:     int(selection.Year),
		})

		log.Printf("Draft Pick: Round %d, Pick %d, Player %s, Position %s, Year %d",
			selection.Round, selection.Pick, selection.PlayerName, selection.PlayerPosition, selection.Year)
	}

	transactionsResp := []TransactionResponse{}

	// Transactions are not perfect in the database. The transactions are "grouped" together by the date.
	// TRADED transaction_types need to be collected in a second query to get the related players.
	transactions := []models.Transaction{}
	if err := database.DB.Where("team_id = ?", team.ID).Order("date desc").Find(&transactions).Error; err != nil {
		slog.Error("Failed to fetch team transactions", "error", err, "team_id", team.ID)
	}

	logging.Infof("Fetched %d transactions for team %s", len(transactions), team.Name)

	tradesDates := []time.Time{}
	for _, transaction := range transactions {
		if transaction.TransactionType == "TRADED" {
			tradesDates = append(tradesDates, transaction.Date)
		}
	}

	tradesTransactions := []models.Transaction{}
	if len(tradesDates) > 0 {
		if err := database.DB.Where("date IN (?)", tradesDates).Find(&tradesTransactions).Error; err != nil {
			slog.Error("Failed to fetch trades transactions", "error", err, "dates", tradesDates)
		}
	}

	tradesGrouped := make(map[time.Time][]models.Transaction)
	for _, trade := range tradesTransactions {
		tradesGrouped[trade.Date] = append(tradesGrouped[trade.Date], trade)
	}

	logging.Infof("Fetched %d trades transactions for team %s", len(tradesTransactions), team.Name)

	// Transform transactions data
	groupedTransactions := make(map[time.Time][]models.Transaction)
	for _, transaction := range transactions {
		// Group transactions by date
		groupedTransactions[transaction.Date] = append(groupedTransactions[transaction.Date], transaction)
	}

	for date, group := range groupedTransactions {
		var playersGained, playersLost []TransactionPlayer
		var transactionType string
		// var description string
		var week uint
		var year uint
		for _, transaction := range group {
			// Set week and year from the first transaction in the group
			if week == 0 {
				week = transaction.Week
				year = transaction.Year
			}
			if transaction.TransactionType == "TRADED" {
				transactionType = "Trade"
				tradeDetails, exists := tradesGrouped[transaction.Date]
				if !exists {
					slog.Warn("Trade details not found for transaction", "date", transaction.Date)
					continue // Skip if no trade details found
				}
				for _, trade := range tradeDetails {
					if trade.TeamID == team.ID {
						// This is the team making the trade
						playersGained = append(playersGained, TransactionPlayer{
							ID:   trade.PlayerID,
							Name: trade.PlayerName})
					} else {
						// This is the trade partner
						playersLost = append(playersLost, TransactionPlayer{
							ID:   trade.PlayerID,
							Name: trade.PlayerName})
					}
				}
			} else if transaction.TransactionType == "FA ADDED" {
				transactionType = "Free Agent"
				playersGained = append(playersGained, TransactionPlayer{
					ID:   transaction.PlayerID,
					Name: transaction.PlayerName,
				})
			} else if transaction.TransactionType == "DROPPED" {
				if transactionType == "" {
					transactionType = "Waiver"
				}
				playersLost = append(playersLost, TransactionPlayer{
					ID:   transaction.PlayerID,
					Name: transaction.PlayerName,
				})
			} else if transaction.TransactionType == "WAIVER ADDED" {
			} else {
				slog.Warn("Unknown transaction type", "type", transaction.TransactionType, "date", date)
				continue
			}
		}
		transactionsResp = append(transactionsResp, TransactionResponse{
			Type:          transactionType,
			Date:          date,
			Year:          year,
			Week:          week,
			PlayersGained: playersGained,
			PlayersLost:   playersLost,
		})
		logging.Infof("Transaction on %s: Type %s, Players Gained %v, Players Lost %v, Week %d",
			date, transactionType, playersGained, playersLost, week)
	}

	slices.SortStableFunc(transactionsResp, func(a, b TransactionResponse) int {
		// Sort by date descending
		if a.Date != b.Date {
			if b.Date.Before(a.Date) {
				return -1 // a is more recent than b
			}
			return 1 // b is more recent than a
		}
		return 0
	})

	// Transform schedule data
	scheduleResponse := []ScheduleGameResponse{}
	for _, matchup := range schedule {
		if !utils.ShouldIncludeInRecord(matchup, fullSchedule) {
			continue
		}

		var opponent string
		var opponentESPNID string
		var teamScore, opponentScore float64
		var isHome bool

		if matchup.HomeTeamID == team.ID {
			// This team is home
			isHome = true
			teamScore = matchup.HomeTeamFinalScore
			opponentScore = matchup.AwayTeamFinalScore
			opponent = teamMap[matchup.AwayTeamID].Owner
			opponentESPNID = fmt.Sprintf("%d", teamMap[matchup.AwayTeamID].ESPNID)
		} else {
			// This team is away
			isHome = false
			teamScore = matchup.AwayTeamFinalScore
			opponentScore = matchup.HomeTeamFinalScore
			opponent = teamMap[matchup.HomeTeamID].Owner
			opponentESPNID = fmt.Sprintf("%d", teamMap[matchup.HomeTeamID].ESPNID)
		}

		// Use our playoff detection utility to determine if this is a playoff game
		isPlayoff := utils.ShouldIncludeInPlayoffRecord(matchup, fullSchedule)

		var result string
		if matchup.Completed {
			if teamScore > opponentScore {
				result = "W"
				team.Wins++
			} else if teamScore < opponentScore {
				result = "L"
				team.Losses++
			} else {
				result = "T"
				team.Ties++
			}
		} else {
			result = "Upcoming"
		}

		scheduleResponse = append(scheduleResponse, ScheduleGameResponse{
			Week:           int(matchup.Week),
			Year:           int(matchup.Year),
			Opponent:       opponent,
			OpponentESPNID: opponentESPNID,
			IsHome:         isHome,
			TeamScore:      teamScore,
			OpponentScore:  opponentScore,
			Result:         result,
			Completed:      matchup.Completed,
			IsPlayoff:      isPlayoff,
		})
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
		CurrentPlayers: []PlayerResponse{}, // TODO: Fetch current roster players
		DraftPicks:     draftPicks,
		Transactions:   transactionsResp,
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
	Week           int     `json:"week"`
	Year           int     `json:"year"`
	Opponent       string  `json:"opponent"`
	OpponentESPNID string  `json:"opponentESPNID"` // Add opponent ESPN ID for linking
	IsHome         bool    `json:"isHome"`
	TeamScore      float64 `json:"teamScore"`
	OpponentScore  float64 `json:"opponentScore"`
	Result         string  `json:"result"` // "W", "L", "T", or "Upcoming"
	Completed      bool    `json:"completed"`
	IsPlayoff      bool    `json:"isPlayoff"`
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

type DraftPickResponse struct {
	PlayerID string `json:"player_id"`
	Round    int    `json:"round"`
	Pick     int    `json:"pick"`
	Player   string `json:"player"`
	Position string `json:"position"`
	TeamID   int    `json:"team_id"`
	Owner    string `json:"owner"`
	Year     int    `json:"year"`
}

type TransactionResponse struct {
	Type          string              `json:"type"` // "Trade", "Waiver", "Free Agent"
	Date          time.Time           `json:"date"`
	Year          uint                `json:"year"`
	Week          uint                `json:"week"`
	Description   string              `json:"description"`
	PlayersGained []TransactionPlayer `json:"playersGained"`
	PlayersLost   []TransactionPlayer `json:"playersLost"`
}

type TransactionPlayer struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// GetCurrentSeasonStandings returns current season standings with expected wins
// GET /api/teams/standings/{year}
func GetCurrentSeasonStandings(c *gin.Context) {
	yearParam := c.Param("year")
	year, err := strconv.ParseUint(yearParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid year"})
		return
	}
	yearUint := uint(year)

	// Fetch all teams
	var allTeams []models.Team
	if err := database.DB.Find(&allTeams).Error; err != nil {
		slog.Error("Failed to fetch teams from database", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch teams"})
		return
	}

	// Fetch current year's matchups
	var matchups []models.Matchup
	if err := database.DB.Where("year = ? AND completed = true", yearUint).Find(&matchups).Error; err != nil {
		slog.Error("Failed to fetch matchups", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch matchups"})
		return
	}

	// Get all weekly expected wins data for the year and aggregate by team
	weeklyExpectedWins, err := models.GetAllWeeklyExpectedWins(database.DB, 345674, yearUint) // Assuming league ID 1
	if err != nil && err.Error() != "record not found" {
		slog.Error("Failed to fetch weekly expected wins", "error", err)
	}

	// Create expected wins map for quick lookup - sum up weekly expected wins for each team
	type TeamExpectedWinsSummary struct {
		TotalExpectedWins   float64
		TotalExpectedLosses float64
		TotalActualWins     int
		TotalActualLosses   int
		WeekCount           int
	}

	expectedWinsMap := make(map[uint]*TeamExpectedWinsSummary)
	for _, ew := range weeklyExpectedWins {
		if _, exists := expectedWinsMap[ew.TeamID]; !exists {
			expectedWinsMap[ew.TeamID] = &TeamExpectedWinsSummary{}
		}

		summary := expectedWinsMap[ew.TeamID]
		// Sum the weekly expected wins (not cumulative - use WeeklyExpectedWins field)
		summary.TotalExpectedWins += ew.WeeklyExpectedWins
		summary.TotalExpectedLosses += ew.WeeklyExpectedLosses
		if ew.WeeklyActualWin {
			summary.TotalActualWins++
		} else {
			summary.TotalActualLosses++
		}
		summary.WeekCount++
	}

	// Build standings response
	var standings []CurrentSeasonStandingResponse
	for _, team := range allTeams {
		// Skip dummy teams
		if team.ESPNID == 2 || team.ESPNID == 8 {
			continue
		}

		standing := CurrentSeasonStandingResponse{
			TeamID:   team.ID,
			ESPNID:   fmt.Sprintf("%d", team.ESPNID),
			Owner:    team.Owner,
			TeamName: team.Name,
			Record:   TeamRecord{Wins: 0, Losses: 0, Ties: 0},
			Points:   TeamPoints{Scored: 0, Against: 0},
		}

		// Calculate record and points from matchups
		for _, matchup := range matchups {
			if matchup.GameType != "NONE" {
				continue // Only regular season games
			}

			if matchup.HomeTeamID == team.ID {
				standing.Points.Scored += matchup.HomeTeamFinalScore
				standing.Points.Against += matchup.AwayTeamFinalScore
				if matchup.HomeTeamFinalScore > matchup.AwayTeamFinalScore {
					standing.Record.Wins++
				} else if matchup.HomeTeamFinalScore < matchup.AwayTeamFinalScore {
					standing.Record.Losses++
				} else {
					standing.Record.Ties++
				}
			} else if matchup.AwayTeamID == team.ID {
				standing.Points.Scored += matchup.AwayTeamFinalScore
				standing.Points.Against += matchup.HomeTeamFinalScore
				if matchup.AwayTeamFinalScore > matchup.HomeTeamFinalScore {
					standing.Record.Wins++
				} else if matchup.AwayTeamFinalScore < matchup.HomeTeamFinalScore {
					standing.Record.Losses++
				} else {
					standing.Record.Ties++
				}
			}
		}

		// Add expected wins data if available (using aggregated weekly data)
		if summary, exists := expectedWinsMap[team.ID]; exists {
			standing.ExpectedWins = &summary.TotalExpectedWins
			standing.ExpectedLosses = &summary.TotalExpectedLosses
			winLuck := float64(standing.Record.Wins) - summary.TotalExpectedWins
			standing.WinLuck = &winLuck
		}

		standings = append(standings, standing)
	}

	// Sort by wins, then by points scored
	slices.SortStableFunc(standings, func(a, b CurrentSeasonStandingResponse) int {
		if a.Record.Wins != b.Record.Wins {
			return b.Record.Wins - a.Record.Wins
		}
		if a.Points.Scored != b.Points.Scored {
			if a.Points.Scored < b.Points.Scored {
				return 1
			}
			return -1
		}
		return 0
	})

	c.JSON(http.StatusOK, GetCurrentSeasonStandingsResponse{
		Year:      yearUint,
		Standings: standings,
	})
}

// Response types for current season standings
type GetCurrentSeasonStandingsResponse struct {
	Year      uint                            `json:"year"`
	Standings []CurrentSeasonStandingResponse `json:"standings"`
}

type CurrentSeasonStandingResponse struct {
	TeamID         uint       `json:"team_id"`
	ESPNID         string     `json:"espn_id"`
	Owner          string     `json:"owner"`
	TeamName       string     `json:"team_name"`
	Record         TeamRecord `json:"record"`
	Points         TeamPoints `json:"points"`
	ExpectedWins   *float64   `json:"expected_wins,omitempty"`
	ExpectedLosses *float64   `json:"expected_losses,omitempty"`
	WinLuck        *float64   `json:"win_luck,omitempty"`
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
