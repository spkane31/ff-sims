package etl

import (
	"time"
)

// YAMLTime is a custom time type that handles the datetime format used in YAML files
type YAMLTime struct {
	time.Time
}

// UnmarshalYAML implements custom unmarshaling for the YAML datetime format
func (t *YAMLTime) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var timeStr string
	if err := unmarshal(&timeStr); err != nil {
		return err
	}

	// Parse the datetime string in the format used in YAML: "2006-01-02 15:04:05"
	parsedTime, err := time.Parse("2006-01-02 15:04:05", timeStr)
	if err != nil {
		return err
	}

	t.Time = parsedTime
	return nil
}

// ETLPlayer represents a fantasy football player with ESPN identification
type ETLPlayer struct {
	ESPNID int    `json:"espn_id" yaml:"espn_id"`
	Name   string `json:"name" yaml:"name"`
}

// ETLTeam represents a fantasy football team
type ETLTeam struct {
	ESPNID int      `json:"espn_id" yaml:"espn_id"`
	Name   string   `json:"name" yaml:"name"`
	Owners []string `json:"owners" yaml:"owners"`
}

// ETLMatchup represents a single matchup between two teams
type ETLMatchup struct {
	Year       int    `json:"year" yaml:"year"`
	Week       int    `json:"week" yaml:"week"`
	HomeTeamID int    `json:"home_team_id" yaml:"home_team_id"`
	AwayTeamID int    `json:"away_team_id" yaml:"away_team_id"`
	GameType   string `json:"game_type" yaml:"game_type"`
	IsPlayoff  bool   `json:"is_playoff" yaml:"is_playoff"`
}

// ETLPlayerBoxscore represents a player's performance in a specific matchup
type ETLPlayerBoxscore struct {
	ETLPlayer                              // Embedded ETLPlayer struct
	Position        string                 `json:"position" yaml:"position"`
	Team            string                 `json:"team" yaml:"team"`
	Points          float64                `json:"points" yaml:"points"`
	ProjectedPoints float64                `json:"projected_points" yaml:"projected_points"`
	OnBye           bool                   `json:"on_bye" yaml:"on_bye"`
	ProOpponent     *string                `json:"pro_opponent" yaml:"pro_opponent"`
	ProPosRank      *int                   `json:"pro_pos_rank" yaml:"pro_pos_rank"`
	GamePlayed      *int                   `json:"game_played" yaml:"game_played"`
	GameDate        *string                `json:"game_date" yaml:"game_date"`
	ActiveStatus    *string                `json:"active_status" yaml:"active_status"`
	EligibleSlots   []string               `json:"eligible_slots" yaml:"eligible_slots"`
	OnTeamID        *int                   `json:"on_team_id" yaml:"on_team_id"`
	Injured         bool                   `json:"injured" yaml:"injured"`
	InjuryStatus    *string                `json:"injury_status" yaml:"injury_status"`
	PercentOwned    *float64               `json:"percent_owned" yaml:"percent_owned"`
	PercentStarted  *float64               `json:"percent_started" yaml:"percent_started"`
	Stats           map[string]interface{} `json:"stats" yaml:"stats"`
}

// ETLBoxscore represents a completed matchup with detailed scoring information
type ETLBoxscore struct {
	ETLMatchup                             // Embedded ETLMatchup struct
	HomeScore          float64             `json:"home_score" yaml:"home_score"`
	AwayScore          float64             `json:"away_score" yaml:"away_score"`
	HomeProjectedScore float64             `json:"home_projected_score" yaml:"home_projected_score"`
	AwayProjectedScore float64             `json:"away_projected_score" yaml:"away_projected_score"`
	HomeRoster         []ETLPlayerBoxscore `json:"home_roster" yaml:"home_roster"`
	AwayRoster         []ETLPlayerBoxscore `json:"away_roster" yaml:"away_roster"`
	Completed          bool                `json:"completed" yaml:"completed"`
}

// ETLBoxScorePlayerData represents individual player statistics for a specific week
type ETLBoxScorePlayerData struct {
	PlayerName      string  `json:"player_name" yaml:"player_name"`
	PlayerID        int     `json:"player_id" yaml:"player_id"`
	ProjectedPoints float64 `json:"projected_points" yaml:"projected_points"`
	ActualPoints    float64 `json:"actual_points" yaml:"actual_points"`
	PlayerPosition  string  `json:"player_position" yaml:"player_position"`
	Status          string  `json:"status" yaml:"status"`
	Week            int     `json:"week" yaml:"week"`
	Year            int     `json:"year" yaml:"year"`
	OwnerESPNID     int     `json:"owner_espn_id" yaml:"owner_espn_id"`
}

// ETLSchedule contains all matchup and scoring data for a season
type ETLSchedule struct {
	Matchups        []ETLMatchup            `json:"matchups" yaml:"matchups"`
	Boxscores       []ETLBoxscore           `json:"boxscores" yaml:"boxscores"`
	BoxScorePlayers []ETLBoxScorePlayerData `json:"box_score_players" yaml:"box_score_players"`
}

// ETLDraftPick represents a single draft selection
type ETLDraftPick struct {
	TeamID         int    `json:"team_id" yaml:"team_id"`
	Round          int    `json:"round" yaml:"round"`
	Pick           int    `json:"pick" yaml:"pick"`
	Keeper         bool   `json:"keeper" yaml:"keeper"`
	PlayerID       int    `json:"player_id" yaml:"player_id"`
	PlayerName     string `json:"player_name" yaml:"player_name"`
	PlayerPosition string `json:"player_position" yaml:"player_position"`
}

// ETLDraft contains all draft picks for a season
type ETLDraft struct {
	Year       int            `json:"year" yaml:"year"`
	Selections []ETLDraftPick `json:"selections" yaml:"selections"`
}

// TransactionType constants for different transaction types
const (
	TransactionTypeAdd          = "ADD"
	TransactionTypeDrop         = "DROP"
	TransactionTypeTrade        = "TRADE"
	TransactionTypeFreeAgentAdd = "FREE_AGENT_ADD"
)

// ETLAction represents a single action within a transaction
type ETLAction struct {
	TeamID         int    `json:"team_id" yaml:"team_id"`
	Type           string `json:"type" yaml:"type"`
	PlayerID       int    `json:"player_id" yaml:"player_id"`
	PlayerName     string `json:"player_name" yaml:"player_name"`
	PlayerPosition string `json:"player_position" yaml:"player_position"`
	BidAmount      int    `json:"bid_amount" yaml:"bid_amount"`
}

// ETLTransaction represents a fantasy transaction (waiver, trade, etc.)
type ETLTransaction struct {
	Actions []ETLAction `json:"actions" yaml:"actions"`
	Date    YAMLTime    `json:"date" yaml:"date"`
	Year    int         `json:"year" yaml:"year"`
}

// LeagueSource constants for different league data sources
const (
	LeagueSourceESPN    = "ESPN"
	LeagueSourceSleeper = "SLEEPER"
)

// ETLLeague represents a complete fantasy football league with all associated data
type ETLLeague struct {
	ID           int              `json:"id" yaml:"id"`
	Year         int              `json:"year" yaml:"year"`
	Teams        []ETLTeam        `json:"teams" yaml:"teams"`
	Schedule     ETLSchedule      `json:"schedule" yaml:"schedule"`
	Players      []ETLPlayer      `json:"players" yaml:"players"`
	Transactions []ETLTransaction `json:"transactions" yaml:"transactions"`
	Draft        ETLDraft         `json:"draft" yaml:"draft"`
	LeagueSource string           `json:"league_source" yaml:"league_source"`
}
