package models

import (
	"encoding/json"
	"time"
)

type SleeperTransaction struct {
	SleeperTransactionID string          `gorm:"primaryKey;column:sleeper_transaction_id"`
	SleeperLeagueID      string          `gorm:"column:sleeper_league_id"`
	Type                 string          `gorm:"column:type"`
	Status               string          `gorm:"column:status"`
	CreatedAtSleeper     int64           `gorm:"column:created_at_sleeper"`
	Leg                  int             `gorm:"column:leg"`
	Adds                 json.RawMessage `gorm:"column:adds;type:jsonb"`
	Drops                json.RawMessage `gorm:"column:drops;type:jsonb"`
	DraftPicks           json.RawMessage `gorm:"column:draft_picks;type:jsonb"`
	WaiverBudget         json.RawMessage `gorm:"column:waiver_budget;type:jsonb"`
	CreatedAt            time.Time       `gorm:"column:created_at;autoCreateTime"`
}

func (SleeperTransaction) TableName() string { return "sleeper_transactions" }
