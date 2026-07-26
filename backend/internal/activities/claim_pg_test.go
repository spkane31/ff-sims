package activities_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/testutil"
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
	scopedDSN := testutil.NewPGSchema(t, dsn, "claim_test")
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
	if l.LeagueType == "" {
		l.LeagueType = "redraft"
	}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed league %s: %v", l.SleeperLeagueID, err)
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

func TestClaimLeaguesForDrafts_ExcludesNonRedraftLeagues(t *testing.T) {
	db := newPGTestDB(t)
	now := time.Now().UTC()
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "redraft-lg", Status: "pre_draft", LastFetchedAt: &now, LeagueType: "redraft"})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "keeper-lg", Status: "pre_draft", LastFetchedAt: &now, LeagueType: "keeper"})
	seedLeague(t, db, models.SleeperLeague{SleeperLeagueID: "dynasty-lg", Status: "pre_draft", LastFetchedAt: &now, LeagueType: "dynasty"})

	a := &activities.DataFetchActivities{DB: db}
	got, err := a.ClaimLeaguesForDrafts(context.Background(), activities.ClaimLeaguesForDraftsParams{BatchSize: 10})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	claimed := map[string]bool{}
	for _, id := range got {
		claimed[id] = true
	}
	if !claimed["redraft-lg"] {
		t.Error("expected redraft-lg to be claimed")
	}
	if claimed["keeper-lg"] || claimed["dynasty-lg"] {
		t.Errorf("expected keeper/dynasty leagues NOT to be claimed, got %v", got)
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
