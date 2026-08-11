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

func TestGetSleeperTrades_FiltersPlayerToRecentWindowAndPaginates(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	now := time.Now().UTC()
	ppr := 1.0
	superflex := true
	if err := db.Create(&models.SleeperLeague{
		SleeperLeagueID: "league-1", Name: "Recent League", Season: "2026",
		PPR: &ppr, IsSuperflex: &superflex, TotalRosters: 12,
	}).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
	trades := []models.SleeperTransaction{
		{SleeperTransactionID: "recent-1", SleeperLeagueID: "league-1", Type: "trade", Status: "complete", CreatedAtSleeper: now.Add(-time.Hour).UnixMilli(), Adds: json.RawMessage(`{"player-1": 1}`), TradeValues: json.RawMessage(`{"1": 1000}`)},
		{SleeperTransactionID: "recent-2", SleeperLeagueID: "league-1", Type: "trade", Status: "complete", CreatedAtSleeper: now.Add(-2 * time.Hour).UnixMilli(), Adds: json.RawMessage(`{"player-1": 2}`), TradeValues: json.RawMessage(`{"2": 1000}`)},
		{SleeperTransactionID: "old", SleeperLeagueID: "league-1", Type: "trade", Status: "complete", CreatedAtSleeper: now.Add(-31 * 24 * time.Hour).UnixMilli(), Adds: json.RawMessage(`{"player-1": 1}`), TradeValues: json.RawMessage(`{"1": 1000}`)},
		{SleeperTransactionID: "other-player", SleeperLeagueID: "league-1", Type: "trade", Status: "complete", CreatedAtSleeper: now.Add(-time.Hour).UnixMilli(), Adds: json.RawMessage(`{"player-2": 1}`), TradeValues: json.RawMessage(`{"1": 1000}`)},
	}
	if err := db.Create(&trades).Error; err != nil {
		t.Fatalf("seed trades: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/sleeper/trades", GetSleeperTrades)
	req := httptest.NewRequest(http.MethodGet, "/sleeper/trades?sleeper_player_id=player-1&days=30&limit=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var response SleeperTradesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Total != 2 || response.TotalPages != 2 {
		t.Errorf("expected two recent player trades across two pages, got total=%d pages=%d", response.Total, response.TotalPages)
	}
	if len(response.Trades) != 1 || response.Trades[0].ID != "recent-1" {
		t.Errorf("expected the most recent matching trade, got %+v", response.Trades)
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

func TestGetSleeperTrades_ReadsPersistedTradeValues(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	ppr := 1.0
	sf := true
	db.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg1", Name: "Test League", Season: "2025",
		PPR: &ppr, IsSuperflex: &sf, TotalRosters: 10, LeagueType: "redraft",
	})
	db.Create(&models.SleeperTransaction{
		SleeperTransactionID: "tx1", SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
		CreatedAtSleeper: time.Now().UTC().UnixMilli(),
		Adds:             json.RawMessage(`{"p1": 7, "p2": 8}`),
		TradeValues:      json.RawMessage(`{"7": 5000}`), // roster 8 intentionally absent (unvalued)
	})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/sleeper/trades", GetSleeperTrades)
	req := httptest.NewRequest(http.MethodGet, "/sleeper/trades", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var response SleeperTradesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(response.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(response.Trades))
	}
	var side7, side8 *TradeSide
	for i := range response.Trades[0].Sides {
		s := &response.Trades[0].Sides[i]
		switch s.RosterID {
		case 7:
			side7 = s
		case 8:
			side8 = s
		}
	}
	if side7 == nil || side7.TotalValue == nil || *side7.TotalValue != 5000 {
		t.Errorf("expected roster 7 total_value 5000, got %+v", side7)
	}
	if side8 == nil || side8.TotalValue != nil {
		t.Errorf("expected roster 8 total_value nil (absent from persisted trade_values), got %+v", side8)
	}
}
