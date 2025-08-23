package etl

import (
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/logging"
	"backend/internal/models"
	"backend/internal/simulation"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

const leagueID = 345674

// processBoxScorePlayers processes box score players data
func processBoxScorePlayers(filePath string) error {
	logging.Infof("Processing box score players data from: %s", filePath)
	// TODO: Implement box score players data processing
	return nil
}

type DraftSelection struct {
	PlayerName     string `json:"player_name"`
	PlayerID       int64  `json:"player_id"`
	PlayerPosition string `json:"player_position"`
	OwnerESPNID    uint   `json:"owner_espn_id"`
	Round          int    `json:"round"`
	Pick           int    `json:"pick"`
	Year           int    `json:"year"`
}

// processDraftSelections processes draft selections data
func processDraftSelections(filePath string) error {
	logging.Infof("Processing draft selections data from: %s", filePath)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read draft selections file %s: %w", filePath, err)
	}

	draftSelections := []DraftSelection{}
	if err := json.Unmarshal(data, &draftSelections); err != nil {
		return fmt.Errorf("failed to unmarshal draft selections data from %s: %w", filePath, err)
	}
	logging.Infof("Successfully processed %d draft selections from %s", len(draftSelections), filePath)

	teams, err := models.GetAllTeamsByLeague(database.DB, leagueID)
	if err != nil {
		return fmt.Errorf("failed to retrieve teams for league ID %d: %w", leagueID, err)
	}

	for _, selection := range draftSelections {
		logging.Infof("Draft Selection - Player: %s, ID: %d, Position: %s, Owner ESPN ID: %d, Round: %d, Pick: %d, Year: %d",
			selection.PlayerName, selection.PlayerID, selection.PlayerPosition,
			selection.OwnerESPNID, selection.Round, selection.Pick, selection.Year)

		// Check if the player exists in the database, create otherwise
		var player models.Player
		if err := database.DB.First(&player, "espn_id = ?", selection.PlayerID).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				return fmt.Errorf("error checking player with ESPN ID %d: %w", selection.PlayerID, err)
			}
			// Player does not exist, create a new one
			player = models.Player{
				ESPNID:   selection.PlayerID,
				Name:     selection.PlayerName,
				Position: selection.PlayerPosition,
			}
			if createErr := database.DB.Create(&player).Error; createErr != nil {
				return fmt.Errorf("error creating new player with ESPN ID %d: %w", selection.PlayerID, createErr)
			}
			logging.Infof("Created new player: %+v", player)
		}

		entry := &models.DraftSelection{
			PlayerName:     selection.PlayerName,
			PlayerID:       player.ID,
			PlayerPosition: selection.PlayerPosition,
			Round:          uint(selection.Round),
			Pick:           uint(selection.Pick),
			Year:           uint(selection.Year),
			LeagueID:       leagueID,
		}

		for _, team := range teams {
			if team.ESPNID == selection.OwnerESPNID {
				entry.TeamID = team.ID
				break
			}
		}

		// Check if the draft selection already exists
		var existingSelection models.DraftSelection
		if err := database.DB.First(&existingSelection, "player_id = ? AND team_id = ? AND year = ?", selection.PlayerID, selection.OwnerESPNID, selection.Year).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				return fmt.Errorf("error checking existing draft selection for player ID %d and owner ESPN ID %d: %w", selection.PlayerID, selection.OwnerESPNID, err)
			}
			// Draft selection does not exist, create a new one
			if createErr := database.DB.Create(entry).Error; createErr != nil {
				return fmt.Errorf("error creating new draft selection for player ID %d: %w", selection.PlayerID, createErr)
			}
			logging.Infof("Created new draft selection: %+v", entry)
		} else {
			// Draft selection exists, update its details
			existingSelection.PlayerName = selection.PlayerName
			existingSelection.PlayerPosition = selection.PlayerPosition
			existingSelection.Round = uint(selection.Round)
			existingSelection.Pick = uint(selection.Pick)
			if err := database.DB.Save(&existingSelection).Error; err != nil {
				return fmt.Errorf("error updating existing draft selection for player ID %d: %w", selection.PlayerID, err)
			}
			logging.Infof("Updated existing draft selection: %+v", existingSelection)
		}
	}

	return nil
}

type Matchup struct {
	Week                       uint           `json:"week"`
	Year                       uint           `json:"year"`
	HomeTeamESPNID             uint           `json:"home_team_espn_id"`
	AwayTeamESPNID             uint           `json:"away_team_espn_id"`
	HomeTeamFinalScore         float64        `json:"home_team_final_score"`
	AwayTeamFinalScore         float64        `json:"away_team_final_score"`
	HomeTeamESPNProjectedScore float64        `json:"home_team_espn_projected_score"`
	AwayTeamESPNProjectedScore float64        `json:"away_team_espn_projected_score"`
	Completed                  bool           `json:"completed"`
	GameType                   string         `json:"game_type"`
	HomeTeamLineup             []PlayerLineup `json:"home_team_lineup"`
	AwayTeamLineup             []PlayerLineup `json:"away_team_lineup"`
}

type PlayerLineup struct {
	SlotPosition    string                 `json:"slot_position"`
	Points          float64                `json:"points"`
	ProjectedPoints float64                `json:"projected_points"`
	ProOpponent     string                 `json:"pro_opponent"`
	ProPositionRank int                    `json:"pro_pos_rank"`
	GamePlayed      int                    `json:"game_played"`
	GameDate        string                 `json:"game_date"`
	OnByeWeek       bool                   `json:"on_bye_week"`
	ActiveStatus    string                 `json:"active_status"`
	PlayerID        int64                  `json:"player_id"`
	PlayerName      string                 `json:"name"`
	EligibleSlots   []string               `json:"eligible_slots"`
	ProTeam         string                 `json:"pro_team"`
	OnTeamID        int64                  `json:"on_team_id"`
	Injured         bool                   `json:"injured"`
	InjuryStatus    string                 `json:"injury_status"`
	PercentOwned    float64                `json:"percent_owned"`
	PercentStarted  float64                `json:"percent_started"`
	Stats           map[string]WeeklyStats `json:"stats"`
}

type WeeklyStats struct {
	ProjectedPoints    float64                   `json:"projected_points"`
	ProjectedBreakdown map[BreakdownKeys]float64 `json:"projected_breakdown"`
	ProjectedAvgPoints float64                   `json:"projected_avg_points"`
	Breakdown          map[BreakdownKeys]float64 `json:"breakdown"`
	AvgPoints          float64                   `json:"avg_points"`
}

type BreakdownKeys string

const (
	PassingYards               BreakdownKeys = "passingYards"
	PassingAttempts            BreakdownKeys = "passingAttempts"
	PassingCompletions         BreakdownKeys = "passingCompletions"
	PassingIncompletions       BreakdownKeys = "passingIncompletions"
	PassingTouchdowns          BreakdownKeys = "passingTouchdowns"
	Passing2PointConversions   BreakdownKeys = "passing2PtConversions"
	RushingAttempts            BreakdownKeys = "rushingAttempts"
	RushingYards               BreakdownKeys = "rushingYards"
	RushingTouchdowns          BreakdownKeys = "rushingTouchdowns"
	Rushing2PointConversions   BreakdownKeys = "rushing2PtConversions"
	ReceivingYards             BreakdownKeys = "receivingYards"
	ReceivingReceptions        BreakdownKeys = "receivingReceptions"
	ReceivingTargets           BreakdownKeys = "receivingTargets"
	ReceivingTouchdowns        BreakdownKeys = "receivingTouchdowns"
	Receiving2PointConversions BreakdownKeys = "receiving2PtConversions"
	Fumbles                    BreakdownKeys = "fumbles"
	LostFumbles                BreakdownKeys = "lostFumbles"
	Turnovers                  BreakdownKeys = "turnovers"
	FieldGoalsMade             BreakdownKeys = "madeFieldGoals"
	FieldGoalsAttempted        BreakdownKeys = "attemptedFieldGoals"
	ExtraPointsMade            BreakdownKeys = "madeExtraPoints"
	ExtraPointsAttempted       BreakdownKeys = "attemptedExtraPoints"
)

// processMatchups processes matchups data
func processMatchups(filePath string) error {
	logging.Infof("Processing matchups data from: %s", filePath)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read matchups file %s: %w", filePath, err)
	}
	matchupsMarshalled := []Matchup{}
	if err := json.Unmarshal(data, &matchupsMarshalled); err != nil {
		return fmt.Errorf("failed to unmarshal matchups data from %s: %w", filePath, err)
	}

	matchups := make([]*models.Matchup, len(matchupsMarshalled))
	for i, matchup := range matchupsMarshalled {
		matchups[i] = &models.Matchup{
			Week:                       matchup.Week,
			Year:                       matchup.Year,
			HomeTeamID:                 matchup.HomeTeamESPNID,
			AwayTeamID:                 matchup.AwayTeamESPNID,
			HomeTeamFinalScore:         matchup.HomeTeamFinalScore,
			AwayTeamFinalScore:         matchup.AwayTeamFinalScore,
			HomeTeamESPNProjectedScore: matchup.HomeTeamESPNProjectedScore,
			AwayTeamESPNProjectedScore: matchup.AwayTeamESPNProjectedScore,
			Completed:                  matchup.Completed,
			GameType:                   matchup.GameType,
		}
	}

	if err := simulation.CalculateExpectedWins(matchups); err != nil {
		return fmt.Errorf("failed to calculate expected wins: %w", err)
	}

	// Need to get the teams to get the {Home,Away}TeamID mappings
	teams := []models.Team{}
	if err := database.DB.Find(&teams, "league_id = ?", leagueID).Error; err != nil {
		return fmt.Errorf("failed to retrieve teams for league ID %d: %w", leagueID, err)
	}

	if len(teams) == 0 {
		return fmt.Errorf("no teams found for league ID %d", leagueID)
	}

	idMap := make(map[uint]uint)
	for _, team := range teams {
		idMap[team.ESPNID] = team.ID
	}

	logging.Infof("Successfully processed %d matchups from %s", len(matchups), filePath)
	for idx, matchup := range matchups {
		logging.Debugf("Matchup - Week: %d, Year: %d, Home Team ESPN ID: %d, Away Team ESPN ID: %d, Home Score: %.2f, Away Score: %.2f, Completed: %t",
			matchup.Week, matchup.Year, matchup.HomeTeamID, matchup.AwayTeamID,
			matchup.HomeTeamFinalScore, matchup.AwayTeamFinalScore, matchup.Completed)
		logging.Debugf("Home Team Projected Score: %.2f, Away Team Projected Score: %.2f",
			matchup.HomeTeamESPNProjectedScore, matchup.AwayTeamESPNProjectedScore)

		logging.Infof("%+v", idMap)

		// Look up the internal database IDs using the ESPN IDs
		homeTeamID, homeTeamExists := idMap[uint(matchup.HomeTeamID)]
		if !homeTeamExists {
			return fmt.Errorf("home team with ESPN ID %d not found in database", matchup.HomeTeamID)
		}

		awayTeamID, awayTeamExists := idMap[uint(matchup.AwayTeamID)]
		if !awayTeamExists {
			return fmt.Errorf("away team with ESPN ID %d not found in database", matchup.AwayTeamID)
		}

		entry := &models.Matchup{
			LeagueID:                   leagueID,
			Week:                       uint(matchup.Week),
			Year:                       uint(matchup.Year),
			Season:                     int(matchup.Year),
			HomeTeamID:                 homeTeamID, // Use mapped internal ID instead of ESPN ID
			AwayTeamID:                 awayTeamID, // Use mapped internal ID instead of ESPN ID
			HomeTeamFinalScore:         matchup.HomeTeamFinalScore,
			AwayTeamFinalScore:         matchup.AwayTeamFinalScore,
			HomeTeamESPNProjectedScore: matchup.HomeTeamESPNProjectedScore,
			AwayTeamESPNProjectedScore: matchup.AwayTeamESPNProjectedScore,
			GameType:                   matchup.GameType,

			Completed: matchup.Completed,
			IsPlayoff: false, // TODO: implement playoff logic
		}

		// Check if the matchup already exists
		var existingMatchup models.Matchup
		if err := database.DB.First(&existingMatchup, "home_team_id = ? AND away_team_id = ? AND week = ? AND year = ?",
			homeTeamID, awayTeamID, matchup.Week, matchup.Year).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				return fmt.Errorf("error checking existing matchup for home team ID %d and away team ID %d: %w",
					homeTeamID, awayTeamID, err)
			}
			// Matchup does not exist, create a new one
			if createErr := database.DB.Create(entry).Error; createErr != nil {
				return fmt.Errorf("error creating new matchup for home team ID %d: %w", homeTeamID, createErr)
			}
			logging.Infof("Created new matchup: %+v", entry)
		} else {
			// Matchup exists, update its details
			existingMatchup.HomeTeamFinalScore = matchup.HomeTeamFinalScore
			existingMatchup.AwayTeamFinalScore = matchup.AwayTeamFinalScore
			existingMatchup.HomeTeamESPNProjectedScore = matchup.HomeTeamESPNProjectedScore
			existingMatchup.AwayTeamESPNProjectedScore = matchup.AwayTeamESPNProjectedScore
			existingMatchup.Completed = matchup.Completed
			existingMatchup.GameType = matchup.GameType
			existingMatchup.Week = uint(matchup.Week)
			existingMatchup.Year = uint(matchup.Year)
			existingMatchup.Season = int(matchup.Year)

			if err := database.DB.Save(&existingMatchup).Error; err != nil {
				return fmt.Errorf("error updating existing matchup for home team ESPN ID %d: %w", matchup.HomeTeamID, err)
			}
			logging.Infof("Updated existing matchup: %+v", existingMatchup)
		}

		if existingMatchup.ID == 0 {
			// If the matchup was just created, use the new ID
			existingMatchup.ID = entry.ID
		}

		// Process home team lineup
		for _, player := range matchupsMarshalled[idx].HomeTeamLineup {
			if err := processPlayerLineUp(player, entry.HomeTeamID, existingMatchup.ID, matchup.Week, matchup.Year); err != nil {
				return fmt.Errorf("error processing home team player lineup for player %s: %w", player.PlayerName, err)
			}
		}

		// Process away team lineup
		for _, player := range matchupsMarshalled[idx].AwayTeamLineup {
			if err := processPlayerLineUp(player, entry.AwayTeamID, existingMatchup.ID, matchup.Week, matchup.Year); err != nil {
				return fmt.Errorf("error processing home team player lineup for player %s: %w", player.PlayerName, err)
			}
		}
	}

	return nil
}

func processPlayerLineUp(player PlayerLineup, teamID, matchupID, week, year uint) error {
	logging.Infof("Processing player lineup - Name: %s, ID: %d, Position: %s, Points: %.2f, Projected Points: %.2f",
		player.PlayerName, player.PlayerID, player.SlotPosition, player.Points, player.ProjectedPoints)

	if teamID == 0 || matchupID == 0 || week == 0 || year == 0 || player.PlayerID == 0 {
		return fmt.Errorf("invalid parameters: teamID=%d, matchupID=%d, week=%d, year=%d", teamID, matchupID, week, year)
	}

	// Check if player exists first to get the correct position
	var existingPlayer models.Player
	playerExists := database.DB.First(&existingPlayer, "espn_id = ?", player.PlayerID).Error == nil

	// Create or update the player in the database
	playerRecord := &models.Player{
		Name:   player.PlayerName,
		ESPNID: player.PlayerID,
	}

	// If player doesn't exist, we need to determine their actual position
	// SlotPosition could be "FLEX", "BE", etc., but we need the actual position
	if !playerExists || existingPlayer.Position == "" {
		// Try to determine position from eligible slots if available
		actualPosition := ""
		if len(player.EligibleSlots) > 0 {
			// Use the first eligible slot that's not a flex position
			valid := map[string]bool{
				"QB":   true,
				"RB":   true,
				"WR":   true,
				"TE":   true,
				"K":    true,
				"D/ST": true,
			}
			for _, slot := range player.EligibleSlots {
				if valid[strings.ToUpper(slot)] {
					actualPosition = slot
					break
				}
			}
		}

		// If we couldn't determine from eligible slots, use slot position but clean it up
		if actualPosition == "" {
			slotPos := strings.ToUpper(player.SlotPosition)
			// Map common slot positions to actual positions
			switch slotPos {
			case "FLEX", "BE", "IR":
				// For these cases, we can't determine the position from slot alone
				// We'll need to leave it empty and update it later when we have better data
				actualPosition = ""
			default:
				actualPosition = slotPos
			}
		}

		playerRecord.Position = actualPosition

		if err := database.DB.Model(playerRecord).Where("espn_id = ?", player.PlayerID).Updates(map[string]interface{}{
			"position": playerRecord.Position,
		}).Error; err != nil {
			return fmt.Errorf("error updating position for player with ESPN ID %d: %w", player.PlayerID, err)
		}
	}

	if err := database.DB.First(&playerRecord, "espn_id = ?", player.PlayerID).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			return fmt.Errorf("error checking existing player with ESPN ID %d: %w", player.PlayerID, err)
		}
		// Player does not exist, create a new one
		if createErr := database.DB.Create(playerRecord).Error; createErr != nil {
			return fmt.Errorf("error creating new player with ESPN ID %d: %w", player.PlayerID, createErr)
		}
		logging.Infof("Created new player: %+v", playerRecord)
	}

	stats, ok := player.Stats[fmt.Sprintf("%d", week)]
	if !ok && !player.OnByeWeek {
		if len(player.Stats) == 1 {
			for _, value := range player.Stats {
				stats = value
			}
		} else {
			logging.Warnf("No stats found for player %s (ID %d) for week %d", player.PlayerName, player.PlayerID, week)
			return fmt.Errorf("no stats found for player %s (ID %d) for week %d", player.PlayerName, player.PlayerID, week)
		}
	}

	gameStats := models.PlayerStats{}
	if !player.OnByeWeek {
		gameStats = models.PlayerStats{
			PassingYards:   stats.Breakdown[PassingYards],
			PassingTDs:     stats.Breakdown[PassingTouchdowns],
			Interceptions:  stats.Breakdown[Turnovers] - stats.Breakdown[LostFumbles],
			RushingYards:   stats.Breakdown[RushingYards],
			RushingTDs:     stats.Breakdown[RushingTouchdowns],
			Receptions:     stats.Breakdown[ReceivingReceptions],
			ReceivingYards: stats.Breakdown[ReceivingYards],
			ReceivingTDs:   stats.Breakdown[ReceivingTouchdowns],
			Fumbles:        stats.Breakdown[Fumbles],
			FieldGoals:     stats.Breakdown[FieldGoalsMade],
			ExtraPoints:    stats.Breakdown[ExtraPointsMade],
		}
	}

	// Create a new BoxScore associated with the player and team
	boxScore := &models.BoxScore{
		MatchupID: matchupID,
		PlayerID:  playerRecord.ID,
		TeamID:    teamID,
		Week:      week,
		Year:      year,

		ActualPoints:    player.Points,
		ProjectedPoints: player.ProjectedPoints,
		GameStats:       gameStats,
	}

	// Check if the box score already exists
	var existingBoxScore models.BoxScore
	if err := database.DB.First(&existingBoxScore, "matchup_id = ? AND player_id = ? AND week = ? AND year = ?",
		matchupID, playerRecord.ID, week, year).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			return fmt.Errorf("error checking existing box score for player ID %d: %w", playerRecord.ID, err)
		}
		// Box score does not exist, create a new one
		if createErr := database.DB.Create(boxScore).Error; createErr != nil {
			return fmt.Errorf("error creating new box score for player ID %d: %w", playerRecord.ID, createErr)
		}
		logging.Infof("Created new box score for player %s (ID %d)", player.PlayerName, playerRecord.ID)
	} else {
		// Box score exists, update its details
		existingBoxScore.ActualPoints = player.Points
		existingBoxScore.ProjectedPoints = player.ProjectedPoints
		existingBoxScore.GameStats = models.PlayerStats{
			PassingYards:   stats.Breakdown[PassingYards],
			PassingTDs:     stats.Breakdown[PassingTouchdowns],
			Interceptions:  stats.Breakdown[Turnovers] - stats.Breakdown[LostFumbles],
			RushingYards:   stats.Breakdown[RushingYards],
			RushingTDs:     stats.Breakdown[RushingTouchdowns],
			Receptions:     stats.Breakdown[ReceivingReceptions],
			ReceivingYards: stats.Breakdown[ReceivingYards],
			ReceivingTDs:   stats.Breakdown[ReceivingTouchdowns],
			Fumbles:        stats.Breakdown[Fumbles],
			FieldGoals:     stats.Breakdown[FieldGoalsMade],
			ExtraPoints:    stats.Breakdown[ExtraPointsMade],
		}
		if err := database.DB.Save(&existingBoxScore).Error; err != nil {
			return fmt.Errorf("error updating existing box score for player ID %d: %w", playerRecord.ID, err)
		}
		logging.Infof("Updated existing box score for player %s (ID %d)", player.PlayerName, playerRecord.ID)
	}

	logging.Infof("Processed player lineup for %s (ID %d)", player.PlayerName, player.PlayerID)
	return nil
}

type Team struct {
	ESPNID   int64  `json:"espn_id"`
	Owner    string `json:"owner"`
	Nickname string `json:"team_name"`
	Year     int    `json:"year"`
}

// processTeams processes teams data
func processTeams(filePath string) error {
	logging.Infof("Processing teams data from: %s", filePath)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read teams file %s: %w", filePath, err)
	}

	teams := []Team{}
	if err := json.Unmarshal(data, &teams); err != nil {
		return fmt.Errorf("failed to unmarshal teams data from %s: %w", filePath, err)
	}
	logging.Infof("Successfully processed %d teams from %s", len(teams), filePath)

	for _, team := range teams {
		logging.Infof("Team - ESPN ID: %d, Owner: %s, Nickname: %s, Year: %d",
			team.ESPNID, team.Owner, team.Nickname, team.Year)

		teamRecord := &models.Team{
			LeagueID: leagueID,
			ESPNID:   uint(team.ESPNID),
		}

		// Check if team already exists
		var existingTeam models.Team
		if err := database.DB.First(&existingTeam, "espn_id = ?", team.ESPNID).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				return fmt.Errorf("error checking existing team with ESPN ID %d: %w", team.ESPNID, err)
			}
			// Team does not exist, create a new one
			teamRecord.Name = team.Nickname
			teamRecord.Owner = team.Owner
			if createErr := database.DB.Session(&gorm.Session{}).Create(teamRecord).Error; createErr != nil {
				return fmt.Errorf("error creating new team with ESPN ID %d: %w", team.ESPNID, createErr)
			}
			logging.Infof("Created new team: %+v", teamRecord)
		} else {
			// Team exists, update its details
			existingTeam.Name = team.Nickname
			existingTeam.Owner = team.Owner
			if err := database.DB.Save(&existingTeam).Error; err != nil {
				return fmt.Errorf("error updating existing team with ESPN ID %d: %w", team.ESPNID, err)
			}
			logging.Infof("Updated existing team: %+v", existingTeam)
		}
	}

	return nil
}

// Transaction represents a fantasy football transaction record
type Transaction struct {
	TeamESPNID      int       `json:"team_espn_id"`
	PlayerID        int       `json:"player_id"`
	TransactionType string    `json:"transaction_type"`
	PlayerName      string    `json:"player_name"`
	PlayerPosition  string    `json:"player_position"`
	BidAmount       int       `json:"bid_amount"`
	Date            time.Time `json:"date"`
	Year            int       `json:"year"`
}

// processTransactions processes transactions data
func processTransactions(filePath string) error {
	logging.Infof("Processing transactions data from: %s", filePath)

	// Read the file content
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading transactions file: %w", err)
	}

	// Parse JSON data into a slice of transactions
	var transactions []Transaction

	// Create a temporary struct to parse the date as string first
	type tempTransaction struct {
		TeamESPNID      int    `json:"team_espn_id"`
		PlayerID        int    `json:"player_id"`
		TransactionType string `json:"transaction_type"`
		PlayerName      string `json:"player_name"`
		PlayerPosition  string `json:"player_position"`
		BidAmount       int    `json:"bid_amount"`
		Date            string `json:"date"`
		Year            int    `json:"year"`
	}

	var tempTransactions []tempTransaction
	if err := json.Unmarshal(data, &tempTransactions); err != nil {
		return fmt.Errorf("error unmarshalling transactions JSON: %w", err)
	}

	// Process each transaction and parse the date
	for _, t := range tempTransactions {
		// Parse the date string into a time.Time
		// The date format is "2024-12-29 09:41:11"
		parsedDate, err := time.Parse("2006-01-02 15:04:05", t.Date)
		if err != nil {
			return fmt.Errorf("error parsing date %s: %w", t.Date, err)
		}

		// Create a Transaction with properly parsed date
		transaction := Transaction{
			TeamESPNID:      t.TeamESPNID,
			PlayerID:        t.PlayerID,
			TransactionType: t.TransactionType,
			PlayerName:      t.PlayerName,
			PlayerPosition:  t.PlayerPosition,
			BidAmount:       t.BidAmount,
			Date:            parsedDate,
			Year:            t.Year,
		}

		transactions = append(transactions, transaction)
	}

	logging.Infof("Successfully processed %d transactions", len(transactions))

	for _, t := range transactions {
		logging.Infof("Transaction - Team ESPN ID: %d, Player ID: %d, Type: %s, Player Name: %s, Player Position: %s, Bid Amount: %d, Date: %s, Year: %d",
			t.TeamESPNID, t.PlayerID, t.TransactionType, t.PlayerName, t.PlayerPosition, t.BidAmount,
			t.Date.Format("2006-01-02 15:04:05"), t.Year)

		// Get the player by ESPN ID
		var player models.Player
		if err := database.DB.First(&player, "espn_id = ?", t.PlayerID).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				return fmt.Errorf("error checking player with ESPN ID %d: %w", t.PlayerID, err)
			}
			// Player does not exist, create a new one
			player = models.Player{
				ESPNID:   int64(t.PlayerID),
				Name:     t.PlayerName,
				Position: t.PlayerPosition, // Now use the position from transaction data
			}
			if createErr := database.DB.Create(&player).Error; createErr != nil {
				return fmt.Errorf("error creating new player with ESPN ID %d: %w", t.PlayerID, createErr)
			}
			logging.Infof("Created new player: %+v", player)
		}

		// Get the team by ESPN ID
		var team models.Team
		if err := database.DB.First(&team, "espn_id = ?", t.TeamESPNID).Error; err != nil {
			return fmt.Errorf("error checking team with ESPN ID %d: %w", t.TeamESPNID, err)
		}

		transactionsRecord := &models.Transaction{
			TeamID:          team.ID,
			PlayerID:        player.ID,
			TransactionType: t.TransactionType,
			PlayerName:      t.PlayerName,
			BidAmount:       t.BidAmount,
			Date:            t.Date,
			Year:            uint(t.Year),
			LeagueID:        team.LeagueID,
		}

		// Check if the transaction already exists
		var existingTransaction models.Transaction
		if err := database.DB.First(&existingTransaction, "team_id = ? AND player_id = ? AND date = ?", t.TeamESPNID, t.PlayerID, t.Date).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				return fmt.Errorf("error checking existing transaction for team ESPN ID %d and player ID %d: %w", t.TeamESPNID, t.PlayerID, err)
			}
			// Transaction does not exist, create a new one
			if createErr := database.DB.Create(transactionsRecord).Error; createErr != nil {
				return fmt.Errorf("error creating new transaction for team ESPN ID %d: %w", t.TeamESPNID, createErr)
			}
			logging.Infof("Created new transaction: %+v", transactionsRecord)
		} else {
			// Transaction exists, update its details
			existingTransaction.TransactionType = t.TransactionType
			existingTransaction.PlayerName = t.PlayerName
			existingTransaction.BidAmount = t.BidAmount
			existingTransaction.Date = t.Date
			if err := database.DB.Save(&existingTransaction).Error; err != nil {
				return fmt.Errorf("error updating existing transaction for team ESPN ID %d: %w", t.TeamESPNID, err)
			}
			logging.Infof("Updated existing transaction: %+v", existingTransaction)
		}
		logging.Infof("Processed transaction for team ESPN ID %d, player ID %d", t.TeamESPNID, t.PlayerID)
	}

	return nil
}

func Upload(directory string) error {
	// Read files from the specified directory
	if directory == "" {
		return fmt.Errorf("data directory cannot be empty")
	}

	files, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", directory, err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}
	if err := database.Initialize(cfg); err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}

	// First, ensure the league exists
	var league models.League
	if err := database.DB.First(&league, leagueID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create the league if it doesn't exist
			league = models.League{
				ID:          leagueID,
				Name:        "Fantasy Football League",
				Description: "Default league created by ETL process",
				ScoringType: "PPR", // Assuming PPR scoring
				Season:      2024,  // Using the default year from the filename
				CurrentWeek: 1,
			}
			if createErr := database.DB.Create(&league).Error; createErr != nil {
				return fmt.Errorf("error creating league with ID %d: %w", leagueID, createErr)
			}
			logging.Infof("Created new league with ID %d", leagueID)
		} else {
			return fmt.Errorf("error checking if league with ID %d exists: %w", leagueID, err)
		}
	} else {
		logging.Infof("Found existing league: %s (ID: %d)", league.Name, league.ID)
	}

	// Regex to extract file type from filename (pattern: {type}_{year}.json)
	re := regexp.MustCompile(`^(.+)_\d{4}\.json$`)

	// First have to create the teams
	for _, file := range files {
		if file.IsDir() {
			continue // Skip directories
		}
		filePath := fmt.Sprintf("%s/%s", directory, file.Name())
		logging.Infof("Processing file: %s", filePath)

		// Extract file type using regex
		matches := re.FindStringSubmatch(file.Name())
		if len(matches) < 2 {
			logging.Infof("Warning: File %s does not match the expected format, skipping", file.Name())
			continue
		}

		fileType := matches[1]
		fileType = strings.Replace(fileType, "-", "_", -1) // Normalize for switch statement

		if fileType == "teams" {
			logging.Infof("Processing teams file: %s", filePath)
			if processErr := processTeams(filePath); processErr != nil {
				return fmt.Errorf("error processing file %s: %w", filePath, processErr)
			}
		}
	}

	for _, file := range files {
		if file.IsDir() {
			continue // Skip directories
		}
		filePath := fmt.Sprintf("%s/%s", directory, file.Name())
		logging.Infof("Processing file: %s", filePath)

		// Extract file type using regex
		matches := re.FindStringSubmatch(file.Name())
		if len(matches) < 2 {
			logging.Infof("Warning: File %s does not match the expected format, skipping", file.Name())
			continue
		}

		fileType := matches[1]
		fileType = strings.Replace(fileType, "-", "_", -1) // Normalize for switch statement

		// Process based on file type
		var processErr error
		switch fileType {
		// case "box_score_players":
		// 	processErr = processBoxScorePlayers(filePath)
		// case "draft_selections":
		// 	processErr = processDraftSelections(filePath)
		case "matchups":
			processErr = processMatchups(filePath)
		// case "transactions":
		// 	processErr = processTransactions(filePath)
		default:
			logging.Warnf("Unrecognized file type %s in file %s, skipping", fileType, file.Name())
			continue
		}

		if processErr != nil {
			return fmt.Errorf("error processing file %s: %w", filePath, processErr)
		}
	}

	return nil
}
