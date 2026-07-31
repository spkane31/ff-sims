package discoverycron

import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"

	"backend/internal/fdb"
	"backend/internal/helpers"
	"backend/internal/sleeper"
)

// pollInterval is deliberately short (not fdb's 2s default) so a production
// run notices newly-claimable work quickly, and so this package's own tests
// (which run against tiny in-memory fixtures) finish in well under a second
// instead of waiting out a multi-second poll cadence.
const pollInterval = 200 * time.Millisecond

// discoveryLogTag is stamped on every discovery-related log line as a "tag"
// field, so a single grep on the shared worker-host journal — which also
// carries transaction-sync and ESPN worker log lines — pulls out exactly
// this pipeline's output:
//
//	journalctl -u ff-sims-discovery | grep discovery_trace
const discoveryLogTag = "discovery_trace"

// Config holds the discovery cron job's tuning knobs, read from env.
//
// Pool sizes are advertised as "scale up via env, no code change needed."
// Unlike before this package moved onto internal/fdb, a fetch goroutine no
// longer holds a DB connection across its Sleeper HTTP call(s) — only a
// batch's flush does, briefly, once per BatchSize/BatchFlushInterval — so
// the binding constraint on pool size is Sleeper API concurrency, not
// DB_MAX_OPEN_CONNS.
type Config struct {
	UserPoolSize      int `env:"CRON_DISCOVERY_USER_POOL_SIZE,default=15,min=1"`
	UserRefillBatch   int `env:"CRON_DISCOVERY_USER_REFILL_BATCH,default=10,min=1"`
	LeaguePoolSize    int `env:"CRON_DISCOVERY_LEAGUE_POOL_SIZE,default=10,min=1"`
	LeagueRefillBatch int `env:"CRON_DISCOVERY_LEAGUE_REFILL_BATCH,default=10,min=1"`
	// UserBatchSize/LeagueBatchSize flush each pool's accumulated results
	// once this many have been fetched.
	UserBatchSize   int `env:"CRON_DISCOVERY_USER_BATCH_SIZE,default=15,min=1"`
	LeagueBatchSize int `env:"CRON_DISCOVERY_LEAGUE_BATCH_SIZE,default=10,min=1"`
	// UserBatchFlushInterval/LeagueBatchFlushInterval flush accumulated
	// results at least this often, even short of the batch size, so results
	// don't sit indefinitely.
	UserBatchFlushInterval   time.Duration `env:"CRON_DISCOVERY_USER_BATCH_FLUSH_INTERVAL_DURATION,default=5s,min=1s"`
	LeagueBatchFlushInterval time.Duration `env:"CRON_DISCOVERY_LEAGUE_BATCH_FLUSH_INTERVAL_DURATION,default=5s,min=1s"`
}

// LoadConfig reads Config from env.
func LoadConfig() Config {
	var cfg Config
	helpers.LoadEnvStruct(&cfg)
	return cfg
}

// Report summarizes one RunDiscovery call.
type Report struct {
	UsersProcessed int
	UsersFailed    int
	// UsersDropped counts users whose fetch succeeded but whose batch's
	// flush failed — their claim stays in place and they're retried once it
	// expires. See fdb.Result.FlushDropped.
	UsersDropped int
	// UserFlushErrors counts how many user-pool flush calls failed
	// (batch-level, not per-user).
	UserFlushErrors int

	LeaguesProcessed int
	LeaguesFailed    int
	// LeaguesDropped is UsersDropped's analog for the league pool.
	LeaguesDropped int
	// LeagueFlushErrors is UserFlushErrors's analog for the league pool.
	LeagueFlushErrors int

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
// returns a summary. Each pool claims, fetches, and batch-flushes
// independently — see internal/fdb.RunPool for the claim/dispatch/batch
// loop shared by both.
func RunDiscovery(ctx context.Context, db *gorm.DB, sleeperClient *sleeper.Client, cfg Config) (Report, error) {
	logger := newStdLogger()
	logger.Info("discovery cron starting", "tag", discoveryLogTag,
		"userPoolSize", cfg.UserPoolSize, "userRefillBatch", cfg.UserRefillBatch,
		"leaguePoolSize", cfg.LeaguePoolSize, "leagueRefillBatch", cfg.LeagueRefillBatch)
	start := time.Now()

	var userResult, leagueResult fdb.Result
	var wg sync.WaitGroup

	wg.Go(func() {
		userResult = fdb.RunPool(ctx, db,
			fdb.Config{
				Size:               cfg.UserPoolSize,
				RefillBatch:        cfg.UserRefillBatch,
				PollInterval:       pollInterval,
				BatchSize:          cfg.UserBatchSize,
				BatchFlushInterval: cfg.UserBatchFlushInterval,
			},
			func(ctx context.Context, db *gorm.DB, n int) ([]string, error) {
				return ClaimStaleUsers(ctx, db, n)
			},
			func(ctx context.Context, db *gorm.DB, userID string) (UserDiscoveryResult, error) {
				return FetchUserLeagues(ctx, sleeperClient, userID)
			},
			FlushUserDiscovery,
			func(userID string, err error, duration time.Duration) {
				logResult(logger, "user", userID, err, duration)
			},
		)
	})

	wg.Go(func() {
		leagueResult = fdb.RunPool(ctx, db,
			fdb.Config{
				Size:               cfg.LeaguePoolSize,
				RefillBatch:        cfg.LeagueRefillBatch,
				PollInterval:       pollInterval,
				BatchSize:          cfg.LeagueBatchSize,
				BatchFlushInterval: cfg.LeagueBatchFlushInterval,
			},
			func(ctx context.Context, db *gorm.DB, n int) ([]string, error) {
				return ClaimStaleLeagues(ctx, db, n)
			},
			func(ctx context.Context, db *gorm.DB, leagueID string) (LeagueDiscoveryResult, error) {
				return FetchLeague(ctx, db, sleeperClient, leagueID)
			},
			FlushLeagueDiscovery,
			func(leagueID string, err error, duration time.Duration) {
				logResult(logger, "league", leagueID, err, duration)
			},
		)
	})

	wg.Wait()

	report := Report{
		UsersProcessed:    userResult.Processed,
		UsersFailed:       userResult.Failed,
		UsersDropped:      userResult.FlushDropped,
		UserFlushErrors:   userResult.FlushErrors,
		LeaguesProcessed:  leagueResult.Processed,
		LeaguesFailed:     leagueResult.Failed,
		LeaguesDropped:    leagueResult.FlushDropped,
		LeagueFlushErrors: leagueResult.FlushErrors,
		UserClaimErrors:   userResult.ClaimErrors,
		LeagueClaimErrors: leagueResult.ClaimErrors,
	}
	logger.Info("discovery cron finished", "tag", discoveryLogTag,
		"duration", time.Since(start),
		"usersProcessed", report.UsersProcessed, "usersFailed", report.UsersFailed, "usersDropped", report.UsersDropped,
		"leaguesProcessed", report.LeaguesProcessed, "leaguesFailed", report.LeaguesFailed, "leaguesDropped", report.LeaguesDropped)
	return report, nil
}

func logResult(logger *stdLogger, kind, id string, err error, duration time.Duration) {
	if err != nil {
		logger.Warn(kind+" failed", "tag", discoveryLogTag, "id", id, "error", err, "duration", duration)
		return
	}
	logger.Info(kind+" completed", "tag", discoveryLogTag, "id", id, "duration", duration)
}
