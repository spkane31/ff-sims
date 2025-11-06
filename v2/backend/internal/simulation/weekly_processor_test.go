package simulation

import (
	"backend/internal/database"
	"backend/internal/models"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to connect to test database")
	}

	// Auto-migrate test schemas
	err = db.AutoMigrate(
		&models.League{},
		&models.Team{},
		&models.TeamNameHistory{},
		&models.Matchup{},
		&models.Player{},
		&models.BoxScore{},
		&models.WeeklyExpectedWins{},
		&models.SeasonExpectedWins{},
	)
	if err != nil {
		panic("failed to migrate test database")
	}

	return db
}

func createTestData(db *gorm.DB) {
	// Create test league
	league := &models.League{
		ID:   1,
		Name: "Test League",
	}
	db.Create(league)

	// Create test teams
	teams := []models.Team{
		{ID: 1, Name: "Team A", Owner: "Owner A", LeagueID: 1, ESPNID: 1},
		{ID: 2, Name: "Team B", Owner: "Owner B", LeagueID: 1, ESPNID: 2},
		{ID: 3, Name: "Team C", Owner: "Owner C", LeagueID: 1, ESPNID: 3},
		{ID: 4, Name: "Team D", Owner: "Owner D", LeagueID: 1, ESPNID: 4},
	}

	for _, team := range teams {
		db.Create(&team)
	}

	// Create test matchups
	matchups := []models.Matchup{
		{
			ID: 1, LeagueID: 1, Week: 1, Year: 2024, Season: 2024,
			HomeTeamID: 1, AwayTeamID: 2,
			HomeTeamFinalScore: 120.5, AwayTeamFinalScore: 95.0,
			Completed: true, GameType: "NONE", GameDate: time.Now(),
		},
		{
			ID: 2, LeagueID: 1, Week: 1, Year: 2024, Season: 2024,
			HomeTeamID: 3, AwayTeamID: 4,
			HomeTeamFinalScore: 110.0, AwayTeamFinalScore: 105.0,
			Completed: true, GameType: "NONE", GameDate: time.Now(),
		},
		{
			ID: 3, LeagueID: 1, Week: 2, Year: 2024, Season: 2024,
			HomeTeamID: 1, AwayTeamID: 3,
			HomeTeamFinalScore: 100.0, AwayTeamFinalScore: 115.0,
			Completed: true, GameType: "NONE", GameDate: time.Now(),
		},
		{
			ID: 4, LeagueID: 1, Week: 2, Year: 2024, Season: 2024,
			HomeTeamID: 2, AwayTeamID: 4,
			HomeTeamFinalScore: 108.0, AwayTeamFinalScore: 102.0,
			Completed: true, GameType: "NONE", GameDate: time.Now(),
		},
	}

	for _, matchup := range matchups {
		db.Create(&matchup)
	}
}

func TestProcessWeeklyExpectedWins(t *testing.T) {
	db := setupTestDB()
	createTestData(db)

	// Set the database connection for the module
	originalDB := database.DB
	database.DB = db
	defer func() { database.DB = originalDB }()

	// Test processing week 1
	err := ProcessWeeklyExpectedWins(1, 2024, 1)
	if err != nil {
		t.Fatalf("Failed to process weekly expected wins: %v", err)
	}

	// Verify records were created
	var weeklyRecords []models.WeeklyExpectedWins
	err = db.Where("league_id = ? AND year = ? AND week = ?", 1, 2024, 1).Find(&weeklyRecords).Error
	if err != nil {
		t.Fatalf("Failed to fetch weekly records: %v", err)
	}

	if len(weeklyRecords) != 4 {
		t.Errorf("Expected 4 weekly records, got %d", len(weeklyRecords))
		// Debug: show which teams we got
		for _, record := range weeklyRecords {
			t.Logf("Got record for team %d: expected=%.3f, weekly=%.3f", record.TeamID, record.ExpectedWins, record.WeeklyExpectedWins)
		}
	}

	// Check that expected wins are calculated properly
	for _, record := range weeklyRecords {
		if record.ExpectedWins < 0 || record.ExpectedWins > float64(record.ActualWins+record.ActualLosses) {
			t.Errorf("Expected wins should be between 0 and total games for team %d, got %.3f", record.TeamID, record.ExpectedWins)
		}
		if record.WeeklyExpectedWins < 0 || record.WeeklyExpectedWins > 1 {
			t.Errorf("Weekly expected wins should be between 0 and 1 for team %d, got %.3f", record.TeamID, record.WeeklyExpectedWins)
		}

		// Check that weekly + cumulative relationship makes sense
		if record.Week == 1 {
			// For week 1, cumulative and weekly should be approximately equal since there's only one week
			// Allow some tolerance for simulation variance
			tolerance := 0.1
			if record.ExpectedWins-record.WeeklyExpectedWins > tolerance || record.WeeklyExpectedWins-record.ExpectedWins > tolerance {
				t.Errorf("Team %d week 1: cumulative expected wins (%.3f) should be close to weekly (%.3f)",
					record.TeamID, record.ExpectedWins, record.WeeklyExpectedWins)
			}
		}
	}
}

func TestProcessWeeklyExpectedWins_NoCompletedGames(t *testing.T) {
	db := setupTestDB()

	// Create league and teams but no completed matchups
	league := &models.League{ID: 1, Name: "Test League"}
	db.Create(league)

	team := &models.Team{ID: 1, Name: "Team A", LeagueID: 1, ESPNID: 1}
	db.Create(team)

	// Set the database connection for the module
	originalDB := database.DB
	database.DB = db
	defer func() { database.DB = originalDB }()

	// Should not error with no completed games
	err := ProcessWeeklyExpectedWins(1, 2024, 1)
	if err != nil {
		t.Errorf("Should not error with no completed games: %v", err)
	}
}

func TestGetCompletedMatchupsByWeek(t *testing.T) {
	db := setupTestDB()
	createTestData(db)

	matchups, err := GetCompletedMatchupsByWeek(db, 1, 2024, 1)
	if err != nil {
		t.Fatalf("Failed to get completed matchups: %v", err)
	}

	if len(matchups) != 2 {
		t.Errorf("Expected 2 matchups for week 1, got %d", len(matchups))
	}

	// Verify all returned matchups are completed
	for _, matchup := range matchups {
		if !matchup.Completed {
			t.Errorf("Expected all matchups to be completed, found incomplete matchup ID %d", matchup.ID)
		}
		if matchup.Week != 1 {
			t.Errorf("Expected week 1, got week %d", matchup.Week)
		}
	}
}

func TestGetTeamMatchupsThroughWeek(t *testing.T) {
	db := setupTestDB()
	createTestData(db)

	// Get Team 1's matchups through week 2
	matchups, err := GetTeamMatchupsThroughWeek(db, 1, 2024, 2)
	if err != nil {
		t.Fatalf("Failed to get team matchups: %v", err)
	}

	if len(matchups) != 2 {
		t.Errorf("Expected 2 matchups for team 1 through week 2, got %d", len(matchups))
	}

	// Verify matchups are in order and involve team 1
	for i, matchup := range matchups {
		if matchup.HomeTeamID != 1 && matchup.AwayTeamID != 1 {
			t.Errorf("Expected matchup to involve team 1, got home=%d away=%d", matchup.HomeTeamID, matchup.AwayTeamID)
		}
		if i > 0 && matchup.Week < matchups[i-1].Week {
			t.Errorf("Expected matchups to be ordered by week")
		}
	}
}

func TestConvertMatchupsToPointers(t *testing.T) {
	matchups := []models.Matchup{
		{ID: 1, Week: 1},
		{ID: 2, Week: 2},
	}

	pointers := convertMatchupsToPointers(matchups)

	if len(pointers) != 2 {
		t.Errorf("Expected 2 pointers, got %d", len(pointers))
	}

	if pointers[0].ID != 1 || pointers[1].ID != 2 {
		t.Errorf("Pointers don't match original matchups")
	}
}

func TestFindTeamMatchupForWeek(t *testing.T) {
	matchups := []models.Matchup{
		{ID: 1, Week: 1},
		{ID: 2, Week: 2},
		{ID: 3, Week: 3},
	}

	// Find week 2
	result := findTeamMatchupForWeek(matchups, 2)
	if result == nil {
		t.Error("Expected to find matchup for week 2")
	} else if result.Week != 2 {
		t.Errorf("Expected week 2, got week %d", result.Week)
	}

	// Find non-existent week
	result = findTeamMatchupForWeek(matchups, 5)
	if result != nil {
		t.Error("Expected nil for non-existent week")
	}
}

// func TestRecalculateWeeklyExpectedWins(t *testing.T) {
// 	db := setupTestDB()
// 	createTestData(db)

// 	// Set the database connection for the module
// 	originalDB := database.DB
// 	database.DB = db
// 	defer func() { database.DB = originalDB }()

// 	// First create some existing records
// 	existingRecord := &models.WeeklyExpectedWins{
// 		TeamID: 1, Week: 1, Year: 2024, LeagueID: 1,
// 		ExpectedWins: 999.0, // Invalid value to check if it gets recalculated
// 	}
// 	db.Create(existingRecord)

// 	// Recalculate
// 	err := RecalculateWeeklyExpectedWins(1, 2024)
// 	if err != nil {
// 		t.Fatalf("Failed to recalculate: %v", err)
// 	}

// 	// Check that records were recalculated
// 	var updatedRecord models.WeeklyExpectedWins
// 	err = db.Where("team_id = ? AND week = ? AND year = ?", 1, 1, 2024).First(&updatedRecord).Error
// 	if err != nil {
// 		t.Fatalf("Failed to find recalculated record: %v", err)
// 	}

// 	if updatedRecord.ExpectedWins == 999.0 {
// 		t.Error("Expected wins was not recalculated")
// 	}
// }

func TestIsRegularSeasonComplete(t *testing.T) {
	db := setupTestDB()
	createTestData(db)

	// Should not be complete with incomplete games
	incomplete := IsRegularSeasonComplete(db, 1, 2024)
	if incomplete {
		t.Error("Season should not be complete with existing incomplete games")
	}

	// Complete all regular season games
	db.Model(&models.Matchup{}).Where("is_playoff = false").Update("completed", true)

	// Should be complete now
	complete := IsRegularSeasonComplete(db, 1, 2024)
	if !complete {
		t.Error("Season should be complete with all games finished")
	}
}

func TestWeeklyExpectedWinsValues(t *testing.T) {
	db := setupTestDB()

	// Set the database connection for the module
	originalDB := database.DB
	database.DB = db
	defer func() { database.DB = originalDB }()

	createTestData(db)

	// Process week 1
	err := ProcessWeeklyExpectedWins(1, 2024, 1)
	if err != nil {
		t.Fatalf("Failed to process week 1: %v", err)
	}

	// Check that weekly expected wins are ≤ 1 and cumulative values are correct
	var weeklyRecords []models.WeeklyExpectedWins
	err = db.Where("league_id = ? AND year = ? AND week = ?", 1, 2024, 1).Find(&weeklyRecords).Error
	if err != nil {
		t.Fatalf("Failed to fetch weekly records: %v", err)
	}

	if len(weeklyRecords) == 0 {
		t.Fatal("No weekly records created")
	}

	for _, record := range weeklyRecords {
		// WeeklyExpectedWins should be ≤ 1 (this week only)
		if record.WeeklyExpectedWins > 1.0 {
			t.Errorf("Team %d: WeeklyExpectedWins should be ≤ 1, got %.3f", record.TeamID, record.WeeklyExpectedWins)
		}
		if record.WeeklyExpectedWins < 0.0 {
			t.Errorf("Team %d: WeeklyExpectedWins should be ≥ 0, got %.3f", record.TeamID, record.WeeklyExpectedWins)
		}

		// For week 1, ExpectedWins (cumulative) should equal WeeklyExpectedWins
		if record.Week == 1 && record.ExpectedWins != record.WeeklyExpectedWins {
			t.Errorf("Team %d: In week 1, ExpectedWins (%.3f) should equal WeeklyExpectedWins (%.3f)",
				record.TeamID, record.ExpectedWins, record.WeeklyExpectedWins)
		}

		// ActualWins/ActualLosses are cumulative, so in week 1 they should be 0 or 1
		if record.Week == 1 && record.ActualWins != 0 && record.ActualWins != 1 {
			t.Errorf("Team %d: In week 1, ActualWins should be 0 or 1, got %d", record.TeamID, record.ActualWins)
		}
		if record.Week == 1 && record.ActualLosses != 0 && record.ActualLosses != 1 {
			t.Errorf("Team %d: In week 1, ActualLosses should be 0 or 1, got %d", record.TeamID, record.ActualLosses)
		}

		// Each team should have exactly 1 game per week (cumulative in week 1)
		if record.Week == 1 && record.ActualWins+record.ActualLosses != 1 {
			t.Errorf("Team %d: In week 1, ActualWins + ActualLosses should equal 1, got %d + %d = %d",
				record.TeamID, record.ActualWins, record.ActualLosses, record.ActualWins+record.ActualLosses)
		}
	}
}

func TestWeeklyProcessorMultipleWeeks(t *testing.T) {
	db := setupTestDB()

	// Set the database connection for the module
	originalDB := database.DB
	database.DB = db
	defer func() { database.DB = originalDB }()

	createTestData(db)

	// Process weeks 1 and 2
	for week := uint(1); week <= 2; week++ {
		err := ProcessWeeklyExpectedWins(1, 2024, week)
		if err != nil {
			t.Fatalf("Failed to process week %d: %v", week, err)
		}
	}

	// Verify each week has its own records with proper cumulative behavior
	for week := uint(1); week <= 2; week++ {
		var weeklyRecords []models.WeeklyExpectedWins
		err := db.Where("league_id = ? AND year = ? AND week = ?", 1, 2024, week).Find(&weeklyRecords).Error
		if err != nil {
			t.Fatalf("Failed to fetch weekly records for week %d: %v", week, err)
		}

		if len(weeklyRecords) == 0 {
			t.Errorf("No weekly records found for week %d", week)
			continue
		}

		for _, record := range weeklyRecords {
			// WeeklyExpectedWins should be ≤ 1 (just this week)
			if record.WeeklyExpectedWins > 1.0 {
				t.Errorf("Week %d, Team %d: WeeklyExpectedWins should be ≤ 1, got %.3f", week, record.TeamID, record.WeeklyExpectedWins)
			}
			if record.WeeklyExpectedWins < 0.0 {
				t.Errorf("Week %d, Team %d: WeeklyExpectedWins should be ≥ 0, got %.3f", week, record.TeamID, record.WeeklyExpectedWins)
			}

			// ExpectedWins (cumulative) should be >= WeeklyExpectedWins and should increase with weeks
			if record.ExpectedWins < record.WeeklyExpectedWins {
				t.Errorf("Week %d, Team %d: ExpectedWins (%.3f) should be >= WeeklyExpectedWins (%.3f)",
					week, record.TeamID, record.ExpectedWins, record.WeeklyExpectedWins)
			}

			// Verify the week field is correct
			if record.Week != week {
				t.Errorf("Expected week %d, got %d for record", week, record.Week)
			}
		}
	}

	// Verify that teams who played in both weeks have separate entries
	var team1Records []models.WeeklyExpectedWins
	err := db.Where("team_id = ? AND year = ?", 1, 2024).Order("week ASC").Find(&team1Records).Error
	if err != nil {
		t.Fatalf("Failed to fetch team 1 records: %v", err)
	}

	if len(team1Records) != 2 {
		t.Errorf("Expected team 1 to have 2 weekly records, got %d", len(team1Records))
	}

	// Each record should be for a different week and have cumulative behavior
	if len(team1Records) >= 2 {
		if team1Records[0].Week != 1 || team1Records[1].Week != 2 {
			t.Errorf("Expected records for weeks 1 and 2, got weeks %d and %d",
				team1Records[0].Week, team1Records[1].Week)
		}

		// Cumulative ExpectedWins should increase or stay the same from week 1 to week 2
		if team1Records[1].ExpectedWins < team1Records[0].ExpectedWins {
			t.Errorf("Team 1: Expected wins should increase or stay same from week 1 (%.3f) to week 2 (%.3f)",
				team1Records[0].ExpectedWins, team1Records[1].ExpectedWins)
		}

		// Cumulative ActualWins should increase or stay the same from week 1 to week 2
		if team1Records[1].ActualWins < team1Records[0].ActualWins {
			t.Errorf("Team 1: Actual wins should increase or stay same from week 1 (%d) to week 2 (%d)",
				team1Records[0].ActualWins, team1Records[1].ActualWins)
		}
	}
}

func TestWeeklyProcessorIdempotent(t *testing.T) {
	db := setupTestDB()

	// Set the database connection for the module
	originalDB := database.DB
	database.DB = db
	defer func() { database.DB = originalDB }()

	createTestData(db)

	// Process week 1 the first time
	err := ProcessWeeklyExpectedWins(1, 2024, 1)
	if err != nil {
		t.Fatalf("Failed to process week 1 (first time): %v", err)
	}

	// Get the records after first processing
	var firstRun []models.WeeklyExpectedWins
	err = db.Where("league_id = ? AND year = ? AND week = ?", 1, 2024, 1).Find(&firstRun).Error
	if err != nil {
		t.Fatalf("Failed to fetch records after first run: %v", err)
	}

	if len(firstRun) == 0 {
		t.Fatal("No records created in first run")
	}

	// Process week 1 again (should be idempotent)
	err = ProcessWeeklyExpectedWins(1, 2024, 1)
	if err != nil {
		t.Fatalf("Failed to process week 1 (second time): %v", err)
	}

	// Get the records after second processing
	var secondRun []models.WeeklyExpectedWins
	err = db.Where("league_id = ? AND year = ? AND week = ?", 1, 2024, 1).Find(&secondRun).Error
	if err != nil {
		t.Fatalf("Failed to fetch records after second run: %v", err)
	}

	// Should have the same number of records (no duplicates)
	if len(firstRun) != len(secondRun) {
		t.Errorf("Expected same number of records after second run. First: %d, Second: %d",
			len(firstRun), len(secondRun))
	}

	// Compare the values - they should be nearly identical
	tolerance := 0.001
	for i, firstRecord := range firstRun {
		found := false
		for _, secondRecord := range secondRun {
			if firstRecord.TeamID == secondRecord.TeamID {
				found = true

				// Check that values are nearly identical
				if absFloat(firstRecord.ExpectedWins-secondRecord.ExpectedWins) > tolerance {
					t.Errorf("Team %d ExpectedWins changed: %.6f -> %.6f",
						firstRecord.TeamID, firstRecord.ExpectedWins, secondRecord.ExpectedWins)
				}

				if absFloat(firstRecord.WeeklyExpectedWins-secondRecord.WeeklyExpectedWins) > tolerance {
					t.Errorf("Team %d WeeklyExpectedWins changed: %.6f -> %.6f",
						firstRecord.TeamID, firstRecord.WeeklyExpectedWins, secondRecord.WeeklyExpectedWins)
				}

				if firstRecord.ActualWins != secondRecord.ActualWins {
					t.Errorf("Team %d ActualWins changed: %d -> %d",
						firstRecord.TeamID, firstRecord.ActualWins, secondRecord.ActualWins)
				}

				if firstRecord.ActualLosses != secondRecord.ActualLosses {
					t.Errorf("Team %d ActualLosses changed: %d -> %d",
						firstRecord.TeamID, firstRecord.ActualLosses, secondRecord.ActualLosses)
				}

				// Check that IDs are preserved (proof of update, not recreation)
				if firstRecord.ID != secondRecord.ID {
					t.Errorf("Team %d: Record ID changed from %d to %d (should be updated, not recreated)",
						firstRecord.TeamID, firstRecord.ID, secondRecord.ID)
				}

				break
			}
		}

		if !found {
			t.Errorf("Team %d from first run not found in second run", firstRun[i].TeamID)
		}
	}
}

// Helper function for absolute value in tests
func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
