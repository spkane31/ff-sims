package workflows_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/sleeper"
	"backend/internal/workflows"
)

func TestProjectionDailySweep_CapturesRemainingWeeksAndReportsFailures(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	pa := &activities.ProjectionActivities{}

	env.OnActivity(pa.GetProjectionNFLState, mock.Anything).
		Return(sleeper.NFLState{Season: "2026", SeasonType: "regular", Week: 17}, nil)
	env.OnActivity(pa.CaptureProjections, mock.Anything, mock.MatchedBy(func(p activities.CaptureProjectionsParams) bool {
		return p.Week == 17 && p.CurrentWeek == 17 && p.Trigger == models.ProjectionTriggerDailySweep && p.CaptureKey != ""
	})).Return(activities.ProjectionCaptureResult{Season: "2026", Week: 17, CaptureID: 1, Rows: 300}, nil).Once()
	env.OnActivity(pa.CaptureProjections, mock.Anything, mock.MatchedBy(func(p activities.CaptureProjectionsParams) bool {
		return p.Week == 18
	})).Return(activities.ProjectionCaptureResult{}, temporal.NewNonRetryableApplicationError("empty projection response", "x", nil)).Once()
	missed := activities.KickoffWindow{Season: "2026", Week: 17, KickoffAt: time.Date(2026, 12, 25, 18, 0, 0, 0, time.UTC)}
	env.OnActivity(pa.ReportProjectionHealth, mock.Anything, activities.ProjectionHealthParams{Season: "2026", Weeks: []int{17, 16}}).
		Return(activities.ProjectionHealth{MissedWindows: []activities.KickoffWindow{missed}, LastCaptureAge: time.Minute}, nil).Once()
	env.OnActivity(pa.PruneProjectionCaptures, mock.Anything).Return(int64(3), nil).Once()

	env.ExecuteWorkflow(workflows.ProjectionDailySweep)

	require.True(t, env.IsWorkflowCompleted())
	err := env.GetWorkflowError()
	require.Error(t, err)
	require.Contains(t, err.Error(), "18: ")
	var appErr *temporal.ApplicationError
	require.True(t, errors.As(err, &appErr))
	var report workflows.ProjectionSweepReport
	require.NoError(t, appErr.Details(&report))
	require.Len(t, report.Captures, 1)
	require.Len(t, report.FailedWeeks, 1)
	require.Equal(t, []activities.KickoffWindow{missed}, report.MissedWindows)
	require.Equal(t, int64(3), report.Pruned)
	env.AssertExpectations(t)
}

func TestProjectionDailySweep_SkipsOutsideSeason(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	pa := &activities.ProjectionActivities{}
	env.OnActivity(pa.GetProjectionNFLState, mock.Anything).
		Return(sleeper.NFLState{Season: "2026", SeasonType: "post", Week: 1}, nil)

	env.ExecuteWorkflow(workflows.ProjectionDailySweep)

	require.NoError(t, env.GetWorkflowError())
	env.AssertNotCalled(t, "CaptureProjections", mock.Anything, mock.Anything)
}

func TestProjectionPregameCapture_RunsAnHourBeforeKickoff(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	pa := &activities.ProjectionActivities{}
	kickoff := time.Date(2026, 12, 6, 18, 0, 0, 0, time.UTC)
	env.SetStartTime(kickoff.Add(-30 * time.Hour))
	w := activities.KickoffWindow{Season: "2026", Week: 13, KickoffAt: kickoff}

	env.OnActivity(pa.KickoffWindowConfirmed, mock.Anything, mock.Anything).Return(true, nil).Once()
	env.OnActivity(pa.CaptureProjections, mock.Anything, mock.MatchedBy(func(p activities.CaptureProjectionsParams) bool {
		return p.Week == 13 && p.CurrentWeek == 13 && p.Trigger == models.ProjectionTriggerPregame &&
			p.WindowKickoff != nil && p.WindowKickoff.Equal(kickoff)
	})).Run(func(mock.Arguments) {
		require.Equal(t, kickoff.Add(-time.Hour), env.Now().UTC())
	}).Return(activities.ProjectionCaptureResult{CaptureID: 7}, nil).Once()

	env.ExecuteWorkflow(workflows.ProjectionPregameCapture, w)

	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestProjectionPregameCapture_SkipsReplacedWindow(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	pa := &activities.ProjectionActivities{}
	kickoff := time.Date(2026, 12, 6, 18, 0, 0, 0, time.UTC)
	env.SetStartTime(kickoff.Add(-5 * time.Hour))
	env.OnActivity(pa.KickoffWindowConfirmed, mock.Anything, mock.Anything).Return(false, nil).Once()

	env.ExecuteWorkflow(workflows.ProjectionPregameCapture, activities.KickoffWindow{Season: "2026", Week: 13, KickoffAt: kickoff})

	require.NoError(t, env.GetWorkflowError())
	var res activities.ProjectionCaptureResult
	require.NoError(t, env.GetWorkflowResult(&res))
	require.True(t, res.Skipped)
	env.AssertNotCalled(t, "CaptureProjections", mock.Anything, mock.Anything)
}

func TestProjectionPregameDispatcher_StartsOneRunPerWindow(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	pa := &activities.ProjectionActivities{}
	early := activities.KickoffWindow{Season: "2026", Week: 13, KickoffAt: time.Date(2026, 12, 6, 18, 0, 0, 0, time.UTC)}
	late := activities.KickoffWindow{Season: "2026", Week: 13, KickoffAt: time.Date(2026, 12, 6, 21, 25, 0, 0, time.UTC)}
	env.OnActivity(pa.ListUpcomingKickoffWindows, mock.Anything, mock.Anything).
		Return([]activities.KickoffWindow{early, late}, nil)
	env.RegisterWorkflow(workflows.ProjectionPregameCapture)
	env.OnWorkflow(workflows.ProjectionPregameCapture, mock.Anything, early).Return(activities.ProjectionCaptureResult{}, nil).Once()
	env.OnWorkflow(workflows.ProjectionPregameCapture, mock.Anything, late).Return(activities.ProjectionCaptureResult{}, nil).Once()

	env.ExecuteWorkflow(workflows.ProjectionPregameDispatcher)

	require.NoError(t, env.GetWorkflowError())
	var started int
	require.NoError(t, env.GetWorkflowResult(&started))
	require.Equal(t, 2, started)
	require.Equal(t, "sleeper-projections-pregame-2026-w13-1796580000", workflows.PregameCaptureWorkflowID(early))
}

func TestProjectionManualCapture_DefaultsToCurrentWeek(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	pa := &activities.ProjectionActivities{}
	env.OnActivity(pa.GetProjectionNFLState, mock.Anything).
		Return(sleeper.NFLState{Season: "2026", SeasonType: "regular", Week: 13}, nil)
	env.OnActivity(pa.CaptureProjections, mock.Anything, mock.MatchedBy(func(p activities.CaptureProjectionsParams) bool {
		return p.Season == "2026" && p.Week == 13 && p.CurrentWeek == 13 && p.Trigger == models.ProjectionTriggerManual
	})).Return(activities.ProjectionCaptureResult{CaptureID: 9}, nil).Once()

	env.ExecuteWorkflow(workflows.ProjectionManualCapture, workflows.ManualCaptureParams{})

	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}
