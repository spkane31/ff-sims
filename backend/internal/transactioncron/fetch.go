package transactioncron

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/activities"
	"backend/internal/helpers"
	"backend/internal/models"
	"backend/internal/sleeper"
)

type ClaimLeaguesForTransactionsParams struct {
	BatchSize int
}

// LeagueTransactionState carries the league ID, season, and leg cursor for one
// claimed league, as returned by ClaimLeaguesForTransactions.
type LeagueTransactionState struct {
	LeagueID       string
	Season         string
	LastLegFetched *int
}

// LeagueTransactionFetchResult is FetchLeagueTransactions's result for one
// claimed league. Rows are split cloud/archive at fetch time (age-based
// routing); FlushLeagueTransactions is the batch-write counterpart that
// persists them.
type LeagueTransactionFetchResult struct {
	LeagueID    string
	CloudRows   []models.SleeperTransaction
	ArchiveRows []models.SleeperTransaction
	MaxLegSeen  int // 0 if no new legs were found this run
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
func ClaimLeaguesForTransactions(ctx context.Context, db *gorm.DB, params ClaimLeaguesForTransactionsParams) ([]LeagueTransactionState, error) {
	var rows []struct {
		SleeperLeagueID           string
		Season                    string
		LastTransactionLegFetched *int
	}
	if err := db.WithContext(ctx).Raw(claimLeaguesForTransactionsSQL, params.BatchSize).Scan(&rows).Error; err != nil {
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

// MaxLegForLeague returns the highest transaction leg worth fetching. Past
// seasons get the full 1..18 sweep; the current season is capped at the
// current NFL week (offseason week 0 still fetches leg 1, where offseason
// moves land). A nil state (state endpoint down) falls back to 18 rather than
// stalling the batch.
func MaxLegForLeague(season string, state *sleeper.NFLState) int {
	if state == nil || season < state.Season {
		return 18
	}
	if state.Week < 1 {
		return 1
	}
	return min(state.Week, 18)
}

// archiveRoutingCutoff returns the age boundary for routing already-old
// transactions straight to archive at ingest time instead of cloud. Reuses
// SCAVENGER_RETENTION_DAYS: "too old for cloud to keep" and "too old to
// bother writing to cloud in the first place" are the same threshold.
func archiveRoutingCutoff() time.Time {
	days := max(helpers.GetEnv("SCAVENGER_RETENTION_DAYS", 30), 1)
	return time.Now().UTC().AddDate(0, 0, -days)
}

// FetchLeagueTransactions walks lg's leg cursor up to maxLeg, splitting each
// leg's rows into cloud-bound and archive-bound sets by age, and returns them
// without writing anything — FlushLeagueTransactions is the batch-write
// counterpart, called once per accumulated batch of these results.
func FetchLeagueTransactions(ctx context.Context, dfa *activities.DataFetchActivities, lg LeagueTransactionState, maxLeg int) (LeagueTransactionFetchResult, error) {
	startLeg := 1
	if lg.LastLegFetched != nil && *lg.LastLegFetched > 1 {
		startLeg = *lg.LastLegFetched - 1
	}

	res := LeagueTransactionFetchResult{LeagueID: lg.LeagueID}
	for leg := startLeg; leg <= maxLeg; leg++ {
		txns, err := dfa.Sleeper.GetTransactions(ctx, lg.LeagueID, leg)
		if err != nil {
			var nfe *sleeper.NotFoundError
			if errors.As(err, &nfe) {
				continue
			}
			return LeagueTransactionFetchResult{}, fmt.Errorf("leg %d: %w", leg, err)
		}
		if len(txns) == 0 {
			continue
		}
		var rows []models.SleeperTransaction
		for _, t := range txns {
			addsJSON, _ := json.Marshal(t.Adds)
			dropsJSON, _ := json.Marshal(t.Drops)
			picksJSON, _ := json.Marshal(t.DraftPicks)
			waiverJSON, _ := json.Marshal(t.WaiverBudget)
			// Picks/FAAB trades are never valued by the valuation model (see
			// activities.IsPlayerOnlyTransaction) and aren't useful trade
			// history either, so they're dropped at ingest time rather than
			// written anywhere.
			if !activities.IsPlayerOnlyTransaction(picksJSON, waiverJSON) {
				continue
			}
			rows = append(rows, models.SleeperTransaction{
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
			})
		}
		if dfa.Archive != nil {
			cutoff := archiveRoutingCutoff()
			for _, r := range rows {
				if time.UnixMilli(r.CreatedAtSleeper).UTC().Before(cutoff) {
					// Old rows route straight to archive, not cloud. Filtering
					// out picks/FAAB rows already happened unconditionally
					// above (before the cloud/archive split), so no re-check
					// here.
					res.ArchiveRows = append(res.ArchiveRows, r)
				} else {
					res.CloudRows = append(res.CloudRows, r)
				}
			}
		} else {
			res.CloudRows = append(res.CloudRows, rows...)
		}
		if leg > res.MaxLegSeen {
			res.MaxLegSeen = leg
		}
	}
	return res, nil
}

// FlushLeagueTransactions is the batch-write counterpart to
// FetchLeagueTransactions: one bulk insert for all of the batch's cloud rows
// (against tx, the transaction fdb.RunPool already opened for this batch),
// one for archive rows (against dfa.Archive directly — a second, distinct
// database fdb's transaction doesn't span; skipped when there are none), one
// bulk claim-clearing update covering every league in the batch, then a
// per-league leg-cursor update only where MaxLegSeen > 0 (that value
// genuinely varies per league, unlike the claim-clear). The archive write's
// error is not swallowed: if it fails, this whole batch's flush fails, so
// fdb rolls tx back and drops the batch for retry rather than committing a
// claim-clear whose archive copy never landed.
func FlushLeagueTransactions(ctx context.Context, dfa *activities.DataFetchActivities, tx *gorm.DB, batch []LeagueTransactionFetchResult) error {
	var cloudRows, archiveRows []models.SleeperTransaction
	leagueIDs := make([]string, len(batch))
	for i, r := range batch {
		leagueIDs[i] = r.LeagueID
		cloudRows = append(cloudRows, r.CloudRows...)
		archiveRows = append(archiveRows, r.ArchiveRows...)
	}

	if len(cloudRows) > 0 {
		if err := tx.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(cloudRows, 500).Error; err != nil {
			return fmt.Errorf("cloud upsert: %w", err)
		}
	}
	if dfa.Archive != nil && len(archiveRows) > 0 {
		if err := upsertArchiveTransactions(ctx, dfa.Archive, archiveRows); err != nil {
			return fmt.Errorf("archive upsert: %w", err)
		}
	}

	if err := tx.WithContext(ctx).
		Model(&models.SleeperLeague{}).
		Where("sleeper_league_id IN ?", leagueIDs).
		Updates(map[string]interface{}{
			"last_transactions_fetched_at": time.Now().UTC(),
			"claimed_at":                   nil,
		}).Error; err != nil {
		return fmt.Errorf("claim-clear update: %w", err)
	}

	for _, r := range batch {
		if r.MaxLegSeen == 0 {
			continue
		}
		if err := tx.WithContext(ctx).
			Model(&models.SleeperLeague{}).
			Where("sleeper_league_id = ?", r.LeagueID).
			Update("last_transaction_leg_fetched", r.MaxLegSeen).Error; err != nil {
			return fmt.Errorf("leg cursor update for %s: %w", r.LeagueID, err)
		}
	}
	return nil
}

// upsertArchiveTransactions writes rows directly to the archive DB, skipping
// cloud — see FetchLeagueTransactions's age-based routing.
func upsertArchiveTransactions(ctx context.Context, archive *gorm.DB, rows []models.SleeperTransaction) error {
	archiveRows := make([]models.ArchiveSleeperTransaction, len(rows))
	for i, r := range rows {
		archiveRows[i] = models.ArchiveSleeperTransaction{
			SleeperTransactionID: r.SleeperTransactionID, SleeperLeagueID: r.SleeperLeagueID,
			Type: r.Type, Status: r.Status, CreatedAtSleeper: r.CreatedAtSleeper, Leg: r.Leg,
			Adds: r.Adds, Drops: r.Drops, DraftPicks: r.DraftPicks, WaiverBudget: r.WaiverBudget,
			CreatedAt: time.Now().UTC(),
		}
	}
	return archive.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(archiveRows, 500).Error
}
