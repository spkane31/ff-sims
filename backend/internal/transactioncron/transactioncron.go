// Package transactioncron implements cmd/cron's "transactions" job: claims
// stale leagues' transaction backlog and syncs it via internal/fdb.RunPool,
// which fetches each league's transactions concurrently and batches the
// resulting DB writes into one bulk statement per flush rather than one
// write per league. It replaced the Temporal-based transaction-sync pipeline
// (workflows.TransactionSyncDispatcher / activities.SyncLeagueTransactionsBatch,
// both since removed) — claim-based resilience via FOR UPDATE SKIP LOCKED and
// a claimed_at TTL is already database-native, so Temporal's orchestration
// machinery was redundant for this workload shape.
package transactioncron

import (
	"context"
	"time"

	"gorm.io/gorm"

	"backend/internal/activities"
	"backend/internal/fdb"
	"backend/internal/helpers"
)

// pollInterval mirrors discoverycron's: short enough that production notices
// newly-claimable work quickly and this package's tests finish fast.
const pollInterval = 200 * time.Millisecond

// Config holds the transaction-sync cron job's tuning knobs, read from env.
// Keep PoolSize comfortably under DB_MAX_OPEN_CONNS, though the constraint is
// looser than it used to be: fetch no longer holds a connection across its
// Sleeper HTTP calls (only flush does, briefly, once per batch), so the
// binding limit on PoolSize is now Sleeper API concurrency, not DB connection
// starvation.
type Config struct {
	PoolSize    int // CRON_TXN_POOL_SIZE, default 10
	RefillBatch int // CRON_TXN_REFILL_BATCH, default 4
	// BatchSize flushes accumulated league results once this many have been
	// fetched.
	BatchSize int // CRON_TXN_BATCH_SIZE, default 20
	// BatchFlushIntervalSec flushes accumulated results at least this often,
	// even short of BatchSize, so results don't sit indefinitely.
	BatchFlushIntervalSec int // CRON_TXN_BATCH_FLUSH_INTERVAL_SECONDS, default 5
}

// LoadConfig reads Config from env, clamped to at least 1.
func LoadConfig() Config {
	return Config{
		PoolSize:              max(helpers.GetEnv("CRON_TXN_POOL_SIZE", 10), 1),
		RefillBatch:           max(helpers.GetEnv("CRON_TXN_REFILL_BATCH", 4), 1),
		BatchSize:             max(helpers.GetEnv("CRON_TXN_BATCH_SIZE", 20), 1),
		BatchFlushIntervalSec: max(helpers.GetEnv("CRON_TXN_BATCH_FLUSH_INTERVAL_SECONDS", 5), 1),
	}
}

// Report summarizes one RunTransactionSync call.
type Report struct {
	LeaguesProcessed int
	LeaguesFailed    int
	// LeaguesDropped counts leagues whose fetch succeeded but whose batch's
	// flush failed — their claim stays in place and they're retried once it
	// expires. See fdb.Result.FlushDropped.
	LeaguesDropped int
	// FlushErrors counts how many flush calls failed (batch-level, not
	// per-league).
	FlushErrors int
	// ClaimErrors counts how many times the claim query returned a non-nil
	// error (e.g. Postgres unreachable) rather than a legitimately empty
	// queue — see discoverycron.Report's identical field for why cmd/cron
	// treats this distinction as failure.
	ClaimErrors int
}

// RunTransactionSync claims and syncs stale leagues' transactions until ctx
// is done (the caller — cmd/cron — sets ctx's deadline to -max-duration),
// then returns a summary. Fetches the NFL state once up front (the current
// week doesn't change within one run) and falls back to the full 18-leg
// sweep if that call fails.
func RunTransactionSync(ctx context.Context, dfa *activities.DataFetchActivities, cfg Config) (Report, error) {
	logger := newStdLogger()
	logger.Info("transaction sync cron starting", "poolSize", cfg.PoolSize, "refillBatch", cfg.RefillBatch,
		"batchSize", cfg.BatchSize, "batchFlushIntervalSec", cfg.BatchFlushIntervalSec)
	start := time.Now()

	state, err := dfa.Sleeper.GetNFLState(ctx)
	if err != nil {
		logger.Warn("GetNFLState failed; falling back to full 18-leg sweep", "error", err)
		state = nil
	}

	result := fdb.RunPool(ctx, dfa.DB,
		fdb.Config{
			Size:               cfg.PoolSize,
			RefillBatch:        cfg.RefillBatch,
			PollInterval:       pollInterval,
			BatchSize:          cfg.BatchSize,
			BatchFlushInterval: time.Duration(cfg.BatchFlushIntervalSec) * time.Second,
		},
		func(ctx context.Context, db *gorm.DB, n int) ([]activities.LeagueTransactionState, error) {
			return dfa.ClaimLeaguesForTransactions(ctx, db, activities.ClaimLeaguesForTransactionsParams{BatchSize: n})
		},
		func(ctx context.Context, db *gorm.DB, lg activities.LeagueTransactionState) (activities.LeagueTransactionFetchResult, error) {
			return dfa.FetchLeagueTransactions(ctx, db, lg, activities.MaxLegForLeague(lg.Season, state))
		},
		dfa.FlushLeagueTransactions,
		func(lg activities.LeagueTransactionState, err error, duration time.Duration) {
			if err != nil {
				logger.Warn("league transaction sync failed", "leagueID", lg.LeagueID, "error", err, "duration", duration)
				return
			}
			logger.Info("league transaction sync completed", "leagueID", lg.LeagueID, "duration", duration)
		},
	)

	report := Report{
		LeaguesProcessed: result.Processed,
		LeaguesFailed:    result.Failed,
		LeaguesDropped:   result.FlushDropped,
		FlushErrors:      result.FlushErrors,
		ClaimErrors:      result.ClaimErrors,
	}
	logger.Info("transaction sync cron finished", "duration", time.Since(start),
		"leaguesProcessed", report.LeaguesProcessed, "leaguesFailed", report.LeaguesFailed,
		"leaguesDropped", report.LeaguesDropped, "flushErrors", report.FlushErrors, "claimErrors", report.ClaimErrors)
	return report, nil
}
