package sleeper

import (
	"encoding/json"
	"strconv"
)

// FlexibleString unmarshals a JSON string or bare number as a string.
// Sleeper's API inconsistently returns some ID fields (espn_id, yahoo_id)
// as either a quoted string or a bare number depending on the player.
type FlexibleString string

func (s *FlexibleString) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*s = ""
		return nil
	}
	if b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = FlexibleString(v)
		return nil
	}
	*s = FlexibleString(b)
	return nil
}

type User struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar"`
}

type LeagueSettings struct {
	// Type encodes the league format: 0=redraft, 1=keeper, 2=dynasty.
	Type int `json:"type"`
}

type League struct {
	LeagueID        string             `json:"league_id"`
	Name            string             `json:"name"`
	Season          string             `json:"season"`
	Sport           string             `json:"sport"`
	Status          string             `json:"status"`
	TotalRosters    int                `json:"total_rosters"`
	Settings        LeagueSettings     `json:"settings"`
	ScoringSettings map[string]float64 `json:"scoring_settings"`
	RosterPositions []string           `json:"roster_positions"`
}

type LeagueUser struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar"`
}

type Draft struct {
	DraftID string `json:"draft_id"`
	Type    string `json:"type"`
	Status  string `json:"status"`
	Season  string `json:"season"`
}

type DraftPick struct {
	Round    int                    `json:"round"`
	PickNo   int                    `json:"pick_no"`
	RosterID int                    `json:"roster_id"`
	PickedBy string                 `json:"picked_by"`
	PlayerID string                 `json:"player_id"`
	Metadata map[string]interface{} `json:"metadata"`
}

type Transaction struct {
	TransactionID string         `json:"transaction_id"`
	Type          string         `json:"type"`
	Status        string         `json:"status"`
	Created       int64          `json:"created"`
	Leg           int            `json:"leg"`
	Adds          map[string]int `json:"adds"`
	Drops         map[string]int `json:"drops"`
	DraftPicks    []interface{}  `json:"draft_picks"`
	WaiverBudget  []interface{}  `json:"waiver_budget"`
	RosterIDs     []int          `json:"roster_ids"`
}

// Player is one entry from the map returned by GET /v1/players/nfl.
// The map key is the player_id; the struct duplicates it for convenience.
type Player struct {
	EspnID   FlexibleString `json:"espn_id"`
	YahooID  FlexibleString `json:"yahoo_id"`
	FullName string         `json:"full_name"`
	Position string         `json:"position"`
	Team     string         `json:"team"`
	Age      int            `json:"age"`
	YearsExp int            `json:"years_exp"`
	Active   *bool          `json:"active"`
	PlayerAvailability
}

// PlayerAvailability holds the status fields Sleeper attaches to a player in
// both the bulk player feed and the nested player object of projection rows.
// Pointers keep a null or absent field distinct from an empty value: a nil
// InjuryStatus means Sleeper reported no designation, not that the player is
// healthy.
type PlayerAvailability struct {
	Status          *string        `json:"status"`
	InjuryStatus    *string        `json:"injury_status"`
	InjuryStartDate *string        `json:"injury_start_date"`
	NewsUpdated     *NullableInt64 `json:"news_updated"`
}

// NullableInt64 decodes a JSON number or numeric string. null, absent and
// unparseable values leave it unset rather than failing the whole response,
// since one malformed timestamp should not drop a ~5MB player feed.
type NullableInt64 struct {
	Value int64
	Valid bool
}

func (n *NullableInt64) UnmarshalJSON(b []byte) error {
	*n = NullableInt64{}
	s := string(b)
	if len(s) >= 2 && s[0] == '"' {
		s = s[1 : len(s)-1]
	}
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		*n = NullableInt64{Value: v, Valid: true}
		return nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		*n = NullableInt64{Value: int64(f), Valid: true}
	}
	return nil
}

// ProjectionPlayer is the nested player object Sleeper embeds in each
// projection row. Metadata-only rows (no stats) still carry it.
type ProjectionPlayer struct {
	Team     *string `json:"team"`
	Position string  `json:"position"`
	PlayerAvailability
}

// ProjectionRow is the subset of a Sleeper projection row needed for player
// status. #229 owns decoding the projection values themselves.
type ProjectionRow struct {
	PlayerID string            `json:"player_id"`
	Company  string            `json:"company"`
	GameID   string            `json:"game_id"`
	Player   *ProjectionPlayer `json:"player"`
}

// NFLState is returned by GET /v1/state/nfl and describes the current NFL week/season,
// used to decide whether a fetched week's stats are final.
type NFLState struct {
	Season     string `json:"season"`
	SeasonType string `json:"season_type"`
	Week       int    `json:"week"`
}

// NotFoundError is returned when the Sleeper API responds with 404.
type NotFoundError struct{ URL string }

func (e *NotFoundError) Error() string { return "sleeper: not found: " + e.URL }
