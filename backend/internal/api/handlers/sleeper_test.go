package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"backend/internal/models"
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
		if got := segmentKeyForLeague(c.ppr, c.superflex, c.rosters, c.leagueType); got != c.want {
			t.Errorf("%s: expected %q, got %q", c.name, c.want, got)
		}
	}
}

func TestFormatScoring(t *testing.T) {
	ppr, half, std, odd := 1.0, 0.5, 0.0, 0.75
	cases := []struct {
		name string
		ppr  *float64
		want string
	}{
		{"ppr", &ppr, "PPR"},
		{"half ppr", &half, "0.5 PPR"},
		{"standard", &std, "Standard"},
		{"odd value", &odd, "Other"},
		{"nil", nil, "Other"},
	}
	for _, c := range cases {
		if got := formatScoring(c.ppr); got != c.want {
			t.Errorf("%s: expected %q, got %q", c.name, c.want, got)
		}
	}
}

func TestFormatLeagueSize(t *testing.T) {
	cases := []struct {
		rosters int
		want    string
	}{
		{8, "8"},
		{10, "10"},
		{12, "12"},
		{14, "14+"},
		{16, "14+"},
		{9, "Other"},
	}
	for _, c := range cases {
		if got := formatLeagueSize(c.rosters); got != c.want {
			t.Errorf("rosters=%d: expected %q, got %q", c.rosters, c.want, got)
		}
	}
}

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

func performGetSleeperStats(t *testing.T, query string) SleeperStatsResponse {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/sleeper/stats", GetSleeperStats)

	req := httptest.NewRequest(http.MethodGet, "/sleeper/stats"+query, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SleeperStatsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return resp
}

// TestGetSleeperStats_ReadsLifetimeCountsNotLiveTables seeds sleeper_leagues/
// sleeper_transactions/sleeper_drafts with counts that disagree with
// sleeper_lifetime_counts — standing in for the purge having trimmed the
// live tables to a hot window narrower than all-time history — and asserts
// the handler reports the lifetime-counts values, not a live COUNT(*).
func TestGetSleeperStats_ReadsLifetimeCountsNotLiveTables(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	now := time.Now().UTC()
	// Only one hot-window row survives in each live table...
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1", Season: "2026", LastFetchedAt: &now})
	db.Create(&models.SleeperTransaction{SleeperTransactionID: "t1", Type: "trade", Status: "complete"})
	db.Create(&models.SleeperDraft{SleeperDraftID: "d1", Status: "complete"})

	// ...but the hourly-snapshotted lifetime table remembers the true, larger, all-time totals.
	trades, drafts := int64(100), int64(55)
	db.Create(&models.SleeperLifetimeCount{
		SnapshotAt: now.Truncate(time.Hour), LeaguesExpanded: 42,
		TradesCompleted: &trades, DraftsCompleted: &drafts,
	})

	resp := performGetSleeperStats(t, "")

	if len(resp.Snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(resp.Snapshots))
	}
	got := resp.Snapshots[0]
	if got.LeaguesExpanded != 42 || got.TradeCount != 100 || got.DraftCount != 55 {
		t.Errorf("snapshot = %+v, want {LeaguesExpanded: 42, TradeCount: 100, DraftCount: 55}", got)
	}
}

// TestGetSleeperStats_OrdersMostRecentFirst seeds two snapshot hours and
// asserts the series comes back newest-first, matching what a "just the
// latest" caller (limit=1) expects from index 0.
func TestGetSleeperStats_OrdersMostRecentFirst(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	older := time.Now().UTC().Truncate(time.Hour).Add(-2 * time.Hour)
	latest := time.Now().UTC().Truncate(time.Hour)
	tenTrades, tenDrafts := int64(10), int64(10)
	hundredTrades, fiftyFiveDrafts := int64(100), int64(55)

	db.Create(&models.SleeperLifetimeCount{
		SnapshotAt: older, LeaguesExpanded: 10, TradesCompleted: &tenTrades, DraftsCompleted: &tenDrafts,
	})
	db.Create(&models.SleeperLifetimeCount{
		SnapshotAt: latest, LeaguesExpanded: 42, TradesCompleted: &hundredTrades, DraftsCompleted: &fiftyFiveDrafts,
	})

	resp := performGetSleeperStats(t, "")

	if len(resp.Snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(resp.Snapshots))
	}
	if resp.Snapshots[0].LeaguesExpanded != 42 || resp.Snapshots[1].LeaguesExpanded != 10 {
		t.Errorf("expected [latest, older] = [42, 10], got [%d, %d]", resp.Snapshots[0].LeaguesExpanded, resp.Snapshots[1].LeaguesExpanded)
	}
}

// TestGetSleeperStats_LimitParam covers the home page's expected use (limit=1
// for just the latest point) and a smaller-than-available limit generally.
func TestGetSleeperStats_LimitParam(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	now := time.Now().UTC().Truncate(time.Hour)
	for i := 0; i < 3; i++ {
		db.Create(&models.SleeperLifetimeCount{SnapshotAt: now.Add(-time.Duration(i) * time.Hour), LeaguesExpanded: int64(i)})
	}

	resp := performGetSleeperStats(t, "?limit=1")

	if len(resp.Snapshots) != 1 {
		t.Fatalf("expected 1 snapshot with limit=1, got %d", len(resp.Snapshots))
	}
	if resp.Snapshots[0].LeaguesExpanded != 0 { // i=0 -> now, the most recent
		t.Errorf("expected the most recent snapshot (LeaguesExpanded 0), got %d", resp.Snapshots[0].LeaguesExpanded)
	}
}

// TestGetSleeperStats_SkipParam covers paging past the most recent rows.
func TestGetSleeperStats_SkipParam(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	now := time.Now().UTC().Truncate(time.Hour)
	for i := 0; i < 3; i++ {
		db.Create(&models.SleeperLifetimeCount{SnapshotAt: now.Add(-time.Duration(i) * time.Hour), LeaguesExpanded: int64(i)})
	}

	resp := performGetSleeperStats(t, "?limit=1&skip=2")

	if len(resp.Snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(resp.Snapshots))
	}
	if resp.Snapshots[0].LeaguesExpanded != 2 { // i=2 -> oldest of the three
		t.Errorf("expected skip=2 to land on the oldest snapshot (LeaguesExpanded 2), got %d", resp.Snapshots[0].LeaguesExpanded)
	}
}

// TestGetSleeperStats_NilArchiveColumnsDefaultToZero covers a snapshot taken
// while no archive DB was configured (transactions_total/trades_completed/
// drafts_completed are NULL, not 0) — the handler must not error dereferencing
// a nil pointer, and must report 0 rather than propagate NULL.
func TestGetSleeperStats_NilArchiveColumnsDefaultToZero(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	db.Create(&models.SleeperLifetimeCount{SnapshotAt: time.Now().UTC().Truncate(time.Hour), LeaguesExpanded: 7})

	resp := performGetSleeperStats(t, "")

	if len(resp.Snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(resp.Snapshots))
	}
	got := resp.Snapshots[0]
	if got.LeaguesExpanded != 7 || got.TradeCount != 0 || got.DraftCount != 0 {
		t.Errorf("snapshot = %+v, want {LeaguesExpanded: 7, TradeCount: 0, DraftCount: 0}", got)
	}
}

func TestGetSleeperStats_EmptyTableReturnsEmptySeries(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	resp := performGetSleeperStats(t, "")

	if len(resp.Snapshots) != 0 {
		t.Errorf("expected an empty (non-nil) snapshots slice, got %d", len(resp.Snapshots))
	}
}

// TestGetSleeperStats_ExposesUsersAndLeaguesBreakdown covers the
// users/leagues total/pending/skipped breakdown fields in the response.
func TestGetSleeperStats_ExposesUsersAndLeaguesBreakdown(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	db.Create(&models.SleeperLifetimeCount{
		SnapshotAt: time.Now().UTC().Truncate(time.Hour),
		UsersTotal: 100, UsersExpanded: 60, UsersPending: 30, UsersSkipped: 10,
		LeaguesTotal: 50, LeaguesExpanded: 42, LeaguesPending: 5, LeaguesSkipped: 3,
	})

	resp := performGetSleeperStats(t, "")

	if len(resp.Snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(resp.Snapshots))
	}
	got := resp.Snapshots[0]
	want := SleeperStatsSnapshot{
		UsersTotal: 100, UsersExpanded: 60, UsersPending: 30, UsersSkipped: 10,
		LeaguesTotal: 50, LeaguesExpanded: 42, LeaguesPending: 5, LeaguesSkipped: 3,
	}
	if got.UsersTotal != want.UsersTotal || got.UsersExpanded != want.UsersExpanded ||
		got.UsersPending != want.UsersPending || got.UsersSkipped != want.UsersSkipped ||
		got.LeaguesTotal != want.LeaguesTotal || got.LeaguesExpanded != want.LeaguesExpanded ||
		got.LeaguesPending != want.LeaguesPending || got.LeaguesSkipped != want.LeaguesSkipped {
		t.Errorf("snapshot = %+v, want %+v", got, want)
	}
}

// TestGetSleeperStats_TransactionsTotalNilVsSet covers the pointer
// pass-through for transactions_total: nil (no archive DB configured for
// that snapshot) must round-trip as a JSON-absent field (omitempty), not a
// false zero, while a set value must pass through unchanged. Unmarshaling
// into the *int64 field is itself the proof: a present-but-omitted key
// leaves the pointer nil, and a present key sets it.
func TestGetSleeperStats_TransactionsTotalNilVsSet(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	withoutArchive := time.Now().UTC().Truncate(time.Hour).Add(-time.Hour)
	withArchive := time.Now().UTC().Truncate(time.Hour)
	txnTotal := int64(12345)

	db.Create(&models.SleeperLifetimeCount{SnapshotAt: withoutArchive})
	db.Create(&models.SleeperLifetimeCount{SnapshotAt: withArchive, TransactionsTotal: &txnTotal})

	resp := performGetSleeperStats(t, "")

	if len(resp.Snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(resp.Snapshots))
	}
	if resp.Snapshots[0].TransactionsTotal == nil || *resp.Snapshots[0].TransactionsTotal != txnTotal {
		t.Errorf("expected TransactionsTotal %d for the archive-configured snapshot, got %v", txnTotal, resp.Snapshots[0].TransactionsTotal)
	}
	if resp.Snapshots[1].TransactionsTotal != nil {
		t.Errorf("expected nil TransactionsTotal for the no-archive snapshot, got %v", *resp.Snapshots[1].TransactionsTotal)
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
