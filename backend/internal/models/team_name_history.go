package models

import (
	"time"

	"gorm.io/gorm"
)

// TeamNameHistory tracks the history of team name changes over time
type TeamNameHistory struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	TeamID    uint       `json:"teamId"`
	Name      string     `json:"name"`
	StartDate time.Time  `json:"startDate"`
	EndDate   *time.Time `json:"endDate"` // NULL indicates this is the current name

	// Relationship
	Team *Team `json:"-"`
}
