package utils

import (
	"backend/internal/models"
	"slices"
)

// PlayoffGameType represents the type of playoff game
type PlayoffGameType string

const (
	PlayoffGameTypeRegular       PlayoffGameType = "REGULAR"
	PlayoffGameTypePlayoff       PlayoffGameType = "PLAYOFF"
	PlayoffGameTypeLosersBracket PlayoffGameType = "LOSERS_BRACKET" // Not used in our logic, but defined for completeness
	PlayoffGameTypeChampionship  PlayoffGameType = "CHAMPIONSHIP"
	PlayoffGameTypeThirdPlace    PlayoffGameType = "THIRD_PLACE"
)

// GetPlayoffGameType determines the playoff type of a game based on all schedule data
func GetPlayoffGameType(game models.Matchup, allSchedule []models.Matchup) PlayoffGameType {
	switch game.GameType {
	case "NONE":
		return PlayoffGameTypeRegular
	case "LOSERS_CONSOLATION_LADDER":
		return PlayoffGameTypeLosersBracket
	case "WINNERS_BRACKET":
		if isChampionshipGame(game, allSchedule) {
			return PlayoffGameTypeChampionship
		}
		return PlayoffGameTypePlayoff
	case "WINNERS_CONSOLATION_LADDER":
		if isThirdPlaceGame(game, allSchedule) {
			return PlayoffGameTypeThirdPlace
		}
		// Other consolation games are not considered playoff games for our purposes
		return PlayoffGameTypeLosersBracket
	default:
		return PlayoffGameTypeRegular
	}
}

// isChampionshipGame determines if a game is the championship game
func isChampionshipGame(game models.Matchup, allSchedule []models.Matchup) bool {
	if game.GameType != "WINNERS_BRACKET" {
		return false
	}

	// Get all WINNERS_BRACKET games for this year
	var yearPlayoffGames []models.Matchup
	for _, g := range allSchedule {
		if g.GameType == "WINNERS_BRACKET" && g.Year == game.Year {
			yearPlayoffGames = append(yearPlayoffGames, g)
		}
	}

	if len(yearPlayoffGames) == 0 {
		return false
	}

	// Find the last week of playoff games
	var lastPlayoffWeek uint
	for _, g := range yearPlayoffGames {
		if g.Week > lastPlayoffWeek {
			lastPlayoffWeek = g.Week
		}
	}

	// Championship game should be in the last week of playoffs
	return game.Week == lastPlayoffWeek
}

// isThirdPlaceGame determines if a consolation game is the actual third place game
func isThirdPlaceGame(game models.Matchup, allSchedule []models.Matchup) bool {
	if game.GameType != "WINNERS_CONSOLATION_LADDER" {
		return false
	}

	// Get all games for this year
	var yearGames []models.Matchup
	var lastWeek uint
	for _, g := range allSchedule {
		if g.Year == game.Year {
			yearGames = append(yearGames, g)
			if g.Week > lastWeek {
				lastWeek = g.Week
			}
		}
	}

	// Third place game should be in the last week
	if game.Week != lastWeek {
		return false
	}

	// Find WINNERS_BRACKET games from the second-to-last week (semifinals)
	secondToLastWeek := lastWeek - 1
	var semifinalGames []models.Matchup
	for _, g := range yearGames {
		if g.GameType == "WINNERS_BRACKET" && g.Week == secondToLastWeek {
			semifinalGames = append(semifinalGames, g)
		}
	}

	if len(semifinalGames) == 0 {
		return false
	}

	// Get the losers from the semifinal games
	var semifinalLosers []uint
	for _, semifinal := range semifinalGames {
		if semifinal.HomeTeamFinalScore > semifinal.AwayTeamFinalScore {
			// Away team lost
			semifinalLosers = append(semifinalLosers, semifinal.AwayTeamID)
		} else if semifinal.AwayTeamFinalScore > semifinal.HomeTeamFinalScore {
			// Home team lost
			semifinalLosers = append(semifinalLosers, semifinal.HomeTeamID)
		}
		// If tied, we can't determine a loser, so skip
	}

	// Check if both teams in the third place game are semifinal losers
	gameTeams := []uint{game.HomeTeamID, game.AwayTeamID}
	if len(semifinalLosers) < 2 {
		return false
	}

	for _, teamID := range gameTeams {
		if !slices.Contains(semifinalLosers, teamID) {
			return false
		}
	}

	return true
}

// FilterPlayoffGames filters schedule data to only include games that should be displayed
// (regular season games, playoff games, championship games, and third place games)
func FilterPlayoffGames(allSchedule []models.Matchup) []models.Matchup {
	var filteredGames []models.Matchup

	for _, game := range allSchedule {
		gameType := GetPlayoffGameType(game, allSchedule)
		if gameType == PlayoffGameTypeRegular ||
			gameType == PlayoffGameTypePlayoff ||
			gameType == PlayoffGameTypeChampionship ||
			gameType == PlayoffGameTypeThirdPlace {
			filteredGames = append(filteredGames, game)
		}
	}

	return filteredGames
}

// ShouldIncludeInPlayoffRecord determines if a game should count towards playoff records
func ShouldIncludeInPlayoffRecord(game models.Matchup, allSchedule []models.Matchup) bool {
	// First check: only consider games that are NOT regular season (GameType != "NONE")
	if game.GameType == "NONE" {
		return false
	}
	
	// Additional check: make sure the game is actually a playoff game by checking game type
	gameType := GetPlayoffGameType(game, allSchedule)
	
	// Only count meaningful playoff games:
	// - Championship games (finals)
	// - Playoff bracket games (semifinals) 
	// - Third place games (verified third place matchup)
	return gameType == PlayoffGameTypeChampionship ||
		(gameType == PlayoffGameTypePlayoff && game.GameType == "WINNERS_BRACKET") ||
		(gameType == PlayoffGameTypeThirdPlace && isThirdPlaceGame(game, allSchedule))
}

// ShouldIncludeInRecord determines if a game should count towards playoff records
func ShouldIncludeInRecord(game models.Matchup, allSchedule []models.Matchup) bool {
	return GetPlayoffGameType(game, allSchedule) != PlayoffGameTypeLosersBracket
}
