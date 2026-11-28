package models

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Projection capture triggers.
const (
	ProjectionTriggerDailySweep = "daily_sweep"
	ProjectionTriggerPregame    = "pregame"
	ProjectionTriggerManual     = "manual"
)

// Week phases a capture can be taken in, judged against nfl_kickoffs.
const (
	// WeekPhasePregame: every game has a verified kickoff after capture time.
	WeekPhasePregame = "pregame"
	// WeekPhaseInProgress: some verified kickoffs have passed. Rows are
	// replayable only for games whose kickoff follows capture time.
	WeekPhaseInProgress = "in_progress"
	// WeekPhaseFuture: a later week without full verified kickoffs. Replay
	// eligibility stays unset until compared with verified kickoffs.
	WeekPhaseFuture = "future"
	// WeekPhaseUnverified: the current or a past week without full verified
	// kickoffs. Not published for replay.
	WeekPhaseUnverified = "unverified"
)

// Projection row kinds. An absent player has no row at all.
const (
	ProjectionRowKindProjection   = "projection"
	ProjectionRowKindMetadataOnly = "metadata_only"
)

// NFLKickoff is one team's kickoff for a week, supplied by an operator from
// the official NFL schedule until validated schedule revisions exist.
type NFLKickoff struct {
	Season    string    `gorm:"primaryKey;column:season"`
	Week      int       `gorm:"primaryKey;column:week"`
	Team      string    `gorm:"primaryKey;column:team"`
	Opponent  string    `gorm:"column:opponent"`
	KickoffAt time.Time `gorm:"column:kickoff_at"`
	Verified  bool      `gorm:"column:verified"`
	Source    string    `gorm:"column:source"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (NFLKickoff) TableName() string { return "nfl_kickoffs" }

// SleeperProjectionCapture is one published projection pull for a week.
type SleeperProjectionCapture struct {
	ID               int64           `gorm:"primaryKey;autoIncrement;column:id"`
	CaptureKey       string          `gorm:"column:capture_key;uniqueIndex"`
	Season           string          `gorm:"column:season"`
	TargetWeek       int             `gorm:"column:target_week"`
	CapturedAt       time.Time       `gorm:"column:captured_at"`
	Source           string          `gorm:"column:source"`
	Trigger          string          `gorm:"column:trigger"`
	WindowKickoffAt  *time.Time      `gorm:"column:window_kickoff_at"`
	WeekPhase        string          `gorm:"column:week_phase"`
	ReplayEligible   *bool           `gorm:"column:replay_eligible"`
	ReplayNote       string          `gorm:"column:replay_note"`
	RowCount         int             `gorm:"column:row_count"`
	CompanyCounts    json.RawMessage `gorm:"column:company_counts;type:jsonb"`
	MissingCompanies json.RawMessage `gorm:"column:missing_companies;type:jsonb"`
	CreatedAt        time.Time       `gorm:"column:created_at;autoCreateTime"`
}

func (SleeperProjectionCapture) TableName() string { return "sleeper_projection_captures" }

// SleeperProjectionCaptureGame records the kickoff cutoff a capture's game was
// judged against.
type SleeperProjectionCaptureGame struct {
	CaptureID       int64      `gorm:"primaryKey;column:capture_id"`
	GameID          string     `gorm:"primaryKey;column:game_id"`
	KickoffAt       *time.Time `gorm:"column:kickoff_at"`
	KickoffVerified bool       `gorm:"column:kickoff_verified"`
	ReplayEligible  *bool      `gorm:"column:replay_eligible"`
}

func (SleeperProjectionCaptureGame) TableName() string { return "sleeper_projection_capture_games" }

// SleeperPlayerProjection is one provider's projection for a player in a capture.
type SleeperPlayerProjection struct {
	CaptureID            int64           `gorm:"primaryKey;column:capture_id"`
	SleeperPlayerID      string          `gorm:"primaryKey;column:sleeper_player_id"`
	Company              string          `gorm:"primaryKey;column:company"`
	RowKind              string          `gorm:"column:row_kind"`
	Position             string          `gorm:"column:position"`
	NflTeam              string          `gorm:"column:nfl_team"`
	Opponent             string          `gorm:"column:opponent"`
	GameID               string          `gorm:"column:game_id"`
	ProviderLastModified *time.Time      `gorm:"column:provider_last_modified"`
	PtsPPR               *float64        `gorm:"column:pts_ppr"`
	PtsHalfPPR           *float64        `gorm:"column:pts_half_ppr"`
	PtsStd               *float64        `gorm:"column:pts_std"`
	Stats                json.RawMessage `gorm:"column:stats;type:jsonb"`
}

func (SleeperPlayerProjection) TableName() string { return "sleeper_player_projections" }

// SleeperPlayerStatusObservation is a player's status as seen in a
// current-week projection response.
type SleeperPlayerStatusObservation struct {
	CaptureID       int64      `gorm:"primaryKey;column:capture_id"`
	SleeperPlayerID string     `gorm:"primaryKey;column:sleeper_player_id"`
	ObservedAt      time.Time  `gorm:"column:observed_at"`
	Source          string     `gorm:"column:source"`
	NflTeam         string     `gorm:"column:nfl_team"`
	InjuryStatus    string     `gorm:"column:injury_status"`
	InjuryStartDate string     `gorm:"column:injury_start_date"`
	NewsUpdatedAt   *time.Time `gorm:"column:news_updated_at"`
}

func (SleeperPlayerStatusObservation) TableName() string { return "sleeper_player_status_observations" }

// ErrProjectionCompanyRequired is returned when a projection query omits the
// provider, since rows from different companies are not interchangeable.
var ErrProjectionCompanyRequired = errors.New("projection query requires a company")

// ReplayProjections returns company's projection rows for season/week from
// the latest replay-eligible capture taken before asOf, keeping only rows
// whose game had a verified kickoff after that capture. It returns nil when no
// eligible capture exists.
func ReplayProjections(db *gorm.DB, season string, week int, company string, asOf time.Time) ([]SleeperPlayerProjection, error) {
	if company == "" {
		return nil, ErrProjectionCompanyRequired
	}
	var capture SleeperProjectionCapture
	err := db.Where("season = ? AND target_week = ? AND replay_eligible = ? AND captured_at < ?", season, week, true, asOf).
		Order("captured_at DESC").
		First(&capture).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rows []SleeperPlayerProjection
	err = db.Table("sleeper_player_projections AS p").
		Select("p.*").
		Joins("JOIN sleeper_projection_capture_games AS g ON g.capture_id = p.capture_id AND g.game_id = p.game_id").
		Where("p.capture_id = ? AND p.company = ? AND g.replay_eligible = ?", capture.ID, company, true).
		Order("p.sleeper_player_id").
		Find(&rows).Error
	return rows, err
}
