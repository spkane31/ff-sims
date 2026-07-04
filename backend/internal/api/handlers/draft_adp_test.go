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
	seedADPRow(t, db, "12-ppr-sf", "2025", "p1", 5.0, 25)
	seedADPRow(t, db, "12-ppr-sf", "2025", "p2", 2.0, 30)

	w, resp := performGetSleeperADP(t, "?season=2025")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(resp.Players) != 2 {
		t.Fatalf("expected 2 players, got %d", len(resp.Players))
	}
	if resp.Players[0].SleeperPlayerID != "p2" {
		t.Errorf("expected p2 (avg 2.0) ranked first, got %s", resp.Players[0].SleeperPlayerID)
	}
	if resp.Season != "2025" {
		t.Errorf("expected season 2025, got %q", resp.Season)
	}
}

func TestGetSleeperADP_MinDraftsFiltersLowSampleSize(t *testing.T) {
	db := newDraftADPTestDB(t)
	withDraftADPTestDB(t, db)

	seedADPPlayer(t, db, "p1", "Under Threshold", "RB", "KC")
	seedADPPlayer(t, db, "p2", "Over Threshold", "WR", "SF")
	seedADPRow(t, db, "12-ppr-sf", "2025", "p1", 5.0, 19) // below default min_drafts=20
	seedADPRow(t, db, "12-ppr-sf", "2025", "p2", 2.0, 20)

	_, resp := performGetSleeperADP(t, "?season=2025")
	if len(resp.Players) != 1 || resp.Players[0].SleeperPlayerID != "p2" {
		t.Errorf("expected only p2 (pick_count >= 20), got %+v", resp.Players)
	}
}

func TestGetSleeperADP_ExplicitFiltersBuildSegmentKey(t *testing.T) {
	db := newDraftADPTestDB(t)
	withDraftADPTestDB(t, db)

	seedADPPlayer(t, db, "p1", "Standard 10 Team", "QB", "BUF")
	seedADPRow(t, db, "10-standard-1qb", "2025", "p1", 3.0, 25)

	_, resp := performGetSleeperADP(t, "?league_size=10&scoring_format=standard&superflex=false&season=2025")
	if len(resp.Players) != 1 || resp.Players[0].SleeperPlayerID != "p1" {
		t.Errorf("expected p1 from 10-standard-1qb/2025, got %+v", resp.Players)
	}
}

// TestGetSleeperADP_SeasonListIsHardcoded verifies the available season list
// and default season come from the hardcoded adpSeasons() list rather than
// from whichever seasons happen to have draft_adp rows for the resolved
// segment — a per-segment DB-driven list meant the "available" seasons (and
// the default picked from them) changed depending on which segment was
// selected, since thin segments could be missing a season entirely.
func TestGetSleeperADP_SeasonListIsHardcoded(t *testing.T) {
	db := newDraftADPTestDB(t)
	withDraftADPTestDB(t, db)

	// Seed only a season far outside the hardcoded range; it must not leak
	// into available_seasons or become reachable as a default.
	seedADPPlayer(t, db, "p-ancient", "Ancient Season", "RB", "KC")
	seedADPRow(t, db, "12-ppr-sf", "2019", "p-ancient", 5.0, 25)

	_, resp := performGetSleeperADP(t, "")
	want := adpSeasons()
	if len(resp.AvailableSeasons) != len(want) {
		t.Fatalf("expected available_seasons %v, got %v", want, resp.AvailableSeasons)
	}
	for i, s := range want {
		if resp.AvailableSeasons[i] != s {
			t.Errorf("expected available_seasons %v, got %v", want, resp.AvailableSeasons)
			break
		}
	}
	if resp.Season != want[0] {
		t.Errorf("expected default season %q (most recent hardcoded), got %q", want[0], resp.Season)
	}
	if len(resp.Players) != 0 {
		t.Errorf("expected no players (2019 data shouldn't be reachable), got %+v", resp.Players)
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
