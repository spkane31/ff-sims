package models

import (
	"time"

	"gorm.io/gorm"
)

// BoxScore represents a player's performance in a specific matchup
type BoxScore struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	MatchupID   uint      `json:"matchup_id"`
	PlayerID    uint      `json:"player_id"`
	TeamID      uint      `json:"team_id"`       // The team the player was on for this matchup
	Week        uint      `json:"week"`
	Year        uint      `json:"year"`
	Season      int       `json:"season"`
	GameDate    time.Time `json:"game_date"`
	StartedFlag bool      `json:"started_flag" gorm:"default:false"` // Whether player was in starting lineup

	// Fantasy points
	ActualPoints   float64 `json:"actual_points"`
	ProjectedPoints float64 `json:"projected_points"`

	// Game stats (embedded from PlayerStats)
	GameStats PlayerStats `json:"game_stats" gorm:"embedded"`

	// Relationships
	Matchup *Matchup `json:"matchup,omitempty"`
	Player  *Player  `json:"player,omitempty"`
	Team    *Team    `json:"team,omitempty"`
}

// GetPlayerBoxScoresByWeek returns all box scores for a player in a specific week and year
func GetPlayerBoxScoresByWeek(db *gorm.DB, playerID uint, week uint, year uint) ([]BoxScore, error) {
	var boxScores []BoxScore
	err := db.Where("player_id = ? AND week = ? AND year = ?", playerID, week, year).Find(&boxScores).Error
	return boxScores, err
}

// GetTeamBoxScoresByMatchup returns all box scores for a team in a specific matchup
func GetTeamBoxScoresByMatchup(db *gorm.DB, teamID uint, matchupID uint) ([]BoxScore, error) {
	var boxScores []BoxScore
	err := db.Where("team_id = ? AND matchup_id = ?", teamID, matchupID).Find(&boxScores).Error
	return boxScores, err
}