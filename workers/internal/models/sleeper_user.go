package models

import "time"

type SleeperUser struct {
	SleeperUserID string     `gorm:"primaryKey;column:sleeper_user_id"`
	Username      string     `gorm:"column:username"`
	DisplayName   string     `gorm:"column:display_name"`
	Avatar        string     `gorm:"column:avatar"`
	LastFetchedAt *time.Time `gorm:"column:last_fetched_at"`
	SkippedAt     *time.Time `gorm:"column:skipped_at"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (SleeperUser) TableName() string { return "sleeper_users" }
