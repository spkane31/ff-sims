package simulation

import (
	"backend/internal/models"
	"os"
	"testing"
	"time"
)

// Helper function to create test matchup
func createTestMatchup(homeTeamID, awayTeamID uint, homeScore, awayScore float64, completed bool) *models.Matchup {
	return &models.Matchup{
		ID:                 1,
		Week:               1,
		Year:               2024,
		Season:             2024,
		HomeTeamID:         homeTeamID,
		AwayTeamID:         awayTeamID,
		GameDate:           time.Now(),
		GameType:           "NONE",
		HomeTeamFinalScore: homeScore,
		AwayTeamFinalScore: awayScore,
		Completed:          completed,
		IsPlayoff:          false,
		LeagueID:           1,
	}
}

func TestCalculateExpectedWins_ExcludesPlayoffGames(t *testing.T) {
	// Create regular season games
	regularSeasonGame := createTestMatchup(1, 2, 100.0, 90.0, true)

	// Create playoff games that should be excluded
	playoffGame1 := createTestMatchup(1, 2, 110.0, 95.0, true)
	playoffGame1.IsPlayoff = true

	playoffGame2 := createTestMatchup(1, 3, 105.0, 100.0, true)
	playoffGame2.GameType = "WINNERS_BRACKET" // Non-NONE game type

	matchups := []*models.Matchup{
		regularSeasonGame,
		playoffGame1,
		playoffGame2,
	}

	results, err := CalculateExpectedWins(matchups)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Should only have results for the regular season game (2 teams)
	if len(results) != 2 {
		t.Errorf("Expected 2 results (only regular season game), got %d", len(results))
	}

	// Find team 1's result
	var team1Result *ExpectedWinsResult
	for i := range results {
		if results[i].TeamID == 1 {
			team1Result = &results[i]
			break
		}
	}

	if team1Result == nil {
		t.Fatal("Could not find result for team 1")
	}

	// Team 1 should only have 1 game (the regular season game), not 3
	if team1Result.TotalGames != 1 {
		t.Errorf("Expected team 1 to have 1 total game (regular season only), got %d", team1Result.TotalGames)
	}

	// Team 1 should have 1 actual win from the regular season game
	if team1Result.ActualWins != 1 {
		t.Errorf("Expected team 1 to have 1 actual win, got %d", team1Result.ActualWins)
	}
}

func TestCalculateExpectedWins_ConsistentGameCounts(t *testing.T) {
	// Set low simulation count for faster testing
	os.Setenv("EXPECTED_WINS_SIMULATIONS", "100")
	defer os.Unsetenv("EXPECTED_WINS_SIMULATIONS")

	// Create a scenario where all teams play the same number of regular season games
	// but some teams also have playoff games (which should be excluded)
	matchups := []*models.Matchup{
		// Regular season games - each team plays 2 games, set different weeks
		createTestMatchup(1, 2, 100.0, 90.0, true),  // Team 1 vs Team 2, week 1
		createTestMatchup(3, 4, 105.0, 95.0, true),  // Team 3 vs Team 4, week 1
		createTestMatchup(1, 3, 110.0, 100.0, true), // Team 1 vs Team 3, week 2
		createTestMatchup(2, 4, 95.0, 105.0, true),  // Team 2 vs Team 4, week 2
	}

	// Set different weeks for proper simulation
	matchups[0].Week = 1
	matchups[1].Week = 1
	matchups[2].Week = 2
	matchups[3].Week = 2

	// Add playoff games for teams 1 and 2 (should be excluded)
	playoffGame := createTestMatchup(1, 2, 120.0, 110.0, true)
	playoffGame.IsPlayoff = true
	playoffGame.Week = 3
	matchups = append(matchups, playoffGame)

	results, err := CalculateExpectedWins(matchups)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if len(results) != 4 {
		t.Errorf("Expected 4 team results, got %d", len(results))
	}

	// All teams should have the same total games (2 regular season games each)
	expectedTotalGames := 2
	for _, result := range results {
		totalGames := result.ActualWins + result.ActualLosses
		if totalGames != expectedTotalGames {
			t.Errorf("Team %d should have %d total games, got %d", result.TeamID, expectedTotalGames, totalGames)
		}

		// With simulation-based approach across 2 weeks, expected wins should average to 2 total games per team
		// Allow for some variance due to simulation randomness
		expectedTotal := result.ExpectedWins + result.ExpectedLosses
		tolerance := 0.1 // More tolerance for simulation variance
		if expectedTotal < float64(expectedTotalGames)-tolerance || expectedTotal > float64(expectedTotalGames)+tolerance {
			t.Errorf("Team %d: expected wins (%.3f) + expected losses (%.3f) = %.3f should be close to %d",
				result.TeamID, result.ExpectedWins, result.ExpectedLosses, expectedTotal, expectedTotalGames)
		}
	}
}

// Removed logistic probability tests as we now use simulation-based approach

func TestCalculateExpectedWins_EmptySchedule(t *testing.T) {
	results, err := CalculateExpectedWins([]*models.Matchup{})

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected empty results, got %d results", len(results))
	}
}

func TestCalculateExpectedWins_IncompleteGames(t *testing.T) {
	matchups := []*models.Matchup{
		createTestMatchup(1, 2, 100.0, 90.0, false), // Incomplete game
	}

	results, err := CalculateExpectedWins(matchups)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected no results for incomplete games, got %d results", len(results))
	}
}

func TestCalculateExpectedWins_SingleGame(t *testing.T) {
	// Set low simulation count for consistent testing
	os.Setenv("EXPECTED_WINS_SIMULATIONS", "100")
	defer os.Unsetenv("EXPECTED_WINS_SIMULATIONS")

	matchups := []*models.Matchup{
		createTestMatchup(1, 2, 100.0, 90.0, true), // Team 1 wins by 10
	}

	results, err := CalculateExpectedWins(matchups)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	// Find results for each team
	var team1Result, team2Result *ExpectedWinsResult
	for i := range results {
		if results[i].TeamID == 1 {
			team1Result = &results[i]
		} else if results[i].TeamID == 2 {
			team2Result = &results[i]
		}
	}

	if team1Result == nil || team2Result == nil {
		t.Fatal("Could not find results for both teams")
	}

	// With simulation-based approach and only 1 week, team 1 should have higher expected wins
	// since they consistently score 100 vs team 2's 90 in random matchups
	if team1Result.ExpectedWins <= 0.5 {
		t.Errorf("Team 1 should have >0.5 expected wins, got %.3f", team1Result.ExpectedWins)
	}

	if team2Result.ExpectedWins >= 0.5 {
		t.Errorf("Team 2 should have <0.5 expected wins, got %.3f", team2Result.ExpectedWins)
	}

	// Check actual wins/losses
	if team1Result.ActualWins != 1 {
		t.Errorf("Team 1 should have 1 actual win, got %d", team1Result.ActualWins)
	}

	if team2Result.ActualLosses != 1 {
		t.Errorf("Team 2 should have 1 actual loss, got %d", team2Result.ActualLosses)
	}

	// Both teams should have 1 total game
	if team1Result.TotalGames != 1 || team2Result.TotalGames != 1 {
		t.Errorf("Both teams should have 1 total game")
	}
}

func TestCalculateExpectedWins_MultipleGames(t *testing.T) {
	// Set low simulation count for faster testing
	os.Setenv("EXPECTED_WINS_SIMULATIONS", "100")
	defer os.Unsetenv("EXPECTED_WINS_SIMULATIONS")

	matchups := []*models.Matchup{
		createTestMatchup(1, 2, 100.0, 90.0, true), // Team 1 wins by 10
		createTestMatchup(1, 3, 80.0, 120.0, true), // Team 1 loses by 40
		createTestMatchup(2, 3, 95.0, 95.0, true),  // Tie game
	}

	// Need to set different weeks for simulation to work properly
	matchups[0].Week = 1
	matchups[1].Week = 2
	matchups[2].Week = 3

	results, err := CalculateExpectedWins(matchups)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// Create map for easier access
	resultMap := make(map[uint]ExpectedWinsResult)
	for _, result := range results {
		resultMap[result.TeamID] = result
	}

	// Verify actual wins/losses from the real schedule
	// Game 1: Team 1 (100) vs Team 2 (90) -> Team 1 wins
	// Game 2: Team 1 (80) vs Team 3 (120) -> Team 3 wins
	// Game 3: Team 2 (95) vs Team 3 (95) -> Tie (no winner)

	team1 := resultMap[1]
	if team1.ActualWins != 1 {
		t.Errorf("Team 1 should have 1 actual win, got %d", team1.ActualWins)
	}
	if team1.ActualLosses != 1 {
		t.Errorf("Team 1 should have 1 actual loss, got %d", team1.ActualLosses)
	}

	team2 := resultMap[2]
	if team2.ActualWins != 0 {
		t.Errorf("Team 2 should have 0 actual wins, got %d", team2.ActualWins)
	}
	if team2.ActualLosses != 1 {
		t.Errorf("Team 2 should have 1 actual loss, got %d", team2.ActualLosses)
	}

	team3 := resultMap[3]
	if team3.ActualWins != 1 {
		t.Errorf("Team 3 should have 1 actual win, got %d", team3.ActualWins)
	}
	if team3.ActualLosses != 0 {
		t.Errorf("Team 3 should have 0 actual losses, got %d", team3.ActualLosses)
	}

	// With simulation-based expected wins, Team 3 should have highest expected wins
	// (scores 120 in week 2, 95 in week 3), Team 1 middle (100, 80), Team 2 lowest (90, doesn't play week 2)
	if team3.ExpectedWins < team1.ExpectedWins {
		t.Errorf("Team 3 should have higher expected wins than Team 1 due to better scoring, got Team3: %.3f vs Team1: %.3f",
			team3.ExpectedWins, team1.ExpectedWins)
	}
}

func TestCalculateExpectedWins_StrengthOfSchedule(t *testing.T) {
	// Create a scenario where we can test strength of schedule
	matchups := []*models.Matchup{
		createTestMatchup(1, 2, 100.0, 90.0, true), // Team 1 beats Team 2
		createTestMatchup(2, 3, 100.0, 90.0, true), // Team 2 beats Team 3
		createTestMatchup(1, 3, 100.0, 90.0, true), // Team 1 beats Team 3
		createTestMatchup(3, 4, 90.0, 100.0, true), // Team 4 beats Team 3
		createTestMatchup(1, 4, 100.0, 90.0, true), // Team 1 beats Team 4
	}

	results, err := CalculateExpectedWins(matchups)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	resultMap := make(map[uint]ExpectedWinsResult)
	for _, result := range results {
		resultMap[result.TeamID] = result
	}

	// Team 1 has played Team 2, 3, and 4 - their strength of schedule should be calculated
	team1 := resultMap[1]
	if team1.StrengthOfSchedule < 0 || team1.StrengthOfSchedule > 1 {
		t.Errorf("Team 1 strength of schedule should be between 0 and 1, got %.3f", team1.StrengthOfSchedule)
	}
}

func TestCalculateWeeklyExpectedWins_SingleWeek(t *testing.T) {
	// Set low simulation count for faster testing
	os.Setenv("EXPECTED_WINS_SIMULATIONS", "100")
	defer os.Unsetenv("EXPECTED_WINS_SIMULATIONS")

	matchups := []*models.Matchup{
		createTestMatchup(1, 2, 100.0, 90.0, true), // Team 1 wins by 10, week 1
		createTestMatchup(3, 4, 110.0, 80.0, true), // Team 3 wins by 30, week 1
	}

	// Both matchups are in week 1
	matchups[0].Week = 1
	matchups[1].Week = 1

	results, err := CalculateWeeklyExpectedWins(matchups, 1)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if len(results) != 4 {
		t.Errorf("Expected 4 results, got %d", len(results))
	}

	// Check that all weekly expected wins are between 0 and 1
	for _, result := range results {
		if result.ExpectedWins < 0.0 || result.ExpectedWins > 1.0 {
			t.Errorf("Team %d: Weekly expected wins should be between 0 and 1, got %.3f",
				result.TeamID, result.ExpectedWins)
		}

		// ExpectedWins + ExpectedLosses should equal 1 for single week
		total := result.ExpectedWins + result.ExpectedLosses
		tolerance := 0.001
		if total < 1.0-tolerance || total > 1.0+tolerance {
			t.Errorf("Team %d: Expected wins + losses should equal 1.0 for single week, got %.3f",
				result.TeamID, total)
		}
	}
}

// Removed season-related tests as CalculateSeasonExpectedWins function was removed
// in favor of the simulation-based approach

// Test edge cases
// Removed logistic probability edge case tests as we now use simulation-based approach

// Benchmark tests
func BenchmarkCalculateExpectedWins(b *testing.B) {
	// Create a realistic set of matchups
	matchups := make([]*models.Matchup, 100)
	for i := 0; i < 100; i++ {
		matchups[i] = createTestMatchup(uint(i%10+1), uint((i+1)%10+1), 100.0+float64(i%20), 95.0+float64(i%15), true)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = CalculateExpectedWins(matchups)
	}
}

// Removed logistic probability benchmark as we now use simulation-based approach
