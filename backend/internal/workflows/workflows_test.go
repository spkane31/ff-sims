package workflows_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/workflows"
)

// ---- DiscoveryBatchDispatcher ----

func TestDispatcher_SpawnsChildWorkflows(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	da := &activities.DiscoveryActivities{}
	env.OnActivity(da.GetStaleUsers, mock.Anything, activities.GetStaleUsersParams{BatchSize: workflows.BatchSize}).
		Return([]string{"u1", "u2"}, nil)

	env.RegisterWorkflow(workflows.UserDiscoveryWorkflow)
	env.OnWorkflow(workflows.UserDiscoveryWorkflow, mock.Anything, workflows.UserDiscoveryParams{UserID: "u1"}).Return(nil)
	env.OnWorkflow(workflows.UserDiscoveryWorkflow, mock.Anything, workflows.UserDiscoveryParams{UserID: "u2"}).Return(nil)

	env.ExecuteWorkflow(workflows.DiscoveryBatchDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestDispatcher_ChildWorkflowIDIsUserScoped(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	da := &activities.DiscoveryActivities{}
	env.OnActivity(da.GetStaleUsers, mock.Anything, activities.GetStaleUsersParams{BatchSize: workflows.BatchSize}).
		Return([]string{"u1", "u2"}, nil)

	env.RegisterWorkflow(workflows.UserDiscoveryWorkflow)

	seenIDs := make(map[string]bool)
	captureID := func(ctx context.Context) bool {
		seenIDs[activity.GetInfo(ctx).WorkflowExecution.ID] = true
		return true
	}
	env.OnActivity(da.FetchUserLeagues, mock.MatchedBy(captureID), activities.FetchUserLeaguesParams{UserID: "u1"}).Return([]string{}, nil)
	env.OnActivity(da.FetchUserLeagues, mock.MatchedBy(captureID), activities.FetchUserLeaguesParams{UserID: "u2"}).Return([]string{}, nil)
	env.OnActivity(da.MarkUserFetched, mock.Anything, activities.MarkUserFetchedParams{UserID: "u1"}).Return(nil)
	env.OnActivity(da.MarkUserFetched, mock.Anything, activities.MarkUserFetchedParams{UserID: "u2"}).Return(nil)

	env.ExecuteWorkflow(workflows.DiscoveryBatchDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.True(t, seenIDs["user-discovery-u1"], "expected child workflow ID %q to have been used", "user-discovery-u1")
	require.True(t, seenIDs["user-discovery-u2"], "expected child workflow ID %q to have been used", "user-discovery-u2")
}

func TestDispatcher_EmptyBatch(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	da := &activities.DiscoveryActivities{}
	env.OnActivity(da.GetStaleUsers, mock.Anything, activities.GetStaleUsersParams{BatchSize: workflows.BatchSize}).Return([]string{}, nil)

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

// ---- DraftSyncDispatcher ----

func TestDraftSyncDispatcher_SpawnsChildWorkflows(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.GetStaleLeaguesForDrafts, mock.Anything, activities.GetStaleLeaguesParams{BatchSize: workflows.SyncBatchSize}).
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
	env.OnActivity(dfa.GetStaleLeaguesForDrafts, mock.Anything, activities.GetStaleLeaguesParams{BatchSize: workflows.SyncBatchSize}).
		Return([]string{}, nil)

	env.ExecuteWorkflow(workflows.DraftSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}

func TestDraftSyncDispatcher_ChildWorkflowIDIsLeagueScoped(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.GetStaleLeaguesForDrafts, mock.Anything, activities.GetStaleLeaguesParams{BatchSize: workflows.SyncBatchSize}).
		Return([]string{"lg1", "lg2"}, nil)

	env.RegisterWorkflow(workflows.LeagueDraftSyncWorkflow)

	seenIDs := make(map[string]bool)
	captureID := func(ctx context.Context) bool {
		seenIDs[activity.GetInfo(ctx).WorkflowExecution.ID] = true
		return true
	}
	env.OnActivity(dfa.FetchLeagueDrafts, mock.MatchedBy(captureID), activities.FetchLeagueDraftsParams{LeagueID: "lg1"}).Return([]string{}, nil)
	env.OnActivity(dfa.FetchLeagueDrafts, mock.MatchedBy(captureID), activities.FetchLeagueDraftsParams{LeagueID: "lg2"}).Return([]string{}, nil)
	env.OnActivity(dfa.MarkLeagueDraftsFetched, mock.Anything, activities.MarkLeagueFetchedParams{LeagueID: "lg1"}).Return(nil)
	env.OnActivity(dfa.MarkLeagueDraftsFetched, mock.Anything, activities.MarkLeagueFetchedParams{LeagueID: "lg2"}).Return(nil)

	env.ExecuteWorkflow(workflows.DraftSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.True(t, seenIDs["draft-sync-lg1"], "expected child workflow ID %q to have been used", "draft-sync-lg1")
	require.True(t, seenIDs["draft-sync-lg2"], "expected child workflow ID %q to have been used", "draft-sync-lg2")
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
	env.OnActivity(dfa.GetStaleLeaguesForTransactions, mock.Anything, activities.GetStaleLeaguesParams{BatchSize: workflows.SyncBatchSize}).
		Return([]activities.LeagueTransactionState{{LeagueID: "lg1"}}, nil)

	env.RegisterWorkflow(workflows.LeagueTransactionSyncWorkflow)
	env.OnWorkflow(workflows.LeagueTransactionSyncWorkflow, mock.Anything, workflows.LeagueSyncParams{LeagueID: "lg1"}).Return(nil)

	env.ExecuteWorkflow(workflows.TransactionSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestTransactionSyncDispatcher_ChildWorkflowIDIsLeagueScoped(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.GetStaleLeaguesForTransactions, mock.Anything, activities.GetStaleLeaguesParams{BatchSize: workflows.SyncBatchSize}).
		Return([]activities.LeagueTransactionState{{LeagueID: "lg1"}, {LeagueID: "lg2"}}, nil)

	env.RegisterWorkflow(workflows.LeagueTransactionSyncWorkflow)

	seenIDs := make(map[string]bool)
	captureID := func(ctx context.Context) bool {
		seenIDs[activity.GetInfo(ctx).WorkflowExecution.ID] = true
		return true
	}
	env.OnActivity(dfa.FetchLeagueTransactions, mock.MatchedBy(captureID), activities.FetchLeagueTransactionsParams{LeagueID: "lg1"}).Return(0, nil)
	env.OnActivity(dfa.FetchLeagueTransactions, mock.MatchedBy(captureID), activities.FetchLeagueTransactionsParams{LeagueID: "lg2"}).Return(0, nil)
	env.OnActivity(dfa.MarkLeagueTransactionsFetched, mock.Anything, activities.MarkLeagueTransactionsFetchedParams{LeagueID: "lg1", MaxLeg: 0}).Return(nil)
	env.OnActivity(dfa.MarkLeagueTransactionsFetched, mock.Anything, activities.MarkLeagueTransactionsFetchedParams{LeagueID: "lg2", MaxLeg: 0}).Return(nil)

	env.ExecuteWorkflow(workflows.TransactionSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.True(t, seenIDs["transaction-sync-lg1"], "expected child workflow ID %q to have been used", "transaction-sync-lg1")
	require.True(t, seenIDs["transaction-sync-lg2"], "expected child workflow ID %q to have been used", "transaction-sync-lg2")
}

// ---- LeagueTransactionSyncWorkflow ----

func TestLeagueTransactionSync_FullPath(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.FetchLeagueTransactions, mock.Anything, activities.FetchLeagueTransactionsParams{LeagueID: "lg1"}).Return(5, nil)
	env.OnActivity(dfa.MarkLeagueTransactionsFetched, mock.Anything, activities.MarkLeagueTransactionsFetchedParams{LeagueID: "lg1", MaxLeg: 5}).Return(nil)

	env.ExecuteWorkflow(workflows.LeagueTransactionSyncWorkflow, workflows.LeagueSyncParams{LeagueID: "lg1"})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestLeagueTransactionSync_WithLegCursor(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	lastLeg := 8
	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.FetchLeagueTransactions, mock.Anything, activities.FetchLeagueTransactionsParams{
		LeagueID:       "lg1",
		LastLegFetched: &lastLeg,
	}).Return(10, nil)
	env.OnActivity(dfa.MarkLeagueTransactionsFetched, mock.Anything, activities.MarkLeagueTransactionsFetchedParams{LeagueID: "lg1", MaxLeg: 10}).Return(nil)

	env.ExecuteWorkflow(workflows.LeagueTransactionSyncWorkflow, workflows.LeagueSyncParams{LeagueID: "lg1", LastLegFetched: &lastLeg})

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

// ---- SyncWeekStats ----

func TestSyncWeekStats_SkipsFinalizedWeeks(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	wsa := &activities.WeekStatsActivities{}
	// Weeks 1 and 2 already finalized — only weeks 3-18 should be fetched.
	env.OnActivity(wsa.GetFinalizedWeeks, mock.Anything, activities.GetFinalizedWeeksParams{Season: "2025"}).
		Return([]int{1, 2}, nil)
	for week := 3; week <= 18; week++ {
		env.OnActivity(wsa.FetchWeekStats, mock.Anything, activities.FetchWeekStatsParams{Season: "2025", Week: week}).Return(nil)
	}

	env.ExecuteWorkflow(workflows.SyncWeekStats, workflows.SyncWeekStatsParams{Season: "2025"})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestSyncWeekStats_AllWeeksFinalized_NoFetchCalls(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	allWeeks := make([]int, 0, 18)
	for w := 1; w <= 18; w++ {
		allWeeks = append(allWeeks, w)
	}

	wsa := &activities.WeekStatsActivities{}
	env.OnActivity(wsa.GetFinalizedWeeks, mock.Anything, activities.GetFinalizedWeeksParams{Season: "2025"}).
		Return(allWeeks, nil)

	env.ExecuteWorkflow(workflows.SyncWeekStats, workflows.SyncWeekStatsParams{Season: "2025"})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

// ---- WeekStatsSyncDispatcher ----

func TestWeekStatsSyncDispatcher_ResolvesSeasonAndSyncs(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	wsa := &activities.WeekStatsActivities{}
	env.OnActivity(wsa.GetCurrentSeason, mock.Anything).Return("2025", nil)
	env.OnActivity(wsa.GetFinalizedWeeks, mock.Anything, activities.GetFinalizedWeeksParams{Season: "2025"}).
		Return([]int{}, nil)
	for week := 1; week <= 18; week++ {
		env.OnActivity(wsa.FetchWeekStats, mock.Anything, activities.FetchWeekStatsParams{Season: "2025", Week: week}).Return(nil)
	}

	env.ExecuteWorkflow(workflows.WeekStatsSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

// ---- ADPRollupDispatcher ----

func TestADPRollupDispatcher_SpawnsChildPerSeasonSegment(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	ara := &activities.ADPRollupActivities{}
	env.OnActivity(ara.ListADPSeasons, mock.Anything).Return([]string{"2024"}, nil)

	env.RegisterWorkflow(workflows.SegmentSeasonADPRollupWorkflow)
	segments := models.AllADPSegments()
	if len(segments) != 24 {
		t.Fatalf("expected 24 segments, got %d", len(segments))
	}
	for _, seg := range segments {
		env.OnWorkflow(workflows.SegmentSeasonADPRollupWorkflow, mock.Anything, workflows.SegmentSeasonADPParams{
			Segment: seg,
			Season:  "2024",
		}).Return(nil)
	}

	env.ExecuteWorkflow(workflows.ADPRollupDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestADPRollupDispatcher_ChildWorkflowIDIsDeterministic(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	ara := &activities.ADPRollupActivities{}
	env.OnActivity(ara.ListADPSeasons, mock.Anything).Return([]string{"2024"}, nil)

	env.RegisterWorkflow(workflows.SegmentSeasonADPRollupWorkflow)

	seenIDs := make(map[string]bool)
	for _, seg := range models.AllADPSegments() {
		env.OnActivity(ara.ComputeSegmentSeasonADP, mock.MatchedBy(func(ctx context.Context) bool {
			seenIDs[activity.GetInfo(ctx).WorkflowExecution.ID] = true
			return true
		}), activities.ComputeSegmentSeasonADPParams{Segment: seg, Season: "2024"}).Return(nil)
	}

	env.ExecuteWorkflow(workflows.ADPRollupDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	for _, seg := range models.AllADPSegments() {
		wantID := "2024-" + seg.Key()
		require.True(t, seenIDs[wantID], "expected child workflow ID %q to have been used", wantID)
	}
}

func TestADPRollupDispatcher_NoSeasons_NoChildren(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	ara := &activities.ADPRollupActivities{}
	env.OnActivity(ara.ListADPSeasons, mock.Anything).Return([]string{}, nil)

	env.ExecuteWorkflow(workflows.ADPRollupDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}

// ---- SegmentSeasonADPRollupWorkflow ----

func TestSegmentSeasonADPRollupWorkflow_CallsComputeActivity(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	seg := models.ADPSegment{LeagueSize: "12", ScoringFormat: "ppr", Superflex: true}
	ara := &activities.ADPRollupActivities{}
	env.OnActivity(ara.ComputeSegmentSeasonADP, mock.Anything, activities.ComputeSegmentSeasonADPParams{
		Segment: seg,
		Season:  "2024",
	}).Return(nil)

	env.ExecuteWorkflow(workflows.SegmentSeasonADPRollupWorkflow, workflows.SegmentSeasonADPParams{
		Segment: seg,
		Season:  "2024",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestSegmentSeasonADPRollupWorkflow_ActivityFailure_WorkflowStillSucceeds(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	seg := models.ADPSegment{LeagueSize: "12", ScoringFormat: "ppr", Superflex: true}
	ara := &activities.ADPRollupActivities{}
	env.OnActivity(ara.ComputeSegmentSeasonADP, mock.Anything, activities.ComputeSegmentSeasonADPParams{
		Segment: seg,
		Season:  "2024",
	}).Return(temporal.NewApplicationError("db error", "DB_ERROR", nil))

	env.ExecuteWorkflow(workflows.SegmentSeasonADPRollupWorkflow, workflows.SegmentSeasonADPParams{
		Segment: seg,
		Season:  "2024",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError()) // logged and swallowed, not propagated
	env.AssertExpectations(t)
}
