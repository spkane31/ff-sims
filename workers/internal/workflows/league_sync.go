package workflows

import (
	"go.temporal.io/sdk/workflow"

	"workers/internal/activities"
)

// LeagueSyncWorkflow handles a full data sync for a single Sleeper league:
// fetches scoring details, drafts + picks, all transaction rounds, then marks the league fetched.
// Returns non-retryable NOT_FOUND if the league has been deleted from Sleeper.
func LeagueSyncWorkflow(ctx workflow.Context, params LeagueSyncParams) error {
	dfa := &activities.DataFetchActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	if err := workflow.ExecuteActivity(actCtx, dfa.FetchLeagueDetails, activities.FetchLeagueDetailsParams{LeagueID: params.LeagueID}).Get(ctx, nil); err != nil {
		if isNotFound(err) {
			return workflow.ExecuteActivity(actCtx, dfa.MarkLeagueSkipped, activities.MarkLeagueSkippedParams{LeagueID: params.LeagueID}).Get(ctx, nil)
		}
		return err
	}

	var completedDraftIDs []string
	if err := workflow.ExecuteActivity(actCtx, dfa.FetchLeagueDrafts, activities.FetchLeagueDraftsParams{LeagueID: params.LeagueID}).Get(ctx, &completedDraftIDs); err != nil {
		return err
	}

	for _, draftID := range completedDraftIDs {
		if err := workflow.ExecuteActivity(actCtx, dfa.FetchDraftPicks, activities.FetchDraftPicksParams{DraftID: draftID}).Get(ctx, nil); err != nil {
			// Don't fail the whole league sync if one draft's picks can't be fetched
			workflow.GetLogger(ctx).Warn("FetchDraftPicks failed, continuing",
				"draftID", draftID, "error", err)
		}
	}

	if err := workflow.ExecuteActivity(actCtx, dfa.FetchLeagueTransactions, activities.FetchLeagueTransactionsParams{LeagueID: params.LeagueID}).Get(ctx, nil); err != nil {
		return err
	}

	return workflow.ExecuteActivity(actCtx, dfa.MarkLeagueFetched, activities.MarkLeagueFetchedParams{LeagueID: params.LeagueID}).Get(ctx, nil)
}
