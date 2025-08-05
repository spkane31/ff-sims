package models

import (
	"time"

	"gorm.io/gorm"
)

type DraftSelection struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	PlayerID       uint   `json:"player_id"` // Reference to Player model
	PlayerName     string `json:"player_name"`
	PlayerPosition string `json:"player_position"` // QB, RB, WR, TE, K, DEF
	TeamID         uint   `json:"team_id"`         // Team drafting the player
	Round          uint   `json:"round"`
	Pick           uint   `json:"pick"` // 1-based index
	Year           uint   `json:"year"` // Draft year
	LeagueID       uint   `json:"league_id"`

	// Relationships
	Team   *Team   `json:"team,omitempty"`
	Player *Player `json:"player,omitempty"`
	League *League `json:"league,omitempty"`
}

// Transaction represents various team transactions like adding/dropping/trading players
type Transaction struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	TeamID          uint      `json:"team_id"`
	PlayerID        uint      `json:"player_id"`        // References Player model
	TransactionType string    `json:"transaction_type"` // ADDED, DROPPED, TRADED
	PlayerName      string    `json:"player_name"`      // Denormalized for quick access
	BidAmount       int       `json:"bid_amount"`       // FAAB bid amount (for waiver claims)
	Date            time.Time `json:"date"`             // When the transaction occurred
	Year            uint      `json:"year"`             // Season year
	Week            uint      `json:"week"`             // Week of transaction
	LeagueID        uint      `json:"league_id"`

	// For trades
	RelatedTransactionID *uint  `json:"related_transaction_id,omitempty"` // For linking trade partners
	TradePartnerTeamID   *uint  `json:"trade_partner_team_id,omitempty"`  // Only for TRADED type
	Notes                string `json:"notes,omitempty"`                  // Additional transaction info

	// Relationships
	Team         *Team        `json:"team,omitempty"`
	Player       *Player      `json:"player,omitempty"`
	League       *League      `json:"league,omitempty"`
	RelatedTrade *Transaction `json:"related_trade,omitempty" gorm:"foreignKey:RelatedTransactionID"`
}

// GetTeamTransactions returns all transactions for a team in a specific season
func GetTeamTransactions(db *gorm.DB, teamID uint, year uint) ([]Transaction, error) {
	var transactions []Transaction
	err := db.Where("team_id = ? AND year = ?", teamID, year).
		Order("date desc").
		Find(&transactions).Error
	return transactions, err
}

// GetPlayerTransactions returns all transactions involving a player in a specific season
func GetPlayerTransactions(db *gorm.DB, playerID uint, year uint) ([]Transaction, error) {
	var transactions []Transaction
	err := db.Where("player_id = ? AND year = ?", playerID, year).
		Order("date desc").
		Find(&transactions).Error
	return transactions, err
}

// GetTeamDraftSelections returns all draft selections for a team in a specific season
func GetTeamDraftSelections(db *gorm.DB, teamID uint, year uint) ([]DraftSelection, error) {
	var selections []DraftSelection
	err := db.Where("team_id = ? AND year = ?", teamID, year).
		Order("round asc, pick asc").
		Find(&selections).Error
	return selections, err
}

// GetLeagueDraftSelections returns all draft selections for a league in a specific season
func GetLeagueDraftSelections(db *gorm.DB, leagueID uint, year uint) ([]DraftSelection, error) {
	var selections []DraftSelection
	err := db.Where("league_id = ? AND year = ?", leagueID, year).
		Order("round asc, pick asc").
		Preload("Team").
		Preload("Player").
		Find(&selections).Error
	return selections, err
}
