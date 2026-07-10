package activities_test

import (
	"context"
	"os"
	"testing"
	"time"

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

func TestReplicateLeaguesBatch_CopiesRowsAndAdvancesCursor(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute) // outside the 5-min safety lag
	ppr := 1.0
	for i, id := range []string{"lg1", "lg2"} {
		if err := cloud.Create(&models.SleeperLeague{
			SleeperLeagueID: id, Name: "League " + id, Season: "2026", LeagueType: "redraft",
			PPR: &ppr, UpdatedAt: now.Add(time.Duration(i) * time.Second),
		}).Error; err != nil {
			t.Fatalf("seed league %s: %v", id, err)
		}
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.ReplicateLeaguesBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("ReplicateLeaguesBatch: %v", err)
	}
	if res.Replicated != 2 || !res.Drained {
		t.Errorf("res = %+v, want {Replicated: 2, Drained: true}", res)
	}

	var count int64
	archive.Table("sleeper_leagues").Count(&count)
	if count != 2 {
		t.Errorf("expected 2 archived leagues, got %d", count)
	}
	var got models.ArchiveSleeperLeague
	if err := archive.Where("sleeper_league_id = ?", "lg1").First(&got).Error; err != nil {
		t.Fatalf("fetch archived league: %v", err)
	}
	if got.Name != "League lg1" || got.LeagueType != "redraft" || got.PPR == nil || *got.PPR != 1.0 {
		t.Errorf("archived row mismatch: %+v", got)
	}
}

func TestReplicateLeaguesBatch_SecondRunIsNoOp(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	if err := cloud.Create(&models.SleeperLeague{SleeperLeagueID: "lg1", Season: "2026", UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	if _, err := a.ReplicateLeaguesBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	res, err := a.ReplicateLeaguesBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Replicated != 0 || !res.Drained {
		t.Errorf("second run = %+v, want {Replicated: 0, Drained: true}", res)
	}
}

func TestReplicateLeaguesBatch_RespectsSafetyLag(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	tooRecent := time.Now().UTC().Add(-1 * time.Minute) // inside the 5-min lag
	if err := cloud.Create(&models.SleeperLeague{SleeperLeagueID: "lg1", Season: "2026", UpdatedAt: tooRecent}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.ReplicateLeaguesBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("ReplicateLeaguesBatch: %v", err)
	}
	if res.Replicated != 0 {
		t.Errorf("expected the too-recent league to be excluded by the safety lag, got %+v", res)
	}
}

func TestReplicateLeaguesBatch_DrainedWhenFewerThanBatchSize(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	for i, id := range []string{"lg1", "lg2", "lg3"} {
		if err := cloud.Create(&models.SleeperLeague{SleeperLeagueID: id, Season: "2026", UpdatedAt: now.Add(time.Duration(i) * time.Second)}).Error; err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.ReplicateLeaguesBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 2})
	if err != nil {
		t.Fatalf("ReplicateLeaguesBatch: %v", err)
	}
	if res.Replicated != 2 || res.Drained {
		t.Errorf("expected a full, non-drained batch of 2, got %+v", res)
	}
}

func TestReplicateTransactionsBatch_CopiesRowsAndAdvancesCursor(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	for i, id := range []string{"t1", "t2"} {
		if err := cloud.Create(&models.SleeperTransaction{
			SleeperTransactionID: id, SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
			CreatedAtSleeper: 1000, CreatedAt: now.Add(time.Duration(i) * time.Second),
		}).Error; err != nil {
			t.Fatalf("seed txn %s: %v", id, err)
		}
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.ReplicateTransactionsBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("ReplicateTransactionsBatch: %v", err)
	}
	if res.Replicated != 2 || !res.Drained {
		t.Errorf("res = %+v, want {Replicated: 2, Drained: true}", res)
	}
	var got models.ArchiveSleeperTransaction
	if err := archive.Where("sleeper_transaction_id = ?", "t1").First(&got).Error; err != nil {
		t.Fatalf("fetch archived txn: %v", err)
	}
	if got.Type != "trade" || got.SleeperLeagueID != "lg1" {
		t.Errorf("archived row mismatch: %+v", got)
	}
}

func TestReplicateTransactionsBatch_SecondRunIsNoOp(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	if err := cloud.Create(&models.SleeperTransaction{SleeperTransactionID: "t1", CreatedAt: now}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	if _, err := a.ReplicateTransactionsBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	res, err := a.ReplicateTransactionsBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Replicated != 0 || !res.Drained {
		t.Errorf("second run = %+v, want {Replicated: 0, Drained: true}", res)
	}
}

func TestReplicateTransactionsBatch_RespectsSafetyLag(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	tooRecent := time.Now().UTC().Add(-1 * time.Minute)
	if err := cloud.Create(&models.SleeperTransaction{SleeperTransactionID: "t1", CreatedAt: tooRecent}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.ReplicateTransactionsBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("ReplicateTransactionsBatch: %v", err)
	}
	if res.Replicated != 0 {
		t.Errorf("expected the too-recent txn to be excluded by the safety lag, got %+v", res)
	}
}
