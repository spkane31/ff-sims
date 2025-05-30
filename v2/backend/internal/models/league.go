package models

import (
	"time"

	"gorm.io/gorm"
)

// League represents a fantasy football league
type League struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	Name         string `json:"name"`
	Description  string `json:"description"`
	ScoringType  string `json:"scoring_type"` // Standard, PPR, Half-PPR
	Teams        []Team `json:"teams,omitempty"`
	Season       int    `json:"season"`
	CurrentWeek  int    `json:"current_week"`
	TotalWeeks   int    `json:"total_weeks" gorm:"default:17"`
	PlayoffWeeks int    `json:"playoff_weeks" gorm:"default:3"`

	// Settings
	RosterSettings  RosterSettings  `json:"roster_settings" gorm:"embedded"`
	ScoringSettings ScoringSettings `json:"scoring_settings" gorm:"embedded"`

	// Relationships
	Matchups    []Matchup    `json:"matchups,omitempty"`
	Simulations []Simulation `json:"-"`
}

// RosterSettings defines the roster composition requirements
type RosterSettings struct {
	QB   int `json:"qb" gorm:"default:1"`
	RB   int `json:"rb" gorm:"default:2"`
	WR   int `json:"wr" gorm:"default:2"`
	TE   int `json:"te" gorm:"default:1"`
	FLEX int `json:"flex" gorm:"default:1"` // RB/WR/TE
	K    int `json:"k" gorm:"default:1"`
	DST  int `json:"dst" gorm:"default:1"`
	BN   int `json:"bn" gorm:"default:6"` // Bench spots
	IR   int `json:"ir" gorm:"default:1"` // Injured reserve
}

// ScoringSettings defines how fantasy points are calculated
type ScoringSettings struct {
	PassingYards    float64 `json:"passing_yards" gorm:"default:0.04"` // Points per passing yard
	PassingTD       float64 `json:"passing_td" gorm:"default:4"`
	Interception    float64 `json:"interception" gorm:"default:-2"`
	RushingYards    float64 `json:"rushing_yards" gorm:"default:0.1"`
	RushingTD       float64 `json:"rushing_td" gorm:"default:6"`
	Reception       float64 `json:"reception" gorm:"default:0"` // PPR=1, Half-PPR=0.5
	ReceivingYards  float64 `json:"receiving_yards" gorm:"default:0.1"`
	ReceivingTD     float64 `json:"receiving_td" gorm:"default:6"`
	Fumble          float64 `json:"fumble" gorm:"default:-2"`
	FieldGoal0to39  float64 `json:"field_goal_0to39" gorm:"default:3"`
	FieldGoal40to49 float64 `json:"field_goal_40to49" gorm:"default:4"`
	FieldGoal50plus float64 `json:"field_goal_50plus" gorm:"default:5"`
	ExtraPoint      float64 `json:"extra_point" gorm:"default:1"`
}
