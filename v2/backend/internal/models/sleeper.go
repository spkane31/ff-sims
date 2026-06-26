package models

import (
	"encoding/json"
	"time"
)

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
	DraftType       string          `gorm:"column:draft_type"`
	ScoringSettings json.RawMessage `gorm:"column:scoring_settings;type:jsonb"`
	RosterPositions json.RawMessage `gorm:"column:roster_positions;type:jsonb"`
	LastFetchedAt             *time.Time      `gorm:"column:last_fetched_at"`
	LastDraftsFetchedAt       *time.Time      `gorm:"column:last_drafts_fetched_at"`
	LastTransactionsFetchedAt *time.Time      `gorm:"column:last_transactions_fetched_at"`
	SkippedAt                 *time.Time      `gorm:"column:skipped_at"`
	CreatedAt       time.Time       `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time       `gorm:"column:updated_at;autoUpdateTime"`
}

func (SleeperLeague) TableName() string { return "sleeper_leagues" }

type SleeperLeagueUser struct {
	SleeperLeagueID string `gorm:"primaryKey;column:sleeper_league_id"`
	SleeperUserID   string `gorm:"primaryKey;column:sleeper_user_id"`
}

func (SleeperLeagueUser) TableName() string { return "sleeper_league_users" }

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
