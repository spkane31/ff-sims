package simulation

import (
	"backend/internal/models"
	"testing"
)

// Helper function to create test teams
func createTestTeams(numTeams int) []models.Team {
	teams := make([]models.Team, numTeams)
	for i := 0; i < numTeams; i++ {
		teams[i] = models.Team{
			ID:       uint(i + 1),
			Name:     "Team " + string(rune('A'+i)),
			Owner:    "Owner " + string(rune('A'+i)),
			LeagueID: 1,
		}
	}
	return teams
}

func TestNewScheduleGenerator(t *testing.T) {
	config := ScheduleConfig{
		NumTeams:       8,
		RegularWeeks:   14,
		PlayoffWeeks:   3,
		MaxGamesVsTeam: 2,
	}

	sg := NewScheduleGenerator(config)

	if sg.config.MaxGamesVsTeam != 2 {
		t.Errorf("Expected MaxGamesVsTeam to be 2, got %d", sg.config.MaxGamesVsTeam)
	}

	if sg.rand == nil {
		t.Error("Expected random generator to be initialized")
	}
}

func TestNewScheduleGeneratorDefaultMaxGames(t *testing.T) {
	config := ScheduleConfig{
		NumTeams:     8,
		RegularWeeks: 14,
		PlayoffWeeks: 3,
		// MaxGamesVsTeam not set (should default to 2)
	}

	sg := NewScheduleGenerator(config)

	if sg.config.MaxGamesVsTeam != 2 {
		t.Errorf("Expected default MaxGamesVsTeam to be 2, got %d", sg.config.MaxGamesVsTeam)
	}
}

func TestGenerateRegularSeasonSchedule_ValidInput(t *testing.T) {
	teams := createTestTeams(10)
	config := ScheduleConfig{
		NumTeams:       10,
		RegularWeeks:   14,
		PlayoffWeeks:   3,
		MaxGamesVsTeam: 2,
	}

	sg := NewScheduleGenerator(config)
	schedule, err := sg.GenerateRegularSeasonSchedule(teams, 2024, 1)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expectedGames := (10 * 14) / 2 // 70 games total
	if len(schedule) != expectedGames {
		t.Errorf("Expected %d games, got %d", expectedGames, len(schedule))
	}

	// Validate all games
	err = sg.ValidateSchedule(schedule)
	if err != nil {
		t.Errorf("Generated schedule failed validation: %v", err)
	}
}

func TestGenerateRegularSeasonSchedule_OddTeams(t *testing.T) {
	teams := createTestTeams(7) // Odd number
	config := ScheduleConfig{
		NumTeams:       7,
		RegularWeeks:   14,
		PlayoffWeeks:   3,
		MaxGamesVsTeam: 2,
	}

	sg := NewScheduleGenerator(config)
	_, err := sg.GenerateRegularSeasonSchedule(teams, 2024, 1)

	if err == nil {
		t.Error("Expected error for odd number of teams")
	}

	expectedError := "number of teams must be even for schedule generation"
	if err.Error() != expectedError {
		t.Errorf("Expected error: %s, got: %s", expectedError, err.Error())
	}
}

func TestGenerateRegularSeasonSchedule_TooFewTeams(t *testing.T) {
	teams := createTestTeams(2)
	config := ScheduleConfig{
		NumTeams:       2,
		RegularWeeks:   14,
		PlayoffWeeks:   3,
		MaxGamesVsTeam: 2,
	}

	sg := NewScheduleGenerator(config)
	_, err := sg.GenerateRegularSeasonSchedule(teams, 2024, 1)

	if err == nil {
		t.Error("Expected error for too few teams")
	}

	expectedError := "need at least 4 teams for schedule generation"
	if err.Error() != expectedError {
		t.Errorf("Expected error: %s, got: %s", expectedError, err.Error())
	}
}

func TestGenerateRegularSeasonSchedule_ImpossibleConstraints(t *testing.T) {
	teams := createTestTeams(4)
	config := ScheduleConfig{
		NumTeams:       4,
		RegularWeeks:   20, // Too many weeks for 4 teams
		PlayoffWeeks:   3,
		MaxGamesVsTeam: 2,
	}

	sg := NewScheduleGenerator(config)
	_, err := sg.GenerateRegularSeasonSchedule(teams, 2024, 1)

	if err == nil {
		t.Error("Expected error for impossible constraints")
	}

	expectedError := "impossible to create schedule with given constraints"
	if err.Error() != expectedError {
		t.Errorf("Expected error: %s, got: %s", expectedError, err.Error())
	}
}

func TestCanTeamsPlay(t *testing.T) {
	config := ScheduleConfig{MaxGamesVsTeam: 2}
	sg := NewScheduleGenerator(config)

	gamesPlayed := make(map[[2]uint]int)
	lastOpponent := make(map[uint]uint)

	// Test: teams can play initially
	if !sg.canTeamsPlay(1, 2, gamesPlayed, lastOpponent) {
		t.Error("Teams should be able to play initially")
	}

	// Test: teams can't play if they've reached max games
	key := sg.makeTeamPairKey(1, 2)
	gamesPlayed[key] = 2

	if sg.canTeamsPlay(1, 2, gamesPlayed, lastOpponent) {
		t.Error("Teams should not be able to play after reaching max games")
	}

	// Reset for next test
	gamesPlayed[key] = 1

	// Test: teams can't play back-to-back
	lastOpponent[1] = 2
	lastOpponent[2] = 1

	if sg.canTeamsPlay(1, 2, gamesPlayed, lastOpponent) {
		t.Error("Teams should not be able to play back-to-back")
	}
}

func TestMakeTeamPairKey(t *testing.T) {
	config := ScheduleConfig{}
	sg := NewScheduleGenerator(config)

	// Test: key should be consistent regardless of order
	key1 := sg.makeTeamPairKey(1, 2)
	key2 := sg.makeTeamPairKey(2, 1)

	if key1 != key2 {
		t.Errorf("Keys should be identical: %v vs %v", key1, key2)
	}

	// Test: smaller ID should be first
	expected := [2]uint{1, 2}
	if key1 != expected {
		t.Errorf("Expected key %v, got %v", expected, key1)
	}
}

func TestValidateSchedule_Empty(t *testing.T) {
	config := ScheduleConfig{}
	sg := NewScheduleGenerator(config)

	var schedule []models.Matchup
	err := sg.ValidateSchedule(schedule)

	if err == nil {
		t.Error("Expected error for empty schedule")
	}

	expectedError := "empty schedule"
	if err.Error() != expectedError {
		t.Errorf("Expected error: %s, got: %s", expectedError, err.Error())
	}
}

func TestValidateSchedule_TooManyGames(t *testing.T) {
	config := ScheduleConfig{MaxGamesVsTeam: 1}
	sg := NewScheduleGenerator(config)

	// Create schedule where teams play more than once
	schedule := []models.Matchup{
		{
			HomeTeamID: 1,
			AwayTeamID: 2,
			Week:       1,
		},
		{
			HomeTeamID: 1,
			AwayTeamID: 2,
			Week:       2,
		},
	}

	err := sg.ValidateSchedule(schedule)

	if err == nil {
		t.Error("Expected error for too many games between teams")
	}

	expectedError := "teams played more than maximum allowed games"
	if err.Error() != expectedError {
		t.Errorf("Expected error: %s, got: %s", expectedError, err.Error())
	}
}

func TestValidateSchedule_BackToBackGames(t *testing.T) {
	config := ScheduleConfig{
		MaxGamesVsTeam: 2,
		RegularWeeks:   3,
	}
	sg := NewScheduleGenerator(config)

	// Create schedule with back-to-back games
	schedule := []models.Matchup{
		{
			HomeTeamID: 1,
			AwayTeamID: 2,
			Week:       1,
		},
		{
			HomeTeamID: 1,
			AwayTeamID: 2,
			Week:       2,
		},
	}

	err := sg.ValidateSchedule(schedule)

	if err == nil {
		t.Error("Expected error for back-to-back games")
	}

	expectedError := "team has back-to-back games against same opponent"
	if err.Error() != expectedError {
		t.Errorf("Expected error: %s, got: %s", expectedError, err.Error())
	}
}

func TestValidateSchedule_ValidSchedule(t *testing.T) {
	config := ScheduleConfig{
		MaxGamesVsTeam: 2,
		RegularWeeks:   4,
	}
	sg := NewScheduleGenerator(config)

	// Create valid schedule
	schedule := []models.Matchup{
		{HomeTeamID: 1, AwayTeamID: 2, Week: 1},
		{HomeTeamID: 3, AwayTeamID: 4, Week: 1},
		{HomeTeamID: 1, AwayTeamID: 3, Week: 2},
		{HomeTeamID: 2, AwayTeamID: 4, Week: 2},
		{HomeTeamID: 1, AwayTeamID: 4, Week: 3},
		{HomeTeamID: 2, AwayTeamID: 3, Week: 3},
		{HomeTeamID: 1, AwayTeamID: 2, Week: 4}, // Second game between 1 and 2
		{HomeTeamID: 3, AwayTeamID: 4, Week: 4}, // Second game between 3 and 4
	}

	err := sg.ValidateSchedule(schedule)

	if err != nil {
		t.Errorf("Valid schedule should not have errors: %v", err)
	}
}

// Integration test: Generate and validate schedules for different team sizes
func TestIntegration_MultipleTeamSizes(t *testing.T) {
	// Use team sizes and week combinations that are mathematically feasible
	testCases := []struct {
		numTeams int
		weeks    int
	}{
		{8, 12},  // 8 teams, 12 weeks - feasible with max 2 games per opponent
		{10, 14}, // 10 teams, 14 weeks - feasible
		{12, 14}, // 12 teams, 14 weeks - feasible
	}

	for _, tc := range testCases {
		t.Run("teams_"+string(rune('0'+tc.numTeams/10))+string(rune('0'+tc.numTeams%10)), func(t *testing.T) {
			teams := createTestTeams(tc.numTeams)
			config := ScheduleConfig{
				NumTeams:       tc.numTeams,
				RegularWeeks:   tc.weeks,
				PlayoffWeeks:   3,
				MaxGamesVsTeam: 2,
			}

			sg := NewScheduleGenerator(config)
			schedule, err := sg.GenerateRegularSeasonSchedule(teams, 2024, 1)

			if err != nil {
				t.Fatalf("Failed to generate schedule for %d teams: %v", tc.numTeams, err)
			}

			// Validate the generated schedule
			err = sg.ValidateSchedule(schedule)
			if err != nil {
				t.Errorf("Generated schedule for %d teams failed validation: %v", tc.numTeams, err)
			}

			// Check that each team plays exactly once per week
			weekTeamCount := make(map[uint]map[uint]int)
			for _, match := range schedule {
				if weekTeamCount[match.Week] == nil {
					weekTeamCount[match.Week] = make(map[uint]int)
				}
				weekTeamCount[match.Week][match.HomeTeamID]++
				weekTeamCount[match.Week][match.AwayTeamID]++
			}

			for week, teamCounts := range weekTeamCount {
				for teamID, count := range teamCounts {
					if count != 1 {
						t.Errorf("Team %d plays %d times in week %d, expected 1", teamID, count, week)
					}
				}
			}
		})
	}
}

// Benchmark schedule generation
func BenchmarkGenerateRegularSeasonSchedule8Teams(b *testing.B) {
	teams := createTestTeams(8)
	config := ScheduleConfig{
		NumTeams:       8,
		RegularWeeks:   14,
		PlayoffWeeks:   3,
		MaxGamesVsTeam: 2,
	}
	sg := NewScheduleGenerator(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sg.GenerateRegularSeasonSchedule(teams, 2024, 1)
	}
}

func BenchmarkGenerateRegularSeasonSchedule12Teams(b *testing.B) {
	teams := createTestTeams(12)
	config := ScheduleConfig{
		NumTeams:       12,
		RegularWeeks:   14,
		PlayoffWeeks:   3,
		MaxGamesVsTeam: 2,
	}
	sg := NewScheduleGenerator(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sg.GenerateRegularSeasonSchedule(teams, 2024, 1)
	}
}

// Helper function to create test standings
func createTestStandings(numTeams int) []TeamStanding {
	standings := make([]TeamStanding, numTeams)
	for i := 0; i < numTeams; i++ {
		standings[i] = TeamStanding{
			TeamID:      uint(i + 1),
			Wins:        numTeams - i - 1, // Better teams have more wins
			Losses:      i,
			Ties:        0,
			Points:      float64(1000 - i*50), // Better teams have more points
			PlayoffSeed: i + 1,
		}
	}
	return standings
}

func TestGeneratePlayoffSchedule_ValidInput(t *testing.T) {
	teams := createTestTeams(8)
	standings := createTestStandings(8)
	config := ScheduleConfig{
		NumTeams:       8,
		RegularWeeks:   14,
		PlayoffWeeks:   3,
		MaxGamesVsTeam: 2,
	}

	sg := NewScheduleGenerator(config)
	playoffSchedule, err := sg.GeneratePlayoffSchedule(teams, standings, 2024, 1, 15)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should have 5 playoff games total (2 wildcard + 2 semifinals + 1 championship)
	expectedGames := 5
	if len(playoffSchedule) != expectedGames {
		t.Errorf("Expected %d playoff games, got %d", expectedGames, len(playoffSchedule))
	}

	// Verify wildcard games
	wildcardGames := 0
	semifinalGames := 0
	championshipGames := 0

	for _, game := range playoffSchedule {
		if !game.IsPlayoff {
			t.Error("All playoff games should have IsPlayoff=true")
		}

		switch game.GameType {
		case "wildcard":
			wildcardGames++
			if game.Week != 15 {
				t.Errorf("Wildcard game should be week 15, got week %d", game.Week)
			}
		case "semifinal":
			semifinalGames++
			if game.Week != 16 {
				t.Errorf("Semifinal game should be week 16, got week %d", game.Week)
			}
		case "championship":
			championshipGames++
			if game.Week != 17 {
				t.Errorf("Championship game should be week 17, got week %d", game.Week)
			}
		default:
			t.Errorf("Unknown game type: %s", game.GameType)
		}
	}

	if wildcardGames != 2 {
		t.Errorf("Expected 2 wildcard games, got %d", wildcardGames)
	}
	if semifinalGames != 2 {
		t.Errorf("Expected 2 semifinal games, got %d", semifinalGames)
	}
	if championshipGames != 1 {
		t.Errorf("Expected 1 championship game, got %d", championshipGames)
	}
}

func TestGeneratePlayoffSchedule_ValidMatchups(t *testing.T) {
	teams := createTestTeams(8)
	standings := createTestStandings(8)
	config := ScheduleConfig{}

	sg := NewScheduleGenerator(config)
	playoffSchedule, err := sg.GeneratePlayoffSchedule(teams, standings, 2024, 1, 15)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Check wildcard matchups (3v6 and 4v5)
	wildcardFound := make(map[string]bool)
	for _, game := range playoffSchedule {
		if game.GameType == "wildcard" {
			if (game.HomeTeamID == 3 && game.AwayTeamID == 6) ||
				(game.HomeTeamID == 6 && game.AwayTeamID == 3) {
				wildcardFound["3v6"] = true
			}
			if (game.HomeTeamID == 4 && game.AwayTeamID == 5) ||
				(game.HomeTeamID == 5 && game.AwayTeamID == 4) {
				wildcardFound["4v5"] = true
			}
		}
	}

	if !wildcardFound["3v6"] {
		t.Error("Expected 3v6 wildcard matchup not found")
	}
	if !wildcardFound["4v5"] {
		t.Error("Expected 4v5 wildcard matchup not found")
	}

	// Check that 1st and 2nd seeds get byes in wildcard round
	for _, game := range playoffSchedule {
		if game.GameType == "wildcard" {
			if game.HomeTeamID == 1 || game.AwayTeamID == 1 {
				t.Error("1st seed should not play in wildcard round")
			}
			if game.HomeTeamID == 2 || game.AwayTeamID == 2 {
				t.Error("2nd seed should not play in wildcard round")
			}
		}
	}
}

func TestGeneratePlayoffSchedule_TooFewTeams(t *testing.T) {
	teams := createTestTeams(4)
	standings := createTestStandings(4) // Only 4 teams
	config := ScheduleConfig{}

	sg := NewScheduleGenerator(config)
	_, err := sg.GeneratePlayoffSchedule(teams, standings, 2024, 1, 15)

	if err == nil {
		t.Error("Expected error for too few teams")
	}

	expectedError := "need at least 6 teams for playoffs"
	if err.Error() != expectedError {
		t.Errorf("Expected error: %s, got: %s", expectedError, err.Error())
	}
}

func TestCreatePlayoffMatchup(t *testing.T) {
	config := ScheduleConfig{}
	sg := NewScheduleGenerator(config)

	matchup := sg.createPlayoffMatchup(1, 2, 15, 2024, 1, "wildcard")

	if matchup.HomeTeamID != 1 {
		t.Errorf("Expected home team ID 1, got %d", matchup.HomeTeamID)
	}
	if matchup.AwayTeamID != 2 {
		t.Errorf("Expected away team ID 2, got %d", matchup.AwayTeamID)
	}
	if matchup.Week != 15 {
		t.Errorf("Expected week 15, got %d", matchup.Week)
	}
	if matchup.Year != 2024 {
		t.Errorf("Expected year 2024, got %d", matchup.Year)
	}
	if matchup.LeagueID != 1 {
		t.Errorf("Expected league ID 1, got %d", matchup.LeagueID)
	}
	if matchup.GameType != "wildcard" {
		t.Errorf("Expected game type 'wildcard', got '%s'", matchup.GameType)
	}
	if !matchup.IsPlayoff {
		t.Error("Expected IsPlayoff to be true")
	}
	if matchup.Completed {
		t.Error("Expected Completed to be false")
	}
}
