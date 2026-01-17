package etl

import (
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/logging"
	"backend/internal/models"
	"backend/internal/simulation"
	"backend/internal/sleeper"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NewUploadOptions struct {
	Directory         string
	MultipleLeagues   bool
	RefreshPlayerData bool // Force refresh from Sleeper API
}

const playersDataFile = "internal/etl/data/players.json"

func NewUpload(opts NewUploadOptions) error {
	logging.Infof("Starting ETL upload from directory: %s", opts.Directory)

	if err := database.Initialize(&config.Config{DB: config.DBConfig{ConnectionString: os.Getenv("DATABASE_URL")}}); err != nil {
		logging.Errorf("Failed to initialize database: %v", err)
	}

	if err := checkPlayerEntries(opts.RefreshPlayerData); err != nil {
		logging.Errorf("Player entries check failed: %v", err)
		return err
	}

	// Read the directory
	var dataFiles []string
	filepath.WalkDir(opts.Directory, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || filepath.Ext(d.Name()) != ".yaml" {
			logging.Debugf("Skipping non-YAML file or directory: %s", path)
			return nil
		}

		dataFiles = append(dataFiles, path)

		return nil
	})

	logging.Infof("Found %d YAML files to process %+v", len(dataFiles), dataFiles)

	for _, dataFile := range dataFiles {
		logging.Infof("Processing file: %s", dataFile)
		if err := uploadYAMLFile(dataFile); err != nil {
			logging.Errorf("Failed to upload data from file %s: %v", dataFile, err)
			return err
		}
	}

	return nil
}

func checkPlayerEntries(forceRefresh bool) error {
	var last_updated models.Player
	if err := database.DB.Model(&models.Player{}).Order("updated_at desc").Limit(1).Find(&last_updated).Error; err != nil {
		return err
	}

	logging.Infof("Last player entry updated at: %v", last_updated.UpdatedAt)

	// Check if data is fresh (exists and less than 2 days old) and not forcing refresh
	twoDaysAgo := time.Now().Add(-48 * time.Hour)
	if !forceRefresh && !last_updated.UpdatedAt.IsZero() && last_updated.UpdatedAt.After(twoDaysAgo) {
		logging.Infof("Player entries are up to date, skipping data fetch")
		return nil
	}

	// Try to load players from local JSON file first
	var players map[string]sleeper.Player
	var err error

	// Check if local file exists and we're not forcing refresh
	if !forceRefresh {
		if _, err := os.Stat(playersDataFile); err == nil {
			logging.Infof("Loading players from local file: %s", playersDataFile)
			players, err = loadPlayersFromFile(playersDataFile)
			if err != nil {
				logging.Warnf("Failed to load players from file, will fetch from API: %v", err)
				players = nil
			} else {
				logging.Infof("Successfully loaded %d players from local file", len(players))
			}
		}
	}

	// If we don't have players from file, fetch from API
	if players == nil || forceRefresh {
		logging.Infof("Fetching players from Sleeper API")
		sleeperClient := sleeper.New()

		players, err = sleeperClient.GetAllPlayers()
		if err != nil {
			return err
		}

		// Save to local file for future use
		if err := savePlayersToFile(playersDataFile, players); err != nil {
			logging.Warnf("Failed to save players to file: %v", err)
		} else {
			logging.Infof("Saved %d players to %s", len(players), playersDataFile)
		}
	}

	// Process players in batches to avoid memory issues
	batchSize := 500
	playersList := make([]models.Player, 0, batchSize)
	count := 0

	for _, player := range players {
		dbPlayer, err := player.ToDBPlayer()
		if err != nil {
			logging.Errorf("Failed to convert Sleeper player to DB player: %v", err)
			continue
		}
		playersList = append(playersList, *dbPlayer)
		count++

		// Insert batch when we reach batch size
		if len(playersList) >= batchSize {
			if err := upsertPlayerBatch(playersList); err != nil {
				logging.Errorf("Failed to upsert player batch: %v", err)
				return err
			}
			logging.Infof("Upserted %d players (total: %d)", len(playersList), count)
			playersList = make([]models.Player, 0, batchSize)
		}
	}

	// Insert remaining players
	if len(playersList) > 0 {
		if err := upsertPlayerBatch(playersList); err != nil {
			logging.Errorf("Failed to upsert final player batch: %v", err)
			return err
		}
		logging.Infof("Upserted final %d players (total: %d)", len(playersList), count)
	}

	logging.Infof("Successfully upserted %d players", count)

	return nil
}

// loadPlayersFromFile reads player data from a JSON file
func loadPlayersFromFile(filepath string) (map[string]sleeper.Player, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	var players map[string]sleeper.Player
	if err := json.Unmarshal(data, &players); err != nil {
		return nil, err
	}

	return players, nil
}

// savePlayersToFile writes player data to a JSON file
func savePlayersToFile(filepath string, players map[string]sleeper.Player) error {
	data, err := json.MarshalIndent(players, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath, data, 0644)
}

func upsertPlayerBatch(players []models.Player) error {
	// Use ON CONFLICT to update existing records or insert new ones
	return database.DB.Model(&models.Player{}).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "sleeper_id"}}, // The unique constraint column
		DoUpdates: clause.AssignmentColumns([]string{
			// Update all fields except ID and timestamps on conflict
			"espn_id", "yahoo_id", "fantasy_data_id", "rotoworld_id", "rotowire_id",
			"sportsradar_id", "stats_id", "first_name", "last_name", "name", "number",
			"hashtag", "position", "fantasy_positions", "team", "status", "sport",
			"age", "height", "weight", "college", "years_exp", "birth_country",
			"depth_chart_position", "depth_chart_order", "search_rank",
			"injury_status", "injury_start_date", "practice_participation",
		}),
	}).Create(&players).Error
}

func uploadYAMLFile(filePath string) error {
	var league ETLLeague
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, &league); err != nil {
		return err
	}

	// 1. Ensure league exists and get its database ID
	var dbLeague models.League
	if err := database.DB.Where("league_id = ? AND source = ?", league.ID, "espn").First(&dbLeague).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create the league
			dbLeague = models.League{
				LeagueID: league.ID,
				Source:   "espn",
			}
			if err := database.DB.Create(&dbLeague).Error; err != nil {
				logging.Errorf("Failed to create league: %v", err)
				return err
			}
			logging.Infof("Created new league with DB ID %d for ESPN league %d", dbLeague.ID, league.ID)
		} else {
			logging.Errorf("Failed to query league: %v", err)
			return err
		}
	}
	logging.Infof("Using database league ID: %d for ESPN league ID: %d", dbLeague.ID, league.ID)

	// 2. Ensure teams exist
	for _, etlTeam := range league.Teams {
		teamID := uint(etlTeam.ESPNID)
		dbTeam := models.Team{
			Name:     etlTeam.Name,
			TeamID:   &teamID,     // Pointer for nullable field
			LeagueID: dbLeague.ID, // Use database PK, not ESPN ID
			Owners:   etlTeam.Owners,
		}
		err := database.DB.Model(&models.Team{}).
			Where("league_id = ? AND team_id = ?", dbLeague.ID, teamID).
			FirstOrCreate(&dbTeam).Error
		if err != nil {
			logging.Errorf("Failed to ensure team exists: %v", err)
			return err
		}
	}

	// 3. Create team ID mapping (ESPN ID -> Database ID)
	var teams []models.Team
	if err := database.DB.Where("league_id = ?", dbLeague.ID).Find(&teams).Error; err != nil {
		logging.Errorf("Failed to fetch teams for matchup creation: %v", err)
		return err
	}

	teamIDMap := make(map[uint]uint) // ESPN ID -> Database ID
	for _, team := range teams {
		if team.TeamID != nil {
			teamIDMap[*team.TeamID] = team.ID
		}
	}

	// 4. Create all matchups
	for _, etlMatchup := range league.Schedule.Matchups {
		homeTeamDBID, homeExists := teamIDMap[etlMatchup.HomeTeamID]
		awayTeamDBID, awayExists := teamIDMap[etlMatchup.AwayTeamID]

		if !homeExists || !awayExists {
			logging.Warnf("Skipping matchup week %d, year %d: team mapping not found (home: %d, away: %d)",
				etlMatchup.Week, etlMatchup.Year, etlMatchup.HomeTeamID, etlMatchup.AwayTeamID)
			continue
		}

		dbMatchup := models.Matchup{
			LeagueID:           dbLeague.ID, // Use database PK
			Season:             etlMatchup.Year,
			Week:               etlMatchup.Week,
			HomeTeamID:         homeTeamDBID, // Use database internal ID
			AwayTeamID:         awayTeamDBID, // Use database internal ID
			GameType:           etlMatchup.GameType,
			IsPlayoff:          etlMatchup.IsPlayoff,
			Completed:          false,
			HomeTeamFinalScore: 0,
			AwayTeamFinalScore: 0,
		}
		err := database.DB.Model(&models.Matchup{}).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "league_id"}, {Name: "season"}, {Name: "week"}, {Name: "home_team_id"}, {Name: "away_team_id"}},
			DoNothing: true,
		}).Create(&dbMatchup).Error
		if err != nil {
			logging.Errorf("Failed to ensure matchup exists: %v", err)
			return err
		}
	}

	// 5. Update matchup scores from boxscores
	for _, etlBoxscore := range league.Schedule.Boxscores {
		// Map ESPN team IDs to database IDs
		homeTeamDBID, homeExists := teamIDMap[etlBoxscore.HomeTeamID]
		awayTeamDBID, awayExists := teamIDMap[etlBoxscore.AwayTeamID]

		if !homeExists || !awayExists {
			logging.Warnf("Skipping boxscore update week %d, year %d: team mapping not found (home: %d, away: %d)",
				etlBoxscore.Week, etlBoxscore.Year, etlBoxscore.HomeTeamID, etlBoxscore.AwayTeamID)
			continue
		}

		// Find the matchup to update
		result := database.DB.Model(&models.Matchup{}).
			Where("league_id = ? AND season = ? AND week = ? AND home_team_id = ? AND away_team_id = ?",
				dbLeague.ID, etlBoxscore.Year, etlBoxscore.Week,
				homeTeamDBID, awayTeamDBID).
			Updates(map[string]interface{}{
				"home_team_final_score":     etlBoxscore.HomeScore,
				"away_team_final_score":     etlBoxscore.AwayScore,
				"home_team_projected_score": etlBoxscore.HomeProjectedScore,
				"away_team_projected_score": etlBoxscore.AwayProjectedScore,
				"completed":                 etlBoxscore.Completed,
			})

		if result.Error != nil {
			logging.Errorf("Failed to update matchup scores for week %d: %v", etlBoxscore.Week, result.Error)
			return result.Error
		}

		if result.RowsAffected == 0 {
			logging.Warnf("No matchup found to update for week %d, year %d (home DB ID: %d, away DB ID: %d)",
				etlBoxscore.Week, etlBoxscore.Year, homeTeamDBID, awayTeamDBID)
			continue
		}
	}

	// 5. Recalculate team records from matchup results
	logging.Infof("Recalculating team records for league %d", dbLeague.ID)
	if err := recalculateTeamRecords(database.DB, dbLeague.ID); err != nil {
		logging.Errorf("Failed to recalculate team records: %v", err)
		return err
	}
	logging.Infof("Successfully recalculated team records")

	// 6. Calculate expected wins for matchups
	logging.Infof("Processing expected wins calculations for league %d, year %d", dbLeague.ID, league.Year)
	if err := processExpectedWinsForLeague(database.DB, dbLeague.ID, uint(league.Year)); err != nil {
		// Log warning but don't fail the ETL - expected wins is supplementary data
		logging.Warnf("Failed to calculate expected wins: %v", err)
	} else {
		logging.Infof("Successfully calculated expected wins")
	}

	// 7. Process draft selections
	if len(league.Draft.Selections) > 0 {
		logging.Infof("Processing %d draft selections for year %d", len(league.Draft.Selections), league.Draft.Year)

		// First, get all teams to map ESPN IDs to internal IDs
		var teams []models.Team
		if err := database.DB.Where("league_id = ?", dbLeague.ID).Find(&teams).Error; err != nil {
			logging.Errorf("Failed to fetch teams for draft processing: %v", err)
			return err
		}

		teamIDMap := make(map[int]uint)
		for _, team := range teams {
			if team.TeamID != nil {
				teamIDMap[int(*team.TeamID)] = team.ID
			}
		}

		for _, pick := range league.Draft.Selections {
			// Find the player by ESPN ID first
			var player models.Player
			err := database.DB.Where("espn_id = ?", pick.PlayerID).First(&player).Error

			if err != nil {
				// Player not found by ESPN ID, try to find by name and update ESPN ID
				err = database.DB.Where("LOWER(name) = LOWER(?)", pick.PlayerName).First(&player).Error
				if err != nil {
					// Player still not found - this shouldn't happen after batch import
					logging.Warnf("Player %s (ESPN ID: %d) not found in database, skipping draft pick",
						pick.PlayerName, pick.PlayerID)
					continue
				}

				// Found by name, update the ESPN ID
				player.ESPNID = int64(pick.PlayerID)
				if player.Position == "" {
					player.Position = pick.PlayerPosition
				}
				if err := database.DB.Save(&player).Error; err != nil {
					logging.Errorf("Failed to update player %s with ESPN ID: %v", pick.PlayerName, err)
					return err
				}
				logging.Infof("Updated player %s (ID: %d) with ESPN ID: %d", player.Name, player.ID, pick.PlayerID)
			}

			// Map ESPN team ID to internal team ID
			internalTeamID, exists := teamIDMap[pick.TeamID]
			if !exists {
				logging.Warnf("Team with ESPN ID %d not found for draft pick, skipping", pick.TeamID)
				continue
			}

			// Check if draft selection already exists
			var existingSelection models.DraftSelection
			err = database.DB.Where("league_id = ? AND year = ? AND round = ? AND pick = ?",
				dbLeague.ID, league.Draft.Year, pick.Round, pick.Pick).First(&existingSelection).Error

			if err != nil {
				// Draft selection doesn't exist, create it
				draftSelection := models.DraftSelection{
					PlayerID:       player.ID,
					PlayerName:     pick.PlayerName,
					PlayerPosition: pick.PlayerPosition,
					TeamID:         internalTeamID,
					Round:          uint(pick.Round),
					Pick:           uint(pick.Pick),
					Year:           uint(league.Draft.Year),
					LeagueID:       dbLeague.ID, // Use database PK
				}

				if err := database.DB.Create(&draftSelection).Error; err != nil {
					logging.Errorf("Failed to create draft selection for player %s: %v", pick.PlayerName, err)
					return err
				}
				logging.Debugf("Created draft pick: Round %d, Pick %d - %s to Team %d",
					pick.Round, pick.Pick, pick.PlayerName, internalTeamID)
			} else {
				// Draft selection exists, update it
				existingSelection.PlayerID = player.ID
				existingSelection.PlayerName = pick.PlayerName
				existingSelection.PlayerPosition = pick.PlayerPosition
				existingSelection.TeamID = internalTeamID

				if err := database.DB.Save(&existingSelection).Error; err != nil {
					logging.Errorf("Failed to update draft selection for player %s: %v", pick.PlayerName, err)
					return err
				}
				logging.Debugf("Updated draft pick: Round %d, Pick %d - %s to Team %d",
					pick.Round, pick.Pick, pick.PlayerName, internalTeamID)
			}
		}

		logging.Infof("Successfully processed %d draft selections", len(league.Draft.Selections))
	}

	// 8. Process box score player stats (TODO)
	// 9. Create new transactions (TODO)
	// 10. Run simulations (TODO)

	logging.Infof("Successfully processed league ID %d for year %d", league.ID, league.Year)

	return nil
}

// recalculateTeamRecords recalculates wins/losses/ties for all teams
// based on completed regular season matchups
func recalculateTeamRecords(db *gorm.DB, leagueID uint) error {
	// 1. Query all completed regular season matchups
	var matchups []models.Matchup
	err := db.Where("league_id = ? AND completed = ? AND is_playoff = ?",
		leagueID, true, false).
		Find(&matchups).Error
	if err != nil {
		return fmt.Errorf("failed to query matchups: %w", err)
	}

	// 2. Build map of team records
	type TeamRecord struct {
		Wins   int
		Losses int
		Ties   int
	}
	records := make(map[uint]TeamRecord)

	// 3. Count results for each matchup
	for _, m := range matchups {
		homeRec := records[m.HomeTeamID]
		awayRec := records[m.AwayTeamID]

		if m.HomeTeamFinalScore > m.AwayTeamFinalScore {
			// Home team wins
			homeRec.Wins++
			awayRec.Losses++
		} else if m.AwayTeamFinalScore > m.HomeTeamFinalScore {
			// Away team wins
			awayRec.Wins++
			homeRec.Losses++
		} else {
			// Tie game
			homeRec.Ties++
			awayRec.Ties++
		}

		records[m.HomeTeamID] = homeRec
		records[m.AwayTeamID] = awayRec
	}

	// 4. Update all teams in a transaction
	return db.Transaction(func(tx *gorm.DB) error {
		for teamID, rec := range records {
			// Use Exec with raw SQL to avoid GORM "record not found" errors
			result := tx.Exec(
				"UPDATE teams SET wins = ?, losses = ?, ties = ?, updated_at = NOW() WHERE id = ? AND league_id = ?",
				rec.Wins, rec.Losses, rec.Ties, teamID, leagueID,
			)

			if result.Error != nil {
				return fmt.Errorf("failed to update team %d: %w", teamID, result.Error)
			}

			if result.RowsAffected == 0 {
				// Team doesn't exist in this league, skip it
				logging.Warnf("Team %d not found in league %d, skipping record update", teamID, leagueID)
			}
		}
		return nil
	})
}

// processExpectedWinsForLeague calculates and updates expected wins for all matchups in a league/season
func processExpectedWinsForLeague(db *gorm.DB, leagueID uint, year uint) error {
	logging.Infof("Calculating expected wins for league %d, year %d", leagueID, year)

	// 1. Query all completed regular season matchups for this league and year
	var matchups []models.Matchup
	err := db.Where("league_id = ? AND season = ? AND completed = ? AND is_playoff = ?",
		leagueID, year, true, false).Find(&matchups).Error
	if err != nil {
		return fmt.Errorf("failed to query matchups: %w", err)
	}

	if len(matchups) == 0 {
		logging.Infof("No completed matchups found for league %d, year %d, skipping expected wins calculation", leagueID, year)
		return nil
	}

	logging.Infof("Found %d completed matchups to process", len(matchups))

	// Convert to slice of pointers for simulation function
	matchupPtrs := make([]*models.Matchup, len(matchups))
	for i := range matchups {
		matchupPtrs[i] = &matchups[i]
	}

	// 2. Calculate per-matchup expected wins using Monte Carlo simulation
	matchupExpectedWins, err := simulation.CalculateMatchupExpectedWins(matchupPtrs)
	if err != nil {
		return fmt.Errorf("failed to calculate expected wins: %w", err)
	}

	logging.Infof("Calculated expected wins for %d matchups", len(matchupExpectedWins))

	// 3. Update each matchup with calculated probabilities
	// Use a transaction for consistency
	return db.Transaction(func(tx *gorm.DB) error {
		for matchupID, expectedWins := range matchupExpectedWins {
			// Validate probabilities are in valid range [0, 1]
			if expectedWins.HomeExpectedWin < 0.0 || expectedWins.HomeExpectedWin > 1.0 {
				logging.Warnf("Invalid home expected win for matchup %d: %.4f", matchupID, expectedWins.HomeExpectedWin)
				continue
			}
			if expectedWins.AwayExpectedWin < 0.0 || expectedWins.AwayExpectedWin > 1.0 {
				logging.Warnf("Invalid away expected win for matchup %d: %.4f", matchupID, expectedWins.AwayExpectedWin)
				continue
			}

			result := tx.Model(&models.Matchup{}).Where("id = ?", matchupID).
				Updates(map[string]interface{}{
					"home_team_expected_win": expectedWins.HomeExpectedWin,
					"away_team_expected_win": expectedWins.AwayExpectedWin,
				})

			if result.Error != nil {
				return fmt.Errorf("failed to update matchup %d: %w", matchupID, result.Error)
			}

			if result.RowsAffected == 0 {
				logging.Warnf("Matchup %d not found, skipping", matchupID)
			}
		}

		logging.Infof("Successfully updated expected wins for %d matchups", len(matchupExpectedWins))
		return nil
	})
}
