package simulation

import (
	"backend/internal/database"
	"backend/internal/models"
	"log"

	"gorm.io/gorm"
)

// ProcessWeeklyExpectedWins calculates expected wins for all teams for a specific week
func ProcessWeeklyExpectedWins(leagueID uint, year uint, week uint) error {
	db := database.DB

	// 1. Get all completed matchups for this week
	matchups, err := GetCompletedMatchupsByWeek(db, leagueID, year, week)
	if err != nil {
		return err
	}

	if len(matchups) == 0 {
		log.Printf("No completed matchups found for league %d, year %d, week %d", leagueID, year, week)
		return nil
	}

	// 2. Get all teams for this league
	teams, err := models.GetAllTeamsByLeague(db, leagueID)
	if err != nil {
		return err
	}

	for _, team := range teams {
		err := processTeamWeeklyExpectedWins(db, team, year, week)
		if err != nil {
			log.Printf("Failed to process weekly expected wins for team %d: %v", team.ID, err)
			// Continue with other teams even if one fails
		}
	}

	return nil
}

// processTeamWeeklyExpectedWins processes expected wins for a single team
func processTeamWeeklyExpectedWins(db *gorm.DB, team models.Team, year uint, week uint) error {
	// Get all matchups for this team through current week (for cumulative calculation)
	teamMatchupsThrough, err := GetTeamMatchupsThroughWeek(db, team.ID, year, week)
	if err != nil {
		return err
	}

	if len(teamMatchupsThrough) == 0 {
		return nil // No matchups for this team yet
	}

	// Convert to pointers for CalculateExpectedWins function
	cumulativeMatchupPointers := convertMatchupsToPointers(teamMatchupsThrough)

	// Calculate cumulative expected wins through this week
	cumulativeResults, err := CalculateExpectedWins(cumulativeMatchupPointers)
	if err != nil {
		return err
	}

	// Find this team's cumulative result
	var teamCumulativeResult *ExpectedWinsResult
	for _, r := range cumulativeResults {
		if r.TeamID == team.ID {
			teamCumulativeResult = &r
			break
		}
	}

	if teamCumulativeResult == nil {
		log.Printf("No cumulative expected wins result found for team %d", team.ID)
		return nil
	}

	// Get this week's specific matchup for additional context
	weekMatchup := findTeamMatchupForWeek(teamMatchupsThrough, week)

	// Skip teams that didn't play this week
	if weekMatchup == nil {
		return nil // Team didn't play this week, skip processing
	}

	var weeklyWin bool
	var opponentID uint
	var teamScore, oppScore, pointDiff float64

	if weekMatchup.HomeTeamID == team.ID {
		teamScore = weekMatchup.HomeTeamFinalScore
		oppScore = weekMatchup.AwayTeamFinalScore
		opponentID = weekMatchup.AwayTeamID
		weeklyWin = teamScore > oppScore
	} else {
		teamScore = weekMatchup.AwayTeamFinalScore
		oppScore = weekMatchup.HomeTeamFinalScore
		opponentID = weekMatchup.HomeTeamID
		weeklyWin = teamScore > oppScore
	}
	pointDiff = teamScore - oppScore

	// Calculate weekly expected wins using simulation for just this week
	var weeklyExpectedWins float64

	// Get all league matchups for just this week to calculate weekly expected wins
	allWeekMatchups, err := GetCompletedMatchupsByWeek(db, team.LeagueID, year, week)
	if err != nil {
		return err
	}

	// Convert to pointers
	weekMatchupPointers := convertMatchupsToPointers(allWeekMatchups)

	// Calculate expected wins for just this week using simulation
	weeklyResults, err := CalculateWeeklyExpectedWins(weekMatchupPointers, week)
	if err != nil {
		return err
	}

	// Find this team's weekly result
	for _, r := range weeklyResults {
		if r.TeamID == team.ID {
			weeklyExpectedWins = r.ExpectedWins // This should be 0-1
			break
		}
	}

	// Only skip if team truly didn't play (no matchup found for this week)
	// weeklyExpectedWins of 0 is valid - it means they were the lowest score of the week

	// Get cumulative actual wins/losses from previous week
	var prevWeekActualWins, prevWeekActualLosses int
	var prevCumulativeExpectedWins, prevCumulativeExpectedLosses float64
	if week > 1 {
		prevWeek, err := models.GetWeeklyExpectedWins(db, team.ID, year, week-1)
		if err == nil && prevWeek != nil {
			prevWeekActualWins = prevWeek.ActualWins
			prevWeekActualLosses = prevWeek.ActualLosses
			prevCumulativeExpectedWins = prevWeek.ExpectedWins + weeklyExpectedWins
			prevCumulativeExpectedLosses = prevWeek.ExpectedLosses + (1.0 - weeklyExpectedWins)
		}
	} else {
		prevCumulativeExpectedWins = weeklyExpectedWins
		prevCumulativeExpectedLosses = 1.0 - weeklyExpectedWins
	}

	// Calculate cumulative actual wins/losses
	cumulativeActualWins := prevWeekActualWins
	cumulativeActualLosses := prevWeekActualLosses
	if weeklyWin {
		cumulativeActualWins += 1
	} else {
		cumulativeActualLosses += 1
	}

	// Create/update weekly record
	weeklyRecord := &models.WeeklyExpectedWins{
		TeamID:               team.ID,
		Week:                 week,
		Year:                 year,
		LeagueID:             team.LeagueID,
		ExpectedWins:         prevCumulativeExpectedWins,   // Cumulative expected wins through this week
		WeeklyExpectedWins:   weeklyExpectedWins,           // Expected wins for just this week (0-1)
		ExpectedLosses:       prevCumulativeExpectedLosses, // Cumulative expected losses through this week
		WeeklyExpectedLosses: 1.0 - weeklyExpectedWins,     // Expected losses for just this week (0-1)
		ActualWins:           cumulativeActualWins,         // Cumulative actual wins through this week
		ActualLosses:         cumulativeActualLosses,       // Cumulative actual losses through this week
		WeeklyActualWin:      weeklyWin,
		WinLuck:              float64(cumulativeActualWins) - teamCumulativeResult.ExpectedWins,
		StrengthOfSchedule:   teamCumulativeResult.StrengthOfSchedule,
		WeeklyWinProbability: weeklyExpectedWins, // Use weekly expected wins as win probability for this week
		TeamScore:            teamScore,
		OpponentScore:        oppScore,
		OpponentTeamID:       opponentID,
		PointDifferential:    pointDiff,
	}

	// Save to database
	return models.SaveWeeklyExpectedWins(db, weeklyRecord)
}

// Helper functions

// GetCompletedMatchupsByWeek returns all completed regular season matchups for a specific week
func GetCompletedMatchupsByWeek(db *gorm.DB, leagueID uint, year uint, week uint) ([]models.Matchup, error) {
	var matchups []models.Matchup
	err := db.Where("league_id = ? AND year = ? AND week = ? AND completed = true AND game_type = ?", leagueID, year, week, "NONE").
		Find(&matchups).Error
	return matchups, err
}

// GetTeamMatchupsThroughWeek returns all regular season matchups for a team through a specific week
func GetTeamMatchupsThroughWeek(db *gorm.DB, teamID uint, year uint, throughWeek uint) ([]models.Matchup, error) {
	var matchups []models.Matchup
	err := db.Where("(home_team_id = ? OR away_team_id = ?) AND year = ? AND week <= ? AND completed = true AND game_type = ?",
		teamID, teamID, year, throughWeek, "NONE").
		Order("week ASC").
		Find(&matchups).Error
	return matchups, err
}

// convertMatchupsToPointers converts slice of Matchup to slice of *Matchup
func convertMatchupsToPointers(matchups []models.Matchup) []*models.Matchup {
	pointers := make([]*models.Matchup, len(matchups))
	for i := range matchups {
		pointers[i] = &matchups[i]
	}
	return pointers
}

// findTeamMatchupForWeek finds the matchup for a specific team in a specific week
func findTeamMatchupForWeek(matchups []models.Matchup, week uint) *models.Matchup {
	for i := range matchups {
		if matchups[i].Week == week {
			return &matchups[i]
		}
	}
	return nil
}

// RecalculateWeeklyExpectedWins recalculates expected wins for all weeks in a season
func RecalculateWeeklyExpectedWins(leagueID uint, year uint) error {
	db := database.DB

	// Get the range of weeks to recalculate
	var minWeek, maxWeek uint
	err := db.Model(&models.Matchup{}).
		Where("league_id = ? AND year = ? AND completed = true", leagueID, year).
		Select("MIN(week) as min_week, MAX(week) as max_week").
		Row().Scan(&minWeek, &maxWeek)
	if err != nil {
		return err
	}

	// Delete existing records for recalculation
	err = db.Where("league_id = ? AND year = ?", leagueID, year).
		Delete(&models.WeeklyExpectedWins{}).Error
	if err != nil {
		return err
	}

	// Recalculate week by week
	for week := minWeek; week <= maxWeek; week++ {
		err := ProcessWeeklyExpectedWins(leagueID, year, week)
		if err != nil {
			log.Printf("Failed to recalculate week %d for league %d, year %d: %v", week, leagueID, year, err)
			// Continue with other weeks
		}
	}

	return nil
}

// IsRegularSeasonComplete checks if the regular season is complete for a league/year
func IsRegularSeasonComplete(db *gorm.DB, leagueID uint, year uint) bool {
	// Check if there are any incomplete regular season games (GameType = 'NONE')
	var incompleteCount int64
	db.Model(&models.Matchup{}).
		Where("league_id = ? AND year = ? AND game_type = ? AND completed = false", leagueID, year, "NONE").
		Count(&incompleteCount)

	return incompleteCount == 0
}
