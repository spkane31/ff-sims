package statscron

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"backend/internal/activities"
)

func TestDrainBatches_StopsOnDrained(t *testing.T) {
	calls := 0
	fn := func(ctx context.Context, p activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error) {
		calls++
		if calls == 1 {
			return activities.ReplicateBatchResult{Replicated: 3, Drained: false}, nil
		}
		return activities.ReplicateBatchResult{Replicated: 1, Drained: true}, nil
	}

	replicated, drained, err := drainBatches(context.Background(), fn, 10, 50)

	require.NoError(t, err)
	require.True(t, drained)
	require.Equal(t, 4, replicated)
	require.Equal(t, 2, calls)
}

func TestDrainBatches_StopsAtMaxBatchesWithoutDraining(t *testing.T) {
	calls := 0
	fn := func(ctx context.Context, p activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error) {
		calls++
		return activities.ReplicateBatchResult{Replicated: 5, Drained: false}, nil
	}

	replicated, drained, err := drainBatches(context.Background(), fn, 10, 3)

	require.NoError(t, err)
	require.False(t, drained)
	require.Equal(t, 15, replicated)
	require.Equal(t, 3, calls)
}

func TestDrainBatches_PropagatesError(t *testing.T) {
	boom := errors.New("boom")
	fn := func(ctx context.Context, p activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error) {
		return activities.ReplicateBatchResult{}, boom
	}

	_, drained, err := drainBatches(context.Background(), fn, 10, 50)

	require.ErrorIs(t, err, boom)
	require.False(t, drained)
}

// fakeScavengerOps is a hand-rolled scavengerOps double letting tests
// script each stream's result the same way the retired Temporal
// ScavengerDispatcher tests did via env.OnActivity mocks.
type fakeScavengerOps struct {
	leagues, transactions, draftHeaders, draftPicks func(context.Context, activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error)
	purgeTxns, purgeDrafts                          func(context.Context, activities.PurgeBatchParams) (activities.PurgeBatchResult, error)

	purgeTxnsCalled, purgeDraftsCalled bool
}

func drainedOnce(replicated int) func(context.Context, activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error) {
	return func(context.Context, activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error) {
		return activities.ReplicateBatchResult{Replicated: replicated, Drained: true}, nil
	}
}

func (f *fakeScavengerOps) ReplicateLeaguesBatch(ctx context.Context, p activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error) {
	return f.leagues(ctx, p)
}
func (f *fakeScavengerOps) ReplicateTransactionsBatch(ctx context.Context, p activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error) {
	return f.transactions(ctx, p)
}
func (f *fakeScavengerOps) ReplicateDraftHeadersBatch(ctx context.Context, p activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error) {
	return f.draftHeaders(ctx, p)
}
func (f *fakeScavengerOps) ReplicateDraftPicksBatch(ctx context.Context, p activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error) {
	return f.draftPicks(ctx, p)
}
func (f *fakeScavengerOps) PurgeTransactionsBatch(ctx context.Context, p activities.PurgeBatchParams) (activities.PurgeBatchResult, error) {
	f.purgeTxnsCalled = true
	return f.purgeTxns(ctx, p)
}
func (f *fakeScavengerOps) PurgeDraftsBatch(ctx context.Context, p activities.PurgeBatchParams) (activities.PurgeBatchResult, error) {
	f.purgeDraftsCalled = true
	return f.purgeDrafts(ctx, p)
}

func baseCfg() activities.ScavengerConfig {
	return activities.ScavengerConfig{
		LeagueBatchSize: 500, TxnBatchSize: 5000, DraftBatchSize: 200, MaxBatchesPerRun: 50,
		RetentionDays: 30, PurgeEnabled: false,
	}
}

func TestSyncArchive_DrainsAllStreams(t *testing.T) {
	sa := &fakeScavengerOps{
		leagues:      drainedOnce(3),
		transactions: drainedOnce(10),
		draftHeaders: drainedOnce(2),
		draftPicks:   drainedOnce(1),
	}

	report, err := syncArchive(context.Background(), sa, baseCfg())

	require.NoError(t, err)
	require.Equal(t, activities.ScavengerReport{
		LeaguesReplicated: 3, TransactionsReplicated: 10, DraftHeadersReplicated: 2, DraftPicksReplicated: 1,
	}, report)
}

func TestSyncArchive_ReplicateStreamFailureIsSwallowedAndOtherStreamsStillRun(t *testing.T) {
	boom := errors.New("boom")
	sa := &fakeScavengerOps{
		leagues: func(context.Context, activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error) {
			return activities.ReplicateBatchResult{}, boom
		},
		transactions: drainedOnce(5),
		draftHeaders: drainedOnce(0),
		draftPicks:   drainedOnce(0),
	}

	report, err := syncArchive(context.Background(), sa, baseCfg())

	require.NoError(t, err) // stream failures are logged and swallowed, not propagated
	require.Equal(t, 0, report.LeaguesReplicated)
	require.Equal(t, 5, report.TransactionsReplicated)
}

func TestSyncArchive_PurgeDisabledByDefault_NeverCallsPurge(t *testing.T) {
	sa := &fakeScavengerOps{
		leagues: drainedOnce(0), transactions: drainedOnce(0), draftHeaders: drainedOnce(0), draftPicks: drainedOnce(0),
	}
	cfg := baseCfg()
	cfg.PurgeEnabled = false

	_, err := syncArchive(context.Background(), sa, cfg)

	require.NoError(t, err)
	require.False(t, sa.purgeTxnsCalled)
	require.False(t, sa.purgeDraftsCalled)
}

func TestSyncArchive_PurgeEnabledAndCaughtUp_RunsPurgeAndAccumulatesReport(t *testing.T) {
	sa := &fakeScavengerOps{
		leagues: drainedOnce(0), transactions: drainedOnce(0), draftHeaders: drainedOnce(0), draftPicks: drainedOnce(0),
		purgeTxns: func(context.Context, activities.PurgeBatchParams) (activities.PurgeBatchResult, error) {
			return activities.PurgeBatchResult{Purged: 100, Unverified: 2, Drained: true}, nil
		},
		purgeDrafts: func(context.Context, activities.PurgeBatchParams) (activities.PurgeBatchResult, error) {
			return activities.PurgeBatchResult{Purged: 4, Unverified: 1, Drained: true}, nil
		},
	}
	cfg := baseCfg()
	cfg.PurgeEnabled = true

	report, err := syncArchive(context.Background(), sa, cfg)

	require.NoError(t, err)
	require.Equal(t, 100, report.TransactionsPurged)
	require.Equal(t, 2, report.TransactionsUnverified)
	require.Equal(t, 4, report.DraftsPurged)
	require.Equal(t, 1, report.DraftsUnverified)
}

func TestSyncArchive_PurgeSkippedWhenReplicateNotCaughtUp(t *testing.T) {
	notDrained := func(context.Context, activities.ReplicateBatchParams) (activities.ReplicateBatchResult, error) {
		return activities.ReplicateBatchResult{Replicated: 500, Drained: false}, nil
	}
	sa := &fakeScavengerOps{
		leagues: notDrained, transactions: notDrained, draftHeaders: notDrained, draftPicks: notDrained,
	}
	cfg := baseCfg()
	cfg.PurgeEnabled = true
	cfg.MaxBatchesPerRun = 1 // every stream hits the iteration cap without draining

	_, err := syncArchive(context.Background(), sa, cfg)

	require.NoError(t, err)
	require.False(t, sa.purgeTxnsCalled)
	require.False(t, sa.purgeDraftsCalled)
}

func TestSyncArchive_PurgeErrorIsNotSwallowed(t *testing.T) {
	boom := errors.New("replication stalled")
	sa := &fakeScavengerOps{
		leagues: drainedOnce(0), transactions: drainedOnce(0), draftHeaders: drainedOnce(0), draftPicks: drainedOnce(0),
		purgeTxns: func(context.Context, activities.PurgeBatchParams) (activities.PurgeBatchResult, error) {
			return activities.PurgeBatchResult{}, boom
		},
	}
	cfg := baseCfg()
	cfg.PurgeEnabled = true

	_, err := syncArchive(context.Background(), sa, cfg)

	require.ErrorIs(t, err, boom) // unlike replicate stream failures, purge errors must NOT be swallowed
}
