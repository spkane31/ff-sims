package models

import (
	"time"

	"gorm.io/gorm"
)

type BoxScore struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	MatchupID   uint `json:"matchup_id"`
	PlayerID    uint `json:"player_id"`
	TeamID      uint `json:"team_id"` // The team the player was on for this matchup
	StartedFlag bool `json:"started_flag" gorm:"default:false"`

	SlotPosition string `json:"slot_position"` // Position in fantasy lineup (e.g., "QB", "RB", etc.)

	// Fantasy points
	ActualPoints    float64 `json:"actual_points"`
	ProjectedPoints float64 `json:"projected_points"`

	GameStats PlayerStats `json:"game_stats" gorm:"embedded"`

	// Relationships
	Matchup *Matchup `json:"matchup,omitempty"`
	Player  *Player  `json:"player,omitempty"`
	Team    *Team    `json:"team,omitempty"`
}

// GetPlayerBoxScoresByWeek returns all box scores for a player in a specific week and year
func GetPlayerBoxScoresByWeek(db *gorm.DB, playerID uint, week uint, year uint) ([]BoxScore, error) {
	var boxScores []BoxScore
	err := db.Preload("Matchup").
		Joins("JOIN matchups ON matchups.id = box_scores.matchup_id AND matchups.week = ? AND matchups.year = ?", week, year).
		Where("box_scores.player_id = ?", playerID).
		Find(&boxScores).Error
	return boxScores, err
}

// GetTeamBoxScoresByMatchup returns all box scores for a team in a specific matchup
func GetTeamBoxScoresByMatchup(db *gorm.DB, teamID uint, matchupID uint) ([]BoxScore, error) {
	var boxScores []BoxScore
	err := db.Where("team_id = ? AND matchup_id = ?", teamID, matchupID).Find(&boxScores).Error
	return boxScores, err
}
