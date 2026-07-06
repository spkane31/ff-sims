package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/helpers"
	"backend/internal/models"
	"backend/internal/sleeper"
)

// DataFetchActivities holds dependencies for per-league data fetching activities.
type DataFetchActivities struct {
	DB      *gorm.DB
	Sleeper *sleeper.Client
}

// FetchLeagueDrafts fetches all drafts for leagueID, upserts them, and returns the IDs
// of completed drafts (status="complete") that are ready for pick fetching.
func (a *DataFetchActivities) FetchLeagueDrafts(ctx context.Context, params FetchLeagueDraftsParams) ([]string, error) {
	drafts, err := a.Sleeper.GetLeagueDrafts(ctx, params.LeagueID)
	if err != nil {
		var nfe *sleeper.NotFoundError
		if errors.As(err, &nfe) {
			return nil, temporal.NewNonRetryableApplicationError(
				"league not found: "+params.LeagueID, "NOT_FOUND", err,
			)
		}
		return nil, err
	}
	var completedIDs []string
	for _, d := range drafts {
		row := models.SleeperDraft{
			SleeperDraftID:  d.DraftID,
			SleeperLeagueID: params.LeagueID,
			Type:            d.Type,
			Status:          d.Status,
			Season:          d.Season,
		}
		if err := a.DB.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "sleeper_draft_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"status", "type", "season"}),
		}).Create(&row).Error; err != nil {
			return nil, err
		}
		if d.Status == "complete" {
			completedIDs = append(completedIDs, d.DraftID)
		}
	}
	return completedIDs, nil
}

// FetchDraftPicks fetches all picks for draftID and upserts them (immutable once complete).
func (a *DataFetchActivities) FetchDraftPicks(ctx context.Context, params FetchDraftPicksParams) error {
	picks, err := a.Sleeper.GetDraftPicks(ctx, params.DraftID)
	if err != nil {
		var nfe *sleeper.NotFoundError
		if errors.As(err, &nfe) {
			return temporal.NewNonRetryableApplicationError(
				"draft not found: "+params.DraftID, "NOT_FOUND", err,
			)
		}
		return err
	}
	for _, p := range picks {
		metadata, _ := json.Marshal(p.Metadata)
		row := models.SleeperDraftPick{
			SleeperDraftID:  params.DraftID,
			Round:           p.Round,
			PickNo:          p.PickNo,
			RosterID:        p.RosterID,
			PickedByUserID:  p.PickedBy,
			SleeperPlayerID: p.PlayerID,
			Metadata:        metadata,
		}
		if err := a.DB.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(&row).Error; err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	return a.DB.WithContext(ctx).
		Model(&models.SleeperDraft{}).
		Where("sleeper_draft_id = ?", params.DraftID).
		Update("last_fetched_at", now).Error
}

// GetStaleLeaguesForDrafts returns up to batchSize league IDs that have had their
// details fetched (last_fetched_at IS NOT NULL) but whose drafts are stale,
// ordered NULL first then oldest.
func (a *DataFetchActivities) GetStaleLeaguesForDrafts(ctx context.Context, params GetStaleLeaguesParams) ([]string, error) {
	var leagues []models.SleeperLeague
	err := a.DB.WithContext(ctx).
		Where("skipped_at IS NULL AND last_fetched_at IS NOT NULL AND season >= '2025'").
		Order("CASE WHEN last_drafts_fetched_at IS NULL THEN 0 ELSE 1 END, last_drafts_fetched_at ASC").
		Limit(params.BatchSize).
		Find(&leagues).Error
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(leagues))
	for i, l := range leagues {
		ids[i] = l.SleeperLeagueID
	}
	return ids, nil
}

// GetTransactionSyncConfig returns the dispatcher tuning knobs from env,
// clamped to at least 1 so a bad value can't stall dispatch or break the
// claim query's LIMIT.
func (a *DataFetchActivities) GetTransactionSyncConfig(ctx context.Context) (TransactionSyncConfig, error) {
	return TransactionSyncConfig{
		ParallelBatches: max(helpers.GetEnv("TXN_SYNC_PARALLEL_BATCHES", 4), 1),
		BatchSize:       max(helpers.GetEnv("TXN_SYNC_BATCH_SIZE", 250), 1),
		Concurrency:     max(helpers.GetEnv("TXN_SYNC_LEAGUE_CONCURRENCY", 12), 1),
	}, nil
}

// claimLeaguesForTransactionsSQL atomically claims up to batchSize stale
// leagues for transaction syncing. FOR UPDATE SKIP LOCKED lets concurrent
// claimers (two fleets, K parallel pipelines) partition the backlog without
// blocking or double-claiming; the 20-minute expiry window re-queues leagues
// claimed by a worker that died mid-batch. Ordering matches the partial index
// idx_sleeper_leagues_txn_stale (never-fetched first, then oldest).
const claimLeaguesForTransactionsSQL = `
UPDATE sleeper_leagues SET claimed_at = now()
WHERE sleeper_league_id IN (
    SELECT sleeper_league_id FROM sleeper_leagues
    WHERE skipped_at IS NULL AND last_fetched_at IS NOT NULL AND season >= '2025'
      AND NOT (status = 'complete' AND last_transactions_fetched_at IS NOT NULL)
      AND (claimed_at IS NULL OR claimed_at < now() - interval '20 minutes')
    ORDER BY last_transactions_fetched_at ASC NULLS FIRST
    LIMIT ?
    FOR UPDATE SKIP LOCKED
)
RETURNING sleeper_league_id, season, last_transaction_leg_fetched`

// ClaimLeaguesForTransactions claims up to BatchSize leagues with stale
// transaction data and returns their sync state. Postgres-only (SKIP LOCKED).
func (a *DataFetchActivities) ClaimLeaguesForTransactions(ctx context.Context, params ClaimLeaguesForTransactionsParams) ([]LeagueTransactionState, error) {
	var rows []struct {
		SleeperLeagueID           string
		Season                    string
		LastTransactionLegFetched *int
	}
	if err := a.DB.WithContext(ctx).Raw(claimLeaguesForTransactionsSQL, params.BatchSize).Scan(&rows).Error; err != nil {
		return nil, err
	}
	states := make([]LeagueTransactionState, len(rows))
	for i, r := range rows {
		states[i] = LeagueTransactionState{
			LeagueID:       r.SleeperLeagueID,
			Season:         r.Season,
			LastLegFetched: r.LastTransactionLegFetched,
		}
	}
	return states, nil
}

// maxLegForLeague returns the highest transaction leg worth fetching. Past
// seasons get the full 1..18 sweep; the current season is capped at the
// current NFL week (offseason week 0 still fetches leg 1, where offseason
// moves land). A nil state (state endpoint down) falls back to 18 rather than
// stalling the batch.
func maxLegForLeague(season string, state *sleeper.NFLState) int {
	if state == nil || season < state.Season {
		return 18
	}
	if state.Week < 1 {
		return 1
	}
	return min(state.Week, 18)
}

// SyncLeagueTransactionsBatch syncs transactions for a claimed batch of
// leagues with bounded concurrency, stamping each league done as it completes.
// Per-league failures are counted, not propagated: a failed league keeps its
// claim and re-enters the queue when the claim expires. The activity heartbeats
// as leagues complete so a dead worker is detected via HeartbeatTimeout.
func (a *DataFetchActivities) SyncLeagueTransactionsBatch(ctx context.Context, params SyncLeagueTransactionsBatchParams) (SyncBatchResult, error) {
	logger := activity.GetLogger(ctx)
	res := SyncBatchResult{}

	// Re-scope to leagues still claimed: on an activity retry, leagues stamped
	// by the previous attempt have claimed_at cleared and must not re-sync.
	ids := make([]string, len(params.Leagues))
	byID := make(map[string]LeagueTransactionState, len(params.Leagues))
	for i, lg := range params.Leagues {
		ids[i] = lg.LeagueID
		byID[lg.LeagueID] = lg
	}
	var stillClaimed []string
	if err := a.DB.WithContext(ctx).Model(&models.SleeperLeague{}).
		Where("sleeper_league_id IN ? AND claimed_at IS NOT NULL", ids).
		Pluck("sleeper_league_id", &stillClaimed).Error; err != nil {
		return res, err
	}
	if len(stillClaimed) == 0 {
		return res, nil
	}

	state, err := a.Sleeper.GetNFLState(ctx)
	if err != nil {
		logger.Warn("GetNFLState failed; falling back to full 18-leg sweep", "error", err)
		state = nil
	}

	concurrency := max(1, params.Concurrency)

	type leagueResult struct {
		leagueID string
		err      error
	}
	sem := make(chan struct{}, concurrency)
	results := make(chan leagueResult, len(stillClaimed))
	var wg sync.WaitGroup
	for _, id := range stillClaimed {
		lg := byID[id]
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			results <- leagueResult{leagueID: lg.LeagueID, err: a.syncOneLeague(ctx, lg, maxLegForLeague(lg.Season, state))}
		})
	}
	go func() { wg.Wait(); close(results) }()

	done := 0
	for r := range results {
		done++
		if r.err != nil {
			res.Failed++
			logger.Warn("league transaction sync failed", "leagueID", r.leagueID, "error", r.err)
		} else {
			res.Processed++
		}
		if done%10 == 0 {
			activity.RecordHeartbeat(ctx, done)
		}
	}
	return res, nil
}

// syncOneLeague fetches transactions for one league from its leg cursor up to
// maxLeg, upserts them, and stamps completion (clearing the claim) in a single
// update. Per-leg 404s mean "no transactions for that leg" and are skipped.
func (a *DataFetchActivities) syncOneLeague(ctx context.Context, lg LeagueTransactionState, maxLeg int) error {
	startLeg := 1
	if lg.LastLegFetched != nil && *lg.LastLegFetched > 1 {
		startLeg = *lg.LastLegFetched - 1
	}

	maxSeen := 0
	for leg := startLeg; leg <= maxLeg; leg++ {
		txns, err := a.Sleeper.GetTransactions(ctx, lg.LeagueID, leg)
		if err != nil {
			var nfe *sleeper.NotFoundError
			if errors.As(err, &nfe) {
				continue
			}
			return fmt.Errorf("leg %d: %w", leg, err)
		}
		if len(txns) == 0 {
			continue
		}
		rows := make([]models.SleeperTransaction, len(txns))
		for i, t := range txns {
			addsJSON, _ := json.Marshal(t.Adds)
			dropsJSON, _ := json.Marshal(t.Drops)
			picksJSON, _ := json.Marshal(t.DraftPicks)
			waiverJSON, _ := json.Marshal(t.WaiverBudget)
			rows[i] = models.SleeperTransaction{
				SleeperTransactionID: t.TransactionID,
				SleeperLeagueID:      lg.LeagueID,
				Type:                 t.Type,
				Status:               t.Status,
				CreatedAtSleeper:     t.Created,
				Leg:                  t.Leg,
				Adds:                 addsJSON,
				Drops:                dropsJSON,
				DraftPicks:           picksJSON,
				WaiverBudget:         waiverJSON,
			}
		}
		if err := a.DB.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(rows, 500).Error; err != nil {
			return fmt.Errorf("leg %d upsert: %w", leg, err)
		}
		if leg > maxSeen {
			maxSeen = leg
		}
	}

	updates := map[string]interface{}{
		"last_transactions_fetched_at": time.Now().UTC(),
		"claimed_at":                   nil,
	}
	if maxSeen > 0 {
		updates["last_transaction_leg_fetched"] = maxSeen
	}
	return a.DB.WithContext(ctx).
		Model(&models.SleeperLeague{}).
		Where("sleeper_league_id = ?", lg.LeagueID).
		Updates(updates).Error
}

// MarkLeagueDraftsFetched sets last_drafts_fetched_at=now() on the given league.
func (a *DataFetchActivities) MarkLeagueDraftsFetched(ctx context.Context, params MarkLeagueFetchedParams) error {
	now := time.Now().UTC()
	return a.DB.WithContext(ctx).
		Model(&models.SleeperLeague{}).
		Where("sleeper_league_id = ?", params.LeagueID).
		Update("last_drafts_fetched_at", now).Error
}

// MarkLeagueSkipped sets skipped_at=now() so the league is excluded from future batches.
func (a *DataFetchActivities) MarkLeagueSkipped(ctx context.Context, params MarkLeagueSkippedParams) error {
	now := time.Now().UTC()
	return a.DB.WithContext(ctx).
		Model(&models.SleeperLeague{}).
		Where("sleeper_league_id = ?", params.LeagueID).
		Update("skipped_at", now).Error
}
