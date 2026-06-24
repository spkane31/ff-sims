package models

import (
	"encoding/json"
	"time"
)

type SleeperDraft struct {
	SleeperDraftID  string     `gorm:"primaryKey;column:sleeper_draft_id"`
	SleeperLeagueID string     `gorm:"column:sleeper_league_id"`
	Type            string     `gorm:"column:type"`
	Status          string     `gorm:"column:status"`
	Season          string     `gorm:"column:season"`
	LastFetchedAt   *time.Time `gorm:"column:last_fetched_at"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (SleeperDraft) TableName() string { return "sleeper_drafts" }

type SleeperDraftPick struct {
	SleeperDraftID  string          `gorm:"primaryKey;column:sleeper_draft_id"`
	Round           int             `gorm:"primaryKey;column:round"`
	PickNo          int             `gorm:"primaryKey;column:pick_no"`
	RosterID        int             `gorm:"column:roster_id"`
	PickedByUserID  string          `gorm:"column:picked_by_user_id"`
	SleeperPlayerID string          `gorm:"column:sleeper_player_id"`
	Metadata        json.RawMessage `gorm:"column:metadata;type:jsonb"`
}

func (SleeperDraftPick) TableName() string { return "sleeper_draft_picks" }
