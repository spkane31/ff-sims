package simulation

import (
	"backend/internal/models"
	"math"
)

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

// CalculateExpectedWins calculates expected wins for all teams based on their schedule
// Expected wins uses a logistic model based on the point differential
func CalculateExpectedWins(schedule []*models.Matchup) ([]ExpectedWinsResult, error) {
	if len(schedule) == 0 {
		return []ExpectedWinsResult{}, nil
	}
	
	// Group matchups by team
	teamMatchups := make(map[uint][]*models.Matchup)
	teamStats := make(map[uint]*ExpectedWinsResult)
	
	for _, matchup := range schedule {
		if !matchup.Completed {
			continue // Only calculate for completed games
		}
		
		// Only include regular season games in expected wins calculation
		if matchup.IsPlayoff || matchup.GameType != "NONE" {
			continue
		}
		
		// Initialize team stats if not exists
		if teamStats[matchup.HomeTeamID] == nil {
			teamStats[matchup.HomeTeamID] = &ExpectedWinsResult{
				TeamID: matchup.HomeTeamID,
			}
		}
		if teamStats[matchup.AwayTeamID] == nil {
			teamStats[matchup.AwayTeamID] = &ExpectedWinsResult{
				TeamID: matchup.AwayTeamID,
			}
		}
		
		// Add matchup to both teams' lists
		teamMatchups[matchup.HomeTeamID] = append(teamMatchups[matchup.HomeTeamID], matchup)
		teamMatchups[matchup.AwayTeamID] = append(teamMatchups[matchup.AwayTeamID], matchup)
		
		// Update actual wins/losses
		if matchup.HomeTeamFinalScore > matchup.AwayTeamFinalScore {
			teamStats[matchup.HomeTeamID].ActualWins++
			teamStats[matchup.AwayTeamID].ActualLosses++
		} else if matchup.AwayTeamFinalScore > matchup.HomeTeamFinalScore {
			teamStats[matchup.AwayTeamID].ActualWins++
			teamStats[matchup.HomeTeamID].ActualLosses++
		}
		
		teamStats[matchup.HomeTeamID].TotalGames++
		teamStats[matchup.AwayTeamID].TotalGames++
	}
	
	// Calculate expected wins for each team
	for teamID, matchups := range teamMatchups {
		stats := teamStats[teamID]
		expectedWins := 0.0
		totalOpponentStrength := 0.0
		
		for _, matchup := range matchups {
			var teamScore, opponentScore float64
			var opponentID uint
			
			if matchup.HomeTeamID == teamID {
				teamScore = matchup.HomeTeamFinalScore
				opponentScore = matchup.AwayTeamFinalScore
				opponentID = matchup.AwayTeamID
			} else {
				teamScore = matchup.AwayTeamFinalScore
				opponentScore = matchup.HomeTeamFinalScore
				opponentID = matchup.HomeTeamID
			}
			
			// Calculate expected win probability using logistic function
			// Based on point differential
			pointDiff := teamScore - opponentScore
			winProbability := logisticWinProbability(pointDiff)
			expectedWins += winProbability
			
			// Add to strength of schedule calculation
			if opponentStats, exists := teamStats[opponentID]; exists {
				opponentWinRate := float64(opponentStats.ActualWins) / float64(opponentStats.TotalGames)
				totalOpponentStrength += opponentWinRate
			}
		}
		
		stats.ExpectedWins = expectedWins
		stats.ExpectedLosses = float64(len(matchups)) - expectedWins
		
		// Calculate strength of schedule (average opponent win rate)
		if len(matchups) > 0 {
			stats.StrengthOfSchedule = totalOpponentStrength / float64(len(matchups))
		}
	}
	
	// Convert map to slice
	results := make([]ExpectedWinsResult, 0, len(teamStats))
	for _, stats := range teamStats {
		results = append(results, *stats)
	}
	
	return results, nil
}

// logisticWinProbability calculates win probability based on point differential
// Uses a logistic function: P(win) = 1 / (1 + e^(-point_diff / scale))
// The scale parameter controls how sensitive the probability is to point differences
func logisticWinProbability(pointDiff float64) float64 {
	scale := 16.0 // Standard scale for fantasy football
	if pointDiff == 0 {
		return 0.5 // Tie game = 50% probability
	}
	return 1.0 / (1.0 + math.Exp(-pointDiff/scale))
}

// CalculateSeasonExpectedWins calculates expected wins for a full season
// This function can be used to project expected wins across an entire season
func CalculateSeasonExpectedWins(teams []models.Team, completedMatchups []*models.Matchup, remainingSchedule []*models.Matchup) ([]ExpectedWinsResult, error) {
	// First calculate expected wins from completed games
	completedResults, err := CalculateExpectedWins(completedMatchups)
	if err != nil {
		return nil, err
	}
	
	// Create a map for easy lookup
	teamResults := make(map[uint]*ExpectedWinsResult)
	for i := range completedResults {
		result := &completedResults[i]
		teamResults[result.TeamID] = result
	}
	
	// For remaining games, use projected scores to estimate expected wins
	for _, matchup := range remainingSchedule {
		if matchup.Completed {
			continue // Skip if already completed
		}
		
		// Only include regular season games in expected wins calculation
		if matchup.IsPlayoff || matchup.GameType != "NONE" {
			continue
		}
		
		// Initialize team results if they don't exist
		if teamResults[matchup.HomeTeamID] == nil {
			teamResults[matchup.HomeTeamID] = &ExpectedWinsResult{
				TeamID: matchup.HomeTeamID,
			}
		}
		if teamResults[matchup.AwayTeamID] == nil {
			teamResults[matchup.AwayTeamID] = &ExpectedWinsResult{
				TeamID: matchup.AwayTeamID,
			}
		}
		
		// Use projected scores for future games
		homeProjDiff := matchup.HomeTeamESPNProjectedScore - matchup.AwayTeamESPNProjectedScore
		awayProjDiff := -homeProjDiff
		
		homeWinProb := logisticWinProbability(homeProjDiff)
		awayWinProb := logisticWinProbability(awayProjDiff)
		
		teamResults[matchup.HomeTeamID].ExpectedWins += homeWinProb
		teamResults[matchup.HomeTeamID].ExpectedLosses += awayWinProb
		teamResults[matchup.HomeTeamID].TotalGames++
		
		teamResults[matchup.AwayTeamID].ExpectedWins += awayWinProb
		teamResults[matchup.AwayTeamID].ExpectedLosses += homeWinProb
		teamResults[matchup.AwayTeamID].TotalGames++
	}
	
	// Convert back to slice
	results := make([]ExpectedWinsResult, 0, len(teamResults))
	for _, result := range teamResults {
		results = append(results, *result)
	}
	
	return results, nil
}
