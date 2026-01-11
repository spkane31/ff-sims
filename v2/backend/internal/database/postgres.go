package database

import (
	"fmt"
	"os"

	"backend/internal/config"
	"backend/internal/logging"
	"backend/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB is the global database instance
var DB *gorm.DB

// Initialize sets up the database connection
func Initialize(cfg *config.Config) error {
	var err error
	logging.Infof("Initializing database connection", "connectionString", cfg.DB.ConnectionString)
	DB, err = gorm.Open(postgres.Open(cfg.DB.ConnectionString), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true, // Disable FK checks during migration
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	logging.Infof("Connected to database successfully")

	// Run migrations
	err = runMigrations()
	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// runMigrations automatically creates or updates database tables
func runMigrations() error {
	logging.Infof("Running database migrations...")

	// NOTE: this only works once because of issues with the unique constraints and GORM's
	// automigration logic. For now, when there's a change I have to manually migrate or
	// delete and recreate the database with the automigration logic.

	// Run the migrations
	if os.Getenv("DB_MIGRATE") != "true" {
		logging.Infof("Skipping migrations, DB_MIGRATE is not set to true")
		return nil
	}

	err := DB.AutoMigrate(
		&models.Team{},
		&models.TeamNameHistory{},
		&models.Player{},
		&models.League{},
		&models.Matchup{},
		&models.Simulation{},
		&models.SimResult{},
		&models.SimTeamResult{},
		&models.DraftSelection{},
		&models.Transaction{},
		&models.BoxScore{},
		&models.WeeklyExpectedWins{},
		&models.SeasonExpectedWins{},
		// Sleeper models
		&models.SleeperLeague{},
		&models.SleeperUser{},
		&models.SleeperTransaction{},
		&models.SleeperDraftPick{},
	)

	if err != nil {
		return err
	}

	logging.Infof("Migrations completed successfully")
	return nil
}
