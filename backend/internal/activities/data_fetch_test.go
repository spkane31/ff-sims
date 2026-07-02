package activities_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
