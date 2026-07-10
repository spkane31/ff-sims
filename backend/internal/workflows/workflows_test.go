package workflows_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
	"backend/internal/models"
	"backend/internal/workflows"
)

// ---- DiscoveryBatchDispatcher ----

func TestDiscoveryDispatcher_DrainsUntilShortClaim(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	da := &activities.DiscoveryActivities{}
	cfg := activities.DiscoveryConfig{ParallelBatches: 2, BatchSize: 2, Concurrency: 4}
	env.OnActivity(da.GetDiscoveryConfig, mock.Anything).Return(cfg, nil)

	full := []string{"u1", "u2"}
	short := []string{"u3"}
	// First claim full, second claim short -> dispatcher must stop claiming after the short one.
	env.OnActivity(da.ClaimStaleUsers, mock.Anything, activities.ClaimStaleUsersParams{BatchSize: 2}).
		Return(full, nil).Once()
	env.OnActivity(da.ClaimStaleUsers, mock.Anything, activities.ClaimStaleUsersParams{BatchSize: 2}).
		Return(short, nil).Once()

	env.OnActivity(da.DiscoverUsersBatch, mock.Anything, activities.DiscoverUsersBatchParams{UserIDs: full, Concurrency: 4}).
		Return(activities.SyncBatchResult{Processed: 2}, nil).Once()
	env.OnActivity(da.DiscoverUsersBatch, mock.Anything, activities.DiscoverUsersBatchParams{UserIDs: short, Concurrency: 4}).
		Return(activities.SyncBatchResult{Processed: 1}, nil).Once()

	env.ExecuteWorkflow(workflows.DiscoveryBatchDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestDiscoveryDispatcher_EmptyClaimStopsImmediately(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	da := &activities.DiscoveryActivities{}
	env.OnActivity(da.GetDiscoveryConfig, mock.Anything).
		Return(activities.DiscoveryConfig{ParallelBatches: 2, BatchSize: 50, Concurrency: 8}, nil)
	env.OnActivity(da.ClaimStaleUsers, mock.Anything, activities.ClaimStaleUsersParams{BatchSize: 50}).
		Return([]string{}, nil).Once()

	env.ExecuteWorkflow(workflows.DiscoveryBatchDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestDiscoveryDispatcher_BatchFailureDoesNotFailRun(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	da := &activities.DiscoveryActivities{}
	env.OnActivity(da.GetDiscoveryConfig, mock.Anything).
		Return(activities.DiscoveryConfig{ParallelBatches: 1, BatchSize: 2, Concurrency: 4}, nil)
	env.OnActivity(da.ClaimStaleUsers, mock.Anything, mock.Anything).Return([]string{"u1"}, nil).Once()
	// Non-retryable so the mock's .Once() isn't consumed by activity retries
	// (batchActivityOptions allows 3 attempts).
	env.OnActivity(da.DiscoverUsersBatch, mock.Anything, mock.Anything).
		Return(activities.SyncBatchResult{}, temporal.NewNonRetryableApplicationError("boom", "test", nil)).Once()

	env.ExecuteWorkflow(workflows.DiscoveryBatchDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	// Failed batches are logged; the users' claims expire and re-queue.
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

// ---- DraftSyncDispatcher ----

func TestDraftSyncDispatcher_DrainsUntilShortClaim(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	cfg := activities.DraftSyncConfig{ParallelBatches: 2, BatchSize: 2, Concurrency: 4}
	env.OnActivity(dfa.GetDraftSyncConfig, mock.Anything).Return(cfg, nil)

	full := []string{"a", "b"}
	short := []string{"c"}
	// First claim full, second claim short -> dispatcher must stop claiming after the short one.
	env.OnActivity(dfa.ClaimLeaguesForDrafts, mock.Anything, activities.ClaimLeaguesForDraftsParams{BatchSize: 2}).
		Return(full, nil).Once()
	env.OnActivity(dfa.ClaimLeaguesForDrafts, mock.Anything, activities.ClaimLeaguesForDraftsParams{BatchSize: 2}).
		Return(short, nil).Once()

	env.OnActivity(dfa.SyncLeagueDraftsBatch, mock.Anything, activities.SyncLeagueDraftsBatchParams{LeagueIDs: full, Concurrency: 4}).
		Return(activities.SyncBatchResult{Processed: 2}, nil).Once()
	env.OnActivity(dfa.SyncLeagueDraftsBatch, mock.Anything, activities.SyncLeagueDraftsBatchParams{LeagueIDs: short, Concurrency: 4}).
		Return(activities.SyncBatchResult{Processed: 1}, nil).Once()

	env.ExecuteWorkflow(workflows.DraftSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestDraftSyncDispatcher_EmptyClaimStopsImmediately(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.GetDraftSyncConfig, mock.Anything).
		Return(activities.DraftSyncConfig{ParallelBatches: 4, BatchSize: 250, Concurrency: 12}, nil)
	env.OnActivity(dfa.ClaimLeaguesForDrafts, mock.Anything, activities.ClaimLeaguesForDraftsParams{BatchSize: 250}).
		Return([]string{}, nil).Once()

	env.ExecuteWorkflow(workflows.DraftSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestDraftSyncDispatcher_BatchFailureDoesNotFailRun(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.GetDraftSyncConfig, mock.Anything).
		Return(activities.DraftSyncConfig{ParallelBatches: 1, BatchSize: 2, Concurrency: 4}, nil)
	env.OnActivity(dfa.ClaimLeaguesForDrafts, mock.Anything, mock.Anything).Return([]string{"a"}, nil).Once()
	// Non-retryable so the mock's .Once() isn't consumed by activity retries
	// (batchActivityOptions allows 3 attempts).
	env.OnActivity(dfa.SyncLeagueDraftsBatch, mock.Anything, mock.Anything).
		Return(activities.SyncBatchResult{}, temporal.NewNonRetryableApplicationError("boom", "test", nil)).Once()

	env.ExecuteWorkflow(workflows.DraftSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	// Failed batches are logged; the leagues' claims expire and re-queue.
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

// ---- TransactionSyncDispatcher ----

func TestTransactionSyncDispatcher_DrainsUntilShortClaim(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	cfg := activities.TransactionSyncConfig{ParallelBatches: 2, BatchSize: 2, Concurrency: 4}
	env.OnActivity(dfa.GetTransactionSyncConfig, mock.Anything).Return(cfg, nil)

	full := []activities.LeagueTransactionState{{LeagueID: "a", Season: "2026"}, {LeagueID: "b", Season: "2026"}}
	short := []activities.LeagueTransactionState{{LeagueID: "c", Season: "2026"}}
	// First claim full, second claim short -> dispatcher must stop claiming after the short one.
	env.OnActivity(dfa.ClaimLeaguesForTransactions, mock.Anything, activities.ClaimLeaguesForTransactionsParams{BatchSize: 2}).
		Return(full, nil).Once()
	env.OnActivity(dfa.ClaimLeaguesForTransactions, mock.Anything, activities.ClaimLeaguesForTransactionsParams{BatchSize: 2}).
		Return(short, nil).Once()

	env.OnActivity(dfa.SyncLeagueTransactionsBatch, mock.Anything, activities.SyncLeagueTransactionsBatchParams{Leagues: full, Concurrency: 4}).
		Return(activities.SyncBatchResult{Processed: 2}, nil).Once()
	env.OnActivity(dfa.SyncLeagueTransactionsBatch, mock.Anything, activities.SyncLeagueTransactionsBatchParams{Leagues: short, Concurrency: 4}).
		Return(activities.SyncBatchResult{Processed: 1}, nil).Once()

	env.ExecuteWorkflow(workflows.TransactionSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestTransactionSyncDispatcher_EmptyClaimStopsImmediately(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.GetTransactionSyncConfig, mock.Anything).
		Return(activities.TransactionSyncConfig{ParallelBatches: 4, BatchSize: 250, Concurrency: 12}, nil)
	env.OnActivity(dfa.ClaimLeaguesForTransactions, mock.Anything, activities.ClaimLeaguesForTransactionsParams{BatchSize: 250}).
		Return([]activities.LeagueTransactionState{}, nil).Once()

	env.ExecuteWorkflow(workflows.TransactionSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestTransactionSyncDispatcher_BatchFailureDoesNotFailRun(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	dfa := &activities.DataFetchActivities{}
	env.OnActivity(dfa.GetTransactionSyncConfig, mock.Anything).
		Return(activities.TransactionSyncConfig{ParallelBatches: 1, BatchSize: 2, Concurrency: 4}, nil)
	short := []activities.LeagueTransactionState{{LeagueID: "a", Season: "2026"}}
	env.OnActivity(dfa.ClaimLeaguesForTransactions, mock.Anything, mock.Anything).Return(short, nil).Once()
	// Non-retryable so the mock's .Once() isn't consumed by activity retries
	// (batchActivityOptions allows 3 attempts).
	env.OnActivity(dfa.SyncLeagueTransactionsBatch, mock.Anything, mock.Anything).
		Return(activities.SyncBatchResult{}, temporal.NewNonRetryableApplicationError("boom", "test", nil)).Once()

	env.ExecuteWorkflow(workflows.TransactionSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	// Failed batches are logged; the leagues' claims expire and re-queue.
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

// ---- ScavengerDispatcher ----

func TestScavengerDispatcher_DrainsAllStreamsUntilShortBatch(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	cfg := activities.ScavengerConfig{LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)

	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, activities.ReplicateBatchParams{BatchSize: 500}).
		Return(activities.ReplicateBatchResult{Replicated: 3, Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateTransactionsBatch, mock.Anything, activities.ReplicateBatchParams{BatchSize: 5000}).
		Return(activities.ReplicateBatchResult{Replicated: 10, Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftHeadersBatch, mock.Anything, activities.ReplicateBatchParams{BatchSize: 200}).
		Return(activities.ReplicateBatchResult{Replicated: 2, Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftPicksBatch, mock.Anything, activities.ReplicateBatchParams{BatchSize: 200}).
		Return(activities.ReplicateBatchResult{Replicated: 1, Drained: true}, nil).Once()

	env.ExecuteWorkflow(workflows.ScavengerDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report activities.ScavengerReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, activities.ScavengerReport{
		LeaguesReplicated: 3, TransactionsReplicated: 10, DraftHeadersReplicated: 2, DraftPicksReplicated: 1,
	}, report)
	env.AssertExpectations(t)
}

func TestScavengerDispatcher_StreamFailureDoesNotBlockOtherStreams(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	cfg := activities.ScavengerConfig{LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)

	// Leagues fails outright; the other three streams must still run.
	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, activities.ReplicateBatchParams{BatchSize: 500}).
		Return(activities.ReplicateBatchResult{}, temporal.NewNonRetryableApplicationError("boom", "test", nil)).Once()
	env.OnActivity(sa.ReplicateTransactionsBatch, mock.Anything, activities.ReplicateBatchParams{BatchSize: 5000}).
		Return(activities.ReplicateBatchResult{Replicated: 5, Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftHeadersBatch, mock.Anything, activities.ReplicateBatchParams{BatchSize: 200}).
		Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftPicksBatch, mock.Anything, activities.ReplicateBatchParams{BatchSize: 200}).
		Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()

	env.ExecuteWorkflow(workflows.ScavengerDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError()) // stream failures are logged and swallowed
	var report activities.ScavengerReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, 0, report.LeaguesReplicated)
	require.Equal(t, 5, report.TransactionsReplicated)
	env.AssertExpectations(t)
}

// ---- ArchiveBackfillWorkflow ----

func TestArchiveBackfillWorkflow_CompletesWhenAllStreamsDrainWithinOneExecution(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	cfg := activities.ScavengerConfig{LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)
	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{Replicated: 3, Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateTransactionsBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{Replicated: 10, Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftHeadersBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{Replicated: 2, Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftPicksBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{Replicated: 1, Drained: true}, nil).Once()

	env.ExecuteWorkflow(workflows.ArchiveBackfillWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestArchiveBackfillWorkflow_ContinuesAsNewWhenAStreamHitsTheBatchCap(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	cfg := activities.ScavengerConfig{LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)
	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	// Transactions never reports Drained within this execution — the "huge
	// backlog" case that must trigger ContinueAsNew.
	env.OnActivity(sa.ReplicateTransactionsBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{Replicated: 1, Drained: false}, nil)
	env.OnActivity(sa.ReplicateDraftHeadersBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftPicksBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()

	env.ExecuteWorkflow(workflows.ArchiveBackfillWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.True(t, workflow.IsContinueAsNewError(env.GetWorkflowError()))
	env.AssertExpectations(t)
}

func TestArchiveBackfillWorkflow_StreamFailureFailsTheExecution(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	cfg := activities.ScavengerConfig{LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)
	// Non-retryable so the mock isn't consumed by activity retries
	// (defaultActivityOptions allows 3 attempts).
	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, mock.Anything).
		Return(activities.ReplicateBatchResult{}, temporal.NewNonRetryableApplicationError("boom", "test", nil))

	env.ExecuteWorkflow(workflows.ArchiveBackfillWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	require.False(t, workflow.IsContinueAsNewError(env.GetWorkflowError()))
}

// ---- ScavengerDispatcher purge phase ----

func TestScavengerDispatcher_PurgeDisabledByDefault_NeverCallsPurgeActivities(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	cfg := activities.ScavengerConfig{
		LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50,
		RetentionDays: 30, PurgeEnabled: false,
	}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)
	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateTransactionsBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftHeadersBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftPicksBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	// No PurgeTransactionsBatch / PurgeDraftsBatch mocks registered: if the
	// dispatcher calls them anyway, the test environment fails on the
	// unmocked activity call.

	env.ExecuteWorkflow(workflows.ScavengerDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestScavengerDispatcher_PurgeEnabledAndCaughtUp_RunsPurgeAndAccumulatesReport(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	cfg := activities.ScavengerConfig{
		LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50,
		RetentionDays: 30, PurgeEnabled: true,
	}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)
	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateTransactionsBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftHeadersBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftPicksBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.PurgeTransactionsBatch, mock.Anything, activities.PurgeBatchParams{BatchSize: 5000, RetentionDays: 30}).
		Return(activities.PurgeBatchResult{Purged: 100, Unverified: 2, Drained: true}, nil).Once()
	env.OnActivity(sa.PurgeDraftsBatch, mock.Anything, activities.PurgeBatchParams{BatchSize: 200, RetentionDays: 30}).
		Return(activities.PurgeBatchResult{Purged: 4, Unverified: 1, Drained: true}, nil).Once()

	env.ExecuteWorkflow(workflows.ScavengerDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report activities.ScavengerReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, 100, report.TransactionsPurged)
	require.Equal(t, 2, report.TransactionsUnverified)
	require.Equal(t, 4, report.DraftsPurged)
	require.Equal(t, 1, report.DraftsUnverified)
	env.AssertExpectations(t)
}

func TestScavengerDispatcher_PurgeSkippedWhenReplicateNotCaughtUp(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	// MaxBatchesPerRun: 1 with Drained: false means every stream hits the
	// iteration cap without catching up this run.
	cfg := activities.ScavengerConfig{
		LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 1,
		RetentionDays: 30, PurgeEnabled: true,
	}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)
	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Replicated: 500, Drained: false}, nil).Once()
	env.OnActivity(sa.ReplicateTransactionsBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Replicated: 5000, Drained: false}, nil).Once()
	env.OnActivity(sa.ReplicateDraftHeadersBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Replicated: 200, Drained: false}, nil).Once()
	env.OnActivity(sa.ReplicateDraftPicksBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Replicated: 200, Drained: false}, nil).Once()
	// No purge mocks: neither stream drained, so purge must not run even
	// though PurgeEnabled is true.

	env.ExecuteWorkflow(workflows.ScavengerDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestScavengerDispatcher_PurgeActivityErrorFailsTheWorkflowRun(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	sa := &activities.ScavengerActivities{}
	cfg := activities.ScavengerConfig{
		LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50,
		RetentionDays: 30, PurgeEnabled: true,
	}
	env.OnActivity(sa.GetScavengerConfig, mock.Anything).Return(cfg, nil)
	env.OnActivity(sa.ReplicateLeaguesBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateTransactionsBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftHeadersBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.ReplicateDraftPicksBatch, mock.Anything, mock.Anything).Return(activities.ReplicateBatchResult{Drained: true}, nil).Once()
	env.OnActivity(sa.PurgeTransactionsBatch, mock.Anything, mock.Anything).
		Return(activities.PurgeBatchResult{}, temporal.NewNonRetryableApplicationError("replication stalled", "test", nil)).Once()

	env.ExecuteWorkflow(workflows.ScavengerDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError()) // unlike replicate stream failures, purge errors must NOT be swallowed
	env.AssertExpectations(t)
}
