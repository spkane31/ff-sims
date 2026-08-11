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

// computeTradeValuesForRows batch-loads player_valuations once per distinct
// segment present in inputs (not once per trade), then computes each
// trade's per-side totals. Mirrors the batching handlers.GetSleeperTrades
// used to do inline before this package took over — see
// docs/superpowers/specs/2026-08-10-trade-valuation-totals-design.md.
// Returns a map from trade ID to its computed trade_values JSON; a trade
// with an empty Segment or no fully-valued side is simply absent from the
// result, not present with a nil/empty value.
func computeTradeValuesForRows(ctx context.Context, db *gorm.DB, inputs []tradeValuationInput) map[string]json.RawMessage {
	result := map[string]json.RawMessage{}

	playersBySegment := map[string]map[string]struct{}{}
	var maxTime time.Time
	for _, in := range inputs {
		if in.Segment == "" {
			continue
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
		historyBySegment[seg] = valuation.LoadSnapshotHistory(db.WithContext(ctx), seg, ids, maxTime)
	}

	for _, in := range inputs {
		if in.Segment == "" {
			continue
		}
		rosterPlayers := valuation.GroupPlayersByRoster(in.Adds)
		if tv, ok := valuation.ComputeTradeValues(rosterPlayers, in.TradeTime, historyBySegment[in.Segment]); ok {
			result[in.ID] = tv
		}
	}
	return result
}
