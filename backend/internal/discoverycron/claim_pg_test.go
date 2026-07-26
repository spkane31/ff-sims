package discoverycron_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"backend/internal/discoverycron"
	"backend/internal/models"
	"backend/internal/testutil"
)

func newPGTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; claim tests need Postgres (FOR UPDATE SKIP LOCKED)")
	}
	scopedDSN := testutil.NewPGSchema(t, dsn, "discoverycron_claim_test")
	db := testutil.OpenGORM(t, scopedDSN)
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

func TestClaimStaleLeagues_OrderingAndEligibility(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour)
	recent := now.Add(-1 * time.Hour)
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "never"})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "oldest", LastFetchedAt: &old})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "recent", LastFetchedAt: &recent})

	got, err := discoverycron.ClaimStaleLeagues(context.Background(), db, 2)
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
	db.Model(&models.SleeperLeague{}).Where("discovery_claimed_at IS NOT NULL").Count(&stamped)
	if stamped != 2 {
		t.Errorf("expected 2 rows stamped discovery_claimed_at, got %d", stamped)
	}
}

func TestClaimStaleLeagues_ExcludesIneligible(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "skipped", SkippedAt: &now})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "old-season", Season: "2024"})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "done-complete", Status: "complete", LastFetchedAt: &now})
	// complete but never actually detail-fetched: still eligible (matches
	// leagueFullySynced's own condition: complete AND last_fetched_at set).
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "complete-unfetched", Status: "complete"})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "in-season-fetched", Status: "in_season", LastFetchedAt: &now})

	got, err := discoverycron.ClaimStaleLeagues(context.Background(), db, 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	claimed := map[string]bool{}
	for _, id := range got {
		claimed[id] = true
	}
	for _, want := range []string{"complete-unfetched", "in-season-fetched"} {
		if !claimed[want] {
			t.Errorf("expected %s to be claimed", want)
		}
	}
	for _, no := range []string{"skipped", "old-season", "done-complete"} {
		if claimed[no] {
			t.Errorf("expected %s NOT to be claimed", no)
		}
	}
}

func TestClaimStaleLeagues_RespectsAndExpiresClaims(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	fresh := now.Add(-1 * time.Minute)
	stale := now.Add(-150 * time.Minute)
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "fresh-claim", DiscoveryClaimedAt: &fresh})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "expired-claim", DiscoveryClaimedAt: &stale})
	// A transactions claim must not block a discovery claim (separate columns).
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "txn-claimed", ClaimedAt: &fresh})

	got, err := discoverycron.ClaimStaleLeagues(context.Background(), db, 10)
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

func TestClaimStaleLeagues_ConcurrentClaimsAreDisjoint(t *testing.T) {
	db := newPGTestDB(t)
	for i := 0; i < 20; i++ {
		seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: fmt.Sprintf("lg%02d", i)})
	}

	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := discoverycron.ClaimStaleLeagues(context.Background(), db, 10)
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

	got, err := discoverycron.ClaimStaleUsers(context.Background(), db, 2)
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
	stale := now.Add(-150 * time.Minute)
	seedUser(t, db, models.SleeperUser{SleeperUserID: "fresh-claim", ClaimedAt: &fresh})
	seedUser(t, db, models.SleeperUser{SleeperUserID: "expired-claim", ClaimedAt: &stale})

	got, err := discoverycron.ClaimStaleUsers(context.Background(), db, 10)
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

	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := discoverycron.ClaimStaleUsers(context.Background(), db, 10)
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
