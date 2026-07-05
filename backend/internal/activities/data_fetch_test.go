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
	"gorm.io/gorm"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/sleeper"
)

func TestFetchLeagueDrafts_ReturnsCompletedOnly(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]sleeper.Draft{
			{DraftID: "d1", Status: "complete", Type: "snake", Season: "2024"},
			{DraftID: "d2", Status: "in_progress", Type: "snake", Season: "2024"},
		})
	}))
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	completedIDs, err := dfa.FetchLeagueDrafts(context.Background(), activities.FetchLeagueDraftsParams{LeagueID: "lg1"})
	if err != nil {
		t.Fatalf("FetchLeagueDrafts error: %v", err)
	}
	if len(completedIDs) != 1 || completedIDs[0] != "d1" {
		t.Errorf("expected [d1], got %v", completedIDs)
	}

	var count int64
	db.Model(&models.SleeperDraft{}).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 drafts in DB, got %d", count)
	}
}

func TestFetchLeagueTransactions_InsertsAndSkips404(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 404 for leg 1, one transaction for leg 2, empty for rest
		if strings.HasSuffix(r.URL.Path, "/1") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/2") {
			json.NewEncoder(w).Encode([]sleeper.Transaction{
				{TransactionID: "tx1", Type: "trade", Status: "complete", Leg: 2},
			})
			return
		}
		json.NewEncoder(w).Encode([]sleeper.Transaction{})
	}))
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	maxLeg, err := dfa.FetchLeagueTransactions(context.Background(), activities.FetchLeagueTransactionsParams{LeagueID: "lg1"})
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if maxLeg != 2 {
		t.Errorf("expected maxLeg 2, got %d", maxLeg)
	}

	var count int64
	db.Model(&models.SleeperTransaction{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 transaction, got %d", count)
	}
}

func TestFetchLeagueTransactions_StartsFromCursor(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1"})

	var requestedLegs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		requestedLegs = append(requestedLegs, parts[len(parts)-1])
		json.NewEncoder(w).Encode([]sleeper.Transaction{})
	}))
	defer srv.Close()

	lastLeg := 5
	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if _, err := dfa.FetchLeagueTransactions(context.Background(), activities.FetchLeagueTransactionsParams{
		LeagueID:       "lg1",
		LastLegFetched: &lastLeg,
	}); err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}

	if len(requestedLegs) == 0 || requestedLegs[0] != "4" {
		t.Errorf("expected first request to be leg 4 (N-1), got %v", requestedLegs)
	}
}

func TestGetStaleLeaguesForDrafts_NullFirst(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	old := now.Add(-1 * time.Hour)

	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-recent", Season: "2025", LastDraftsFetchedAt: &now, LastFetchedAt: &now})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-old", Season: "2025", LastDraftsFetchedAt: &old, LastFetchedAt: &now})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-null", Season: "2025", LastFetchedAt: &now})

	dfa := &activities.DataFetchActivities{DB: db}
	ids, err := dfa.GetStaleLeaguesForDrafts(context.Background(), activities.GetStaleLeaguesParams{BatchSize: 2})
	if err != nil {
		t.Fatalf("GetStaleLeaguesForDrafts error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2, got %d: %v", len(ids), ids)
	}
	if ids[0] != "lg-null" {
		t.Errorf("expected lg-null first, got %q", ids[0])
	}
	if ids[1] != "lg-old" {
		t.Errorf("expected lg-old second, got %q", ids[1])
	}
}

func TestGetStaleLeaguesForDrafts_RequiresDetailsFetched(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-no-details", Season: "2025"})
	now := time.Now()
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-ready", Season: "2025", LastFetchedAt: &now})

	dfa := &activities.DataFetchActivities{DB: db}
	ids, err := dfa.GetStaleLeaguesForDrafts(context.Background(), activities.GetStaleLeaguesParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "lg-ready" {
		t.Errorf("expected [lg-ready], got %v", ids)
	}
}

func TestGetStaleLeaguesForTransactions_NullFirst(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	old := now.Add(-1 * time.Hour)

	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-recent", Season: "2025", LastTransactionsFetchedAt: &now, LastFetchedAt: &now})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-old", Season: "2025", LastTransactionsFetchedAt: &old, LastFetchedAt: &now})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-null", Season: "2025", LastFetchedAt: &now})

	dfa := &activities.DataFetchActivities{DB: db}
	states, err := dfa.GetStaleLeaguesForTransactions(context.Background(), activities.GetStaleLeaguesParams{BatchSize: 2})
	if err != nil {
		t.Fatalf("GetStaleLeaguesForTransactions error: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("expected 2, got %d: %v", len(states), states)
	}
	if states[0].LeagueID != "lg-null" {
		t.Errorf("expected lg-null first, got %q", states[0].LeagueID)
	}
	if states[1].LeagueID != "lg-old" {
		t.Errorf("expected lg-old second, got %q", states[1].LeagueID)
	}
}

func TestGetStaleLeaguesForTransactions_ExcludesCompletedSynced(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()

	// complete + already synced — should be excluded
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-done", Season: "2025", Status: "complete", LastFetchedAt: &now, LastTransactionsFetchedAt: &now})
	// complete but never synced — should be included
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-complete-new", Season: "2025", Status: "complete", LastFetchedAt: &now})
	// active + already synced — should be included
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-active", Season: "2025", Status: "in_season", LastFetchedAt: &now, LastTransactionsFetchedAt: &now})

	dfa := &activities.DataFetchActivities{DB: db}
	states, err := dfa.GetStaleLeaguesForTransactions(context.Background(), activities.GetStaleLeaguesParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("GetStaleLeaguesForTransactions error: %v", err)
	}
	ids := make([]string, len(states))
	for i, s := range states {
		ids[i] = s.LeagueID
	}
	for _, id := range ids {
		if id == "lg-done" {
			t.Error("completed+synced league should be excluded")
		}
	}
	found := map[string]bool{}
	for _, id := range ids {
		found[id] = true
	}
	if !found["lg-complete-new"] {
		t.Error("complete but unsynced league should be included")
	}
	if !found["lg-active"] {
		t.Error("active league should be included")
	}
}

func TestMarkLeagueDraftsFetched_SetsTimestamp(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1"})

	dfa := &activities.DataFetchActivities{DB: db}
	if err := dfa.MarkLeagueDraftsFetched(context.Background(), activities.MarkLeagueFetchedParams{LeagueID: "lg1"}); err != nil {
		t.Fatalf("MarkLeagueDraftsFetched error: %v", err)
	}

	var l models.SleeperLeague
	db.First(&l, "sleeper_league_id = ?", "lg1")
	if l.LastDraftsFetchedAt == nil {
		t.Error("expected last_drafts_fetched_at to be set")
	}
}

func TestMarkLeagueTransactionsFetched_SetsTimestampAndLeg(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1"})

	dfa := &activities.DataFetchActivities{DB: db}
	if err := dfa.MarkLeagueTransactionsFetched(context.Background(), activities.MarkLeagueTransactionsFetchedParams{LeagueID: "lg1", MaxLeg: 7}); err != nil {
		t.Fatalf("MarkLeagueTransactionsFetched error: %v", err)
	}

	var l models.SleeperLeague
	db.First(&l, "sleeper_league_id = ?", "lg1")
	if l.LastTransactionsFetchedAt == nil {
		t.Error("expected last_transactions_fetched_at to be set")
	}
	if l.LastTransactionLegFetched == nil || *l.LastTransactionLegFetched != 7 {
		t.Errorf("expected last_transaction_leg_fetched=7, got %v", l.LastTransactionLegFetched)
	}
}

func TestMarkLeagueTransactionsFetched_ZeroLegSkipsLegUpdate(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1"})

	dfa := &activities.DataFetchActivities{DB: db}
	if err := dfa.MarkLeagueTransactionsFetched(context.Background(), activities.MarkLeagueTransactionsFetchedParams{LeagueID: "lg1", MaxLeg: 0}); err != nil {
		t.Fatalf("MarkLeagueTransactionsFetched error: %v", err)
	}

	var l models.SleeperLeague
	db.First(&l, "sleeper_league_id = ?", "lg1")
	if l.LastTransactionsFetchedAt == nil {
		t.Error("expected last_transactions_fetched_at to be set")
	}
	if l.LastTransactionLegFetched != nil {
		t.Errorf("expected last_transaction_leg_fetched to remain NULL, got %v", *l.LastTransactionLegFetched)
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
	// 1 state call + legs 1..3 = 4 total; the old code would have made 19.
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
