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

	LeagueID   uint      `json:"league_id" gorm:"uniqueIndex:idx_matchup_unique"`
	Week       uint      `json:"week" gorm:"uniqueIndex:idx_matchup_unique"`
	Season     uint      `json:"season" gorm:"uniqueIndex:idx_matchup_unique"`
	HomeTeamID uint      `json:"home_team_id" gorm:"uniqueIndex:idx_matchup_unique"`
	AwayTeamID uint      `json:"away_team_id" gorm:"uniqueIndex:idx_matchup_unique"`
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
	return fmt.Sprintf("Matchup(ID=%d, Week=%d, Season=%d, HomeTeamID=%d, AwayTeamID=%d, HomeScore=%.2f, AwayScore=%.2f, Completed=%t)", m.ID, m.Week, m.Season, m.HomeTeamID, m.AwayTeamID, m.HomeTeamFinalScore, m.AwayTeamFinalScore, m.Completed)
}
