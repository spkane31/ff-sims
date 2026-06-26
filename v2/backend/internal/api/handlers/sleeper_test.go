package handlers

import (
	"encoding/json"
	"testing"
)

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
