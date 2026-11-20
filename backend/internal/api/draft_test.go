package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"backend/internal/database"
	"backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDraftRewind(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.League{}, &models.Team{}, &models.Player{}, &models.DraftSelection{}, &models.Matchup{}, &models.BoxScore{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	oldDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = oldDB })

	seed := func(records any) {
		t.Helper()
		if err := db.Session(&gorm.Session{SkipHooks: true}).Create(records).Error; err != nil {
			t.Fatal(err)
		}
	}
	seed(&[]models.League{{ID: 1, Name: "First", Platform: "ESPN", ExternalID: "1"}, {ID: 2, Name: "Second", Platform: "ESPN", ExternalID: "2"}})
	seed(&[]models.Team{
		{ID: 1, LeagueID: 1, ESPNID: 1, Name: "Comets"},
		{ID: 2, LeagueID: 1, ESPNID: 2, Name: "Owls"},
	})
	seed(&[]models.DraftSelection{
		{ID: 1, LeagueID: 1, Year: 2024, Round: 1, Pick: 1, TeamID: 1, PlayerID: 11},
		{ID: 2, LeagueID: 1, Year: 2024, Round: 1, Pick: 2, TeamID: 2, PlayerID: 12},
		{ID: 3, LeagueID: 1, Year: 2024, Round: 1, Pick: 3, TeamID: 1, PlayerID: 13},
		{ID: 4, LeagueID: 1, Year: 2024, Round: 2, Pick: 1, TeamID: 2, PlayerID: 14},
		{ID: 5, LeagueID: 1, Year: 2024, Round: 2, Pick: 2, TeamID: 2, PlayerID: 15},
		{ID: 6, LeagueID: 1, Year: 2024, Round: 2, Pick: 3, TeamID: 1, PlayerID: 16},
		{ID: 7, LeagueID: 1, Year: 2024, Round: 3, Pick: 1, TeamID: 2, PlayerID: 17},
		{ID: 8, LeagueID: 1, Year: 2025, Round: 1, Pick: 1, TeamID: 2, PlayerID: 11},
		{ID: 9, LeagueID: 2, Year: 2023, Round: 1, Pick: 1, TeamID: 2, PlayerID: 11},
	})
	seed(&[]models.Matchup{
		{ID: 1, LeagueID: 1, Year: 2024, Week: 1, Completed: true, GameType: "NONE"},
		{ID: 2, LeagueID: 1, Year: 2024, Week: 2, Completed: true, GameType: "REGULAR"},
		{ID: 3, LeagueID: 1, Year: 2024, Week: 3, Completed: false, GameType: "NONE"},
		{ID: 4, LeagueID: 1, Year: 2024, Week: 3, Completed: true, GameType: "NONE"},
		{ID: 5, LeagueID: 1, Year: 2024, Week: 2, Completed: true, GameType: "LOSERS_CONSOLATION_LADDER"},
		{ID: 6, LeagueID: 2, Year: 2024, Week: 2, Completed: true, GameType: "NONE"},
		{ID: 7, LeagueID: 1, Year: 2023, Week: 2, Completed: true, GameType: "NONE"},
		{ID: 8, LeagueID: 1, Year: 2024, Week: 4, Completed: true, GameType: "NONE"},
	})
	seed(&[]models.BoxScore{
		{MatchupID: 1, PlayerID: 11, TeamID: 1, ActualPoints: 12.50, ProjectedPoints: 9},
		{MatchupID: 2, PlayerID: 11, TeamID: 2, ActualPoints: 7.50},
		{MatchupID: 2, PlayerID: 11, TeamID: 1, ActualPoints: 7.50},
		{MatchupID: 1, PlayerID: 12, TeamID: 2, ActualPoints: 20.001, ProjectedPoints: 22},
		{MatchupID: 2, PlayerID: 13, TeamID: 1, ActualPoints: 2},
		{MatchupID: 1, PlayerID: 14, TeamID: 2, ActualPoints: 0},
		{MatchupID: 1, PlayerID: 16, TeamID: 1, ActualPoints: 150},
		{MatchupID: 1, PlayerID: 16, TeamID: 2, ActualPoints: 15},
		{MatchupID: 2, PlayerID: 17, TeamID: 2, ActualPoints: 30},
		{MatchupID: 3, PlayerID: 11, ActualPoints: 100},
		{MatchupID: 4, PlayerID: 11, ActualPoints: 100},
		{MatchupID: 5, PlayerID: 11, ActualPoints: 100},
		{MatchupID: 6, PlayerID: 11, ActualPoints: 100},
		{MatchupID: 7, PlayerID: 11, ActualPoints: 100},
		{MatchupID: 8, PlayerID: 11, ActualPoints: 100},
	})

	router := gin.New()
	SetupRouter(router)
	get := func(t *testing.T, path string, status int, dest any) {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/leagues/"+path, nil))
		if w.Code != status {
			t.Fatalf("%s: got %d, want %d: %s", path, w.Code, status, w.Body.String())
		}
		if dest != nil {
			if err := json.Unmarshal(w.Body.Bytes(), dest); err != nil {
				t.Fatal(err)
			}
		}
	}

	t.Run("draft years include picks without matchups", func(t *testing.T) {
		var seasons struct{ Years []uint }
		get(t, "1/draft/seasons", http.StatusOK, &seasons)
		if !reflect.DeepEqual(seasons.Years, []uint{2025, 2024}) {
			t.Errorf("unexpected available years: %v", seasons.Years)
		}
	})

	type pick struct {
		TeamID                 uint     `json:"team_id"`
		RoundRank              *uint    `json:"round_rank"`
		OverallRank            *uint    `json:"overall_rank"`
		ActualPoints           *float64 `json:"actual_points"`
		ProjectedPoints        *float64 `json:"projected_points"`
		ProjectionActualPoints *float64 `json:"projection_actual_points"`
		ObservedWeeks          int      `json:"observed_weeks"`
		ProjectionWeeks        int      `json:"projection_weeks"`
		ScoreStatus            string   `json:"score_status"`
	}
	type report struct {
		ThroughWeek uint `json:"through_week"`
		RankedPicks int  `json:"ranked_picks"`
		TotalPicks  int  `json:"total_picks"`
		Picks       []pick
		Teams       []struct {
			ID          uint
			Name        string
			RankedPicks int      `json:"ranked_picks"`
			TotalPicks  int      `json:"total_picks"`
			Actual      *float64 `json:"actual_points"`
		}
	}
	t.Run("ranks the full draft and credits the original drafting team", func(t *testing.T) {
		var data report
		get(t, "1/draft/2024", http.StatusOK, &data)
		if data.ThroughWeek != 2 || data.RankedPicks != 5 || data.TotalPicks != 7 || len(data.Picks) != 7 {
			t.Fatalf("unexpected report: %+v", data)
		}
		want := []struct {
			points         float64
			round, overall uint
			status         string
		}{
			{20, 1, 2, "ranked"}, {20, 1, 2, "ranked"}, {2, 3, 4, "ranked"},
			{0, 1, 5, "ranked"}, {0, 0, 0, "no_scores"}, {0, 0, 0, "conflicting_scores"},
			{30, 1, 1, "ranked"},
		}
		for i, p := range data.Picks {
			expected := want[i]
			if p.ScoreStatus != expected.status {
				t.Errorf("pick %d: status = %q, want %q", i+1, p.ScoreStatus, expected.status)
			}
			if expected.round == 0 {
				if p.ActualPoints != nil || p.RoundRank != nil || p.OverallRank != nil {
					t.Errorf("pick %d: unranked pick has a score or rank", i+1)
				}
				continue
			}
			if p.ActualPoints == nil || *p.ActualPoints != expected.points || p.RoundRank == nil || *p.RoundRank != expected.round || p.OverallRank == nil || *p.OverallRank != expected.overall {
				t.Errorf("pick %d: want %v points, round rank %d and overall rank %d, got %+v", i+1, expected.points, expected.round, expected.overall, p)
			}
		}
		first := data.Picks[0]
		if first.TeamID != 1 || first.ObservedWeeks != 2 || first.ProjectionWeeks != 1 || first.ProjectedPoints == nil || *first.ProjectedPoints != 9 || first.ProjectionActualPoints == nil || *first.ProjectionActualPoints != 12.50 {
			t.Errorf("expected paired projection comparison and original drafting team, got %+v", first)
		}
		if len(data.Teams) != 2 {
			t.Fatalf("expected two drafting teams, got %+v", data.Teams)
		}
		team := data.Teams[0]
		if team.ID != 1 || team.Name != "Comets" || team.RankedPicks != 2 || team.TotalPicks != 3 || team.Actual == nil || *team.Actual != 22 {
			t.Errorf("unexpected original drafting team totals: %+v", team)
		}
	})

	t.Run("preseason shows the draft without ranks", func(t *testing.T) {
		var data report
		get(t, "1/draft/2025", http.StatusOK, &data)
		if data.ThroughWeek != 0 || data.TotalPicks != 1 || len(data.Picks) != 1 || data.Picks[0].OverallRank != nil {
			t.Fatalf("unexpected preseason report: %+v", data)
		}
	})

	t.Run("invalid or missing league and year", func(t *testing.T) {
		get(t, "1/draft/-1", http.StatusBadRequest, nil)
		get(t, "0/draft/seasons", http.StatusBadRequest, nil)
		get(t, "99/draft/2024", http.StatusNotFound, nil)
	})
}
