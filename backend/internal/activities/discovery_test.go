package activities_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/sleeper"
)

func TestSeasons_StartsAt2025AndIncludesCurrentYear(t *testing.T) {
	seasons := activities.Seasons()

	if len(seasons) == 0 {
		t.Fatal("expected at least one season")
	}
	if seasons[0] != "2025" {
		t.Errorf("expected seasons to start at 2025, got %q", seasons[0])
	}
	for _, s := range seasons {
		if s < "2025" {
			t.Errorf("seasons %v should not include a pre-2025 year, found %q", seasons, s)
		}
	}

	currentYear := strconv.Itoa(time.Now().Year())
	found := false
	for _, s := range seasons {
		if s == currentYear {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected seasons %v to include current year %q", seasons, currentYear)
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Each pooled connection to sqlite ":memory:" gets its own empty database;
	// pin the pool to one connection so concurrent test code (e.g. the batch
	// sync activity's goroutines) sees the migrated schema.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("unwrap sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&models.SleeperUser{},
		&models.SleeperLeague{},
		&models.SleeperLeagueUser{},
		&models.SleeperDraft{},
		&models.SleeperDraftPick{},
		&models.SleeperTransaction{},
		&models.SleeperPlayer{},
		&models.SleeperPlayerWeekStat{},
		&models.SleeperWeekStatFetch{},
		&models.DraftADP{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestFetchUserLeagues_UpsertsLeagues(t *testing.T) {
	db := newTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]sleeper.League{
			{LeagueID: "lg1", Name: "Test League", Season: "2024", Sport: "nfl", Status: "complete"},
		})
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{
		DB:      db,
		Sleeper: sleeper.NewWithBaseURL(srv.URL),
	}

	leagueIDs, err := da.FetchUserLeagues(context.Background(), activities.FetchUserLeaguesParams{UserID: "user1"})
	if err != nil {
		t.Fatalf("FetchUserLeagues error: %v", err)
	}
	// one league returned per scanned season, deduped to a single "lg1" row in DB
	if len(leagueIDs) == 0 {
		t.Fatal("expected at least one leagueID")
	}

	var count int64
	db.Model(&models.SleeperLeague{}).Where("sleeper_league_id = ?", "lg1").Count(&count)
	if count != 1 {
		t.Errorf("expected 1 league row (upserted), got %d", count)
	}

	// Junction row should exist
	var jcount int64
	db.Model(&models.SleeperLeagueUser{}).
		Where("sleeper_league_id = ? AND sleeper_user_id = ?", "lg1", "user1").
		Count(&jcount)
	if jcount != 1 {
		t.Errorf("expected 1 junction row, got %d", jcount)
	}
}

func TestFetchLeagueMembers_InsertsUsers(t *testing.T) {
	db := newTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]sleeper.LeagueUser{
			{UserID: "u1", Username: "alice", DisplayName: "Alice"},
			{UserID: "u2", Username: "bob", DisplayName: "Bob"},
		})
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{
		DB:      db,
		Sleeper: sleeper.NewWithBaseURL(srv.URL),
	}

	if err := da.FetchLeagueMembers(context.Background(), activities.FetchLeagueMembersParams{LeagueID: "lg1"}); err != nil {
		t.Fatalf("FetchLeagueMembers error: %v", err)
	}

	var count int64
	db.Model(&models.SleeperUser{}).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 users, got %d", count)
	}

	// New users should have NULL last_fetched_at (picked up by future runs)
	var u models.SleeperUser
	db.First(&u, "sleeper_user_id = ?", "u1")
	if u.LastFetchedAt != nil {
		t.Error("new user should have NULL last_fetched_at")
	}
}

func TestFetchLeagueDetails_Discovery_SetsScoring(t *testing.T) {
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

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := da.FetchLeagueDetails(context.Background(), activities.FetchLeagueDetailsParams{LeagueID: "lg1"}); err != nil {
		t.Fatalf("FetchLeagueDetails error: %v", err)
	}

	var l models.SleeperLeague
	db.First(&l, "sleeper_league_id = ?", "lg1")
	if l.PPR == nil || *l.PPR != 0.5 {
		t.Errorf("expected PPR 0.5, got %v", l.PPR)
	}
	if l.IsSuperflex == nil || !*l.IsSuperflex {
		t.Error("expected is_superflex = true")
	}
	if l.LastFetchedAt == nil {
		t.Error("expected last_fetched_at to be stamped")
	}
}

func TestFetchLeagueDetails_Discovery_NotFound(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "gone"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	err := da.FetchLeagueDetails(context.Background(), activities.FetchLeagueDetailsParams{LeagueID: "gone"})
	if err == nil {
		t.Fatal("expected NOT_FOUND error")
	}
}

func TestFetchLeagueDetails_SkipsCompletedLeague(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	db.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg-done",
		Status:          "complete",
		LastFetchedAt:   &now,
	})

	apiCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		json.NewEncoder(w).Encode(sleeper.League{})
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := da.FetchLeagueDetails(context.Background(), activities.FetchLeagueDetailsParams{LeagueID: "lg-done"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if apiCalled {
		t.Error("Sleeper API should not be called for a completed league")
	}
}
