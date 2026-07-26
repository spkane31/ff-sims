package activities_test

import (
	"context"
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

// batchTestServer fakes per-league transaction legs (/v1/league/{id}/transactions/{leg}).
// legs maps "leagueID/leg" -> transactions; missing keys 404 (empty leg).
// FetchLeagueTransactions never calls /v1/state/nfl itself (that's
// transactioncron's job, once per run, to compute maxLeg via
// MaxLegForLeague) so this fake doesn't need to serve it.
func batchTestServer(t *testing.T, legs map[string][]sleeper.Transaction, calls *atomic.Int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			calls.Add(1)
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

func claimedLeague(t *testing.T, db *gorm.DB, id string) models.SleeperLeague {
	t.Helper()
	now := time.Now().UTC()
	l := models.SleeperLeague{SleeperLeagueID: id, Season: "2026", LastFetchedAt: &now, ClaimedAt: &now}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return l
}

func TestMaxLegForLeague(t *testing.T) {
	tests := []struct {
		name   string
		season string
		state  *sleeper.NFLState
		want   int
	}{
		{"nil state falls back to full sweep", "2026", nil, 18},
		{"past season gets full sweep", "2025", &sleeper.NFLState{Season: "2026", Week: 3}, 18},
		{"current season capped at current week", "2026", &sleeper.NFLState{Season: "2026", Week: 5}, 5},
		{"current season week beyond 18 clamps to 18", "2026", &sleeper.NFLState{Season: "2026", Week: 20}, 18},
		{"offseason week 0 still fetches leg 1", "2026", &sleeper.NFLState{Season: "2026", Week: 0}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := activities.MaxLegForLeague(tt.season, tt.state); got != tt.want {
				t.Errorf("MaxLegForLeague(%q, %+v) = %d, want %d", tt.season, tt.state, got, tt.want)
			}
		})
	}
}

func TestFetchLeagueTransactions_FetchesLegsUpToMaxLeg(t *testing.T) {
	db := newTestDB(t)
	var calls atomic.Int64
	srv := batchTestServer(t, nil, &calls) // all legs 404
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if _, err := dfa.FetchLeagueTransactions(context.Background(), db, activities.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, 3); err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("expected exactly 3 HTTP calls (legs 1..3), got %d", got)
	}
}

func TestFetchLeagueTransactions_ResumesFromLastLegFetched(t *testing.T) {
	db := newTestDB(t)
	var calls atomic.Int64
	srv := batchTestServer(t, nil, &calls) // all legs 404
	defer srv.Close()

	lastLeg := 5
	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if _, err := dfa.FetchLeagueTransactions(context.Background(), db,
		activities.LeagueTransactionState{LeagueID: "lg1", Season: "2026", LastLegFetched: &lastLeg}, 7); err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	// Resumes at lastLeg-1 (4) through maxLeg (7): legs 4,5,6,7 = 4 calls.
	if got := calls.Load(); got != 4 {
		t.Errorf("expected 4 HTTP calls (legs 4..7), got %d", got)
	}
}

func TestFetchLeagueTransactions_ReturnsRowsAndMaxLegSeen(t *testing.T) {
	db := newTestDB(t)
	srv := batchTestServer(t, map[string][]sleeper.Transaction{
		"lg1/2": {{TransactionID: "tx1", Type: "waiver", Status: "complete", Leg: 2}},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res, err := dfa.FetchLeagueTransactions(context.Background(), db, activities.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, 3)
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if len(res.CloudRows) != 1 || res.CloudRows[0].SleeperTransactionID != "tx1" {
		t.Fatalf("expected tx1 in CloudRows, got %+v", res.CloudRows)
	}
	if res.MaxLegSeen != 2 {
		t.Errorf("expected MaxLegSeen == 2, got %d", res.MaxLegSeen)
	}
}

func TestFetchLeagueTransactions_PropagatesNonNotFoundLegErrors(t *testing.T) {
	db := newTestDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // non-retryable, non-404
	}))
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if _, err := dfa.FetchLeagueTransactions(context.Background(), db, activities.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, 3); err == nil {
		t.Fatal("expected a non-404 leg error to propagate")
	}
}

func TestFetchLeagueTransactions_SplitsOldRowsToArchiveByAge(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)

	now := time.Now().UTC()
	recentMs := now.Add(-1 * time.Hour).UnixMilli()
	oldMs := now.AddDate(0, 0, -60).UnixMilli() // 60 days ago, past the 30-day default retention

	srv := batchTestServer(t, map[string][]sleeper.Transaction{
		"lg1/2": {
			{TransactionID: "tx-recent", Type: "waiver", Status: "complete", Leg: 2, Created: recentMs},
			{TransactionID: "tx-old", Type: "waiver", Status: "complete", Leg: 2, Created: oldMs},
		},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res, err := dfa.FetchLeagueTransactions(context.Background(), cloud, activities.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, 3)
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if len(res.CloudRows) != 1 || res.CloudRows[0].SleeperTransactionID != "tx-recent" {
		t.Errorf("expected only tx-recent in CloudRows, got %+v", res.CloudRows)
	}
	if len(res.ArchiveRows) != 1 || res.ArchiveRows[0].SleeperTransactionID != "tx-old" {
		t.Errorf("expected only tx-old in ArchiveRows, got %+v", res.ArchiveRows)
	}
}

func TestFetchLeagueTransactions_ArchiveExcludesTransactionsWithDraftPicksOrFAAB(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)

	oldMs := time.Now().UTC().AddDate(0, 0, -60).UnixMilli() // past the 30-day default retention
	srv := batchTestServer(t, map[string][]sleeper.Transaction{
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
	res, err := dfa.FetchLeagueTransactions(context.Background(), cloud, activities.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, 3)
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if len(res.ArchiveRows) != 1 || res.ArchiveRows[0].SleeperTransactionID != "tx-clean" {
		t.Errorf("expected only tx-clean in ArchiveRows, got %+v", res.ArchiveRows)
	}
	if len(res.CloudRows) != 0 {
		t.Errorf("expected no CloudRows (all old), got %+v", res.CloudRows)
	}
}

// TestFetchLeagueTransactions_ExcludesDraftPicksOrFAABEvenWhenCloudBound
// guards the fix from #191: isPlayerOnlyTransaction must apply
// unconditionally to every fetched row, not only the ones old enough to
// route to archive — otherwise picks/FAAB trades leak into the live
// sleeper_transactions table via CloudRows.
func TestFetchLeagueTransactions_ExcludesDraftPicksOrFAABEvenWhenCloudBound(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)

	recentMs := time.Now().UTC().Add(-1 * time.Hour).UnixMilli()
	srv := batchTestServer(t, map[string][]sleeper.Transaction{
		"lg1/2": {
			{TransactionID: "tx-clean", Type: "waiver", Status: "complete", Leg: 2, Created: recentMs},
			{TransactionID: "tx-picks", Type: "trade", Status: "complete", Leg: 2, Created: recentMs,
				DraftPicks: []interface{}{map[string]interface{}{"season": "2027", "round": float64(1)}}},
			{TransactionID: "tx-faab", Type: "waiver", Status: "complete", Leg: 2, Created: recentMs,
				WaiverBudget: []interface{}{map[string]interface{}{"amount": float64(10)}}},
		},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res, err := dfa.FetchLeagueTransactions(context.Background(), cloud, activities.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, 3)
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if len(res.CloudRows) != 1 || res.CloudRows[0].SleeperTransactionID != "tx-clean" {
		t.Errorf("expected only tx-clean in CloudRows, got %+v", res.CloudRows)
	}
	if len(res.ArchiveRows) != 0 {
		t.Errorf("expected no ArchiveRows (all recent), got %+v", res.ArchiveRows)
	}
}

func TestFetchLeagueTransactions_AllRowsToCloudWhenArchiveNil(t *testing.T) {
	cloud := newTestDB(t)
	oldMs := time.Now().UTC().AddDate(0, 0, -60).UnixMilli()
	srv := batchTestServer(t, map[string][]sleeper.Transaction{
		"lg1/2": {{TransactionID: "tx-old", Type: "waiver", Status: "complete", Leg: 2, Created: oldMs}},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Sleeper: sleeper.NewWithBaseURL(srv.URL)} // Archive nil
	res, err := dfa.FetchLeagueTransactions(context.Background(), cloud, activities.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, 3)
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if len(res.CloudRows) != 1 {
		t.Errorf("expected the old txn to fall back to CloudRows when Archive is nil, got %+v", res.CloudRows)
	}
	if len(res.ArchiveRows) != 0 {
		t.Errorf("expected no ArchiveRows when Archive is nil, got %+v", res.ArchiveRows)
	}
}

func TestFlushLeagueTransactions_StampsClearsClaimsAndWritesRows(t *testing.T) {
	db := newTestDB(t)
	claimedLeague(t, db, "lg1")
	claimedLeague(t, db, "lg2")

	batch := []activities.LeagueTransactionFetchResult{
		{
			LeagueID:   "lg1",
			CloudRows:  []models.SleeperTransaction{{SleeperTransactionID: "tx1", SleeperLeagueID: "lg1", Type: "waiver", Status: "complete", Leg: 2}},
			MaxLegSeen: 2,
		},
		{LeagueID: "lg2"}, // nothing new this run — MaxLegSeen 0
	}
	dfa := &activities.DataFetchActivities{DB: db}
	if err := dfa.FlushLeagueTransactions(context.Background(), db, batch); err != nil {
		t.Fatalf("FlushLeagueTransactions error: %v", err)
	}

	var lg1, lg2 models.SleeperLeague
	db.First(&lg1, "sleeper_league_id = ?", "lg1")
	db.First(&lg2, "sleeper_league_id = ?", "lg2")
	if lg1.LastTransactionsFetchedAt == nil || lg1.ClaimedAt != nil {
		t.Errorf("lg1 not stamped/unclaimed: %+v", lg1)
	}
	if lg1.LastTransactionLegFetched == nil || *lg1.LastTransactionLegFetched != 2 {
		t.Errorf("lg1 leg cursor = %v, want 2", lg1.LastTransactionLegFetched)
	}
	if lg2.LastTransactionsFetchedAt == nil || lg2.ClaimedAt != nil {
		t.Errorf("lg2 not stamped/unclaimed: %+v", lg2)
	}
	if lg2.LastTransactionLegFetched != nil {
		t.Errorf("expected lg2's leg cursor untouched (MaxLegSeen 0), got %v", lg2.LastTransactionLegFetched)
	}
	var txCount int64
	db.Model(&models.SleeperTransaction{}).Count(&txCount)
	if txCount != 1 {
		t.Errorf("expected 1 transaction row, got %d", txCount)
	}
}

func TestFlushLeagueTransactions_WritesArchiveRowsToArchiveDB(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)
	claimedLeague(t, cloud, "lg1")

	batch := []activities.LeagueTransactionFetchResult{
		{
			LeagueID:    "lg1",
			ArchiveRows: []models.SleeperTransaction{{SleeperTransactionID: "tx-old", SleeperLeagueID: "lg1", Type: "waiver", Status: "complete", Leg: 2}},
			MaxLegSeen:  2,
		},
	}
	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive}
	if err := dfa.FlushLeagueTransactions(context.Background(), cloud, batch); err != nil {
		t.Fatalf("FlushLeagueTransactions error: %v", err)
	}

	var archiveIDs []string
	archive.Model(&models.ArchiveSleeperTransaction{}).Pluck("sleeper_transaction_id", &archiveIDs)
	if len(archiveIDs) != 1 || archiveIDs[0] != "tx-old" {
		t.Errorf("expected tx-old written to archive, got %v", archiveIDs)
	}
	var cloudCount int64
	cloud.Model(&models.SleeperTransaction{}).Count(&cloudCount)
	if cloudCount != 0 {
		t.Errorf("expected no cloud rows (batch had only ArchiveRows), got %d", cloudCount)
	}
}

func TestFetchLeagueTransactions_ExcludesDraftPicksWhenArchiveNil(t *testing.T) {
	cloud := newTestDB(t)

	recentMs := time.Now().UTC().Add(-1 * time.Hour).UnixMilli()
	srv := batchTestServer(t, map[string][]sleeper.Transaction{
		"lg1/2": {
			{TransactionID: "tx-clean", Type: "waiver", Status: "complete", Leg: 2, Created: recentMs},
			{TransactionID: "tx-picks", Type: "trade", Status: "complete", Leg: 2, Created: recentMs,
				DraftPicks: []interface{}{map[string]interface{}{"season": "2027", "round": float64(1)}}},
		},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Sleeper: sleeper.NewWithBaseURL(srv.URL)} // Archive nil
	res, err := dfa.FetchLeagueTransactions(context.Background(), cloud, activities.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, 3)
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if len(res.CloudRows) != 1 || res.CloudRows[0].SleeperTransactionID != "tx-clean" {
		t.Errorf("expected only tx-clean in CloudRows (no archive configured), got %+v", res.CloudRows)
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
