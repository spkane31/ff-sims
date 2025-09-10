package simulation

import (
	"backend/internal/database"
	"backend/internal/models"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFinalizeSeasonExpectedWins(t *testing.T) {
	db := setupTestDBForSeason()
	createSeasonTestData(db)

	// Set the database connection for the module
	originalDB := database.DB
	database.DB = db
	defer func() { database.DB = originalDB }()

	err := FinalizeSeasonExpectedWins(1, 2024)
	if err != nil {
		t.Fatalf("Failed to finalize season: %v", err)
	}

	// Verify season records were created
	var seasonRecords []models.SeasonExpectedWins
	err = db.Where("league_id = ? AND year = ?", 1, 2024).Find(&seasonRecords).Error
	if err != nil {
		t.Fatalf("Failed to fetch season records: %v", err)
	}

	if len(seasonRecords) == 0 {
		t.Error("Expected season records to be created")
	}

	// Check that season data is populated
	for _, record := range seasonRecords {
		if record.ExpectedWins <= 0 {
			t.Errorf("Expected wins should be > 0, got %.3f for team %d", record.ExpectedWins, record.TeamID)
		}
		if record.FinalWeek == 0 {
			t.Errorf("Final week should be set, got 0 for team %d", record.TeamID)
		}
	}
}

func TestCalculateSeasonAggregates(t *testing.T) {
	db := setupTestDBForSeason()
	createSeasonTestData(db)

	aggregates, err := models.CalculateSeasonAggregates(db, 1, 2024, 2)
	if err != nil {
		t.Fatalf("Failed to calculate season aggregates: %v", err)
	}

	if aggregates.TotalPointsFor <= 0 {
		t.Errorf("Expected total points for > 0, got %.2f", aggregates.TotalPointsFor)
	}
	if aggregates.TotalPointsAgainst <= 0 {
		t.Errorf("Expected total points against > 0, got %.2f", aggregates.TotalPointsAgainst)
	}
	if aggregates.GamesPlayed != 2 {
		t.Errorf("Expected 2 games played, got %d", aggregates.GamesPlayed)
	}
	if aggregates.AveragePointsFor <= 0 {
		t.Errorf("Expected average points for > 0, got %.2f", aggregates.AveragePointsFor)
	}
}

func TestGetSeasonExpectedWinsRankings(t *testing.T) {
	db := setupTestDBForSeason()
	createSeasonTestData(db)

	// Create some season records first
	seasonRecords := []models.SeasonExpectedWins{
		{TeamID: 1, Year: 2024, LeagueID: 1, ExpectedWins: 8.5, ActualWins: 9},
		{TeamID: 2, Year: 2024, LeagueID: 1, ExpectedWins: 7.2, ActualWins: 6},
		{TeamID: 3, Year: 2024, LeagueID: 1, ExpectedWins: 6.8, ActualWins: 7},
	}

	for _, record := range seasonRecords {
		db.Create(&record)
	}

	rankings, err := GetSeasonExpectedWinsRankings(1, 2024)
	if err != nil {
		t.Fatalf("Failed to get rankings: %v", err)
	}

	// Check expected wins ranking
	if len(rankings.ByExpectedWins) != 3 {
		t.Errorf("Expected 3 teams in expected wins ranking, got %d", len(rankings.ByExpectedWins))
	}
	if rankings.ByExpectedWins[0].TeamID != 1 {
		t.Errorf("Expected team 1 to be first in expected wins, got team %d", rankings.ByExpectedWins[0].TeamID)
	}

	// Check luck ranking
	if len(rankings.ByLuck) != 3 {
		t.Errorf("Expected 3 teams in luck ranking, got %d", len(rankings.ByLuck))
	}
	if rankings.ByLuck[0].TeamID != 1 {
		t.Errorf("Expected team 1 to be luckiest, got team %d", rankings.ByLuck[0].TeamID)
	}
}

func TestCalculateLeagueLuckDistribution(t *testing.T) {
	db := setupTestDBForSeason()
	createSeasonTestData(db)

	// Create season records with varying luck
	seasonRecords := []models.SeasonExpectedWins{
		{TeamID: 1, Year: 2024, LeagueID: 1, Team: &models.Team{Owner: "Owner A"}},
		{TeamID: 2, Year: 2024, LeagueID: 1, Team: &models.Team{Owner: "Owner B"}},
		{TeamID: 3, Year: 2024, LeagueID: 1, Team: &models.Team{Owner: "Owner C"}},
	}

	for _, record := range seasonRecords {
		db.Create(&record)
	}

	distribution, err := CalculateLeagueLuckDistribution(1, 2024)
	if err != nil {
		t.Fatalf("Failed to calculate luck distribution: %v", err)
	}

	if distribution.MostLuckyLuck != 2.0 {
		t.Errorf("Expected most lucky luck to be 2.0, got %.1f", distribution.MostLuckyLuck)
	}
	if distribution.MostUnluckyLuck != -1.5 {
		t.Errorf("Expected most unlucky luck to be -1.5, got %.1f", distribution.MostUnluckyLuck)
	}
	if distribution.LuckRange != 3.5 {
		t.Errorf("Expected luck range to be 3.5, got %.1f", distribution.LuckRange)
	}
}

func TestRecalculateSeasonExpectedWins(t *testing.T) {
	db := setupTestDBForSeason()
	createSeasonTestData(db)

	// Create some existing season records
	existingRecord := &models.SeasonExpectedWins{
		TeamID: 1, Year: 2024, LeagueID: 1,
		ExpectedWins: 999.0, // Invalid value to check if it gets recalculated
	}
	db.Create(existingRecord)

	// Recalculate
	err := RecalculateSeasonExpectedWins(1, 2024)
	if err != nil {
		t.Fatalf("Failed to recalculate season: %v", err)
	}

	// Check that records were recalculated
	var updatedRecord models.SeasonExpectedWins
	err = db.Where("team_id = ? AND year = ?", 1, 2024).First(&updatedRecord).Error
	if err != nil {
		t.Fatalf("Failed to find recalculated record: %v", err)
	}

	if updatedRecord.ExpectedWins == 999.0 {
		t.Error("Season expected wins was not recalculated")
	}
}

func TestUpdateSeasonStandings(t *testing.T) {
	db := setupTestDBForSeason()

	// Create season records with different wins
	seasonRecords := []models.SeasonExpectedWins{
		{TeamID: 1, Year: 2024, LeagueID: 1, ActualWins: 10, TotalPointsFor: 1500},
		{TeamID: 2, Year: 2024, LeagueID: 1, ActualWins: 8, TotalPointsFor: 1400},
		{TeamID: 3, Year: 2024, LeagueID: 1, ActualWins: 6, TotalPointsFor: 1300},
	}

	for _, record := range seasonRecords {
		db.Create(&record)
	}

	err := UpdateSeasonStandings(1, 2024)
	if err != nil {
		t.Fatalf("Failed to update standings: %v", err)
	}

	// Check standings were updated
	var updatedRecords []models.SeasonExpectedWins
	err = db.Where("league_id = ? AND year = ?", 1, 2024).
		Order("final_standing ASC").
		Find(&updatedRecords).Error
	if err != nil {
		t.Fatalf("Failed to fetch updated records: %v", err)
	}

	if len(updatedRecords) != 3 {
		t.Fatalf("Expected 3 records, got %d", len(updatedRecords))
	}

	// Check standings are correct (team with most wins should be 1st)
	if updatedRecords[0].TeamID != 1 || updatedRecords[0].FinalStanding != 1 {
		t.Errorf("Expected team 1 to be 1st place, got team %d in position %d", updatedRecords[0].TeamID, updatedRecords[0].FinalStanding)
	}
	if updatedRecords[1].TeamID != 2 || updatedRecords[1].FinalStanding != 2 {
		t.Errorf("Expected team 2 to be 2nd place, got team %d in position %d", updatedRecords[1].TeamID, updatedRecords[1].FinalStanding)
	}
}

// Helper functions
func setupTestDBForSeason() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to connect to test database")
	}

	// Auto-migrate test schemas
	err = db.AutoMigrate(
		&models.League{},
		&models.Team{},
		&models.Matchup{},
		&models.WeeklyExpectedWins{},
		&models.SeasonExpectedWins{},
	)
	if err != nil {
		panic("failed to migrate test database")
	}

	return db
}

func createSeasonTestData(db *gorm.DB) {
	// Create test league
	league := &models.League{ID: 1, Name: "Test League"}
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

	// Create completed regular season matchups
	matchups := []models.Matchup{
		{
			ID: 1, LeagueID: 1, Week: 1, Year: 2024, Season: 2024,
			HomeTeamID: 1, AwayTeamID: 2,
			HomeTeamFinalScore: 120.5, AwayTeamFinalScore: 95.0,
			Completed: true, IsPlayoff: false, GameType: "NONE", GameDate: time.Now(),
		},
		{
			ID: 2, LeagueID: 1, Week: 2, Year: 2024, Season: 2024,
			HomeTeamID: 1, AwayTeamID: 3,
			HomeTeamFinalScore: 110.0, AwayTeamFinalScore: 105.0,
			Completed: true, IsPlayoff: false, GameType: "NONE", GameDate: time.Now(),
		},
	}

	for _, matchup := range matchups {
		db.Create(&matchup)
	}

	// Create some weekly expected wins data
	weeklyData := []models.WeeklyExpectedWins{
		{TeamID: 1, Week: 2, Year: 2024, LeagueID: 1, ExpectedWins: 1.8, ActualWins: 2},
		{TeamID: 2, Week: 2, Year: 2024, LeagueID: 1, ExpectedWins: 0.3, ActualWins: 0},
		{TeamID: 3, Week: 2, Year: 2024, LeagueID: 1, ExpectedWins: 0.2, ActualWins: 0},
	}

	for _, data := range weeklyData {
		db.Create(&data)
	}
}
