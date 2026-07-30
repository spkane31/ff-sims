package statscron_test

import (
	"context"
	"os"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"backend/internal/dbmigrate"
	"backend/internal/models"
	"backend/internal/statscron"
	"backend/internal/testutil"
	"backend/internal/to"
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
		&models.SleeperTransactionFetchAgeSnapshot{},
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

func newStatscronSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:statscron?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.SleeperUser{}, &models.SleeperLeague{}, &models.SleeperLifetimeCount{},
		&models.SleeperTransactionFetchAgeSnapshot{},
	); err != nil {
		t.Fatalf("automigrate sqlite: %v", err)
	}
	return db
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

	row, err := statscron.RunSnapshot(context.Background(), cloud, archive)
	if err != nil {
		t.Fatalf("RunSnapshot: %v", err)
	}

	if row.UsersTotal != 3 || row.UsersExpanded != 1 || row.UsersPending != 1 || row.UsersSkipped != 1 {
		t.Errorf("users counts = %+v, want total=3 expanded=1 pending=1 skipped=1", row)
	}
	if row.LeaguesTotal != 3 || row.LeaguesExpanded != 2 || row.LeaguesPending != 1 || row.LeaguesSkipped != 0 {
		t.Errorf("leagues counts = %+v, want total=3 expanded=2 pending=1 skipped=0", row)
	}
	if !row.SnapshotAt.Equal(row.SnapshotAt.Truncate(time.Hour)) {
		t.Errorf("expected SnapshotAt to be truncated to the hour, got %v", row.SnapshotAt)
	}

	var persisted models.SleeperLifetimeCount
	if err := cloud.Where("snapshot_at = ?", row.SnapshotAt).First(&persisted).Error; err != nil {
		t.Fatalf("fetch persisted row: %v", err)
	}
	if persisted.UsersTotal != 3 || persisted.LeaguesExpanded != 2 {
		t.Errorf("persisted row = %+v, want it to match the returned row", persisted)
	}
}

func TestRunSnapshot_WritesCurrentSeasonFetchAgeBuckets(t *testing.T) {
	cloud := newStatscronSQLiteDB(t)
	var archive *gorm.DB
	now := time.Now().UTC()

	ages := []struct {
		id        string
		fetchedAt *time.Time
	}{
		{id: "never"},
		{id: "fresh", fetchedAt: to.Ptr(now.Add(-2 * time.Hour))},
		{id: "four-hours", fetchedAt: to.Ptr(now.Add(-6 * time.Hour))},
		{id: "eight-hours", fetchedAt: to.Ptr(now.Add(-10 * time.Hour))},
		{id: "twelve-hours", fetchedAt: to.Ptr(now.Add(-14 * time.Hour))},
		{id: "sixteen-hours", fetchedAt: to.Ptr(now.Add(-18 * time.Hour))},
		{id: "twenty-hours", fetchedAt: to.Ptr(now.Add(-22 * time.Hour))},
		{id: "stale", fetchedAt: to.Ptr(now.Add(-26 * time.Hour))},
	}
	for _, tc := range ages {
		if err := cloud.Create(&models.SleeperLeague{
			SleeperLeagueID: tc.id, Season: "2026", LastTransactionsFetchedAt: tc.fetchedAt,
		}).Error; err != nil {
			t.Fatalf("create current-season league %q: %v", tc.id, err)
		}
	}
	if err := cloud.Create(&models.SleeperLeague{
		SleeperLeagueID: "skipped", Season: "2026", LastTransactionsFetchedAt: to.Ptr(now.Add(-26 * time.Hour)), SkippedAt: &now,
	}).Error; err != nil {
		t.Fatalf("create skipped league: %v", err)
	}
	if err := cloud.Create(&models.SleeperLeague{
		SleeperLeagueID: "previous-season", Season: "2025", LastTransactionsFetchedAt: to.Ptr(now.Add(-26 * time.Hour)),
	}).Error; err != nil {
		t.Fatalf("create previous-season league: %v", err)
	}

	first, err := statscron.RunSnapshot(context.Background(), cloud, archive)
	if err != nil {
		t.Fatalf("RunSnapshot: %v", err)
	}

	type snapshotRow struct {
		SnapshotAt                     time.Time
		Season                         string
		NeverFetched                   int64
		FetchedWithinFourHours         int64
		FetchedFourToEightHours        int64
		FetchedEightToTwelveHours      int64
		FetchedTwelveToSixteenHours    int64
		FetchedSixteenToTwentyHours    int64
		FetchedTwentyToTwentyFourHours int64
		FetchedTwentyFourOrMoreHours   int64
	}
	var snapshot snapshotRow
	if err := cloud.Table("sleeper_transaction_fetch_age_snapshots").
		Where("snapshot_at = ? AND season = ?", first.SnapshotAt, "2026").
		Take(&snapshot).Error; err != nil {
		t.Fatalf("read fetch-age snapshots: %v", err)
	}
	if snapshot.NeverFetched != 1 || snapshot.FetchedWithinFourHours != 1 ||
		snapshot.FetchedFourToEightHours != 1 || snapshot.FetchedEightToTwelveHours != 1 ||
		snapshot.FetchedTwelveToSixteenHours != 1 || snapshot.FetchedSixteenToTwentyHours != 1 ||
		snapshot.FetchedTwentyToTwentyFourHours != 1 || snapshot.FetchedTwentyFourOrMoreHours != 1 {
		t.Errorf("fetch-age snapshot = %+v, want each bucket to equal 1", snapshot)
	}

	if err := cloud.Model(&models.SleeperLeague{}).
		Where("sleeper_league_id = ?", "never").
		Update("last_transactions_fetched_at", now).Error; err != nil {
		t.Fatalf("mark never-fetched league fresh: %v", err)
	}
	second, err := statscron.RunSnapshot(context.Background(), cloud, archive)
	if err != nil {
		t.Fatalf("retry RunSnapshot: %v", err)
	}
	if !second.SnapshotAt.Equal(first.SnapshotAt) {
		t.Fatalf("retry snapshot hour = %s, want %s", second.SnapshotAt, first.SnapshotAt)
	}

	snapshot = snapshotRow{}
	if err := cloud.Table("sleeper_transaction_fetch_age_snapshots").
		Where("snapshot_at = ? AND season = ?", first.SnapshotAt, "2026").
		Take(&snapshot).Error; err != nil {
		t.Fatalf("read retried fetch-age snapshots: %v", err)
	}
	if snapshot.NeverFetched != 0 || snapshot.FetchedWithinFourHours != 2 {
		t.Errorf("retried snapshot = %+v, want never=0 fresh=2", snapshot)
	}
}

// TestRunSnapshot_TransactionAndDraftColumnsReflectFullArchiveHistory seeds
// cloud with only a trimmed hot window (as if the scavenger's purge phase
// already ran) and archive with the same rows plus older, already-purged
// ones, then asserts the archive-sourced columns reflect the full history,
// not just what's still in cloud.
func TestRunSnapshot_TransactionAndDraftColumnsReflectFullArchiveHistory(t *testing.T) {
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

	row, err := statscron.RunSnapshot(context.Background(), cloud, archive)
	if err != nil {
		t.Fatalf("RunSnapshot: %v", err)
	}

	if row.TransactionsTotal == nil || *row.TransactionsTotal != 4 {
		t.Errorf("transactions_total = %v, want 4 (full archive history)", row.TransactionsTotal)
	}
	if row.TradesCompleted == nil || *row.TradesCompleted != 3 {
		t.Errorf("trades_completed = %v, want 3, not just cloud's hot-window 1", row.TradesCompleted)
	}
	if row.DraftsCompleted == nil || *row.DraftsCompleted != 2 {
		t.Errorf("drafts_completed = %v, want 2, not just cloud's hot-window 1", row.DraftsCompleted)
	}
}

func TestRunSnapshot_LeavesArchiveColumnsNilWhenArchiveIsNil(t *testing.T) {
	cloud, _ := newStatscronTestDBs(t)

	row, err := statscron.RunSnapshot(context.Background(), cloud, nil)
	if err != nil {
		t.Fatalf("RunSnapshot: %v", err)
	}

	if row.TransactionsTotal != nil || row.TradesCompleted != nil || row.DraftsCompleted != nil {
		t.Errorf("expected archive-sourced columns to stay nil (not a misleading zero) when archive is nil, got %+v", row)
	}

	var persisted models.SleeperLifetimeCount
	if err := cloud.Where("snapshot_at = ?", row.SnapshotAt).First(&persisted).Error; err != nil {
		t.Fatalf("fetch persisted row: %v", err)
	}
	if persisted.TransactionsTotal != nil {
		t.Errorf("expected persisted transactions_total to be NULL, got %v", *persisted.TransactionsTotal)
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
	row, err := statscron.RunSnapshot(context.Background(), cloud, archive)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if row.UsersExpanded != 2 {
		t.Errorf("expected second run to report 2 expanded users, got %d", row.UsersExpanded)
	}

	var rowCount int64
	cloud.Model(&models.SleeperLifetimeCount{}).Where("snapshot_at = ?", row.SnapshotAt).Count(&rowCount)
	if rowCount != 1 {
		t.Errorf("expected exactly one row for this hour (upsert, not insert), got %d", rowCount)
	}
}
