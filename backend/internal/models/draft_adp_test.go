package models_test

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"backend/internal/models"
)

func TestADPSegmentKey(t *testing.T) {
	cases := []struct {
		leagueSize    string
		scoringFormat string
		superflex     bool
		want          string
	}{
		{"12", "ppr", true, "12-ppr-sf"},
		{"10", "half_ppr", false, "10-half_ppr-1qb"},
		{"14+", "standard", true, "14+-standard-sf"},
	}
	for _, c := range cases {
		if got := models.ADPSegmentKey(c.leagueSize, c.scoringFormat, c.superflex); got != c.want {
			t.Errorf("ADPSegmentKey(%q, %q, %v) = %q, want %q", c.leagueSize, c.scoringFormat, c.superflex, got, c.want)
		}
	}
}

func TestDraftADP_CIFieldsRoundTrip(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.DraftADP{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	row := models.DraftADP{
		Segment:         "12-ppr-sf",
		Season:          "2025",
		SleeperPlayerID: "p1",
		AvgPickNo:       10.0,
		PickCount:       25,
		MinPickNo:       1,
		MaxPickNo:       30,
		CILowPickNo:     2.5,
		CIHighPickNo:    22.5,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got models.DraftADP
	if err := db.First(&got, "sleeper_player_id = ?", "p1").Error; err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got.CILowPickNo != 2.5 || got.CIHighPickNo != 22.5 {
		t.Errorf("expected ci_low=2.5 ci_high=22.5, got ci_low=%v ci_high=%v", got.CILowPickNo, got.CIHighPickNo)
	}
}

func TestAllADPSegments_Has24UniqueKeys(t *testing.T) {
	segments := models.AllADPSegments()
	if len(segments) != 24 {
		t.Fatalf("expected 24 segments, got %d", len(segments))
	}
	seen := make(map[string]bool, 24)
	for _, s := range segments {
		key := s.Key()
		if seen[key] {
			t.Errorf("duplicate segment key %q", key)
		}
		seen[key] = true
	}
}
