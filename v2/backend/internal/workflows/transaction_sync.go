package workflows

import (
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// TransactionSyncDispatcher is a scheduled workflow that queries for leagues with stale
// transaction data and spawns LeagueTransactionSyncWorkflow children for each (fire-and-forget).
func TransactionSyncDispatcher(ctx workflow.Context) error {
	dfa := &activities.DataFetchActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var leagueIDs []string
	if err := workflow.ExecuteActivity(actCtx, dfa.GetStaleLeaguesForTransactions, activities.GetStaleLeaguesParams{BatchSize: BatchSize}).Get(ctx, &leagueIDs); err != nil {
		return err
	}
	for _, lid := range leagueIDs {
		cwo := workflow.ChildWorkflowOptions{
			TaskQueue:         TaskQueueTransactions,
			ParentClosePolicy: enumspb.PARENT_CLOSE_POLICY_ABANDON,
		}
		f := workflow.ExecuteChildWorkflow(workflow.WithChildOptions(ctx, cwo), LeagueTransactionSyncWorkflow, LeagueSyncParams{LeagueID: lid})
		if err := f.GetChildWorkflowExecution().Get(ctx, nil); err != nil {
			workflow.GetLogger(ctx).Warn("failed to start LeagueTransactionSyncWorkflow", "leagueID", lid, "error", err)
		}
	}
	return nil
}

// LeagueTransactionSyncWorkflow fetches all transaction rounds (1–18) for a single league,
// then stamps last_transactions_fetched_at.
func LeagueTransactionSyncWorkflow(ctx workflow.Context, params LeagueSyncParams) error {
	dfa := &activities.DataFetchActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	if err := workflow.ExecuteActivity(actCtx, dfa.FetchLeagueTransactions, activities.FetchLeagueTransactionsParams{LeagueID: params.LeagueID}).Get(ctx, nil); err != nil {
		return err
	}

	return workflow.ExecuteActivity(actCtx, dfa.MarkLeagueTransactionsFetched, activities.MarkLeagueFetchedParams{LeagueID: params.LeagueID}).Get(ctx, nil)
}
