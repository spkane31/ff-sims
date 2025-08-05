package models

import (
	"time"

	"gorm.io/gorm"
)

// Matchup represents a fantasy football matchup between two teams
type Matchup struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	LeagueID   uint      `json:"league_id"`
	Week       uint      `json:"week"`
	Year       uint      `json:"year"`
	Season     int       `json:"season"`
	HomeTeamID uint      `json:"home_team_id"`
	AwayTeamID uint      `json:"away_team_id"`
	GameDate   time.Time `json:"game_date"`
	GameType   string    `json:"game_type"`

	// Score information
	HomeTeamFinalScore         float64 `json:"home_score"`
	AwayTeamFinalScore         float64 `json:"away_score"`
	HomeTeamESPNProjectedScore float64 `json:"home_projected_score"`
	AwayTeamESPNProjectedScore float64 `json:"away_projected_score"`

	// Status flags
	Completed bool `json:"completed" gorm:"default:false"`
	IsPlayoff bool `json:"is_playoff" gorm:"default:false"`

	// Relationships
	League     *League     `json:"-"`
	HomeTeam   *Team       `json:"home_team,omitempty" gorm:"foreignKey:HomeTeamID"`
	AwayTeam   *Team       `json:"away_team,omitempty" gorm:"foreignKey:AwayTeamID"`
	BoxScores  []BoxScore  `json:"box_scores,omitempty" gorm:"foreignKey:MatchupID"`
	SimResults []SimResult `json:"-"`
}

// GetMatchupsByWeek returns all matchups for a specific week and year
func GetMatchupsByWeek(db *gorm.DB, leagueID uint, week uint, year uint) ([]Matchup, error) {
	var matchups []Matchup
	err := db.Where("league_id = ? AND week = ? AND year = ?", leagueID, week, year).Find(&matchups).Error
	return matchups, err
}

// GetTeamMatchups returns all matchups for a specific team (either home or away)
func GetTeamMatchups(db *gorm.DB, teamID uint, year uint) ([]Matchup, error) {
	var matchups []Matchup
	err := db.Where("(home_team_id = ? OR away_team_id = ?) AND year = ?", teamID, teamID, year).Find(&matchups).Error
	return matchups, err
}

// LoadFullMatchup loads a matchup with all related box scores and team information
func LoadFullMatchup(db *gorm.DB, matchupID uint) (*Matchup, error) {
	var matchup Matchup
	err := db.Preload("HomeTeam").Preload("AwayTeam").Preload("BoxScores.Player").Where("id = ?", matchupID).First(&matchup).Error
	return &matchup, err
}
