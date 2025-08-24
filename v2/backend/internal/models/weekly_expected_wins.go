package models

import (
	"time"

	"gorm.io/gorm"
)

// WeeklyExpectedWins stores expected wins calculations for each team, week, and year
type WeeklyExpectedWins struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	// Identifiers
	TeamID   uint `json:"team_id" gorm:"index:idx_team_week_year,unique"`
	Week     uint `json:"week" gorm:"index:idx_team_week_year,unique;index:idx_league_week_year"`
	Year     uint `json:"year" gorm:"index:idx_team_week_year,unique;index:idx_league_week_year"`
	LeagueID uint `json:"league_id" gorm:"index:idx_league_week_year"`

	// Expected wins data
	ExpectedWins         float64 `json:"expected_wins"`          // Cumulative expected wins through this week
	WeeklyExpectedWins   float64 `json:"weekly_expected_wins"`   // Expected wins for just this week (≤ 1)
	ExpectedLosses       float64 `json:"expected_losses"`        // Cumulative expected losses through this week
	WeeklyExpectedLosses float64 `json:"weekly_expected_losses"` // Expected losses for just this week (≤ 1)

	// Actual performance
	ActualWins      int  `json:"actual_wins"`       // Cumulative actual wins through this week
	ActualLosses    int  `json:"actual_losses"`     // Cumulative actual losses through this week
	WeeklyActualWin bool `json:"weekly_actual_win"` // Did they win this week

	// Metrics
	WinLuck              float64 `json:"win_luck"`               // ActualWins - ExpectedWins
	StrengthOfSchedule   float64 `json:"strength_of_schedule"`   // Average opponent strength
	WeeklyWinProbability float64 `json:"weekly_win_probability"` // Win probability for this week's matchup

	// Performance context
	TeamScore         float64 `json:"team_score"`         // Team's score this week
	OpponentScore     float64 `json:"opponent_score"`     // Opponent's score this week
	OpponentTeamID    uint    `json:"opponent_team_id"`   // Who they played
	PointDifferential float64 `json:"point_differential"` // TeamScore - OpponentScore

	// Relationships
	Team         *Team   `json:"team,omitempty"`
	OpponentTeam *Team   `json:"opponent_team,omitempty" gorm:"foreignKey:OpponentTeamID"`
	League       *League `json:"-"`
}

// GetWeeklyExpectedWins returns weekly expected wins for a specific team, year, and week
func GetWeeklyExpectedWins(db *gorm.DB, teamID uint, year uint, week uint) (*WeeklyExpectedWins, error) {
	var weeklyExpectedWins WeeklyExpectedWins
	err := db.Where("team_id = ? AND year = ? AND week = ?", teamID, year, week).
		Preload("Team").
		Preload("OpponentTeam").
		First(&weeklyExpectedWins).Error
	if err != nil {
		return nil, err
	}
	return &weeklyExpectedWins, nil
}

// GetWeeklyExpectedWinsData returns all weekly expected wins for a league, year, and specific week
func GetWeeklyExpectedWinsData(db *gorm.DB, leagueID uint, year uint, week uint) ([]WeeklyExpectedWins, error) {
	var results []WeeklyExpectedWins
	err := db.Where("league_id = ? AND year = ? AND week = ?", leagueID, year, week).
		Preload("Team").
		Preload("OpponentTeam").
		Order("expected_wins DESC").
		Find(&results).Error
	return results, err
}

// GetAllWeeklyExpectedWins returns all weekly expected wins for a league and year (all weeks)
func GetAllWeeklyExpectedWins(db *gorm.DB, leagueID uint, year uint) ([]WeeklyExpectedWins, error) {
	var results []WeeklyExpectedWins
	err := db.Where("league_id = ? AND year = ?", leagueID, year).
		Preload("Team").
		Preload("OpponentTeam").
		Order("week ASC, expected_wins DESC").
		Find(&results).Error
	return results, err
}

// GetTeamWeeklyProgression returns all weekly expected wins for a specific team and year
func GetTeamWeeklyProgression(db *gorm.DB, teamID uint, year uint) ([]WeeklyExpectedWins, error) {
	var results []WeeklyExpectedWins
	err := db.Where("team_id = ? AND year = ?", teamID, year).
		Preload("Team").
		Preload("OpponentTeam").
		Order("week ASC").
		Find(&results).Error
	return results, err
}

// SaveWeeklyExpectedWins saves or updates a weekly expected wins record (idempotent)
func SaveWeeklyExpectedWins(db *gorm.DB, weeklyRecord *WeeklyExpectedWins) error {
	// First, try to find an existing record
	var existingRecord WeeklyExpectedWins
	err := db.Where("team_id = ? AND year = ? AND week = ?",
		weeklyRecord.TeamID, weeklyRecord.Year, weeklyRecord.Week).
		First(&existingRecord).Error

	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	if err == gorm.ErrRecordNotFound {
		// Create new record
		return db.Create(weeklyRecord).Error
	} else {
		// Update existing record - preserve the ID and timestamps
		weeklyRecord.ID = existingRecord.ID
		weeklyRecord.CreatedAt = existingRecord.CreatedAt
		return db.Save(weeklyRecord).Error
	}
}

// DeleteWeeklyExpectedWins deletes weekly expected wins records for recalculation
func DeleteWeeklyExpectedWins(db *gorm.DB, leagueID uint, year uint, week uint) error {
	return db.Where("league_id = ? AND year = ? AND week = ?", leagueID, year, week).
		Delete(&WeeklyExpectedWins{}).Error
}

// GetLastCompletedWeek returns the most recent week with completed games for a league
func GetLastCompletedWeek(db *gorm.DB, leagueID uint, year uint) (uint, error) {
	var maxWeek *uint
	err := db.Model(&Matchup{}).
		Where("league_id = ? AND year = ? AND completed = true AND game_type = 'NONE'", leagueID, year).
		Select("MAX(week)").
		Scan(&maxWeek).Error

	if err != nil {
		return 0, err
	}

	if maxWeek == nil {
		return 0, nil // No completed weeks found
	}

	return *maxWeek, nil
}

// IsWeekProcessed checks if weekly expected wins have been calculated for a specific week
func IsWeekProcessed(db *gorm.DB, leagueID uint, year uint, week uint) (bool, error) {
	var count int64
	err := db.Model(&WeeklyExpectedWins{}).
		Where("league_id = ? AND year = ? AND week = ?", leagueID, year, week).
		Count(&count).Error
	return count > 0, err
}

// GetFinalRegularSeasonWeek determines the last regular season week (excludes playoffs)
func GetFinalRegularSeasonWeek(db *gorm.DB, leagueID uint, year uint) (uint, error) {
	var maxWeek *uint
	err := db.Model(&Matchup{}).
		Where("league_id = ? AND year = ? AND game_type = ? AND completed = true", leagueID, year, "NONE").
		Select("MAX(week)").
		Scan(&maxWeek).Error

	if err != nil {
		return 0, err
	}

	if maxWeek == nil {
		return 0, nil // No completed regular season weeks found
	}

	return *maxWeek, nil
}
