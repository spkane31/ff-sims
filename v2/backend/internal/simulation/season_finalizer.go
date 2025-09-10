package simulation

import (
	"backend/internal/database"
	"backend/internal/models"
	"log"

	"gorm.io/gorm"
)

// FinalizeSeasonExpectedWins creates season totals from the final regular season week
func FinalizeSeasonExpectedWins(leagueID uint, year uint) error {
	db := database.DB

	// 1. Determine final regular season week (exclude playoffs)
	finalWeek, err := models.GetFinalRegularSeasonWeek(db, leagueID, year)
	if err != nil {
		return err
	}

	if finalWeek == 0 {
		log.Printf("No completed regular season games found for league %d, year %d", leagueID, year)
		return nil
	}

	// 2. Get all teams for this league
	teams, err := models.GetAllTeamsByLeague(db, leagueID)
	if err != nil {
		return err
	}

	for _, team := range teams {
		err := finalizeTeamSeasonExpectedWins(db, team, year, finalWeek)
		if err != nil {
			log.Printf("Failed to finalize season expected wins for team %d: %v", team.ID, err)
			// Continue with other teams even if one fails
		}
	}

	return nil
}

// finalizeTeamSeasonExpectedWins creates season totals for a single team
func finalizeTeamSeasonExpectedWins(db *gorm.DB, team models.Team, year uint, finalWeek uint) error {
	// Find the latest week this team actually has data for
	teamFinalWeek, err := getTeamFinalWeek(db, team.ID, year, finalWeek)
	if err != nil {
		log.Printf("Failed to determine final week for team %d, year %d: %v", team.ID, year, err)
		return err
	}

	if teamFinalWeek == 0 {
		log.Printf("No completed weeks found for team %d, year %d - skipping season finalization", team.ID, year)
		return nil // Skip this team - no data to finalize
	}

	// Get all weekly data up to the final week to calculate proper cumulative totals
	allWeeklyData, err := models.GetTeamWeeklyProgression(db, team.ID, year)
	if err != nil || len(allWeeklyData) == 0 {
		log.Printf("No weekly progression data found for team %d, year %d", team.ID, year)
		return err
	}
	
	// Calculate proper cumulative totals from all weekly data
	var cumulativeExpectedWins, cumulativeExpectedLosses float64
	var cumulativeActualWins, cumulativeActualLosses int
	var lastStrengthOfSchedule float64
	
	for _, weekData := range allWeeklyData {
		if weekData.Week <= teamFinalWeek {
			cumulativeExpectedWins += weekData.WeeklyExpectedWins
			cumulativeExpectedLosses += weekData.WeeklyExpectedLosses
			if weekData.WeeklyActualWin {
				cumulativeActualWins++
			} else {
				cumulativeActualLosses++
			}
			lastStrengthOfSchedule = weekData.StrengthOfSchedule // Use the most recent SOS
		}
	}
	
	// Win luck is calculated on-demand in API responses (actual_wins - expected_wins)

	// Calculate season aggregates using the team's actual final week
	seasonStats, err := models.CalculateSeasonAggregates(db, team.ID, year, teamFinalWeek)
	if err != nil {
		return err
	}

	// Get playoff and standing info
	playoffMade, finalStanding := models.GetTeamSeasonOutcome(db, team.ID, year)

	// Create season record using calculated cumulative data
	seasonRecord := &models.SeasonExpectedWins{
		TeamID:               team.ID,
		Year:                 year,
		LeagueID:             team.LeagueID,
		FinalWeek:            teamFinalWeek,
		ExpectedWins:         cumulativeExpectedWins,
		ExpectedLosses:       cumulativeExpectedLosses,
		ActualWins:           cumulativeActualWins,
		ActualLosses:         cumulativeActualLosses,
		StrengthOfSchedule:   lastStrengthOfSchedule,
		TotalPointsFor:       seasonStats.TotalPointsFor,
		TotalPointsAgainst:   seasonStats.TotalPointsAgainst,
		AveragePointsFor:     seasonStats.AveragePointsFor,
		AveragePointsAgainst: seasonStats.AveragePointsAgainst,
		PlayoffMade:          playoffMade,
		FinalStanding:        finalStanding,
	}

	return models.SaveSeasonExpectedWins(db, seasonRecord)
}

// RecalculateSeasonExpectedWins recalculates season expected wins for all teams
func RecalculateSeasonExpectedWins(leagueID uint, year uint) error {
	db := database.DB

	// Delete existing season records for recalculation
	err := db.Where("league_id = ? AND year = ?", leagueID, year).
		Delete(&models.SeasonExpectedWins{}).Error
	if err != nil {
		return err
	}

	// Recalculate season totals
	return FinalizeSeasonExpectedWins(leagueID, year)
}

// UpdateSeasonStandings updates final standings for all teams in a season
// This should be called after playoffs are complete
func UpdateSeasonStandings(leagueID uint, year uint) error {
	db := database.DB

	// Get all season expected wins records for this league/year
	var seasonRecords []models.SeasonExpectedWins
	err := db.Where("league_id = ? AND year = ?", leagueID, year).
		Order("actual_wins DESC, total_points_for DESC").
		Find(&seasonRecords).Error
	if err != nil {
		return err
	}

	// Update standings (1st place, 2nd place, etc.)
	for i, record := range seasonRecords {
		record.FinalStanding = i + 1
		err := models.SaveSeasonExpectedWins(db, &record)
		if err != nil {
			log.Printf("Failed to update standing for team %d: %v", record.TeamID, err)
		}
	}

	return nil
}

// GetSeasonExpectedWinsRankings returns teams ranked by various metrics
type SeasonRankings struct {
	ByExpectedWins []models.SeasonExpectedWins `json:"by_expected_wins"`
	ByActualWins   []models.SeasonExpectedWins `json:"by_actual_wins"`
	ByLuck         []models.SeasonExpectedWins `json:"by_luck"`     // Most lucky teams first
	ByUnlucky      []models.SeasonExpectedWins `json:"by_unlucky"`  // Most unlucky teams first
	ByStrength     []models.SeasonExpectedWins `json:"by_strength"` // Hardest schedule first
}

func GetSeasonExpectedWinsRankings(leagueID uint, year uint) (*SeasonRankings, error) {
	db := database.DB

	rankings := &SeasonRankings{}

	// By Expected Wins
	err := db.Where("league_id = ? AND year = ?", leagueID, year).
		Preload("Team").
		Order("expected_wins DESC, total_points_for DESC").
		Find(&rankings.ByExpectedWins).Error
	if err != nil {
		return nil, err
	}

	// By Actual Wins
	err = db.Where("league_id = ? AND year = ?", leagueID, year).
		Preload("Team").
		Order("actual_wins DESC, total_points_for DESC").
		Find(&rankings.ByActualWins).Error
	if err != nil {
		return nil, err
	}

	// By Luck (positive luck = over-performed)
	err = db.Where("league_id = ? AND year = ?", leagueID, year).
		Preload("Team").
		Order("win_luck DESC").
		Find(&rankings.ByLuck).Error
	if err != nil {
		return nil, err
	}

	// By Unlucky (negative luck = under-performed)
	err = db.Where("league_id = ? AND year = ?", leagueID, year).
		Preload("Team").
		Order("win_luck ASC").
		Find(&rankings.ByUnlucky).Error
	if err != nil {
		return nil, err
	}

	// By Strength of Schedule
	err = db.Where("league_id = ? AND year = ?", leagueID, year).
		Preload("Team").
		Order("strength_of_schedule DESC").
		Find(&rankings.ByStrength).Error
	if err != nil {
		return nil, err
	}

	return rankings, nil
}

// CalculateLeagueLuckDistribution calculates how much luck affected the league
type LuckDistribution struct {
	TotalLuckVariance    float64 `json:"total_luck_variance"`    // How much luck affected outcomes
	MostLuckyTeam        string  `json:"most_lucky_team"`        // Team name
	MostLuckyLuck        float64 `json:"most_lucky_luck"`        // Luck value
	MostUnluckyTeam      string  `json:"most_unlucky_team"`      // Team name
	MostUnluckyLuck      float64 `json:"most_unlucky_luck"`      // Luck value
	LuckRange            float64 `json:"luck_range"`             // Difference between most/least lucky
	PlayoffLuckImpact    int     `json:"playoff_luck_impact"`    // Teams that made playoffs due to luck
	AverageLuckMagnitude float64 `json:"average_luck_magnitude"` // Average absolute luck value
}

func CalculateLeagueLuckDistribution(leagueID uint, year uint) (*LuckDistribution, error) {
	db := database.DB

	var seasonRecords []models.SeasonExpectedWins
	err := db.Where("league_id = ? AND year = ?", leagueID, year).
		Preload("Team").
		Find(&seasonRecords).Error
	if err != nil {
		return nil, err
	}

	if len(seasonRecords) == 0 {
		return &LuckDistribution{}, nil
	}

	distribution := &LuckDistribution{}

	// Calculate luck on-demand for each team (actual_wins - expected_wins)
	firstLuck := float64(seasonRecords[0].ActualWins) - seasonRecords[0].ExpectedWins
	maxLuck := firstLuck
	minLuck := firstLuck
	maxLuckTeam := seasonRecords[0].Team.Owner
	minLuckTeam := seasonRecords[0].Team.Owner

	var luckSum, luckSumAbs float64

	for _, record := range seasonRecords {
		luck := float64(record.ActualWins) - record.ExpectedWins
		luckSum += luck
		luckSumAbs += abs(luck)

		if luck > maxLuck {
			maxLuck = luck
			if record.Team != nil {
				maxLuckTeam = record.Team.Owner
			}
		}

		if luck < minLuck {
			minLuck = luck
			if record.Team != nil {
				minLuckTeam = record.Team.Owner
			}
		}
	}

	// Calculate variance
	mean := luckSum / float64(len(seasonRecords))
	var variance float64
	for _, record := range seasonRecords {
		luck := float64(record.ActualWins) - record.ExpectedWins
		diff := luck - mean
		variance += diff * diff
	}
	variance /= float64(len(seasonRecords))

	distribution.TotalLuckVariance = variance
	distribution.MostLuckyTeam = maxLuckTeam
	distribution.MostLuckyLuck = maxLuck
	distribution.MostUnluckyTeam = minLuckTeam
	distribution.MostUnluckyLuck = minLuck
	distribution.LuckRange = maxLuck - minLuck
	distribution.AverageLuckMagnitude = luckSumAbs / float64(len(seasonRecords))

	// TODO: Calculate playoff luck impact by comparing expected wins standings to actual playoff teams

	return distribution, nil
}

// getTeamFinalWeek finds the latest week that a specific team has weekly expected wins data
// This handles cases where teams might not have data for the global final week
func getTeamFinalWeek(db *gorm.DB, teamID uint, year uint, globalFinalWeek uint) (uint, error) {
	var maxWeek *uint
	err := db.Model(&models.WeeklyExpectedWins{}).
		Where("team_id = ? AND year = ? AND week <= ?", teamID, year, globalFinalWeek).
		Select("MAX(week)").
		Scan(&maxWeek).Error

	if err != nil {
		return 0, err
	}

	if maxWeek == nil {
		return 0, nil // No weeks found for this team
	}

	return *maxWeek, nil
}

// Helper function for absolute value
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
