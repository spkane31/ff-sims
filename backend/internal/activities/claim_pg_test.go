package activities_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/activities"
	"backend/internal/models"
)

// newPGTestDB opens TEST_DATABASE_URL inside a fresh throwaway schema and
// migrates SleeperLeague into it. Skips the test when the env var is unset —
// claim queries use FOR UPDATE SKIP LOCKED, which SQLite cannot express.
func newPGTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; claim tests need Postgres (FOR UPDATE SKIP LOCKED)")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	schema := fmt.Sprintf("claim_test_%d", rand.Int63())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		sqlDB, _ := admin.DB()
		sqlDB.Close()
	})

	// search_path must ride in the DSN (not a session SET) so that every
	// pooled connection — e.g. those used by concurrent claimers — sees the
	// test schema.
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err := gorm.Open(postgres.Open(dsn+sep+"search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open postgres (schema-scoped): %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	})
	if err := db.AutoMigrate(&models.SleeperLeague{}, &models.SleeperUser{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func seedLeague(t *testing.T, db *gorm.DB, l models.SleeperLeague) {
	t.Helper()
	if l.Season == "" {
		l.Season = "2026"
	}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed league %s: %v", l.SleeperLeagueID, err)
	}
}

func TestClaimLeagues_OrderingLimitAndStamp(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour)
	recent := now.Add(-1 * time.Hour)
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "never", LastFetchedAt: &now})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "oldest", LastFetchedAt: &now, LastTransactionsFetchedAt: &old})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "recent", LastFetchedAt: &now, LastTransactionsFetchedAt: &recent})

	a := &activities.DataFetchActivities{DB: db}
	got, err := a.ClaimLeaguesForTransactions(context.Background(), activities.ClaimLeaguesForTransactionsParams{BatchSize: 2})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	// Which leagues get claimed is what matters (RETURNING order is not
	// guaranteed): batch of 2 must be the never-fetched and the oldest.
	claimed := map[string]bool{}
	for _, s := range got {
		claimed[s.LeagueID] = true
		if s.Season != "2026" {
			t.Errorf("expected Season populated, got %+v", s)
		}
	}
	if len(got) != 2 || !claimed["never"] || !claimed["oldest"] {
		t.Fatalf("expected {never, oldest}, got %+v", got)
	}
	var stamped int64
	db.Model(&models.SleeperLeague{}).Where("claimed_at IS NOT NULL").Count(&stamped)
	if stamped != 2 {
		t.Errorf("expected 2 rows stamped claimed_at, got %d", stamped)
	}
}

func TestClaimLeagues_ExcludesIneligible(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "skipped", LastFetchedAt: &now, SkippedAt: &now})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "unfetched"}) // last_fetched_at NULL
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "old-season", Season: "2024", LastFetchedAt: &now})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "done-complete", Status: "complete", LastFetchedAt: &now, LastTransactionsFetchedAt: &now})
	// complete but never transaction-synced: still eligible
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "complete-unsynced", Status: "complete", LastFetchedAt: &now})

	a := &activities.DataFetchActivities{DB: db}
	got, err := a.ClaimLeaguesForTransactions(context.Background(), activities.ClaimLeaguesForTransactionsParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(got) != 1 || got[0].LeagueID != "complete-unsynced" {
		t.Fatalf("expected only complete-unsynced, got %+v", got)
	}
}

func TestClaimLeagues_RespectsAndExpiresClaims(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	fresh := now.Add(-1 * time.Minute)
	stale := now.Add(-30 * time.Minute)
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "fresh-claim", LastFetchedAt: &now, ClaimedAt: &fresh})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "expired-claim", LastFetchedAt: &now, ClaimedAt: &stale})

	a := &activities.DataFetchActivities{DB: db}
	got, err := a.ClaimLeaguesForTransactions(context.Background(), activities.ClaimLeaguesForTransactionsParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(got) != 1 || got[0].LeagueID != "expired-claim" {
		t.Fatalf("expected only expired-claim to be re-claimable, got %+v", got)
	}
}

func TestClaimLeagues_ConcurrentClaimsAreDisjoint(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	for i := 0; i < 20; i++ {
		seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: fmt.Sprintf("lg%02d", i), LastFetchedAt: &now})
	}

	a := &activities.DataFetchActivities{DB: db}
	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := a.ClaimLeaguesForTransactions(context.Background(), activities.ClaimLeaguesForTransactionsParams{BatchSize: 10})
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, s := range got {
				seen[s.LeagueID]++
			}
		}()
	}
	wg.Wait()
	if len(seen) != 20 {
		t.Errorf("expected 20 distinct leagues claimed, got %d", len(seen))
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("league %s claimed %d times", id, n)
		}
	}
}

func TestClaimLeaguesForDrafts_Eligibility(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour)
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "never", Status: "pre_draft", LastFetchedAt: &now})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "predraft-fetched", Status: "pre_draft", LastFetchedAt: &now, LastDraftsFetchedAt: &old})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "inseason-fetched", Status: "in_season", LastFetchedAt: &now, LastDraftsFetchedAt: &old})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "complete-fetched", Status: "complete", LastFetchedAt: &now, LastDraftsFetchedAt: &old})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "inseason-never", Status: "in_season", LastFetchedAt: &now})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "skipped", Status: "pre_draft", LastFetchedAt: &now, SkippedAt: &now})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "old-season", Season: "2024", Status: "pre_draft", LastFetchedAt: &now})

	a := &activities.DataFetchActivities{DB: db}
	got, err := a.ClaimLeaguesForDrafts(context.Background(), activities.ClaimLeaguesForDraftsParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	claimed := map[string]bool{}
	for _, id := range got {
		claimed[id] = true
	}
	// Eligible: never-fetched (any status), and pre_draft even when previously
	// fetched (drafts can still change until they complete).
	for _, want := range []string{"never", "predraft-fetched", "inseason-never"} {
		if !claimed[want] {
			t.Errorf("expected %s to be claimed", want)
		}
	}
	// Ineligible: drafting finished and already fetched, skipped, old seasons.
	for _, no := range []string{"inseason-fetched", "complete-fetched", "skipped", "old-season"} {
		if claimed[no] {
			t.Errorf("expected %s NOT to be claimed", no)
		}
	}
	var stamped int64
	db.Model(&models.SleeperLeague{}).Where("drafts_claimed_at IS NOT NULL").Count(&stamped)
	if int(stamped) != len(got) {
		t.Errorf("expected %d rows stamped drafts_claimed_at, got %d", len(got), stamped)
	}
}

func TestClaimLeaguesForDrafts_RespectsAndExpiresClaims(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	fresh := now.Add(-1 * time.Minute)
	stale := now.Add(-30 * time.Minute)
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "fresh-claim", Status: "pre_draft", LastFetchedAt: &now, DraftsClaimedAt: &fresh})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "expired-claim", Status: "pre_draft", LastFetchedAt: &now, DraftsClaimedAt: &stale})
	// A transactions claim must not block a drafts claim (separate columns).
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "txn-claimed", Status: "pre_draft", LastFetchedAt: &now, ClaimedAt: &fresh})

	a := &activities.DataFetchActivities{DB: db}
	got, err := a.ClaimLeaguesForDrafts(context.Background(), activities.ClaimLeaguesForDraftsParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	claimed := map[string]bool{}
	for _, id := range got {
		claimed[id] = true
	}
	if len(got) != 2 || !claimed["expired-claim"] || !claimed["txn-claimed"] {
		t.Fatalf("expected {expired-claim, txn-claimed}, got %v", got)
	}
}

func TestClaimLeaguesForDrafts_ConcurrentClaimsAreDisjoint(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	for i := 0; i < 20; i++ {
		seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: fmt.Sprintf("dlg%02d", i), Status: "pre_draft", LastFetchedAt: &now})
	}

	a := &activities.DataFetchActivities{DB: db}
	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := a.ClaimLeaguesForDrafts(context.Background(), activities.ClaimLeaguesForDraftsParams{BatchSize: 10})
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range got {
				seen[id]++
			}
		}()
	}
	wg.Wait()
	if len(seen) != 20 {
		t.Errorf("expected 20 distinct leagues claimed, got %d", len(seen))
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("league %s claimed %d times", id, n)
		}
	}
}

func seedUser(t *testing.T, db *gorm.DB, u models.SleeperUser) {
	t.Helper()
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("seed user %s: %v", u.SleeperUserID, err)
	}
}

func TestClaimStaleUsers_OrderingAndEligibility(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour)
	recent := now.Add(-1 * time.Hour)
	seedUser(t, db, models.SleeperUser{SleeperUserID: "never"})
	seedUser(t, db, models.SleeperUser{SleeperUserID: "oldest", LastFetchedAt: &old})
	seedUser(t, db, models.SleeperUser{SleeperUserID: "recent", LastFetchedAt: &recent})
	seedUser(t, db, models.SleeperUser{SleeperUserID: "skipped", SkippedAt: &now})

	a := &activities.DiscoveryActivities{DB: db}
	got, err := a.ClaimStaleUsers(context.Background(), activities.ClaimStaleUsersParams{BatchSize: 2})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	claimed := map[string]bool{}
	for _, id := range got {
		claimed[id] = true
	}
	if len(got) != 2 || !claimed["never"] || !claimed["oldest"] {
		t.Fatalf("expected {never, oldest}, got %v", got)
	}
	var stamped int64
	db.Model(&models.SleeperUser{}).Where("claimed_at IS NOT NULL").Count(&stamped)
	if stamped != 2 {
		t.Errorf("expected 2 users stamped claimed_at, got %d", stamped)
	}
}

func TestClaimStaleUsers_RespectsAndExpiresClaims(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	fresh := now.Add(-1 * time.Minute)
	stale := now.Add(-30 * time.Minute)
	seedUser(t, db, models.SleeperUser{SleeperUserID: "fresh-claim", ClaimedAt: &fresh})
	seedUser(t, db, models.SleeperUser{SleeperUserID: "expired-claim", ClaimedAt: &stale})

	a := &activities.DiscoveryActivities{DB: db}
	got, err := a.ClaimStaleUsers(context.Background(), activities.ClaimStaleUsersParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(got) != 1 || got[0] != "expired-claim" {
		t.Fatalf("expected only expired-claim to be re-claimable, got %v", got)
	}
}

func TestClaimStaleUsers_ConcurrentClaimsAreDisjoint(t *testing.T) {
	db := newPGTestDB(t)
	for i := 0; i < 20; i++ {
		seedUser(t, db, models.SleeperUser{SleeperUserID: fmt.Sprintf("u%02d", i)})
	}

	a := &activities.DiscoveryActivities{DB: db}
	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := a.ClaimStaleUsers(context.Background(), activities.ClaimStaleUsersParams{BatchSize: 10})
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range got {
				seen[id]++
			}
		}()
	}
	wg.Wait()
	if len(seen) != 20 {
		t.Errorf("expected 20 distinct users claimed, got %d", len(seen))
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("user %s claimed %d times", id, n)
		}
	}
}
