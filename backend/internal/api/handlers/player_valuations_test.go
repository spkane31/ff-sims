package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/database"
	"backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetPlayerValuationHistory_UsesDefaultSegmentAndChronologicalOrder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Player{}); err != nil {
		t.Fatalf("automigrate player: %v", err)
	}
	if err := db.Exec(`CREATE TABLE player_valuations (
		segment TEXT NOT NULL,
		sleeper_player_id TEXT NOT NULL,
		valuation_date DATE NOT NULL,
		value FLOAT NOT NULL
	)`).Error; err != nil {
		t.Fatalf("create player_valuations: %v", err)
	}
	player := models.Player{SleeperID: "sleeper-1", Name: "Player One"}
	if err := db.Create(&player).Error; err != nil {
		t.Fatalf("create player: %v", err)
	}
	for _, row := range []struct {
		segment, date string
		value         float64
	}{
		{"ppr-sf-10", "2026-08-02", 4200},
		{"ppr-sf-10", "2026-08-01", 4000},
		{"ppr-sf-12", "2026-08-03", 9999},
	} {
		if err := db.Exec(
			"INSERT INTO player_valuations (segment, sleeper_player_id, valuation_date, value) VALUES (?, ?, ?, ?)",
			row.segment, "sleeper-1", row.date, row.value,
		).Error; err != nil {
			t.Fatalf("seed valuation: %v", err)
		}
	}

	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/players/:id/valuation-history", GetPlayerValuationHistory)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/players/1/valuation-history", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var response PlayerValuationHistoryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Segment != defaultPlayerValuationSegment {
		t.Errorf("expected segment %q, got %q", defaultPlayerValuationSegment, response.Segment)
	}
	if len(response.Valuations) != 2 {
		t.Fatalf("expected 2 default-segment valuations, got %d", len(response.Valuations))
	}
	if response.Valuations[0].Date != "2026-08-01" || response.Valuations[0].Value != 4000 {
		t.Errorf("unexpected first valuation: %+v", response.Valuations[0])
	}
	if response.Valuations[1].Date != "2026-08-02" || response.Valuations[1].Value != 4200 {
		t.Errorf("unexpected second valuation: %+v", response.Valuations[1])
	}
}
