package statscron

import (
	"context"
	"log"

	"backend/internal/activities"
)

// drainBatches runs up to maxBatches batches of a Replicate*Batch activity
// method (batchFn), accumulating the replicated count. Returns once a batch
// reports Drained (the stream is caught up), the batch cap is hit (more work
// remains — the next hourly run resumes from wherever the cursor landed), or
// the activity errors. Plain-Go equivalent of the Temporal-era
// internal/workflows/scavenger.go's drainStream (still used there by
// ArchiveBackfillWorkflow), just calling batchFn directly instead of
// workflow.ExecuteActivity.
func drainBatches(
	ctx context.Context,
	batchFn func(context.Context, activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error),
	batchSize, maxBatches int,
) (replicated int, drained bool, err error) {
	for i := 0; i < maxBatches; i++ {
		res, err := batchFn(ctx, activities.ReplicateBatchParams{BatchSize: batchSize})
		if err != nil {
			return replicated, false, err
		}
		replicated += res.Replicated
		if res.Drained {
			return replicated, true, nil
		}
	}
	return replicated, false, nil
}

// scavengerOps is the subset of *activities.ScavengerActivities' methods
// syncArchive needs — an interface (rather than taking the concrete type
// directly) so tests can supply a fake and assert syncArchive's
// orchestration (stream order, which errors are swallowed vs propagated,
// purge gating) without a real database, the same thing the Temporal-era
// ScavengerDispatcher's tests did via env.OnActivity mocks.
type scavengerOps interface {
	ReplicateLeaguesBatch(context.Context, activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error)
	ReplicateTransactionsBatch(context.Context, activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error)
	ReplicateDraftHeadersBatch(context.Context, activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error)
	ReplicateDraftPicksBatch(context.Context, activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error)
	PurgeTransactionsBatch(context.Context, activities.PurgeBatchParams) (activities.PurgeBatchResult, error)
	PurgeDraftsBatch(context.Context, activities.PurgeBatchParams) (activities.PurgeBatchResult, error)
}

// syncArchive replicates cloud → archive across four streams, in order:
// leagues, transactions, draft headers, draft picks. Each stream drains
// independently up to cfg.MaxBatchesPerRun batches or until a short batch
// signals it's caught up; a stream's replicate error is logged and stops
// only that stream for this run — the cursor didn't move (advance commits
// atomically with the copied rows), so the next hourly run resumes from the
// same position.
//
// After replication, the purge phase (transactions, then drafts+picks)
// deletes verified-old cloud rows — but only when cfg.PurgeEnabled is true
// AND the corresponding replicate stream(s) drained this run, so purge never
// scans ahead of a backlog it already knows exists. Unlike the replicate
// loops above, a purge error is NOT swallowed: PurgeTransactionsBatch and
// PurgeDraftsBatch only ever return an error when the oldest unverified row
// has sat past retention+15d, meaning replication has stalled — that must
// surface as a failed run (RunSnapshot's caller treats a non-nil error as
// "skip writing this hour's row"), the intended stalled-replication alarm
// now that there's no Temporal UI to show a red run.
//
// This is the direct successor to the Temporal-era ScavengerDispatcher
// workflow (formerly internal/workflows/scavenger.go), ported to run inline
// as part of the lifetime-counts cron job instead of on its own Temporal
// schedule — see RunSnapshot's doc comment for why.
func syncArchive(ctx context.Context, sa scavengerOps, cfg activities.ScavengerConfig) (activities.ScavengerReport, error) {
	var report activities.ScavengerReport

	replicated, _, err := drainBatches(ctx, sa.ReplicateLeaguesBatch, cfg.LeagueBatchSize, cfg.MaxBatchesPerRun)
	if err != nil {
		log.Printf("statscron: replicate leagues batch failed; stopping leagues for this run: %v", err)
	}
	report.LeaguesReplicated = replicated

	replicated, txnDrained, err := drainBatches(ctx, sa.ReplicateTransactionsBatch, cfg.TxnBatchSize, cfg.MaxBatchesPerRun)
	if err != nil {
		log.Printf("statscron: replicate transactions batch failed; stopping transactions for this run: %v", err)
	}
	report.TransactionsReplicated = replicated

	replicated, draftHeadersDrained, err := drainBatches(ctx, sa.ReplicateDraftHeadersBatch, cfg.DraftBatchSize, cfg.MaxBatchesPerRun)
	if err != nil {
		log.Printf("statscron: replicate draft headers batch failed; stopping draft headers for this run: %v", err)
	}
	report.DraftHeadersReplicated = replicated

	replicated, draftPicksDrained, err := drainBatches(ctx, sa.ReplicateDraftPicksBatch, cfg.DraftBatchSize, cfg.MaxBatchesPerRun)
	if err != nil {
		log.Printf("statscron: replicate draft picks batch failed; stopping draft picks for this run: %v", err)
	}
	report.DraftPicksReplicated = replicated

	if cfg.PurgeEnabled && txnDrained {
		for i := 0; i < cfg.MaxBatchesPerRun; i++ {
			res, err := sa.PurgeTransactionsBatch(ctx, activities.PurgeBatchParams{
				BatchSize: cfg.TxnBatchSize, RetentionDays: cfg.RetentionDays,
			})
			if err != nil {
				return report, err
			}
			report.TransactionsPurged += res.Purged
			report.TransactionsUnverified += res.Unverified
			if res.Drained {
				break
			}
		}
	}

	if cfg.PurgeEnabled && draftHeadersDrained && draftPicksDrained {
		for i := 0; i < cfg.MaxBatchesPerRun; i++ {
			res, err := sa.PurgeDraftsBatch(ctx, activities.PurgeBatchParams{
				BatchSize: cfg.DraftBatchSize, RetentionDays: cfg.RetentionDays,
			})
			if err != nil {
				return report, err
			}
			report.DraftsPurged += res.Purged
			report.DraftsUnverified += res.Unverified
			if res.Drained {
				break
			}
		}
	}

	log.Printf("statscron: archive sync complete leagues=%d transactions=%d draftHeaders=%d draftPicks=%d transactionsPurged=%d transactionsUnverified=%d draftsPurged=%d draftsUnverified=%d",
		report.LeaguesReplicated, report.TransactionsReplicated, report.DraftHeadersReplicated, report.DraftPicksReplicated,
		report.TransactionsPurged, report.TransactionsUnverified, report.DraftsPurged, report.DraftsUnverified)
	return report, nil
}
