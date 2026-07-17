package statscron_test

import (
	"context"
	"os"
	"testing"
	"time"

	"gorm.io/gorm"

	"backend/internal/dbmigrate"
	"backend/internal/models"
	"backend/internal/statscron"
	"backend/internal/testutil"
	archivemigrations "backend/migrations/archive"
)

// newStatscronTestDBs mirrors newScavengerTestDBs in
// internal/activities/scavenger_test.go: two throwaway schemas on
// TEST_DATABASE_URL, cloud AutoMigrated with the live sleeper models, archive
// migrated with the real goose migration files.
func newStatscronTestDBs(t *testing.T) (cloud, archive *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	cloudDSN := testutil.NewPGSchema(t, dsn, "statscron_cloud")
	cloud = testutil.OpenGORM(t, cloudDSN)
	if err := cloud.AutoMigrate(
		&models.SleeperUser{}, &models.SleeperLeague{}, &models.SleeperTransaction{},
		&models.SleeperDraft{}, &models.SleeperDraftPick{}, &models.SleeperLifetimeCount{},
	); err != nil {
		t.Fatalf("automigrate cloud: %v", err)
	}

	archiveDSN := testutil.NewPGSchema(t, dsn, "statscron_archive")
	if err := dbmigrate.Run(archiveDSN, archivemigrations.FS, "up", nil); err != nil {
		t.Fatalf("migrate archive: %v", err)
	}
	archive = testutil.OpenGORM(t, archiveDSN)

	return cloud, archive
}

func TestRunSnapshot_CountsDiscoveryStateFromCloud(t *testing.T) {
	cloud, archive := newStatscronTestDBs(t)
	now := time.Now().UTC()

	// users: 1 expanded, 1 pending, 1 skipped -> total 3
	cloud.Create(&models.SleeperUser{SleeperUserID: "u-expanded", LastFetchedAt: &now})
	cloud.Create(&models.SleeperUser{SleeperUserID: "u-pending"})
	cloud.Create(&models.SleeperUser{SleeperUserID: "u-skipped", SkippedAt: &now})

	// leagues: 2 expanded, 1 pending -> total 3
	cloud.Create(&models.SleeperLeague{SleeperLeagueID: "lg-a", Season: "2026", LastFetchedAt: &now})
	cloud.Create(&models.SleeperLeague{SleeperLeagueID: "lg-b", Season: "2026", LastFetchedAt: &now})
	cloud.Create(&models.SleeperLeague{SleeperLeagueID: "lg-c", Season: "2026"})

	report, err := statscron.RunSnapshot(context.Background(), cloud, archive)
	if err != nil {
		t.Fatalf("RunSnapshot: %v", err)
	}

	want := map[string]int64{
		models.LifetimeMetricUsersTotal:    3,
		models.LifetimeMetricUsersExpanded: 1,
		models.LifetimeMetricUsersPending:  1,
		models.LifetimeMetricUsersSkipped:  1,

		models.LifetimeMetricLeaguesTotal:    3,
		models.LifetimeMetricLeaguesExpanded: 2,
		models.LifetimeMetricLeaguesPending:  1,
		models.LifetimeMetricLeaguesSkipped:  0,
	}
	for metric, count := range want {
		if report.Counts[metric] != count {
			t.Errorf("metric %s: got %d, want %d", metric, report.Counts[metric], count)
		}
	}

	if !report.SnapshotAt.Equal(report.SnapshotAt.Truncate(time.Hour)) {
		t.Errorf("expected SnapshotAt to be truncated to the hour, got %v", report.SnapshotAt)
	}

	var rows []models.SleeperLifetimeCount
	if err := cloud.Find(&rows).Error; err != nil {
		t.Fatalf("fetch persisted rows: %v", err)
	}
	for _, r := range rows {
		if !r.SnapshotAt.Equal(report.SnapshotAt) {
			t.Errorf("row %s: SnapshotAt = %v, want %v", r.Metric, r.SnapshotAt, report.SnapshotAt)
		}
	}
}

// TestRunSnapshot_TransactionAndDraftMetricsReflectFullArchiveHistory seeds
// cloud with only a trimmed hot window (as if the scavenger's purge phase
// already ran) and archive with the same rows plus older, already-purged
// ones, then asserts the archive-sourced metrics reflect the full history,
// not just what's still in cloud.
func TestRunSnapshot_TransactionAndDraftMetricsReflectFullArchiveHistory(t *testing.T) {
	cloud, archive := newStatscronTestDBs(t)
	now := time.Now().UTC()

	// Cloud only has the hot-window row (purge already removed the rest).
	cloud.Create(&models.SleeperTransaction{
		SleeperTransactionID: "t-hot", Type: "trade", Status: "complete", CreatedAt: now,
	})
	cloud.Create(&models.SleeperDraft{SleeperDraftID: "d-hot", Status: "complete", Season: "2026", CreatedAt: now})

	// Archive holds the hot row plus history already purged from cloud.
	for _, id := range []string{"t-hot", "t-purged-1", "t-purged-2"} {
		archive.Create(&models.ArchiveSleeperTransaction{
			SleeperTransactionID: id, Type: "trade", Status: "complete", CreatedAt: now,
		})
	}
	archive.Create(&models.ArchiveSleeperTransaction{
		SleeperTransactionID: "t-waiver", Type: "waiver", Status: "complete", CreatedAt: now,
	})
	for _, id := range []string{"d-hot", "d-purged"} {
		archive.Create(&models.ArchiveSleeperDraft{
			SleeperDraftID: id, Status: "complete", Season: "2025", CreatedAt: now,
		})
	}
	archive.Create(&models.ArchiveSleeperDraft{
		SleeperDraftID: "d-pending", Status: "pre_draft", Season: "2026", CreatedAt: now,
	})

	report, err := statscron.RunSnapshot(context.Background(), cloud, archive)
	if err != nil {
		t.Fatalf("RunSnapshot: %v", err)
	}

	if report.Counts[models.LifetimeMetricTransactionsTotal] != 4 {
		t.Errorf("transactions_total = %d, want 4 (full archive history)", report.Counts[models.LifetimeMetricTransactionsTotal])
	}
	if report.Counts[models.LifetimeMetricTradesCompleted] != 3 {
		t.Errorf("trades_completed = %d, want 3, not just cloud's hot-window 1", report.Counts[models.LifetimeMetricTradesCompleted])
	}
	if report.Counts[models.LifetimeMetricDraftsCompleted] != 2 {
		t.Errorf("drafts_completed = %d, want 2, not just cloud's hot-window 1", report.Counts[models.LifetimeMetricDraftsCompleted])
	}
}

func TestRunSnapshot_SkipsArchiveMetricsWhenArchiveIsNil(t *testing.T) {
	cloud, _ := newStatscronTestDBs(t)

	report, err := statscron.RunSnapshot(context.Background(), cloud, nil)
	if err != nil {
		t.Fatalf("RunSnapshot: %v", err)
	}

	for _, metric := range []string{
		models.LifetimeMetricTransactionsTotal,
		models.LifetimeMetricTradesCompleted,
		models.LifetimeMetricDraftsCompleted,
	} {
		if _, ok := report.Counts[metric]; ok {
			t.Errorf("expected metric %s to be skipped (not written as a misleading zero) when archive is nil", metric)
		}
	}

	var rowCount int64
	cloud.Model(&models.SleeperLifetimeCount{}).Where("metric = ?", models.LifetimeMetricTransactionsTotal).Count(&rowCount)
	if rowCount != 0 {
		t.Errorf("expected no persisted row for an archive-only metric when archive is nil, got %d", rowCount)
	}
}

func TestRunSnapshot_SameHourUpsertsInsteadOfDuplicating(t *testing.T) {
	cloud, archive := newStatscronTestDBs(t)
	now := time.Now().UTC()
	cloud.Create(&models.SleeperUser{SleeperUserID: "u1", LastFetchedAt: &now})

	if _, err := statscron.RunSnapshot(context.Background(), cloud, archive); err != nil {
		t.Fatalf("first run: %v", err)
	}
	cloud.Create(&models.SleeperUser{SleeperUserID: "u2", LastFetchedAt: &now})
	report, err := statscron.RunSnapshot(context.Background(), cloud, archive)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if report.Counts[models.LifetimeMetricUsersExpanded] != 2 {
		t.Errorf("expected second run to report 2 expanded users, got %d", report.Counts[models.LifetimeMetricUsersExpanded])
	}

	var rowCount int64
	cloud.Model(&models.SleeperLifetimeCount{}).
		Where("metric = ? AND snapshot_at = ?", models.LifetimeMetricUsersExpanded, report.SnapshotAt).
		Count(&rowCount)
	if rowCount != 1 {
		t.Errorf("expected exactly one row for this hour+metric (upsert, not insert), got %d", rowCount)
	}
}
