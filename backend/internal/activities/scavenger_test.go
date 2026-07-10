package activities_test

import (
	"context"
	"os"
	"testing"

	"gorm.io/gorm"

	"backend/internal/activities"
	"backend/internal/dbmigrate"
	"backend/internal/models"
	"backend/internal/testutil"
	archivemigrations "backend/migrations/archive"
)

func TestGetScavengerConfig_ReadsEnvWithDefaults(t *testing.T) {
	a := &activities.ScavengerActivities{}
	cfg, err := a.GetScavengerConfig(context.Background())
	if err != nil {
		t.Fatalf("GetScavengerConfig: %v", err)
	}
	if cfg.LeagueBatchSize != 500 || cfg.TxnBatchSize != 5000 || cfg.DraftBatchSize != 200 || cfg.MaxBatchesPerRun != 50 {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestGetScavengerConfig_ReadsOverrides(t *testing.T) {
	t.Setenv("SCAVENGER_LEAGUE_BATCH_SIZE", "10")
	t.Setenv("SCAVENGER_TXN_BATCH_SIZE", "20")
	t.Setenv("SCAVENGER_DRAFT_BATCH_SIZE", "30")
	t.Setenv("SCAVENGER_MAX_BATCHES_PER_RUN", "5")

	a := &activities.ScavengerActivities{}
	cfg, err := a.GetScavengerConfig(context.Background())
	if err != nil {
		t.Fatalf("GetScavengerConfig: %v", err)
	}
	want := activities.ScavengerConfig{LeagueBatchSize: 10, TxnBatchSize: 20, DraftBatchSize: 30, MaxBatchesPerRun: 5}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

// newScavengerTestDBs opens two throwaway schemas on TEST_DATABASE_URL — one
// migrated with the cloud sleeper models (AutoMigrate, matching the existing
// claim_pg_test.go convention), one migrated with the real archive goose
// migrations (dbmigrate.Run against archivemigrations.FS, so these tests
// also exercise the actual migration files, not just the Go structs).
func newScavengerTestDBs(t *testing.T) (cloud, archive *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	cloudDSN := testutil.NewPGSchema(t, dsn, "scavenger_cloud")
	cloud = testutil.OpenGORM(t, cloudDSN)
	if err := cloud.AutoMigrate(&models.SleeperLeague{}, &models.SleeperTransaction{}, &models.SleeperDraft{}, &models.SleeperDraftPick{}); err != nil {
		t.Fatalf("automigrate cloud: %v", err)
	}

	archiveDSN := testutil.NewPGSchema(t, dsn, "scavenger_archive")
	if err := dbmigrate.Run(archiveDSN, archivemigrations.FS, "up", nil); err != nil {
		t.Fatalf("migrate archive: %v", err)
	}
	archive = testutil.OpenGORM(t, archiveDSN)

	return cloud, archive
}
