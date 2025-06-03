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

	LeagueID                   uint    `json:"league_id"`
	Week                       int     `json:"week"`
	Year                       uint    `json:"year"`
	Season                     int     `json:"season"`
	HomeTeamID                 uint    `json:"home_team_id"`
	AwayTeamID                 uint    `json:"away_team_id"`
	HomeTeamESPNID             uint    `json:"home_team_espn_id"`
	AwayTeamESPNID             uint    `json:"away_team_espn_id"`
	HomeTeamFinalScore         float64 `json:"home_score"`
	AwayTeamFinalScore         float64 `json:"away_score"`
	HomeTeamESPNProjectedScore float64 `json:"home_projected_score"`
	AwayTeamESPNProjectedScore float64 `json:"away_projected_score"`

	Completed bool `json:"completed" gorm:"default:false"`
	IsPlayoff bool `json:"is_playoff" gorm:"default:false"`

	// Relationships
	League     *League     `json:"-"`
	HomeTeam   *Team       `json:"home_team,omitempty"`
	AwayTeam   *Team       `json:"away_team,omitempty"`
	SimResults []SimResult `json:"-"`
}
