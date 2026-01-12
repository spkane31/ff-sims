package etl

import (
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/logging"
	"backend/internal/models"
	"backend/internal/sleeper"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
	"gorm.io/gorm/clause"
)

type NewUploadOptions struct {
	Directory       string
	MultipleLeagues bool
}

func NewUpload(opts NewUploadOptions) error {
	logging.Infof("Starting ETL upload from directory: %s", opts.Directory)

	if err := database.Initialize(&config.Config{DB: config.DBConfig{ConnectionString: os.Getenv("DATABASE_URL")}}); err != nil {
		logging.Errorf("Failed to initialize database: %v", err)
	}

	if err := checkPlayerEntries(); err != nil {
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

func checkPlayerEntries() error {
	var last_updated models.Player
	if err := database.DB.Model(&models.Player{}).Order("updated_at desc").Limit(1).Find(&last_updated).Error; err != nil {
		return err
	}

	logging.Infof("Last player entry updated at: %v", last_updated.UpdatedAt)

	// Check if data is fresh (exists and less than 2 days old)
	twoDaysAgo := time.Now().Add(-48 * time.Hour)
	if !last_updated.UpdatedAt.IsZero() && last_updated.UpdatedAt.After(twoDaysAgo) {
		logging.Infof("Player entries are up to date, skipping data fetch")
		return nil
	}

	logging.Infof("Player entries are outdated or missing, fetching from Sleeper API")

	sleeperClient := sleeper.New()

	players, err := sleeperClient.GetAllPlayers()
	if err != nil {
		return err
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

	logging.Infof("Successfully upserted %d players from Sleeper API", count)

	return nil
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

	// 1. Ensure league exists
	if err := database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "league_id"}},
		DoNothing: true,
	}).Create(&models.League{LeagueID: league.ID}).Error; err != nil {
		logging.Errorf("Failed to ensure league exists: %v", err)
		return err
	}

	// 2. Ensure teams exist
	for _, etlTeam := range league.Teams {
		dbTeam := models.Team{
			Name:     etlTeam.Name,
			ESPNID:   uint(etlTeam.ESPNID),
			LeagueID: league.ID,
			Wins:     0,
			Losses:   0,
			Ties:     0,
			Points:   0,
			Owners:   models.StringSlice(etlTeam.Owners),
		}
		err := database.DB.Model(&models.Team{}).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "league_id"}, {Name: "espn_id"}},
			DoNothing: true,
		}).Create(&dbTeam).Error
		if err != nil {
			logging.Errorf("Failed to ensure team exists: %v", err)
			return err
		}
	}

	// 3. Create all matchups
	// 4. Update existing boxscores
	// 5. Update expected wins counts
	// 6. Create new transactions
	// 7. Run simulations

	logging.Infof("Successfully unmarshaled league ID %d for year %d", league.ID, league.Year)

	return nil
}
