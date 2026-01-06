package models

import (
	"time"

	"gorm.io/gorm"
)

type SleeperLeague struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	SleeperID string `json:"sleeperId" gorm:"uniqueIndex"`
	// LastScraped gets updated when all the users from a league are scraped and inserted
	LastScraped *time.Time `json:"lastScraped"`
}

type SleeperUser struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	SleeperID string `json:"sleeperId" gorm:"uniqueIndex"`
	// LastScraped gets updated when all the leagues for a user are scraped and inserted
	LastScraped *time.Time `json:"lastScraped"`
}

// SleeperTransaction represents any transaction (trade, waiver, free agent, etc.)
type SleeperTransaction struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	// Core transaction fields
	TransactionID string `json:"transactionId" gorm:"uniqueIndex"`
	Type          string `json:"type" gorm:"index"` // "trade", "waiver", "free_agent"
	Status        string `json:"status"`            // "complete", "pending", etc.
	Leg           int    `json:"leg"`               // Week number
	Creator       string `json:"creator"`           // User ID who initiated

	// Timestamps (milliseconds from Sleeper API)
	StatusUpdated int64 `json:"statusUpdated"`
	TransactionCreated int64 `json:"transactionCreated" gorm:"column:transaction_created"`

	// League relationship
	SleeperLeagueID string         `json:"sleeperLeagueId" gorm:"index"`
	League          *SleeperLeague `json:"league" gorm:"foreignKey:SleeperLeagueID;references:SleeperID"`

	// Arrays stored as PostgreSQL arrays or JSONB
	RosterIDs     []int  `json:"rosterIds" gorm:"type:integer[]"`     // Rosters involved
	ConsenterIDs  []int  `json:"consenterIds" gorm:"type:integer[]"`  // Who agreed

	// Complex data stored as JSONB for flexibility
	Adds         map[string]any   `json:"adds" gorm:"type:jsonb"`         // player_id -> roster_id
	Drops        map[string]any   `json:"drops" gorm:"type:jsonb"`        // player_id -> roster_id
	WaiverBudget []map[string]any `json:"waiverBudget" gorm:"type:jsonb"` // FAAB transfers
	Settings     map[string]any   `json:"settings" gorm:"type:jsonb"`     // Additional settings
	Metadata     map[string]any   `json:"metadata" gorm:"type:jsonb"`     // Extra metadata

	// Relationships
	DraftPicks []SleeperDraftPick `json:"draftPicks" gorm:"foreignKey:TransactionID;constraint:OnDelete:CASCADE"`
}

// SleeperDraftPick represents a draft pick involved in a trade
type SleeperDraftPick struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	// Foreign key to transaction
	TransactionID uint                `json:"transactionId" gorm:"index"`
	Transaction   *SleeperTransaction `json:"-" gorm:"foreignKey:TransactionID"`

	// Draft pick details
	Season           string `json:"season"`           // "2019", "2020", etc.
	Round            int    `json:"round"`            // Which round
	RosterID         int    `json:"rosterId"`         // Original owner's roster_id
	PreviousOwnerID  int    `json:"previousOwnerId"`  // Previous owner in this trade
	OwnerID          int    `json:"ownerId"`          // New owner after trade
}
