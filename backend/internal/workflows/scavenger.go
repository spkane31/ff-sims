package workflows

import (
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// ScavengerDispatcher replicates cloud → archive across four streams, in
// order: leagues, transactions, draft headers, draft picks. Each stream
// drains independently up to MaxBatchesPerRun batches or until a short
// batch signals it's caught up; a stream's activity failure is logged and
// stops only that stream for this run — the cursor didn't move (advance
// commits atomically with the copied rows), so the next 6h run resumes from
// the same position. All five activity calls use defaultActivityOptions
// (not batchActivityOptions): unlike the per-league sync batch activities,
// these are fast single-query DB-to-DB copies with no external API calls
// and no activity.RecordHeartbeat — batchActivityOptions' HeartbeatTimeout
// is for activities that actually heartbeat. Runs on the archive-maintenance
// queue, which only exists when ARCHIVE_DATABASE_URL is set — see
// cmd/worker/main.go.
func ScavengerDispatcher(ctx workflow.Context) (activities.ScavengerReport, error) {
	sa := &activities.ScavengerActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	logger := workflow.GetLogger(ctx)

	var cfg activities.ScavengerConfig
	if err := workflow.ExecuteActivity(actCtx, sa.GetScavengerConfig).Get(ctx, &cfg); err != nil {
		return activities.ScavengerReport{}, err
	}

	var report activities.ScavengerReport

	for i := 0; i < cfg.MaxBatchesPerRun; i++ {
		var res activities.ReplicateBatchResult
		if err := workflow.ExecuteActivity(actCtx, sa.ReplicateLeaguesBatch, activities.ReplicateBatchParams{BatchSize: cfg.LeagueBatchSize}).Get(ctx, &res); err != nil {
			logger.Error("replicate leagues batch failed; stopping leagues for this run", "error", err)
			break
		}
		report.LeaguesReplicated += res.Replicated
		if res.Drained {
			break
		}
	}

	for i := 0; i < cfg.MaxBatchesPerRun; i++ {
		var res activities.ReplicateBatchResult
		if err := workflow.ExecuteActivity(actCtx, sa.ReplicateTransactionsBatch, activities.ReplicateBatchParams{BatchSize: cfg.TxnBatchSize}).Get(ctx, &res); err != nil {
			logger.Error("replicate transactions batch failed; stopping transactions for this run", "error", err)
			break
		}
		report.TransactionsReplicated += res.Replicated
		if res.Drained {
			break
		}
	}

	for i := 0; i < cfg.MaxBatchesPerRun; i++ {
		var res activities.ReplicateBatchResult
		if err := workflow.ExecuteActivity(actCtx, sa.ReplicateDraftHeadersBatch, activities.ReplicateBatchParams{BatchSize: cfg.DraftBatchSize}).Get(ctx, &res); err != nil {
			logger.Error("replicate draft headers batch failed; stopping draft headers for this run", "error", err)
			break
		}
		report.DraftHeadersReplicated += res.Replicated
		if res.Drained {
			break
		}
	}

	for i := 0; i < cfg.MaxBatchesPerRun; i++ {
		var res activities.ReplicateBatchResult
		if err := workflow.ExecuteActivity(actCtx, sa.ReplicateDraftPicksBatch, activities.ReplicateBatchParams{BatchSize: cfg.DraftBatchSize}).Get(ctx, &res); err != nil {
			logger.Error("replicate draft picks batch failed; stopping draft picks for this run", "error", err)
			break
		}
		report.DraftPicksReplicated += res.Replicated
		if res.Drained {
			break
		}
	}

	logger.Info("scavenger run complete", "leagues", report.LeaguesReplicated, "transactions", report.TransactionsReplicated,
		"draftHeaders", report.DraftHeadersReplicated, "draftPicks", report.DraftPicksReplicated)
	return report, nil
}
