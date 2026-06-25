package workflows

import (
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// DiscoveryBatchDispatcher is the scheduled parent workflow for user/league discovery.
// It queries for stale users and spawns UserDiscoveryWorkflow children (fire-and-forget).
// Draft and transaction sync are handled by DraftSyncDispatcher and TransactionSyncDispatcher.
func DiscoveryBatchDispatcher(ctx workflow.Context) error {
	da := &activities.DiscoveryActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var userIDs []string
	if err := workflow.ExecuteActivity(actCtx, da.GetStaleUsers, activities.GetStaleUsersParams{BatchSize: BatchSize}).Get(ctx, &userIDs); err != nil {
		return err
	}
	for _, uid := range userIDs {
		cwo := workflow.ChildWorkflowOptions{
			TaskQueue:         TaskQueueDiscovery,
			ParentClosePolicy: enumspb.PARENT_CLOSE_POLICY_ABANDON,
		}
		f := workflow.ExecuteChildWorkflow(workflow.WithChildOptions(ctx, cwo), UserDiscoveryWorkflow, UserDiscoveryParams{UserID: uid})
		if err := f.GetChildWorkflowExecution().Get(ctx, nil); err != nil {
			workflow.GetLogger(ctx).Warn("failed to start UserDiscoveryWorkflow", "userID", uid, "error", err)
		}
	}
	return nil
}
