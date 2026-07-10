package config

import (
	"log/slog"
	"os"
	"strconv"

	_ "github.com/joho/godotenv/autoload"
)

// Config contains all configuration for the application
type Config struct {
	Server    ServerConfig
	DB        DBConfig
	ArchiveDB ArchiveDBConfig
}

// ServerConfig contains server-specific configuration
type ServerConfig struct {
	Port int
	Env  string
}

// DBConfig contains database-specific configuration
type DBConfig struct {
	ConnectionString string
	// Pool limits prevent exhausting connection slots on managed-DB instances.
	PoolMaxOpenConns    int
	PoolMaxIdleConns    int
	PoolConnMaxLifetime int // seconds
}

// ArchiveDBConfig contains archive-database-specific configuration. An empty
// ConnectionString means the archive DB is disabled — local dev and any
// fleet without ARCHIVE_DATABASE_URL set keep working unchanged.
type ArchiveDBConfig struct {
	ConnectionString string
	// Pool limits prevent exhausting connection slots on managed-DB instances.
	PoolMaxOpenConns    int
	PoolMaxIdleConns    int
	PoolConnMaxLifetime int // seconds
}

// Enabled reports whether an archive database is configured.
func (c ArchiveDBConfig) Enabled() bool {
	return c.ConnectionString != ""
}

// Load reads the configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: getEnvAsInt("SERVER_PORT", 8080),
			Env:  getEnv("ENV", "development"),
		},
		DB: DBConfig{
			ConnectionString:    getEnv("DATABASE_URL", "postgresql://postgres@localhost:5432/ffsims"),
			PoolMaxOpenConns:    getEnvAsInt("DB_MAX_OPEN_CONNS", 10),
			PoolMaxIdleConns:    getEnvAsInt("DB_MAX_IDLE_CONNS", 5),
			PoolConnMaxLifetime: getEnvAsInt("DB_CONN_MAX_LIFETIME_SECS", 300),
		},
		ArchiveDB: ArchiveDBConfig{
			ConnectionString:    getEnv("ARCHIVE_DATABASE_URL", ""),
			PoolMaxOpenConns:    getEnvAsInt("ARCHIVE_DB_MAX_OPEN_CONNS", 10),
			PoolMaxIdleConns:    getEnvAsInt("ARCHIVE_DB_MAX_IDLE_CONNS", 5),
			PoolConnMaxLifetime: getEnvAsInt("ARCHIVE_DB_CONN_MAX_LIFETIME_SECS", 300),
		},
	}

	return cfg, nil
}

// Helper functions for reading environment variables
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		slog.Debug("Using environment variable", "key", key, "value", value)
		return value
	}
	slog.Info("Using default value for environment variable", "key", key, "defaultValue", defaultValue)
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}
