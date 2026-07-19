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

	"backend/internal/activities"
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

func TestProcessUser_DiscoversLeaguesAndStamps(t *testing.T) {
	db := newSQLiteDB(t)
	db.Create(&models.SleeperUser{SleeperUserID: "user1"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]sleeper.League{
			{LeagueID: "lg1", Name: "Test League", Season: "2026", Sport: "nfl", Status: "in_season"},
		})
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := discoverycron.ProcessUser(context.Background(), da, "user1"); err != nil {
		t.Fatalf("ProcessUser error: %v", err)
	}

	var u models.SleeperUser
	db.First(&u, "sleeper_user_id = ?", "user1")
	if u.LastFetchedAt == nil {
		t.Error("expected user stamped last_fetched_at")
	}

	var lg models.SleeperLeague
	db.First(&lg, "sleeper_league_id = ?", "lg1")
	if lg.LastFetchedAt != nil {
		t.Error("expected league NOT to have members/details fetched yet — that's league-pool work now")
	}
	if lg.DiscoveryClaimedAt != nil {
		t.Error("newly discovered league should not be claimed yet")
	}
}

func TestProcessUser_NotFoundMarksSkipped(t *testing.T) {
	db := newSQLiteDB(t)
	db.Create(&models.SleeperUser{SleeperUserID: "gone"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := discoverycron.ProcessUser(context.Background(), da, "gone"); err != nil {
		t.Fatalf("ProcessUser error: %v", err)
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

func TestProcessLeague_FetchesMembersAndDetailsAtomically(t *testing.T) {
	db := newSQLiteDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/users") {
			json.NewEncoder(w).Encode([]sleeper.LeagueUser{{UserID: "u1", Username: "alice"}})
			return
		}
		json.NewEncoder(w).Encode(sleeper.League{LeagueID: "lg1", Name: "L", Status: "in_season", TotalRosters: 12})
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := discoverycron.ProcessLeague(context.Background(), da, "lg1"); err != nil {
		t.Fatalf("ProcessLeague error: %v", err)
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
	if lg.DiscoveryClaimedAt != nil {
		t.Error("expected discovery_claimed_at cleared on success")
	}
}

func TestProcessLeague_DetailsFailureRollsBackMembers(t *testing.T) {
	db := newSQLiteDB(t)
	claimedAt := time.Now().UTC()
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1", DiscoveryClaimedAt: &claimedAt})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/users") {
			json.NewEncoder(w).Encode([]sleeper.LeagueUser{{UserID: "u1", Username: "alice"}})
			return
		}
		w.WriteHeader(http.StatusBadRequest) // details call fails
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := discoverycron.ProcessLeague(context.Background(), da, "lg1"); err == nil {
		t.Fatal("expected an error when the details fetch fails")
	}

	var count int64
	db.Model(&models.SleeperUser{}).Where("sleeper_user_id = ?", "u1").Count(&count)
	if count != 0 {
		t.Error("expected the member upsert to roll back when details fetch fails within the same transaction")
	}

	var lg models.SleeperLeague
	db.First(&lg, "sleeper_league_id = ?", "lg1")
	if lg.DiscoveryClaimedAt == nil {
		t.Error("expected discovery_claimed_at to remain set (claim stays in place) after a failed attempt")
	}
}
