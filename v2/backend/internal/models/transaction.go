package models

import (
	"time"

	"gorm.io/gorm"
)

type DraftSelection struct {
	ID             uint           `json:"id" gorm:"primarykey"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
	PlayerName     string         `json:"player_name"`
	PlayerPosition string         `json:"player_position"` // QB, RB, WR, TE, K, DEF
	TeamID         uint           `json:"team_id"`
	PlayerID       int64          `json:"player_id"`
	Round          uint           `json:"round"`
	Pick           uint           `json:"pick"` // 1-based index
	Year           uint           `json:"year"`
	OwnerESPNID    uint           `json:"owner_espn_id"` // ESPN ID of the team owner
}
