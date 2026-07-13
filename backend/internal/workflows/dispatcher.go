package workflows

import (
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// DiscoveryBatchDispatcher drains the stale-users queue by fanning out
// claim→batch pipelines, mirroring the transaction/draft sync dispatchers.
// Each iteration claims up to ParallelBatches batches of users (atomically,
// via FOR UPDATE SKIP LOCKED on sleeper_users.claimed_at) and runs a
// DiscoverUsersBatch activity per claim in parallel. A short or empty claim
// means the queue is drained for now, so the run exits and the schedule takes
// over. Failed batch activities are logged, not propagated: their users'
// claims expire after 20 minutes and re-queue. Claiming (instead of the old
// re-select + child-workflow-ID dedupe) means a stuck cohort can never
// head-of-line-block discovery of the users behind it.
func DiscoveryBatchDispatcher(ctx workflow.Context) (DiscoveryReport, error) {
	da := &activities.DiscoveryActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	batchCtx := workflow.WithActivityOptions(ctx, batchActivityOptions)
	logger := workflow.GetLogger(ctx)

	var cfg activities.DiscoveryConfig
	if err := workflow.ExecuteActivity(actCtx, da.GetDiscoveryConfig).Get(ctx, &cfg); err != nil {
		return DiscoveryReport{}, err
	}

	var report DiscoveryReport
	for iter := 0; iter < MaxDispatchIterations; iter++ {
		var futures []workflow.Future
		drained := false
		for k := 0; k < cfg.ParallelBatches; k++ {
			var userIDs []string
			err := workflow.ExecuteActivity(actCtx, da.ClaimStaleUsers, activities.ClaimStaleUsersParams{
				BatchSize: cfg.BatchSize,
			}).Get(ctx, &userIDs)
			if err != nil {
				logger.Error("user claim failed; stopping dispatch for this run", "error", err)
				drained = true
				break
			}
			if len(userIDs) == 0 {
				drained = true
				break
			}
			futures = append(futures, workflow.ExecuteActivity(batchCtx, da.DiscoverUsersBatch, activities.DiscoverUsersBatchParams{
				UserIDs:     userIDs,
				Concurrency: cfg.Concurrency,
			}))
			if len(userIDs) < cfg.BatchSize {
				drained = true
				break
			}
		}
		for _, f := range futures {
			var res activities.SyncBatchResult
			if err := f.Get(ctx, &res); err != nil {
				logger.Error("discovery batch failed; claims will expire and re-queue", "error", err)
				continue
			}
			logger.Info("discovery batch done", "processed", res.Processed, "failed", res.Failed)
			report.UsersProcessed += res.Processed
			report.UsersFailed += res.Failed
		}
		if drained {
			break
		}
	}
	return report, nil
}
