package activities_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/sleeper"
)

func TestSeasons_StartsAt2025AndIncludesCurrentYear(t *testing.T) {
	seasons := activities.Seasons()

	if len(seasons) == 0 {
		t.Fatal("expected at least one season")
	}
	if seasons[0] != "2025" {
		t.Errorf("expected seasons to start at 2025, got %q", seasons[0])
	}
	for _, s := range seasons {
		if s < "2025" {
			t.Errorf("seasons %v should not include a pre-2025 year, found %q", seasons, s)
		}
	}

	currentYear := strconv.Itoa(time.Now().Year())
	found := false
	for _, s := range seasons {
		if s == currentYear {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected seasons %v to include current year %q", seasons, currentYear)
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Each pooled connection to sqlite ":memory:" gets its own empty database;
	// pin the pool to one connection so concurrent test code (e.g. the batch
	// sync activity's goroutines) sees the migrated schema.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("unwrap sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&models.SleeperUser{},
		&models.SleeperLeague{},
		&models.SleeperLeagueUser{},
		&models.SleeperDraft{},
		&models.SleeperDraftPick{},
		&models.SleeperTransaction{},
		&models.SleeperPlayer{},
		&models.SleeperPlayerWeekStat{},
		&models.SleeperWeekStatFetch{},
		&models.DraftADP{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

// discoveryTestServer fakes the three discovery endpoints:
// /v1/user/{id}/leagues/nfl/{season}, /v1/league/{id}/users, /v1/league/{id}.
// userLeagues maps userID -> leagues (returned for every scanned season);
// missing user keys 404. members maps leagueID -> league users.
func discoveryTestServer(t *testing.T, userLeagues map[string][]sleeper.League, members map[string][]sleeper.LeagueUser) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case parts[1] == "user":
			leagues, ok := userLeagues[parts[2]]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(leagues)
		case parts[1] == "league" && strings.HasSuffix(r.URL.Path, "/users"):
			json.NewEncoder(w).Encode(members[parts[2]])
		case parts[1] == "league":
			// league details: echo a minimal league for the requested ID
			json.NewEncoder(w).Encode(sleeper.League{LeagueID: parts[2], Name: "L", Status: "in_season", TotalRosters: 12})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func runDiscoveryBatch(t *testing.T, da *activities.DiscoveryActivities, params activities.DiscoverUsersBatchParams) activities.SyncBatchResult {
	t.Helper()
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestActivityEnvironment()
	env.RegisterActivity(da.DiscoverUsersBatch)
	val, err := env.ExecuteActivity(da.DiscoverUsersBatch, params)
	if err != nil {
		t.Fatalf("discovery batch activity: %v", err)
	}
	var res activities.SyncBatchResult
	if err := val.Get(&res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return res
}

func claimedUser(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	now := time.Now().UTC()
	u := models.SleeperUser{SleeperUserID: id, ClaimedAt: &now}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func TestDiscoverUsersBatch_DiscoversAndStamps(t *testing.T) {
	db := newTestDB(t)
	claimedUser(t, db, "user1")

	srv := discoveryTestServer(t,
		map[string][]sleeper.League{
			"user1": {{LeagueID: "lg1", Name: "Test League", Season: "2026", Sport: "nfl", Status: "in_season"}},
		},
		map[string][]sleeper.LeagueUser{
			"lg1": {{UserID: "user1", Username: "me"}, {UserID: "u-new", Username: "newbie"}},
		})
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runDiscoveryBatch(t, da, activities.DiscoverUsersBatchParams{UserIDs: []string{"user1"}, Concurrency: 2})
	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("expected 1 processed / 0 failed, got %+v", res)
	}

	var u models.SleeperUser
	db.First(&u, "sleeper_user_id = ?", "user1")
	if u.LastFetchedAt == nil || u.ClaimedAt != nil {
		t.Errorf("user not stamped/unclaimed: %+v", u)
	}
	// Discovered member enters the queue with NULL last_fetched_at.
	var newbie models.SleeperUser
	db.First(&newbie, "sleeper_user_id = ?", "u-new")
	if newbie.LastFetchedAt != nil {
		t.Error("new member should have NULL last_fetched_at")
	}
	// League upserted with details populated.
	var lg models.SleeperLeague
	db.First(&lg, "sleeper_league_id = ?", "lg1")
	if lg.LastFetchedAt == nil {
		t.Errorf("league details not stamped: %+v", lg)
	}
}

func TestDiscoverUsersBatch_User404MarksSkipped(t *testing.T) {
	db := newTestDB(t)
	claimedUser(t, db, "gone")

	srv := discoveryTestServer(t, map[string][]sleeper.League{}, nil) // every user 404s
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runDiscoveryBatch(t, da, activities.DiscoverUsersBatchParams{UserIDs: []string{"gone"}, Concurrency: 1})
	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("expected skip to count as processed, got %+v", res)
	}
	var u models.SleeperUser
	db.First(&u, "sleeper_user_id = ?", "gone")
	if u.SkippedAt == nil || u.ClaimedAt != nil {
		t.Errorf("user should be skipped and unclaimed: %+v", u)
	}
	if u.LastFetchedAt != nil {
		t.Errorf("skipped user must not be stamped fetched: %+v", u)
	}
}

func TestDiscoverUsersBatch_LeagueFailuresContinue(t *testing.T) {
	db := newTestDB(t)
	claimedUser(t, db, "user1")

	// User's leagues resolve, but every league-level endpoint fails with a
	// non-retryable 400: discovery must warn, continue, and still stamp the user.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if parts[1] == "user" {
			json.NewEncoder(w).Encode([]sleeper.League{{LeagueID: "lg1", Season: "2026", Sport: "nfl"}})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runDiscoveryBatch(t, da, activities.DiscoverUsersBatchParams{UserIDs: []string{"user1"}, Concurrency: 1})
	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("expected league failures to be tolerated, got %+v", res)
	}
	var u models.SleeperUser
	db.First(&u, "sleeper_user_id = ?", "user1")
	if u.LastFetchedAt == nil || u.ClaimedAt != nil {
		t.Errorf("user must be stamped despite league failures: %+v", u)
	}
}

func TestDiscoverUsersBatch_RetrySkipsAlreadyStampedUsers(t *testing.T) {
	db := newTestDB(t)
	// u1 was stamped by a previous attempt (claim cleared); u2 still claimed.
	now := time.Now().UTC()
	db.Create(&models.SleeperUser{SleeperUserID: "u1", LastFetchedAt: &now})
	claimedUser(t, db, "u2")

	srv := discoveryTestServer(t, map[string][]sleeper.League{"u1": {}, "u2": {}}, nil)
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runDiscoveryBatch(t, da, activities.DiscoverUsersBatchParams{UserIDs: []string{"u1", "u2"}, Concurrency: 1})
	if res.Processed != 1 {
		t.Fatalf("expected only still-claimed u2 processed, got %+v", res)
	}
}

// TestDiscoverUsersBatch_PerUserTimeoutDoesNotStallOtherUsers is the
// regression test for the production incident: DiscoverUsersBatch's
// wg.Wait blocks until every user in the batch finishes, so one user stuck
// behind a slow/hanging Sleeper response used to drag the whole batch (and
// eventually the whole activity, StartToCloseTimeout and all) down with it.
// Each user now gets its own UserTimeoutSeconds sub-context, so a stuck user
// times out and re-queues via claim expiry without blocking the rest.
func TestDiscoverUsersBatch_PerUserTimeoutDoesNotStallOtherUsers(t *testing.T) {
	db := newTestDB(t)
	claimedUser(t, db, "slow")
	claimedUser(t, db, "fast")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case parts[1] == "user" && parts[2] == "slow":
			// Outlast the 1s per-user timeout used below; the request context
			// should be cancelled out from under this handler well before it
			// would otherwise respond.
			select {
			case <-time.After(2 * time.Second):
			case <-r.Context().Done():
			}
		case parts[1] == "user" && parts[2] == "fast":
			json.NewEncoder(w).Encode([]sleeper.League{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	start := time.Now()
	res := runDiscoveryBatch(t, da, activities.DiscoverUsersBatchParams{
		UserIDs:            []string{"slow", "fast"},
		Concurrency:        2,
		UserTimeoutSeconds: 1,
	})
	elapsed := time.Since(start)

	if res.Processed != 1 || res.Failed != 1 {
		t.Fatalf("expected the fast user to succeed and the slow user to time out, got %+v", res)
	}
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("batch took %s; the slow user's per-user timeout should bound the whole batch near 1s, not the full 2s server delay", elapsed)
	}

	var fast models.SleeperUser
	db.First(&fast, "sleeper_user_id = ?", "fast")
	if fast.LastFetchedAt == nil || fast.ClaimedAt != nil {
		t.Errorf("fast user should be stamped done despite the slow user's timeout: %+v", fast)
	}
	var slow models.SleeperUser
	db.First(&slow, "sleeper_user_id = ?", "slow")
	if slow.ClaimedAt == nil {
		t.Errorf("slow user should remain claimed so it re-queues via claim expiry: %+v", slow)
	}
}

// TestDiscoverOneUser_StopsProcessingLeaguesAfterContextCancelled is the
// regression test for the log-spam side effect: once ctx is done, discovery
// must not keep looping through the rest of leagueIDs, each of which would
// otherwise fail instantly on ctx.Err() and flood the log (this is what
// produced the 1,000+ near-simultaneous WARN lines seen in production once
// an activity's StartToCloseTimeout fired).
func TestDiscoverOneUser_StopsProcessingLeaguesAfterContextCancelled(t *testing.T) {
	db := newTestDB(t)
	claimedUser(t, db, "user1")

	var mu sync.Mutex
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if parts[1] == "user" {
			json.NewEncoder(w).Encode([]sleeper.League{
				{LeagueID: "lg1", Season: "2026", Sport: "nfl"},
				{LeagueID: "lg2", Season: "2026", Sport: "nfl"},
				{LeagueID: "lg3", Season: "2026", Sport: "nfl"},
			})
			return
		}
		mu.Lock()
		hits = append(hits, r.URL.Path)
		mu.Unlock()
		if parts[2] == "lg1" {
			// Outlast the per-user timeout so ctx is already done both when
			// this handler would respond and when lg2/lg3 would be reached.
			select {
			case <-time.After(2 * time.Second):
			case <-r.Context().Done():
			}
		}
		json.NewEncoder(w).Encode(sleeper.League{LeagueID: parts[2]})
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runDiscoveryBatch(t, da, activities.DiscoverUsersBatchParams{
		UserIDs:            []string{"user1"},
		Concurrency:        1,
		UserTimeoutSeconds: 1,
	})
	if res.Failed != 1 {
		t.Fatalf("expected the user to fail via per-user timeout, got %+v", res)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, h := range hits {
		if strings.Contains(h, "lg2") || strings.Contains(h, "lg3") {
			t.Errorf("expected lg2/lg3 to be skipped once the context was cancelled, but saw a request to %s", h)
		}
	}
}

// TestDiscoverOneUser_FansOutLeagueFetchesConcurrently is the regression test
// for the "mega-user" incident: a Sleeper user can belong to hundreds of
// leagues, and fetching them one at a time (2 sequential calls per league)
// made such a user structurally unable to finish within any reasonable
// per-user timeout — and since ClaimStaleUsers orders never-fetched users
// first, a user that can never finish permanently squats at the head of the
// discovery queue. League fetches within a single user now fan out with
// bounded concurrency instead of running strictly one at a time.
func TestDiscoverOneUser_FansOutLeagueFetchesConcurrently(t *testing.T) {
	db := newTestDB(t)
	claimedUser(t, db, "poweruser")

	const numLeagues = 30
	const perCallDelay = 150 * time.Millisecond

	leagues := make([]sleeper.League, numLeagues)
	for i := range leagues {
		leagues[i] = sleeper.League{LeagueID: strconv.Itoa(i), Season: "2026", Sport: "nfl"}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if parts[1] == "user" {
			json.NewEncoder(w).Encode(leagues)
			return
		}
		time.Sleep(perCallDelay)
		if strings.HasSuffix(r.URL.Path, "/users") {
			json.NewEncoder(w).Encode([]sleeper.LeagueUser{})
			return
		}
		json.NewEncoder(w).Encode(sleeper.League{LeagueID: parts[2]})
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	start := time.Now()
	res := runDiscoveryBatch(t, da, activities.DiscoverUsersBatchParams{
		UserIDs:            []string{"poweruser"},
		Concurrency:        1,
		UserTimeoutSeconds: 30,
		LeagueConcurrency:  10,
	})
	elapsed := time.Since(start)

	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("expected the user to complete despite the large league count, got %+v", res)
	}
	// Sequential would take numLeagues * 2 * perCallDelay = 9s. At
	// LeagueConcurrency 10, ~3 rounds of concurrent (members + details)
	// pairs should land well under half that.
	if elapsed > 4*time.Second {
		t.Fatalf("discovery took %s; expected league fetches to fan out concurrently instead of running one at a time", elapsed)
	}

	var u models.SleeperUser
	db.First(&u, "sleeper_user_id = ?", "poweruser")
	if u.LastFetchedAt == nil || u.ClaimedAt != nil {
		t.Errorf("power user should be fully stamped done: %+v", u)
	}
}

// TestDiscoverOneUser_SkipsMembersForFullySyncedLeague extends the existing
// FetchLeagueDetails completed-league skip to FetchLeagueMembers too: a
// league marked complete with details already fetched has immutable
// membership, so re-fetching it on every discovery pass (for every one of
// its members, every time any of them comes up for rediscovery) was pure
// waste — and for mega-users, it's exactly the redundant work that made
// them unable to ever finish.
func TestDiscoverOneUser_SkipsMembersForFullySyncedLeague(t *testing.T) {
	db := newTestDB(t)
	claimedUser(t, db, "user1")
	past := time.Now().Add(-24 * time.Hour)
	db.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg-done",
		Status:          "complete",
		LastFetchedAt:   &past,
	})

	var membersHit, detailsHit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if parts[1] == "user" {
			json.NewEncoder(w).Encode([]sleeper.League{{LeagueID: "lg-done", Season: "2026", Sport: "nfl"}})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/users") {
			membersHit = true
			json.NewEncoder(w).Encode([]sleeper.LeagueUser{})
			return
		}
		detailsHit = true
		json.NewEncoder(w).Encode(sleeper.League{LeagueID: "lg-done"})
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runDiscoveryBatch(t, da, activities.DiscoverUsersBatchParams{
		UserIDs: []string{"user1"}, Concurrency: 1, UserTimeoutSeconds: 5, LeagueConcurrency: 5,
	})
	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("expected user to complete, got %+v", res)
	}
	if membersHit {
		t.Error("FetchLeagueMembers should be skipped for an already complete+fetched league")
	}
	if detailsHit {
		t.Error("FetchLeagueDetails should be skipped for an already complete+fetched league")
	}
}

func TestFetchUserLeagues_UpsertsLeagues(t *testing.T) {
	db := newTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]sleeper.League{
			{LeagueID: "lg1", Name: "Test League", Season: "2024", Sport: "nfl", Status: "complete"},
		})
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{
		DB:      db,
		Sleeper: sleeper.NewWithBaseURL(srv.URL),
	}

	leagueIDs, err := da.FetchUserLeagues(context.Background(), activities.FetchUserLeaguesParams{UserID: "user1"})
	if err != nil {
		t.Fatalf("FetchUserLeagues error: %v", err)
	}
	// one league returned per scanned season, deduped to a single "lg1" row in DB
	if len(leagueIDs) == 0 {
		t.Fatal("expected at least one leagueID")
	}

	var count int64
	db.Model(&models.SleeperLeague{}).Where("sleeper_league_id = ?", "lg1").Count(&count)
	if count != 1 {
		t.Errorf("expected 1 league row (upserted), got %d", count)
	}

	// Junction row should exist
	var jcount int64
	db.Model(&models.SleeperLeagueUser{}).
		Where("sleeper_league_id = ? AND sleeper_user_id = ?", "lg1", "user1").
		Count(&jcount)
	if jcount != 1 {
		t.Errorf("expected 1 junction row, got %d", jcount)
	}
}

func TestFetchLeagueMembers_InsertsUsers(t *testing.T) {
	db := newTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]sleeper.LeagueUser{
			{UserID: "u1", Username: "alice", DisplayName: "Alice"},
			{UserID: "u2", Username: "bob", DisplayName: "Bob"},
		})
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{
		DB:      db,
		Sleeper: sleeper.NewWithBaseURL(srv.URL),
	}

	if err := da.FetchLeagueMembers(context.Background(), activities.FetchLeagueMembersParams{LeagueID: "lg1"}); err != nil {
		t.Fatalf("FetchLeagueMembers error: %v", err)
	}

	var count int64
	db.Model(&models.SleeperUser{}).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 users, got %d", count)
	}

	// New users should have NULL last_fetched_at (picked up by future runs)
	var u models.SleeperUser
	db.First(&u, "sleeper_user_id = ?", "u1")
	if u.LastFetchedAt != nil {
		t.Error("new user should have NULL last_fetched_at")
	}
}

func TestFetchLeagueDetails_Discovery_SetsScoring(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "lg1"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sleeper.League{
			LeagueID:        "lg1",
			Name:            "My League",
			Status:          "complete",
			TotalRosters:    12,
			ScoringSettings: map[string]float64{"rec": 0.5, "bonus_rec_te": 0.5},
			RosterPositions: []string{"QB", "WR", "SUPER_FLEX", "BN"},
		})
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := da.FetchLeagueDetails(context.Background(), activities.FetchLeagueDetailsParams{LeagueID: "lg1"}); err != nil {
		t.Fatalf("FetchLeagueDetails error: %v", err)
	}

	var l models.SleeperLeague
	db.First(&l, "sleeper_league_id = ?", "lg1")
	if l.PPR == nil || *l.PPR != 0.5 {
		t.Errorf("expected PPR 0.5, got %v", l.PPR)
	}
	if l.IsSuperflex == nil || !*l.IsSuperflex {
		t.Error("expected is_superflex = true")
	}
	if l.LastFetchedAt == nil {
		t.Error("expected last_fetched_at to be stamped")
	}
}

func TestFetchLeagueDetails_Discovery_NotFound(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperLeague{SleeperLeagueID: "gone"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	err := da.FetchLeagueDetails(context.Background(), activities.FetchLeagueDetailsParams{LeagueID: "gone"})
	if err == nil {
		t.Fatal("expected NOT_FOUND error")
	}
}

func TestFetchLeagueDetails_SkipsCompletedLeague(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	db.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg-done",
		Status:          "complete",
		LastFetchedAt:   &now,
	})

	apiCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		json.NewEncoder(w).Encode(sleeper.League{})
	}))
	defer srv.Close()

	da := &activities.DiscoveryActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := da.FetchLeagueDetails(context.Background(), activities.FetchLeagueDetailsParams{LeagueID: "lg-done"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if apiCalled {
		t.Error("Sleeper API should not be called for a completed league")
	}
}
