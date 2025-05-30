package models

import (
	"time"

	"gorm.io/gorm"
)

// Team represents a fantasy football team
type Team struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	Name      string  `json:"name"`
	OwnerName string  `json:"owner_name"`
	LeagueID  uint    `json:"league_id"`
	Wins      int     `json:"wins" gorm:"default:0"`
	Losses    int     `json:"losses" gorm:"default:0"`
	Ties      int     `json:"ties" gorm:"default:0"`
	Points    float64 `json:"points" gorm:"default:0"`

	// Relationships
	Players      []Player    `json:"players,omitempty" gorm:"many2many:team_players;"`
	Matchups     []Matchup   `json:"-" gorm:"foreignKey:HomeTeamID;references:ID"`
	AwayMatchups []Matchup   `json:"-" gorm:"foreignKey:AwayTeamID;references:ID"`
	League       *League     `json:"league,omitempty"`
	SimResults   []SimResult `json:"-"`
}
