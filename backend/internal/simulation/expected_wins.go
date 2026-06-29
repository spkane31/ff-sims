package simulation

import (
	"backend/internal/models"
	"math/rand"
	"os"
	"strconv"
	"time"
)

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

// CalculateExpectedWins calculates expected wins using hypothetical schedule simulations
// This approach generates thousands of random schedules using actual team scores
// and averages the win totals to determine true expected performance
func CalculateExpectedWins(schedule []*models.Matchup) ([]ExpectedWinsResult, error) {
	if len(schedule) == 0 {
		return []ExpectedWinsResult{}, nil
	}

	// Extract team weekly scores from actual matchups
	teamWeeklyScores, weeks := extractTeamWeeklyScores(schedule)
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
func extractTeamWeeklyScores(schedule []*models.Matchup) (map[uint]map[uint]float64, []uint) {
	teamWeeklyScores := make(map[uint]map[uint]float64)
	weeksSet := make(map[uint]bool)

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

	return teamWeeklyScores, weeks
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

// runScheduleSimulations runs thousands of simulations with randomized schedules
func runScheduleSimulations(teamWeeklyScores map[uint]map[uint]float64, teamIDs []uint, weeks []uint, numSimulations int) map[uint]float64 {
	results := make(map[uint]float64)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for sim := 0; sim < numSimulations; sim++ {
		// Generate random schedule for this simulation
		scheduleWins := simulateRandomSchedule(teamWeeklyScores, teamIDs, weeks, rng)

		// Accumulate wins for each team
		for teamID, wins := range scheduleWins {
			results[teamID] += float64(wins)
		}
	}

	return results
}

// simulateRandomSchedule generates one random schedule and calculates wins
func simulateRandomSchedule(teamWeeklyScores map[uint]map[uint]float64, teamIDs []uint, weeks []uint, rng *rand.Rand) map[uint]int {
	wins := make(map[uint]int)

	// Ensure even number of teams
	if len(teamIDs)%2 != 0 {
		return wins
	}

	// For each week, create random pairings
	for _, week := range weeks {
		// Shuffle teams for this week
		shuffledTeams := make([]uint, len(teamIDs))
		copy(shuffledTeams, teamIDs)
		rng.Shuffle(len(shuffledTeams), func(i, j int) {
			shuffledTeams[i], shuffledTeams[j] = shuffledTeams[j], shuffledTeams[i]
		})

		// Create pairings and determine winners
		for i := 0; i < len(shuffledTeams); i += 2 {
			team1ID := shuffledTeams[i]
			team2ID := shuffledTeams[i+1]

			// Get scores for this week (if they played in this week)
			team1Score, team1HasScore := teamWeeklyScores[team1ID][week]
			team2Score, team2HasScore := teamWeeklyScores[team2ID][week]

			// Only count if both teams have scores for this week
			if team1HasScore && team2HasScore {
				if team1Score > team2Score {
					wins[team1ID]++
				} else if team2Score > team1Score {
					wins[team2ID]++
				}
				// Ties don't count as wins for either team
			}
		}
	}

	return wins
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
