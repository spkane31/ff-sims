package transactioncron

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"backend/internal/valuation"
)

// tradeValuationInput is one trade candidate for value computation — either
// a freshly-fetched row (FlushLeagueTransactions) or one read back from the
// DB with a null trade_values column (ReconcileTradeValues). Segment is
// resolved by the caller (it needs the trade's league settings, which this
// package's two call sites fetch differently) and is "" for trades outside
// the model's covered segments — computeTradeValuesForRows skips those
// without touching the database.
type tradeValuationInput struct {
	ID        string
	TradeTime time.Time
	Adds      map[string]int
	Segment   string
}

// tradeValuationResult is one trade's outcome from computeTradeValuesForRows.
// Values holds whatever sides resolved (nil if none), and Complete reports
// whether every side that has traded players now has a value — a trade can
// have Values non-nil (one side resolved) while Complete is still false
// (another side is still pending), which is what lets ReconcileTradeValues
// keep retrying it instead of treating it as settled just because
// trade_values is non-null.
type tradeValuationResult struct {
	Values   json.RawMessage
	Complete bool
}

// computeTradeValuesForRows batch-loads player_valuations once per distinct
// segment present in inputs (not once per trade), then computes each
// trade's per-side totals. Mirrors the batching handlers.GetSleeperTrades
// used to do inline before this package took over — see
// docs/superpowers/specs/2026-08-10-trade-valuation-totals-design.md.
// Returns a map from trade ID to its result; a trade with an empty Segment
// is simply absent from the result (it was never attempted at all — the
// caller decides which trades qualify, this function skips only that
// exclusion), not present with a zero-value result.
func computeTradeValuesForRows(ctx context.Context, db *gorm.DB, inputs []tradeValuationInput) map[string]tradeValuationResult {
	result := map[string]tradeValuationResult{}

	playersBySegment := map[string]map[string]struct{}{}
	var minTime, maxTime time.Time
	for _, in := range inputs {
		if in.Segment == "" {
			continue
		}
		if minTime.IsZero() || in.TradeTime.Before(minTime) {
			minTime = in.TradeTime
		}
		if in.TradeTime.After(maxTime) {
			maxTime = in.TradeTime
		}
		if playersBySegment[in.Segment] == nil {
			playersBySegment[in.Segment] = map[string]struct{}{}
		}
		for pid := range in.Adds {
			playersBySegment[in.Segment][pid] = struct{}{}
		}
	}
	if len(playersBySegment) == 0 {
		return result
	}

	historyBySegment := map[string]map[string][]valuation.Snapshot{}
	for seg, idSet := range playersBySegment {
		ids := make([]string, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		historyBySegment[seg] = valuation.LoadSnapshotHistory(db.WithContext(ctx), seg, ids, minTime.Add(-valuation.FreshnessWindow), maxTime)
	}

	for _, in := range inputs {
		if in.Segment == "" {
			continue
		}
		rosterPlayers := valuation.GroupPlayersByRoster(in.Adds)
		values, complete := valuation.ComputeTradeValues(rosterPlayers, in.TradeTime, historyBySegment[in.Segment])
		result[in.ID] = tradeValuationResult{Values: values, Complete: complete}
	}
	return result
}
