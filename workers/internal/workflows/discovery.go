package workflows

import (
	"go.temporal.io/sdk/workflow"

	"workers/internal/activities"
)

// UserDiscoveryWorkflow handles discovery for a single Sleeper user: fetches all leagues
// across configured seasons, upserts member users (with NULL last_fetched_at so they are
// picked up by future dispatcher runs), then marks the user as fetched.
// Returns a non-retryable NOT_FOUND if the user has been deleted from Sleeper.
func UserDiscoveryWorkflow(ctx workflow.Context, params UserDiscoveryParams) error {
	da := &activities.DiscoveryActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var leagueIDs []string
	err := workflow.ExecuteActivity(actCtx, da.FetchUserLeagues, activities.FetchUserLeaguesParams{UserID: params.UserID}).Get(ctx, &leagueIDs)
	if err != nil {
		if isNotFound(err) {
			return workflow.ExecuteActivity(actCtx, da.MarkUserSkipped, activities.MarkUserSkippedParams{UserID: params.UserID}).Get(ctx, nil)
		}
		return err
	}

	for _, lid := range leagueIDs {
		if err := workflow.ExecuteActivity(actCtx, da.FetchLeagueMembers, activities.FetchLeagueMembersParams{LeagueID: lid}).Get(ctx, nil); err != nil {
			// Don't fail the whole workflow if one league's members can't be fetched
			workflow.GetLogger(ctx).Warn("FetchLeagueMembers failed, continuing",
				"leagueID", lid, "error", err)
		}
	}

	return workflow.ExecuteActivity(actCtx, da.MarkUserFetched, activities.MarkUserFetchedParams{UserID: params.UserID}).Get(ctx, nil)
}
