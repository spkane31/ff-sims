package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

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
	Host     string
	Port     int
	Name     string
	User     string
	Password string
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
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvAsInt("DB_PORT", 5432),
			Name:     getEnv("DB_NAME", "ffsims"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
		},
	}

	return cfg, nil
}

// Helper functions for reading environment variables
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}
