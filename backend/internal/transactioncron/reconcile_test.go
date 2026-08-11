package transactioncron_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"backend/internal/models"
	"backend/internal/transactioncron"
	"backend/internal/valuation"
)

func TestReconcileTradeValues_FillsNullRowWhenValuationExists(t *testing.T) {
	db := newTestDB(t)
	ppr10League(t, db, "lg1")

	tradeTime := time.Now().UTC().Add(-time.Hour)
	db.Create(&models.SleeperTransaction{
		SleeperTransactionID: "tx1", SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
		CreatedAtSleeper: tradeTime.UnixMilli(), Adds: json.RawMessage(`{"p1": 7}`),
	})
	db.Create(&valuation.Snapshot{Segment: "ppr-sf-10", SleeperPlayerID: "p1", ValuationDate: tradeTime.Add(-6 * time.Hour), Value: 4200})

	if err := transactioncron.ReconcileTradeValues(context.Background(), db, 200); err != nil {
		t.Fatalf("ReconcileTradeValues error: %v", err)
	}

	var tx models.SleeperTransaction
	db.First(&tx, "sleeper_transaction_id = ?", "tx1")
	if len(tx.TradeValues) == 0 {
		t.Fatal("expected trade_values to be filled in")
	}
	if !tx.TradeValuesComplete {
		t.Error("expected trade_values_complete=true — the trade's only roster resolved")
	}
	var totals map[string]float64
	json.Unmarshal(tx.TradeValues, &totals)
	if totals["7"] != 4200 {
		t.Errorf("expected {7:4200}, got %+v", totals)
	}
}

func TestReconcileTradeValues_LeavesRowNullWhenStillUnvalued(t *testing.T) {
	db := newTestDB(t)
	ppr10League(t, db, "lg1")
	db.Create(&models.SleeperTransaction{
		SleeperTransactionID: "tx1", SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
		CreatedAtSleeper: time.Now().UTC().UnixMilli(), Adds: json.RawMessage(`{"p1": 7}`),
	})

	if err := transactioncron.ReconcileTradeValues(context.Background(), db, 200); err != nil {
		t.Fatalf("ReconcileTradeValues error: %v", err)
	}

	var tx models.SleeperTransaction
	db.First(&tx, "sleeper_transaction_id = ?", "tx1")
	if len(tx.TradeValues) != 0 {
		t.Errorf("expected trade_values to stay null, got %s", tx.TradeValues)
	}
	if tx.TradeValuesComplete {
		t.Error("expected trade_values_complete=false — the trade's only roster never resolved")
	}
}

// TestReconcileTradeValues_SkipsCompleteRows covers the correct "already
// settled" case: a row is only left untouched when TradeValuesComplete is
// true, not merely because trade_values happens to be non-null (that
// distinction is exactly what TestReconcileTradeValues_FillsInMissingSideOfPartiallyValuedTrade
// covers on the other side).
func TestReconcileTradeValues_SkipsCompleteRows(t *testing.T) {
	db := newTestDB(t)
	ppr10League(t, db, "lg1")
	existing := json.RawMessage(`{"7": 999}`)
	db.Create(&models.SleeperTransaction{
		SleeperTransactionID: "tx1", SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
		CreatedAtSleeper: time.Now().UTC().UnixMilli(), Adds: json.RawMessage(`{"p1": 7}`),
		TradeValues: existing, TradeValuesComplete: true,
	})
	// A newer valuation for p1 exists, but must never be applied — this row
	// is already marked complete and the query must not even select it.
	db.Create(&valuation.Snapshot{Segment: "ppr-sf-10", SleeperPlayerID: "p1", ValuationDate: time.Now().UTC(), Value: 4200})

	if err := transactioncron.ReconcileTradeValues(context.Background(), db, 200); err != nil {
		t.Fatalf("ReconcileTradeValues error: %v", err)
	}

	var tx models.SleeperTransaction
	db.First(&tx, "sleeper_transaction_id = ?", "tx1")
	var totals map[string]float64
	json.Unmarshal(tx.TradeValues, &totals)
	if totals["7"] != 999 {
		t.Errorf("expected the pre-existing value 999 left untouched, got %+v", totals)
	}
}

// TestReconcileTradeValues_FillsInMissingSideOfPartiallyValuedTrade is the
// direct regression test for the reported bug: a two-sided trade where side
// A already resolved (trade_values non-null, from an earlier insert-time or
// reconcile pass) but side B's player had no valuation yet, so
// trade_values_complete was left false. Once side B's player gets a
// valuation, the next reconcile pass must fill it in — the old
// "trade_values IS NULL" gate would have skipped this row forever the
// moment side A resolved.
func TestReconcileTradeValues_FillsInMissingSideOfPartiallyValuedTrade(t *testing.T) {
	db := newTestDB(t)
	ppr10League(t, db, "lg1")

	tradeTime := time.Now().UTC().Add(-time.Hour)
	// Side A (roster 7, p1) already resolved by an earlier pass.
	db.Create(&models.SleeperTransaction{
		SleeperTransactionID: "tx1", SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
		CreatedAtSleeper: tradeTime.UnixMilli(), Adds: json.RawMessage(`{"p1": 7, "p2": 8}`),
		TradeValues: json.RawMessage(`{"7": 5000}`), TradeValuesComplete: false,
	})
	db.Create(&valuation.Snapshot{Segment: "ppr-sf-10", SleeperPlayerID: "p1", ValuationDate: tradeTime.Add(-6 * time.Hour), Value: 5000})
	// Side B's player (p2, roster 8) only just got a valuation.
	db.Create(&valuation.Snapshot{Segment: "ppr-sf-10", SleeperPlayerID: "p2", ValuationDate: tradeTime.Add(-6 * time.Hour), Value: 1800})

	if err := transactioncron.ReconcileTradeValues(context.Background(), db, 200); err != nil {
		t.Fatalf("ReconcileTradeValues error: %v", err)
	}

	var tx models.SleeperTransaction
	db.First(&tx, "sleeper_transaction_id = ?", "tx1")
	if !tx.TradeValuesComplete {
		t.Error("expected trade_values_complete=true — both rosters are now resolved")
	}
	var totals map[string]float64
	json.Unmarshal(tx.TradeValues, &totals)
	if totals["7"] != 5000 {
		t.Errorf("expected roster 7 = 5000 (already resolved) preserved, got %+v", totals)
	}
	if totals["8"] != 1800 {
		t.Errorf("expected roster 8 = 1800 now filled in, got %+v", totals)
	}
}

func TestReconcileTradeValues_RespectsLimit(t *testing.T) {
	db := newTestDB(t)
	ppr10League(t, db, "lg1")
	base := time.Now().UTC()
	for i := 0; i < 3; i++ {
		db.Create(&models.SleeperTransaction{
			SleeperTransactionID: "tx" + string(rune('1'+i)), SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
			CreatedAtSleeper: base.Add(-time.Duration(i) * time.Hour).UnixMilli(),
			Adds:             json.RawMessage(`{"p1": 7}`),
		})
	}
	db.Create(&valuation.Snapshot{Segment: "ppr-sf-10", SleeperPlayerID: "p1", ValuationDate: base.Add(-6 * time.Hour), Value: 4200})

	if err := transactioncron.ReconcileTradeValues(context.Background(), db, 1); err != nil {
		t.Fatalf("ReconcileTradeValues error: %v", err)
	}

	var filled int64
	db.Model(&models.SleeperTransaction{}).Where("trade_values_complete = ?", true).Count(&filled)
	if filled != 1 {
		t.Errorf("expected exactly 1 row filled with limit=1, got %d", filled)
	}
}
