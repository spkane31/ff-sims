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
}

func TestReconcileTradeValues_SkipsAlreadyValuedRows(t *testing.T) {
	db := newTestDB(t)
	ppr10League(t, db, "lg1")
	existing := json.RawMessage(`{"7": 999}`)
	db.Create(&models.SleeperTransaction{
		SleeperTransactionID: "tx1", SleeperLeagueID: "lg1", Type: "trade", Status: "complete",
		CreatedAtSleeper: time.Now().UTC().UnixMilli(), Adds: json.RawMessage(`{"p1": 7}`),
		TradeValues: existing,
	})
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
	db.Model(&models.SleeperTransaction{}).Where("trade_values IS NOT NULL").Count(&filled)
	if filled != 1 {
		t.Errorf("expected exactly 1 row filled with limit=1, got %d", filled)
	}
}
