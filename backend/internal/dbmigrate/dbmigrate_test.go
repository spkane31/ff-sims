package dbmigrate_test

import (
	"os"
	"testing"

	"backend/internal/dbmigrate"
	"backend/internal/testutil"
	"backend/migrations"
	archivemigrations "backend/migrations/archive"
)

func tableExists(t *testing.T, scopedDSN, table string) bool {
	t.Helper()
	db := testutil.OpenGORM(t, scopedDSN)
	var exists bool
	if err := db.Raw(
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = ?)", table,
	).Scan(&exists).Error; err != nil {
		t.Fatalf("check table %s exists: %v", table, err)
	}
	return exists
}

func indexExists(t *testing.T, scopedDSN, index string) bool {
	t.Helper()
	db := testutil.OpenGORM(t, scopedDSN)
	var exists bool
	if err := db.Raw(
		"SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = ?)", index,
	).Scan(&exists).Error; err != nil {
		t.Fatalf("check index %s exists: %v", index, err)
	}
	return exists
}

func TestRun_ArchiveMigrations_CreatesSyncStateTable(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	scopedDSN := testutil.NewPGSchema(t, dsn, "archive_migrate_test")

	if err := dbmigrate.Run(scopedDSN, archivemigrations.FS, "up", nil); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if !tableExists(t, scopedDSN, "archive_sync_state") {
		t.Error("expected archive_sync_state table to exist after migrate up")
	}

	// Idempotent: re-running up against an up-to-date schema is a no-op, not
	// an error — this is what makes it safe for cmd/worker to call on every
	// startup.
	if err := dbmigrate.Run(scopedDSN, archivemigrations.FS, "up", nil); err != nil {
		t.Fatalf("migrate up (second run): %v", err)
	}
}

func TestRun_CloudMigrations_ApplyCleanlyAndCreateScavengerIndexes(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	scopedDSN := testutil.NewPGSchema(t, dsn, "cloud_migrate_test")

	if err := dbmigrate.Run(scopedDSN, migrations.FS, "up", nil); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	for _, idx := range []string{"idx_sleeper_transactions_created_at", "idx_sleeper_drafts_last_fetched_at"} {
		if !indexExists(t, scopedDSN, idx) {
			t.Errorf("expected index %s to exist after migrate up", idx)
		}
	}
}
