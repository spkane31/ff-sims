package models

import (
	"time"

	"gorm.io/gorm"
)

// Player represents a football player
type Player struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	// Platform IDs
	ESPNID        int64  `json:"espn_id" gorm:"index:idx_players_espn_id"`              // ESPN ID for the player
	SleeperID     string `json:"sleeper_id" gorm:"index:idx_players_sleeper_id,unique"` // Unique Sleeper ID
	YahooID       string `json:"yahoo_id,omitempty"`
	FantasyDataID int    `json:"fantasy_data_id,omitempty"`
	RotoworldID   int    `json:"rotoworld_id,omitempty"`
	RotowireID    string `json:"rotowire_id,omitempty"`
	SportsradarID string `json:"sportsradar_id,omitempty"`
	StatsID       string `json:"stats_id,omitempty"`

	// Basic Info
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Name      string `json:"name"`    // Full name for compatibility
	Number    int    `json:"number"`  // Jersey number
	Hashtag   string `json:"hashtag"` // Social media hashtag

	// Position & Team
	Position         string `json:"position"`                           // Primary position: QB, RB, WR, TE, K, DEF
	FantasyPositions string `json:"fantasy_positions" gorm:"type:text"` // JSON array of eligible positions
	Team             string `json:"team"`                               // NFL team abbreviation
	Status           string `json:"status"`                             // Active, Injured, etc.
	Sport            string `json:"sport" gorm:"default:'nfl'"`

	// Biographical Info
	Age          int    `json:"age,omitempty"`
	Height       string `json:"height,omitempty"` // e.g., "6'2""
	Weight       string `json:"weight,omitempty"` // e.g., "225"
	College      string `json:"college,omitempty"`
	YearsExp     int    `json:"years_exp,omitempty"`
	BirthCountry string `json:"birth_country,omitempty"`

	// Depth Chart & Rankings
	DepthChartPosition int `json:"depth_chart_position,omitempty"`
	DepthChartOrder    int `json:"depth_chart_order,omitempty"`
	SearchRank         int `json:"search_rank,omitempty"` // Sleeper's search ranking

	// Injury Tracking
	InjuryStatus          string `json:"injury_status,omitempty"`
	InjuryStartDate       string `json:"injury_start_date,omitempty"`
	PracticeParticipation string `json:"practice_participation,omitempty"`

	// Fantasy Stats
	FantasyPoints float64 `json:"fantasy_points" gorm:"default:0"`

	// Base stats - these represent career or season totals
	Stats PlayerStats `json:"stats" gorm:"embedded"`

	// Relationships
	Teams     []Team     `json:"-" gorm:"many2many:team_players;"`
	BoxScores []BoxScore `json:"box_scores,omitempty" gorm:"foreignKey:PlayerID"`
}

// PlayerStats represents the statistical categories for a player
type PlayerStats struct {
	PassingYards   float64 `json:"passing_yards" gorm:"default:0"`
	PassingTDs     float64 `json:"passing_tds" gorm:"default:0"`
	Interceptions  float64 `json:"float64erceptions" gorm:"default:0"`
	RushingYards   float64 `json:"rushing_yards" gorm:"default:0"`
	RushingTDs     float64 `json:"rushing_tds" gorm:"default:0"`
	Receptions     float64 `json:"receptions" gorm:"default:0"`
	ReceivingYards float64 `json:"receiving_yards" gorm:"default:0"`
	ReceivingTDs   float64 `json:"receiving_tds" gorm:"default:0"`
	Fumbles        float64 `json:"fumbles" gorm:"default:0"`
	FieldGoals     float64 `json:"field_goals" gorm:"default:0"`
	ExtraPoints    float64 `json:"extra_points" gorm:"default:0"`
}

// GetPlayerByESPNID retrieves a player by their ESPN ID
func GetPlayerByESPNID(db *gorm.DB, espnID int64) (*Player, error) {
	var player Player
	err := db.Where("espn_id = ?", espnID).First(&player).Error
	return &player, err
}

// GetPlayerBySleeperID retrieves a player by their Sleeper ID
func GetPlayerBySleeperID(db *gorm.DB, sleeperID string) (*Player, error) {
	var player Player
	err := db.Where("sleeper_id = ?", sleeperID).First(&player).Error
	return &player, err
}

// GetPlayerBoxScores retrieves all box scores for a player in a specific season
func GetPlayerBoxScores(db *gorm.DB, playerID uint, year uint) ([]BoxScore, error) {
	var boxScores []BoxScore
	err := db.Where("player_id = ? AND year = ?", playerID, year).Order("week asc").Find(&boxScores).Error
	return boxScores, err
}

// GetAllPlayersByTeam retrieves all players for a specific team in a season
func GetAllPlayersByTeam(db *gorm.DB, teamID uint, year uint) ([]Player, error) {
	var players []Player
	err := db.Joins("JOIN team_players ON team_players.player_id = players.id").
		Where("team_players.team_id = ?", teamID).
		Find(&players).Error
	return players, err
}
