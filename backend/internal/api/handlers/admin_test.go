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
	if err := db.AutoMigrate(&models.SleeperLeague{}, &models.SleeperTransaction{}, &models.SleeperUser{}, &models.SleeperLifetimeCount{}, &models.SleeperDraft{}); err != nil {
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

func performGetAdminSegments(t *testing.T) AdminSegmentsResponse {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin/segments", GetAdminSegments)

	req := httptest.NewRequest(http.MethodGet, "/admin/segments", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp AdminSegmentsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return resp
}

func performGetAdminDiscoveryFrontier(t *testing.T) AdminDiscoveryFrontierResponse {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin/discovery-frontier", GetAdminDiscoveryFrontier)

	req := httptest.NewRequest(http.MethodGet, "/admin/discovery-frontier", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp AdminDiscoveryFrontierResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return resp
}

func findLeagueSeasonRow(rows []AdminDiscoveryLeagueSeasonRow, season string) *AdminDiscoveryLeagueSeasonRow {
	for i := range rows {
		if rows[i].Season == season {
			return &rows[i]
		}
	}
	return nil
}

func findSegmentRow(rows []AdminSegmentRow, scoring string, superflex bool, size string) *AdminSegmentRow {
	for i := range rows {
		if rows[i].Scoring == scoring && rows[i].Superflex == superflex && rows[i].LeagueSize == size {
			return &rows[i]
		}
	}
	return nil
}

func TestGetAdminSegments_Buckets(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	now := time.Now().UTC()
	sf, oneQB := true, false
	seg := func(id string, ppr float64, superflex *bool, size int) *models.SleeperLeague {
		p := ppr
		return &models.SleeperLeague{
			SleeperLeagueID: id, Season: "2025", PPR: &p, IsSuperflex: superflex,
			TotalRosters: size, LastFetchedAt: &now,
		}
	}

	db.Create(seg("a", 1, &sf, 12))
	db.Create(seg("b", 1, &sf, 12))
	db.Create(seg("c", 0.5, &oneQB, 10))
	db.Create(seg("d", 0, &oneQB, 8))
	db.Create(seg("e", 1, &sf, 14))
	db.Create(seg("f", 1, &sf, 16))    // buckets with 14 as "14+"
	db.Create(seg("g", 0.75, &sf, 12)) // odd scoring -> "Other"
	db.Create(seg("h", 1, &oneQB, 9))  // odd size -> "Other"

	// excluded: details never fetched, or skipped
	p := 1.0
	db.Create(&models.SleeperLeague{SleeperLeagueID: "never", Season: "2025", PPR: &p, TotalRosters: 12})
	skippedAt := now
	db.Create(&models.SleeperLeague{
		SleeperLeagueID: "skip", Season: "2025", PPR: &p, TotalRosters: 12,
		LastFetchedAt: &now, SkippedAt: &skippedAt,
	})

	// leagues "a" and "b" (PPR/superflex/12) get 2 and 1 transactions; "c" (0.5 PPR/10) gets none.
	db.Create(&models.SleeperTransaction{SleeperTransactionID: "tx1", SleeperLeagueID: "a", Type: "trade", Status: "complete"})
	db.Create(&models.SleeperTransaction{SleeperTransactionID: "tx2", SleeperLeagueID: "a", Type: "waiver", Status: "complete"})
	db.Create(&models.SleeperTransaction{SleeperTransactionID: "tx3", SleeperLeagueID: "b", Type: "trade", Status: "complete"})

	resp := performGetAdminSegments(t)

	if resp.TotalLeagues != 8 {
		t.Errorf("expected 8 total leagues, got %d", resp.TotalLeagues)
	}
	if resp.TotalTransactions != 3 {
		t.Errorf("expected 3 total transactions, got %d", resp.TotalTransactions)
	}

	checks := []struct {
		scoring      string
		superflex    bool
		size         string
		leagues      int64
		transactions int64
	}{
		{"PPR", true, "12", 2, 3},
		{"0.5 PPR", false, "10", 1, 0},
		{"Standard", false, "8", 1, 0},
		{"PPR", true, "14+", 2, 0},
		{"Other", true, "12", 1, 0},
		{"PPR", false, "Other", 1, 0},
	}
	for _, c := range checks {
		row := findSegmentRow(resp.Segments, c.scoring, c.superflex, c.size)
		if row == nil {
			t.Errorf("missing row scoring=%q superflex=%v size=%q", c.scoring, c.superflex, c.size)
			continue
		}
		if row.Leagues != c.leagues {
			t.Errorf("row %q/%v/%q: expected %d leagues, got %d",
				c.scoring, c.superflex, c.size, c.leagues, row.Leagues)
		}
		if row.Transactions != c.transactions {
			t.Errorf("row %q/%v/%q: expected %d transactions, got %d",
				c.scoring, c.superflex, c.size, c.transactions, row.Transactions)
		}
	}

	// sorted by count descending
	for i := 1; i < len(resp.Segments); i++ {
		if resp.Segments[i].Leagues > resp.Segments[i-1].Leagues {
			t.Errorf("segments not sorted by leagues desc at index %d", i)
		}
	}
}

func TestGetAdminSegments_EmptyTable(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	resp := performGetAdminSegments(t)

	if resp.TotalLeagues != 0 {
		t.Errorf("expected 0 total leagues, got %d", resp.TotalLeagues)
	}
	if resp.Segments == nil {
		t.Error("expected empty (non-nil) segments slice")
	}
	if len(resp.Segments) != 0 {
		t.Errorf("expected no segment rows, got %d", len(resp.Segments))
	}
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

func TestGetAdminBacklog_Buckets(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	now := time.Now().UTC()
	at := func(d time.Duration) *time.Time {
		ts := now.Add(d)
		return &ts
	}

	db.Create(&models.SleeperLeague{SleeperLeagueID: "never", Season: "2026"})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "b0", Season: "2026", LastTransactionsFetchedAt: at(-1 * time.Hour)})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "b4", Season: "2026", LastTransactionsFetchedAt: at(-5 * time.Hour)})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "b8", Season: "2026", LastTransactionsFetchedAt: at(-9 * time.Hour)})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "b12", Season: "2026", LastTransactionsFetchedAt: at(-13 * time.Hour)})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "b16", Season: "2026", LastTransactionsFetchedAt: at(-17 * time.Hour)})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "b20", Season: "2026", LastTransactionsFetchedAt: at(-21 * time.Hour)})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "b24", Season: "2026", LastTransactionsFetchedAt: at(-30 * time.Hour)})

	resp := performGetAdminBacklog(t)

	if len(resp.Buckets) != 8 {
		t.Fatalf("expected 8 buckets, got %d", len(resp.Buckets))
	}

	wantOrder := []string{
		"Never fetched", "0h-3h59m", "4h-7h59m", "8h-11h59m",
		"12h-15h59m", "16h-19h59m", "20h-23h59m", "24h+",
	}
	for i, label := range wantOrder {
		if resp.Buckets[i].Label != label {
			t.Errorf("index %d: expected label %q, got %q", i, label, resp.Buckets[i].Label)
		}
		if resp.Buckets[i].Leagues != 1 {
			t.Errorf("bucket %q: expected 1 league, got %d", label, resp.Buckets[i].Leagues)
		}
	}
}

func TestGetAdminBacklog_BucketsExcludeOtherSeasonsAndSkipped(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	now := time.Now().UTC()
	skippedAt := now
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-2026", Season: "2026", LastTransactionsFetchedAt: &now})
	db.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg-2026-skipped", Season: "2026", LastTransactionsFetchedAt: &now, SkippedAt: &skippedAt,
	})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-2025", Season: "2025", LastTransactionsFetchedAt: &now})

	resp := performGetAdminBacklog(t)

	if resp.Season != "2026" {
		t.Fatalf("expected season 2026, got %q", resp.Season)
	}

	var total int64
	for _, row := range resp.Buckets {
		total += row.Leagues
	}
	if total != 1 {
		t.Errorf("expected 1 league counted across buckets (excluding other season + skipped), got %d", total)
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
	if len(resp.Buckets) != 8 {
		t.Fatalf("expected 8 buckets, got %d", len(resp.Buckets))
	}
	for _, row := range resp.Buckets {
		if row.Leagues != 0 {
			t.Errorf("bucket %q: expected 0 leagues, got %d", row.Label, row.Leagues)
		}
	}
}

func TestFillBacklogBuckets_ZeroFillsMissingLabels(t *testing.T) {
	rows := []AdminBacklogBucketRow{
		{Label: "24h+", Leagues: 3},
		{Label: "Never fetched", Leagues: 5},
	}

	filled := fillBacklogBuckets(rows)

	want := []AdminBacklogBucketRow{
		{Label: "Never fetched", Leagues: 5},
		{Label: "0h-3h59m", Leagues: 0},
		{Label: "4h-7h59m", Leagues: 0},
		{Label: "8h-11h59m", Leagues: 0},
		{Label: "12h-15h59m", Leagues: 0},
		{Label: "16h-19h59m", Leagues: 0},
		{Label: "20h-23h59m", Leagues: 0},
		{Label: "24h+", Leagues: 3},
	}
	if len(filled) != len(want) {
		t.Fatalf("expected %d buckets, got %d", len(want), len(filled))
	}
	for i, w := range want {
		if filled[i] != w {
			t.Errorf("index %d: expected %+v, got %+v", i, w, filled[i])
		}
	}
}

func TestFillBacklogBuckets_EmptyInput(t *testing.T) {
	filled := fillBacklogBuckets(nil)

	if len(filled) != len(backlogBucketLabels) {
		t.Fatalf("expected %d buckets, got %d", len(backlogBucketLabels), len(filled))
	}
	for i, row := range filled {
		if row.Leagues != 0 {
			t.Errorf("index %d: expected 0 leagues, got %d", i, row.Leagues)
		}
		if row.Label != backlogBucketLabels[i] {
			t.Errorf("index %d: expected label %q, got %q", i, backlogBucketLabels[i], row.Label)
		}
	}
}

func TestGetAdminDatabaseSize_RequiresPostgres(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin/database-size", GetAdminDatabaseSize)

	req := httptest.NewRequest(http.MethodGet, "/admin/database-size", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on non-Postgres backend, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if body["error"] == "" {
		t.Error("expected non-empty error message")
	}
}

func TestGetAdminDiscoveryFrontier_UserCounts(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	now := time.Now().UTC()

	db.Create(&models.SleeperUser{SleeperUserID: "expanded-1", LastFetchedAt: &now})
	db.Create(&models.SleeperUser{SleeperUserID: "expanded-2", LastFetchedAt: &now})
	db.Create(&models.SleeperUser{SleeperUserID: "pending-1"})
	db.Create(&models.SleeperUser{SleeperUserID: "pending-2"})
	db.Create(&models.SleeperUser{SleeperUserID: "pending-3"})
	db.Create(&models.SleeperUser{SleeperUserID: "skipped-1", SkippedAt: &now})

	resp := performGetAdminDiscoveryFrontier(t)

	if resp.Users.Total != 6 {
		t.Errorf("expected 6 total users, got %d", resp.Users.Total)
	}
	if resp.Users.Expanded != 2 {
		t.Errorf("expected 2 expanded users, got %d", resp.Users.Expanded)
	}
	if resp.Users.Pending != 3 {
		t.Errorf("expected 3 pending users, got %d", resp.Users.Pending)
	}
	if resp.Users.Skipped != 1 {
		t.Errorf("expected 1 skipped user, got %d", resp.Users.Skipped)
	}
}

func TestGetAdminDiscoveryFrontier_LeaguesBySeason(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	now := time.Now().UTC()

	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-2026-a", Season: "2026", LastFetchedAt: &now})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-2026-b", Season: "2026"})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-2026-c", Season: "2026", SkippedAt: &now})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-2025-a", Season: "2025", LastFetchedAt: &now})
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-2025-b", Season: "2025", LastFetchedAt: &now})

	resp := performGetAdminDiscoveryFrontier(t)

	if len(resp.LeaguesBySeason) != 2 {
		t.Fatalf("expected 2 seasons, got %d", len(resp.LeaguesBySeason))
	}

	// ordered season descending
	if resp.LeaguesBySeason[0].Season != "2026" || resp.LeaguesBySeason[1].Season != "2025" {
		t.Errorf("expected seasons ordered [2026, 2025], got [%s, %s]",
			resp.LeaguesBySeason[0].Season, resp.LeaguesBySeason[1].Season)
	}

	row2026 := findLeagueSeasonRow(resp.LeaguesBySeason, "2026")
	if row2026 == nil {
		t.Fatal("missing 2026 row")
	}
	if row2026.Total != 3 || row2026.Expanded != 1 || row2026.Pending != 1 || row2026.Skipped != 1 {
		t.Errorf("2026: expected total=3 expanded=1 pending=1 skipped=1, got total=%d expanded=%d pending=%d skipped=%d",
			row2026.Total, row2026.Expanded, row2026.Pending, row2026.Skipped)
	}

	row2025 := findLeagueSeasonRow(resp.LeaguesBySeason, "2025")
	if row2025 == nil {
		t.Fatal("missing 2025 row")
	}
	if row2025.Total != 2 || row2025.Expanded != 2 || row2025.Pending != 0 || row2025.Skipped != 0 {
		t.Errorf("2025: expected total=2 expanded=2 pending=0 skipped=0, got total=%d expanded=%d pending=%d skipped=%d",
			row2025.Total, row2025.Expanded, row2025.Pending, row2025.Skipped)
	}
}

func TestGetAdminDiscoveryFrontier_EmptyTables(t *testing.T) {
	db := newAdminTestDB(t)
	withAdminTestDB(t, db)

	resp := performGetAdminDiscoveryFrontier(t)

	if resp.Users.Total != 0 || resp.Users.Expanded != 0 || resp.Users.Pending != 0 || resp.Users.Skipped != 0 {
		t.Errorf("expected all-zero user counts, got %+v", resp.Users)
	}
	if resp.LeaguesBySeason == nil {
		t.Error("expected empty (non-nil) leagues_by_season slice")
	}
	if len(resp.LeaguesBySeason) != 0 {
		t.Errorf("expected no league season rows, got %d", len(resp.LeaguesBySeason))
	}
}
