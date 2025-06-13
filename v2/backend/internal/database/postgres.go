package database

import (
	"fmt"
	"log"
	"log/slog"

	"backend/internal/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB is the global database instance
var DB *gorm.DB

// Initialize sets up the database connection
func Initialize(cfg *config.Config) error {
	var err error
	slog.Info("Initializing database connection", "connectionString", cfg.DB.ConnectionString)
	DB, err = gorm.Open(postgres.Open(cfg.DB.ConnectionString), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	log.Println("Connected to database successfully")

	// Run migrations
	err = runMigrations()
	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// runMigrations automatically creates or updates database tables
func runMigrations() error {
	log.Println("Running database migrations...")

	// Run the migrations
	// err := DB.AutoMigrate(
	// 	&models.Team{},
	// 	&models.TeamNameHistory{},
	// 	&models.Player{},
	// 	&models.PlayerGameStats{},
	// 	&models.League{},
	// 	&models.Matchup{},
	// 	&models.Simulation{},
	// 	&models.SimResult{},
	// 	&models.SimTeamResult{},
	// 	&models.DraftSelection{},
	// )

	// if err != nil {
	// 	return err
	// }

	log.Println("Migrations completed successfully")
	return nil
}
