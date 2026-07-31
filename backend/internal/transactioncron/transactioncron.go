// Package transactioncron implements cmd/cron's "transactions" job: claims
// stale leagues' transaction backlog and syncs it via internal/fdb.RunPool,
// which fetches each league's transactions concurrently and batches the
// resulting DB writes into one bulk statement per flush rather than one
// write per league.
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
// Keep PoolSize comfortably under DB_MAX_OPEN_CONNS: fetch doesn't hold a
// connection across its Sleeper HTTP calls (only flush does, briefly, once
// per batch), so the binding limit on PoolSize is Sleeper API concurrency,
// not DB connection starvation.
type Config struct {
	PoolSize    int `env:"CRON_TXN_POOL_SIZE,default=80,min=1"`
	RefillBatch int `env:"CRON_TXN_REFILL_BATCH,default=40,min=1"`
	// BatchSize flushes accumulated league results once this many have been
	// fetched.
	BatchSize int `env:"CRON_TXN_BATCH_SIZE,default=160,min=1"`
	// BatchFlushInterval flushes accumulated results at least this often,
	// even short of BatchSize, so results don't sit indefinitely.
	BatchFlushInterval time.Duration `env:"CRON_TXN_BATCH_FLUSH_INTERVAL_DURATION,default=5s,min=1s"`
	// ShutdownGracePeriod stops new claims before the cron's hard deadline so
	// in-flight Sleeper requests and their final DB batch can finish cleanly.
	ShutdownGracePeriod time.Duration `env:"CRON_TXN_SHUTDOWN_GRACE_PERIOD_DURATION,default=30s,min=0s"`
}

// LoadConfig reads Config from env.
func LoadConfig() Config {
	var cfg Config
	helpers.LoadEnvStruct(&cfg)
	return cfg
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
		"batchSize", cfg.BatchSize, "batchFlushInterval", cfg.BatchFlushInterval,
		"shutdownGracePeriod", cfg.ShutdownGracePeriod)
	start := time.Now()

	state, err := dfa.Sleeper.GetNFLState(ctx)
	if err != nil {
		logger.Warn("GetNFLState failed; falling back to full 18-leg sweep", "error", err)
		state = nil
	}

	result := fdb.RunPool(ctx, dfa.DB,
		fdb.Config{
			Size:                cfg.PoolSize,
			RefillBatch:         cfg.RefillBatch,
			PollInterval:        pollInterval,
			BatchSize:           cfg.BatchSize,
			BatchFlushInterval:  cfg.BatchFlushInterval,
			ShutdownGracePeriod: cfg.ShutdownGracePeriod,
		},
		func(ctx context.Context, db *gorm.DB, n int) ([]LeagueTransactionState, error) {
			return ClaimLeaguesForTransactions(ctx, db, ClaimLeaguesForTransactionsParams{BatchSize: n})
		},
		func(ctx context.Context, db *gorm.DB, lg LeagueTransactionState) (LeagueTransactionFetchResult, error) {
			return FetchLeagueTransactions(ctx, dfa, lg, state)
		},
		func(ctx context.Context, tx *gorm.DB, batch []LeagueTransactionFetchResult) error {
			flushStart := time.Now()
			err := FlushLeagueTransactions(ctx, dfa, tx, batch)
			var rows, advances int
			for _, r := range batch {
				rows += len(r.CloudRows) + len(r.ArchiveRows)
				if r.WeekWatermark > 0 {
					advances++
				}
			}
			if err != nil {
				logger.Warn("batch flush failed", "leagues", len(batch), "rows", rows, "duration", time.Since(flushStart), "error", err)
			} else {
				logger.Info("batch flush completed", "leagues", len(batch), "rows", rows, "watermarkAdvances", advances, "duration", time.Since(flushStart))
			}
			return err
		},
		func(lg LeagueTransactionState, err error, duration time.Duration) {
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
