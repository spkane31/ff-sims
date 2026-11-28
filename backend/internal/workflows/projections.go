package workflows

import (
	"errors"
	"fmt"
	"strings"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/sleeper"
)

// TaskQueueProjections serves Sleeper projection capture workflows.
const TaskQueueProjections = "sleeper-projections"

// pregameDispatchHorizon is how far ahead ProjectionPregameDispatcher looks
// for verified kickoff windows to schedule captures for.
const pregameDispatchHorizon = 36 * time.Hour

// ProjectionSweepReport is returned by ProjectionDailySweep.
type ProjectionSweepReport struct {
	Season         string
	CurrentWeek    int
	Captures       []activities.ProjectionCaptureResult
	FailedWeeks    []string
	MissedWindows  []activities.KickoffWindow
	LastCaptureAge time.Duration
	Pruned         int64
}

// ProjectionDailySweep captures projections for the current and every
// remaining regular-season week through Week 18, skipping weeks whose final
// verified kickoff has passed (e.g. when Sleeper still reports last week on a
// Tuesday). It then reports pregame windows missed in the current and previous
// week, the age of the last capture, and prunes old captures. Every week is
// attempted; the workflow fails after the sweep if any week failed.
func ProjectionDailySweep(ctx workflow.Context) (ProjectionSweepReport, error) {
	pa := &activities.ProjectionActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	logger := workflow.GetLogger(ctx)

	var report ProjectionSweepReport
	season, current, ok, err := resolveProjectionWeek(ctx, actCtx)
	if err != nil {
		return report, err
	}
	report.Season, report.CurrentWeek = season, current
	if !ok {
		logger.Info("projection sweep skipped outside regular season", "season", season)
		return report, nil
	}

	runID := workflow.GetInfo(ctx).WorkflowExecution.RunID
	for week := current; week <= activities.LastRegularSeasonWeek; week++ {
		var res activities.ProjectionCaptureResult
		err := workflow.ExecuteActivity(actCtx, pa.CaptureProjections, activities.CaptureProjectionsParams{
			CaptureKey:  fmt.Sprintf("%s:%s:%d", runID, season, week),
			Season:      season,
			Week:        week,
			CurrentWeek: current,
			Trigger:     models.ProjectionTriggerDailySweep,
		}).Get(ctx, &res)
		if err != nil {
			logger.Error("projection capture failed", "season", season, "week", week, "error", err)
			report.FailedWeeks = append(report.FailedWeeks, fmt.Sprintf("%d: %v", week, err))
			continue
		}
		logCapture(logger, res)
		report.Captures = append(report.Captures, res)
	}

	weeks := []int{current}
	if current > 1 {
		weeks = append(weeks, current-1)
	}
	var health activities.ProjectionHealth
	if err := workflow.ExecuteActivity(actCtx, pa.ReportProjectionHealth, activities.ProjectionHealthParams{Season: season, Weeks: weeks}).Get(ctx, &health); err != nil {
		return report, err
	}
	report.MissedWindows = health.MissedWindows
	report.LastCaptureAge = health.LastCaptureAge
	for _, w := range health.MissedWindows {
		logger.Warn("missed pregame projection window", "season", w.Season, "week", w.Week, "kickoff", w.KickoffAt)
	}
	logger.Info("projection sweep health", "missed_windows", len(health.MissedWindows), "last_capture_age", health.LastCaptureAge)

	if err := workflow.ExecuteActivity(actCtx, pa.PruneProjectionCaptures).Get(ctx, &report.Pruned); err != nil {
		return report, err
	}

	if len(report.FailedWeeks) > 0 {
		return report, temporal.NewApplicationError(
			"projection capture failed for weeks: "+strings.Join(report.FailedWeeks, "; "),
			"ProjectionCaptureFailed", report)
	}
	return report, nil
}

// ProjectionPregameDispatcher runs hourly. It starts one ProjectionPregameCapture
// per verified kickoff window in the next 36 hours, keyed by the window's
// time, so repeat dispatches never double-schedule a window and a moved
// kickoff gets a fresh run while the old run finds its window gone.
func ProjectionPregameDispatcher(ctx workflow.Context) (int, error) {
	pa := &activities.ProjectionActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	logger := workflow.GetLogger(ctx)

	var windows []activities.KickoffWindow
	if err := workflow.ExecuteActivity(actCtx, pa.ListUpcomingKickoffWindows, activities.ListKickoffWindowsParams{Horizon: pregameDispatchHorizon}).Get(ctx, &windows); err != nil {
		return 0, err
	}
	started := 0
	for _, w := range windows {
		childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
			WorkflowID:            PregameCaptureWorkflowID(w),
			TaskQueue:             TaskQueueProjections,
			ParentClosePolicy:     enumspb.PARENT_CLOSE_POLICY_ABANDON,
			WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		})
		f := workflow.ExecuteChildWorkflow(childCtx, ProjectionPregameCapture, w)
		if err := f.GetChildWorkflowExecution().Get(ctx, nil); err != nil {
			var already *temporal.ChildWorkflowExecutionAlreadyStartedError
			if errors.As(err, &already) {
				continue
			}
			return started, fmt.Errorf("start pregame capture for %s: %w", PregameCaptureWorkflowID(w), err)
		}
		logger.Info("scheduled pregame projection capture", "season", w.Season, "week", w.Week, "kickoff", w.KickoffAt)
		started++
	}
	return started, nil
}

// PregameCaptureWorkflowID names the pregame capture run for a kickoff window.
func PregameCaptureWorkflowID(w activities.KickoffWindow) string {
	return fmt.Sprintf("sleeper-projections-pregame-%s-w%d-%d", w.Season, w.Week, w.KickoffAt.Unix())
}

// ProjectionPregameCapture sleeps until 60 minutes before the window's
// kickoff, confirms the window is still verified at that time, and captures
// the window's week.
func ProjectionPregameCapture(ctx workflow.Context, w activities.KickoffWindow) (activities.ProjectionCaptureResult, error) {
	pa := &activities.ProjectionActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	logger := workflow.GetLogger(ctx)

	if wait := w.KickoffAt.Add(-activities.PregameCaptureLead).Sub(workflow.Now(ctx)); wait > 0 {
		if err := workflow.Sleep(ctx, wait); err != nil {
			return activities.ProjectionCaptureResult{}, err
		}
	}

	var confirmed bool
	if err := workflow.ExecuteActivity(actCtx, pa.KickoffWindowConfirmed, w).Get(ctx, &confirmed); err != nil {
		return activities.ProjectionCaptureResult{}, err
	}
	res := activities.ProjectionCaptureResult{Season: w.Season, Week: w.Week, Trigger: models.ProjectionTriggerPregame}
	if !confirmed {
		res.Skipped = true
		res.SkipReason = "kickoff window no longer verified; replaced or withdrawn"
		logger.Warn("pregame projection window replaced", "season", w.Season, "week", w.Week, "kickoff", w.KickoffAt)
		return res, nil
	}

	kickoff := w.KickoffAt.UTC()
	err := workflow.ExecuteActivity(actCtx, pa.CaptureProjections, activities.CaptureProjectionsParams{
		CaptureKey:    workflow.GetInfo(ctx).WorkflowExecution.RunID,
		Season:        w.Season,
		Week:          w.Week,
		CurrentWeek:   w.Week,
		Trigger:       models.ProjectionTriggerPregame,
		WindowKickoff: &kickoff,
	}).Get(ctx, &res)
	if err != nil {
		return res, err
	}
	logCapture(logger, res)
	return res, nil
}

// ManualCaptureParams selects the week for ProjectionManualCapture. Empty
// Season or zero Week default to Sleeper's current NFL state.
type ManualCaptureParams struct {
	Season string
	Week   int
}

// ProjectionManualCapture is the operator entry point, started with
// `temporal workflow start --task-queue sleeper-projections --type
// ProjectionManualCapture`. It records a present-time observation under the
// same kickoff cutoffs as scheduled captures.
func ProjectionManualCapture(ctx workflow.Context, params ManualCaptureParams) (activities.ProjectionCaptureResult, error) {
	pa := &activities.ProjectionActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	season, current, ok, err := resolveProjectionWeek(ctx, actCtx)
	if err != nil {
		return activities.ProjectionCaptureResult{}, err
	}
	switch {
	case params.Season != "" && params.Season < season:
		// Every week of a past season has been reached.
		season, current = params.Season, activities.LastRegularSeasonWeek+1
	case params.Season != "" && params.Season > season:
		// Every week of a later season is a forward capture.
		season, current = params.Season, 0
	case !ok:
		current = 1
	}
	week := params.Week
	if week == 0 {
		week = current
	}

	var res activities.ProjectionCaptureResult
	err = workflow.ExecuteActivity(actCtx, pa.CaptureProjections, activities.CaptureProjectionsParams{
		CaptureKey:  workflow.GetInfo(ctx).WorkflowExecution.RunID,
		Season:      season,
		Week:        week,
		CurrentWeek: current,
		Trigger:     models.ProjectionTriggerManual,
	}).Get(ctx, &res)
	if err != nil {
		return res, err
	}
	logCapture(workflow.GetLogger(ctx), res)
	return res, nil
}

// resolveProjectionWeek returns the season and first week to capture. During
// the preseason it starts at Week 1; outside the preseason and regular season
// ok is false.
func resolveProjectionWeek(ctx, actCtx workflow.Context) (string, int, bool, error) {
	pa := &activities.ProjectionActivities{}
	var state sleeper.NFLState
	if err := workflow.ExecuteActivity(actCtx, pa.GetProjectionNFLState).Get(ctx, &state); err != nil {
		return "", 0, false, err
	}
	switch state.SeasonType {
	case "regular":
		week := max(state.Week, 1)
		return state.Season, week, week <= activities.LastRegularSeasonWeek, nil
	case "pre":
		return state.Season, 1, true, nil
	default:
		return state.Season, 0, false, nil
	}
}

func logCapture(logger log.Logger, res activities.ProjectionCaptureResult) {
	if res.Skipped {
		logger.Info("projection capture skipped", "season", res.Season, "week", res.Week, "trigger", res.Trigger, "reason", res.SkipReason)
		return
	}
	logger.Info("projection capture published",
		"season", res.Season, "week", res.Week, "trigger", res.Trigger,
		"capture_id", res.CaptureID, "idempotent", res.Idempotent,
		"phase", res.WeekPhase, "source_rows", res.SourceRows, "rows", res.Rows,
		"metadata_only_rows", res.MetadataOnlyRows, "status_observations", res.StatusObservations,
		"company_counts", res.CompanyCounts)
	if len(res.MissingCompanies) > 0 {
		logger.Warn("projection capture missing configured companies", "season", res.Season, "week", res.Week, "missing", res.MissingCompanies)
	}
	if res.ReplayEligible != nil && !*res.ReplayEligible {
		logger.Warn("projection capture not published for replay", "season", res.Season, "week", res.Week, "reason", res.ReplayNote)
	}
}
