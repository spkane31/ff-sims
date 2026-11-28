package activities_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/playerstatus"
	"backend/internal/sleeper"
)

func TestFetchAndUpsertAllPlayers_InsertsPlayers(t *testing.T) {
	db := newTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/state/nfl" {
			fmt.Fprint(w, `{"season":"2026","season_type":"regular","week":13}`)
			return
		}
		if r.URL.Path != "/v1/players/nfl" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		payload := map[string]sleeper.Player{
			"4017": {FullName: "Josh Allen", Position: "QB", Team: "BUF", EspnID: "3054211", Age: 28, YearsExp: 7},
			"4663": {FullName: "Ja'Marr Chase", Position: "WR", Team: "CIN", EspnID: "4372016", Age: 24, YearsExp: 4},
		}
		json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	psa := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result, err := psa.FetchAndUpsertAllPlayers(context.Background())
	if err != nil {
		t.Fatalf("FetchAndUpsertAllPlayers error: %v", err)
	}
	if result.PlayersUpserted != 2 {
		t.Errorf("expected PlayersUpserted 2, got %d", result.PlayersUpserted)
	}

	var count int64
	db.Model(&models.SleeperPlayer{}).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 players, got %d", count)
	}

	var p models.SleeperPlayer
	db.First(&p, "sleeper_player_id = ?", "4017")
	if p.EspnID != "3054211" {
		t.Errorf("expected espn_id 3054211, got %q", p.EspnID)
	}
	if p.FullName != "Josh Allen" {
		t.Errorf("expected full_name 'Josh Allen', got %q", p.FullName)
	}
}

func TestFetchAndUpsertAllPlayers_Idempotent(t *testing.T) {
	db := newTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]sleeper.Player{
			"4017": {FullName: "Josh Allen", Position: "QB", Team: "BUF"},
		})
	}))
	defer srv.Close()

	psa := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result1, err := psa.FetchAndUpsertAllPlayers(context.Background())
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if result1.PlayersUpserted != 1 {
		t.Errorf("expected first run PlayersUpserted 1, got %d", result1.PlayersUpserted)
	}
	result2, err := psa.FetchAndUpsertAllPlayers(context.Background())
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if result2.PlayersUpserted != 1 {
		t.Errorf("expected second run PlayersUpserted 1, got %d", result2.PlayersUpserted)
	}

	var count int64
	db.Model(&models.SleeperPlayer{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 player after 2 runs, got %d", count)
	}
}

func TestFetchAndUpsertAllPlayers_NumericYahooAndEspnID(t *testing.T) {
	db := newTestDB(t)

	// Simulate Sleeper returning yahoo_id and espn_id as bare JSON numbers,
	// which it does inconsistently for some players.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"4017":{"full_name":"Josh Allen","position":"QB","team":"BUF","espn_id":3054211,"yahoo_id":30942,"age":28,"years_exp":7}}`)
	}))
	defer srv.Close()

	psa := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result, err := psa.FetchAndUpsertAllPlayers(context.Background())
	if err != nil {
		t.Fatalf("FetchAndUpsertAllPlayers error: %v", err)
	}
	if result.PlayersUpserted != 1 {
		t.Errorf("expected PlayersUpserted 1, got %d", result.PlayersUpserted)
	}

	var p models.SleeperPlayer
	db.First(&p, "sleeper_player_id = ?", "4017")
	if p.YahooID != "30942" {
		t.Errorf("expected yahoo_id '30942', got %q", p.YahooID)
	}
	if p.EspnID != "3054211" {
		t.Errorf("expected espn_id '3054211', got %q", p.EspnID)
	}
}

func TestFetchAndUpsertAllPlayers_UpdatesExisting(t *testing.T) {
	db := newTestDB(t)

	// First sync with age=27
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]sleeper.Player{
			"4017": {FullName: "Josh Allen", Position: "QB", Team: "BUF", Age: 27},
		})
	}))
	defer srv1.Close()

	psa1 := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv1.URL)}
	if _, err := psa1.FetchAndUpsertAllPlayers(context.Background()); err != nil {
		t.Fatalf("first run error: %v", err)
	}

	// Second sync with age=28 (simulating next year)
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]sleeper.Player{
			"4017": {FullName: "Josh Allen", Position: "QB", Team: "BUF", Age: 28},
		})
	}))
	defer srv2.Close()

	psa2 := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv2.URL)}
	result, err := psa2.FetchAndUpsertAllPlayers(context.Background())
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if result.PlayersUpserted != 1 {
		t.Errorf("expected PlayersUpserted 1, got %d", result.PlayersUpserted)
	}

	var p models.SleeperPlayer
	db.First(&p, "sleeper_player_id = ?", "4017")
	if p.Age != 28 {
		t.Errorf("expected age 28 after update, got %d", p.Age)
	}
}

// bulkWithStatus is a /v1/players/nfl response covering a healthy starter
// (null injury_status), an IR player, a free agent who is still active, a
// retired player with no team, and a non-fantasy position.
const bulkWithStatus = `{
  "4017": {"full_name":"Josh Allen","position":"QB","team":"BUF","active":true,"status":"Active","injury_status":null,"news_updated":1764000000000},
  "4866": {"full_name":"D'Andre Swift","position":"RB","team":"CHI","active":true,"status":"Injured Reserve","injury_status":"IR","injury_start_date":"2026-11-02","news_updated":"1763000000000"},
  "5000": {"full_name":"Free Agent","position":"WR","team":null,"active":true,"status":"Active"},
  "1001": {"full_name":"Retired Guy","position":"QB","team":null,"active":false,"status":"Inactive"},
  "2002": {"full_name":"Linebacker","position":"LB","team":"DAL","active":true,"injury_status":"Out"}
}`

func TestFetchAndUpsertAllPlayers_SnapshotsStatusFromSameFetch(t *testing.T) {
	db := newTestDB(t)
	bulkHits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/players/nfl":
			bulkHits++
			fmt.Fprint(w, bulkWithStatus)
		case "/v1/state/nfl":
			fmt.Fprint(w, `{"season":"2026","season_type":"regular","week":13}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	psa := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result, err := psa.FetchAndUpsertAllPlayers(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, bulkHits, "status snapshot must reuse the bulk response")
	require.Equal(t, 5, result.PlayersUpserted)

	st := result.Status
	require.Empty(t, st.WriteError)
	require.Equal(t, playerstatus.SourcePlayersBulk, st.Source)
	require.Equal(t, 3, st.Observations, "retired and non-fantasy players are skipped")
	require.Equal(t, 3, st.ApplicabilityUnknown)
	require.Nil(t, st.PreviousCaptureAt)

	got, err := playerstatus.AsOf(context.Background(), db, st.ObservedAt, playerstatus.SourcePlayersBulk,
		[]string{"4017", "4866", "5000", "1001", "2002"})
	require.NoError(t, err)
	require.Len(t, got, 3)

	allen := got["4017"]
	require.Equal(t, st.CaptureID, allen.CaptureID)
	require.Equal(t, "2026", *allen.ObservedSeason)
	require.Equal(t, 13, *allen.ObservedWeek)
	require.Equal(t, "BUF", *allen.NflTeam)
	require.Equal(t, "Active", *allen.RosterStatus)
	require.Nil(t, allen.InjuryStatus, "null injury_status must not be stored as healthy")
	require.Equal(t, int64(1764000000000), *allen.NewsUpdatedMs)

	swift := got["4866"]
	require.Equal(t, "IR", *swift.InjuryStatus)
	require.Equal(t, "2026-11-02", *swift.InjuryStartDate)
	require.Equal(t, int64(1763000000000), *swift.NewsUpdatedMs)

	require.Nil(t, got["5000"].NflTeam, "free agent keeps no team so as-of does not return a stale one")
}

// A failed status write is reported in the result instead of failing the
// activity, because a retry would refetch the once-a-day bulk feed and the
// player upsert already succeeded.
func TestFetchAndUpsertAllPlayers_StatusWriteFailureReported(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Migrator().DropTable(&models.SleeperPlayerStatusObservation{}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/state/nfl" {
			http.Error(w, "boom", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, bulkWithStatus)
	}))
	defer srv.Close()

	psa := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result, err := psa.FetchAndUpsertAllPlayers(context.Background())
	require.NoError(t, err)
	require.Equal(t, 5, result.PlayersUpserted)
	require.NotEmpty(t, result.Status.WriteError)
	require.Zero(t, result.Status.Observations)
	require.Len(t, result.Status.Warnings, 1, "unknown observed week is reported")

	var captures int64
	require.NoError(t, db.Model(&models.SleeperStatusCapture{}).Count(&captures).Error)
	require.Zero(t, captures, "capture row rolls back with the failed observation insert")
}
