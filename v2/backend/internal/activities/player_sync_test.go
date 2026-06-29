package activities_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/sleeper"
)

func TestFetchAndUpsertAllPlayers_InsertsPlayers(t *testing.T) {
	db := newTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if err := psa.FetchAndUpsertAllPlayers(context.Background()); err != nil {
		t.Fatalf("FetchAndUpsertAllPlayers error: %v", err)
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
	if err := psa.FetchAndUpsertAllPlayers(context.Background()); err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if err := psa.FetchAndUpsertAllPlayers(context.Background()); err != nil {
		t.Fatalf("second run error: %v", err)
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
	if err := psa.FetchAndUpsertAllPlayers(context.Background()); err != nil {
		t.Fatalf("FetchAndUpsertAllPlayers error: %v", err)
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
	if err := psa1.FetchAndUpsertAllPlayers(context.Background()); err != nil {
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
	if err := psa2.FetchAndUpsertAllPlayers(context.Background()); err != nil {
		t.Fatalf("second run error: %v", err)
	}

	var p models.SleeperPlayer
	db.First(&p, "sleeper_player_id = ?", "4017")
	if p.Age != 28 {
		t.Errorf("expected age 28 after update, got %d", p.Age)
	}
}
