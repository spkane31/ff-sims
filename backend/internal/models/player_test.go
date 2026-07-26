package models_test

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"backend/internal/models"
)

func newPlayerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Player{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestGetPlayersByESPNIDs_PartialMatchAndZeroSentinelIgnored(t *testing.T) {
	db := newPlayerTestDB(t)
	db.Create(&models.Player{ESPNID: 111, Name: "A"})
	db.Create(&models.Player{ESPNID: 222, Name: "B"})
	db.Create(&models.Player{ESPNID: 0, Name: "Unset Sentinel"})

	got, err := models.GetPlayersByESPNIDs(db, []int64{111, 999, 0})
	if err != nil {
		t.Fatalf("GetPlayersByESPNIDs error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 match, got %d: %+v", len(got), got)
	}
	if got[111].Name != "A" {
		t.Errorf("expected espn_id 111 to resolve to player A, got %+v", got[111])
	}
	if _, ok := got[0]; ok {
		t.Error("espn_id=0 (the unset sentinel) must never be treated as a real match")
	}
}

func TestGetPlayersByESPNIDs_EmptyInput(t *testing.T) {
	db := newPlayerTestDB(t)
	got, err := models.GetPlayersByESPNIDs(db, nil)
	if err != nil {
		t.Fatalf("GetPlayersByESPNIDs error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no matches for empty input, got %+v", got)
	}
}

func TestGetPlayersBySleeperIDs_PartialMatchAndEmptySentinelIgnored(t *testing.T) {
	db := newPlayerTestDB(t)
	db.Create(&models.Player{SleeperID: "sp1", Name: "A"})
	db.Create(&models.Player{SleeperID: "sp2", Name: "B"})
	db.Create(&models.Player{SleeperID: "", Name: "Unset Sentinel"})

	got, err := models.GetPlayersBySleeperIDs(db, []string{"sp1", "sp-unknown", ""})
	if err != nil {
		t.Fatalf("GetPlayersBySleeperIDs error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 match, got %d: %+v", len(got), got)
	}
	if got["sp1"].Name != "A" {
		t.Errorf("expected sleeper_id sp1 to resolve to player A, got %+v", got["sp1"])
	}
	if _, ok := got[""]; ok {
		t.Error("sleeper_id=\"\" (the unset sentinel) must never be treated as a real match")
	}
}

func TestGetPlayersBySleeperIDs_EmptyInput(t *testing.T) {
	db := newPlayerTestDB(t)
	got, err := models.GetPlayersBySleeperIDs(db, nil)
	if err != nil {
		t.Fatalf("GetPlayersBySleeperIDs error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no matches for empty input, got %+v", got)
	}
}
