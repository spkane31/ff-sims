package workflows

import (
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// TransactionSyncDispatcher drains the stale-transactions backlog by fanning
// out claim→batch pipelines. Each iteration claims up to ParallelBatches
// batches of leagues (atomically, via FOR UPDATE SKIP LOCKED in Postgres) and
// runs a SyncLeagueTransactionsBatch activity per claim in parallel. A short
// or empty claim means the backlog is drained for now, so the run exits and
// the 5-minute schedule takes over. Failed batch activities are logged, not
// propagated: their leagues' claims expire after 20 minutes and re-queue.
func TransactionSyncDispatcher(ctx workflow.Context) error {
	dfa := &activities.DataFetchActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	batchCtx := workflow.WithActivityOptions(ctx, batchActivityOptions)
	logger := workflow.GetLogger(ctx)

	var cfg activities.TransactionSyncConfig
	if err := workflow.ExecuteActivity(actCtx, dfa.GetTransactionSyncConfig).Get(ctx, &cfg); err != nil {
		return err
	}

	for iter := 0; iter < TxnMaxDispatchIterations; iter++ {
		var futures []workflow.Future
		drained := false
		for k := 0; k < cfg.ParallelBatches; k++ {
			var leagues []activities.LeagueTransactionState
			err := workflow.ExecuteActivity(actCtx, dfa.ClaimLeaguesForTransactions, activities.ClaimLeaguesForTransactionsParams{
				BatchSize: cfg.BatchSize,
			}).Get(ctx, &leagues)
			if err != nil {
				logger.Error("claim failed; stopping dispatch for this run", "error", err)
				drained = true
				break
			}
			if len(leagues) == 0 {
				drained = true
				break
			}
			futures = append(futures, workflow.ExecuteActivity(batchCtx, dfa.SyncLeagueTransactionsBatch, activities.SyncLeagueTransactionsBatchParams{
				Leagues:     leagues,
				Concurrency: cfg.Concurrency,
			}))
			if len(leagues) < cfg.BatchSize {
				drained = true
				break
			}
		}
		for _, f := range futures {
			var res activities.SyncBatchResult
			if err := f.Get(ctx, &res); err != nil {
				logger.Error("transaction batch failed; claims will expire and re-queue", "error", err)
				continue
			}
			logger.Info("transaction batch done", "processed", res.Processed, "failed", res.Failed)
		}
		if drained {
			break
		}
	}
	return nil
}
