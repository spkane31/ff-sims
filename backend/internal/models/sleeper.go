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
	ClaimedAt     *time.Time `gorm:"column:claimed_at"`
	SkippedAt     *time.Time `gorm:"column:skipped_at"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (SleeperUser) TableName() string { return "sleeper_users" }

type SleeperLeague struct {
	SleeperLeagueID           string          `gorm:"primaryKey;column:sleeper_league_id"`
	Name                      string          `gorm:"column:name"`
	Season                    string          `gorm:"column:season"`
	Sport                     string          `gorm:"column:sport"`
	Status                    string          `gorm:"column:status"`
	TotalRosters              int             `gorm:"column:total_rosters"`
	PPR                       *float64        `gorm:"column:ppr"`
	TEPremium                 *float64        `gorm:"column:te_premium"`
	IsSuperflex               *bool           `gorm:"column:is_superflex"`
	DraftType                 string          `gorm:"column:draft_type"`
	LeagueType                string          `gorm:"column:league_type"`
	ScoringSettings           json.RawMessage `gorm:"column:scoring_settings;type:jsonb"`
	RosterPositions           json.RawMessage `gorm:"column:roster_positions;type:jsonb"`
	LastFetchedAt             *time.Time      `gorm:"column:last_fetched_at"`
	LastDraftsFetchedAt       *time.Time      `gorm:"column:last_drafts_fetched_at"`
	LastTransactionsFetchedAt *time.Time      `gorm:"column:last_transactions_fetched_at"`
	LastTransactionLegFetched *int            `gorm:"column:last_transaction_leg_fetched"`
	ClaimedAt                 *time.Time      `gorm:"column:claimed_at"`
	DraftsClaimedAt           *time.Time      `gorm:"column:drafts_claimed_at"`
	DiscoveryClaimedAt        *time.Time      `gorm:"column:discovery_claimed_at"`
	SkippedAt                 *time.Time      `gorm:"column:skipped_at"`
	CreatedAt                 time.Time       `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                 time.Time       `gorm:"column:updated_at;autoUpdateTime"`
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

type SleeperPlayerWeekStat struct {
	Season          string          `gorm:"primaryKey;column:season"`
	Week            int             `gorm:"primaryKey;column:week"`
	SleeperPlayerID string          `gorm:"primaryKey;column:sleeper_player_id"`
	PtsPPR          *float64        `gorm:"column:pts_ppr"`
	PtsHalfPPR      *float64        `gorm:"column:pts_half_ppr"`
	PtsStd          *float64        `gorm:"column:pts_std"`
	Stats           json.RawMessage `gorm:"column:stats;type:jsonb"`
	CreatedAt       time.Time       `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time       `gorm:"column:updated_at;autoUpdateTime"`
}

func (SleeperPlayerWeekStat) TableName() string { return "sleeper_player_week_stats" }

type SleeperWeekStatFetch struct {
	Season        string     `gorm:"primaryKey;column:season"`
	Week          int        `gorm:"primaryKey;column:week"`
	LastFetchedAt *time.Time `gorm:"column:last_fetched_at"`
	Finalized     bool       `gorm:"column:finalized"`
}

func (SleeperWeekStatFetch) TableName() string { return "sleeper_week_stat_fetches" }

// SleeperLifetimeCount is one hourly snapshot row of data-scraping table
// sizes, written by cmd/cron's "lifetime-counts" job (internal/statscron).
// It exists because sleeper_transactions and sleeper_drafts are trimmed to a
// hot window by the scavenger's purge phase, and drafts are additionally
// routed straight to the archive DB at ingest once configured (see
// syncOneLeagueDrafts) — so a plain COUNT(*) against the cloud tables
// undercounts all-time totals, and there was previously no way to see growth
// over time at all. SnapshotAt is truncated to the hour so a retried run
// upserts the same row instead of duplicating it.
//
// Users/leagues columns are counted live from cloud (those tables are never
// purged, so a live COUNT is already exact). Transactions/drafts columns are
// counted from the archive DB (the full-history store, immune to purge) and
// are nil — not zero — for any snapshot taken while no archive DB is
// configured, so a local/dev run can't be mistaken for "genuinely zero
// trades/drafts ever happened."
//
// New columns can be added here (plus a migration) as more of the /admin
// page's counts are wanted; existing rows default new columns to 0/NULL,
// which is an acceptable gap for historical snapshots taken before the
// column existed.
type SleeperLifetimeCount struct {
	SnapshotAt time.Time `gorm:"primaryKey;column:snapshot_at"`

	UsersTotal    int64 `gorm:"column:users_total"`
	UsersExpanded int64 `gorm:"column:users_expanded"`
	UsersPending  int64 `gorm:"column:users_pending"`
	UsersSkipped  int64 `gorm:"column:users_skipped"`

	LeaguesTotal    int64 `gorm:"column:leagues_total"`
	LeaguesExpanded int64 `gorm:"column:leagues_expanded"`
	LeaguesPending  int64 `gorm:"column:leagues_pending"`
	LeaguesSkipped  int64 `gorm:"column:leagues_skipped"`

	TransactionsTotal *int64 `gorm:"column:transactions_total"`
	TradesCompleted   *int64 `gorm:"column:trades_completed"`
	DraftsCompleted   *int64 `gorm:"column:drafts_completed"`
}

func (SleeperLifetimeCount) TableName() string { return "sleeper_lifetime_counts" }
