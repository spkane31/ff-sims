package transactioncron_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/sleeper"
	"backend/internal/transactioncron"
)

// newTestDB opens an in-memory SQLite DB migrated with the cloud models this
// package's fetch/flush tests need.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("unwrap sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.SleeperLeague{}, &models.SleeperTransaction{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

// newArchiveTestDB opens an in-memory SQLite DB migrated with the archive
// models — a lightweight stand-in for the archive DB in tests that need to
// prove routing actually lands rows in a *different* database than cloud.
func newArchiveTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("unwrap sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.ArchiveSleeperTransaction{}); err != nil {
		t.Fatalf("automigrate archive: %v", err)
	}
	return db
}

// batchTestServer fakes per-league transaction legs (/v1/league/{id}/transactions/{leg}).
// legs maps "leagueID/leg" -> transactions; missing keys 404 (empty leg).
// FetchLeagueTransactions never calls /v1/state/nfl itself (that's
// transactioncron's job, once per run, to compute maxLeg via
// MaxLegForLeague) so this fake doesn't need to serve it.
func batchTestServer(t *testing.T, legs map[string][]sleeper.Transaction, calls *atomic.Int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			calls.Add(1)
		}
		// path: /v1/league/{id}/transactions/{leg}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		key := parts[2] + "/" + parts[4]
		txns, ok := legs[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(txns)
	}))
}

func claimedLeague(t *testing.T, db *gorm.DB, id string) models.SleeperLeague {
	t.Helper()
	now := time.Now().UTC()
	l := models.SleeperLeague{SleeperLeagueID: id, Season: "2026", LastFetchedAt: &now, ClaimedAt: &now}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return l
}

func TestMaxLegForLeague(t *testing.T) {
	tests := []struct {
		name   string
		season string
		state  *sleeper.NFLState
		want   int
	}{
		{"nil state falls back to full sweep", "2026", nil, 18},
		{"past season gets full sweep", "2025", &sleeper.NFLState{Season: "2026", Week: 3}, 18},
		{"current season capped at current week", "2026", &sleeper.NFLState{Season: "2026", Week: 5}, 5},
		{"current season week beyond 18 clamps to 18", "2026", &sleeper.NFLState{Season: "2026", Week: 20}, 18},
		{"offseason week 0 still fetches leg 1", "2026", &sleeper.NFLState{Season: "2026", Week: 0}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := transactioncron.MaxLegForLeague(tt.season, tt.state); got != tt.want {
				t.Errorf("MaxLegForLeague(%q, %+v) = %d, want %d", tt.season, tt.state, got, tt.want)
			}
		})
	}
}

func TestFetchLeagueTransactions_FetchesLegsUpToMaxLeg(t *testing.T) {
	db := newTestDB(t)
	var calls atomic.Int64
	srv := batchTestServer(t, nil, &calls) // all legs 404
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if _, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, 3); err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("expected exactly 3 HTTP calls (legs 1..3), got %d", got)
	}
}

func TestFetchLeagueTransactions_ResumesFromLastLegFetched(t *testing.T) {
	db := newTestDB(t)
	var calls atomic.Int64
	srv := batchTestServer(t, nil, &calls) // all legs 404
	defer srv.Close()

	lastLeg := 5
	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if _, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026", LastLegFetched: &lastLeg}, 7); err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	// Resumes at lastLeg-1 (4) through maxLeg (7): legs 4,5,6,7 = 4 calls.
	if got := calls.Load(); got != 4 {
		t.Errorf("expected 4 HTTP calls (legs 4..7), got %d", got)
	}
}

func TestFetchLeagueTransactions_ReturnsRowsAndMaxLegSeen(t *testing.T) {
	db := newTestDB(t)
	srv := batchTestServer(t, map[string][]sleeper.Transaction{
		"lg1/2": {{TransactionID: "tx1", Type: "waiver", Status: "complete", Leg: 2}},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, 3)
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if len(res.CloudRows) != 1 || res.CloudRows[0].SleeperTransactionID != "tx1" {
		t.Fatalf("expected tx1 in CloudRows, got %+v", res.CloudRows)
	}
	if res.MaxLegSeen != 2 {
		t.Errorf("expected MaxLegSeen == 2, got %d", res.MaxLegSeen)
	}
}

func TestFetchLeagueTransactions_PropagatesNonNotFoundLegErrors(t *testing.T) {
	db := newTestDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // non-retryable, non-404
	}))
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if _, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, 3); err == nil {
		t.Fatal("expected a non-404 leg error to propagate")
	}
}

func TestFetchLeagueTransactions_SplitsOldRowsToArchiveByAge(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)

	now := time.Now().UTC()
	recentMs := now.Add(-1 * time.Hour).UnixMilli()
	oldMs := now.AddDate(0, 0, -60).UnixMilli() // 60 days ago, past the 30-day default retention

	srv := batchTestServer(t, map[string][]sleeper.Transaction{
		"lg1/2": {
			{TransactionID: "tx-recent", Type: "waiver", Status: "complete", Leg: 2, Created: recentMs},
			{TransactionID: "tx-old", Type: "waiver", Status: "complete", Leg: 2, Created: oldMs},
		},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, 3)
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if len(res.CloudRows) != 1 || res.CloudRows[0].SleeperTransactionID != "tx-recent" {
		t.Errorf("expected only tx-recent in CloudRows, got %+v", res.CloudRows)
	}
	if len(res.ArchiveRows) != 1 || res.ArchiveRows[0].SleeperTransactionID != "tx-old" {
		t.Errorf("expected only tx-old in ArchiveRows, got %+v", res.ArchiveRows)
	}
}

func TestFetchLeagueTransactions_ArchiveExcludesTransactionsWithDraftPicksOrFAAB(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)

	oldMs := time.Now().UTC().AddDate(0, 0, -60).UnixMilli() // past the 30-day default retention
	srv := batchTestServer(t, map[string][]sleeper.Transaction{
		"lg1/2": {
			{TransactionID: "tx-clean", Type: "waiver", Status: "complete", Leg: 2, Created: oldMs},
			{TransactionID: "tx-picks", Type: "trade", Status: "complete", Leg: 2, Created: oldMs,
				DraftPicks: []interface{}{map[string]interface{}{"season": "2027", "round": float64(1)}}},
			{TransactionID: "tx-faab", Type: "waiver", Status: "complete", Leg: 2, Created: oldMs,
				WaiverBudget: []interface{}{map[string]interface{}{"amount": float64(10)}}},
		},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, 3)
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if len(res.ArchiveRows) != 1 || res.ArchiveRows[0].SleeperTransactionID != "tx-clean" {
		t.Errorf("expected only tx-clean in ArchiveRows, got %+v", res.ArchiveRows)
	}
	if len(res.CloudRows) != 0 {
		t.Errorf("expected no CloudRows (all old), got %+v", res.CloudRows)
	}
}

// TestFetchLeagueTransactions_ExcludesDraftPicksOrFAABEvenWhenCloudBound
// guards the fix from #191: isPlayerOnlyTransaction must apply
// unconditionally to every fetched row, not only the ones old enough to
// route to archive — otherwise picks/FAAB trades leak into the live
// sleeper_transactions table via CloudRows.
func TestFetchLeagueTransactions_ExcludesDraftPicksOrFAABEvenWhenCloudBound(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)

	recentMs := time.Now().UTC().Add(-1 * time.Hour).UnixMilli()
	srv := batchTestServer(t, map[string][]sleeper.Transaction{
		"lg1/2": {
			{TransactionID: "tx-clean", Type: "waiver", Status: "complete", Leg: 2, Created: recentMs},
			{TransactionID: "tx-picks", Type: "trade", Status: "complete", Leg: 2, Created: recentMs,
				DraftPicks: []interface{}{map[string]interface{}{"season": "2027", "round": float64(1)}}},
			{TransactionID: "tx-faab", Type: "waiver", Status: "complete", Leg: 2, Created: recentMs,
				WaiverBudget: []interface{}{map[string]interface{}{"amount": float64(10)}}},
		},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, 3)
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if len(res.CloudRows) != 1 || res.CloudRows[0].SleeperTransactionID != "tx-clean" {
		t.Errorf("expected only tx-clean in CloudRows, got %+v", res.CloudRows)
	}
	if len(res.ArchiveRows) != 0 {
		t.Errorf("expected no ArchiveRows (all recent), got %+v", res.ArchiveRows)
	}
}

func TestFetchLeagueTransactions_AllRowsToCloudWhenArchiveNil(t *testing.T) {
	cloud := newTestDB(t)
	oldMs := time.Now().UTC().AddDate(0, 0, -60).UnixMilli()
	srv := batchTestServer(t, map[string][]sleeper.Transaction{
		"lg1/2": {{TransactionID: "tx-old", Type: "waiver", Status: "complete", Leg: 2, Created: oldMs}},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Sleeper: sleeper.NewWithBaseURL(srv.URL)} // Archive nil
	res, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, 3)
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if len(res.CloudRows) != 1 {
		t.Errorf("expected the old txn to fall back to CloudRows when Archive is nil, got %+v", res.CloudRows)
	}
	if len(res.ArchiveRows) != 0 {
		t.Errorf("expected no ArchiveRows when Archive is nil, got %+v", res.ArchiveRows)
	}
}

func TestFlushLeagueTransactions_StampsClearsClaimsAndWritesRows(t *testing.T) {
	db := newTestDB(t)
	claimedLeague(t, db, "lg1")
	claimedLeague(t, db, "lg2")

	batch := []transactioncron.LeagueTransactionFetchResult{
		{
			LeagueID:   "lg1",
			CloudRows:  []models.SleeperTransaction{{SleeperTransactionID: "tx1", SleeperLeagueID: "lg1", Type: "waiver", Status: "complete", Leg: 2}},
			MaxLegSeen: 2,
		},
		{LeagueID: "lg2"}, // nothing new this run — MaxLegSeen 0
	}
	dfa := &activities.DataFetchActivities{DB: db}
	if err := transactioncron.FlushLeagueTransactions(context.Background(), dfa, db, batch); err != nil {
		t.Fatalf("FlushLeagueTransactions error: %v", err)
	}

	var lg1, lg2 models.SleeperLeague
	db.First(&lg1, "sleeper_league_id = ?", "lg1")
	db.First(&lg2, "sleeper_league_id = ?", "lg2")
	if lg1.LastTransactionsFetchedAt == nil || lg1.ClaimedAt != nil {
		t.Errorf("lg1 not stamped/unclaimed: %+v", lg1)
	}
	if lg1.LastTransactionLegFetched == nil || *lg1.LastTransactionLegFetched != 2 {
		t.Errorf("lg1 leg cursor = %v, want 2", lg1.LastTransactionLegFetched)
	}
	if lg2.LastTransactionsFetchedAt == nil || lg2.ClaimedAt != nil {
		t.Errorf("lg2 not stamped/unclaimed: %+v", lg2)
	}
	if lg2.LastTransactionLegFetched != nil {
		t.Errorf("expected lg2's leg cursor untouched (MaxLegSeen 0), got %v", lg2.LastTransactionLegFetched)
	}
	var txCount int64
	db.Model(&models.SleeperTransaction{}).Count(&txCount)
	if txCount != 1 {
		t.Errorf("expected 1 transaction row, got %d", txCount)
	}
}

func TestFlushLeagueTransactions_WritesArchiveRowsToArchiveDB(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)
	claimedLeague(t, cloud, "lg1")

	batch := []transactioncron.LeagueTransactionFetchResult{
		{
			LeagueID:    "lg1",
			ArchiveRows: []models.SleeperTransaction{{SleeperTransactionID: "tx-old", SleeperLeagueID: "lg1", Type: "waiver", Status: "complete", Leg: 2}},
			MaxLegSeen:  2,
		},
	}
	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive}
	if err := transactioncron.FlushLeagueTransactions(context.Background(), dfa, cloud, batch); err != nil {
		t.Fatalf("FlushLeagueTransactions error: %v", err)
	}

	var archiveIDs []string
	archive.Model(&models.ArchiveSleeperTransaction{}).Pluck("sleeper_transaction_id", &archiveIDs)
	if len(archiveIDs) != 1 || archiveIDs[0] != "tx-old" {
		t.Errorf("expected tx-old written to archive, got %v", archiveIDs)
	}
	var cloudCount int64
	cloud.Model(&models.SleeperTransaction{}).Count(&cloudCount)
	if cloudCount != 0 {
		t.Errorf("expected no cloud rows (batch had only ArchiveRows), got %d", cloudCount)
	}
}

func TestFetchLeagueTransactions_ExcludesDraftPicksWhenArchiveNil(t *testing.T) {
	cloud := newTestDB(t)

	recentMs := time.Now().UTC().Add(-1 * time.Hour).UnixMilli()
	srv := batchTestServer(t, map[string][]sleeper.Transaction{
		"lg1/2": {
			{TransactionID: "tx-clean", Type: "waiver", Status: "complete", Leg: 2, Created: recentMs},
			{TransactionID: "tx-picks", Type: "trade", Status: "complete", Leg: 2, Created: recentMs,
				DraftPicks: []interface{}{map[string]interface{}{"season": "2027", "round": float64(1)}}},
		},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Sleeper: sleeper.NewWithBaseURL(srv.URL)} // Archive nil
	res, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, 3)
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if len(res.CloudRows) != 1 || res.CloudRows[0].SleeperTransactionID != "tx-clean" {
		t.Errorf("expected only tx-clean in CloudRows (no archive configured), got %+v", res.CloudRows)
	}
}
