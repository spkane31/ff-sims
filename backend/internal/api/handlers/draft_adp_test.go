package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"backend/internal/database"
	"backend/internal/models"
)

func newDraftADPTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.DraftADP{}, &models.SleeperPlayer{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func withDraftADPTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })
}

func seedADPPlayer(t *testing.T, db *gorm.DB, id, name, position, team string) {
	t.Helper()
	if err := db.Create(&models.SleeperPlayer{
		SleeperPlayerID: id,
		FullName:        name,
		Position:        position,
		NflTeam:         team,
	}).Error; err != nil {
		t.Fatalf("seed player %s: %v", id, err)
	}
}

func seedADPRow(t *testing.T, db *gorm.DB, segment, season, playerID string, avgPick float64, pickCount int) {
	t.Helper()
	if err := db.Create(&models.DraftADP{
		Segment:         segment,
		Season:          season,
		SleeperPlayerID: playerID,
		AvgPickNo:       avgPick,
		PickCount:       pickCount,
		MinPickNo:       int(avgPick),
		MaxPickNo:       int(avgPick),
	}).Error; err != nil {
		t.Fatalf("seed adp row %s/%s/%s: %v", segment, season, playerID, err)
	}
}

func performGetSleeperADP(t *testing.T, query string) (*httptest.ResponseRecorder, SleeperADPResponse) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/sleeper/adp", GetSleeperADP)

	req := httptest.NewRequest(http.MethodGet, "/sleeper/adp"+query, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp SleeperADPResponse
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
	}
	return w, resp
}

func TestGetSleeperADP_DefaultsAndOrdering(t *testing.T) {
	db := newDraftADPTestDB(t)
	withDraftADPTestDB(t, db)

	seedADPPlayer(t, db, "p1", "Player One", "RB", "KC")
	seedADPPlayer(t, db, "p2", "Player Two", "WR", "SF")
	seedADPRow(t, db, "12-ppr-sf", "2024", "p1", 5.0, 25)
	seedADPRow(t, db, "12-ppr-sf", "2024", "p2", 2.0, 30)

	w, resp := performGetSleeperADP(t, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(resp.Players) != 2 {
		t.Fatalf("expected 2 players, got %d", len(resp.Players))
	}
	if resp.Players[0].SleeperPlayerID != "p2" {
		t.Errorf("expected p2 (avg 2.0) ranked first, got %s", resp.Players[0].SleeperPlayerID)
	}
	if resp.Season != "2024" {
		t.Errorf("expected default season 2024, got %q", resp.Season)
	}
}

func TestGetSleeperADP_MinDraftsFiltersLowSampleSize(t *testing.T) {
	db := newDraftADPTestDB(t)
	withDraftADPTestDB(t, db)

	seedADPPlayer(t, db, "p1", "Under Threshold", "RB", "KC")
	seedADPPlayer(t, db, "p2", "Over Threshold", "WR", "SF")
	seedADPRow(t, db, "12-ppr-sf", "2024", "p1", 5.0, 19) // below default min_drafts=20
	seedADPRow(t, db, "12-ppr-sf", "2024", "p2", 2.0, 20)

	_, resp := performGetSleeperADP(t, "")
	if len(resp.Players) != 1 || resp.Players[0].SleeperPlayerID != "p2" {
		t.Errorf("expected only p2 (pick_count >= 20), got %+v", resp.Players)
	}
}

func TestGetSleeperADP_ExplicitFiltersBuildSegmentKey(t *testing.T) {
	db := newDraftADPTestDB(t)
	withDraftADPTestDB(t, db)

	seedADPPlayer(t, db, "p1", "Standard 10 Team", "QB", "BUF")
	seedADPRow(t, db, "10-standard-1qb", "2023", "p1", 3.0, 25)

	_, resp := performGetSleeperADP(t, "?league_size=10&scoring_format=standard&superflex=false&season=2023")
	if len(resp.Players) != 1 || resp.Players[0].SleeperPlayerID != "p1" {
		t.Errorf("expected p1 from 10-standard-1qb/2023, got %+v", resp.Players)
	}
}

func TestGetSleeperADP_SeasonDefaultsToMostRecent(t *testing.T) {
	db := newDraftADPTestDB(t)
	withDraftADPTestDB(t, db)

	seedADPPlayer(t, db, "p-old", "Old Season", "RB", "KC")
	seedADPPlayer(t, db, "p-new", "New Season", "RB", "KC")
	seedADPRow(t, db, "12-ppr-sf", "2023", "p-old", 5.0, 25)
	seedADPRow(t, db, "12-ppr-sf", "2024", "p-new", 5.0, 25)

	_, resp := performGetSleeperADP(t, "")
	if resp.Season != "2024" {
		t.Errorf("expected default season 2024 (most recent), got %q", resp.Season)
	}
	if len(resp.Players) != 1 || resp.Players[0].SleeperPlayerID != "p-new" {
		t.Errorf("expected only 2024's player, got %+v", resp.Players)
	}
	if len(resp.AvailableSeasons) != 2 || resp.AvailableSeasons[0] != "2024" || resp.AvailableSeasons[1] != "2023" {
		t.Errorf("expected available_seasons [2024, 2023], got %v", resp.AvailableSeasons)
	}
}

func TestGetSleeperADP_EmptyTable(t *testing.T) {
	db := newDraftADPTestDB(t)
	withDraftADPTestDB(t, db)

	w, resp := performGetSleeperADP(t, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(resp.Players) != 0 || resp.Total != 0 {
		t.Errorf("expected empty result, got %+v", resp)
	}
}
