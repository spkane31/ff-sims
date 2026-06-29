package database

import (
	"fmt"
	"log"
	"log/slog"
	"time"

	"backend/internal/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB is the global database instance
var DB *gorm.DB

// Initialize sets up the database connection and configures the connection pool.
// Pool limits are required to avoid exhausting connection slots on managed-DB
// instances (e.g. DigitalOcean PostgreSQL) when multiple Temporal workers run
// high-concurrency activities against the same database.
func Initialize(cfg *config.Config) error {
	var err error
	slog.Debug("Initializing database connection", "connectionString", cfg.DB.ConnectionString)
	DB, err = gorm.Open(postgres.Open(cfg.DB.ConnectionString), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("get underlying sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.DB.PoolMaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DB.PoolMaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.DB.PoolConnMaxLifetime) * time.Second)

	log.Printf("Connected to database (maxOpen=%d, maxIdle=%d, connLifetime=%ds)",
		cfg.DB.PoolMaxOpenConns, cfg.DB.PoolMaxIdleConns, cfg.DB.PoolConnMaxLifetime)
	return nil
}
