package discoverycron_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"backend/internal/discoverycron"
	"backend/internal/models"
	"backend/internal/sleeper"
	"backend/internal/testutil"
)

// TestRunDiscovery_ProcessesUsersAndLeaguesToCompletion needs real Postgres,
// not the package's usual SQLite fixture (newSQLiteDB, from fetch_test.go):
// RunDiscovery's two pools claim through ClaimStaleUsers and
// ClaimStaleLeagues, both of which use `now()`, `interval`, and `FOR UPDATE
// SKIP LOCKED` — syntax SQLite's parser rejects outright ("syntax error near
// '120 minutes'"), not just semantics it happens to get wrong. Every other
// test touching those claim queries (claim_pg_test.go, both packages) is
// already gated the same way, and ci.yml documents exactly this: "tests
// that need real Postgres semantics ... skip when TEST_DATABASE_URL is
// unset."
func TestRunDiscovery_ProcessesUsersAndLeaguesToCompletion(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; RunDiscovery needs Postgres (FOR UPDATE SKIP LOCKED claim queries)")
	}
	scopedDSN := testutil.NewPGSchema(t, dsn, "discoverycron_rundiscovery_test")
	db := testutil.OpenGORM(t, scopedDSN)
	if err := db.AutoMigrate(&models.SleeperUser{}, &models.SleeperLeague{}, &models.SleeperLeagueUser{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	db.Create(&models.SleeperUser{SleeperUserID: "user1"})
	// Season must be >= "2025" (claimStaleLeaguesSQL's eligibility filter,
	// matching firstScannedSeason) for ClaimStaleLeagues to pick this row up
	// at all.
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg-existing", Season: "2026"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case parts[1] == "user":
			json.NewEncoder(w).Encode([]sleeper.League{{LeagueID: "lg-new", Season: "2026", Sport: "nfl"}})
		case strings.HasSuffix(r.URL.Path, "/users"):
			json.NewEncoder(w).Encode([]sleeper.LeagueUser{})
		default:
			json.NewEncoder(w).Encode(sleeper.League{LeagueID: parts[2]})
		}
	}))
	defer srv.Close()

	sleeperClient := sleeper.NewWithBaseURL(srv.URL)
	// Small batch size and a short flush interval — not huge values forcing
	// only the shutdown-time flush (see fdb's own tests for that guarantee
	// in isolation): this test's whole point is that lg-new, discovered
	// mid-run by the user pool, must actually land in sleeper_leagues in
	// time for the league pool to claim and process it inside the same
	// 600ms run, so the user pool's flush needs to happen promptly.
	cfg := discoverycron.Config{
		UserPoolSize: 2, UserRefillBatch: 1, LeaguePoolSize: 2, LeagueRefillBatch: 1,
		UserBatchSize: 1, LeagueBatchSize: 1,
		UserBatchFlushInterval: 50 * time.Millisecond, LeagueBatchFlushInterval: 50 * time.Millisecond,
	}

	// The job's own deadline stands in for cmd/cron's -max-duration. fdb.RunPool
	// has no early-exit-on-empty-queue behavior by design — it keeps polling
	// until ctx is done, matching production (a cron run uses any leftover
	// time to keep checking for newly-claimable work). So this test genuinely
	// runs for close to its full deadline, not less; keep that deadline short
	// so the suite stays fast.
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	report, err := discoverycron.RunDiscovery(ctx, db, sleeperClient, cfg)
	if err != nil {
		t.Fatalf("RunDiscovery error: %v", err)
	}
	// >= 1, not == 1: ClaimStaleUsers has no "already synced this run" guard
	// beyond the 120-minute claimed_at TTL — the instant a flush clears
	// claimed_at, user1's row is claimable again. With pollInterval this
	// short, fdb's poll-until-ctx-done loop reliably reclaims and reprocesses
	// it more than once inside 600ms; a strict == 1 fails deterministically,
	// not flakily. UsersFailed/UsersDropped are not asserted at all here for
	// the same reason: a reclaim can legitimately race the ctx deadline.
	if report.UsersProcessed < 1 {
		t.Errorf("expected at least 1 user processed, got %+v", report)
	}
	if report.LeaguesProcessed < 2 {
		t.Errorf("expected both lg-existing and lg-new (discovered mid-run) processed, got %+v", report)
	}

	var newLeague models.SleeperLeague
	if err := db.First(&newLeague, "sleeper_league_id = ?", "lg-new").Error; err != nil {
		t.Fatalf("expected lg-new to have been discovered and claimed by the league pool: %v", err)
	}
	if newLeague.LastFetchedAt == nil {
		t.Error("expected lg-new's details to have been fetched")
	}
}

// TestRunDiscovery_AggregatesClaimErrorsFromBothPools uses the package's
// SQLite fixture deliberately, not TEST_DATABASE_URL: both claimStaleUsersSQL
// and claimStaleLeaguesSQL use Postgres-only syntax (now(), interval, FOR
// UPDATE SKIP LOCKED), so every claim attempt against SQLite fails outright
// — a cheap, deterministic way to force claim errors on both pools without a
// real unreachable-Postgres scenario, and it runs in every environment
// (no TEST_DATABASE_URL gate needed).
func TestRunDiscovery_AggregatesClaimErrorsFromBothPools(t *testing.T) {
	db := newSQLiteDB(t)

	cfg := discoverycron.Config{
		UserPoolSize: 1, UserRefillBatch: 1, LeaguePoolSize: 1, LeagueRefillBatch: 1,
		UserBatchSize: 10, LeagueBatchSize: 10,
		UserBatchFlushInterval: time.Hour, LeagueBatchFlushInterval: time.Hour,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	report, err := discoverycron.RunDiscovery(ctx, db, sleeper.New(), cfg)
	if err != nil {
		t.Fatalf("RunDiscovery error: %v", err)
	}

	if report.UserClaimErrors == 0 {
		t.Error("expected UserClaimErrors > 0 when the user claim query fails every attempt")
	}
	if report.LeagueClaimErrors == 0 {
		t.Error("expected LeagueClaimErrors > 0 when the league claim query fails every attempt")
	}
	if report.UsersProcessed != 0 || report.LeaguesProcessed != 0 {
		t.Errorf("expected nothing processed when claiming always fails, got %+v", report)
	}
}
