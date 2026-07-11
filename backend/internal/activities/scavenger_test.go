package activities_test

import (
	"context"
	"fmt"
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
	if cfg.RetentionDays != 30 {
		t.Errorf("RetentionDays = %d, want 30", cfg.RetentionDays)
	}
	if cfg.PurgeEnabled {
		t.Errorf("PurgeEnabled = true, want false (kill-switch defaults off)")
	}
}

func TestGetScavengerConfig_ReadsOverrides(t *testing.T) {
	t.Setenv("SCAVENGER_LEAGUE_BATCH_SIZE", "10")
	t.Setenv("SCAVENGER_TXN_BATCH_SIZE", "20")
	t.Setenv("SCAVENGER_DRAFT_BATCH_SIZE", "30")
	t.Setenv("SCAVENGER_MAX_BATCHES_PER_RUN", "5")
	t.Setenv("SCAVENGER_RETENTION_DAYS", "45")
	t.Setenv("SCAVENGER_PURGE_ENABLED", "true")

	a := &activities.ScavengerActivities{}
	cfg, err := a.GetScavengerConfig(context.Background())
	if err != nil {
		t.Fatalf("GetScavengerConfig: %v", err)
	}
	want := activities.ScavengerConfig{
		LeagueBatchSize: 10, TxnBatchSize: 20, DraftBatchSize: 30, MaxBatchesPerRun: 5,
		RetentionDays: 45, PurgeEnabled: true,
	}
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

func TestReplicateDraftHeadersBatch_CopiesRowsAndAdvancesCursor(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	for i, id := range []string{"d1", "d2"} {
		if err := cloud.Create(&models.SleeperDraft{
			SleeperDraftID: id, SleeperLeagueID: "lg1", Type: "snake", Status: "pre_draft",
			Season: "2026", CreatedAt: now.Add(time.Duration(i) * time.Second),
		}).Error; err != nil {
			t.Fatalf("seed draft %s: %v", id, err)
		}
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.ReplicateDraftHeadersBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("ReplicateDraftHeadersBatch: %v", err)
	}
	if res.Replicated != 2 || !res.Drained {
		t.Errorf("res = %+v, want {Replicated: 2, Drained: true}", res)
	}
	var got models.ArchiveSleeperDraft
	if err := archive.Where("sleeper_draft_id = ?", "d1").First(&got).Error; err != nil {
		t.Fatalf("fetch archived draft: %v", err)
	}
	if got.Type != "snake" || got.Status != "pre_draft" {
		t.Errorf("archived row mismatch: %+v", got)
	}
}

func TestReplicateDraftHeadersBatch_SecondRunIsNoOp(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	if err := cloud.Create(&models.SleeperDraft{SleeperDraftID: "d1", Season: "2026", CreatedAt: now}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	if _, err := a.ReplicateDraftHeadersBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	res, err := a.ReplicateDraftHeadersBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Replicated != 0 || !res.Drained {
		t.Errorf("second run = %+v, want {Replicated: 0, Drained: true}", res)
	}
}

func TestReplicateDraftPicksBatch_CopiesDraftAndPicksWhenLastFetchedAtSet(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	fetchedAt := now
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Type: "snake", Status: "complete",
		Season: "2026", LastFetchedAt: &fetchedAt, CreatedAt: now.Add(-time.Hour),
	}).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	for i := 1; i <= 2; i++ {
		if err := cloud.Create(&models.SleeperDraftPick{
			SleeperDraftID: "d1", Round: 1, PickNo: i, RosterID: i, SleeperPlayerID: fmt.Sprintf("p%d", i),
		}).Error; err != nil {
			t.Fatalf("seed pick %d: %v", i, err)
		}
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.ReplicateDraftPicksBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("ReplicateDraftPicksBatch: %v", err)
	}
	if res.Replicated != 1 || !res.Drained {
		t.Errorf("res = %+v, want {Replicated: 1, Drained: true} (1 draft)", res)
	}

	var draft models.ArchiveSleeperDraft
	if err := archive.Where("sleeper_draft_id = ?", "d1").First(&draft).Error; err != nil {
		t.Fatalf("fetch archived draft: %v", err)
	}
	if draft.Status != "complete" || draft.LastFetchedAt == nil {
		t.Errorf("archived draft mismatch: %+v", draft)
	}
	var pickCount int64
	archive.Model(&models.ArchiveSleeperDraftPick{}).Where("sleeper_draft_id = ?", "d1").Count(&pickCount)
	if pickCount != 2 {
		t.Errorf("expected 2 archived picks, got %d", pickCount)
	}
}

func TestReplicateDraftPicksBatch_SkipsDraftsWithoutPicksYet(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", Status: "pre_draft", Season: "2026", CreatedAt: now, LastFetchedAt: nil,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.ReplicateDraftPicksBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("ReplicateDraftPicksBatch: %v", err)
	}
	if res.Replicated != 0 || !res.Drained {
		t.Errorf("expected no drafts eligible (last_fetched_at NULL), got %+v", res)
	}
}

func TestReplicateDraftPicksBatch_SecondRunIsNoOp(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	now := time.Now().UTC().Add(-10 * time.Minute)
	fetchedAt := now
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", Status: "complete", Season: "2026", LastFetchedAt: &fetchedAt, CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	if _, err := a.ReplicateDraftPicksBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	res, err := a.ReplicateDraftPicksBatch(context.Background(), activities.ReplicateBatchParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Replicated != 0 || !res.Drained {
		t.Errorf("second run = %+v, want {Replicated: 0, Drained: true}", res)
	}
}

func TestPurgeTransactionsBatch_DeletesVerifiedOldRows(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	old := time.Now().UTC().AddDate(0, 0, -400) // event happened well over a year ago
	recentInsert := time.Now().UTC()            // but only just inserted — the exact scenario this fixes
	for i, id := range []string{"t1", "t2"} {
		if err := cloud.Create(&models.SleeperTransaction{
			SleeperTransactionID: id, SleeperLeagueID: "lg1",
			CreatedAtSleeper: old.Add(time.Duration(i) * time.Second).UnixMilli(),
			CreatedAt:        recentInsert,
		}).Error; err != nil {
			t.Fatalf("seed cloud txn %s: %v", id, err)
		}
		if err := archive.Create(&models.ArchiveSleeperTransaction{
			SleeperTransactionID: id, SleeperLeagueID: "lg1",
			CreatedAtSleeper: old.Add(time.Duration(i) * time.Second).UnixMilli(),
			CreatedAt:        recentInsert,
		}).Error; err != nil {
			t.Fatalf("seed archive txn %s: %v", id, err)
		}
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeTransactionsBatch: %v", err)
	}
	if res.Purged != 2 || res.Unverified != 0 || !res.Drained {
		t.Errorf("res = %+v, want {Purged: 2, Unverified: 0, Drained: true}", res)
	}
	var count int64
	cloud.Model(&models.SleeperTransaction{}).Count(&count)
	if count != 0 {
		t.Errorf("expected cloud transactions purged, got %d remaining", count)
	}
}

func TestPurgeTransactionsBatch_SkipsUnverifiedRows(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	old := time.Now().UTC().AddDate(0, 0, -40)
	if err := cloud.Create(&models.SleeperTransaction{
		SleeperTransactionID: "t1", CreatedAtSleeper: old.UnixMilli(), CreatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Not replicated to archive.

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeTransactionsBatch: %v", err)
	}
	if res.Purged != 0 || res.Unverified != 1 {
		t.Errorf("res = %+v, want {Purged: 0, Unverified: 1}", res)
	}
	var count int64
	cloud.Model(&models.SleeperTransaction{}).Count(&count)
	if count != 1 {
		t.Errorf("expected the unverified row to remain in cloud, got %d rows", count)
	}
}

func TestPurgeTransactionsBatch_IgnoresRowsWithinRetention(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	recent := time.Now().UTC().AddDate(0, 0, -5) // event happened within the 30-day retention window
	if err := cloud.Create(&models.SleeperTransaction{
		SleeperTransactionID: "t1", CreatedAtSleeper: recent.UnixMilli(), CreatedAt: recent,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeTransactionsBatch: %v", err)
	}
	if res.Purged != 0 || res.Unverified != 0 || !res.Drained {
		t.Errorf("res = %+v, want no candidates found (event is within retention)", res)
	}
}

func TestPurgeTransactionsBatch_ErrorsWhenOldestUnverifiedPastAlarmThreshold(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	waaayOld := time.Now().UTC().AddDate(0, 0, -46) // 30d retention + 15d alarm + 1, by INSERT time — the alarm clock
	if err := cloud.Create(&models.SleeperTransaction{
		SleeperTransactionID: "t1", CreatedAtSleeper: waaayOld.UnixMilli(), CreatedAt: waaayOld,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Not replicated to archive — stalled.

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	_, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err == nil {
		t.Fatal("expected an error once the oldest unverified row exceeds retention+15d (by insert time), got nil")
	}
}

func TestPurgeTransactionsBatch_DrainedWhenFewerThanBatchSize(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	old := time.Now().UTC().AddDate(0, 0, -400)
	for i, id := range []string{"t1", "t2", "t3"} {
		ts := old.Add(time.Duration(i) * time.Second)
		if err := cloud.Create(&models.SleeperTransaction{
			SleeperTransactionID: id, CreatedAtSleeper: ts.UnixMilli(), CreatedAt: time.Now().UTC(),
		}).Error; err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
		if err := archive.Create(&models.ArchiveSleeperTransaction{
			SleeperTransactionID: id, CreatedAtSleeper: ts.UnixMilli(), CreatedAt: time.Now().UTC(),
		}).Error; err != nil {
			t.Fatalf("seed archive %s: %v", id, err)
		}
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 2, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeTransactionsBatch: %v", err)
	}
	if res.Purged != 2 || res.Drained {
		t.Errorf("expected a full, non-drained batch of 2, got %+v", res)
	}
}

func TestPurgeTransactionsBatch_EligibleByEventTimeDespiteRecentInsertTime(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	eventTime := time.Now().UTC().AddDate(-1, 0, 0) // a year-old Sleeper transaction
	insertTime := time.Now().UTC()                  // freshly inserted today (e.g. a new league's backfill)
	if err := cloud.Create(&models.SleeperTransaction{
		SleeperTransactionID: "t1", CreatedAtSleeper: eventTime.UnixMilli(), CreatedAt: insertTime,
	}).Error; err != nil {
		t.Fatalf("seed cloud: %v", err)
	}
	if err := archive.Create(&models.ArchiveSleeperTransaction{
		SleeperTransactionID: "t1", CreatedAtSleeper: eventTime.UnixMilli(), CreatedAt: insertTime,
	}).Error; err != nil {
		t.Fatalf("seed archive: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeTransactionsBatch: %v", err)
	}
	if res.Purged != 1 {
		t.Errorf("expected the row to be purge-eligible despite being inserted today, because the underlying event is a year old; got %+v", res)
	}
}

func TestPurgeDraftsBatch_DeletesVerifiedDraftAndPicks(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	fetchedAt := time.Now().UTC()
	old := time.Now().UTC().AddDate(0, 0, -40)
	if err := cloud.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg1", Season: "2026", Status: "complete", LastDraftsFetchedAt: &fetchedAt,
	}).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: "2026",
		LastFetchedAt: &fetchedAt, CreatedAt: old,
	}).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	if err := cloud.Create(&models.SleeperDraftPick{SleeperDraftID: "d1", Round: 1, PickNo: 1, SleeperPlayerID: "p1"}).Error; err != nil {
		t.Fatalf("seed pick: %v", err)
	}
	if err := archive.Create(&models.ArchiveSleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: "2026",
		LastFetchedAt: &fetchedAt, CreatedAt: old,
	}).Error; err != nil {
		t.Fatalf("seed archive draft: %v", err)
	}
	if err := archive.Create(&models.ArchiveSleeperDraftPick{SleeperDraftID: "d1", Round: 1, PickNo: 1, SleeperPlayerID: "p1"}).Error; err != nil {
		t.Fatalf("seed archive pick: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeDraftsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeDraftsBatch: %v", err)
	}
	if res.Purged != 1 || res.Unverified != 0 || !res.Drained {
		t.Errorf("res = %+v, want {Purged: 1, Unverified: 0, Drained: true}", res)
	}
	var draftCount, pickCount int64
	cloud.Model(&models.SleeperDraft{}).Count(&draftCount)
	cloud.Model(&models.SleeperDraftPick{}).Count(&pickCount)
	if draftCount != 0 || pickCount != 0 {
		t.Errorf("expected draft and picks purged from cloud, got draftCount=%d pickCount=%d", draftCount, pickCount)
	}
}

func TestPurgeDraftsBatch_SkipsLeagueStillInSyncPool(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	old := time.Now().UTC().AddDate(0, 0, -40)
	if err := cloud.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg1", Season: "2026", Status: "pre_draft", // not yet excluded from the claim pool
	}).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
	fetchedAt := time.Now().UTC()
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: "2026",
		LastFetchedAt: &fetchedAt, CreatedAt: old,
	}).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	if err := archive.Create(&models.ArchiveSleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: "2026",
		LastFetchedAt: &fetchedAt, CreatedAt: old,
	}).Error; err != nil {
		t.Fatalf("seed archive draft: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeDraftsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeDraftsBatch: %v", err)
	}
	if res.Purged != 0 || !res.Drained {
		t.Errorf("expected the draft to be excluded from purge candidates entirely (league still claimable), got %+v", res)
	}
	var draftCount int64
	cloud.Model(&models.SleeperDraft{}).Count(&draftCount)
	if draftCount != 1 {
		t.Errorf("expected the draft to remain in cloud, got %d", draftCount)
	}
}

func TestPurgeDraftsBatch_SkipsPickCountMismatch(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	fetchedAt := time.Now().UTC()
	old := time.Now().UTC().AddDate(0, 0, -40)
	if err := cloud.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg1", Season: "2026", Status: "complete", LastDraftsFetchedAt: &fetchedAt,
	}).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: "2026",
		LastFetchedAt: &fetchedAt, CreatedAt: old,
	}).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	for _, pickNo := range []int{1, 2} {
		if err := cloud.Create(&models.SleeperDraftPick{SleeperDraftID: "d1", Round: 1, PickNo: pickNo}).Error; err != nil {
			t.Fatalf("seed pick %d: %v", pickNo, err)
		}
	}
	if err := archive.Create(&models.ArchiveSleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: "2026",
		LastFetchedAt: &fetchedAt, CreatedAt: old,
	}).Error; err != nil {
		t.Fatalf("seed archive draft: %v", err)
	}
	// Only 1 of 2 picks made it to archive — parity mismatch.
	if err := archive.Create(&models.ArchiveSleeperDraftPick{SleeperDraftID: "d1", Round: 1, PickNo: 1}).Error; err != nil {
		t.Fatalf("seed archive pick: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeDraftsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeDraftsBatch: %v", err)
	}
	if res.Purged != 0 || res.Unverified != 1 {
		t.Errorf("res = %+v, want {Purged: 0, Unverified: 1} (pick count mismatch)", res)
	}
	var draftCount int64
	cloud.Model(&models.SleeperDraft{}).Count(&draftCount)
	if draftCount != 1 {
		t.Errorf("expected the draft to remain in cloud, got %d", draftCount)
	}
}

func TestPurgeDraftsBatch_IgnoresRecentDrafts(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	fetchedAt := time.Now().UTC()
	recent := time.Now().UTC().AddDate(0, 0, -5)
	if err := cloud.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg1", Season: "2026", Status: "complete", LastDraftsFetchedAt: &fetchedAt,
	}).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: "2026",
		LastFetchedAt: &fetchedAt, CreatedAt: recent,
	}).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeDraftsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeDraftsBatch: %v", err)
	}
	if res.Purged != 0 || !res.Drained {
		t.Errorf("expected no candidates (draft is within retention), got %+v", res)
	}
}
