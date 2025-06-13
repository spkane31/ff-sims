package models

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Team represents a fantasy football team
type Team struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	Name     string  `json:"name"`
	Owner    string  `json:"owner_name"`
	ESPNID   uint    `json:"espn_id"` // gorm:"uniqueIndex:uni_teams_espn_id"`
	LeagueID uint    `json:"league_id"`
	Wins     int     `json:"wins" gorm:"default:0"`
	Losses   int     `json:"losses" gorm:"default:0"`
	Ties     int     `json:"ties" gorm:"default:0"`
	Points   float64 `json:"points" gorm:"default:0"`

	// // Relationships
	Players  []Player  `json:"players,omitempty" gorm:"many2many:team_players;"`
	Matchups []Matchup `json:"-" gorm:"foreignKey:HomeTeamID;references:ID"`
	// AwayMatchups []Matchup         `json:"-" gorm:"foreignKey:AwayTeamID;references:ID"`
	League      *League           `json:"league,omitempty"`
	SimResults  []SimResult       `json:"-"`
	NameHistory []TeamNameHistory `json:"name_history,omitempty" gorm:"foreignKey:TeamID"`
}

// AfterCreate hook is triggered after creating a new team
func (t *Team) AfterCreate(tx *gorm.DB) error {
	// When a team is first created, add the initial name to history
	if t.Name != "" {
		nameHistory := TeamNameHistory{
			TeamID:    t.ID,
			Name:      t.Name,
			StartDate: time.Now(),
		}
		if err := tx.Create(&nameHistory).Error; err != nil {
			return fmt.Errorf("failed to create initial team name history: %w", err)
		}
	}
	return nil
}

// BeforeUpdate hook is triggered before updating a team
func (t *Team) BeforeUpdate(tx *gorm.DB) error {
	// Check if name is being changed
	var oldTeam Team
	if err := tx.First(&oldTeam, t.ID).Error; err != nil {
		return err
	}

	// If the name has changed, update history
	if oldTeam.Name != t.Name && t.Name != "" {
		// Close previous name record
		var lastNameRecord TeamNameHistory
		if err := tx.Where("team_id = ? AND end_date IS NULL", t.ID).
			Order("start_date DESC").
			First(&lastNameRecord).Error; err == nil {
			now := time.Now()
			lastNameRecord.EndDate = &now
			if err := tx.Save(&lastNameRecord).Error; err != nil {
				return fmt.Errorf("failed to close last name record: %w", err)
			}
		}

		// Create new name record
		nameHistory := TeamNameHistory{
			TeamID:    t.ID,
			Name:      t.Name,
			StartDate: time.Now(),
		}
		if err := tx.Create(&nameHistory).Error; err != nil {
			return fmt.Errorf("failed to create new team name history: %w", err)
		}
	}
	return nil
}

// UpdateTeamName updates a team's name and records the change in history
func UpdateTeamName(db *gorm.DB, teamID uint, newName string) error {
	// Transaction to ensure both team update and history are consistent
	return db.Transaction(func(tx *gorm.DB) error {
		// Get the team
		var team Team
		if err := tx.First(&team, teamID).Error; err != nil {
			return err
		}

		// Only proceed if the name is actually changing
		if team.Name != newName {
			// Update the team's name
			team.Name = newName
			if err := tx.Save(&team).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// GetTeamNameHistory returns the complete name history for a team
func GetTeamNameHistory(db *gorm.DB, teamID uint) ([]TeamNameHistory, error) {
	var history []TeamNameHistory
	err := db.Where("team_id = ?", teamID).
		Order("start_date DESC").
		Find(&history).Error
	return history, err
}

// GetTeamNameAt returns what a team's name was at a specific point in time
func GetTeamNameAt(db *gorm.DB, teamID uint, date time.Time) (string, error) {
	var nameRecord TeamNameHistory
	err := db.Where("team_id = ? AND start_date <= ? AND (end_date IS NULL OR end_date >= ?)",
		teamID, date, date).
		Order("start_date DESC").
		First(&nameRecord).Error

	if err != nil {
		return "", err
	}
	return nameRecord.Name, nil
}
