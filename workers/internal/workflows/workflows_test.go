package workflows_test

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"workers/internal/activities"
	"workers/internal/workflows"
)

// ---- DiscoveryBatchDispatcher ----

func TestDispatcher_SpawnsChildWorkflows(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	da := &activities.DiscoveryActivities{}
	dfa := &activities.DataFetchActivities{}

	env.OnActivity(da.GetStaleUsers, mock.Anything, workflows.BatchSize).
		Return([]string{"u1", "u2"}, nil)
	env.OnActivity(dfa.GetStaleLeagues, mock.Anything, workflows.BatchSize).
		Return([]string{"lg1"}, nil)

	env.RegisterWorkflow(workflows.UserDiscoveryWorkflow)
	env.RegisterWorkflow(workflows.LeagueSyncWorkflow)
	env.OnWorkflow(workflows.UserDiscoveryWorkflow, mock.Anything, "u1").Return(nil)
	env.OnWorkflow(workflows.UserDiscoveryWorkflow, mock.Anything, "u2").Return(nil)
	env.OnWorkflow(workflows.LeagueSyncWorkflow, mock.Anything, "lg1").Return(nil)

	env.ExecuteWorkflow(workflows.DiscoveryBatchDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestDispatcher_EmptyBatch(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	da := &activities.DiscoveryActivities{}
	dfa := &activities.DataFetchActivities{}

	env.OnActivity(da.GetStaleUsers, mock.Anything, workflows.BatchSize).Return([]string{}, nil)
	env.OnActivity(dfa.GetStaleLeagues, mock.Anything, workflows.BatchSize).Return([]string{}, nil)

	env.ExecuteWorkflow(workflows.DiscoveryBatchDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}

// ---- UserDiscoveryWorkflow ----

func TestUserDiscovery_CallsMarkFetchedOnSuccess(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	da := &activities.DiscoveryActivities{}
	env.OnActivity(da.FetchUserLeagues, mock.Anything, "u1").Return([]string{"lg1"}, nil)
	env.OnActivity(da.FetchLeagueMembers, mock.Anything, "lg1").Return(nil)
	env.OnActivity(da.MarkUserFetched, mock.Anything, "u1").Return(nil)

	env.ExecuteWorkflow(workflows.UserDiscoveryWorkflow, "u1")

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestUserDiscovery_NotFoundCallsSkip(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	da := &activities.DiscoveryActivities{}
	env.OnActivity(da.FetchUserLeagues, mock.Anything, "ghost").
		Return(nil, temporal.NewNonRetryableApplicationError("user not found", "NOT_FOUND", nil))
	env.OnActivity(da.MarkUserSkipped, mock.Anything, "ghost").Return(nil)

	env.ExecuteWorkflow(workflows.UserDiscoveryWorkflow, "ghost")

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestUserDiscovery_MemberFetchFailureContinues(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	da := &activities.DiscoveryActivities{}
	env.OnActivity(da.FetchUserLeagues, mock.Anything, "u1").
		Return([]string{"lg1", "lg2"}, nil)
	// lg1 fails, lg2 succeeds — workflow should still complete
	env.OnActivity(da.FetchLeagueMembers, mock.Anything, "lg1").
		Return(temporal.NewApplicationError("network error", "NETWORK", nil))
	env.OnActivity(da.FetchLeagueMembers, mock.Anything, "lg2").Return(nil)
	env.OnActivity(da.MarkUserFetched, mock.Anything, "u1").Return(nil)

	env.ExecuteWorkflow(workflows.UserDiscoveryWorkflow, "u1")

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

// ---- LeagueSyncWorkflow ----

func TestLeagueSync_FullPath(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.FetchLeagueDetails, mock.Anything, "lg1").Return(nil)
	env.OnActivity(dfa.FetchLeagueDrafts, mock.Anything, "lg1").Return([]string{"d1", "d2"}, nil)
	env.OnActivity(dfa.FetchDraftPicks, mock.Anything, "d1").Return(nil)
	env.OnActivity(dfa.FetchDraftPicks, mock.Anything, "d2").Return(nil)
	env.OnActivity(dfa.FetchLeagueTransactions, mock.Anything, "lg1").Return(nil)
	env.OnActivity(dfa.MarkLeagueFetched, mock.Anything, "lg1").Return(nil)

	env.ExecuteWorkflow(workflows.LeagueSyncWorkflow, "lg1")

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestLeagueSync_NotFoundCallsSkip(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.FetchLeagueDetails, mock.Anything, "gone").
		Return(temporal.NewNonRetryableApplicationError("league not found", "NOT_FOUND", nil))
	env.OnActivity(dfa.MarkLeagueSkipped, mock.Anything, "gone").Return(nil)

	env.ExecuteWorkflow(workflows.LeagueSyncWorkflow, "gone")

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestLeagueSync_DraftPicksFailureContinues(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.FetchLeagueDetails, mock.Anything, "lg1").Return(nil)
	env.OnActivity(dfa.FetchLeagueDrafts, mock.Anything, "lg1").Return([]string{"d1"}, nil)
	env.OnActivity(dfa.FetchDraftPicks, mock.Anything, "d1").
		Return(temporal.NewApplicationError("timeout", "TIMEOUT", nil))
	env.OnActivity(dfa.FetchLeagueTransactions, mock.Anything, "lg1").Return(nil)
	env.OnActivity(dfa.MarkLeagueFetched, mock.Anything, "lg1").Return(nil)

	env.ExecuteWorkflow(workflows.LeagueSyncWorkflow, "lg1")

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

// ---- PlayerDatabaseSyncWorkflow ----

func TestPlayerSync_CallsFetchAndUpsert(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	psa := &activities.PlayerSyncActivities{}
	env.OnActivity(psa.FetchAndUpsertAllPlayers, mock.Anything).Return(nil)

	env.ExecuteWorkflow(workflows.PlayerDatabaseSyncWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}
