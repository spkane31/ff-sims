// Package transactioncron replaces the Temporal-based transaction-sync
// pipeline (workflows.TransactionSyncDispatcher /
// activities.SyncLeagueTransactionsBatch) with a plain Go implementation
// driven by a systemd timer, mirroring internal/discoverycron's design — see
// docs/superpowers/specs/2026-07-15-discovery-cron-migration-design.md for
// the original rationale (claim-based resilience is already database-native,
// so Temporal's orchestration machinery is redundant for this workload
// shape). Both paths claim through the same sleeper_leagues.claimed_at
// column and are safe to run concurrently — FOR UPDATE SKIP LOCKED already
// partitions the backlog across claimers, same as the Temporal dispatcher's
// own ParallelBatches does with itself.
package transactioncron

import (
	"context"
	"sync"
	"time"

	"backend/internal/activities"
	"backend/internal/cronpool"
	"backend/internal/helpers"
)

// pollInterval mirrors discoverycron's: short enough that production notices
// newly-claimable work quickly and this package's tests finish fast.
const pollInterval = 200 * time.Millisecond

// Config holds the transaction-sync cron job's tuning knobs, read from env.
// Uses a CRON_TXN_ prefix (distinct from the Temporal path's TXN_SYNC_* vars,
// which remain in effect for workflows.TransactionSyncDispatcher) so the two
// paths can be tuned independently while both run. Keep PoolSize comfortably
// under DB_MAX_OPEN_CONNS — SyncOneLeagueTransactions holds a pooled
// connection for the duration of its Sleeper HTTP calls plus its writes, not
// just the writes (same consideration as discoverycron's league pool).
type Config struct {
	PoolSize    int // CRON_TXN_POOL_SIZE, default 8
	RefillBatch int // CRON_TXN_REFILL_BATCH, default 4
}

// LoadConfig reads Config from env, clamped to at least 1.
func LoadConfig() Config {
	return Config{
		PoolSize:    max(helpers.GetEnv("CRON_TXN_POOL_SIZE", 8), 1),
		RefillBatch: max(helpers.GetEnv("CRON_TXN_REFILL_BATCH", 4), 1),
	}
}

// Report summarizes one RunTransactionSync call.
type Report struct {
	LeaguesProcessed int
	LeaguesFailed    int
	// ClaimErrors counts how many times the claim query returned a non-nil
	// error (e.g. Postgres unreachable) rather than a legitimately empty
	// queue — see discoverycron.Report's identical field for why cmd/cron
	// treats this distinction as failure.
	ClaimErrors int
}

// RunTransactionSync claims and syncs stale leagues' transactions until ctx
// is done (the caller — cmd/cron — sets ctx's deadline to -max-duration),
// then returns a summary. Fetches the NFL state once up front (mirroring
// SyncLeagueTransactionsBatch's once-per-batch fetch — the current week
// doesn't change within one run) and falls back to the full 18-leg sweep if
// that call fails.
func RunTransactionSync(ctx context.Context, dfa *activities.DataFetchActivities, cfg Config) (Report, error) {
	logger := newStdLogger()
	logger.Info("transaction sync cron starting", "poolSize", cfg.PoolSize, "refillBatch", cfg.RefillBatch)
	start := time.Now()

	state, err := dfa.Sleeper.GetNFLState(ctx)
	if err != nil {
		logger.Warn("GetNFLState failed; falling back to full 18-leg sweep", "error", err)
		state = nil
	}

	// RunPool's claim/process functions carry only item IDs; the claim query
	// returns richer per-league state (season, leg cursor) that
	// SyncOneLeagueTransactions needs, so stash it here between claim and
	// process rather than re-querying per item.
	var mu sync.Mutex
	pending := make(map[string]activities.LeagueTransactionState)

	result := cronpool.RunPool(ctx,
		cronpool.PoolConfig{Size: cfg.PoolSize, RefillBatch: cfg.RefillBatch, PollInterval: pollInterval},
		func(ctx context.Context, n int) ([]string, error) {
			leagues, err := dfa.ClaimLeaguesForTransactions(ctx, activities.ClaimLeaguesForTransactionsParams{BatchSize: n})
			if err != nil {
				return nil, err
			}
			ids := make([]string, len(leagues))
			mu.Lock()
			for i, lg := range leagues {
				ids[i] = lg.LeagueID
				pending[lg.LeagueID] = lg
			}
			mu.Unlock()
			return ids, nil
		},
		func(ctx context.Context, id string) error {
			mu.Lock()
			lg, ok := pending[id]
			delete(pending, id)
			mu.Unlock()
			if !ok {
				// Should be unreachable: RunPool only ever processes IDs it
				// just received from the claim function above.
				return nil
			}
			return dfa.SyncOneLeagueTransactions(ctx, lg, activities.MaxLegForLeague(lg.Season, state))
		},
		func(id string, err error, duration time.Duration) {
			if err != nil {
				logger.Warn("league transaction sync failed", "leagueID", id, "error", err, "duration", duration)
				return
			}
			logger.Info("league transaction sync completed", "leagueID", id, "duration", duration)
		},
	)

	report := Report{
		LeaguesProcessed: result.Processed,
		LeaguesFailed:    result.Failed,
		ClaimErrors:      result.ClaimErrors,
	}
	logger.Info("transaction sync cron finished", "duration", time.Since(start),
		"leaguesProcessed", report.LeaguesProcessed, "leaguesFailed", report.LeaguesFailed, "claimErrors", report.ClaimErrors)
	return report, nil
}
