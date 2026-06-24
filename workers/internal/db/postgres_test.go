package db_test

import (
	"os"
	"testing"

	"workers/internal/db"
)

func TestConnect_SkipsWithoutURL(t *testing.T) {
	orig := os.Getenv("DATABASE_URL")
	os.Unsetenv("DATABASE_URL")
	defer os.Setenv("DATABASE_URL", orig)

	_, err := db.Connect()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is empty")
	}
}

func TestConnect_Integration(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set")
	}
	conn, err := db.Connect()
	if err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	sqlDB, _ := conn.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Ping() error: %v", err)
	}
}
