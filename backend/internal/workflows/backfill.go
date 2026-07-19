package workflows

import (
	"fmt"

	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// backfillBatchesPerExecution bounds how many batches each stream drains
// within a single workflow execution before ContinueAsNew hands off to a
// fresh one. Unlike the scheduled dispatchers (which rely on the next
// scheduled tick to pick up any remainder — see MaxDispatchIterations),
// ArchiveBackfillWorkflow has no schedule to fall back on, so it must keep
// itself going; ContinueAsNew keeps any single execution's history small
// regardless of how large the backlog is.
const backfillBatchesPerExecution = 100

// drainStream runs up to maxBatches batches of a Replicate*Batch activity
// (activityFn — one of ScavengerActivities' four replicate methods),
// accumulating the replicated count. Returns once a batch reports Drained
// (the stream is caught up), the batch cap is hit (more work remains), or
// the activity errors. The plain-Go equivalent for the recurring hourly
// archive sync (internal/statscron/archive_sync.go's drainBatches) fails a
// stream independently and moves on, since there's a next tick to retry;
// this one is only used by ArchiveBackfillWorkflow below, which has no next
// tick and so fails the whole execution on any error instead.
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

// ArchiveBackfillWorkflow is started once, manually, to catch the archive up
// on pre-existing cloud history — the recurring hourly archive sync
// (internal/statscron's "lifetime-counts" cron job) only replicates forward
// from wherever the cursors already are. Reuses the same four replicate
// activities and cursors as that job; it's the same copy operation, just
// run back-to-back until there's nothing left instead of capped per tick.
// Unlike the recurring sync, a stream's activity failure here fails the
// whole execution rather than being logged and skipped: this is a
// manually-monitored one-time job, and silently reporting "drained" while a
// stream is actually broken risks leaving data behind unnoticed.
func ArchiveBackfillWorkflow(ctx workflow.Context) (BackfillReport, error) {
	sa := &activities.ScavengerActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	logger := workflow.GetLogger(ctx)

	var cfg activities.ScavengerConfig
	if err := workflow.ExecuteActivity(actCtx, sa.GetScavengerConfig).Get(ctx, &cfg); err != nil {
		return BackfillReport{}, err
	}

	allDrained := true

	leaguesReplicated, leaguesDrained, err := drainStream(ctx, actCtx, sa.ReplicateLeaguesBatch, cfg.LeagueBatchSize, backfillBatchesPerExecution)
	if err != nil {
		return BackfillReport{}, fmt.Errorf("replicate leagues: %w", err)
	}
	allDrained = allDrained && leaguesDrained

	txnReplicated, txnDrained, err := drainStream(ctx, actCtx, sa.ReplicateTransactionsBatch, cfg.TxnBatchSize, backfillBatchesPerExecution)
	if err != nil {
		return BackfillReport{}, fmt.Errorf("replicate transactions: %w", err)
	}
	allDrained = allDrained && txnDrained

	headersReplicated, headersDrained, err := drainStream(ctx, actCtx, sa.ReplicateDraftHeadersBatch, cfg.DraftBatchSize, backfillBatchesPerExecution)
	if err != nil {
		return BackfillReport{}, fmt.Errorf("replicate draft headers: %w", err)
	}
	allDrained = allDrained && headersDrained

	picksReplicated, picksDrained, err := drainStream(ctx, actCtx, sa.ReplicateDraftPicksBatch, cfg.DraftBatchSize, backfillBatchesPerExecution)
	if err != nil {
		return BackfillReport{}, fmt.Errorf("replicate draft picks: %w", err)
	}
	allDrained = allDrained && picksDrained

	report := BackfillReport{
		LeaguesReplicated:      leaguesReplicated,
		TransactionsReplicated: txnReplicated,
		DraftHeadersReplicated: headersReplicated,
		DraftPicksReplicated:   picksReplicated,
	}

	logger.Info("backfill execution complete", "leagues", leaguesReplicated, "transactions", txnReplicated,
		"draftHeaders", headersReplicated, "draftPicks", picksReplicated, "allDrained", allDrained)

	if !allDrained {
		return report, workflow.NewContinueAsNewError(ctx, ArchiveBackfillWorkflow)
	}
	logger.Info("archive backfill complete")
	return report, nil
}
