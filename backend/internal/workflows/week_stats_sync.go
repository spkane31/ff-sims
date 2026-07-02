package workflows

import (
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// lastFantasyWeek is the last fantasy-relevant regular season week.
const lastFantasyWeek = 18

// SyncWeekStats fetches weekly Sleeper stats for every week 1-18 of season that
// isn't already finalized. Directly invocable (e.g. via `temporal workflow start
// --type SyncWeekStats --input '"2025"'`) for backfills, and delegated to by
// WeekStatsSyncDispatcher for the in-season schedule.
func SyncWeekStats(ctx workflow.Context, season string) error {
	wsa := &activities.WeekStatsActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var finalizedWeeks []int
	if err := workflow.ExecuteActivity(actCtx, wsa.GetFinalizedWeeks, activities.GetFinalizedWeeksParams{Season: season}).Get(ctx, &finalizedWeeks); err != nil {
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
		if err := workflow.ExecuteActivity(actCtx, wsa.FetchWeekStats, activities.FetchWeekStatsParams{Season: season, Week: week}).Get(ctx, nil); err != nil {
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
	return SyncWeekStats(ctx, season)
}
