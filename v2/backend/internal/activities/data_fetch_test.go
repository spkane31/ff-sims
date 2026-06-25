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
	if err := dfa.FetchLeagueTransactions(context.Background(), activities.FetchLeagueTransactionsParams{LeagueID: "lg1"}); err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}

	var count int64
	db.Model(&models.SleeperTransaction{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 transaction, got %d", count)
	}
}

func TestGetStaleLeaguesForDrafts_NullFirst(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	old := now.Add(-1 * time.Hour)

	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-recent", LastDraftsFetchedAt: &now, LastFetchedAt: &now})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-old", LastDraftsFetchedAt: &old, LastFetchedAt: &now})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-null", LastFetchedAt: &now})

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
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-no-details"})
	now := time.Now()
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-ready", LastFetchedAt: &now})

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

	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-recent", LastTransactionsFetchedAt: &now, LastFetchedAt: &now})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-old", LastTransactionsFetchedAt: &old, LastFetchedAt: &now})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-null", LastFetchedAt: &now})

	dfa := &activities.DataFetchActivities{DB: db}
	ids, err := dfa.GetStaleLeaguesForTransactions(context.Background(), activities.GetStaleLeaguesParams{BatchSize: 2})
	if err != nil {
		t.Fatalf("GetStaleLeaguesForTransactions error: %v", err)
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

func TestMarkLeagueTransactionsFetched_SetsTimestamp(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1"})

	dfa := &activities.DataFetchActivities{DB: db}
	if err := dfa.MarkLeagueTransactionsFetched(context.Background(), activities.MarkLeagueFetchedParams{LeagueID: "lg1"}); err != nil {
		t.Fatalf("MarkLeagueTransactionsFetched error: %v", err)
	}

	var l models.SleeperLeague
	db.First(&l, "sleeper_league_id = ?", "lg1")
	if l.LastTransactionsFetchedAt == nil {
		t.Error("expected last_transactions_fetched_at to be set")
	}
}
