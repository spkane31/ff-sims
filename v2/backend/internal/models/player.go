package models

import (
	"time"

	"gorm.io/gorm"
)

// Player represents a football player
type Player struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	ESPNID        int64   `json:"espn_id" gorm:"index:idx_players_espn_id,unique"` // Unique ESPN ID for the player
	Name          string  `json:"name"`
	Position      string  `json:"position"` // QB, RB, WR, TE, K, DEF
	Team          string  `json:"team"`     // NFL team abbreviation
	FantasyPoints float64 `json:"fantasy_points" gorm:"default:0"`
	Status        string  `json:"status"` // Active, Injured, etc.

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

// PlayerGameStats represents a player's stats for a specific game
type PlayerGameStats struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	PlayerID      uint        `json:"player_id"`
	PlayerName    string      `json:"player_name"`
	Week          int         `json:"week"`
	Season        int         `json:"season"`
	GameStats     PlayerStats `json:"game_stats" gorm:"embedded"`
	FantasyPoints float64     `json:"fantasy_points"`

	// Relationships
	Player *Player `json:"-"`
}

// GetPlayerByESPNID retrieves a player by their ESPN ID
func GetPlayerByESPNID(db *gorm.DB, espnID int64) (*Player, error) {
	var player Player
	err := db.Where("espn_id = ?", espnID).First(&player).Error
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
