package simulation

import (
	"backend/internal/models"
	"fmt"
	"os"
	"strconv"
)

// Expected Wins Calculation - Schedule-Based Monte Carlo Approach
//
// This package calculates "expected wins" - how many wins a team would have earned
// if they played their actual scores against all possible opponent schedules.
//
// APPROACH:
// Uses Monte Carlo simulation with complete valid season schedules (via SeasonSimulator).
// For each of N simulations (default 10,000):
//   1. Generate a complete valid schedule (respecting constraints)
//   2. Apply actual team scores to the simulated schedule
//   3. Count wins for each team
//   4. Average results across all simulations
//
// WHY SCHEDULE-BASED (vs random pairings):
// - Old: Randomly paired teams each week, allowing unrealistic scenarios
// - New: Generates complete schedules with real constraints (max 2 games per opponent, no back-to-back)
// - Result: More realistic expected wins that account for schedule fairness
//
// EXAMPLE SCENARIO:
// Team A (always 2nd-lowest scorer) vs Team B (always lowest scorer)
// - Old approach: Team A could "play" Team B every week, getting credit for all those wins
// - New approach: Valid schedule means Team A only plays Team B 1-2 times, more realistic
//
// CONFIGURATION:
// Set EXPECTED_WINS_SIMULATIONS env var to override default (10,000)
// Higher = more accurate but slower, lower = faster but more variance
//
// INTEGRATION:
// Called automatically by ETL pipeline after matchup scores are loaded.
// Results stored in Matchup.HomeTeamExpectedWin and Matchup.AwayTeamExpectedWin fields.

// ExpectedWinsConfig holds configuration for expected wins calculations
type ExpectedWinsConfig struct {
	NumSimulations int
}

// GetExpectedWinsConfig returns configuration with defaults
func GetExpectedWinsConfig() ExpectedWinsConfig {
	config := ExpectedWinsConfig{
		NumSimulations: 10000, // Default to 10,000 simulations
	}

	// Allow override via environment variable
	if envSims := os.Getenv("EXPECTED_WINS_SIMULATIONS"); envSims != "" {
		if sims, err := strconv.Atoi(envSims); err == nil && sims > 0 {
			config.NumSimulations = sims
		}
	}

	return config
}

// ExpectedWinsResult contains expected wins calculation for a team
type ExpectedWinsResult struct {
	TeamID             uint    `json:"team_id"`
	ExpectedWins       float64 `json:"expected_wins"`
	ExpectedLosses     float64 `json:"expected_losses"`
	ActualWins         int     `json:"actual_wins"`
	ActualLosses       int     `json:"actual_losses"`
	TotalGames         int     `json:"total_games"`
	StrengthOfSchedule float64 `json:"strength_of_schedule"`
}

// MatchupExpectedWins contains expected win probabilities for both teams in a matchup
type MatchupExpectedWins struct {
	HomeExpectedWin float64 `json:"home_expected_win"`
	AwayExpectedWin float64 `json:"away_expected_win"`
}

// CalculateExpectedWins calculates expected wins using hypothetical schedule simulations
// This approach generates thousands of random schedules using actual team scores
// and averages the win totals to determine true expected performance
func CalculateExpectedWins(schedule []*models.Matchup) ([]ExpectedWinsResult, error) {
	if len(schedule) == 0 {
		return []ExpectedWinsResult{}, nil
	}

	// Extract team weekly scores from actual matchups
	teamWeeklyScores, weeks, err := extractTeamWeeklyScores(schedule)
	if err != nil {
		return nil, fmt.Errorf("failed to extract team scores: %w", err)
	}
	if len(teamWeeklyScores) == 0 {
		return []ExpectedWinsResult{}, nil
	}

	// Get team IDs
	teamIDs := make([]uint, 0, len(teamWeeklyScores))
	for teamID := range teamWeeklyScores {
		teamIDs = append(teamIDs, teamID)
	}

	// Calculate actual wins/losses for comparison
	actualStats := calculateActualStats(schedule)

	// Run simulations with random schedules
	config := GetExpectedWinsConfig()
	simulationResults := runScheduleSimulations(teamWeeklyScores, teamIDs, weeks, config.NumSimulations)

	// Calculate strength of schedule
	strengthOfSchedule := calculateStrengthOfSchedule(schedule, actualStats)

	// Combine results
	results := make([]ExpectedWinsResult, 0, len(teamIDs))
	for _, teamID := range teamIDs {
		expectedWins := simulationResults[teamID] / float64(config.NumSimulations)
		actualData := actualStats[teamID]

		results = append(results, ExpectedWinsResult{
			TeamID:             teamID,
			ExpectedWins:       expectedWins,
			ExpectedLosses:     float64(len(weeks)) - expectedWins,
			ActualWins:         actualData.ActualWins,
			ActualLosses:       actualData.ActualLosses,
			TotalGames:         actualData.TotalGames,
			StrengthOfSchedule: strengthOfSchedule[teamID],
		})
	}

	return results, nil
}

// CalculateWeeklyExpectedWins calculates expected wins for a specific week only
func CalculateWeeklyExpectedWins(schedule []*models.Matchup, targetWeek uint) ([]ExpectedWinsResult, error) {
	if len(schedule) == 0 {
		return []ExpectedWinsResult{}, nil
	}

	// Filter to only the target week
	weekMatchups := make([]*models.Matchup, 0)
	for _, matchup := range schedule {
		if !matchup.Completed {
			continue
		}

		// Only include regular season games for target week
		if matchup.IsPlayoff || matchup.GameType != "NONE" || matchup.Week != targetWeek {
			continue
		}

		weekMatchups = append(weekMatchups, matchup)
	}

	if len(weekMatchups) == 0 {
		return []ExpectedWinsResult{}, nil
	}

	// Extract team weekly scores for just this week
	// Note: For a single week, season validation is less critical but still good practice
	_, _, err := extractTeamWeeklyScores(weekMatchups)
	if err != nil {
		return nil, fmt.Errorf("failed to extract team scores for week %d: %w", targetWeek, err)
	}

	teamWeeklyScores := make(map[uint]map[uint]float64)
	for _, matchup := range weekMatchups {
		if teamWeeklyScores[matchup.HomeTeamID] == nil {
			teamWeeklyScores[matchup.HomeTeamID] = make(map[uint]float64)
		}
		if teamWeeklyScores[matchup.AwayTeamID] == nil {
			teamWeeklyScores[matchup.AwayTeamID] = make(map[uint]float64)
		}

		teamWeeklyScores[matchup.HomeTeamID][targetWeek] = matchup.HomeTeamFinalScore
		teamWeeklyScores[matchup.AwayTeamID][targetWeek] = matchup.AwayTeamFinalScore
	}

	// Get team IDs
	teamIDs := make([]uint, 0, len(teamWeeklyScores))
	for teamID := range teamWeeklyScores {
		teamIDs = append(teamIDs, teamID)
	}

	if len(teamIDs) == 0 {
		return []ExpectedWinsResult{}, nil
	}

	// Calculate actual stats for this week only
	actualStats := calculateActualStats(weekMatchups)

	// Run simulations for just this week
	config := GetExpectedWinsConfig()
	weeks := []uint{targetWeek}
	simulationResults := runScheduleSimulations(teamWeeklyScores, teamIDs, weeks, config.NumSimulations)

	// Calculate strength of schedule for this week
	strengthOfSchedule := calculateStrengthOfSchedule(weekMatchups, actualStats)

	// Combine results
	results := make([]ExpectedWinsResult, 0, len(teamIDs))
	for _, teamID := range teamIDs {
		expectedWins := simulationResults[teamID] / float64(config.NumSimulations)
		actualData := actualStats[teamID]

		results = append(results, ExpectedWinsResult{
			TeamID:             teamID,
			ExpectedWins:       expectedWins,       // For single week, this should be 0-1
			ExpectedLosses:     1.0 - expectedWins, // For single week
			ActualWins:         actualData.ActualWins,
			ActualLosses:       actualData.ActualLosses,
			TotalGames:         actualData.TotalGames,
			StrengthOfSchedule: strengthOfSchedule[teamID],
		})
	}

	return results, nil
}

// extractTeamWeeklyScores extracts actual scores for each team by week
// Returns an error if matchups contain multiple seasons to prevent cross-season contamination
func extractTeamWeeklyScores(schedule []*models.Matchup) (map[uint]map[uint]float64, []uint, error) {
	teamWeeklyScores := make(map[uint]map[uint]float64)
	weeksSet := make(map[uint]bool)

	// Validate all matchups are from the same season
	// This prevents cross-season data mixing that would inflate expected wins
	var firstSeason uint
	seasonSet := false
	for _, matchup := range schedule {
		if !matchup.Completed {
			continue
		}
		if matchup.IsPlayoff || matchup.GameType != "NONE" {
			continue
		}

		if !seasonSet {
			firstSeason = matchup.Season
			seasonSet = true
		} else if matchup.Season != firstSeason {
			return nil, nil, fmt.Errorf("matchups contain multiple seasons: %d and %d (this would cause inflated expected wins)", firstSeason, matchup.Season)
		}
	}

	for _, matchup := range schedule {
		if !matchup.Completed {
			continue
		}

		// Only include regular season games
		if matchup.IsPlayoff || matchup.GameType != "NONE" {
			continue
		}

		// Initialize team score maps
		if teamWeeklyScores[matchup.HomeTeamID] == nil {
			teamWeeklyScores[matchup.HomeTeamID] = make(map[uint]float64)
		}
		if teamWeeklyScores[matchup.AwayTeamID] == nil {
			teamWeeklyScores[matchup.AwayTeamID] = make(map[uint]float64)
		}

		// Record scores
		teamWeeklyScores[matchup.HomeTeamID][matchup.Week] = matchup.HomeTeamFinalScore
		teamWeeklyScores[matchup.AwayTeamID][matchup.Week] = matchup.AwayTeamFinalScore
		weeksSet[matchup.Week] = true
	}

	// Convert weeks set to sorted slice
	weeks := make([]uint, 0, len(weeksSet))
	for week := range weeksSet {
		weeks = append(weeks, week)
	}

	// Sort weeks
	for i := 0; i < len(weeks); i++ {
		for j := i + 1; j < len(weeks); j++ {
			if weeks[i] > weeks[j] {
				weeks[i], weeks[j] = weeks[j], weeks[i]
			}
		}
	}

	return teamWeeklyScores, weeks, nil
}

// calculateActualStats calculates actual wins/losses from the real schedule
func calculateActualStats(schedule []*models.Matchup) map[uint]struct {
	ActualWins   int
	ActualLosses int
	TotalGames   int
} {
	stats := make(map[uint]struct {
		ActualWins   int
		ActualLosses int
		TotalGames   int
	})

	for _, matchup := range schedule {
		if !matchup.Completed {
			continue
		}

		// Only include regular season games
		if matchup.IsPlayoff || matchup.GameType != "NONE" {
			continue
		}

		homeStats := stats[matchup.HomeTeamID]
		awayStats := stats[matchup.AwayTeamID]

		if matchup.HomeTeamFinalScore > matchup.AwayTeamFinalScore {
			homeStats.ActualWins++
			awayStats.ActualLosses++
		} else if matchup.AwayTeamFinalScore > matchup.HomeTeamFinalScore {
			awayStats.ActualWins++
			homeStats.ActualLosses++
		}

		homeStats.TotalGames++
		awayStats.TotalGames++

		stats[matchup.HomeTeamID] = homeStats
		stats[matchup.AwayTeamID] = awayStats
	}

	return stats
}

// runScheduleSimulations runs thousands of simulations with valid generated schedules
// This replaces the old random pairing approach with complete schedule generation
func runScheduleSimulations(teamWeeklyScores map[uint]map[uint]float64, teamIDs []uint, weeks []uint, numSimulations int) map[uint]float64 {
	results := make(map[uint]float64)

	// Create SeasonSimulator for generating valid schedules
	// Use dummy leagueID and year since they don't affect schedule generation logic
	simulator := NewSeasonSimulator(teamIDs, len(weeks), 1, 2024)

	successfulSimulations := 0

	for sim := 0; sim < numSimulations; sim++ {
		// Generate a complete valid schedule
		schedule, err := simulator.GenerateSchedule()
		if err != nil {
			// Schedule generation failed, skip this simulation
			// This can happen with tight constraints or odd number of teams
			continue
		}

		// Apply actual team scores to the generated schedule
		scheduleWins := applyScoresToSchedule(schedule, teamWeeklyScores, weeks)

		// Accumulate wins for each team
		for teamID, wins := range scheduleWins {
			results[teamID] += float64(wins)
		}

		successfulSimulations++
	}

	// Normalize by successful simulations (not total attempts)
	// This ensures probabilities are correct even if some simulations failed
	if successfulSimulations > 0 && successfulSimulations < numSimulations {
		normalizationFactor := float64(numSimulations) / float64(successfulSimulations)
		for teamID := range results {
			results[teamID] *= normalizationFactor
		}
	}

	return results
}


// calculateStrengthOfSchedule calculates opponent strength for each team
// This now includes both completed and future games to give full season SOS
func calculateStrengthOfSchedule(schedule []*models.Matchup, actualStats map[uint]struct {
	ActualWins   int
	ActualLosses int
	TotalGames   int
}) map[uint]float64 {
	strengthOfSchedule := make(map[uint]float64)
	opponentCounts := make(map[uint]int)

	for _, matchup := range schedule {
		// Only include regular season games (both completed and future)
		if matchup.IsPlayoff || matchup.GameType != "NONE" {
			continue
		}

		// Calculate opponent win rates based on completed games
		// For future games, we still use the opponent's current win rate
		homeOpponentStats := actualStats[matchup.AwayTeamID]
		awayOpponentStats := actualStats[matchup.HomeTeamID]

		// For home team: add away team's win rate
		if homeOpponentStats.TotalGames > 0 {
			opponentWinRate := float64(homeOpponentStats.ActualWins) / float64(homeOpponentStats.TotalGames)
			strengthOfSchedule[matchup.HomeTeamID] += opponentWinRate
			opponentCounts[matchup.HomeTeamID]++
		} else if !matchup.Completed {
			// Future game against team with no completed games yet
			// Count the game but use 0.5 as neutral win rate
			strengthOfSchedule[matchup.HomeTeamID] += 0.5
			opponentCounts[matchup.HomeTeamID]++
		}

		// For away team: add home team's win rate
		if awayOpponentStats.TotalGames > 0 {
			opponentWinRate := float64(awayOpponentStats.ActualWins) / float64(awayOpponentStats.TotalGames)
			strengthOfSchedule[matchup.AwayTeamID] += opponentWinRate
			opponentCounts[matchup.AwayTeamID]++
		} else if !matchup.Completed {
			// Future game against team with no completed games yet
			// Count the game but use 0.5 as neutral win rate
			strengthOfSchedule[matchup.AwayTeamID] += 0.5
			opponentCounts[matchup.AwayTeamID]++
		}
	}

	// Average the opponent win rates
	for teamID, totalStrength := range strengthOfSchedule {
		if opponentCounts[teamID] > 0 {
			strengthOfSchedule[teamID] = totalStrength / float64(opponentCounts[teamID])
		}
	}

	return strengthOfSchedule
}

// CalculateMatchupExpectedWins calculates expected win probability for each matchup
// Returns a map of matchup ID -> MatchupExpectedWins containing both home and away probabilities (0-1)
//
// This works by:
// 1. Running Monte Carlo simulation to get per-team-per-week expected wins
// 2. For each matchup, extracting both the home and away teams' weekly expected win values
//
// Note: The home and away expected wins don't have to sum to 1.0 because they're
// calculated independently based on each team's strength against random opponents.
func CalculateMatchupExpectedWins(schedule []*models.Matchup) (map[uint]MatchupExpectedWins, error) {
	if len(schedule) == 0 {
		return map[uint]MatchupExpectedWins{}, nil
	}

	// Group matchups by week for per-week calculations
	matchupsByWeek := make(map[uint][]*models.Matchup)
	allMatchups := make([]*models.Matchup, 0)

	for _, matchup := range schedule {
		if !matchup.Completed {
			continue
		}

		// Only include regular season games
		if matchup.IsPlayoff || matchup.GameType != "NONE" {
			continue
		}

		matchupsByWeek[matchup.Week] = append(matchupsByWeek[matchup.Week], matchup)
		allMatchups = append(allMatchups, matchup)
	}

	if len(allMatchups) == 0 {
		return map[uint]MatchupExpectedWins{}, nil
	}

	// Extract team weekly scores
	teamWeeklyScores, weeks, err := extractTeamWeeklyScores(allMatchups)
	if err != nil {
		return nil, fmt.Errorf("failed to extract team scores for matchup expected wins: %w", err)
	}
	if len(teamWeeklyScores) == 0 {
		return map[uint]MatchupExpectedWins{}, nil
	}

	// Get team IDs
	teamIDs := make([]uint, 0, len(teamWeeklyScores))
	for teamID := range teamWeeklyScores {
		teamIDs = append(teamIDs, teamID)
	}

	// Calculate per-team per-week expected wins using Monte Carlo simulation
	config := GetExpectedWinsConfig()
	teamWeeklyExpectedWins := calculateTeamWeeklyExpectedWins(teamWeeklyScores, teamIDs, weeks, config.NumSimulations)

	// Assign expected wins to matchups based on team-week values
	matchupExpectedWins := make(map[uint]MatchupExpectedWins)

	for _, matchup := range allMatchups {
		// Get both teams' expected win values for this week
		homeExpectedWin := teamWeeklyExpectedWins[matchup.HomeTeamID][matchup.Week]
		awayExpectedWin := teamWeeklyExpectedWins[matchup.AwayTeamID][matchup.Week]

		matchupExpectedWins[matchup.ID] = MatchupExpectedWins{
			HomeExpectedWin: homeExpectedWin,
			AwayExpectedWin: awayExpectedWin,
		}
	}

	return matchupExpectedWins, nil
}

// applyScoresToSchedule applies actual team scores to a simulated schedule and counts wins
// Returns a map of teamID -> win count for this schedule
func applyScoresToSchedule(
	schedule *SimulatedSchedule,
	teamWeeklyScores map[uint]map[uint]float64,
	weeks []uint,
) map[uint]int {
	wins := make(map[uint]int)

	for _, matchup := range schedule.Matchups {
		// Only count weeks we have scores for
		if !weekInSlice(matchup.Week, weeks) {
			continue
		}

		homeScore, homeHasScore := teamWeeklyScores[matchup.HomeTeamID][matchup.Week]
		awayScore, awayHasScore := teamWeeklyScores[matchup.AwayTeamID][matchup.Week]

		if !homeHasScore || !awayHasScore {
			continue
		}

		if homeScore > awayScore {
			wins[matchup.HomeTeamID]++
		} else if awayScore > homeScore {
			wins[matchup.AwayTeamID]++
		}
		// Ties: no wins for either team
	}

	return wins
}

// weekInSlice checks if a week is in the slice of weeks
func weekInSlice(week uint, weeks []uint) bool {
	for _, w := range weeks {
		if w == week {
			return true
		}
	}
	return false
}

// calculateTeamWeeklyExpectedWins calculates expected wins for each team per week
// Returns map[teamID][week]expectedWin (0-1 value per week)
// Uses complete schedule generation instead of random weekly pairings
func calculateTeamWeeklyExpectedWins(
	teamWeeklyScores map[uint]map[uint]float64,
	teamIDs []uint,
	weeks []uint,
	numSimulations int,
) map[uint]map[uint]float64 {
	// Initialize result map
	teamWeeklyExpectedWins := make(map[uint]map[uint]float64)
	for _, teamID := range teamIDs {
		teamWeeklyExpectedWins[teamID] = make(map[uint]float64)
		for _, week := range weeks {
			teamWeeklyExpectedWins[teamID][week] = 0.0
		}
	}

	// Create SeasonSimulator for generating valid schedules
	simulator := NewSeasonSimulator(teamIDs, len(weeks), 1, 2024)

	successfulSimulations := 0

	for sim := 0; sim < numSimulations; sim++ {
		// Generate a complete valid schedule
		schedule, err := simulator.GenerateSchedule()
		if err != nil {
			// Schedule generation failed, skip this simulation
			continue
		}

		// For each matchup in the schedule, track wins by week
		for _, matchup := range schedule.Matchups {
			if !weekInSlice(matchup.Week, weeks) {
				continue
			}

			homeScore, homeHasScore := teamWeeklyScores[matchup.HomeTeamID][matchup.Week]
			awayScore, awayHasScore := teamWeeklyScores[matchup.AwayTeamID][matchup.Week]

			if homeHasScore && awayHasScore {
				if homeScore > awayScore {
					teamWeeklyExpectedWins[matchup.HomeTeamID][matchup.Week] += 1.0
				} else if awayScore > homeScore {
					teamWeeklyExpectedWins[matchup.AwayTeamID][matchup.Week] += 1.0
				}
				// Ties don't award wins to either team
			}
		}

		successfulSimulations++
	}

	// Convert counts to probabilities (wins / successful simulations)
	if successfulSimulations > 0 {
		for teamID := range teamWeeklyExpectedWins {
			for week := range teamWeeklyExpectedWins[teamID] {
				teamWeeklyExpectedWins[teamID][week] /= float64(successfulSimulations)
			}
		}
	}

	return teamWeeklyExpectedWins
}
