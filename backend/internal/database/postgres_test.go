package database_test

import (
	"os"
	"testing"

	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/testutil"
)

func TestInitializeArchive_ErrorsWhenDisabled(t *testing.T) {
	cfg := &config.Config{} // ArchiveDB.ConnectionString is empty
	if err := database.InitializeArchive(cfg); err == nil {
		t.Fatal("expected error when ARCHIVE_DATABASE_URL is not configured")
	}
}

func TestInitializeArchive_ConnectsWhenConfigured(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	scopedDSN := testutil.NewPGSchema(t, dsn, "archive_handle_test")

	cfg := &config.Config{ArchiveDB: config.ArchiveDBConfig{
		ConnectionString:    scopedDSN,
		PoolMaxOpenConns:    5,
		PoolMaxIdleConns:    2,
		PoolConnMaxLifetime: 60,
	}}

	if err := database.InitializeArchive(cfg); err != nil {
		t.Fatalf("InitializeArchive: %v", err)
	}
	if database.Archive == nil {
		t.Fatal("expected database.Archive to be set")
	}
	sqlDB, err := database.Archive.DB()
	if err != nil {
		t.Fatalf("get underlying sql.DB: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping archive db: %v", err)
	}
}
