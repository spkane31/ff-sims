package models

import (
	"time"

	"gorm.io/gorm"
)

// TeamNameHistory tracks the history of team name changes over time
type TeamNameHistory struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	TeamID    uint      `json:"team_id"`
	Name      string    `json:"name"`
	StartDate time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"` // NULL indicates this is the current name

	// Relationship
	Team *Team `json:"-"`
}
