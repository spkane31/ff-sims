package handlers

import (
	"encoding/json"
	"testing"
	"time"
)

func TestValueAsOf(t *testing.T) {
	d := func(day int) time.Time { return time.Date(2025, 9, day, 0, 0, 0, 0, time.UTC) }
	snaps := []valuationSnap{
		{ValuationDate: d(8), Value: 1000},
		{ValuationDate: d(15), Value: 1200},
		{ValuationDate: d(22), Value: 900},
	}

	if _, ok := valueAsOf(snaps, d(7)); ok {
		t.Error("expected no value before first snapshot")
	}
	if v, ok := valueAsOf(snaps, time.Date(2025, 9, 18, 14, 30, 0, 0, time.UTC)); !ok || v != 1200 {
		t.Errorf("expected 1200 between snapshots, got %v ok=%v", v, ok)
	}
	if v, ok := valueAsOf(snaps, d(8)); !ok || v != 1000 {
		t.Errorf("expected same-day snapshot 1000, got %v ok=%v", v, ok)
	}
	if v, ok := valueAsOf(snaps, d(30)); !ok || v != 900 {
		t.Errorf("expected latest snapshot 900 after all, got %v ok=%v", v, ok)
	}
	if _, ok := valueAsOf(nil, d(30)); ok {
		t.Error("expected no value for player with no snapshots")
	}
}

func TestApplySideValues(t *testing.T) {
	sides := []TradeSide{
		{RosterID: 1, Players: []TradeSidePlayer{{ID: "p1"}, {ID: "p2"}}},
		{RosterID: 2, Players: []TradeSidePlayer{{ID: "p3"}, {ID: "unvalued"}}},
	}
	values := map[string]float64{"p1": 5000, "p2": 1500, "p3": 7000}

	applySideValues(sides, values)

	if sides[0].TotalValue == nil || *sides[0].TotalValue != 6500 {
		t.Errorf("expected side 1 total 6500, got %v", sides[0].TotalValue)
	}
	if sides[1].TotalValue == nil || *sides[1].TotalValue != 7000 {
		t.Errorf("expected side 2 total 7000 (unvalued player skipped), got %v", sides[1].TotalValue)
	}
	if sides[0].Players[0].Value == nil || *sides[0].Players[0].Value != 5000 {
		t.Errorf("expected p1 value 5000, got %v", sides[0].Players[0].Value)
	}
	if sides[1].Players[1].Value != nil {
		t.Errorf("expected nil value for unvalued player, got %v", *sides[1].Players[1].Value)
	}
}

func TestApplySideValues_NoValuations(t *testing.T) {
	sides := []TradeSide{
		{RosterID: 1, Players: []TradeSidePlayer{{ID: "p1"}}},
	}

	applySideValues(sides, map[string]float64{})

	if sides[0].TotalValue != nil {
		t.Errorf("expected nil total when no players valued, got %v", *sides[0].TotalValue)
	}
}

func TestBuildTradeSides_TwoRosters(t *testing.T) {
	adds := map[string]int{
		"6797": 7,
		"8146": 7,
		"6904": 8,
	}
	players := map[string]TradeSidePlayer{
		"6797": {ID: "6797", Name: "Justin Jefferson", Position: "WR"},
		"8146": {ID: "8146", Name: "Davante Adams", Position: "WR"},
		"6904": {ID: "6904", Name: "Travis Kelce", Position: "TE"},
	}

	sides := buildTradeSides(adds, players, nil)

	if len(sides) != 2 {
		t.Fatalf("expected 2 sides, got %d", len(sides))
	}
	if sides[0].RosterID != 7 {
		t.Errorf("expected first side roster_id=7, got %d", sides[0].RosterID)
	}
	if len(sides[0].Players) != 2 {
		t.Errorf("expected 2 players on side 7, got %d", len(sides[0].Players))
	}
	if sides[1].RosterID != 8 {
		t.Errorf("expected second side roster_id=8, got %d", sides[1].RosterID)
	}
	if len(sides[1].Players) != 1 {
		t.Errorf("expected 1 player on side 8, got %d", len(sides[1].Players))
	}
}

func TestBuildTradeSides_MissingPlayer(t *testing.T) {
	adds := map[string]int{"9999": 3}
	players := map[string]TradeSidePlayer{}

	sides := buildTradeSides(adds, players, nil)

	if len(sides) != 1 {
		t.Fatalf("expected 1 side, got %d", len(sides))
	}
	if sides[0].Players[0].ID != "9999" {
		t.Errorf("expected fallback ID '9999', got %q", sides[0].Players[0].ID)
	}
	if sides[0].Players[0].Name != "9999" {
		t.Errorf("expected fallback Name '9999', got %q", sides[0].Players[0].Name)
	}
}

func TestBuildTradeSides_EmptyAdds(t *testing.T) {
	sides := buildTradeSides(map[string]int{}, map[string]TradeSidePlayer{}, nil)
	if len(sides) != 0 {
		t.Fatalf("expected 0 sides for empty adds, got %d", len(sides))
	}
}

func TestBuildTradeSides_SortedByRosterID(t *testing.T) {
	adds := map[string]int{"p1": 10, "p2": 2}
	players := map[string]TradeSidePlayer{}

	sides := buildTradeSides(adds, players, nil)

	if sides[0].RosterID != 2 || sides[1].RosterID != 10 {
		t.Errorf("expected sides sorted by roster_id asc, got %d, %d", sides[0].RosterID, sides[1].RosterID)
	}
}

func TestBuildTradeSides_PicksOnly(t *testing.T) {
	// Trade where one side sends a player and the other sends only a draft pick.
	adds := map[string]int{"6797": 2} // roster 2 receives a player
	players := map[string]TradeSidePlayer{
		"6797": {ID: "6797", Name: "Justin Jefferson", Position: "WR"},
	}
	rawPicks, _ := json.Marshal([]map[string]interface{}{
		{"season": "2026", "round": 1, "owner_id": 1, "roster_id": 2, "previous_owner_id": 2},
	})

	sides := buildTradeSides(adds, players, rawPicks)

	if len(sides) != 2 {
		t.Fatalf("expected 2 sides, got %d", len(sides))
	}
	// roster 1 receives the pick
	if sides[0].RosterID != 1 {
		t.Errorf("expected first side roster_id=1, got %d", sides[0].RosterID)
	}
	if len(sides[0].Picks) != 1 || sides[0].Picks[0] != "2026 Round 1 pick" {
		t.Errorf("expected pick label '2026 Round 1 pick', got %v", sides[0].Picks)
	}
	if len(sides[0].Players) != 0 {
		t.Errorf("expected no players on side 1, got %d", len(sides[0].Players))
	}
	// roster 2 receives the player
	if sides[1].RosterID != 2 {
		t.Errorf("expected second side roster_id=2, got %d", sides[1].RosterID)
	}
	if len(sides[1].Players) != 1 {
		t.Errorf("expected 1 player on side 2, got %d", len(sides[1].Players))
	}
	if len(sides[1].Picks) != 0 {
		t.Errorf("expected no picks on side 2, got %v", sides[1].Picks)
	}
}
