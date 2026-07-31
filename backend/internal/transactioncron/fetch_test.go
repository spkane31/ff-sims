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

// week3 is the NFL state most fetch tests use: current season, week 3, so
// MaxLegForLeague caps the sweep at leg 3 for a "2026" league.
func week3() *sleeper.NFLState { return &sleeper.NFLState{Season: "2026", Week: 3} }

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
	if _, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, week3()); err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("expected exactly 3 HTTP calls (legs 1..3), got %d", got)
	}
}

func TestFetchLeagueTransactions_ResumesFromWatermark(t *testing.T) {
	db := newTestDB(t)
	var calls atomic.Int64
	srv := batchTestServer(t, nil, &calls) // all legs 404
	defer srv.Close()

	watermark := 5
	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if _, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026", LastLegFetched: &watermark}, &sleeper.NFLState{Season: "2026", Week: 7}); err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	// Weeks below the watermark are final; resumes at the watermark week (5)
	// through the current week (7): legs 5,6,7 = 3 calls.
	if got := calls.Load(); got != 3 {
		t.Errorf("expected 3 HTTP calls (legs 5..7), got %d", got)
	}
}

func TestFetchLeagueTransactions_ReturnsRowsAndAdvancesNilWatermark(t *testing.T) {
	db := newTestDB(t)
	srv := batchTestServer(t, map[string][]sleeper.Transaction{
		"lg1/2": {{TransactionID: "tx1", Type: "waiver", Status: "complete", Leg: 2}},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, week3())
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if len(res.CloudRows) != 1 || res.CloudRows[0].SleeperTransactionID != "tx1" {
		t.Fatalf("expected tx1 in CloudRows, got %+v", res.CloudRows)
	}
	// Nil watermark + known week: the backfill succeeded, so the watermark
	// moves to the Sleeper-reported week (3) — not to the last leg with a
	// transaction (2).
	if res.WeekWatermark != 3 {
		t.Errorf("expected WeekWatermark == 3 (current week), got %d", res.WeekWatermark)
	}
}

// The next four tests pin the watermark semantics from #211: the cursor is a
// week watermark driven by Sleeper's reported week, not by whether any leg
// happened to contain a transaction.

func TestFetchLeagueTransactions_RefetchesEmptyActiveWeek(t *testing.T) {
	db := newTestDB(t)
	var calls atomic.Int64
	srv := batchTestServer(t, nil, &calls) // all legs 404 (empty)
	defer srv.Close()

	watermark := 5
	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa,
		transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026", LastLegFetched: &watermark},
		&sleeper.NFLState{Season: "2026", Week: 5})
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	// Same week as the watermark: exactly one call, re-fetching the active
	// week — no reset to a wider scan just because the week was empty.
	if got := calls.Load(); got != 1 {
		t.Errorf("expected exactly 1 HTTP call (re-fetch of active week 5), got %d", got)
	}
	if res.WeekWatermark != 0 {
		t.Errorf("same-week visit must not advance the watermark, got %d", res.WeekWatermark)
	}
}

func TestFetchLeagueTransactions_TransactionDoesNotAdvanceWatermark(t *testing.T) {
	db := newTestDB(t)
	srv := batchTestServer(t, map[string][]sleeper.Transaction{
		"lg1/6": {{TransactionID: "tx1", Type: "waiver", Status: "complete", Leg: 6}},
	}, nil)
	defer srv.Close()

	watermark := 6
	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa,
		transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026", LastLegFetched: &watermark},
		&sleeper.NFLState{Season: "2026", Week: 6})
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if len(res.CloudRows) != 1 {
		t.Fatalf("expected the active week's transaction in CloudRows, got %+v", res.CloudRows)
	}
	if res.WeekWatermark != 0 {
		t.Errorf("a transaction in the active week must not advance the watermark, got %d", res.WeekWatermark)
	}
}

func TestFetchLeagueTransactions_WeekRolloverAdvancesWatermark(t *testing.T) {
	db := newTestDB(t)
	var calls atomic.Int64
	srv := batchTestServer(t, nil, &calls) // all legs 404 (empty)
	defer srv.Close()

	watermark := 5
	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa,
		transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026", LastLegFetched: &watermark},
		&sleeper.NFLState{Season: "2026", Week: 6})
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	// Rollover from week 5 to 6: fetch the closed watermark week's tail (5)
	// plus the new active week (6), then advance the watermark to 6 even
	// though both weeks were empty.
	if got := calls.Load(); got != 2 {
		t.Errorf("expected 2 HTTP calls (legs 5..6), got %d", got)
	}
	if res.WeekWatermark != 6 {
		t.Errorf("expected watermark to advance to 6 on week rollover, got %d", res.WeekWatermark)
	}
}

func TestFetchLeagueTransactions_NilStateNeverAdvancesWatermark(t *testing.T) {
	db := newTestDB(t)
	srv := batchTestServer(t, map[string][]sleeper.Transaction{
		"lg1/2": {{TransactionID: "tx1", Type: "waiver", Status: "complete", Leg: 2}},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa,
		transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, nil)
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if len(res.CloudRows) != 1 {
		t.Fatalf("expected the fallback sweep to still collect rows, got %+v", res.CloudRows)
	}
	// State endpoint down: the 18-leg fallback sweep must not stamp a
	// watermark it can't justify — otherwise a current-season league would
	// skip every week between the fake watermark and reality.
	if res.WeekWatermark != 0 {
		t.Errorf("nil state must not advance the watermark, got %d", res.WeekWatermark)
	}
}

func TestFetchLeagueTransactions_PropagatesNonNotFoundLegErrors(t *testing.T) {
	db := newTestDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // non-retryable, non-404
	}))
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if _, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, week3()); err == nil {
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
	res, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, week3())
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
	res, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, week3())
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
	res, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, week3())
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
	res, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, week3())
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
			LeagueID:      "lg1",
			CloudRows:     []models.SleeperTransaction{{SleeperTransactionID: "tx1", SleeperLeagueID: "lg1", Type: "waiver", Status: "complete", Leg: 2}},
			WeekWatermark: 2,
		},
		{LeagueID: "lg2"}, // no watermark movement this run — WeekWatermark 0
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
		t.Errorf("expected lg2's watermark untouched (WeekWatermark 0), got %v", lg2.LastTransactionLegFetched)
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
			LeagueID:      "lg1",
			ArchiveRows:   []models.SleeperTransaction{{SleeperTransactionID: "tx-old", SleeperLeagueID: "lg1", Type: "waiver", Status: "complete", Leg: 2}},
			WeekWatermark: 2,
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

func TestFlushLeagueTransactions_WatermarkNeverRegresses(t *testing.T) {
	db := newTestDB(t)
	lg := claimedLeague(t, db, "lg1")
	stored := 7
	if err := db.Model(&lg).Update("last_transaction_leg_fetched", stored).Error; err != nil {
		t.Fatalf("seed watermark: %v", err)
	}

	dfa := &activities.DataFetchActivities{DB: db}
	batch := []transactioncron.LeagueTransactionFetchResult{{LeagueID: "lg1", WeekWatermark: 6}}
	if err := transactioncron.FlushLeagueTransactions(context.Background(), dfa, db, batch); err != nil {
		t.Fatalf("FlushLeagueTransactions error: %v", err)
	}

	var got models.SleeperLeague
	db.First(&got, "sleeper_league_id = ?", "lg1")
	if got.LastTransactionLegFetched == nil || *got.LastTransactionLegFetched != 7 {
		t.Errorf("watermark regressed: got %v, want 7", got.LastTransactionLegFetched)
	}
	if got.LastTransactionsFetchedAt == nil || got.ClaimedAt != nil {
		t.Errorf("expected lg1 stamped and unclaimed despite the stale watermark, got %+v", got)
	}
}

// TestFlushLeagueTransactions_ErrorLeavesStateUntouched guards existing
// ordering (it passes before and after #211's fix): a failed flush must not
// stamp last_transactions_fetched_at, clear the claim, or move the watermark
// — the league stays claimed and is retried when the claim expires.
func TestFlushLeagueTransactions_ErrorLeavesStateUntouched(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)
	claimedLeague(t, cloud, "lg1")
	sqlDB, err := archive.DB()
	if err != nil {
		t.Fatalf("unwrap archive sql.DB: %v", err)
	}
	sqlDB.Close() // make the archive upsert fail

	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive}
	batch := []transactioncron.LeagueTransactionFetchResult{{
		LeagueID:      "lg1",
		ArchiveRows:   []models.SleeperTransaction{{SleeperTransactionID: "tx-old", SleeperLeagueID: "lg1", Type: "waiver", Status: "complete", Leg: 2}},
		WeekWatermark: 2,
	}}
	if err := transactioncron.FlushLeagueTransactions(context.Background(), dfa, cloud, batch); err == nil {
		t.Fatal("expected flush to fail when the archive write fails")
	}

	var got models.SleeperLeague
	cloud.First(&got, "sleeper_league_id = ?", "lg1")
	if got.LastTransactionsFetchedAt != nil || got.ClaimedAt == nil || got.LastTransactionLegFetched != nil {
		t.Errorf("failed flush must leave claim/stamp/watermark untouched, got %+v", got)
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
	res, err := transactioncron.FetchLeagueTransactions(context.Background(), dfa, transactioncron.LeagueTransactionState{LeagueID: "lg1", Season: "2026"}, week3())
	if err != nil {
		t.Fatalf("FetchLeagueTransactions error: %v", err)
	}
	if len(res.CloudRows) != 1 || res.CloudRows[0].SleeperTransactionID != "tx-clean" {
		t.Errorf("expected only tx-clean in CloudRows (no archive configured), got %+v", res.CloudRows)
	}
}
