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

// batchTestServer fakes /v1/state/nfl plus per-league transaction legs.
// legs maps "leagueID/leg" -> transactions; missing keys 404 (empty leg).
func batchTestServer(t *testing.T, week int, legs map[string][]sleeper.Transaction, calls *atomic.Int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			calls.Add(1)
		}
		if strings.HasSuffix(r.URL.Path, "/state/nfl") {
			json.NewEncoder(w).Encode(sleeper.NFLState{Season: "2026", SeasonType: "regular", Week: week})
			return
		}
		// path: /v1/league/{id}/transactions/{leg}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		key := parts[2] + "/" + parts[4]
		txns, ok := legs[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(txns)
	}))
}

func runBatch(t *testing.T, dfa *activities.DataFetchActivities, params activities.SyncLeagueTransactionsBatchParams) activities.SyncBatchResult {
	t.Helper()
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestActivityEnvironment()
	env.RegisterActivity(dfa.SyncLeagueTransactionsBatch)
	val, err := env.ExecuteActivity(dfa.SyncLeagueTransactionsBatch, params)
	if err != nil {
		t.Fatalf("batch activity: %v", err)
	}
	var res activities.SyncBatchResult
	if err := val.Get(&res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return res
}

func claimedLeague(t *testing.T, db *gorm.DB, id string) models.SleeperLeague {
	t.Helper()
	now := time.Now().UTC()
	l := models.SleeperLeague{SleeperLeagueID: id, Season: "2026", LastFetchedAt: &now, ClaimedAt: &now}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return l
}

func TestSyncBatch_StampsAndClearsClaims(t *testing.T) {
	db := newTestDB(t)
	claimedLeague(t, db, "lg1")
	claimedLeague(t, db, "lg2")

	srv := batchTestServer(t, 3, map[string][]sleeper.Transaction{
		"lg1/2": {{TransactionID: "tx1", Type: "waiver", Status: "complete", Leg: 2}},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runBatch(t, dfa, activities.SyncLeagueTransactionsBatchParams{
		Leagues: []activities.LeagueTransactionState{
			{LeagueID: "lg1", Season: "2026"},
			{LeagueID: "lg2", Season: "2026"},
		},
		Concurrency: 2,
	})
	if res.Processed != 2 || res.Failed != 0 {
		t.Fatalf("expected 2 processed / 0 failed, got %+v", res)
	}

	var lg1 models.SleeperLeague
	db.First(&lg1, "sleeper_league_id = ?", "lg1")
	if lg1.LastTransactionsFetchedAt == nil || lg1.ClaimedAt != nil {
		t.Errorf("lg1 not stamped/unclaimed: %+v", lg1)
	}
	if lg1.LastTransactionLegFetched == nil || *lg1.LastTransactionLegFetched != 2 {
		t.Errorf("lg1 leg cursor = %v, want 2", lg1.LastTransactionLegFetched)
	}
	var txCount int64
	db.Model(&models.SleeperTransaction{}).Count(&txCount)
	if txCount != 1 {
		t.Errorf("expected 1 transaction row, got %d", txCount)
	}
}

func TestSyncBatch_LegLoopCappedAtCurrentWeek(t *testing.T) {
	db := newTestDB(t)
	claimedLeague(t, db, "lg1")

	var calls atomic.Int64
	srv := batchTestServer(t, 3, nil, &calls) // all legs 404
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	runBatch(t, dfa, activities.SyncLeagueTransactionsBatchParams{
		Leagues:     []activities.LeagueTransactionState{{LeagueID: "lg1", Season: "2026"}},
		Concurrency: 1,
	})
	// 1 state call + legs 1..3 = 4 total.
	if got := calls.Load(); got != 4 {
		t.Errorf("expected 4 HTTP calls (state + 3 legs), got %d", got)
	}
}

func TestSyncBatch_PastSeasonFetchesAllLegs(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().UTC()
	l := models.SleeperLeague{SleeperLeagueID: "lg1", Season: "2025", LastFetchedAt: &now, ClaimedAt: &now}
	db.Create(&l)

	var calls atomic.Int64
	srv := batchTestServer(t, 3, nil, &calls)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	runBatch(t, dfa, activities.SyncLeagueTransactionsBatchParams{
		Leagues:     []activities.LeagueTransactionState{{LeagueID: "lg1", Season: "2025"}},
		Concurrency: 1,
	})
	// 2025 < state season 2026: full historical sweep, legs 1..18 + state = 19.
	if got := calls.Load(); got != 19 {
		t.Errorf("expected 19 HTTP calls, got %d", got)
	}
}

func TestSyncBatch_PerLeagueFailureDoesNotFailBatch(t *testing.T) {
	db := newTestDB(t)
	claimedLeague(t, db, "bad")
	claimedLeague(t, db, "good")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/state/nfl") {
			json.NewEncoder(w).Encode(sleeper.NFLState{Season: "2026", Week: 1})
			return
		}
		if strings.Contains(r.URL.Path, "/league/bad/") {
			w.WriteHeader(http.StatusBadRequest) // non-retryable, non-404
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runBatch(t, dfa, activities.SyncLeagueTransactionsBatchParams{
		Leagues: []activities.LeagueTransactionState{
			{LeagueID: "bad", Season: "2026"},
			{LeagueID: "good", Season: "2026"},
		},
		Concurrency: 2,
	})
	if res.Processed != 1 || res.Failed != 1 {
		t.Fatalf("expected 1/1, got %+v", res)
	}
	var bad, good models.SleeperLeague
	db.First(&bad, "sleeper_league_id = ?", "bad")
	db.First(&good, "sleeper_league_id = ?", "good")
	if bad.ClaimedAt == nil || bad.LastTransactionsFetchedAt != nil {
		t.Errorf("failed league must stay claimed and unstamped: %+v", bad)
	}
	if good.ClaimedAt != nil || good.LastTransactionsFetchedAt == nil {
		t.Errorf("good league must be stamped and unclaimed: %+v", good)
	}
}

func TestSyncBatch_RetrySkipsAlreadyStampedLeagues(t *testing.T) {
	db := newTestDB(t)
	// lg1 was stamped by a previous attempt (claim cleared); lg2 still claimed.
	now := time.Now().UTC()
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1", Season: "2026", LastFetchedAt: &now, LastTransactionsFetchedAt: &now})
	claimedLeague(t, db, "lg2")

	var calls atomic.Int64
	srv := batchTestServer(t, 1, nil, &calls)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runBatch(t, dfa, activities.SyncLeagueTransactionsBatchParams{
		Leagues: []activities.LeagueTransactionState{
			{LeagueID: "lg1", Season: "2026"},
			{LeagueID: "lg2", Season: "2026"},
		},
		Concurrency: 1,
	})
	if res.Processed != 1 {
		t.Fatalf("expected only still-claimed lg2 processed, got %+v", res)
	}
	// state + leg 1 for lg2 only
	if got := calls.Load(); got != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", got)
	}
}

func TestSyncBatch_RoutesOldTransactionsToArchiveOnly(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)
	claimedLeague(t, cloud, "lg1")

	now := time.Now().UTC()
	recentMs := now.Add(-1 * time.Hour).UnixMilli()
	oldMs := now.AddDate(0, 0, -60).UnixMilli() // 60 days ago, past the 30-day default retention

	srv := batchTestServer(t, 3, map[string][]sleeper.Transaction{
		"lg1/2": {
			{TransactionID: "tx-recent", Type: "waiver", Status: "complete", Leg: 2, Created: recentMs},
			{TransactionID: "tx-old", Type: "waiver", Status: "complete", Leg: 2, Created: oldMs},
		},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runBatch(t, dfa, activities.SyncLeagueTransactionsBatchParams{
		Leagues:     []activities.LeagueTransactionState{{LeagueID: "lg1", Season: "2026"}},
		Concurrency: 1,
	})
	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("expected 1 processed / 0 failed, got %+v", res)
	}

	var cloudIDs, archiveIDs []string
	cloud.Model(&models.SleeperTransaction{}).Pluck("sleeper_transaction_id", &cloudIDs)
	archive.Model(&models.ArchiveSleeperTransaction{}).Pluck("sleeper_transaction_id", &archiveIDs)
	if len(cloudIDs) != 1 || cloudIDs[0] != "tx-recent" {
		t.Errorf("expected only tx-recent in cloud, got %v", cloudIDs)
	}
	if len(archiveIDs) != 1 || archiveIDs[0] != "tx-old" {
		t.Errorf("expected only tx-old in archive, got %v", archiveIDs)
	}
}

func TestSyncBatch_ArchiveExcludesTransactionsWithDraftPicksOrFAAB(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)
	claimedLeague(t, cloud, "lg1")

	oldMs := time.Now().UTC().AddDate(0, 0, -60).UnixMilli() // past the 30-day default retention
	srv := batchTestServer(t, 3, map[string][]sleeper.Transaction{
		"lg1/2": {
			{TransactionID: "tx-clean", Type: "waiver", Status: "complete", Leg: 2, Created: oldMs},
			{TransactionID: "tx-picks", Type: "trade", Status: "complete", Leg: 2, Created: oldMs,
				DraftPicks: []interface{}{map[string]interface{}{"season": "2027", "round": float64(1)}}},
			{TransactionID: "tx-faab", Type: "waiver", Status: "complete", Leg: 2, Created: oldMs,
				WaiverBudget: []interface{}{map[string]interface{}{"amount": float64(10)}}},
		},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	runBatch(t, dfa, activities.SyncLeagueTransactionsBatchParams{
		Leagues:     []activities.LeagueTransactionState{{LeagueID: "lg1", Season: "2026"}},
		Concurrency: 1,
	})

	var archiveIDs []string
	archive.Model(&models.ArchiveSleeperTransaction{}).Pluck("sleeper_transaction_id", &archiveIDs)
	if len(archiveIDs) != 1 || archiveIDs[0] != "tx-clean" {
		t.Errorf("expected only tx-clean in archive, got %v", archiveIDs)
	}
	var cloudCount int64
	cloud.Model(&models.SleeperTransaction{}).Count(&cloudCount)
	if cloudCount != 0 {
		t.Errorf("expected no rows in cloud (all old), got %d", cloudCount)
	}
}

func TestSyncBatch_AllTransactionsToCloudWhenArchiveNil(t *testing.T) {
	cloud := newTestDB(t)
	claimedLeague(t, cloud, "lg1")

	oldMs := time.Now().UTC().AddDate(0, 0, -60).UnixMilli()
	srv := batchTestServer(t, 3, map[string][]sleeper.Transaction{
		"lg1/2": {{TransactionID: "tx-old", Type: "waiver", Status: "complete", Leg: 2, Created: oldMs}},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Sleeper: sleeper.NewWithBaseURL(srv.URL)} // Archive nil
	runBatch(t, dfa, activities.SyncLeagueTransactionsBatchParams{
		Leagues:     []activities.LeagueTransactionState{{LeagueID: "lg1", Season: "2026"}},
		Concurrency: 1,
	})

	var count int64
	cloud.Model(&models.SleeperTransaction{}).Count(&count)
	if count != 1 {
		t.Errorf("expected the old txn to fall back to cloud when Archive is nil, got %d rows", count)
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
