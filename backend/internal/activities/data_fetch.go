package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

// FetchLeagueTransactions fetches transactions for leagueID starting from the leg cursor.
// When LastLegFetched is set it starts from max(LastLegFetched-1, 1) to catch late-processed
// transactions on the previous leg. 404 responses for a leg are treated as empty and skipped.
// Returns the highest leg number for which any transactions were received (0 if none).
func (a *DataFetchActivities) FetchLeagueTransactions(ctx context.Context, params FetchLeagueTransactionsParams) (int, error) {
	startLeg := 1
	if params.LastLegFetched != nil && *params.LastLegFetched > 1 {
		startLeg = *params.LastLegFetched - 1
	}

	maxLeg := 0
	for leg := startLeg; leg <= 18; leg++ {
		txns, err := a.Sleeper.GetTransactions(ctx, params.LeagueID, leg)
		if err != nil {
			var nfe *sleeper.NotFoundError
			if errors.As(err, &nfe) {
				continue // no transactions for this leg is normal
			}
			return 0, fmt.Errorf("leg %d: %w", leg, err)
		}
		for _, t := range txns {
			addsJSON, _ := json.Marshal(t.Adds)
			dropsJSON, _ := json.Marshal(t.Drops)
			picksJSON, _ := json.Marshal(t.DraftPicks)
			waiverJSON, _ := json.Marshal(t.WaiverBudget)
			row := models.SleeperTransaction{
				SleeperTransactionID: t.TransactionID,
				SleeperLeagueID:      params.LeagueID,
				Type:                 t.Type,
				Status:               t.Status,
				CreatedAtSleeper:     t.Created,
				Leg:                  t.Leg,
				Adds:                 addsJSON,
				Drops:                dropsJSON,
				DraftPicks:           picksJSON,
				WaiverBudget:         waiverJSON,
			}
			if err := a.DB.WithContext(ctx).
				Clauses(clause.OnConflict{DoNothing: true}).
				Create(&row).Error; err != nil {
				return 0, err
			}
		}
		if len(txns) > 0 && leg > maxLeg {
			maxLeg = leg
		}
	}
	return maxLeg, nil
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

// GetStaleLeaguesForTransactions returns up to batchSize leagues that have had their
// details fetched (last_fetched_at IS NOT NULL) but whose transactions are stale,
// ordered NULL first then oldest. Completed leagues that have already been synced are excluded.
func (a *DataFetchActivities) GetStaleLeaguesForTransactions(ctx context.Context, params GetStaleLeaguesParams) ([]LeagueTransactionState, error) {
	var leagues []models.SleeperLeague
	err := a.DB.WithContext(ctx).
		Where("skipped_at IS NULL AND last_fetched_at IS NOT NULL AND season >= '2025' AND NOT (status = 'complete' AND last_transactions_fetched_at IS NOT NULL)").
		Order("CASE WHEN last_transactions_fetched_at IS NULL THEN 0 ELSE 1 END, last_transactions_fetched_at ASC").
		Limit(params.BatchSize).
		Find(&leagues).Error
	if err != nil {
		return nil, err
	}
	states := make([]LeagueTransactionState, len(leagues))
	for i, l := range leagues {
		states[i] = LeagueTransactionState{
			LeagueID:       l.SleeperLeagueID,
			LastLegFetched: l.LastTransactionLegFetched,
		}
	}
	return states, nil
}

// MarkLeagueDraftsFetched sets last_drafts_fetched_at=now() on the given league.
func (a *DataFetchActivities) MarkLeagueDraftsFetched(ctx context.Context, params MarkLeagueFetchedParams) error {
	now := time.Now().UTC()
	return a.DB.WithContext(ctx).
		Model(&models.SleeperLeague{}).
		Where("sleeper_league_id = ?", params.LeagueID).
		Update("last_drafts_fetched_at", now).Error
}

// MarkLeagueTransactionsFetched sets last_transactions_fetched_at=now() on the given league.
// If MaxLeg > 0 it also advances last_transaction_leg_fetched to the highest leg seen.
func (a *DataFetchActivities) MarkLeagueTransactionsFetched(ctx context.Context, params MarkLeagueTransactionsFetchedParams) error {
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"last_transactions_fetched_at": now,
	}
	if params.MaxLeg > 0 {
		updates["last_transaction_leg_fetched"] = params.MaxLeg
	}
	return a.DB.WithContext(ctx).
		Model(&models.SleeperLeague{}).
		Where("sleeper_league_id = ?", params.LeagueID).
		Updates(updates).Error
}

// MarkLeagueSkipped sets skipped_at=now() so the league is excluded from future batches.
func (a *DataFetchActivities) MarkLeagueSkipped(ctx context.Context, params MarkLeagueSkippedParams) error {
	now := time.Now().UTC()
	return a.DB.WithContext(ctx).
		Model(&models.SleeperLeague{}).
		Where("sleeper_league_id = ?", params.LeagueID).
		Update("skipped_at", now).Error
}
