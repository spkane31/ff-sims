package models_test

import (
	"testing"

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
