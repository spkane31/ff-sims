package simulation

import (
	"backend/internal/models"
	"math"
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
	// Create a scenario where all teams play the same number of regular season games
	// but some teams also have playoff games (which should be excluded)
	matchups := []*models.Matchup{
		// Regular season games - each team plays 2 games
		createTestMatchup(1, 2, 100.0, 90.0, true),  // Team 1 vs Team 2
		createTestMatchup(3, 4, 105.0, 95.0, true),  // Team 3 vs Team 4
		createTestMatchup(1, 3, 110.0, 100.0, true), // Team 1 vs Team 3
		createTestMatchup(2, 4, 95.0, 105.0, true),  // Team 2 vs Team 4
	}
	
	// Add playoff games for teams 1 and 2 (should be excluded)
	playoffGame := createTestMatchup(1, 2, 120.0, 110.0, true)
	playoffGame.IsPlayoff = true
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
		
		// Expected wins + expected losses should equal total games
		expectedTotal := result.ExpectedWins + result.ExpectedLosses
		tolerance := 0.001
		if expectedTotal < float64(expectedTotalGames)-tolerance || expectedTotal > float64(expectedTotalGames)+tolerance {
			t.Errorf("Team %d: expected wins (%.3f) + expected losses (%.3f) = %.3f should equal %d", 
				result.TeamID, result.ExpectedWins, result.ExpectedLosses, expectedTotal, expectedTotalGames)
		}
	}
}

func TestLogisticWinProbability(t *testing.T) {
	tests := []struct {
		pointDiff float64
		expected  float64
		tolerance float64
	}{
		{0.0, 0.5, 0.001},        // Tie game should be 50%
		{16.0, 0.731, 0.001},     // 1-scale advantage ~73%
		{-16.0, 0.269, 0.001},    // 1-scale disadvantage ~27%
		{32.0, 0.881, 0.001},     // 2-scale advantage ~88%
		{-32.0, 0.119, 0.001},    // 2-scale disadvantage ~12%
		{100.0, 0.999, 0.001},    // Huge advantage ~100%
		{-100.0, 0.001, 0.001},   // Huge disadvantage ~0%
	}

	for _, test := range tests {
		result := logisticWinProbability(test.pointDiff)
		if math.Abs(result-test.expected) > test.tolerance {
			t.Errorf("logisticWinProbability(%.1f) = %.3f, expected %.3f", 
				test.pointDiff, result, test.expected)
		}
	}
}

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
	
	// Team 1 should have higher expected wins (won by 10 points)
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
	matchups := []*models.Matchup{
		createTestMatchup(1, 2, 100.0, 90.0, true),  // Team 1 wins by 10
		createTestMatchup(1, 3, 80.0, 120.0, true),  // Team 1 loses by 40
		createTestMatchup(2, 3, 95.0, 95.0, true),   // Tie game
	}
	
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
	
	// Team 1: 1 win, 1 loss
	team1 := resultMap[1]
	if team1.ActualWins != 1 {
		t.Errorf("Team 1 should have 1 actual win, got %d", team1.ActualWins)
	}
	if team1.ActualLosses != 1 {
		t.Errorf("Team 1 should have 1 actual loss, got %d", team1.ActualLosses)
	}
	if team1.TotalGames != 2 {
		t.Errorf("Team 1 should have 2 total games, got %d", team1.TotalGames)
	}
	
	// Let me recalculate the expected results:
	// Game 1: Team 1 (100) vs Team 2 (90) -> Team 1 wins
	// Game 2: Team 1 (80) vs Team 3 (120) -> Team 3 wins  
	// Game 3: Team 2 (95) vs Team 3 (95) -> Tie (no winner)
	//
	// Final records:
	// Team 1: 1 win, 1 loss
	// Team 2: 0 wins, 1 loss (ties don't count as wins or losses)
	// Team 3: 1 win, 0 losses
	
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
}

func TestCalculateExpectedWins_StrengthOfSchedule(t *testing.T) {
	// Create a scenario where we can test strength of schedule
	matchups := []*models.Matchup{
		createTestMatchup(1, 2, 100.0, 90.0, true),  // Team 1 beats Team 2
		createTestMatchup(2, 3, 100.0, 90.0, true),  // Team 2 beats Team 3
		createTestMatchup(1, 3, 100.0, 90.0, true),  // Team 1 beats Team 3
		createTestMatchup(3, 4, 90.0, 100.0, true),  // Team 4 beats Team 3
		createTestMatchup(1, 4, 100.0, 90.0, true),  // Team 1 beats Team 4
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

func TestCalculateSeasonExpectedWins_CompleteGames(t *testing.T) {
	teams := createTestTeams(4)
	
	completedMatchups := []*models.Matchup{
		createTestMatchup(1, 2, 100.0, 90.0, true),
	}
	
	// No remaining schedule
	remainingSchedule := []*models.Matchup{}
	
	results, err := CalculateSeasonExpectedWins(teams, completedMatchups, remainingSchedule)
	
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestCalculateSeasonExpectedWins_WithProjections(t *testing.T) {
	teams := createTestTeams(4)
	
	completedMatchups := []*models.Matchup{
		createTestMatchup(1, 2, 100.0, 90.0, true),
	}
	
	// Add future games with projected scores
	futureGame := createTestMatchup(3, 4, 0.0, 0.0, false)
	futureGame.HomeTeamESPNProjectedScore = 105.0
	futureGame.AwayTeamESPNProjectedScore = 95.0
	
	remainingSchedule := []*models.Matchup{futureGame}
	
	results, err := CalculateSeasonExpectedWins(teams, completedMatchups, remainingSchedule)
	
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	
	if len(results) != 4 {
		t.Errorf("Expected 4 results, got %d", len(results))
	}
	
	// Find Team 3 and Team 4 results
	resultMap := make(map[uint]ExpectedWinsResult)
	for _, result := range results {
		resultMap[result.TeamID] = result
	}
	
	team3 := resultMap[3]
	team4 := resultMap[4]
	
	// Team 3 is projected to score higher, so should have >0.5 expected wins for that game
	if team3.ExpectedWins <= 0.5 {
		t.Errorf("Team 3 should have >0.5 expected wins from projected game, got %.3f", team3.ExpectedWins)
	}
	
	// Team 4 should have <0.5 expected wins
	if team4.ExpectedWins >= 0.5 {
		t.Errorf("Team 4 should have <0.5 expected wins from projected game, got %.3f", team4.ExpectedWins)
	}
}

func TestCalculateSeasonExpectedWins_SkipCompletedInRemaining(t *testing.T) {
	teams := createTestTeams(4)
	
	completedMatchups := []*models.Matchup{}
	
	// Add a "remaining" game that's actually completed - should be skipped
	completedGame := createTestMatchup(1, 2, 100.0, 90.0, true)
	remainingSchedule := []*models.Matchup{completedGame}
	
	results, err := CalculateSeasonExpectedWins(teams, completedMatchups, remainingSchedule)
	
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	
	// Should have no results since the completed game in remaining schedule is skipped
	// and there are no actual completed games
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

// Test edge cases
func TestLogisticWinProbability_EdgeCases(t *testing.T) {
	// Test very large positive value
	result := logisticWinProbability(1000)
	if result < 0.999 {
		t.Errorf("Very large positive value should be ~1.0, got %.6f", result)
	}
	
	// Test very large negative value
	result = logisticWinProbability(-1000)
	if result > 0.001 {
		t.Errorf("Very large negative value should be ~0.0, got %.6f", result)
	}
	
	// Test exact zero
	result = logisticWinProbability(0)
	if result != 0.5 {
		t.Errorf("Zero point diff should be exactly 0.5, got %.6f", result)
	}
}

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

func BenchmarkLogisticWinProbability(b *testing.B) {
	pointDiffs := []float64{-50, -25, -10, 0, 10, 25, 50}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, diff := range pointDiffs {
			_ = logisticWinProbability(diff)
		}
	}
}