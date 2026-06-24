package workflows

import (
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/workflow"

	"workers/internal/activities"
)

// DiscoveryBatchDispatcher is the scheduled parent workflow. It queries Postgres for the
// least-recently-fetched users and leagues, then spawns independent child workflows for
// each with ABANDON close policy (fire-and-forget).
func DiscoveryBatchDispatcher(ctx workflow.Context) error {
	da := &activities.DiscoveryActivities{}
	dfa := &activities.DataFetchActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var userIDs []string
	if err := workflow.ExecuteActivity(actCtx, da.GetStaleUsers, BatchSize).Get(ctx, &userIDs); err != nil {
		return err
	}
	for _, uid := range userIDs {
		cwo := workflow.ChildWorkflowOptions{
			TaskQueue:         TaskQueueDiscovery,
			ParentClosePolicy: enumspb.PARENT_CLOSE_POLICY_ABANDON,
		}
		workflow.ExecuteChildWorkflow(workflow.WithChildOptions(ctx, cwo), UserDiscoveryWorkflow, uid)
	}

	var leagueIDs []string
	if err := workflow.ExecuteActivity(actCtx, dfa.GetStaleLeagues, BatchSize).Get(ctx, &leagueIDs); err != nil {
		return err
	}
	for _, lid := range leagueIDs {
		cwo := workflow.ChildWorkflowOptions{
			TaskQueue:         TaskQueueData,
			ParentClosePolicy: enumspb.PARENT_CLOSE_POLICY_ABANDON,
		}
		workflow.ExecuteChildWorkflow(workflow.WithChildOptions(ctx, cwo), LeagueSyncWorkflow, lid)
	}

	return nil
}
