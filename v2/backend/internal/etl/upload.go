package etl

import (
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/logging"
	"backend/internal/models"
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
	OwnerESPNID    int64  `json:"owner_espn_id"`
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

	for _, selection := range draftSelections {
		logging.Debugf("Draft Selection - Player: %s, ID: %d, Position: %s, Owner ESPN ID: %d, Round: %d, Pick: %d, Year: %d",
			selection.PlayerName, selection.PlayerID, selection.PlayerPosition,
			selection.OwnerESPNID, selection.Round, selection.Pick, selection.Year)
	}

	return nil
}

type Matchup struct {
	Week                       int            `json:"week"`
	Year                       int            `json:"year"`
	HomeTeamESPNID             int64          `json:"home_team_espn_id"`
	AwayTeamESPNID             int64          `json:"away_team_espn_id"`
	HomeTeamFinalScore         float64        `json:"home_team_final_score"`
	AwayTeamFinalScore         float64        `json:"away_team_final_score"`
	HomeTeamEspnProjectedScore float64        `json:"home_team_espn_projected_score"`
	AwayTeamEspnProjectedScore float64        `json:"away_team_espn_projected_score"`
	Completed                  bool           `json:"completed"`
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
	ProjectedPoints    float64            `json:"projected_points"`
	ProjectedBreakdown map[string]float64 `json:"projected_breakdown"`
	ProjectedAvgPoints float64            `json:"projected_avg_points"`
	Breakdown          map[string]float64 `json:"breakdown"`
	AvgPoints          float64            `json:"avg_points"`
}

// processMatchups processes matchups data
func processMatchups(filePath string) error {
	logging.Infof("Processing matchups data from: %s", filePath)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read matchups file %s: %w", filePath, err)
	}
	matchups := []Matchup{}
	if err := json.Unmarshal(data, &matchups); err != nil {
		return fmt.Errorf("failed to unmarshal matchups data from %s: %w", filePath, err)
	}
	logging.Infof("Successfully processed %d matchups from %s", len(matchups), filePath)
	for _, matchup := range matchups {
		logging.Debugf("Matchup - Week: %d, Year: %d, Home Team ESPN ID: %d, Away Team ESPN ID: %d, Home Score: %.2f, Away Score: %.2f, Completed: %t",
			matchup.Week, matchup.Year, matchup.HomeTeamESPNID, matchup.AwayTeamESPNID,
			matchup.HomeTeamFinalScore, matchup.AwayTeamFinalScore, matchup.Completed)
		logging.Debugf("Home Team Projected Score: %.2f, Away Team Projected Score: %.2f",
			matchup.HomeTeamEspnProjectedScore, matchup.AwayTeamEspnProjectedScore)
		logging.Debugf("Home Team Lineup: %+v", matchup.HomeTeamLineup)
		logging.Debugf("Away Team Lineup: %+v", matchup.AwayTeamLineup)
	}

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
			BidAmount:       t.BidAmount,
			Date:            parsedDate,
			Year:            t.Year,
		}

		transactions = append(transactions, transaction)
	}

	logging.Infof("Successfully processed %d transactions", len(transactions))
	// TODO: Store transactions in database or perform further processing

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
		case "box_score_players":
			processErr = processBoxScorePlayers(filePath)
		case "draft_selections":
			processErr = processDraftSelections(filePath)
		case "matchups":
			processErr = processMatchups(filePath)
		case "teams":
			processErr = processTeams(filePath)
		case "transactions":
			processErr = processTransactions(filePath)
		default:
			logging.Infof("Warning: Unrecognized file type %s in file %s, skipping", fileType, file.Name())
			continue
		}

		if processErr != nil {
			return fmt.Errorf("error processing file %s: %w", filePath, processErr)
		}
	}

	return nil
}
