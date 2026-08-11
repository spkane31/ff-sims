package valuation_test

import (
	"encoding/json"
	"testing"
	"time"

	"backend/internal/valuation"
)

func TestSegmentKeyForLeague(t *testing.T) {
	ppr, half := 1.0, 0.5
	sf, oneQB := true, false

	cases := []struct {
		name       string
		ppr        *float64
		superflex  *bool
		rosters    int
		leagueType string
		want       string
	}{
		{"ppr superflex 12 redraft", &ppr, &sf, 12, "redraft", "ppr-sf-12"},
		{"ppr superflex 10 redraft", &ppr, &sf, 10, "redraft", "ppr-sf-10"},
		{"ppr superflex 8 redraft", &ppr, &sf, 8, "redraft", "ppr-sf-8"},
		{"unsupported size", &ppr, &sf, 14, "redraft", ""},
		{"half ppr", &half, &sf, 12, "redraft", ""},
		{"one qb", &ppr, &oneQB, 12, "redraft", ""},
		{"dynasty", &ppr, &sf, 12, "dynasty", ""},
		{"nil ppr", nil, &sf, 12, "redraft", ""},
		{"nil superflex", &ppr, nil, 12, "redraft", ""},
	}
	for _, c := range cases {
		if got := valuation.SegmentKeyForLeague(c.ppr, c.superflex, c.rosters, c.leagueType); got != c.want {
			t.Errorf("%s: expected %q, got %q", c.name, c.want, got)
		}
	}
}

func TestValueAsOf(t *testing.T) {
	d := func(day int) time.Time { return time.Date(2025, 9, day, 0, 0, 0, 0, time.UTC) }
	snaps := []valuation.Snapshot{
		{ValuationDate: d(8), Value: 1000},
		{ValuationDate: d(15), Value: 1200},
		{ValuationDate: d(22), Value: 900},
	}

	if _, ok := valuation.ValueAsOf(snaps, d(7), valuation.FreshnessWindow); ok {
		t.Error("expected no value before first snapshot")
	}
	if v, ok := valuation.ValueAsOf(snaps, d(15).Add(14*time.Hour), valuation.FreshnessWindow); !ok || v != 1200 {
		t.Errorf("expected 1200 between snapshots (within freshness window), got %v ok=%v", v, ok)
	}
	if v, ok := valuation.ValueAsOf(snaps, d(8), valuation.FreshnessWindow); !ok || v != 1000 {
		t.Errorf("expected same-day snapshot 1000, got %v ok=%v", v, ok)
	}
	// d(30) is 8 days after the latest snapshot d(22) — beyond the 24h
	// freshness window, so it must be treated as absent even though it's
	// the latest-known value chronologically.
	if _, ok := valuation.ValueAsOf(snaps, d(30), valuation.FreshnessWindow); ok {
		t.Error("expected no value once the latest snapshot is beyond the freshness window")
	}
	// Same snapshot set, just after it was published (a few hours later,
	// same UTC day) — within the freshness window.
	if v, ok := valuation.ValueAsOf(snaps, d(22).Add(6*time.Hour), valuation.FreshnessWindow); !ok || v != 900 {
		t.Errorf("expected 900 within the freshness window of the latest snapshot, got %v ok=%v", v, ok)
	}
	if _, ok := valuation.ValueAsOf(nil, d(30), valuation.FreshnessWindow); ok {
		t.Error("expected no value for player with no snapshots")
	}
}

func TestGroupPlayersByRoster(t *testing.T) {
	adds := map[string]int{"p1": 7, "p2": 7, "p3": 8}
	got := valuation.GroupPlayersByRoster(adds)
	if len(got[7]) != 2 || len(got[8]) != 1 {
		t.Fatalf("expected roster 7 with 2 players and roster 8 with 1, got %+v", got)
	}
	if len(got) != 2 {
		t.Errorf("expected exactly 2 rosters, got %d", len(got))
	}
}

func TestComputeTradeValues_FullCoverage(t *testing.T) {
	now := time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)
	rosterPlayers := map[int][]string{
		7: {"p1", "p2"},
		8: {"p3"},
	}
	history := map[string][]valuation.Snapshot{
		"p1": {{ValuationDate: now.Add(-6 * time.Hour), Value: 5000}},
		"p2": {{ValuationDate: now.Add(-6 * time.Hour), Value: 1500}},
		"p3": {{ValuationDate: now.Add(-6 * time.Hour), Value: 7000}},
	}

	raw, complete := valuation.ComputeTradeValues(rosterPlayers, now, history)
	if !complete {
		t.Fatal("expected complete=true — every roster resolved")
	}
	var totals map[string]float64
	if err := json.Unmarshal(raw, &totals); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if totals["7"] != 6500 {
		t.Errorf("expected roster 7 total 6500, got %v", totals["7"])
	}
	if totals["8"] != 7000 {
		t.Errorf("expected roster 8 total 7000, got %v", totals["8"])
	}
}

// TestComputeTradeValues_PartialSideStaysAbsent is the regression test for
// the bug where a trade with one resolved side and one still-pending side
// was marked complete (because ComputeTradeValues's second return value used
// to mean "got anything at all", not "everything resolved"), which made
// ReconcileTradeValues's `trade_values IS NULL` gate skip it forever —
// pending side 7 could never be filled in once its player's valuation
// arrived. complete must be false here even though values is non-nil.
func TestComputeTradeValues_PartialSideStaysAbsent(t *testing.T) {
	now := time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)
	rosterPlayers := map[int][]string{
		7: {"p1", "unvalued"},
		8: {"p3"},
	}
	history := map[string][]valuation.Snapshot{
		"p1": {{ValuationDate: now.Add(-6 * time.Hour), Value: 5000}},
		"p3": {{ValuationDate: now.Add(-6 * time.Hour), Value: 7000}},
	}

	raw, complete := valuation.ComputeTradeValues(rosterPlayers, now, history)
	if complete {
		t.Fatal("expected complete=false — roster 7 still has an unvalued player")
	}
	if raw == nil {
		t.Fatal("expected a non-nil result — roster 8 is fully valued")
	}
	var totals map[string]float64
	if err := json.Unmarshal(raw, &totals); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := totals["7"]; present {
		t.Errorf("expected roster 7 absent (one unvalued player), got %v", totals["7"])
	}
	if totals["8"] != 7000 {
		t.Errorf("expected roster 8 total 7000, got %v", totals["8"])
	}
}

func TestComputeTradeValues_StaleSnapshotTreatedAsAbsent(t *testing.T) {
	now := time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)
	rosterPlayers := map[int][]string{7: {"p1"}}
	history := map[string][]valuation.Snapshot{
		"p1": {{ValuationDate: now.Add(-30 * time.Hour), Value: 5000}},
	}

	raw, complete := valuation.ComputeTradeValues(rosterPlayers, now, history)
	if raw != nil {
		t.Errorf("expected nil result — the only snapshot is 30h stale, beyond FreshnessWindow, got %s", raw)
	}
	if complete {
		t.Error("expected complete=false — roster 7 never resolved")
	}
}

func TestComputeTradeValues_PicksOnlySideStaysAbsent(t *testing.T) {
	now := time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)
	// Roster 9 traded only a pick (not represented in rosterPlayers at all,
	// since GroupPlayersByRoster only sees `adds`), roster 7 traded a valued
	// player.
	rosterPlayers := map[int][]string{7: {"p1"}}
	history := map[string][]valuation.Snapshot{
		"p1": {{ValuationDate: now.Add(-6 * time.Hour), Value: 5000}},
	}

	raw, complete := valuation.ComputeTradeValues(rosterPlayers, now, history)
	if !complete {
		t.Fatal("expected complete=true — roster 7 is the only non-empty roster and it resolved")
	}
	var totals map[string]float64
	json.Unmarshal(raw, &totals)
	if len(totals) != 1 || totals["7"] != 5000 {
		t.Errorf("expected only roster 7 valued at 5000, got %+v", totals)
	}
}

func TestComputeTradeValues_NoneValuedReturnsFalse(t *testing.T) {
	now := time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)
	rosterPlayers := map[int][]string{7: {"unvalued"}}
	raw, complete := valuation.ComputeTradeValues(rosterPlayers, now, map[string][]valuation.Snapshot{})
	if raw != nil {
		t.Errorf("expected nil result when nothing is valued, got %s", raw)
	}
	if complete {
		t.Error("expected complete=false — roster 7 never resolved")
	}
}

// TestComputeTradeValues_EmptyRosterPlayersIsVacuouslyComplete covers a
// trade whose adds map is empty (e.g. a picks-only trade — GroupPlayersByRoster
// never produces a roster entry for a side that only received picks). There
// is nothing to resolve, so it must be reported complete immediately rather
// than being retried by ReconcileTradeValues forever.
func TestComputeTradeValues_EmptyRosterPlayersIsVacuouslyComplete(t *testing.T) {
	now := time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)
	raw, complete := valuation.ComputeTradeValues(map[int][]string{}, now, map[string][]valuation.Snapshot{})
	if raw != nil {
		t.Errorf("expected nil result for an empty trade, got %s", raw)
	}
	if !complete {
		t.Error("expected complete=true — there is nothing left to resolve")
	}
}
