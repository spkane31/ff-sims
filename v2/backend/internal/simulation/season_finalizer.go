package simulation

import (
	"backend/internal/database"
	"backend/internal/models"
	"log"

	"gorm.io/gorm"
)

// FinalizeSeasonExpectedWins creates season totals from existing weekly expected wins data
func FinalizeSeasonExpectedWins(leagueID uint, year uint) error {
	db := database.DB

	// 1. Check if we have any weekly expected wins data for this league/year
	var weeklyCount int64
	db.Model(&models.WeeklyExpectedWins{}).
		Where("league_id = ? AND year = ?", leagueID, year).
		Count(&weeklyCount)

	if weeklyCount == 0 {
		log.Printf("No weekly expected wins data found for league %d, year %d", leagueID, year)
		return nil
	}

	log.Printf("Found %d weekly expected wins records for league %d, year %d. Creating season aggregates.", weeklyCount, leagueID, year)

	// 2. Get all teams that have weekly data for this league/year
	var teamIDs []uint
	db.Model(&models.WeeklyExpectedWins{}).
		Where("league_id = ? AND year = ?", leagueID, year).
		Distinct("team_id").
		Pluck("team_id", &teamIDs)

	if len(teamIDs) == 0 {
		log.Printf("No teams found with weekly expected wins data for league %d, year %d", leagueID, year)
		return nil
	}

	// 3. Process each team
	for _, teamID := range teamIDs {
		err := finalizeTeamSeasonFromWeeklyData(db, teamID, leagueID, year)
		if err != nil {
			log.Printf("Failed to finalize season expected wins for team %d: %v", teamID, err)
			// Continue with other teams even if one fails
		}
	}

	return nil
}

// finalizeTeamSeasonFromWeeklyData creates season totals by aggregating existing weekly data
func finalizeTeamSeasonFromWeeklyData(db *gorm.DB, teamID uint, leagueID uint, year uint) error {
	// Get all weekly data for this team/year
	allWeeklyData, err := models.GetTeamWeeklyProgression(db, teamID, year)
	if err != nil || len(allWeeklyData) == 0 {
		log.Printf("No weekly progression data found for team %d, year %d", teamID, year)
		return err
	}

	// Find the latest week with data (this is our "final week")
	finalWeek := uint(0)
	var finalWeekData *models.WeeklyExpectedWins
	for i := range allWeeklyData {
		if allWeeklyData[i].Week > finalWeek {
			finalWeek = allWeeklyData[i].Week
			finalWeekData = &allWeeklyData[i]
		}
	}

	if finalWeekData == nil {
		log.Printf("No final week data found for team %d, year %d", teamID, year)
		return nil
	}

	// The final week record already contains cumulative totals, so we can use them directly
	cumulativeExpectedWins := finalWeekData.ExpectedWins
	cumulativeExpectedLosses := finalWeekData.ExpectedLosses
	cumulativeActualWins := finalWeekData.ActualWins
	cumulativeActualLosses := finalWeekData.ActualLosses
	lastStrengthOfSchedule := finalWeekData.StrengthOfSchedule

	// Calculate season aggregates for points
	seasonStats, err := models.CalculateSeasonAggregates(db, teamID, year, finalWeek)
	if err != nil {
		log.Printf("Failed to calculate season aggregates for team %d, year %d: %v", teamID, year, err)
		// Create empty stats if calculation fails
		seasonStats = &models.SeasonAggregates{}
	}

	// Get playoff and standing info
	playoffMade, finalStanding := models.GetTeamSeasonOutcome(db, teamID, year)

	// Create season record using the aggregated data
	seasonRecord := &models.SeasonExpectedWins{
		TeamID:               teamID,
		Year:                 year,
		LeagueID:             leagueID,
		FinalWeek:            finalWeek,
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

	log.Printf("Creating season record for team %d, year %d: %.2f expected wins, %d actual wins", 
		teamID, year, cumulativeExpectedWins, cumulativeActualWins)

	return models.SaveSeasonExpectedWins(db, seasonRecord)
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


// Helper function for absolute value
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
