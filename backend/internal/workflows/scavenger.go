package workflows

import (
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// drainStream runs up to maxBatches batches of a Replicate*Batch activity
// (activityFn — one of ScavengerActivities' four replicate methods),
// accumulating the replicated count. Returns once a batch reports Drained
// (the stream is caught up), the batch cap is hit (more work remains), or
// the activity errors. Callers decide what an error means for their own
// context — ScavengerDispatcher logs and moves on (the next 6h tick
// self-heals); ArchiveBackfillWorkflow fails the whole execution (it has no
// next tick to fall back on).
func drainStream(ctx, actCtx workflow.Context, activityFn interface{}, batchSize, maxBatches int) (replicated int, drained bool, err error) {
	for i := 0; i < maxBatches; i++ {
		var res activities.ReplicateBatchResult
		if err := workflow.ExecuteActivity(actCtx, activityFn, activities.ReplicateBatchParams{BatchSize: batchSize}).Get(ctx, &res); err != nil {
			return replicated, false, err
		}
		replicated += res.Replicated
		if res.Drained {
			return replicated, true, nil
		}
	}
	return replicated, false, nil
}

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

	replicated, _, err := drainStream(ctx, actCtx, sa.ReplicateLeaguesBatch, cfg.LeagueBatchSize, cfg.MaxBatchesPerRun)
	if err != nil {
		logger.Error("replicate leagues batch failed; stopping leagues for this run", "error", err)
	}
	report.LeaguesReplicated = replicated

	replicated, _, err = drainStream(ctx, actCtx, sa.ReplicateTransactionsBatch, cfg.TxnBatchSize, cfg.MaxBatchesPerRun)
	if err != nil {
		logger.Error("replicate transactions batch failed; stopping transactions for this run", "error", err)
	}
	report.TransactionsReplicated = replicated

	replicated, _, err = drainStream(ctx, actCtx, sa.ReplicateDraftHeadersBatch, cfg.DraftBatchSize, cfg.MaxBatchesPerRun)
	if err != nil {
		logger.Error("replicate draft headers batch failed; stopping draft headers for this run", "error", err)
	}
	report.DraftHeadersReplicated = replicated

	replicated, _, err = drainStream(ctx, actCtx, sa.ReplicateDraftPicksBatch, cfg.DraftBatchSize, cfg.MaxBatchesPerRun)
	if err != nil {
		logger.Error("replicate draft picks batch failed; stopping draft picks for this run", "error", err)
	}
	report.DraftPicksReplicated = replicated

	logger.Info("scavenger run complete", "leagues", report.LeaguesReplicated, "transactions", report.TransactionsReplicated,
		"draftHeaders", report.DraftHeadersReplicated, "draftPicks", report.DraftPicksReplicated)
	return report, nil
}
