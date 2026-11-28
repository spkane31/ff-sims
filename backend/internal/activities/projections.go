package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/models"
	"backend/internal/sleeper"
)

const projectionSource = "sleeper"

// PregameCaptureLead is how far before a kickoff window the pregame capture
// runs. A capture counts toward a window only within PregameCaptureMaxAge of
// its kickoff.
const (
	PregameCaptureLead   = 60 * time.Minute
	PregameCaptureMaxAge = 2 * time.Hour
)

// ProjectionActivities captures Sleeper weekly projections and reports on
// pregame window coverage.
type ProjectionActivities struct {
	DB      *gorm.DB
	Sleeper *sleeper.Client
	// Companies are the projection providers expected in every response;
	// a response missing one is reported, not failed.
	Companies []string
	// RetentionDays bounds how long captures that are not replay-eligible
	// are kept. Zero keeps them forever.
	RetentionDays int
	// Now overrides the clock in tests.
	Now func() time.Time
}

// NewProjectionActivitiesFromEnv reads SLEEPER_PROJECTION_COMPANIES
// (comma-separated, default "rotowire") and PROJECTION_RETENTION_DAYS
// (default 90, 0 disables pruning).
func NewProjectionActivitiesFromEnv(db *gorm.DB, sc *sleeper.Client) (*ProjectionActivities, error) {
	companies := []string{"rotowire"}
	if v := strings.TrimSpace(os.Getenv("SLEEPER_PROJECTION_COMPANIES")); v != "" {
		companies = nil
		for _, c := range strings.Split(v, ",") {
			if c = strings.TrimSpace(c); c != "" {
				companies = append(companies, c)
			}
		}
	}
	retention := 90
	if v := strings.TrimSpace(os.Getenv("PROJECTION_RETENTION_DAYS")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("PROJECTION_RETENTION_DAYS must be a non-negative integer, got %q", v)
		}
		retention = n
	}
	return &ProjectionActivities{DB: db, Sleeper: sc, Companies: companies, RetentionDays: retention}, nil
}

func (a *ProjectionActivities) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

// GetProjectionNFLState returns Sleeper's current NFL state.
func (a *ProjectionActivities) GetProjectionNFLState(ctx context.Context) (sleeper.NFLState, error) {
	s, err := a.Sleeper.GetNFLState(ctx)
	if err != nil {
		return sleeper.NFLState{}, fmt.Errorf("get nfl state: %w", err)
	}
	return *s, nil
}

// CaptureProjections pulls one week of Sleeper projections, judges replay
// eligibility against nfl_kickoffs at capture time, and publishes the capture,
// its rows, its game cutoffs and (for the current week) player status
// observations in one transaction. A week whose final verified kickoff has
// passed is skipped without writing. A retry with the same CaptureKey returns
// the capture already published.
func (a *ProjectionActivities) CaptureProjections(ctx context.Context, params CaptureProjectionsParams) (ProjectionCaptureResult, error) {
	res := ProjectionCaptureResult{Season: params.Season, Week: params.Week, Trigger: params.Trigger}
	if params.CaptureKey == "" || params.Season == "" || params.Week < 1 || params.Week > LastRegularSeasonWeek {
		return res, fmt.Errorf("invalid capture params: %+v", params)
	}

	var existing models.SleeperProjectionCapture
	err := a.DB.WithContext(ctx).Where("capture_key = ?", params.CaptureKey).First(&existing).Error
	if err == nil {
		return resultFromCapture(res, existing), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return res, fmt.Errorf("look up capture %s: %w", params.CaptureKey, err)
	}

	raw, err := a.Sleeper.GetProjections(ctx, params.Season, params.Week, fantasyPositions)
	if err != nil {
		return res, fmt.Errorf("fetch projections %s week %d: %w", params.Season, params.Week, err)
	}
	capturedAt := a.now()

	parsed, err := parseProjections(raw, params.Season, params.Week)
	if err != nil {
		return res, fmt.Errorf("projections %s week %d: %w", params.Season, params.Week, err)
	}
	res.SourceRows = len(raw)

	var kickoffs []models.NFLKickoff
	if err := a.DB.WithContext(ctx).Where("season = ? AND week = ?", params.Season, params.Week).Find(&kickoffs).Error; err != nil {
		return res, fmt.Errorf("load kickoffs: %w", err)
	}
	elig := judgeEligibility(parsed.rows, kickoffs, capturedAt, params.Week > params.CurrentWeek)
	if elig.fullyStarted {
		res.Skipped = true
		res.SkipReason = fmt.Sprintf("final verified kickoff %s has passed", elig.finalKickoff.Format(time.RFC3339))
		return res, nil
	}

	companyCounts := map[string]int{}
	for _, r := range parsed.rows {
		companyCounts[r.Company]++
	}
	var missing []string
	for _, c := range a.Companies {
		if companyCounts[c] == 0 {
			missing = append(missing, c)
		}
	}
	countsJSON, _ := json.Marshal(companyCounts)
	missingJSON, _ := json.Marshal(nonNil(missing))

	capture := models.SleeperProjectionCapture{
		CaptureKey:       params.CaptureKey,
		Season:           params.Season,
		TargetWeek:       params.Week,
		CapturedAt:       capturedAt,
		Source:           projectionSource,
		Trigger:          params.Trigger,
		WindowKickoffAt:  params.WindowKickoff,
		WeekPhase:        elig.phase,
		ReplayEligible:   elig.replayEligible,
		ReplayNote:       elig.note,
		RowCount:         len(parsed.rows),
		CompanyCounts:    countsJSON,
		MissingCompanies: missingJSON,
	}

	err = a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ins := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "capture_key"}}, DoNothing: true}).Create(&capture)
		if ins.Error != nil {
			return fmt.Errorf("insert capture: %w", ins.Error)
		}
		if ins.RowsAffected == 0 {
			// A concurrent attempt published first.
			if err := tx.Where("capture_key = ?", params.CaptureKey).First(&capture).Error; err != nil {
				return fmt.Errorf("reload capture: %w", err)
			}
			res.Idempotent = true
			return nil
		}
		for i := range parsed.rows {
			parsed.rows[i].CaptureID = capture.ID
		}
		if err := tx.CreateInBatches(parsed.rows, 500).Error; err != nil {
			return fmt.Errorf("insert projection rows: %w", err)
		}
		games := elig.games
		for i := range games {
			games[i].CaptureID = capture.ID
		}
		if len(games) > 0 {
			if err := tx.CreateInBatches(games, 500).Error; err != nil {
				return fmt.Errorf("insert capture games: %w", err)
			}
		}
		if params.Week == params.CurrentWeek {
			obs := parsed.statuses
			for i := range obs {
				obs[i].CaptureID = capture.ID
				obs[i].ObservedAt = capturedAt
			}
			if len(obs) > 0 {
				if err := tx.CreateInBatches(obs, 500).Error; err != nil {
					return fmt.Errorf("insert status observations: %w", err)
				}
			}
			res.StatusObservations = len(obs)
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	if res.Idempotent {
		return resultFromCapture(res, capture), nil
	}

	res.CaptureID = capture.ID
	res.CapturedAt = capturedAt
	res.WeekPhase = elig.phase
	res.ReplayEligible = elig.replayEligible
	res.ReplayNote = elig.note
	res.Rows = len(parsed.rows)
	res.MetadataOnlyRows = parsed.metadataOnly
	res.CompanyCounts = companyCounts
	res.MissingCompanies = missing
	return res, nil
}

func resultFromCapture(res ProjectionCaptureResult, c models.SleeperProjectionCapture) ProjectionCaptureResult {
	res.Idempotent = true
	res.CaptureID = c.ID
	res.CapturedAt = c.CapturedAt
	res.WeekPhase = c.WeekPhase
	res.ReplayEligible = c.ReplayEligible
	res.ReplayNote = c.ReplayNote
	res.Rows = c.RowCount
	_ = json.Unmarshal(c.CompanyCounts, &res.CompanyCounts)
	_ = json.Unmarshal(c.MissingCompanies, &res.MissingCompanies)
	return res
}

type parsedProjections struct {
	rows         []models.SleeperPlayerProjection
	statuses     []models.SleeperPlayerStatusObservation
	metadataOnly int
}

// parseProjections validates a projection response and converts the
// fantasy-position rows. An empty response, a row for another week, a row
// missing its player or company, or a repeated player/company pair fails the
// whole response.
func parseProjections(raw []sleeper.Projection, season string, week int) (parsedProjections, error) {
	var out parsedProjections
	if len(raw) == 0 {
		return out, errors.New("empty projection response")
	}
	keep := make(map[string]bool, len(fantasyPositions))
	for _, p := range fantasyPositions {
		keep[p] = true
	}
	type key struct{ player, company string }
	seen := map[key]bool{}
	statusSeen := map[string]bool{}
	for i, r := range raw {
		if r.PlayerID == "" || r.Company == "" {
			return out, fmt.Errorf("malformed row %d: missing player_id or company", i)
		}
		if r.Season != season || r.Week != week {
			return out, fmt.Errorf("malformed row %d: player %s is for %s week %d", i, r.PlayerID, r.Season, r.Week)
		}
		if r.Player == nil || !keep[r.Player.Position] {
			continue
		}
		k := key{r.PlayerID, r.Company}
		if seen[k] {
			return out, fmt.Errorf("duplicate row for player %s company %s", r.PlayerID, r.Company)
		}
		seen[k] = true

		row := models.SleeperPlayerProjection{
			SleeperPlayerID: r.PlayerID,
			Company:         r.Company,
			Position:        r.Player.Position,
			NflTeam:         string(r.Team),
			Opponent:        string(r.Opponent),
			GameID:          string(r.GameID),
		}
		if r.LastModified != nil {
			t := time.UnixMilli(*r.LastModified).UTC()
			row.ProviderLastModified = &t
		}
		kind, pts, err := classifyStats(r.Stats)
		if err != nil {
			return out, fmt.Errorf("malformed stats for player %s company %s: %w", r.PlayerID, r.Company, err)
		}
		row.RowKind = kind
		if kind == models.ProjectionRowKindMetadataOnly {
			out.metadataOnly++
		}
		row.PtsPPR, row.PtsHalfPPR, row.PtsStd = pts.PtsPPR, pts.PtsHalfPPR, pts.PtsStd
		if len(r.Stats) > 0 && string(r.Stats) != "null" {
			row.Stats = r.Stats
		}
		out.rows = append(out.rows, row)

		if !statusSeen[r.PlayerID] {
			statusSeen[r.PlayerID] = true
			obs := models.SleeperPlayerStatusObservation{
				SleeperPlayerID: r.PlayerID,
				Source:          projectionSource,
				NflTeam:         string(r.Player.Team),
				InjuryStatus:    string(r.Player.InjuryStatus),
				InjuryStartDate: string(r.Player.InjuryStartDate),
			}
			if r.Player.NewsUpdated != nil {
				t := time.UnixMilli(*r.Player.NewsUpdated).UTC()
				obs.NewsUpdatedAt = &t
			}
			out.statuses = append(out.statuses, obs)
		}
	}
	return out, nil
}

// classifyStats decides whether a stats object holds a projection. Sleeper
// sends rows whose stats only carry draft-ranking metadata (adp_*,
// pos_adp_*); those are metadata-only and keep null point totals.
func classifyStats(stats json.RawMessage) (string, weekStatPoints, error) {
	var pts weekStatPoints
	if len(stats) == 0 || string(stats) == "null" {
		return models.ProjectionRowKindMetadataOnly, pts, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(stats, &fields); err != nil {
		return "", pts, err
	}
	projected := false
	for k := range fields {
		if !strings.HasPrefix(k, "adp_") && !strings.HasPrefix(k, "pos_adp_") {
			projected = true
			break
		}
	}
	if !projected {
		return models.ProjectionRowKindMetadataOnly, pts, nil
	}
	if err := json.Unmarshal(stats, &pts); err != nil {
		return "", pts, err
	}
	return models.ProjectionRowKindProjection, pts, nil
}

type eligibility struct {
	phase          string
	replayEligible *bool
	note           string
	games          []models.SleeperProjectionCaptureGame
	fullyStarted   bool
	finalKickoff   time.Time
}

// judgeEligibility compares capture time with the week's kickoffs. A week is
// fully covered only when every team with a game in the response has a
// verified kickoff; only then can its phase be pregame or in-progress, or the
// week be skipped as fully started.
func judgeEligibility(rows []models.SleeperPlayerProjection, kickoffs []models.NFLKickoff, capturedAt time.Time, futureWeek bool) eligibility {
	byTeam := make(map[string]models.NFLKickoff, len(kickoffs))
	for _, k := range kickoffs {
		byTeam[k.Team] = k
	}
	gameTeams := map[string][]string{}
	for _, r := range rows {
		if r.GameID == "" || r.NflTeam == "" {
			continue
		}
		teams := gameTeams[r.GameID]
		if !containsString(teams, r.NflTeam) {
			gameTeams[r.GameID] = append(teams, r.NflTeam)
		}
	}

	var e eligibility
	var unverified []string
	var first time.Time
	for _, teams := range gameTeams {
		for _, t := range teams {
			k, ok := byTeam[t]
			if !ok || !k.Verified {
				unverified = append(unverified, t)
				continue
			}
			if first.IsZero() || k.KickoffAt.Before(first) {
				first = k.KickoffAt
			}
			if k.KickoffAt.After(e.finalKickoff) {
				e.finalKickoff = k.KickoffAt
			}
		}
	}
	sort.Strings(unverified)
	covered := len(gameTeams) > 0 && len(unverified) == 0

	switch {
	case covered && !capturedAt.Before(e.finalKickoff):
		e.fullyStarted = true
		return e
	case covered && capturedAt.Before(first):
		e.phase = models.WeekPhasePregame
		e.replayEligible = boolPtr(true)
	case covered:
		e.phase = models.WeekPhaseInProgress
		e.replayEligible = boolPtr(true)
		e.note = fmt.Sprintf("week in progress at capture; first kickoff %s", first.Format(time.RFC3339))
	case futureWeek:
		e.phase = models.WeekPhaseFuture
		e.note = "future week without full verified kickoffs; replay eligibility unset"
	default:
		e.phase = models.WeekPhaseUnverified
		e.replayEligible = boolPtr(false)
		if len(gameTeams) == 0 {
			e.note = "no games in response to compare with verified kickoffs"
		} else {
			e.note = "missing verified kickoff for " + strings.Join(unverified, ",")
		}
	}

	gameIDs := make([]string, 0, len(gameTeams))
	for id := range gameTeams {
		gameIDs = append(gameIDs, id)
	}
	sort.Strings(gameIDs)
	for _, id := range gameIDs {
		g := models.SleeperProjectionCaptureGame{GameID: id, KickoffVerified: true}
		for _, t := range gameTeams[id] {
			k, ok := byTeam[t]
			if !ok {
				g.KickoffVerified = false
				continue
			}
			if !k.Verified {
				g.KickoffVerified = false
			}
			at := k.KickoffAt.UTC()
			g.KickoffAt = &at
		}
		if g.KickoffAt == nil {
			g.KickoffVerified = false
		}
		switch e.phase {
		case models.WeekPhasePregame, models.WeekPhaseInProgress:
			g.ReplayEligible = boolPtr(g.KickoffVerified && g.KickoffAt.After(capturedAt))
		case models.WeekPhaseUnverified:
			g.ReplayEligible = boolPtr(false)
		}
		e.games = append(e.games, g)
	}
	return e
}

// ListUpcomingKickoffWindows returns the distinct verified kickoff times
// within the next horizon, each tagged with its season and week.
func (a *ProjectionActivities) ListUpcomingKickoffWindows(ctx context.Context, params ListKickoffWindowsParams) ([]KickoffWindow, error) {
	now := a.now()
	var windows []KickoffWindow
	err := a.DB.WithContext(ctx).Model(&models.NFLKickoff{}).
		Select("DISTINCT season, week, kickoff_at").
		Where("verified = ? AND kickoff_at > ? AND kickoff_at <= ?", true, now, now.Add(params.Horizon)).
		Order("kickoff_at").
		Scan(&windows).Error
	if err != nil {
		return nil, fmt.Errorf("list kickoff windows: %w", err)
	}
	return windows, nil
}

// KickoffWindowConfirmed reports whether a verified kickoff still exists at
// exactly the window's time, so a pregame run scheduled for a time that has
// since moved does nothing.
func (a *ProjectionActivities) KickoffWindowConfirmed(ctx context.Context, w KickoffWindow) (bool, error) {
	var n int64
	err := a.DB.WithContext(ctx).Model(&models.NFLKickoff{}).
		Where("season = ? AND week = ? AND verified = ? AND kickoff_at = ?", w.Season, w.Week, true, w.KickoffAt.UTC()).
		Count(&n).Error
	if err != nil {
		return false, fmt.Errorf("check kickoff window: %w", err)
	}
	return n > 0, nil
}

// ReportProjectionHealth lists verified kickoff windows in the given weeks
// that have passed without a capture taken within PregameCaptureMaxAge before
// kickoff, and the age of the last published capture.
func (a *ProjectionActivities) ReportProjectionHealth(ctx context.Context, params ProjectionHealthParams) (ProjectionHealth, error) {
	now := a.now()
	var h ProjectionHealth
	var windows []KickoffWindow
	err := a.DB.WithContext(ctx).Model(&models.NFLKickoff{}).
		Select("DISTINCT season, week, kickoff_at").
		Where("season = ? AND week IN ? AND verified = ? AND kickoff_at <= ?", params.Season, params.Weeks, true, now).
		Order("kickoff_at").
		Scan(&windows).Error
	if err != nil {
		return h, fmt.Errorf("list past kickoff windows: %w", err)
	}
	for _, w := range windows {
		var n int64
		err := a.DB.WithContext(ctx).Model(&models.SleeperProjectionCapture{}).
			Where("season = ? AND target_week = ? AND captured_at >= ? AND captured_at < ?",
				w.Season, w.Week, w.KickoffAt.Add(-PregameCaptureMaxAge), w.KickoffAt).
			Count(&n).Error
		if err != nil {
			return h, fmt.Errorf("check window coverage: %w", err)
		}
		if n == 0 {
			h.MissedWindows = append(h.MissedWindows, w)
		}
	}

	var last models.SleeperProjectionCapture
	err = a.DB.WithContext(ctx).Order("captured_at DESC").First(&last).Error
	switch {
	case err == nil:
		h.LastCaptureAt = &last.CapturedAt
		h.LastCaptureAge = now.Sub(last.CapturedAt)
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return h, fmt.Errorf("load last capture: %w", err)
	}
	return h, nil
}

// PruneProjectionCaptures deletes captures older than RetentionDays that are
// not replay-eligible. Replay-eligible captures are kept indefinitely.
func (a *ProjectionActivities) PruneProjectionCaptures(ctx context.Context) (int64, error) {
	if a.RetentionDays <= 0 {
		return 0, nil
	}
	cutoff := a.now().AddDate(0, 0, -a.RetentionDays)
	var ids []int64
	if err := a.DB.WithContext(ctx).Model(&models.SleeperProjectionCapture{}).
		Where("captured_at < ? AND (replay_eligible IS NULL OR replay_eligible = ?)", cutoff, false).
		Pluck("id", &ids).Error; err != nil {
		return 0, fmt.Errorf("find prunable captures: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	err := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete children explicitly rather than relying on ON DELETE
		// CASCADE, which SQLite test databases do not enforce by default.
		for _, m := range []any{&models.SleeperPlayerProjection{}, &models.SleeperProjectionCaptureGame{}, &models.SleeperPlayerStatusObservation{}} {
			if err := tx.Where("capture_id IN ?", ids).Delete(m).Error; err != nil {
				return err
			}
		}
		return tx.Where("id IN ?", ids).Delete(&models.SleeperProjectionCapture{}).Error
	})
	if err != nil {
		return 0, fmt.Errorf("prune captures: %w", err)
	}
	return int64(len(ids)), nil
}

func boolPtr(b bool) *bool { return &b }

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
