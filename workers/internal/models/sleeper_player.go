package models

import "time"

type SleeperPlayer struct {
	SleeperPlayerID string     `gorm:"primaryKey;column:sleeper_player_id"`
	EspnID          string     `gorm:"column:espn_id"`
	YahooID         string     `gorm:"column:yahoo_id"`
	FullName        string     `gorm:"column:full_name"`
	Position        string     `gorm:"column:position"`
	NflTeam         string     `gorm:"column:nfl_team"`
	Age             int        `gorm:"column:age"`
	YearsExp        int        `gorm:"column:years_exp"`
	LastFetchedAt   *time.Time `gorm:"column:last_fetched_at"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (SleeperPlayer) TableName() string { return "sleeper_players" }
