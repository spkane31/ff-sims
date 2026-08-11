package transactioncron

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"backend/internal/models"
	"backend/internal/valuation"
)

// ReconcileTradeValues fills in trade_values for up to limit trades whose
// value is still null, newest first, restricted at the query level to
// leagues in a covered valuation format (matching
// valuation.SegmentKeyForLeague's own conditions) so a permanently-uncovered
// league's trades can never crowd the LIMIT window and starve reconcilable
// ones. This is the sole backfill mechanism for pre-existing trades — see
// docs/superpowers/specs/2026-08-10-trade-valuation-totals-design.md — and
// must stay cheap: RunTransactionSync runs it concurrently with, not after,
// each tick's fetch/flush, under its own short deadline, so it can never
// delay new-trade ingestion.
func ReconcileTradeValues(ctx context.Context, db *gorm.DB, limit int) error {
	type row struct {
		SleeperTransactionID string          `gorm:"column:sleeper_transaction_id"`
		Adds                 json.RawMessage `gorm:"column:adds"`
		CreatedAtSleeper     int64           `gorm:"column:created_at_sleeper"`
		PPR                  *float64        `gorm:"column:ppr"`
		IsSuperflex          *bool           `gorm:"column:is_superflex"`
		TotalRosters         int             `gorm:"column:total_rosters"`
		LeagueType           string          `gorm:"column:league_type"`
	}
	var rows []row
	if err := db.WithContext(ctx).Table("sleeper_transactions t").
		Select("t.sleeper_transaction_id, t.adds, t.created_at_sleeper, l.ppr, l.is_superflex, l.total_rosters, l.league_type").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = t.sleeper_league_id").
		Where("t.type = ? AND t.status = ? AND t.trade_values IS NULL AND l.ppr = ? AND l.is_superflex = ? AND l.league_type = ?",
			"trade", "complete", 1.0, true, "redraft").
		Order("t.created_at_sleeper DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return fmt.Errorf("select null trade_values: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	inputs := make([]tradeValuationInput, 0, len(rows))
	for _, r := range rows {
		seg := valuation.SegmentKeyForLeague(r.PPR, r.IsSuperflex, r.TotalRosters, r.LeagueType)
		if seg == "" {
			continue
		}
		var adds map[string]int
		if len(r.Adds) > 0 {
			if err := json.Unmarshal(r.Adds, &adds); err != nil {
				continue
			}
		}
		inputs = append(inputs, tradeValuationInput{
			ID: r.SleeperTransactionID, TradeTime: time.UnixMilli(r.CreatedAtSleeper).UTC(),
			Adds: adds, Segment: seg,
		})
	}
	if len(inputs) == 0 {
		return nil
	}

	for id, tv := range computeTradeValuesForRows(ctx, db, inputs) {
		if err := db.WithContext(ctx).Model(&models.SleeperTransaction{}).
			Where("sleeper_transaction_id = ? AND trade_values IS NULL", id).
			Update("trade_values", tv).Error; err != nil {
			return fmt.Errorf("update trade_values for %s: %w", id, err)
		}
	}
	return nil
}
