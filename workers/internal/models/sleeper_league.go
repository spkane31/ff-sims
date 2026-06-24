package models

import (
	"encoding/json"
	"time"
)

type SleeperLeague struct {
	SleeperLeagueID string          `gorm:"primaryKey;column:sleeper_league_id"`
	Name            string          `gorm:"column:name"`
	Season          string          `gorm:"column:season"`
	Sport           string          `gorm:"column:sport"`
	Status          string          `gorm:"column:status"`
	TotalRosters    int             `gorm:"column:total_rosters"`
	PPR             *float64        `gorm:"column:ppr"`
	TEPremium       *float64        `gorm:"column:te_premium"`
	IsSuperflex     *bool           `gorm:"column:is_superflex"`
	ScoringSettings json.RawMessage `gorm:"column:scoring_settings;type:jsonb"`
	RosterPositions json.RawMessage `gorm:"column:roster_positions;type:jsonb"`
	LastFetchedAt   *time.Time      `gorm:"column:last_fetched_at"`
	SkippedAt       *time.Time      `gorm:"column:skipped_at"`
	CreatedAt       time.Time       `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time       `gorm:"column:updated_at;autoUpdateTime"`
}

func (SleeperLeague) TableName() string { return "sleeper_leagues" }

type SleeperLeagueUser struct {
	SleeperLeagueID string `gorm:"primaryKey;column:sleeper_league_id"`
	SleeperUserID   string `gorm:"primaryKey;column:sleeper_user_id"`
}

func (SleeperLeagueUser) TableName() string { return "sleeper_league_users" }
