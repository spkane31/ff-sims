package models

import (
	"time"

	"gorm.io/gorm"
)

// Simulation represents a fantasy football simulation run
type Simulation struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	LeagueID       uint   `json:"league_id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Season         int    `json:"season"`
	StartWeek      int    `json:"start_week"`
	EndWeek        int    `json:"end_week"`
	NumSimulations int    `json:"num_simulations" gorm:"default:1000"`
	Completed      bool   `json:"completed" gorm:"default:false"`

	// Simulation parameters
	VarFactor float64 `json:"var_factor" gorm:"default:1.0"` // Variance factor for simulations

	// Relationships
	League      *League         `json:"-"`
	Results     []SimResult     `json:"results,omitempty"`
	TeamResults []SimTeamResult `json:"team_results,omitempty"`
}

// SimResult represents a result of a single simulation matchup
type SimResult struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	SimulationID  uint    `json:"simulation_id"`
	MatchupID     uint    `json:"matchup_id"`
	TeamID        uint    `json:"team_id"`
	OpponentID    uint    `json:"opponent_id"`
	Score         float64 `json:"score"`
	OpponentScore float64 `json:"opponent_score"`
	Win           bool    `json:"win"`
	SimRun        int     `json:"sim_run"` // Which simulation run this result is from

	// Relationships
	Simulation *Simulation `json:"-"`
	Matchup    *Matchup    `json:"-"`
	Team       *Team       `json:"-"`
}

// SimTeamResult represents aggregated results for a team across simulations
type SimTeamResult struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	SimulationID     uint    `json:"simulation_id"`
	TeamID           uint    `json:"team_id"`
	Wins             int     `json:"wins"`
	Losses           int     `json:"losses"`
	PlayoffOdds      float64 `json:"playoff_odds"`
	ChampionshipOdds float64 `json:"championship_odds"`
	AvgPoints        float64 `json:"avg_points"`

	// Relationships
	Simulation *Simulation `json:"-"`
	Team       *Team       `json:"-"`
}
