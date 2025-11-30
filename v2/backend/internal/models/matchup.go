package models

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Matchup represents a fantasy football matchup between two teams
type Matchup struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	LeagueID   uint      `json:"league_id"`
	Week       uint      `json:"week"`
	Year       uint      `json:"year"`
	Season     int       `json:"season"`
	HomeTeamID uint      `json:"home_team_id"`
	AwayTeamID uint      `json:"away_team_id"`
	GameDate   time.Time `json:"game_date"`
	GameType   string    `json:"game_type"`

	// Score information
	HomeTeamFinalScore         float64 `json:"home_score"`
	AwayTeamFinalScore         float64 `json:"away_score"`
	HomeTeamESPNProjectedScore float64 `json:"home_projected_score"`
	AwayTeamESPNProjectedScore float64 `json:"away_projected_score"`

	// Expected wins
	// HomeTeamExpectedWin float64 `json:"home_expected_win"`
	// AwayTeamExpectedWin float64 `json:"away_expected_win"`

	// Status flags
	Completed bool `json:"completed" gorm:"default:false"`
	IsPlayoff bool `json:"is_playoff" gorm:"default:false"`

	// Relationships
	League     *League     `json:"-"`
	HomeTeam   *Team       `json:"home_team,omitempty" gorm:"foreignKey:HomeTeamID"`
	AwayTeam   *Team       `json:"away_team,omitempty" gorm:"foreignKey:AwayTeamID"`
	BoxScores  []BoxScore  `json:"box_scores,omitempty" gorm:"foreignKey:MatchupID"`
	SimResults []SimResult `json:"-"`
}

func (m *Matchup) String() string {
	return fmt.Sprintf("Matchup(ID=%d, Week=%d, Year=%d, HomeTeamID=%d, AwayTeamID=%d, HomeScore=%.2f, AwayScore=%.2f, Completed=%t)", m.ID, m.Week, m.Year, m.HomeTeamID, m.AwayTeamID, m.HomeTeamFinalScore, m.AwayTeamFinalScore, m.Completed)
}
