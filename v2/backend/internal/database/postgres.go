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
	slog.Debug("Initializing database connection", "connectionString", cfg.DB.ConnectionString)
	DB, err = gorm.Open(postgres.Open(cfg.DB.ConnectionString), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	log.Println("Connected to database successfully")
	return nil
}
