package models

import (
	"encoding/json"
	"time"
)

// Archive* mirror their cloud counterparts (see sleeper.go) but hold only
// the columns the archive DB stores: no FKs (arrival order must not matter
// during async replication), and no cloud-only claim/sync-bookkeeping
// columns (claimed_at, drafts_claimed_at, skipped_at, last_*_fetched_at,
// last_transaction_leg_fetched) — those are transient cloud sync state, not
// data, and no archive-side reader needs them. Distinct Go types (rather
// than reusing the cloud models) keep "what gets copied" an explicit,
// visible mapping in the replicate activities instead of an implicit field
// subset.

type ArchiveSleeperLeague struct {
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
	LeagueType      string          `gorm:"column:league_type"`
	ScoringSettings json.RawMessage `gorm:"column:scoring_settings;type:jsonb"`
	RosterPositions json.RawMessage `gorm:"column:roster_positions;type:jsonb"`
	CreatedAt       time.Time       `gorm:"column:created_at"`
	UpdatedAt       time.Time       `gorm:"column:updated_at"`
}

func (ArchiveSleeperLeague) TableName() string { return "sleeper_leagues" }

type ArchiveSleeperTransaction struct {
	SleeperTransactionID string          `gorm:"primaryKey;column:sleeper_transaction_id"`
	SleeperLeagueID      string          `gorm:"column:sleeper_league_id"`
	Type                 string          `gorm:"column:type"`
	Status               string          `gorm:"column:status"`
	CreatedAtSleeper      int64          `gorm:"column:created_at_sleeper"`
	Leg                  int             `gorm:"column:leg"`
	Adds                 json.RawMessage `gorm:"column:adds;type:jsonb"`
	Drops                json.RawMessage `gorm:"column:drops;type:jsonb"`
	DraftPicks           json.RawMessage `gorm:"column:draft_picks;type:jsonb"`
	WaiverBudget         json.RawMessage `gorm:"column:waiver_budget;type:jsonb"`
	CreatedAt            time.Time       `gorm:"column:created_at"`
}

func (ArchiveSleeperTransaction) TableName() string { return "sleeper_transactions" }

type ArchiveSleeperDraft struct {
	SleeperDraftID  string     `gorm:"primaryKey;column:sleeper_draft_id"`
	SleeperLeagueID string     `gorm:"column:sleeper_league_id"`
	Type            string     `gorm:"column:type"`
	Status          string     `gorm:"column:status"`
	Season          string     `gorm:"column:season"`
	LastFetchedAt   *time.Time `gorm:"column:last_fetched_at"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (ArchiveSleeperDraft) TableName() string { return "sleeper_drafts" }

type ArchiveSleeperDraftPick struct {
	SleeperDraftID  string          `gorm:"primaryKey;column:sleeper_draft_id"`
	Round           int             `gorm:"primaryKey;column:round"`
	PickNo          int             `gorm:"primaryKey;column:pick_no"`
	RosterID        int             `gorm:"column:roster_id"`
	PickedByUserID  string          `gorm:"column:picked_by_user_id"`
	SleeperPlayerID string          `gorm:"column:sleeper_player_id"`
	Metadata        json.RawMessage `gorm:"column:metadata;type:jsonb"`
}

func (ArchiveSleeperDraftPick) TableName() string { return "sleeper_draft_picks" }
