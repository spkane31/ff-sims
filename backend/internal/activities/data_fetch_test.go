package activities_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/sleeper"
)

// newArchiveTestDB opens an in-memory SQLite DB migrated with the archive
// models — a lightweight stand-in for the archive DB in tests that need to
// prove routing actually lands rows in a *different* database than cloud.
// No PG-specific SQL is involved in this routing logic, so SQLite suffices
// here (unlike the scavenger's keyset-cursor queries).
func newArchiveTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("unwrap sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&models.ArchiveSleeperLeague{}, &models.ArchiveSleeperTransaction{},
		&models.ArchiveSleeperDraft{}, &models.ArchiveSleeperDraftPick{},
	); err != nil {
		t.Fatalf("automigrate archive: %v", err)
	}
	return db
}

// draftsTestServer fakes /v1/league/{id}/drafts and /v1/draft/{id}/picks.
// drafts maps leagueID -> drafts; picks maps draftID -> picks. Missing league
// keys 404; missing pick keys return an empty list.
func draftsTestServer(t *testing.T, drafts map[string][]sleeper.Draft, picks map[string][]sleeper.DraftPick, calls *atomic.Int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			calls.Add(1)
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case strings.HasSuffix(r.URL.Path, "/drafts"):
			ds, ok := drafts[parts[2]]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(ds)
		case strings.HasSuffix(r.URL.Path, "/picks"):
			json.NewEncoder(w).Encode(picks[parts[2]])
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func runDraftsBatch(t *testing.T, dfa *activities.DataFetchActivities, params activities.SyncLeagueDraftsBatchParams) activities.SyncBatchResult {
	t.Helper()
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestActivityEnvironment()
	env.RegisterActivity(dfa.SyncLeagueDraftsBatch)
	val, err := env.ExecuteActivity(dfa.SyncLeagueDraftsBatch, params)
	if err != nil {
		t.Fatalf("drafts batch activity: %v", err)
	}
	var res activities.SyncBatchResult
	if err := val.Get(&res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return res
}

func draftClaimedLeague(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	now := time.Now().UTC()
	l := models.SleeperLeague{SleeperLeagueID: id, Season: "2026", LastFetchedAt: &now, DraftsClaimedAt: &now}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestSyncDraftsBatch_FetchesPicksAndStamps(t *testing.T) {
	db := newTestDB(t)
	draftClaimedLeague(t, db, "lg1")

	srv := draftsTestServer(t,
		map[string][]sleeper.Draft{
			"lg1": {
				{DraftID: "d1", Status: "complete", Type: "snake", Season: "2026"},
				{DraftID: "d2", Status: "in_progress", Type: "snake", Season: "2026"},
			},
		},
		map[string][]sleeper.DraftPick{
			"d1": {{Round: 1, PickNo: 1, RosterID: 1, PlayerID: "p1"}, {Round: 1, PickNo: 2, RosterID: 2, PlayerID: "p2"}},
		}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runDraftsBatch(t, dfa, activities.SyncLeagueDraftsBatchParams{LeagueIDs: []string{"lg1"}, Concurrency: 2})
	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("expected 1 processed / 0 failed, got %+v", res)
	}

	var draftCount, pickCount int64
	db.Model(&models.SleeperDraft{}).Count(&draftCount)
	db.Model(&models.SleeperDraftPick{}).Count(&pickCount)
	if draftCount != 2 || pickCount != 2 {
		t.Errorf("expected 2 drafts / 2 picks, got %d / %d", draftCount, pickCount)
	}

	var d1 models.SleeperDraft
	db.First(&d1, "sleeper_draft_id = ?", "d1")
	if d1.LastFetchedAt == nil {
		t.Error("completed draft d1 should be stamped last_fetched_at")
	}
	var lg models.SleeperLeague
	db.First(&lg, "sleeper_league_id = ?", "lg1")
	if lg.LastDraftsFetchedAt == nil || lg.DraftsClaimedAt != nil {
		t.Errorf("league not stamped/unclaimed: %+v", lg)
	}
}

func TestSyncDraftsBatch_PicksAreFetchOnce(t *testing.T) {
	db := newTestDB(t)
	draftClaimedLeague(t, db, "lg1")
	// Draft already fetched by an earlier sweep.
	fetched := time.Now().UTC()
	db.Create(&models.SleeperDraft{SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", LastFetchedAt: &fetched})

	var calls atomic.Int64
	srv := draftsTestServer(t,
		map[string][]sleeper.Draft{
			"lg1": {{DraftID: "d1", Status: "complete", Type: "snake", Season: "2026"}},
		},
		map[string][]sleeper.DraftPick{
			"d1": {{Round: 1, PickNo: 1, RosterID: 1, PlayerID: "p1"}},
		}, &calls)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runDraftsBatch(t, dfa, activities.SyncLeagueDraftsBatchParams{LeagueIDs: []string{"lg1"}, Concurrency: 1})
	if res.Processed != 1 {
		t.Fatalf("expected 1 processed, got %+v", res)
	}
	// Only the /drafts call — no /picks call for the already-fetched draft.
	if got := calls.Load(); got != 1 {
		t.Errorf("expected 1 HTTP call (drafts only), got %d", got)
	}
	var pickCount int64
	db.Model(&models.SleeperDraftPick{}).Count(&pickCount)
	if pickCount != 0 {
		t.Errorf("expected no picks refetched, got %d", pickCount)
	}
}

func TestSyncDraftsBatch_League404MarksSkipped(t *testing.T) {
	db := newTestDB(t)
	draftClaimedLeague(t, db, "gone")

	srv := draftsTestServer(t, map[string][]sleeper.Draft{}, nil, nil) // every league 404s
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runDraftsBatch(t, dfa, activities.SyncLeagueDraftsBatchParams{LeagueIDs: []string{"gone"}, Concurrency: 1})
	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("expected skip to count as processed, got %+v", res)
	}
	var lg models.SleeperLeague
	db.First(&lg, "sleeper_league_id = ?", "gone")
	if lg.SkippedAt == nil || lg.DraftsClaimedAt != nil {
		t.Errorf("league should be skipped and unclaimed: %+v", lg)
	}
	if lg.LastDraftsFetchedAt != nil {
		t.Errorf("skipped league must not be stamped fetched: %+v", lg)
	}
}

func TestSyncDraftsBatch_PerLeagueFailureDoesNotFailBatch(t *testing.T) {
	db := newTestDB(t)
	draftClaimedLeague(t, db, "bad")
	draftClaimedLeague(t, db, "good")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/league/bad/") {
			w.WriteHeader(http.StatusBadRequest) // non-retryable, non-404
			return
		}
		json.NewEncoder(w).Encode([]sleeper.Draft{})
	}))
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runDraftsBatch(t, dfa, activities.SyncLeagueDraftsBatchParams{LeagueIDs: []string{"bad", "good"}, Concurrency: 2})
	if res.Processed != 1 || res.Failed != 1 {
		t.Fatalf("expected 1/1, got %+v", res)
	}
	var bad, good models.SleeperLeague
	db.First(&bad, "sleeper_league_id = ?", "bad")
	db.First(&good, "sleeper_league_id = ?", "good")
	if bad.DraftsClaimedAt == nil || bad.LastDraftsFetchedAt != nil {
		t.Errorf("failed league must stay claimed and unstamped: %+v", bad)
	}
	if good.DraftsClaimedAt != nil || good.LastDraftsFetchedAt == nil {
		t.Errorf("good league must be stamped and unclaimed: %+v", good)
	}
}

func TestSyncDraftsBatch_RetrySkipsAlreadyStampedLeagues(t *testing.T) {
	db := newTestDB(t)
	// lg1 was stamped by a previous attempt (claim cleared); lg2 still claimed.
	now := time.Now().UTC()
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1", Season: "2026", LastFetchedAt: &now, LastDraftsFetchedAt: &now})
	draftClaimedLeague(t, db, "lg2")

	var calls atomic.Int64
	srv := draftsTestServer(t, map[string][]sleeper.Draft{"lg1": {}, "lg2": {}}, nil, &calls)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runDraftsBatch(t, dfa, activities.SyncLeagueDraftsBatchParams{LeagueIDs: []string{"lg1", "lg2"}, Concurrency: 1})
	if res.Processed != 1 {
		t.Fatalf("expected only still-claimed lg2 processed, got %+v", res)
	}
	// drafts call for lg2 only
	if got := calls.Load(); got != 1 {
		t.Errorf("expected 1 HTTP call, got %d", got)
	}
}

func TestSyncDraftsBatch_RoutesDraftToArchiveOnly(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)
	draftClaimedLeague(t, cloud, "lg1")

	srv := draftsTestServer(t,
		map[string][]sleeper.Draft{
			"lg1": {{DraftID: "d-old", Status: "complete", Type: "snake", Season: "2024"}},
		},
		map[string][]sleeper.DraftPick{
			"d-old": {{Round: 1, PickNo: 1, RosterID: 1, PlayerID: "p1"}},
		}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runDraftsBatch(t, dfa, activities.SyncLeagueDraftsBatchParams{LeagueIDs: []string{"lg1"}, Concurrency: 1})
	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("expected 1 processed / 0 failed, got %+v", res)
	}

	var cloudCount int64
	cloud.Model(&models.SleeperDraft{}).Count(&cloudCount)
	if cloudCount != 0 {
		t.Errorf("expected no draft rows in cloud, got %d", cloudCount)
	}
	var archiveDraft models.ArchiveSleeperDraft
	if err := archive.Where("sleeper_draft_id = ?", "d-old").First(&archiveDraft).Error; err != nil {
		t.Fatalf("expected d-old in archive: %v", err)
	}
	if archiveDraft.LastFetchedAt == nil {
		t.Error("expected archive draft's picks to be fetched (last_fetched_at set)")
	}
	var pickCount int64
	archive.Model(&models.ArchiveSleeperDraftPick{}).Where("sleeper_draft_id = ?", "d-old").Count(&pickCount)
	if pickCount != 1 {
		t.Errorf("expected 1 archived pick, got %d", pickCount)
	}
}

func TestSyncDraftsBatch_CurrentSeasonDraftRoutesToArchiveToo(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)
	draftClaimedLeague(t, cloud, "lg1")

	srv := draftsTestServer(t,
		map[string][]sleeper.Draft{"lg1": {{DraftID: "d-current", Status: "complete", Type: "snake", Season: "2026"}}},
		map[string][]sleeper.DraftPick{"d-current": {{Round: 1, PickNo: 1, RosterID: 1, PlayerID: "p1"}}}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	runDraftsBatch(t, dfa, activities.SyncLeagueDraftsBatchParams{LeagueIDs: []string{"lg1"}, Concurrency: 1})

	var cloudCount, archiveCount int64
	cloud.Model(&models.SleeperDraft{}).Where("sleeper_draft_id = ?", "d-current").Count(&cloudCount)
	archive.Model(&models.ArchiveSleeperDraft{}).Where("sleeper_draft_id = ?", "d-current").Count(&archiveCount)
	if cloudCount != 0 {
		t.Errorf("expected current-season draft NOT in cloud when Archive is configured, got %d", cloudCount)
	}
	if archiveCount != 1 {
		t.Errorf("expected current-season draft in archive, got %d", archiveCount)
	}
}

func TestSyncDraftsBatch_AllDraftsToCloudWhenArchiveNil(t *testing.T) {
	cloud := newTestDB(t)
	draftClaimedLeague(t, cloud, "lg1")

	srv := draftsTestServer(t,
		map[string][]sleeper.Draft{"lg1": {{DraftID: "d-old", Status: "complete", Type: "snake", Season: "2024"}}},
		map[string][]sleeper.DraftPick{"d-old": {{Round: 1, PickNo: 1, RosterID: 1, PlayerID: "p1"}}}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Sleeper: sleeper.NewWithBaseURL(srv.URL)} // Archive nil
	runDraftsBatch(t, dfa, activities.SyncLeagueDraftsBatchParams{LeagueIDs: []string{"lg1"}, Concurrency: 1})

	var count int64
	cloud.Model(&models.SleeperDraft{}).Where("sleeper_draft_id = ?", "d-old").Count(&count)
	if count != 1 {
		t.Errorf("expected old draft to fall back to cloud when Archive is nil, got %d", count)
	}
}

func TestSyncDraftsBatch_OldDraftPicksFetchOnce(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)
	draftClaimedLeague(t, cloud, "lg1")
	// Old draft already fetched by an earlier sweep — into archive, not cloud.
	fetched := time.Now().UTC()
	archive.Create(&models.ArchiveSleeperDraft{
		SleeperDraftID: "d-old", SleeperLeagueID: "lg1", Status: "complete", Season: "2024", LastFetchedAt: &fetched,
	})

	var calls atomic.Int64
	srv := draftsTestServer(t,
		map[string][]sleeper.Draft{"lg1": {{DraftID: "d-old", Status: "complete", Type: "snake", Season: "2024"}}},
		map[string][]sleeper.DraftPick{"d-old": {{Round: 1, PickNo: 1, RosterID: 1, PlayerID: "p1"}}}, &calls)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runDraftsBatch(t, dfa, activities.SyncLeagueDraftsBatchParams{LeagueIDs: []string{"lg1"}, Concurrency: 1})
	if res.Processed != 1 {
		t.Fatalf("expected 1 processed, got %+v", res)
	}
	// Only the /drafts call — no /picks call for the already-fetched archived draft.
	if got := calls.Load(); got != 1 {
		t.Errorf("expected 1 HTTP call (drafts only), got %d", got)
	}
	var pickCount int64
	archive.Model(&models.ArchiveSleeperDraftPick{}).Count(&pickCount)
	if pickCount != 0 {
		t.Errorf("expected no picks refetched, got %d", pickCount)
	}
}
