package activities_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"workers/internal/activities"
	"workers/internal/models"
	"workers/internal/sleeper"
)

func TestGetStaleLeagues_NullFirst(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	old := now.Add(-1 * time.Hour)

	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-recent", LastFetchedAt: &now})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-old", LastFetchedAt: &old})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-null"}) // NULL last_fetched_at

	dfa := &activities.DataFetchActivities{DB: db}
	ids, err := dfa.GetStaleLeagues(context.Background(), activities.GetStaleLeaguesParams{BatchSize: 2})
	if err != nil {
		t.Fatalf("GetStaleLeagues error: %v", err)
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

func TestGetStaleLeagues_ExcludesSkipped(t *testing.T) {
	db := newTestDB(t)
	skipped := time.Now()
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-skip", SkippedAt: &skipped})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-ok"})

	dfa := &activities.DataFetchActivities{DB: db}
	ids, err := dfa.GetStaleLeagues(context.Background(), activities.GetStaleLeaguesParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("GetStaleLeagues error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "lg-ok" {
		t.Errorf("expected [lg-ok], got %v", ids)
	}
}

func TestFetchLeagueDetails_SetsScoring(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sleeper.League{
			LeagueID:        "lg1",
			Name:            "My League",
			Status:          "complete",
			TotalRosters:    12,
			ScoringSettings: map[string]float64{"rec": 0.5, "bonus_rec_te": 0.5},
			RosterPositions: []string{"QB", "WR", "SUPER_FLEX", "BN"},
		})
	}))
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := dfa.FetchLeagueDetails(context.Background(), activities.FetchLeagueDetailsParams{LeagueID: "lg1"}); err != nil {
		t.Fatalf("FetchLeagueDetails error: %v", err)
	}

	var l models.SleeperLeague
	db.First(&l, "sleeper_league_id = ?", "lg1")
	if l.PPR == nil || *l.PPR != 0.5 {
		t.Errorf("expected PPR 0.5, got %v", l.PPR)
	}
	if l.TEPremium == nil || *l.TEPremium != 0.5 {
		t.Errorf("expected TEPremium 0.5, got %v", l.TEPremium)
	}
	if l.IsSuperflex == nil || !*l.IsSuperflex {
		t.Error("expected is_superflex = true")
	}
	if l.Name != "My League" {
		t.Errorf("expected name 'My League', got %q", l.Name)
	}
}

func TestFetchLeagueDetails_StandardScoring(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg2"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sleeper.League{
			LeagueID:        "lg2",
			ScoringSettings: map[string]float64{"rec": 0},
			RosterPositions: []string{"QB", "WR", "RB", "TE", "K", "BN"},
		})
	}))
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := dfa.FetchLeagueDetails(context.Background(), activities.FetchLeagueDetailsParams{LeagueID: "lg2"}); err != nil {
		t.Fatalf("FetchLeagueDetails error: %v", err)
	}

	var l models.SleeperLeague
	db.First(&l, "sleeper_league_id = ?", "lg2")
	if l.IsSuperflex == nil || *l.IsSuperflex {
		t.Error("expected is_superflex = false for standard roster")
	}
}

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

func TestMarkLeagueFetched_SetsTimestamp(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1"})

	dfa := &activities.DataFetchActivities{DB: db}
	if err := dfa.MarkLeagueFetched(context.Background(), activities.MarkLeagueFetchedParams{LeagueID: "lg1"}); err != nil {
		t.Fatalf("MarkLeagueFetched error: %v", err)
	}

	var l models.SleeperLeague
	db.First(&l, "sleeper_league_id = ?", "lg1")
	if l.LastFetchedAt == nil {
		t.Error("expected last_fetched_at to be set")
	}
}
