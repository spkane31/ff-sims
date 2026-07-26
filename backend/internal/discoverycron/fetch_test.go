package discoverycron_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/discoverycron"
	"backend/internal/models"
	"backend/internal/sleeper"
)

func newSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("unwrap sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.SleeperUser{}, &models.SleeperLeague{}, &models.SleeperLeagueUser{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestFetchUserLeagues_ReturnsLeaguesAndMemberships(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]sleeper.League{
			{LeagueID: "lg1", Name: "Test League", Season: "2026", Sport: "nfl", Status: "in_season"},
		})
	}))
	defer srv.Close()

	res, err := discoverycron.FetchUserLeagues(context.Background(), sleeper.NewWithBaseURL(srv.URL), "user1")
	if err != nil {
		t.Fatalf("FetchUserLeagues error: %v", err)
	}
	if res.NotFound {
		t.Fatal("expected NotFound false")
	}
	if len(res.Leagues) == 0 || res.Leagues[0].SleeperLeagueID != "lg1" {
		t.Fatalf("expected lg1 in Leagues, got %+v", res.Leagues)
	}
	if len(res.Memberships) == 0 || res.Memberships[0].SleeperUserID != "user1" {
		t.Fatalf("expected user1 membership, got %+v", res.Memberships)
	}
}

func TestFetchUserLeagues_NotFoundReturnsSuccessWithFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	res, err := discoverycron.FetchUserLeagues(context.Background(), sleeper.NewWithBaseURL(srv.URL), "gone")
	if err != nil {
		t.Fatalf("expected a Sleeper 404 to be a successful, batchable outcome, got error: %v", err)
	}
	if !res.NotFound {
		t.Error("expected NotFound true")
	}
	if len(res.Leagues) != 0 || len(res.Memberships) != 0 {
		t.Errorf("expected no leagues/memberships for a not-found user, got %+v", res)
	}
}

func TestFlushUserDiscovery_WritesLeaguesAndStampsFetched(t *testing.T) {
	db := newSQLiteDB(t)
	db.Create(&models.SleeperUser{SleeperUserID: "user1"})

	batch := []discoverycron.UserDiscoveryResult{
		{
			UserID:      "user1",
			Leagues:     []models.SleeperLeague{{SleeperLeagueID: "lg1", Name: "Test League", Season: "2026"}},
			Memberships: []models.SleeperLeagueUser{{SleeperLeagueID: "lg1", SleeperUserID: "user1"}},
		},
	}
	if err := discoverycron.FlushUserDiscovery(context.Background(), db, batch); err != nil {
		t.Fatalf("FlushUserDiscovery error: %v", err)
	}

	var u models.SleeperUser
	db.First(&u, "sleeper_user_id = ?", "user1")
	if u.LastFetchedAt == nil || u.ClaimedAt != nil {
		t.Errorf("expected user1 stamped/unclaimed, got %+v", u)
	}

	var lg models.SleeperLeague
	if err := db.First(&lg, "sleeper_league_id = ?", "lg1").Error; err != nil {
		t.Fatalf("expected lg1 upserted: %v", err)
	}
	if lg.LastFetchedAt != nil {
		t.Error("expected league NOT to have members/details fetched yet — that's league-pool work")
	}
	if lg.DiscoveryClaimedAt != nil {
		t.Error("newly discovered league should not be claimed yet")
	}

	var jcount int64
	db.Model(&models.SleeperLeagueUser{}).Where("sleeper_league_id = ? AND sleeper_user_id = ?", "lg1", "user1").Count(&jcount)
	if jcount != 1 {
		t.Errorf("expected 1 junction row, got %d", jcount)
	}
}

func TestFlushUserDiscovery_NotFoundMarksSkipped(t *testing.T) {
	db := newSQLiteDB(t)
	db.Create(&models.SleeperUser{SleeperUserID: "gone"})

	batch := []discoverycron.UserDiscoveryResult{{UserID: "gone", NotFound: true}}
	if err := discoverycron.FlushUserDiscovery(context.Background(), db, batch); err != nil {
		t.Fatalf("FlushUserDiscovery error: %v", err)
	}

	var u models.SleeperUser
	db.First(&u, "sleeper_user_id = ?", "gone")
	if u.SkippedAt == nil {
		t.Error("expected user marked skipped")
	}
	if u.LastFetchedAt != nil {
		t.Error("skipped user must not be stamped fetched")
	}
}

func TestFetchLeague_ReturnsMembersAndDetails(t *testing.T) {
	db := newSQLiteDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/users") {
			json.NewEncoder(w).Encode([]sleeper.LeagueUser{{UserID: "u1", Username: "alice"}})
			return
		}
		json.NewEncoder(w).Encode(sleeper.League{
			LeagueID: "lg1", Name: "L", Status: "in_season", TotalRosters: 12,
			ScoringSettings: map[string]float64{"rec": 0.5, "bonus_rec_te": 0.5},
			RosterPositions: []string{"QB", "WR", "SUPER_FLEX", "BN"},
		})
	}))
	defer srv.Close()

	res, err := discoverycron.FetchLeague(context.Background(), db, sleeper.NewWithBaseURL(srv.URL), "lg1")
	if err != nil {
		t.Fatalf("FetchLeague error: %v", err)
	}
	if len(res.Members) != 1 || res.Members[0].SleeperUserID != "u1" {
		t.Fatalf("expected member u1, got %+v", res.Members)
	}
	if res.Details == nil {
		t.Fatal("expected Details to be populated for a not-yet-synced league")
	}
	if res.Details.PPR != 0.5 {
		t.Errorf("expected PPR 0.5, got %v", res.Details.PPR)
	}
	if !res.Details.IsSuperflex {
		t.Error("expected is_superflex true")
	}
}

func TestFetchLeague_SkipsDetailsWhenFullySynced(t *testing.T) {
	db := newSQLiteDB(t)
	now := time.Now().UTC()
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-done", Status: "complete", LastFetchedAt: &now})

	apiCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/users") {
			json.NewEncoder(w).Encode([]sleeper.LeagueUser{})
			return
		}
		apiCalled = true
		json.NewEncoder(w).Encode(sleeper.League{})
	}))
	defer srv.Close()

	res, err := discoverycron.FetchLeague(context.Background(), db, sleeper.NewWithBaseURL(srv.URL), "lg-done")
	if err != nil {
		t.Fatalf("FetchLeague error: %v", err)
	}
	if res.Details != nil {
		t.Error("expected no Details for an already fully-synced league")
	}
	if apiCalled {
		t.Error("Sleeper league-details API should not be called for a completed league")
	}
}

func TestFetchLeague_PropagatesDetailsFetchError(t *testing.T) {
	db := newSQLiteDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/users") {
			json.NewEncoder(w).Encode([]sleeper.LeagueUser{{UserID: "u1", Username: "alice"}})
			return
		}
		w.WriteHeader(http.StatusBadRequest) // details call fails
	}))
	defer srv.Close()

	if _, err := discoverycron.FetchLeague(context.Background(), db, sleeper.NewWithBaseURL(srv.URL), "lg1"); err == nil {
		t.Fatal("expected an error when the details fetch fails")
	}
}

func TestFlushLeagueDiscovery_WritesMembersDetailsAndClearsClaim(t *testing.T) {
	db := newSQLiteDB(t)
	claimedAt := time.Now().UTC()
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1", DiscoveryClaimedAt: &claimedAt})

	batch := []discoverycron.LeagueDiscoveryResult{
		{
			LeagueID: "lg1",
			Members:  []models.SleeperUser{{SleeperUserID: "u1", Username: "alice"}},
			Details:  &discoverycron.LeagueDetailsUpdate{Name: "L", Status: "in_season", TotalRosters: 12, PPR: 0.5, LeagueType: "redraft"},
		},
	}
	if err := discoverycron.FlushLeagueDiscovery(context.Background(), db, batch); err != nil {
		t.Fatalf("FlushLeagueDiscovery error: %v", err)
	}

	var u models.SleeperUser
	if err := db.First(&u, "sleeper_user_id = ?", "u1").Error; err != nil {
		t.Fatalf("expected member u1 to be upserted: %v", err)
	}

	var lg models.SleeperLeague
	db.First(&lg, "sleeper_league_id = ?", "lg1")
	if lg.LastFetchedAt == nil {
		t.Error("expected league details stamped")
	}
	if lg.PPR == nil || *lg.PPR != 0.5 {
		t.Errorf("expected PPR 0.5, got %v", lg.PPR)
	}
	if lg.DiscoveryClaimedAt != nil {
		t.Error("expected discovery_claimed_at cleared on success")
	}
}

func TestFlushLeagueDiscovery_ClearsClaimEvenWhenDetailsNil(t *testing.T) {
	db := newSQLiteDB(t)
	claimedAt := time.Now().UTC()
	fetchedAt := time.Now().UTC()
	db.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg-done", Status: "complete", LastFetchedAt: &fetchedAt, DiscoveryClaimedAt: &claimedAt,
	})

	batch := []discoverycron.LeagueDiscoveryResult{
		{LeagueID: "lg-done", Members: []models.SleeperUser{{SleeperUserID: "u1", Username: "alice"}}}, // Details nil: already fully synced
	}
	if err := discoverycron.FlushLeagueDiscovery(context.Background(), db, batch); err != nil {
		t.Fatalf("FlushLeagueDiscovery error: %v", err)
	}

	var lg models.SleeperLeague
	db.First(&lg, "sleeper_league_id = ?", "lg-done")
	if lg.DiscoveryClaimedAt != nil {
		t.Error("expected discovery_claimed_at cleared even though Details was nil")
	}
	if lg.Status != "complete" {
		t.Errorf("expected status untouched ('complete'), got %q", lg.Status)
	}
}

// TestFetchLeague_DetailsFailureMeansNoResult guards the same property the
// old ProcessLeague/db.Transaction combination used to provide (members
// never persist when the details fetch fails), but the guarantee is now
// structural rather than transactional: if FetchLeague returns an error,
// nothing about this league — members or details — is ever written by any
// flush call, because it never enters a batch at all.
func TestFetchLeague_DetailsFailureMeansNoResult(t *testing.T) {
	db := newSQLiteDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/users") {
			json.NewEncoder(w).Encode([]sleeper.LeagueUser{{UserID: "u1", Username: "alice"}})
			return
		}
		w.WriteHeader(http.StatusBadRequest) // details call fails
	}))
	defer srv.Close()

	_, err := discoverycron.FetchLeague(context.Background(), db, sleeper.NewWithBaseURL(srv.URL), "lg1")
	if err == nil {
		t.Fatal("expected an error when the details fetch fails")
	}

	var count int64
	db.Model(&models.SleeperUser{}).Where("sleeper_user_id = ?", "u1").Count(&count)
	if count != 0 {
		t.Error("expected no member row written — FetchLeague's error means this league never enters a batch")
	}
}
