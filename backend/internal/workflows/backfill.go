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

// ArchiveBackfillWorkflow is started once, manually, to catch the archive up
// on pre-existing cloud history — the scavenger's 6h schedule only
// replicates forward from wherever the cursors already are. Reuses the same
// four replicate activities and cursors as ScavengerDispatcher; it's the
// same copy operation, just run back-to-back until there's nothing left
// instead of capped per 6h tick. Unlike the scheduled scavenger, a stream's
// activity failure here fails the whole execution rather than being logged
// and skipped: this is a manually-monitored one-time job, and silently
// reporting "drained" while a stream is actually broken risks leaving data
// behind unnoticed.
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
