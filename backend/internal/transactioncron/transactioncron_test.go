package transactioncron_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/sleeper"
	"backend/internal/testutil"
	"backend/internal/transactioncron"
)

// TestRunTransactionSync_ProcessesLeaguesToCompletion needs real Postgres,
// not SQLite: ClaimLeaguesForTransactions uses now(), interval, and FOR
// UPDATE SKIP LOCKED — syntax SQLite's parser rejects outright. Mirrors
// discoverycron's TestRunDiscovery_ProcessesUsersAndLeaguesToCompletion.
func TestRunTransactionSync_ProcessesLeaguesToCompletion(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; RunTransactionSync needs Postgres (FOR UPDATE SKIP LOCKED claim query)")
	}
	scopedDSN := testutil.NewPGSchema(t, dsn, "transactioncron_run_test")
	db := testutil.OpenGORM(t, scopedDSN)
	if err := db.AutoMigrate(&models.SleeperLeague{}, &models.SleeperTransaction{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	now := time.Now().UTC()
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1", Season: "2026", LastFetchedAt: &now})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/state/nfl" {
			json.NewEncoder(w).Encode(sleeper.NFLState{Season: "2026", Week: 3})
			return
		}
		w.WriteHeader(http.StatusNotFound) // no transactions on any leg
	}))
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	cfg := transactioncron.Config{PoolSize: 2, RefillBatch: 1}

	// Short deadline: RunPool polls until ctx is done (no early-exit on an
	// empty queue), so the test genuinely runs close to its full deadline —
	// see discoverycron's identical test for the same reasoning.
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	report, err := transactioncron.RunTransactionSync(ctx, dfa, cfg)
	if err != nil {
		t.Fatalf("RunTransactionSync error: %v", err)
	}
	if report.LeaguesProcessed < 1 {
		t.Errorf("expected at least 1 league processed, got %+v", report)
	}

	var lg models.SleeperLeague
	if err := db.First(&lg, "sleeper_league_id = ?", "lg1").Error; err != nil {
		t.Fatalf("lookup league: %v", err)
	}
	if lg.LastTransactionsFetchedAt == nil {
		t.Error("expected lg1 stamped last_transactions_fetched_at")
	}
	if lg.ClaimedAt != nil {
		t.Error("expected lg1's claim cleared on completion")
	}
}

// TestRunTransactionSync_AggregatesClaimErrors uses the package's SQLite
// fixture deliberately: claimLeaguesForTransactionsSQL uses Postgres-only
// syntax, so every claim attempt fails outright — a cheap, deterministic way
// to force claim errors without a real unreachable-Postgres scenario.
// Mirrors discoverycron's TestRunDiscovery_AggregatesClaimErrorsFromBothPools.
func TestRunTransactionSync_AggregatesClaimErrors(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.SleeperLeague{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	// A fake server (never a real sleeper.New()) so the GetNFLState call at
	// the top of RunTransactionSync doesn't reach out to the real network in
	// a unit test; 404 exercises the "state endpoint down" fallback path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	cfg := transactioncron.Config{PoolSize: 1, RefillBatch: 1}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	report, err := transactioncron.RunTransactionSync(ctx, dfa, cfg)
	if err != nil {
		t.Fatalf("RunTransactionSync error: %v", err)
	}

	if report.ClaimErrors == 0 {
		t.Error("expected ClaimErrors > 0 when the claim query fails every attempt")
	}
	if report.LeaguesProcessed != 0 {
		t.Errorf("expected nothing processed when claiming always fails, got %+v", report)
	}
}
