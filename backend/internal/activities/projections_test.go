package activities_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/sleeper"
)

func newProjectionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newTestDB(t)
	if err := db.AutoMigrate(
		&models.NFLKickoff{},
		&models.SleeperProjectionCapture{},
		&models.SleeperProjectionCaptureGame{},
		&models.SleeperPlayerProjection{},
		&models.SleeperPlayerStatusObservation{},
	); err != nil {
		t.Fatalf("automigrate projections: %v", err)
	}
	return db
}

type projRow map[string]any

func projectionRow(player, company, position, team, gameID string, stats map[string]any) projRow {
	r := projRow{
		"player_id": player, "company": company, "category": "proj",
		"season": "2026", "season_type": "regular", "week": 13,
		"stats": stats,
		"player": map[string]any{
			"position": position, "team": team, "injury_status": nil,
			"injury_start_date": nil, "news_updated": nil,
		},
	}
	if team != "" {
		r["team"] = team
	}
	if gameID != "" {
		r["game_id"] = gameID
		r["last_modified"] = int64(1795887636945)
	}
	return r
}

// projectionServer serves rows for every projections request and records the
// last query string.
func projectionServer(t *testing.T, rows []projRow, lastQuery *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/projections/nfl/2026/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if lastQuery != nil {
			*lastQuery = r.URL.RawQuery
		}
		json.NewEncoder(w).Encode(rows)
	}))
	t.Cleanup(srv.Close)
	return srv
}

var (
	thu     = time.Date(2026, 12, 4, 1, 15, 0, 0, time.UTC)  // Thu 20:15 ET
	sunEarl = time.Date(2026, 12, 6, 18, 0, 0, 0, time.UTC)  // Sun 13:00 ET
	sunLate = time.Date(2026, 12, 6, 21, 25, 0, 0, time.UTC) // Sun 16:25 ET
	mon     = time.Date(2026, 12, 8, 1, 15, 0, 0, time.UTC)  // Mon 20:15 ET
)

// seedSplitSlate schedules a week with Thursday, Sunday early, Sunday late
// and Monday games.
func seedSplitSlate(t *testing.T, db *gorm.DB, verified bool) {
	t.Helper()
	for _, g := range []struct {
		away, home string
		at         time.Time
	}{
		{"GB", "DET", thu},
		{"DAL", "SEA", sunEarl},
		{"KC", "DEN", sunLate},
		{"NYG", "PHI", mon},
	} {
		for _, k := range []models.NFLKickoff{
			{Season: "2026", Week: 13, Team: g.away, Opponent: g.home, KickoffAt: g.at, Verified: verified, Source: "test"},
			{Season: "2026", Week: 13, Team: g.home, Opponent: g.away, KickoffAt: g.at, Verified: verified, Source: "test"},
		} {
			if err := db.Create(&k).Error; err != nil {
				t.Fatalf("seed kickoff: %v", err)
			}
		}
	}
}

func slateRows() []projRow {
	return []projRow{
		projectionRow("gb1", "rotowire", "QB", "GB", "g-thu", map[string]any{"pts_ppr": 20.5, "pass_yd": 250.0}),
		projectionRow("det1", "rotowire", "RB", "DET", "g-thu", map[string]any{"pts_ppr": 11.0, "rush_yd": 70.0}),
		projectionRow("sea1", "rotowire", "WR", "SEA", "g-early", map[string]any{"pts_ppr": 28.43, "pts_half_ppr": 24.58, "pts_std": 20.72, "rec": 7.7}),
		projectionRow("sea1", "numberfire", "WR", "SEA", "g-early", map[string]any{"pts_ppr": 25.0, "rec": 7.0}),
		projectionRow("dal1", "rotowire", "K", "DAL", "g-early", map[string]any{"pts_ppr": 0.0, "pts_std": 0.0, "fgm": 0.0}),
		projectionRow("DEN", "rotowire", "DEF", "DEN", "g-late", map[string]any{"pts_ppr": 12.63, "sack": 3.07}),
		projectionRow("kc1", "rotowire", "TE", "KC", "g-late", map[string]any{"pts_ppr": 9.0, "rec": 5.0}),
		projectionRow("phi1", "rotowire", "RB", "PHI", "g-mon", map[string]any{"pts_ppr": 17.0, "rush_yd": 90.0}),
		projectionRow("nyg1", "rotowire", "WR", "NYG", "g-mon", map[string]any{"pts_ppr": 8.0, "rec": 4.0}),
		// Free agent with only draft-ranking metadata.
		projectionRow("fa1", "rotowire", "WR", "", "", map[string]any{"adp_dd_ppr": 1000.0}),
		// IDP row: filtered out.
		projectionRow("lb1", "rotowire", "LB", "SEA", "g-early", map[string]any{"idp_tkl": 6.0}),
	}
}

func newProjectionActivities(db *gorm.DB, srv *httptest.Server, now time.Time) *activities.ProjectionActivities {
	return &activities.ProjectionActivities{
		DB:        db,
		Sleeper:   sleeper.NewWithBaseURL(srv.URL),
		Companies: []string{"rotowire", "numberfire", "fantasypros"},
		Now:       func() time.Time { return now },
	}
}

func captureParams(key string, week, current int) activities.CaptureProjectionsParams {
	return activities.CaptureProjectionsParams{CaptureKey: key, Season: "2026", Week: week, CurrentWeek: current, Trigger: models.ProjectionTriggerDailySweep}
}

func TestCaptureProjections_PregamePreservesRowKindsAndStatuses(t *testing.T) {
	db := newProjectionTestDB(t)
	seedSplitSlate(t, db, true)
	var query string
	srv := projectionServer(t, slateRows(), &query)
	capturedAt := thu.Add(-3 * time.Hour)
	pa := newProjectionActivities(db, srv, capturedAt)

	res, err := pa.CaptureProjections(context.Background(), captureParams("run-1:13", 13, 13))
	if err != nil {
		t.Fatalf("CaptureProjections: %v", err)
	}
	if !strings.Contains(query, "position%5B%5D=QB") || !strings.Contains(query, "season_type=regular") {
		t.Errorf("unexpected query %q", query)
	}
	if res.Skipped || res.WeekPhase != models.WeekPhasePregame || res.ReplayEligible == nil || !*res.ReplayEligible {
		t.Fatalf("expected replay-eligible pregame capture, got %+v", res)
	}
	if res.Rows != 10 || res.MetadataOnlyRows != 1 || res.SourceRows != 11 {
		t.Errorf("rows=%d metadata=%d source=%d, want 10/1/11", res.Rows, res.MetadataOnlyRows, res.SourceRows)
	}
	if res.CompanyCounts["rotowire"] != 9 || res.CompanyCounts["numberfire"] != 1 {
		t.Errorf("company counts %v", res.CompanyCounts)
	}
	if len(res.MissingCompanies) != 1 || res.MissingCompanies[0] != "fantasypros" {
		t.Errorf("missing companies %v, want [fantasypros]", res.MissingCompanies)
	}

	var capture models.SleeperProjectionCapture
	db.First(&capture)
	if !capture.CapturedAt.Equal(capturedAt) || capture.Source != "sleeper" || capture.TargetWeek != 13 {
		t.Errorf("capture %+v", capture)
	}

	get := func(player, company string) *models.SleeperPlayerProjection {
		var p models.SleeperPlayerProjection
		if err := db.Where("sleeper_player_id = ? AND company = ?", player, company).First(&p).Error; err != nil {
			return nil
		}
		return &p
	}
	if k := get("dal1", "rotowire"); k == nil || k.PtsPPR == nil || *k.PtsPPR != 0 || k.PtsHalfPPR != nil || k.RowKind != models.ProjectionRowKindProjection {
		t.Errorf("explicit zero not preserved: %+v", k)
	}
	if fa := get("fa1", "rotowire"); fa == nil || fa.RowKind != models.ProjectionRowKindMetadataOnly || fa.PtsPPR != nil {
		t.Errorf("metadata-only row not preserved: %+v", fa)
	}
	if lb := get("lb1", "rotowire"); lb != nil {
		t.Errorf("IDP row stored: %+v", lb)
	}
	if nf := get("sea1", "numberfire"); nf == nil || *nf.PtsPPR != 25 {
		t.Errorf("second company row: %+v", nf)
	}
	if rw := get("sea1", "rotowire"); rw == nil || rw.ProviderLastModified == nil || rw.ProviderLastModified.UnixMilli() != 1795887636945 {
		t.Errorf("provider last_modified: %+v", rw)
	}

	var obs []models.SleeperPlayerStatusObservation
	db.Order("sleeper_player_id").Find(&obs)
	if len(obs) != 9 || res.StatusObservations != 9 {
		t.Fatalf("expected one status observation per player (9), got %d", len(obs))
	}
	for _, o := range obs {
		if o.CaptureID != capture.ID || !o.ObservedAt.Equal(capturedAt) {
			t.Errorf("observation not tied to capture: %+v", o)
		}
	}

	var games []models.SleeperProjectionCaptureGame
	db.Find(&games)
	if len(games) != 4 {
		t.Fatalf("expected 4 games, got %d", len(games))
	}
	for _, g := range games {
		if !g.KickoffVerified || g.ReplayEligible == nil || !*g.ReplayEligible || g.KickoffAt == nil {
			t.Errorf("game %+v should be verified and replay-eligible", g)
		}
	}
}

func TestCaptureProjections_SkipsWeekAfterPostMNFRewrite(t *testing.T) {
	db := newProjectionTestDB(t)
	seedSplitSlate(t, db, true)
	// Tuesday morning: Sleeper still reports week 13 and has rewritten
	// projections after Monday Night Football.
	rows := slateRows()
	rows[7]["stats"] = map[string]any{"pts_ppr": 31.0, "rush_yd": 160.0}
	srv := projectionServer(t, rows, nil)
	pa := newProjectionActivities(db, srv, mon.Add(10*time.Hour))

	res, err := pa.CaptureProjections(context.Background(), captureParams("run-1:13", 13, 13))
	if err != nil {
		t.Fatalf("CaptureProjections: %v", err)
	}
	if !res.Skipped || !strings.Contains(res.SkipReason, "final verified kickoff") {
		t.Fatalf("expected skip after final kickoff, got %+v", res)
	}
	var n int64
	db.Model(&models.SleeperProjectionCapture{}).Count(&n)
	if n != 0 {
		t.Errorf("expected no capture written, got %d", n)
	}
}

func TestCaptureProjections_SplitSundaySlateLimitsReplayToLaterGames(t *testing.T) {
	db := newProjectionTestDB(t)
	seedSplitSlate(t, db, true)
	srv := projectionServer(t, slateRows(), nil)
	// Between the Sunday early and late windows.
	capturedAt := sunEarl.Add(90 * time.Minute)
	pa := newProjectionActivities(db, srv, capturedAt)

	res, err := pa.CaptureProjections(context.Background(), captureParams("run-1:13", 13, 13))
	if err != nil {
		t.Fatalf("CaptureProjections: %v", err)
	}
	if res.WeekPhase != models.WeekPhaseInProgress || res.ReplayEligible == nil || !*res.ReplayEligible {
		t.Fatalf("expected in-progress capture, got %+v", res)
	}
	eligible := map[string]bool{}
	var games []models.SleeperProjectionCaptureGame
	db.Find(&games)
	for _, g := range games {
		eligible[g.GameID] = g.ReplayEligible != nil && *g.ReplayEligible
	}
	want := map[string]bool{"g-thu": false, "g-early": false, "g-late": true, "g-mon": true}
	for id, w := range want {
		if eligible[id] != w {
			t.Errorf("game %s eligible=%v, want %v", id, eligible[id], w)
		}
	}

	got, err := models.ReplayProjections(db, "2026", 13, "rotowire", sunLate)
	if err != nil {
		t.Fatalf("ReplayProjections: %v", err)
	}
	var ids []string
	for _, p := range got {
		ids = append(ids, p.SleeperPlayerID)
	}
	if strings.Join(ids, ",") != "DEN,kc1,nyg1,phi1" {
		t.Errorf("replay rows %v, want only late Sunday and Monday players", ids)
	}
	if _, err := models.ReplayProjections(db, "2026", 13, "", sunLate); err != models.ErrProjectionCompanyRequired {
		t.Errorf("expected company to be required, got %v", err)
	}
}

func TestCaptureProjections_DelayedCaptureMissesThursdayWindow(t *testing.T) {
	db := newProjectionTestDB(t)
	seedSplitSlate(t, db, true)
	srv := projectionServer(t, slateRows(), nil)
	// The Thursday pregame run was delayed past kickoff.
	capturedAt := thu.Add(30 * time.Minute)
	pa := newProjectionActivities(db, srv, capturedAt)
	params := captureParams("pregame-thu", 13, 13)
	params.Trigger = models.ProjectionTriggerPregame
	params.WindowKickoff = &thu

	if _, err := pa.CaptureProjections(context.Background(), params); err != nil {
		t.Fatalf("CaptureProjections: %v", err)
	}
	var g models.SleeperProjectionCaptureGame
	db.Where("game_id = ?", "g-thu").First(&g)
	if g.ReplayEligible == nil || *g.ReplayEligible {
		t.Errorf("Thursday game must not be replay-eligible from a post-kickoff capture: %+v", g)
	}
	if g.KickoffAt == nil || !g.KickoffAt.Equal(thu) {
		t.Errorf("game cutoff not recorded: %+v", g)
	}

	pa.Now = func() time.Time { return capturedAt.Add(time.Hour) }
	health, err := pa.ReportProjectionHealth(context.Background(), activities.ProjectionHealthParams{Season: "2026", Weeks: []int{13}})
	if err != nil {
		t.Fatalf("ReportProjectionHealth: %v", err)
	}
	if len(health.MissedWindows) != 1 || !health.MissedWindows[0].KickoffAt.Equal(thu) {
		t.Errorf("expected the Thursday window reported missed, got %+v", health.MissedWindows)
	}
	if health.LastCaptureAge != time.Hour {
		t.Errorf("last capture age %s, want 1h", health.LastCaptureAge)
	}
}

func TestCaptureProjections_WithoutVerifiedSchedule(t *testing.T) {
	db := newProjectionTestDB(t)
	seedSplitSlate(t, db, false) // provisional only
	srv := projectionServer(t, slateRows(), nil)
	pa := newProjectionActivities(db, srv, thu.Add(-72*time.Hour))

	future, err := pa.CaptureProjections(context.Background(), captureParams("run-1:13", 13, 12))
	if err != nil {
		t.Fatalf("future capture: %v", err)
	}
	if future.WeekPhase != models.WeekPhaseFuture || future.ReplayEligible != nil || future.StatusObservations != 0 {
		t.Errorf("future week should leave eligibility unset and skip statuses: %+v", future)
	}

	current, err := pa.CaptureProjections(context.Background(), captureParams("run-2:13", 13, 13))
	if err != nil {
		t.Fatalf("current capture: %v", err)
	}
	if current.WeekPhase != models.WeekPhaseUnverified || current.ReplayEligible == nil || *current.ReplayEligible {
		t.Errorf("current week without verified kickoffs must not be replay-eligible: %+v", current)
	}
	if !strings.Contains(current.ReplayNote, "missing verified kickoff") {
		t.Errorf("expected reason, got %q", current.ReplayNote)
	}
	if got, _ := models.ReplayProjections(db, "2026", 13, "rotowire", mon); got != nil {
		t.Errorf("unverified capture leaked into replay: %+v", got)
	}
}

func TestCaptureProjections_RetryIsIdempotent(t *testing.T) {
	db := newProjectionTestDB(t)
	seedSplitSlate(t, db, true)
	srv := projectionServer(t, slateRows(), nil)
	pa := newProjectionActivities(db, srv, thu.Add(-3*time.Hour))

	first, err := pa.CaptureProjections(context.Background(), captureParams("run-1:13", 13, 13))
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	pa.Now = func() time.Time { return thu.Add(-2 * time.Hour) }
	second, err := pa.CaptureProjections(context.Background(), captureParams("run-1:13", 13, 13))
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !second.Idempotent || second.CaptureID != first.CaptureID || !second.CapturedAt.Equal(first.CapturedAt) {
		t.Errorf("retry should return the first capture: first=%+v second=%+v", first, second)
	}
	var captures, rows int64
	db.Model(&models.SleeperProjectionCapture{}).Count(&captures)
	db.Model(&models.SleeperPlayerProjection{}).Count(&rows)
	if captures != 1 || rows != 10 {
		t.Errorf("captures=%d rows=%d, want 1/10", captures, rows)
	}
}

func TestCaptureProjections_RejectsMalformedResponses(t *testing.T) {
	dup := slateRows()
	dup = append(dup, projectionRow("sea1", "rotowire", "WR", "SEA", "g-early", map[string]any{"pts_ppr": 1.0}))
	wrongWeek := slateRows()
	wrongWeek[0]["week"] = 14

	for name, tc := range map[string]struct {
		rows []projRow
		want string
	}{
		"empty":      {rows: []projRow{}, want: "empty projection response"},
		"duplicate":  {rows: dup, want: "duplicate row for player sea1 company rotowire"},
		"wrong week": {rows: wrongWeek, want: "week 14"},
	} {
		t.Run(name, func(t *testing.T) {
			db := newProjectionTestDB(t)
			srv := projectionServer(t, tc.rows, nil)
			pa := newProjectionActivities(db, srv, thu)
			_, err := pa.CaptureProjections(context.Background(), captureParams("k", 13, 13))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
			var n int64
			db.Model(&models.SleeperProjectionCapture{}).Count(&n)
			if n != 0 {
				t.Errorf("malformed response published %d captures", n)
			}
		})
	}
}

func TestPruneProjectionCaptures_KeepsReplayEligible(t *testing.T) {
	db := newProjectionTestDB(t)
	now := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	yes, no := true, false
	for i, c := range []struct {
		age      time.Duration
		eligible *bool
	}{
		{100 * 24 * time.Hour, nil},  // old future-week capture: pruned
		{100 * 24 * time.Hour, &no},  // old unverified capture: pruned
		{100 * 24 * time.Hour, &yes}, // old replay capture: kept
		{10 * 24 * time.Hour, nil},   // recent: kept
	} {
		capture := models.SleeperProjectionCapture{
			CaptureKey: string(rune('a' + i)), Season: "2026", TargetWeek: 13,
			CapturedAt: now.Add(-c.age), Source: "sleeper", Trigger: models.ProjectionTriggerDailySweep,
			WeekPhase: models.WeekPhaseFuture, ReplayEligible: c.eligible,
			CompanyCounts: json.RawMessage(`{}`), MissingCompanies: json.RawMessage(`[]`),
		}
		db.Create(&capture)
		db.Create(&models.SleeperPlayerProjection{CaptureID: capture.ID, SleeperPlayerID: "p", Company: "rotowire", RowKind: models.ProjectionRowKindProjection, Position: "QB"})
	}

	pa := &activities.ProjectionActivities{DB: db, RetentionDays: 90, Now: func() time.Time { return now }}
	n, err := pa.PruneProjectionCaptures(context.Background())
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 2 {
		t.Errorf("pruned %d, want 2", n)
	}
	var captures, rows int64
	db.Model(&models.SleeperProjectionCapture{}).Count(&captures)
	db.Model(&models.SleeperPlayerProjection{}).Count(&rows)
	if captures != 2 || rows != 2 {
		t.Errorf("captures=%d rows=%d after prune, want 2/2", captures, rows)
	}
}

func TestKickoffWindows_ListAndConfirm(t *testing.T) {
	db := newProjectionTestDB(t)
	seedSplitSlate(t, db, true)
	db.Create(&models.NFLKickoff{Season: "2026", Week: 13, Team: "LV", Opponent: "LAC", KickoffAt: sunLate.Add(time.Hour), Verified: false, Source: "test"})
	pa := &activities.ProjectionActivities{DB: db, Now: func() time.Time { return thu.Add(time.Minute) }}

	windows, err := pa.ListUpcomingKickoffWindows(context.Background(), activities.ListKickoffWindowsParams{Horizon: 4 * 24 * time.Hour})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var got []time.Time
	for _, w := range windows {
		got = append(got, w.KickoffAt)
	}
	if len(got) != 3 || !got[0].Equal(sunEarl) || !got[1].Equal(sunLate) || !got[2].Equal(mon) {
		t.Errorf("windows %v, want distinct verified Sunday early, Sunday late, Monday", got)
	}

	ok, err := pa.KickoffWindowConfirmed(context.Background(), activities.KickoffWindow{Season: "2026", Week: 13, KickoffAt: sunLate})
	if err != nil || !ok {
		t.Errorf("expected Sunday late window confirmed, got %v %v", ok, err)
	}
	ok, err = pa.KickoffWindowConfirmed(context.Background(), activities.KickoffWindow{Season: "2026", Week: 13, KickoffAt: sunLate.Add(time.Hour)})
	if err != nil || ok {
		t.Errorf("provisional/moved window must not be confirmed, got %v %v", ok, err)
	}
}
