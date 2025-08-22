package config

import (
	"log/slog"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

func init() {
	godotenv.Load()
}

// Config contains all configuration for the application
type Config struct {
	Server ServerConfig
	DB     DBConfig
}

// ServerConfig contains server-specific configuration
type ServerConfig struct {
	Port int
	Env  string
}

// DBConfig contains database-specific configuration
type DBConfig struct {
	ConnectionString string
}

// Load reads the configuration from environment variables
func Load() (*Config, error) {
	// Load .env file if it exists
	godotenv.Load()

	cfg := &Config{
		Server: ServerConfig{
			Port: getEnvAsInt("SERVER_PORT", 8080),
			Env:  getEnv("ENV", "development"),
		},
		DB: DBConfig{
			ConnectionString: getEnv("DATABASE_URL", getEnv("COCKROACHDB_URL", "postgresql://postgres@localhost:5432/ffsims")),
		},
	}

	return cfg, nil
}

// Helper functions for reading environment variables
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		slog.Info("Using environment variable", "key", key, "value", value)
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
