package workflows

import (
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// lastFantasyWeek is the last fantasy-relevant regular season week.
const lastFantasyWeek = 18

// SyncWeekStats fetches weekly Sleeper stats for every week 1-18 of params.Season
// that isn't already finalized. Directly invocable (e.g. via `temporal workflow
// start --type SyncWeekStats --input '{"Season":"2025"}'`) for backfills, and
// delegated to by WeekStatsSyncDispatcher for the in-season schedule. Takes a
// params struct (rather than a bare string) so future fields — e.g. a week
// override or a force-refetch flag — don't require a breaking signature change.
func SyncWeekStats(ctx workflow.Context, params SyncWeekStatsParams) error {
	wsa := &activities.WeekStatsActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var finalizedWeeks []int
	if err := workflow.ExecuteActivity(actCtx, wsa.GetFinalizedWeeks, activities.GetFinalizedWeeksParams{Season: params.Season}).Get(ctx, &finalizedWeeks); err != nil {
		return err
	}
	finalized := make(map[int]bool, len(finalizedWeeks))
	for _, w := range finalizedWeeks {
		finalized[w] = true
	}

	for week := 1; week <= lastFantasyWeek; week++ {
		if finalized[week] {
			continue
		}
		if err := workflow.ExecuteActivity(actCtx, wsa.FetchWeekStats, activities.FetchWeekStatsParams{Season: params.Season, Week: week}).Get(ctx, nil); err != nil {
			return err
		}
	}
	return nil
}

// WeekStatsSyncDispatcher is the scheduled entry point: it resolves the current NFL
// season via Sleeper's state endpoint, then runs SyncWeekStats for it.
func WeekStatsSyncDispatcher(ctx workflow.Context) error {
	wsa := &activities.WeekStatsActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var season string
	if err := workflow.ExecuteActivity(actCtx, wsa.GetCurrentSeason).Get(ctx, &season); err != nil {
		return err
	}
	return SyncWeekStats(ctx, SyncWeekStatsParams{Season: season})
}
