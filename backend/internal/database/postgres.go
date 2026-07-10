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

// Archive is the global archive-database instance. Nil unless
// InitializeArchive has been called — only the worker does so, and only when
// cfg.ArchiveDB.Enabled() (i.e. ARCHIVE_DATABASE_URL is set).
var Archive *gorm.DB

// InitializeArchive sets up the archive database connection and configures
// its connection pool. Mirrors Initialize but targets cfg.ArchiveDB. Callers
// must check cfg.ArchiveDB.Enabled() first — this returns an error rather
// than silently no-op-ing when ConnectionString is empty, so a misconfigured
// call site fails loudly instead of leaving Archive nil.
func InitializeArchive(cfg *config.Config) error {
	if !cfg.ArchiveDB.Enabled() {
		return fmt.Errorf("archive database not configured (ARCHIVE_DATABASE_URL is empty)")
	}
	var err error
	slog.Debug("Initializing archive database connection", "connectionString", cfg.ArchiveDB.ConnectionString)
	Archive, err = gorm.Open(postgres.Open(cfg.ArchiveDB.ConnectionString), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to archive database: %w", err)
	}

	sqlDB, err := Archive.DB()
	if err != nil {
		return fmt.Errorf("get underlying archive sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.ArchiveDB.PoolMaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.ArchiveDB.PoolMaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ArchiveDB.PoolConnMaxLifetime) * time.Second)

	log.Printf("Connected to archive database (maxOpen=%d, maxIdle=%d, connLifetime=%ds)",
		cfg.ArchiveDB.PoolMaxOpenConns, cfg.ArchiveDB.PoolMaxIdleConns, cfg.ArchiveDB.PoolConnMaxLifetime)
	return nil
}
