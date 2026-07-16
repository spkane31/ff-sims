package discoverycron

import (
	"context"
	"sync"
	"time"

	"backend/internal/activities"
	"backend/internal/helpers"
)

// pollInterval is deliberately short (not defaultPollInterval's 2s) so a
// production run notices newly-claimable work quickly, and so this
// package's own tests (which run against tiny in-memory fixtures) finish in
// well under a second instead of waiting out a multi-second poll cadence.
const pollInterval = 200 * time.Millisecond

// Config holds the discovery cron job's tuning knobs, read from env. Uses a
// CRON_DISCOVERY_ prefix (distinct from the Temporal path's DISCOVERY_*
// vars, which remain in effect for workflows.DiscoveryBatchDispatcher) so
// the two paths can be tuned independently while both run.
//
// UserPoolSize and LeaguePoolSize are advertised as "scale up via env, no
// code change needed" — but keep UserPoolSize + LeaguePoolSize comfortably
// under DB_MAX_OPEN_CONNS (the process-wide DB connection pool ceiling,
// shared with everything else the process does). ProcessLeague (process.go)
// wraps FetchLeagueMembers + FetchLeagueDetails — each a Sleeper HTTP fetch
// (up to 30s, times retries) plus a DB write — inside a single
// db.Transaction, which holds a pooled connection (and row locks on the
// leagues/users being upserted) for the full duration of both HTTP calls,
// not just the writes. Pushing these pool sizes up toward or past
// DB_MAX_OPEN_CONNS causes connection-acquisition starvation and stalls,
// not more throughput.
type Config struct {
	UserPoolSize      int // CRON_DISCOVERY_USER_POOL_SIZE, default 4
	UserRefillBatch   int // CRON_DISCOVERY_USER_REFILL_BATCH, default 2
	LeaguePoolSize    int // CRON_DISCOVERY_LEAGUE_POOL_SIZE, default 4
	LeagueRefillBatch int // CRON_DISCOVERY_LEAGUE_REFILL_BATCH, default 2
}

// LoadConfig reads Config from env, clamped to at least 1 so a bad value
// can't stall the pools or break a claim query's LIMIT.
func LoadConfig() Config {
	return Config{
		UserPoolSize:      max(helpers.GetEnv("CRON_DISCOVERY_USER_POOL_SIZE", 4), 1),
		UserRefillBatch:   max(helpers.GetEnv("CRON_DISCOVERY_USER_REFILL_BATCH", 2), 1),
		LeaguePoolSize:    max(helpers.GetEnv("CRON_DISCOVERY_LEAGUE_POOL_SIZE", 4), 1),
		LeagueRefillBatch: max(helpers.GetEnv("CRON_DISCOVERY_LEAGUE_REFILL_BATCH", 2), 1),
	}
}

// Report summarizes one RunDiscovery call.
type Report struct {
	UsersProcessed   int
	UsersFailed      int
	LeaguesProcessed int
	LeaguesFailed    int
	// UserClaimErrors and LeagueClaimErrors count how many times each pool's
	// claim call returned a non-nil error (e.g. Postgres unreachable) rather
	// than a legitimately empty queue. A run with zero processed/failed items
	// but nonzero claim errors means the job made no progress because it
	// couldn't talk to the database, not because there was nothing to do —
	// callers (cmd/cron) should treat that distinction as failure.
	UserClaimErrors   int
	LeagueClaimErrors int
}

// RunDiscovery runs the user pool and league pool concurrently until ctx is
// done (the caller — cmd/cron — sets ctx's deadline to -max-duration), then
// returns a summary. Each pool claims and processes items independently;
// see RunPool for the claim/process/refill loop shared by both.
func RunDiscovery(ctx context.Context, da *activities.DiscoveryActivities, cfg Config) (Report, error) {
	logger := newStdLogger()
	logger.Info("discovery cron starting", "tag", activities.DiscoveryLogTag,
		"userPoolSize", cfg.UserPoolSize, "userRefillBatch", cfg.UserRefillBatch,
		"leaguePoolSize", cfg.LeaguePoolSize, "leagueRefillBatch", cfg.LeagueRefillBatch)
	start := time.Now()

	var userResult, leagueResult PoolResult
	var wg sync.WaitGroup

	wg.Go(func() {
		userResult = RunPool(ctx,
			PoolConfig{Size: cfg.UserPoolSize, RefillBatch: cfg.UserRefillBatch, PollInterval: pollInterval},
			func(ctx context.Context, n int) ([]string, error) {
				return da.ClaimStaleUsers(ctx, activities.ClaimStaleUsersParams{BatchSize: n})
			},
			func(ctx context.Context, id string) error {
				return ProcessUser(ctx, da, id)
			},
			func(id string, err error, duration time.Duration) {
				logResult(logger, "user", id, err, duration)
			},
		)
	})

	wg.Go(func() {
		leagueResult = RunPool(ctx,
			PoolConfig{Size: cfg.LeaguePoolSize, RefillBatch: cfg.LeagueRefillBatch, PollInterval: pollInterval},
			func(ctx context.Context, n int) ([]string, error) {
				return ClaimStaleLeagues(ctx, da.DB, n)
			},
			func(ctx context.Context, id string) error {
				return ProcessLeague(ctx, da, id)
			},
			func(id string, err error, duration time.Duration) {
				logResult(logger, "league", id, err, duration)
			},
		)
	})

	wg.Wait()

	report := Report{
		UsersProcessed:    userResult.Processed,
		UsersFailed:       userResult.Failed,
		LeaguesProcessed:  leagueResult.Processed,
		LeaguesFailed:     leagueResult.Failed,
		UserClaimErrors:   userResult.ClaimErrors,
		LeagueClaimErrors: leagueResult.ClaimErrors,
	}
	logger.Info("discovery cron finished", "tag", activities.DiscoveryLogTag,
		"duration", time.Since(start),
		"usersProcessed", report.UsersProcessed, "usersFailed", report.UsersFailed,
		"leaguesProcessed", report.LeaguesProcessed, "leaguesFailed", report.LeaguesFailed)
	return report, nil
}

func logResult(logger *stdLogger, kind, id string, err error, duration time.Duration) {
	if err != nil {
		logger.Warn(kind+" failed", "tag", activities.DiscoveryLogTag, "id", id, "error", err, "duration", duration)
		return
	}
	logger.Info(kind+" completed", "tag", activities.DiscoveryLogTag, "id", id, "duration", duration)
}
