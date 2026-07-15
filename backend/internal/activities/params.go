package activities

import "backend/internal/models"

type ClaimStaleUsersParams struct {
	BatchSize int
}

// DiscoveryConfig is read from env by GetDiscoveryConfig so the dispatcher
// workflow (which cannot read env deterministically) can be tuned without a
// redeploy of workflow code. Discovery batches are smaller than the sync
// paths because each user fans out into per-league member/detail fetches.
type DiscoveryConfig struct {
	ParallelBatches    int // DISCOVERY_PARALLEL_BATCHES, default 1
	BatchSize          int // DISCOVERY_BATCH_SIZE, default 20
	Concurrency        int // DISCOVERY_USER_CONCURRENCY, default 4
	UserTimeoutSeconds int // DISCOVERY_USER_TIMEOUT_SECONDS, default 90
	LeagueConcurrency  int // DISCOVERY_LEAGUE_CONCURRENCY, default 10
}

type DiscoverUsersBatchParams struct {
	UserIDs            []string
	Concurrency        int
	UserTimeoutSeconds int
	LeagueConcurrency  int
}

type FetchUserLeaguesParams struct {
	UserID string
}

type FetchLeagueMembersParams struct {
	LeagueID string
}

type FetchLeagueDetailsParams struct {
	LeagueID string
}

type ClaimLeaguesForDraftsParams struct {
	BatchSize int
}

// DraftSyncConfig is read from env by GetDraftSyncConfig so the dispatcher
// workflow (which cannot read env deterministically) can be tuned without a
// redeploy of workflow code.
type DraftSyncConfig struct {
	ParallelBatches int // DRAFT_SYNC_PARALLEL_BATCHES, default 2
	BatchSize       int // DRAFT_SYNC_BATCH_SIZE, default 100
	Concurrency     int // DRAFT_SYNC_LEAGUE_CONCURRENCY, default 8
}

type SyncLeagueDraftsBatchParams struct {
	LeagueIDs   []string
	Concurrency int
}

type ClaimLeaguesForTransactionsParams struct {
	BatchSize int
}

// TransactionSyncConfig is read from env by GetTransactionSyncConfig so the
// dispatcher workflow (which cannot read env deterministically) can be tuned
// without a redeploy of workflow code.
type TransactionSyncConfig struct {
	ParallelBatches int // TXN_SYNC_PARALLEL_BATCHES, default 2
	BatchSize       int // TXN_SYNC_BATCH_SIZE, default 100
	Concurrency     int // TXN_SYNC_LEAGUE_CONCURRENCY, default 8
}

// LeagueTransactionState carries the league ID, season, and leg cursor for one
// claimed league, as returned by ClaimLeaguesForTransactions.
type LeagueTransactionState struct {
	LeagueID       string
	Season         string
	LastLegFetched *int
}

type SyncLeagueTransactionsBatchParams struct {
	Leagues     []LeagueTransactionState
	Concurrency int
}

// SyncBatchResult summarizes one batch activity execution. Failed leagues keep
// their claim and re-enter the queue when it expires.
type SyncBatchResult struct {
	Processed int
	Failed    int
}

type FetchWeekStatsParams struct {
	Season string
	Week   int
}

type GetFinalizedWeeksParams struct {
	Season string
}

type ComputeSegmentSeasonADPParams struct {
	Segment models.ADPSegment
	Season  string
}

// ScavengerConfig is read from env by GetScavengerConfig so the dispatcher
// workflow (which cannot read env deterministically) can be tuned without a
// redeploy of workflow code.
type ScavengerConfig struct {
	LeagueBatchSize  int  // SCAVENGER_LEAGUE_BATCH_SIZE, default 500
	TxnBatchSize     int  // SCAVENGER_TXN_BATCH_SIZE, default 5000
	DraftBatchSize   int  // SCAVENGER_DRAFT_BATCH_SIZE, default 200 (drafts per batch; each draft's picks are copied alongside it)
	MaxBatchesPerRun int  // SCAVENGER_MAX_BATCHES_PER_RUN, default 50
	RetentionDays    int  // SCAVENGER_RETENTION_DAYS, default 30 — cloud rows older than this are purge candidates
	PurgeEnabled     bool // SCAVENGER_PURGE_ENABLED, default true — kill-switch; purge activities only run when true
}

// ReplicateBatchParams is shared by all four Replicate*Batch activities —
// they differ only in which stream/table they read and write.
type ReplicateBatchParams struct {
	BatchSize int
}

// ReplicateBatchResult reports one batch's outcome. Drained means fewer than
// BatchSize rows were found — the stream is caught up for this run.
type ReplicateBatchResult struct {
	Replicated int
	Drained    bool
}

// ScavengerReport summarizes one ScavengerDispatcher run.
type ScavengerReport struct {
	LeaguesReplicated      int
	TransactionsReplicated int
	DraftHeadersReplicated int
	DraftPicksReplicated   int
	TransactionsPurged     int
	TransactionsUnverified int
	DraftsPurged           int
	DraftsUnverified       int
}

// PurgeBatchParams is shared by PurgeTransactionsBatch and PurgeDraftsBatch —
// they differ only in which table(s) and verification rule they use.
type PurgeBatchParams struct {
	BatchSize     int
	RetentionDays int
}

// PurgeBatchResult reports one purge batch's outcome. Purged counts rows
// actually deleted from cloud. Unverified counts candidates left in place
// because they couldn't be confirmed present (and, for drafts, pick-count
// matched) in the archive yet — they are retried by the next batch/run.
// Drained means fewer than BatchSize purge candidates were found past the
// retention cutoff — this data type is caught up for this run.
type PurgeBatchResult struct {
	Purged     int
	Unverified int
	Drained    bool
}

// PlayerSyncResult reports how many players FetchAndUpsertAllPlayers upserted.
type PlayerSyncResult struct {
	PlayersUpserted int
}

// WeekStatsResult reports how many player rows FetchWeekStats upserted for one
// week, and whether Sleeper considers that week finalized.
type WeekStatsResult struct {
	PlayersUpserted int
	Finalized       bool
}

// ADPRollupResult reports how many player rows ComputeSegmentSeasonADP
// upserted for one (segment, season) pair.
type ADPRollupResult struct {
	PlayersUpserted int
}
