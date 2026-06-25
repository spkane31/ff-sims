package workflows_test

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"backend/internal/activities"
	"backend/internal/workflows"
)

// ---- DiscoveryBatchDispatcher ----

func TestDispatcher_SpawnsChildWorkflows(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	da := &activities.DiscoveryActivities{}
	dfa := &activities.DataFetchActivities{}

	env.OnActivity(da.GetStaleUsers, mock.Anything, activities.GetStaleUsersParams{BatchSize: workflows.BatchSize}).
		Return([]string{"u1", "u2"}, nil)
	env.OnActivity(dfa.GetStaleLeagues, mock.Anything, activities.GetStaleLeaguesParams{BatchSize: workflows.BatchSize}).
		Return([]string{"lg1"}, nil)

	env.RegisterWorkflow(workflows.UserDiscoveryWorkflow)
	env.RegisterWorkflow(workflows.LeagueSyncWorkflow)
	env.OnWorkflow(workflows.UserDiscoveryWorkflow, mock.Anything, workflows.UserDiscoveryParams{UserID: "u1"}).Return(nil)
	env.OnWorkflow(workflows.UserDiscoveryWorkflow, mock.Anything, workflows.UserDiscoveryParams{UserID: "u2"}).Return(nil)
	env.OnWorkflow(workflows.LeagueSyncWorkflow, mock.Anything, workflows.LeagueSyncParams{LeagueID: "lg1"}).Return(nil)

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

	env.OnActivity(da.GetStaleUsers, mock.Anything, activities.GetStaleUsersParams{BatchSize: workflows.BatchSize}).Return([]string{}, nil)
	env.OnActivity(dfa.GetStaleLeagues, mock.Anything, activities.GetStaleLeaguesParams{BatchSize: workflows.BatchSize}).Return([]string{}, nil)

	env.ExecuteWorkflow(workflows.DiscoveryBatchDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}

// ---- UserDiscoveryWorkflow ----

func TestUserDiscovery_CallsMarkFetchedOnSuccess(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	da := &activities.DiscoveryActivities{}
	env.OnActivity(da.FetchUserLeagues, mock.Anything, activities.FetchUserLeaguesParams{UserID: "u1"}).Return([]string{"lg1"}, nil)
	env.OnActivity(da.FetchLeagueMembers, mock.Anything, activities.FetchLeagueMembersParams{LeagueID: "lg1"}).Return(nil)
	env.OnActivity(da.FetchLeagueDetails, mock.Anything, activities.FetchLeagueDetailsParams{LeagueID: "lg1"}).Return(nil)
	env.OnActivity(da.MarkUserFetched, mock.Anything, activities.MarkUserFetchedParams{UserID: "u1"}).Return(nil)

	env.ExecuteWorkflow(workflows.UserDiscoveryWorkflow, workflows.UserDiscoveryParams{UserID: "u1"})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestUserDiscovery_NotFoundCallsSkip(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	da := &activities.DiscoveryActivities{}
	env.OnActivity(da.FetchUserLeagues, mock.Anything, activities.FetchUserLeaguesParams{UserID: "ghost"}).
		Return(nil, temporal.NewNonRetryableApplicationError("user not found", "NOT_FOUND", nil))
	env.OnActivity(da.MarkUserSkipped, mock.Anything, activities.MarkUserSkippedParams{UserID: "ghost"}).Return(nil)

	env.ExecuteWorkflow(workflows.UserDiscoveryWorkflow, workflows.UserDiscoveryParams{UserID: "ghost"})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestUserDiscovery_MemberFetchFailureContinues(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	da := &activities.DiscoveryActivities{}
	env.OnActivity(da.FetchUserLeagues, mock.Anything, activities.FetchUserLeaguesParams{UserID: "u1"}).
		Return([]string{"lg1", "lg2"}, nil)
	// lg1 member fetch fails, but FetchLeagueDetails still runs for both
	env.OnActivity(da.FetchLeagueMembers, mock.Anything, activities.FetchLeagueMembersParams{LeagueID: "lg1"}).
		Return(temporal.NewApplicationError("network error", "NETWORK", nil))
	env.OnActivity(da.FetchLeagueDetails, mock.Anything, activities.FetchLeagueDetailsParams{LeagueID: "lg1"}).Return(nil)
	env.OnActivity(da.FetchLeagueMembers, mock.Anything, activities.FetchLeagueMembersParams{LeagueID: "lg2"}).Return(nil)
	env.OnActivity(da.FetchLeagueDetails, mock.Anything, activities.FetchLeagueDetailsParams{LeagueID: "lg2"}).Return(nil)
	env.OnActivity(da.MarkUserFetched, mock.Anything, activities.MarkUserFetchedParams{UserID: "u1"}).Return(nil)

	env.ExecuteWorkflow(workflows.UserDiscoveryWorkflow, workflows.UserDiscoveryParams{UserID: "u1"})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

// ---- LeagueSyncWorkflow ----

func TestLeagueSync_FullPath(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.FetchLeagueDetails, mock.Anything, activities.FetchLeagueDetailsParams{LeagueID: "lg1"}).Return(nil)
	env.OnActivity(dfa.FetchLeagueDrafts, mock.Anything, activities.FetchLeagueDraftsParams{LeagueID: "lg1"}).Return([]string{"d1", "d2"}, nil)
	env.OnActivity(dfa.FetchDraftPicks, mock.Anything, activities.FetchDraftPicksParams{DraftID: "d1"}).Return(nil)
	env.OnActivity(dfa.FetchDraftPicks, mock.Anything, activities.FetchDraftPicksParams{DraftID: "d2"}).Return(nil)
	env.OnActivity(dfa.FetchLeagueTransactions, mock.Anything, activities.FetchLeagueTransactionsParams{LeagueID: "lg1"}).Return(nil)
	env.OnActivity(dfa.MarkLeagueFetched, mock.Anything, activities.MarkLeagueFetchedParams{LeagueID: "lg1"}).Return(nil)

	env.ExecuteWorkflow(workflows.LeagueSyncWorkflow, workflows.LeagueSyncParams{LeagueID: "lg1"})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestLeagueSync_NotFoundCallsSkip(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.FetchLeagueDetails, mock.Anything, activities.FetchLeagueDetailsParams{LeagueID: "gone"}).
		Return(temporal.NewNonRetryableApplicationError("league not found", "NOT_FOUND", nil))
	env.OnActivity(dfa.MarkLeagueSkipped, mock.Anything, activities.MarkLeagueSkippedParams{LeagueID: "gone"}).Return(nil)

	env.ExecuteWorkflow(workflows.LeagueSyncWorkflow, workflows.LeagueSyncParams{LeagueID: "gone"})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestLeagueSync_DraftPicksFailureContinues(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.FetchLeagueDetails, mock.Anything, activities.FetchLeagueDetailsParams{LeagueID: "lg1"}).Return(nil)
	env.OnActivity(dfa.FetchLeagueDrafts, mock.Anything, activities.FetchLeagueDraftsParams{LeagueID: "lg1"}).Return([]string{"d1"}, nil)
	env.OnActivity(dfa.FetchDraftPicks, mock.Anything, activities.FetchDraftPicksParams{DraftID: "d1"}).
		Return(temporal.NewApplicationError("timeout", "TIMEOUT", nil))
	env.OnActivity(dfa.FetchLeagueTransactions, mock.Anything, activities.FetchLeagueTransactionsParams{LeagueID: "lg1"}).Return(nil)
	env.OnActivity(dfa.MarkLeagueFetched, mock.Anything, activities.MarkLeagueFetchedParams{LeagueID: "lg1"}).Return(nil)

	env.ExecuteWorkflow(workflows.LeagueSyncWorkflow, workflows.LeagueSyncParams{LeagueID: "lg1"})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

// ---- DraftSyncDispatcher ----

func TestDraftSyncDispatcher_SpawnsChildWorkflows(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.GetStaleLeaguesForDrafts, mock.Anything, activities.GetStaleLeaguesParams{BatchSize: workflows.BatchSize}).
		Return([]string{"lg1", "lg2"}, nil)

	env.RegisterWorkflow(workflows.LeagueDraftSyncWorkflow)
	env.OnWorkflow(workflows.LeagueDraftSyncWorkflow, mock.Anything, workflows.LeagueSyncParams{LeagueID: "lg1"}).Return(nil)
	env.OnWorkflow(workflows.LeagueDraftSyncWorkflow, mock.Anything, workflows.LeagueSyncParams{LeagueID: "lg2"}).Return(nil)

	env.ExecuteWorkflow(workflows.DraftSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestDraftSyncDispatcher_EmptyBatch(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.GetStaleLeaguesForDrafts, mock.Anything, activities.GetStaleLeaguesParams{BatchSize: workflows.BatchSize}).
		Return([]string{}, nil)

	env.ExecuteWorkflow(workflows.DraftSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}

// ---- LeagueDraftSyncWorkflow ----

func TestLeagueDraftSync_FullPath(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.FetchLeagueDrafts, mock.Anything, activities.FetchLeagueDraftsParams{LeagueID: "lg1"}).
		Return([]string{"d1", "d2"}, nil)
	env.OnActivity(dfa.FetchDraftPicks, mock.Anything, activities.FetchDraftPicksParams{DraftID: "d1"}).Return(nil)
	env.OnActivity(dfa.FetchDraftPicks, mock.Anything, activities.FetchDraftPicksParams{DraftID: "d2"}).Return(nil)
	env.OnActivity(dfa.MarkLeagueDraftsFetched, mock.Anything, activities.MarkLeagueFetchedParams{LeagueID: "lg1"}).Return(nil)

	env.ExecuteWorkflow(workflows.LeagueDraftSyncWorkflow, workflows.LeagueSyncParams{LeagueID: "lg1"})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestLeagueDraftSync_NotFoundCallsSkip(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.FetchLeagueDrafts, mock.Anything, activities.FetchLeagueDraftsParams{LeagueID: "gone"}).
		Return(nil, temporal.NewNonRetryableApplicationError("league not found", "NOT_FOUND", nil))
	env.OnActivity(dfa.MarkLeagueSkipped, mock.Anything, activities.MarkLeagueSkippedParams{LeagueID: "gone"}).Return(nil)

	env.ExecuteWorkflow(workflows.LeagueDraftSyncWorkflow, workflows.LeagueSyncParams{LeagueID: "gone"})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestLeagueDraftSync_PicksFailureContinues(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.FetchLeagueDrafts, mock.Anything, activities.FetchLeagueDraftsParams{LeagueID: "lg1"}).
		Return([]string{"d1"}, nil)
	env.OnActivity(dfa.FetchDraftPicks, mock.Anything, activities.FetchDraftPicksParams{DraftID: "d1"}).
		Return(temporal.NewApplicationError("timeout", "TIMEOUT", nil))
	env.OnActivity(dfa.MarkLeagueDraftsFetched, mock.Anything, activities.MarkLeagueFetchedParams{LeagueID: "lg1"}).Return(nil)

	env.ExecuteWorkflow(workflows.LeagueDraftSyncWorkflow, workflows.LeagueSyncParams{LeagueID: "lg1"})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

// ---- TransactionSyncDispatcher ----

func TestTransactionSyncDispatcher_SpawnsChildWorkflows(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.GetStaleLeaguesForTransactions, mock.Anything, activities.GetStaleLeaguesParams{BatchSize: workflows.BatchSize}).
		Return([]string{"lg1"}, nil)

	env.RegisterWorkflow(workflows.LeagueTransactionSyncWorkflow)
	env.OnWorkflow(workflows.LeagueTransactionSyncWorkflow, mock.Anything, workflows.LeagueSyncParams{LeagueID: "lg1"}).Return(nil)

	env.ExecuteWorkflow(workflows.TransactionSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

// ---- LeagueTransactionSyncWorkflow ----

func TestLeagueTransactionSync_FullPath(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.FetchLeagueTransactions, mock.Anything, activities.FetchLeagueTransactionsParams{LeagueID: "lg1"}).Return(nil)
	env.OnActivity(dfa.MarkLeagueTransactionsFetched, mock.Anything, activities.MarkLeagueFetchedParams{LeagueID: "lg1"}).Return(nil)

	env.ExecuteWorkflow(workflows.LeagueTransactionSyncWorkflow, workflows.LeagueSyncParams{LeagueID: "lg1"})

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
