package workflows

import (
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// DraftSyncDispatcher is a scheduled workflow that queries for leagues with stale draft data
// and spawns LeagueDraftSyncWorkflow children for each (fire-and-forget).
func DraftSyncDispatcher(ctx workflow.Context) error {
	dfa := &activities.DataFetchActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var leagueIDs []string
	if err := workflow.ExecuteActivity(actCtx, dfa.GetStaleLeaguesForDrafts, activities.GetStaleLeaguesParams{BatchSize: SyncBatchSize}).Get(ctx, &leagueIDs); err != nil {
		return err
	}
	for _, lid := range leagueIDs {
		cwo := workflow.ChildWorkflowOptions{
			TaskQueue:         TaskQueueDrafts,
			ParentClosePolicy: enumspb.PARENT_CLOSE_POLICY_ABANDON,
		}
		f := workflow.ExecuteChildWorkflow(workflow.WithChildOptions(ctx, cwo), LeagueDraftSyncWorkflow, LeagueSyncParams{LeagueID: lid})
		if err := f.GetChildWorkflowExecution().Get(ctx, nil); err != nil {
			workflow.GetLogger(ctx).Warn("failed to start LeagueDraftSyncWorkflow", "leagueID", lid, "error", err)
		}
	}
	return nil
}

// LeagueDraftSyncWorkflow fetches all completed drafts and their picks for a single league,
// then stamps last_drafts_fetched_at. Pick failures are logged and skipped (warn+continue).
func LeagueDraftSyncWorkflow(ctx workflow.Context, params LeagueSyncParams) error {
	dfa := &activities.DataFetchActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var completedDraftIDs []string
	if err := workflow.ExecuteActivity(actCtx, dfa.FetchLeagueDrafts, activities.FetchLeagueDraftsParams{LeagueID: params.LeagueID}).Get(ctx, &completedDraftIDs); err != nil {
		if isNotFound(err) {
			return workflow.ExecuteActivity(actCtx, dfa.MarkLeagueSkipped, activities.MarkLeagueSkippedParams{LeagueID: params.LeagueID}).Get(ctx, nil)
		}
		return err
	}

	for _, draftID := range completedDraftIDs {
		if err := workflow.ExecuteActivity(actCtx, dfa.FetchDraftPicks, activities.FetchDraftPicksParams{DraftID: draftID}).Get(ctx, nil); err != nil {
			workflow.GetLogger(ctx).Warn("FetchDraftPicks failed, continuing",
				"draftID", draftID, "error", err)
		}
	}

	return workflow.ExecuteActivity(actCtx, dfa.MarkLeagueDraftsFetched, activities.MarkLeagueFetchedParams{LeagueID: params.LeagueID}).Get(ctx, nil)
}
