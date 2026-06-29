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

	var states []activities.LeagueTransactionState
	if err := workflow.ExecuteActivity(actCtx, dfa.GetStaleLeaguesForTransactions, activities.GetStaleLeaguesParams{BatchSize: SyncBatchSize}).Get(ctx, &states); err != nil {
		return err
	}
	for _, s := range states {
		cwo := workflow.ChildWorkflowOptions{
			TaskQueue:         TaskQueueTransactions,
			ParentClosePolicy: enumspb.PARENT_CLOSE_POLICY_ABANDON,
		}
		f := workflow.ExecuteChildWorkflow(workflow.WithChildOptions(ctx, cwo), LeagueTransactionSyncWorkflow, LeagueSyncParams{
			LeagueID:       s.LeagueID,
			LastLegFetched: s.LastLegFetched,
		})
		if err := f.GetChildWorkflowExecution().Get(ctx, nil); err != nil {
			workflow.GetLogger(ctx).Warn("failed to start LeagueTransactionSyncWorkflow", "leagueID", s.LeagueID, "error", err)
		}
	}
	return nil
}

// LeagueTransactionSyncWorkflow fetches transactions for a single league starting from the
// leg cursor, then stamps last_transactions_fetched_at and advances the leg cursor.
func LeagueTransactionSyncWorkflow(ctx workflow.Context, params LeagueSyncParams) error {
	dfa := &activities.DataFetchActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var maxLeg int
	if err := workflow.ExecuteActivity(actCtx, dfa.FetchLeagueTransactions, activities.FetchLeagueTransactionsParams{
		LeagueID:       params.LeagueID,
		LastLegFetched: params.LastLegFetched,
	}).Get(ctx, &maxLeg); err != nil {
		return err
	}

	return workflow.ExecuteActivity(actCtx, dfa.MarkLeagueTransactionsFetched, activities.MarkLeagueTransactionsFetchedParams{
		LeagueID: params.LeagueID,
		MaxLeg:   maxLeg,
	}).Get(ctx, nil)
}
