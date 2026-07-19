package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"go.temporal.io/sdk/activity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/helpers"
	"backend/internal/models"
	"backend/internal/sleeper"
)

// DataFetchActivities holds dependencies for per-league data fetching activities.
// Archive is nil unless ARCHIVE_DATABASE_URL is configured — every use of it
// is nil-checked, falling back to cloud-only (the pre-T13 behavior) when
// unset. Unlike ScavengerActivities/ADPRollupActivities, this struct's
// worker is never gated on archive availability: sync must keep working
// with or without one.
type DataFetchActivities struct {
	DB      *gorm.DB
	Archive *gorm.DB
	Sleeper *sleeper.Client
}

// archiveRoutingCutoff returns the age boundary for routing already-old data
// straight to archive at ingest time instead of cloud — see syncOneLeague
// and syncOneLeagueDrafts. Reuses SCAVENGER_RETENTION_DAYS (T6): "too old
// for cloud to keep" and "too old to bother writing to cloud in the first
// place" are the same threshold.
func archiveRoutingCutoff() time.Time {
	days := max(helpers.GetEnv("SCAVENGER_RETENTION_DAYS", 30), 1)
	return time.Now().UTC().AddDate(0, 0, -days)
}

// currentSleeperSeason anchors "current" the same way Seasons() does
// (discovery.go) — the calendar year, not NFL-state week boundaries.
func currentSleeperSeason() string {
	return strconv.Itoa(time.Now().Year())
}

// GetDraftSyncConfig returns the draft dispatcher tuning knobs from env,
// clamped to at least 1 so a bad value can't stall dispatch or break the
// claim query's LIMIT.
func (a *DataFetchActivities) GetDraftSyncConfig(ctx context.Context) (DraftSyncConfig, error) {
	return DraftSyncConfig{
		ParallelBatches: max(helpers.GetEnv("DRAFT_SYNC_PARALLEL_BATCHES", 2), 1),
		BatchSize:       max(helpers.GetEnv("DRAFT_SYNC_BATCH_SIZE", 100), 1),
		Concurrency:     max(helpers.GetEnv("DRAFT_SYNC_LEAGUE_CONCURRENCY", 8), 1),
	}, nil
}

// claimLeaguesForDraftsSQL atomically claims up to batchSize stale leagues for
// draft syncing (same pattern as transactions, on a separate claim column so
// the two sync paths never contend). Leagues whose drafting is finished
// (in_season/complete) and already fetched are excluded — completed drafts are
// immutable, so refetching them buys nothing; pre_draft and drafting leagues
// keep rechecking until their drafts complete. league_type = 'redraft'
// excludes keeper/dynasty leagues, which the valuation model never reads
// (analysis/src/db.py get_adp); last_fetched_at IS NOT NULL above guarantees
// league_type is already populated (set together in FetchLeagueDetails).
const claimLeaguesForDraftsSQL = `
UPDATE sleeper_leagues SET drafts_claimed_at = now()
WHERE sleeper_league_id IN (
    SELECT sleeper_league_id FROM sleeper_leagues
    WHERE skipped_at IS NULL AND last_fetched_at IS NOT NULL AND season >= '2025'
      AND league_type = 'redraft'
      AND NOT (status IN ('in_season', 'complete') AND last_drafts_fetched_at IS NOT NULL)
      AND (drafts_claimed_at IS NULL OR drafts_claimed_at < now() - interval '20 minutes')
    ORDER BY last_drafts_fetched_at ASC NULLS FIRST
    LIMIT ?
    FOR UPDATE SKIP LOCKED
)
RETURNING sleeper_league_id`

// ClaimLeaguesForDrafts claims up to BatchSize leagues with stale draft data.
// Postgres-only (SKIP LOCKED).
func (a *DataFetchActivities) ClaimLeaguesForDrafts(ctx context.Context, params ClaimLeaguesForDraftsParams) ([]string, error) {
	var ids []string
	if err := a.DB.WithContext(ctx).Raw(claimLeaguesForDraftsSQL, params.BatchSize).Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// SyncLeagueDraftsBatch syncs drafts and picks for a claimed batch of leagues
// with bounded concurrency, stamping each league done as it completes.
// Per-league failures are counted, not propagated: a failed league keeps its
// claim and re-enters the queue when the claim expires. The activity
// heartbeats as leagues complete so a dead worker is detected via
// HeartbeatTimeout.
func (a *DataFetchActivities) SyncLeagueDraftsBatch(ctx context.Context, params SyncLeagueDraftsBatchParams) (SyncBatchResult, error) {
	logger := activity.GetLogger(ctx)
	res := SyncBatchResult{}

	// Re-scope to leagues still claimed: on an activity retry, leagues stamped
	// by the previous attempt have drafts_claimed_at cleared and must not re-sync.
	var stillClaimed []string
	if err := a.DB.WithContext(ctx).Model(&models.SleeperLeague{}).
		Where("sleeper_league_id IN ? AND drafts_claimed_at IS NOT NULL", params.LeagueIDs).
		Pluck("sleeper_league_id", &stillClaimed).Error; err != nil {
		return res, err
	}
	if len(stillClaimed) == 0 {
		return res, nil
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
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			results <- leagueResult{leagueID: id, err: a.syncOneLeagueDrafts(ctx, id)}
		})
	}
	go func() { wg.Wait(); close(results) }()

	done := 0
	for r := range results {
		done++
		if r.err != nil {
			res.Failed++
			logger.Warn("league draft sync failed", "leagueID", r.leagueID, "error", r.err)
		} else {
			res.Processed++
		}
		if done%10 == 0 {
			activity.RecordHeartbeat(ctx, done)
		}
	}
	return res, nil
}

// syncOneLeagueDrafts upserts a league's drafts, fetches picks for completed
// drafts that haven't been picked up yet (completed drafts are immutable, so
// picks are fetch-once), and stamps completion (clearing the claim) in one
// update. A 404 on the league marks it skipped; a 404 on a draft's picks
// skips that draft. Each draft routes as a whole unit (header + picks) to
// archive whenever Archive is configured — draft data is immutable and has
// no live-API reader, so there's no reason for any of it to ever land in
// cloud. Falls back to cloud-only when no archive DB is configured.
func (a *DataFetchActivities) syncOneLeagueDrafts(ctx context.Context, leagueID string) error {
	drafts, err := a.Sleeper.GetLeagueDrafts(ctx, leagueID)
	if err != nil {
		var nfe *sleeper.NotFoundError
		if errors.As(err, &nfe) {
			return a.DB.WithContext(ctx).
				Model(&models.SleeperLeague{}).
				Where("sleeper_league_id = ?", leagueID).
				Updates(map[string]interface{}{
					"skipped_at":        time.Now().UTC(),
					"drafts_claimed_at": nil,
				}).Error
		}
		return err
	}

	var cloudCompletedIDs, archiveCompletedIDs []string
	for _, d := range drafts {
		if a.Archive != nil {
			if err := a.upsertArchiveDraftHeader(ctx, d, leagueID); err != nil {
				return err
			}
			if d.Status == "complete" {
				archiveCompletedIDs = append(archiveCompletedIDs, d.DraftID)
			}
			continue
		}
		row := models.SleeperDraft{
			SleeperDraftID:  d.DraftID,
			SleeperLeagueID: leagueID,
			Type:            d.Type,
			Status:          d.Status,
			Season:          d.Season,
		}
		if err := a.DB.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "sleeper_draft_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"status", "type", "season"}),
		}).Create(&row).Error; err != nil {
			return err
		}
		if d.Status == "complete" {
			cloudCompletedIDs = append(cloudCompletedIDs, d.DraftID)
		}
	}

	if len(cloudCompletedIDs) > 0 {
		var pending []string
		if err := a.DB.WithContext(ctx).Model(&models.SleeperDraft{}).
			Where("sleeper_draft_id IN ? AND last_fetched_at IS NULL", cloudCompletedIDs).
			Pluck("sleeper_draft_id", &pending).Error; err != nil {
			return err
		}
		for _, draftID := range pending {
			if err := a.fetchDraftPicks(ctx, draftID); err != nil {
				var nfe *sleeper.NotFoundError
				if errors.As(err, &nfe) {
					continue // draft gone on Sleeper's side; nothing to fetch
				}
				return fmt.Errorf("draft %s: %w", draftID, err)
			}
		}
	}

	if len(archiveCompletedIDs) > 0 {
		var pending []string
		if err := a.Archive.WithContext(ctx).Model(&models.ArchiveSleeperDraft{}).
			Where("sleeper_draft_id IN ? AND last_fetched_at IS NULL", archiveCompletedIDs).
			Pluck("sleeper_draft_id", &pending).Error; err != nil {
			return err
		}
		for _, draftID := range pending {
			if err := a.fetchArchiveDraftPicks(ctx, draftID); err != nil {
				var nfe *sleeper.NotFoundError
				if errors.As(err, &nfe) {
					continue
				}
				return fmt.Errorf("draft %s (archive): %w", draftID, err)
			}
		}
	}

	return a.DB.WithContext(ctx).
		Model(&models.SleeperLeague{}).
		Where("sleeper_league_id = ?", leagueID).
		Updates(map[string]interface{}{
			"last_drafts_fetched_at": time.Now().UTC(),
			"drafts_claimed_at":      nil,
		}).Error
}

// fetchDraftPicks fetches and upserts all picks for draftID, then stamps the
// draft's last_fetched_at so it is never refetched.
func (a *DataFetchActivities) fetchDraftPicks(ctx context.Context, draftID string) error {
	picks, err := a.Sleeper.GetDraftPicks(ctx, draftID)
	if err != nil {
		return err
	}
	if len(picks) > 0 {
		rows := make([]models.SleeperDraftPick, len(picks))
		for i, p := range picks {
			metadata, _ := json.Marshal(p.Metadata)
			rows[i] = models.SleeperDraftPick{
				SleeperDraftID:  draftID,
				Round:           p.Round,
				PickNo:          p.PickNo,
				RosterID:        p.RosterID,
				PickedByUserID:  p.PickedBy,
				SleeperPlayerID: p.PlayerID,
				Metadata:        metadata,
			}
		}
		if err := a.DB.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(rows, 500).Error; err != nil {
			return err
		}
	}
	return a.DB.WithContext(ctx).
		Model(&models.SleeperDraft{}).
		Where("sleeper_draft_id = ?", draftID).
		Update("last_fetched_at", time.Now().UTC()).Error
}

// upsertArchiveDraftHeader upserts d directly into the archive DB, skipping
// cloud — see syncOneLeagueDrafts's age-based routing.
func (a *DataFetchActivities) upsertArchiveDraftHeader(ctx context.Context, d sleeper.Draft, leagueID string) error {
	now := time.Now().UTC()
	row := models.ArchiveSleeperDraft{
		SleeperDraftID:  d.DraftID,
		SleeperLeagueID: leagueID,
		Type:            d.Type,
		Status:          d.Status,
		Season:          d.Season,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	return a.Archive.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "sleeper_draft_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "type", "season", "updated_at"}),
	}).Create(&row).Error
}

// fetchArchiveDraftPicks mirrors fetchDraftPicks but writes directly to the
// archive DB for an old (archive-routed) draft — see syncOneLeagueDrafts.
func (a *DataFetchActivities) fetchArchiveDraftPicks(ctx context.Context, draftID string) error {
	picks, err := a.Sleeper.GetDraftPicks(ctx, draftID)
	if err != nil {
		return err
	}
	if len(picks) > 0 {
		rows := make([]models.ArchiveSleeperDraftPick, len(picks))
		for i, p := range picks {
			metadata, _ := json.Marshal(p.Metadata)
			rows[i] = models.ArchiveSleeperDraftPick{
				SleeperDraftID:  draftID,
				Round:           p.Round,
				PickNo:          p.PickNo,
				RosterID:        p.RosterID,
				PickedByUserID:  p.PickedBy,
				SleeperPlayerID: p.PlayerID,
				Metadata:        metadata,
			}
		}
		if err := a.Archive.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(rows, 500).Error; err != nil {
			return err
		}
	}
	return a.Archive.WithContext(ctx).
		Model(&models.ArchiveSleeperDraft{}).
		Where("sleeper_draft_id = ?", draftID).
		Update("last_fetched_at", time.Now().UTC()).Error
}

// GetTransactionSyncConfig returns the dispatcher tuning knobs from env,
// clamped to at least 1 so a bad value can't stall dispatch or break the
// claim query's LIMIT.
func (a *DataFetchActivities) GetTransactionSyncConfig(ctx context.Context) (TransactionSyncConfig, error) {
	return TransactionSyncConfig{
		ParallelBatches: max(helpers.GetEnv("TXN_SYNC_PARALLEL_BATCHES", 2), 1),
		BatchSize:       max(helpers.GetEnv("TXN_SYNC_BATCH_SIZE", 100), 1),
		Concurrency:     max(helpers.GetEnv("TXN_SYNC_LEAGUE_CONCURRENCY", 8), 1),
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
		cloudRows := rows
		if a.Archive != nil {
			cutoff := archiveRoutingCutoff()
			var newRows, oldRows []models.SleeperTransaction
			for _, r := range rows {
				if time.UnixMilli(r.CreatedAtSleeper).UTC().Before(cutoff) {
					// Old rows route straight to archive; skipping here means the
					// row is dropped entirely, not sent to cloud instead.
					if isPlayerOnlyTransaction(r.DraftPicks, r.WaiverBudget) {
						oldRows = append(oldRows, r)
					}
				} else {
					newRows = append(newRows, r)
				}
			}
			if len(oldRows) > 0 {
				if err := a.upsertArchiveTransactions(ctx, oldRows); err != nil {
					return fmt.Errorf("leg %d archive upsert: %w", leg, err)
				}
			}
			cloudRows = newRows
		}
		if len(cloudRows) > 0 {
			if err := a.DB.WithContext(ctx).
				Clauses(clause.OnConflict{DoNothing: true}).
				CreateInBatches(cloudRows, 500).Error; err != nil {
				return fmt.Errorf("leg %d upsert: %w", leg, err)
			}
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

// isPlayerOnlyTransaction mirrors the valuation model's query-time filter
// (analysis/src/parsing.py parse_trade) at write time, so this data never
// reaches the archive DB at all.
func isPlayerOnlyTransaction(draftPicks, waiverBudget json.RawMessage) bool {
	return isEmptyJSONArray(draftPicks) && isEmptyJSONArray(waiverBudget)
}

func isEmptyJSONArray(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true // SQL NULL / zero value
	}
	var v []json.RawMessage
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	return len(v) == 0
}

// upsertArchiveTransactions writes rows directly to the archive DB, skipping
// cloud — see syncOneLeague's age-based routing.
func (a *DataFetchActivities) upsertArchiveTransactions(ctx context.Context, rows []models.SleeperTransaction) error {
	archiveRows := make([]models.ArchiveSleeperTransaction, len(rows))
	for i, r := range rows {
		archiveRows[i] = models.ArchiveSleeperTransaction{
			SleeperTransactionID: r.SleeperTransactionID, SleeperLeagueID: r.SleeperLeagueID,
			Type: r.Type, Status: r.Status, CreatedAtSleeper: r.CreatedAtSleeper, Leg: r.Leg,
			Adds: r.Adds, Drops: r.Drops, DraftPicks: r.DraftPicks, WaiverBudget: r.WaiverBudget,
			CreatedAt: time.Now().UTC(),
		}
	}
	return a.Archive.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(archiveRows, 500).Error
}
