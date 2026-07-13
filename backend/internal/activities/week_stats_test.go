package activities_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/sleeper"
)

func weekStatsServer(t *testing.T, statsBody string, nflWeek int, nflSeason string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/state/nfl":
			json.NewEncoder(w).Encode(sleeper.NFLState{Season: nflSeason, SeasonType: "regular", Week: nflWeek})
		default:
			if statsBody == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Write([]byte(statsBody))
		}
	}))
}

func TestFetchWeekStats_FiltersToFantasyPositionsAndUpserts(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperPlayer{SleeperPlayerID: "421", Position: "RB"})
	db.Create(&models.SleeperPlayer{SleeperPlayerID: "999", Position: "DL"}) // not fantasy-relevant
	// "555" is absent from sleeper_players entirely — must be skipped too.

	body := `{"421":{"pts_ppr":24.06,"pts_half_ppr":20.56,"pts_std":17.06},"999":{"pts_ppr":5},"555":{"pts_ppr":3}}`
	srv := weekStatsServer(t, body, 10, "2025")
	defer srv.Close()

	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result, err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 3})
	if err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}
	if result.PlayersUpserted != 1 {
		t.Errorf("expected PlayersUpserted 1, got %d", result.PlayersUpserted)
	}
	if !result.Finalized {
		t.Errorf("expected Finalized true (week 3, current week 10), got false")
	}

	var rows []models.SleeperPlayerWeekStat
	db.Find(&rows)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (only fantasy position kept), got %d: %+v", len(rows), rows)
	}
	if rows[0].SleeperPlayerID != "421" || rows[0].PtsPPR == nil || *rows[0].PtsPPR != 24.06 {
		t.Errorf("unexpected row: %+v", rows[0])
	}
}

func TestFetchWeekStats_RefetchOverwrites(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperPlayer{SleeperPlayerID: "421", Position: "RB"})

	srv1 := weekStatsServer(t, `{"421":{"pts_ppr":10}}`, 10, "2025")
	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv1.URL)}
	if _, err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 3}); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	srv1.Close()

	srv2 := weekStatsServer(t, `{"421":{"pts_ppr":15.5}}`, 10, "2025")
	defer srv2.Close()
	wsa2 := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv2.URL)}
	result, err := wsa2.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 3})
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if result.PlayersUpserted != 1 {
		t.Errorf("expected PlayersUpserted 1 on refetch, got %d", result.PlayersUpserted)
	}

	var row models.SleeperPlayerWeekStat
	db.First(&row)
	if row.PtsPPR == nil || *row.PtsPPR != 15.5 {
		t.Errorf("expected overwritten PtsPPR 15.5, got %+v", row.PtsPPR)
	}
	var count int64
	db.Model(&models.SleeperPlayerWeekStat{}).Count(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 row after refetch, got %d", count)
	}
}

func TestFetchWeekStats_MarksFinalized_PastWeek(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperPlayer{SleeperPlayerID: "421", Position: "RB"})
	srv := weekStatsServer(t, `{"421":{"pts_ppr":10}}`, 10, "2025") // current week is 10
	defer srv.Close()

	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result, err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 3})
	if err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}
	if !result.Finalized {
		t.Errorf("expected result.Finalized true, got false")
	}

	var fetch models.SleeperWeekStatFetch
	db.First(&fetch)
	if !fetch.Finalized {
		t.Errorf("expected week 3 finalized (current week is 10), got %+v", fetch)
	}
}

func TestFetchWeekStats_NotFinalized_CurrentWeek(t *testing.T) {
	db := newTestDB(t)
	srv := weekStatsServer(t, `{}`, 10, "2025") // current week is 10, fetching week 10
	defer srv.Close()

	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result, err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 10})
	if err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}
	if result.Finalized {
		t.Errorf("expected result.Finalized false, got true")
	}

	var fetch models.SleeperWeekStatFetch
	db.First(&fetch)
	if fetch.Finalized {
		t.Errorf("expected current week not finalized, got %+v", fetch)
	}
}

func TestFetchWeekStats_PastSeasonAlwaysFinalized(t *testing.T) {
	db := newTestDB(t)
	srv := weekStatsServer(t, `{}`, 3, "2026") // NFL is now in season 2026
	defer srv.Close()

	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result, err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 18})
	if err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}
	if !result.Finalized {
		t.Errorf("expected result.Finalized true, got false")
	}

	var fetch models.SleeperWeekStatFetch
	db.First(&fetch)
	if !fetch.Finalized {
		t.Errorf("expected past-season week finalized, got %+v", fetch)
	}
}

func TestFetchWeekStats_EmptyWeek404_NoRowsButFetchStamped(t *testing.T) {
	db := newTestDB(t)
	srv := weekStatsServer(t, "", 10, "2025") // stats endpoint 404s
	defer srv.Close()

	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result, err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 20})
	if err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}
	if result.PlayersUpserted != 0 {
		t.Errorf("expected PlayersUpserted 0 for 404 week, got %d", result.PlayersUpserted)
	}

	var statCount int64
	db.Model(&models.SleeperPlayerWeekStat{}).Count(&statCount)
	if statCount != 0 {
		t.Errorf("expected no stat rows for 404 week, got %d", statCount)
	}
	var fetchCount int64
	db.Model(&models.SleeperWeekStatFetch{}).Count(&fetchCount)
	if fetchCount != 1 {
		t.Errorf("expected fetch row still stamped for 404 week, got %d", fetchCount)
	}
}

func TestGetFinalizedWeeks_ReturnsOnlyFinalized(t *testing.T) {
	db := newTestDB(t)
	db.Create(&models.SleeperWeekStatFetch{Season: "2025", Week: 1, Finalized: true})
	db.Create(&models.SleeperWeekStatFetch{Season: "2025", Week: 2, Finalized: true})
	db.Create(&models.SleeperWeekStatFetch{Season: "2025", Week: 3, Finalized: false})
	db.Create(&models.SleeperWeekStatFetch{Season: "2024", Week: 1, Finalized: true}) // different season

	wsa := &activities.WeekStatsActivities{DB: db}
	weeks, err := wsa.GetFinalizedWeeks(context.Background(), activities.GetFinalizedWeeksParams{Season: "2025"})
	if err != nil {
		t.Fatalf("GetFinalizedWeeks error: %v", err)
	}
	if len(weeks) != 2 {
		t.Fatalf("expected 2 finalized weeks, got %v", weeks)
	}
}

func TestGetCurrentSeason_ReturnsSleeperState(t *testing.T) {
	srv := weekStatsServer(t, "", 5, "2025")
	defer srv.Close()

	wsa := &activities.WeekStatsActivities{Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	season, err := wsa.GetCurrentSeason(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentSeason error: %v", err)
	}
	if season != "2025" {
		t.Errorf("got season %q, want 2025", season)
	}
}
