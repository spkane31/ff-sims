package models

import (
	"time"

	"gorm.io/gorm"
)

// SeasonExpectedWins stores final season totals (calculated from final regular season week)
type SeasonExpectedWins struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	// Identifiers
	TeamID    uint `json:"team_id" gorm:"index:idx_season_team,unique"`
	Year      uint `json:"year" gorm:"index:idx_season_team,unique"`
	LeagueID  uint `json:"league_id"`
	FinalWeek uint `json:"final_week"` // Last regular season week used for calculation

	// Season totals
	ExpectedWins       float64 `json:"expected_wins"`
	ExpectedLosses     float64 `json:"expected_losses"`
	ActualWins         int     `json:"actual_wins"`
	ActualLosses       int     `json:"actual_losses"`
	StrengthOfSchedule float64 `json:"strength_of_schedule"`

	// Performance metrics
	TotalPointsFor       float64 `json:"total_points_for"`
	TotalPointsAgainst   float64 `json:"total_points_against"`
	AveragePointsFor     float64 `json:"average_points_for"`
	AveragePointsAgainst float64 `json:"average_points_against"`

	// Season context
	PlayoffMade   bool `json:"playoff_made"`
	FinalStanding int  `json:"final_standing"`

	// Relationships
	Team   *Team   `json:"team,omitempty"`
	League *League `json:"-"`
}

// WinLuck calculates luck as actual wins minus expected wins
func (s *SeasonExpectedWins) WinLuck() float64 {
	return float64(s.ActualWins) - s.ExpectedWins
}

// SeasonAggregates holds calculated season statistics
type SeasonAggregates struct {
	TotalPointsFor       float64
	TotalPointsAgainst   float64
	AveragePointsFor     float64
	AveragePointsAgainst float64
	GamesPlayed          int
}

// GetSeasonExpectedWinsData returns all season expected wins for a league and year
func GetSeasonExpectedWinsData(db *gorm.DB, leagueID uint, year uint) ([]SeasonExpectedWins, error) {
	var results []SeasonExpectedWins
	err := db.Where("league_id = ? AND year = ?", leagueID, year).
		Preload("Team").
		Order("expected_wins DESC").
		Find(&results).Error
	return results, err
}

// GetTeamSeasonExpectedWins returns season expected wins for a specific team and year
func GetTeamSeasonExpectedWins(db *gorm.DB, teamID uint, year uint) (*SeasonExpectedWins, error) {
	var seasonExpectedWins SeasonExpectedWins
	err := db.Where("team_id = ? AND year = ?", teamID, year).
		Preload("Team").
		First(&seasonExpectedWins).Error
	if err != nil {
		return nil, err
	}
	return &seasonExpectedWins, nil
}

// SaveSeasonExpectedWins saves or updates a season expected wins record (idempotent)
func SaveSeasonExpectedWins(db *gorm.DB, seasonRecord *SeasonExpectedWins) error {
	// First, try to find an existing record
	var existingRecord SeasonExpectedWins
	err := db.Where("team_id = ? AND year = ?", seasonRecord.TeamID, seasonRecord.Year).
		First(&existingRecord).Error

	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	if err == gorm.ErrRecordNotFound {
		// Create new record
		return db.Create(seasonRecord).Error
	} else {
		// Update existing record - preserve the ID and timestamps
		seasonRecord.ID = existingRecord.ID
		seasonRecord.CreatedAt = existingRecord.CreatedAt
		return db.Save(seasonRecord).Error
	}
}

// CalculateSeasonAggregates calculates total and average points for a team's season
func CalculateSeasonAggregates(db *gorm.DB, teamID uint, year uint, throughWeek uint) (*SeasonAggregates, error) {
	var aggregates SeasonAggregates

	// Calculate points for (when team is home)
	var homeStats struct {
		TotalPoints float64
		GameCount   int
	}
	err := db.Model(&Matchup{}).
		Where("home_team_id = ? AND year = ? AND week <= ? AND completed = true", teamID, year, throughWeek).
		Select("SUM(home_team_final_score) as total_points, COUNT(*) as game_count").
		Scan(&homeStats).Error
	if err != nil {
		return nil, err
	}

	// Calculate points for (when team is away)
	var awayStats struct {
		TotalPoints float64
		GameCount   int
	}
	err = db.Model(&Matchup{}).
		Where("away_team_id = ? AND year = ? AND week <= ? AND completed = true", teamID, year, throughWeek).
		Select("SUM(away_team_final_score) as total_points, COUNT(*) as game_count").
		Scan(&awayStats).Error
	if err != nil {
		return nil, err
	}

	// Calculate points against (when team is home)
	var homeAgainstStats struct {
		TotalPoints float64
	}
	err = db.Model(&Matchup{}).
		Where("home_team_id = ? AND year = ? AND week <= ? AND completed = true", teamID, year, throughWeek).
		Select("SUM(away_team_final_score) as total_points").
		Scan(&homeAgainstStats).Error
	if err != nil {
		return nil, err
	}

	// Calculate points against (when team is away)
	var awayAgainstStats struct {
		TotalPoints float64
	}
	err = db.Model(&Matchup{}).
		Where("away_team_id = ? AND year = ? AND week <= ? AND completed = true", teamID, year, throughWeek).
		Select("SUM(home_team_final_score) as total_points").
		Scan(&awayAgainstStats).Error
	if err != nil {
		return nil, err
	}

	// Combine results
	aggregates.TotalPointsFor = homeStats.TotalPoints + awayStats.TotalPoints
	aggregates.TotalPointsAgainst = homeAgainstStats.TotalPoints + awayAgainstStats.TotalPoints
	aggregates.GamesPlayed = homeStats.GameCount + awayStats.GameCount

	if aggregates.GamesPlayed > 0 {
		aggregates.AveragePointsFor = aggregates.TotalPointsFor / float64(aggregates.GamesPlayed)
		aggregates.AveragePointsAgainst = aggregates.TotalPointsAgainst / float64(aggregates.GamesPlayed)
	}

	return &aggregates, nil
}

// GetTeamSeasonOutcome determines if a team made playoffs and their final standing
func GetTeamSeasonOutcome(db *gorm.DB, teamID uint, year uint) (bool, int) {
	// Check if team played in any playoff games (GameType != 'NONE')
	var playoffGameCount int64
	db.Model(&Matchup{}).
		Where("(home_team_id = ? OR away_team_id = ?) AND year = ? AND game_type != ?", teamID, teamID, year, "NONE").
		Count(&playoffGameCount)

	playoffMade := playoffGameCount > 0

	// For now, return placeholder final standing
	// In a real implementation, this would calculate based on final standings
	// TODO: Implement proper final standings calculation based on wins/losses/points
	finalStanding := 1

	return playoffMade, finalStanding
}

// GetHistoricalSeasonExpectedWins returns season expected wins for a team across multiple years
func GetHistoricalSeasonExpectedWins(db *gorm.DB, teamID uint, startYear uint, endYear uint) ([]SeasonExpectedWins, error) {
	var results []SeasonExpectedWins
	err := db.Where("team_id = ? AND year >= ? AND year <= ?", teamID, startYear, endYear).
		Preload("Team").
		Order("year DESC").
		Find(&results).Error
	return results, err
}

// GetLeagueSeasonRankings returns teams ranked by expected wins for a specific season
func GetLeagueSeasonRankings(db *gorm.DB, leagueID uint, year uint) ([]SeasonExpectedWins, error) {
	var results []SeasonExpectedWins
	err := db.Where("league_id = ? AND year = ?", leagueID, year).
		Preload("Team").
		Order("expected_wins DESC, total_points_for DESC").
		Find(&results).Error
	return results, err
}
