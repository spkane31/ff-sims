package database

import (
	"fmt"
	"log"
	"log/slog"
	"os"

	"backend/internal/config"
	"backend/migrations"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
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
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	log.Println("Connected to database successfully")

	if err := runMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// runMigrations applies all pending goose migrations.
// Set DB_MIGRATE=true to run on server startup; prefer `make migrate` for explicit runs.
func runMigrations() error {
	if os.Getenv("DB_MIGRATE") != "true" {
		log.Println("Skipping migrations (DB_MIGRATE != true); run `make migrate` to apply")
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("get underlying sql.DB: %w", err)
	}

	goose.SetBaseFS(migrations.FS)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}

	log.Println("Running database migrations...")
	if err := goose.Up(sqlDB, "."); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}

	log.Println("Migrations completed successfully")
	return nil
}
