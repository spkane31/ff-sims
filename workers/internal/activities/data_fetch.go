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

	"workers/internal/models"
	"workers/internal/sleeper"
)

// DataFetchActivities holds dependencies for per-league data fetching activities.
type DataFetchActivities struct {
	DB      *gorm.DB
	Sleeper *sleeper.Client
}

// GetStaleLeagues returns up to batchSize league IDs ordered by last_fetched_at ASC NULLS FIRST,
// excluding leagues permanently skipped due to 404.
func (a *DataFetchActivities) GetStaleLeagues(ctx context.Context, params GetStaleLeaguesParams) ([]string, error) {
	var leagues []models.SleeperLeague
	err := a.DB.WithContext(ctx).
		Where("skipped_at IS NULL").
		Order("CASE WHEN last_fetched_at IS NULL THEN 0 ELSE 1 END, last_fetched_at ASC").
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

// FetchLeagueDetails fetches the full league object and updates scoring format fields
// (ppr, te_premium, is_superflex, scoring_settings, roster_positions).
func (a *DataFetchActivities) FetchLeagueDetails(ctx context.Context, params FetchLeagueDetailsParams) error {
	league, err := a.Sleeper.GetLeague(ctx, params.LeagueID)
	if err != nil {
		var nfe *sleeper.NotFoundError
		if errors.As(err, &nfe) {
			return temporal.NewNonRetryableApplicationError(
				"league not found: "+params.LeagueID, "NOT_FOUND", err,
			)
		}
		return err
	}

	scoringJSON, _ := json.Marshal(league.ScoringSettings)
	rosterJSON, _ := json.Marshal(league.RosterPositions)

	ppr := league.ScoringSettings["rec"]
	tePremium := league.ScoringSettings["bonus_rec_te"]
	isSuperflex := false
	for _, pos := range league.RosterPositions {
		if pos == "SUPER_FLEX" {
			isSuperflex = true
			break
		}
	}

	return a.DB.WithContext(ctx).
		Model(&models.SleeperLeague{}).
		Where("sleeper_league_id = ?", params.LeagueID).
		Updates(map[string]interface{}{
			"name":             league.Name,
			"status":           league.Status,
			"total_rosters":    league.TotalRosters,
			"ppr":              ppr,
			"te_premium":       tePremium,
			"is_superflex":     isSuperflex,
			"scoring_settings": scoringJSON,
			"roster_positions": rosterJSON,
		}).Error
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

// FetchLeagueTransactions fetches transactions for rounds 1–18 for leagueID.
// 404 responses for a round are treated as empty (no transactions) and skipped.
func (a *DataFetchActivities) FetchLeagueTransactions(ctx context.Context, params FetchLeagueTransactionsParams) error {
	for leg := 1; leg <= 18; leg++ {
		txns, err := a.Sleeper.GetTransactions(ctx, params.LeagueID, leg)
		if err != nil {
			var nfe *sleeper.NotFoundError
			if errors.As(err, &nfe) {
				continue // no transactions for this round is normal
			}
			return fmt.Errorf("leg %d: %w", leg, err)
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
				return err
			}
		}
	}
	return nil
}

// MarkLeagueFetched sets last_fetched_at=now() on the given league.
func (a *DataFetchActivities) MarkLeagueFetched(ctx context.Context, params MarkLeagueFetchedParams) error {
	now := time.Now().UTC()
	return a.DB.WithContext(ctx).
		Model(&models.SleeperLeague{}).
		Where("sleeper_league_id = ?", params.LeagueID).
		Update("last_fetched_at", now).Error
}

// MarkLeagueSkipped sets skipped_at=now() so the league is excluded from future batches.
func (a *DataFetchActivities) MarkLeagueSkipped(ctx context.Context, params MarkLeagueSkippedParams) error {
	now := time.Now().UTC()
	return a.DB.WithContext(ctx).
		Model(&models.SleeperLeague{}).
		Where("sleeper_league_id = ?", params.LeagueID).
		Update("skipped_at", now).Error
}
