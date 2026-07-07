package activities

import "backend/internal/models"

type GetStaleUsersParams struct {
	BatchSize int
}

type FetchUserLeaguesParams struct {
	UserID string
}

type FetchLeagueMembersParams struct {
	LeagueID string
}

type MarkUserFetchedParams struct {
	UserID string
}

type MarkUserSkippedParams struct {
	UserID string
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
	ParallelBatches int // DRAFT_SYNC_PARALLEL_BATCHES, default 4
	BatchSize       int // DRAFT_SYNC_BATCH_SIZE, default 250
	Concurrency     int // DRAFT_SYNC_LEAGUE_CONCURRENCY, default 12
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
	ParallelBatches int // TXN_SYNC_PARALLEL_BATCHES, default 4
	BatchSize       int // TXN_SYNC_BATCH_SIZE, default 250
	Concurrency     int // TXN_SYNC_LEAGUE_CONCURRENCY, default 12
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
