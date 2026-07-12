package workflows

import (
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// DraftSyncDispatcher drains the stale-drafts backlog by fanning out
// claim→batch pipelines, mirroring TransactionSyncDispatcher. Each iteration
// claims up to ParallelBatches batches of leagues (atomically, via FOR UPDATE
// SKIP LOCKED on drafts_claimed_at) and runs a SyncLeagueDraftsBatch activity
// per claim in parallel. A short or empty claim means the backlog is drained
// for now, so the run exits and the schedule takes over. Failed batch
// activities are logged, not propagated: their leagues' claims expire after
// 20 minutes and re-queue.
func DraftSyncDispatcher(ctx workflow.Context) (DraftSyncReport, error) {
	dfa := &activities.DataFetchActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	batchCtx := workflow.WithActivityOptions(ctx, batchActivityOptions)
	logger := workflow.GetLogger(ctx)

	var cfg activities.DraftSyncConfig
	if err := workflow.ExecuteActivity(actCtx, dfa.GetDraftSyncConfig).Get(ctx, &cfg); err != nil {
		return DraftSyncReport{}, err
	}

	var report DraftSyncReport
	for iter := 0; iter < MaxDispatchIterations; iter++ {
		var futures []workflow.Future
		drained := false
		for k := 0; k < cfg.ParallelBatches; k++ {
			var leagueIDs []string
			err := workflow.ExecuteActivity(actCtx, dfa.ClaimLeaguesForDrafts, activities.ClaimLeaguesForDraftsParams{
				BatchSize: cfg.BatchSize,
			}).Get(ctx, &leagueIDs)
			if err != nil {
				logger.Error("draft claim failed; stopping dispatch for this run", "error", err)
				drained = true
				break
			}
			if len(leagueIDs) == 0 {
				drained = true
				break
			}
			futures = append(futures, workflow.ExecuteActivity(batchCtx, dfa.SyncLeagueDraftsBatch, activities.SyncLeagueDraftsBatchParams{
				LeagueIDs:   leagueIDs,
				Concurrency: cfg.Concurrency,
			}))
			if len(leagueIDs) < cfg.BatchSize {
				drained = true
				break
			}
		}
		for _, f := range futures {
			var res activities.SyncBatchResult
			if err := f.Get(ctx, &res); err != nil {
				logger.Error("draft batch failed; claims will expire and re-queue", "error", err)
				continue
			}
			logger.Info("draft batch done", "processed", res.Processed, "failed", res.Failed)
			report.LeaguesProcessed += res.Processed
			report.LeaguesFailed += res.Failed
		}
		if drained {
			break
		}
	}
	return report, nil
}
