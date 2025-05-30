package models

import (
	"time"

	"gorm.io/gorm"
)

// Matchup represents a fantasy football matchup between two teams
type Matchup struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	LeagueID   uint    `json:"league_id"`
	Week       int     `json:"week"`
	Season     int     `json:"season"`
	HomeTeamID uint    `json:"home_team_id"`
	AwayTeamID uint    `json:"away_team_id"`
	HomeScore  float64 `json:"home_score"`
	AwayScore  float64 `json:"away_score"`
	Completed  bool    `json:"completed" gorm:"default:false"`
	IsPlayoff  bool    `json:"is_playoff" gorm:"default:false"`

	// Relationships
	League     *League     `json:"-"`
	HomeTeam   *Team       `json:"home_team,omitempty"`
	AwayTeam   *Team       `json:"away_team,omitempty"`
	SimResults []SimResult `json:"-"`
}
