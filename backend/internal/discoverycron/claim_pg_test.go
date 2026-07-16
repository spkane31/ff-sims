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
	if err := db.AutoMigrate(&models.SleeperLeague{}); err != nil {
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
