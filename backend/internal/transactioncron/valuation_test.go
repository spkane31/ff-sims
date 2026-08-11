package transactioncron

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/valuation"
)

func newValuationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&valuation.Snapshot{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestComputeTradeValuesForRows(t *testing.T) {
	db := newValuationTestDB(t)
	now := time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)
	db.Create(&valuation.Snapshot{Segment: "ppr-sf-10", SleeperPlayerID: "p1", ValuationDate: now.Add(-6 * time.Hour), Value: 5000})
	db.Create(&valuation.Snapshot{Segment: "ppr-sf-10", SleeperPlayerID: "p2", ValuationDate: now.Add(-6 * time.Hour), Value: 1500})
	// p3 has no snapshot at all.

	inputs := []tradeValuationInput{
		{ID: "tx-full", TradeTime: now, Adds: map[string]int{"p1": 7, "p2": 8}, Segment: "ppr-sf-10"},
		{ID: "tx-partial", TradeTime: now, Adds: map[string]int{"p1": 7, "p3": 8}, Segment: "ppr-sf-10"},
		{ID: "tx-uncovered", TradeTime: now, Adds: map[string]int{"p1": 7}, Segment: ""},
	}

	got := computeTradeValuesForRows(context.Background(), db, inputs)

	if _, ok := got["tx-full"]; !ok {
		t.Fatal("expected tx-full to have computed values")
	}
	var full map[string]float64
	json.Unmarshal(got["tx-full"], &full)
	if full["7"] != 5000 || full["8"] != 1500 {
		t.Errorf("expected {7:5000, 8:1500}, got %+v", full)
	}

	if raw, ok := got["tx-partial"]; ok {
		var partial map[string]float64
		json.Unmarshal(raw, &partial)
		if _, present := partial["8"]; present {
			t.Errorf("expected roster 8 absent for tx-partial (p3 unvalued), got %+v", partial)
		}
		if partial["7"] != 5000 {
			t.Errorf("expected roster 7 = 5000 for tx-partial, got %+v", partial)
		}
	} else {
		t.Error("expected tx-partial to still have a partial result (roster 7 is valued)")
	}

	if _, ok := got["tx-uncovered"]; ok {
		t.Error("expected tx-uncovered (empty segment) to be skipped entirely")
	}
}

func TestComputeTradeValuesForRows_EmptyInputsReturnsEmptyMap(t *testing.T) {
	db := newValuationTestDB(t)
	got := computeTradeValuesForRows(context.Background(), db, nil)
	if len(got) != 0 {
		t.Errorf("expected empty result, got %+v", got)
	}
}
