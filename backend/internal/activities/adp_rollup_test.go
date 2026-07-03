package activities_test

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"backend/internal/activities"
	"backend/internal/models"
)

func floatPtr(v float64) *float64 { return &v }
func boolPtr(v bool) *bool        { return &v }

func seedADPLeague(t *testing.T, db *gorm.DB, id string, totalRosters int, ppr float64, superflex bool, leagueType string) {
	t.Helper()
	if err := db.Create(&models.SleeperLeague{
		SleeperLeagueID: id,
		TotalRosters:    totalRosters,
		PPR:             floatPtr(ppr),
		IsSuperflex:     boolPtr(superflex),
		LeagueType:      leagueType,
	}).Error; err != nil {
		t.Fatalf("seed league %s: %v", id, err)
	}
}

func seedADPDraft(t *testing.T, db *gorm.DB, id, leagueID, draftType, status, season string) {
	t.Helper()
	if err := db.Create(&models.SleeperDraft{
		SleeperDraftID:  id,
		SleeperLeagueID: leagueID,
		Type:            draftType,
		Status:          status,
		Season:          season,
	}).Error; err != nil {
		t.Fatalf("seed draft %s: %v", id, err)
	}
}

func seedADPPick(t *testing.T, db *gorm.DB, draftID string, round, pickNo int, playerID string) {
	t.Helper()
	if err := db.Create(&models.SleeperDraftPick{
		SleeperDraftID:  draftID,
		Round:           round,
		PickNo:          pickNo,
		SleeperPlayerID: playerID,
	}).Error; err != nil {
		t.Fatalf("seed pick %s/%d: %v", draftID, pickNo, err)
	}
}

var adpTestSegment = models.ADPSegment{LeagueSize: "12", ScoringFormat: "ppr", Superflex: true}

func TestListADPSeasons_ReturnsOnlyQualifyingSeasons(t *testing.T) {
	db := newTestDB(t)
	seedADPLeague(t, db, "lg1", 12, 1.0, true, "redraft")
	seedADPDraft(t, db, "d1", "lg1", "snake", "complete", "2024")   // qualifying
	seedADPDraft(t, db, "d2", "lg1", "auction", "complete", "2025") // wrong draft type
	seedADPLeague(t, db, "lg2", 12, 1.0, true, "dynasty")
	seedADPDraft(t, db, "d3", "lg2", "snake", "complete", "2026") // wrong league type

	a := &activities.ADPRollupActivities{DB: db}
	seasons, err := a.ListADPSeasons(context.Background())
	if err != nil {
		t.Fatalf("ListADPSeasons error: %v", err)
	}
	if len(seasons) != 1 || seasons[0] != "2024" {
		t.Errorf("expected [2024], got %v", seasons)
	}
}

func TestComputeSegmentSeasonADP_ComputesAverages(t *testing.T) {
	db := newTestDB(t)
	seedADPLeague(t, db, "lg1", 12, 1.0, true, "redraft")
	seedADPDraft(t, db, "d1", "lg1", "snake", "complete", "2024")
	seedADPDraft(t, db, "d2", "lg1", "snake", "complete", "2024")
	seedADPPick(t, db, "d1", 1, 1, "p1")
	seedADPPick(t, db, "d1", 1, 2, "p2")
	seedADPPick(t, db, "d2", 1, 3, "p1")
	seedADPPick(t, db, "d2", 1, 4, "p2")

	a := &activities.ADPRollupActivities{DB: db}
	if err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	}); err != nil {
		t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
	}

	var p1, p2 models.DraftADP
	if err := db.Where("segment = ? AND season = ? AND sleeper_player_id = ?", "12-ppr-sf", "2024", "p1").First(&p1).Error; err != nil {
		t.Fatalf("fetch p1 row: %v", err)
	}
	if p1.AvgPickNo != 2 || p1.PickCount != 2 || p1.MinPickNo != 1 || p1.MaxPickNo != 3 {
		t.Errorf("p1: got avg=%v count=%v min=%v max=%v", p1.AvgPickNo, p1.PickCount, p1.MinPickNo, p1.MaxPickNo)
	}

	if err := db.Where("segment = ? AND season = ? AND sleeper_player_id = ?", "12-ppr-sf", "2024", "p2").First(&p2).Error; err != nil {
		t.Fatalf("fetch p2 row: %v", err)
	}
	if p2.AvgPickNo != 3 || p2.PickCount != 2 || p2.MinPickNo != 2 || p2.MaxPickNo != 4 {
		t.Errorf("p2: got avg=%v count=%v min=%v max=%v", p2.AvgPickNo, p2.PickCount, p2.MinPickNo, p2.MaxPickNo)
	}
}

func TestComputeSegmentSeasonADP_ExcludesAuctionAndNonRedraft(t *testing.T) {
	db := newTestDB(t)
	seedADPLeague(t, db, "lg1", 12, 1.0, true, "redraft")
	seedADPDraft(t, db, "d-auction", "lg1", "auction", "complete", "2024")
	seedADPPick(t, db, "d-auction", 1, 1, "p-auction")

	seedADPLeague(t, db, "lg2", 12, 1.0, true, "dynasty")
	seedADPDraft(t, db, "d-dynasty", "lg2", "snake", "complete", "2024")
	seedADPPick(t, db, "d-dynasty", 1, 1, "p-dynasty")

	a := &activities.ADPRollupActivities{DB: db}
	if err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	}); err != nil {
		t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
	}

	var count int64
	db.Model(&models.DraftADP{}).Count(&count)
	if count != 0 {
		t.Errorf("expected no rows (auction/dynasty excluded), got %d", count)
	}
}

func TestComputeSegmentSeasonADP_NoMinDraftsThresholdAtWriteTime(t *testing.T) {
	db := newTestDB(t)
	seedADPLeague(t, db, "lg1", 12, 1.0, true, "redraft")
	seedADPDraft(t, db, "d1", "lg1", "snake", "complete", "2024")
	seedADPPick(t, db, "d1", 1, 1, "p1") // only 1 qualifying draft, well under the API's 20-draft threshold

	a := &activities.ADPRollupActivities{DB: db}
	if err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	}); err != nil {
		t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
	}

	var row models.DraftADP
	if err := db.Where("segment = ? AND season = ? AND sleeper_player_id = ?", "12-ppr-sf", "2024", "p1").First(&row).Error; err != nil {
		t.Fatalf("expected sub-threshold row to still be upserted: %v", err)
	}
	if row.PickCount != 1 {
		t.Errorf("expected pick_count 1, got %d", row.PickCount)
	}
}

func TestComputeSegmentSeasonADP_UpsertOverwritesPreviousRun(t *testing.T) {
	db := newTestDB(t)
	seedADPLeague(t, db, "lg1", 12, 1.0, true, "redraft")
	seedADPDraft(t, db, "d1", "lg1", "snake", "complete", "2024")
	seedADPPick(t, db, "d1", 1, 1, "p1")

	a := &activities.ADPRollupActivities{DB: db}
	run := func() {
		if err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
			Segment: adpTestSegment,
			Season:  "2024",
		}); err != nil {
			t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
		}
	}
	run() // first run: p1 picks [1] -> avg=1, count=1

	seedADPDraft(t, db, "d2", "lg1", "snake", "complete", "2024")
	seedADPPick(t, db, "d2", 1, 5, "p1")
	run() // second run: p1 picks [1,5] -> avg=3, count=2

	var rows []models.DraftADP
	db.Where("segment = ? AND season = ? AND sleeper_player_id = ?", "12-ppr-sf", "2024", "p1").Find(&rows)
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 row after upsert, got %d", len(rows))
	}
	if rows[0].AvgPickNo != 3 || rows[0].PickCount != 2 {
		t.Errorf("expected updated avg=3 count=2, got avg=%v count=%v", rows[0].AvgPickNo, rows[0].PickCount)
	}
}
