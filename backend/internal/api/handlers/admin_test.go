package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"backend/internal/database"
	"backend/internal/models"
)

func newAdminTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.SleeperLeague{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func withAdminTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })
}

func performGetAdminBacklog(t *testing.T) AdminBacklogResponse {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin/backlog", GetAdminBacklog)

	req := httptest.NewRequest(http.MethodGet, "/admin/backlog", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp AdminBacklogResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return resp
}

func TestGetAdminBacklog_MixedFetchState(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	now := time.Now().UTC().Truncate(time.Second)
	older := now.Add(-48 * time.Hour)

	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-never", Season: "2026"})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-recent", Season: "2026", LastTransactionsFetchedAt: &now})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-old", Season: "2026", LastTransactionsFetchedAt: &older})
	// different (older) season — must not be counted in the 2026 totals
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-2025", Season: "2025", LastTransactionsFetchedAt: &now})

	resp := performGetAdminBacklog(t)

	if resp.Season != "2026" {
		t.Errorf("expected season 2026, got %q", resp.Season)
	}
	if resp.TotalLeagues != 3 {
		t.Errorf("expected 3 leagues in 2026, got %d", resp.TotalLeagues)
	}
	if resp.NeverFetchedCount != 1 {
		t.Errorf("expected 1 never-fetched, got %d", resp.NeverFetchedCount)
	}
	if resp.OldestTransactionsFetchedAt == nil {
		t.Fatal("expected non-nil oldest fetch timestamp")
	}
	if !resp.OldestTransactionsFetchedAt.Equal(older) {
		t.Errorf("expected oldest fetch %v, got %v", older, *resp.OldestTransactionsFetchedAt)
	}
}

func TestGetAdminBacklog_AllNeverFetched(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-a", Season: "2026"})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-b", Season: "2026"})

	resp := performGetAdminBacklog(t)

	if resp.TotalLeagues != 2 || resp.NeverFetchedCount != 2 {
		t.Errorf("expected 2/2 never fetched, got total=%d never=%d", resp.TotalLeagues, resp.NeverFetchedCount)
	}
	if resp.OldestTransactionsFetchedAt != nil {
		t.Errorf("expected nil oldest fetch timestamp, got %v", *resp.OldestTransactionsFetchedAt)
	}
}

func TestGetAdminBacklog_ExcludesSkipped(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	skippedAt := time.Now().UTC()
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-skipped", Season: "2026", SkippedAt: &skippedAt})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-active", Season: "2026"})

	resp := performGetAdminBacklog(t)

	if resp.TotalLeagues != 1 {
		t.Errorf("expected 1 non-skipped league, got %d", resp.TotalLeagues)
	}
	if resp.NeverFetchedCount != 1 {
		t.Errorf("expected 1 never-fetched (excluding skipped), got %d", resp.NeverFetchedCount)
	}
}

func TestGetAdminBacklog_EmptyTable(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	resp := performGetAdminBacklog(t)

	if resp.Season != "" {
		t.Errorf("expected empty season, got %q", resp.Season)
	}
	if resp.TotalLeagues != 0 || resp.NeverFetchedCount != 0 {
		t.Errorf("expected 0/0, got total=%d never=%d", resp.TotalLeagues, resp.NeverFetchedCount)
	}
	if resp.OldestTransactionsFetchedAt != nil {
		t.Error("expected nil oldest fetch timestamp for empty table")
	}
}
