package sleeper

import (
	"backend/internal/models"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

var ErrNotImplemented = fmt.Errorf("not implemented")

// FlexInt handles JSON fields that might be either a number or a string
type FlexInt struct {
	Valid bool
	Int   int
}

// UnmarshalJSON implements custom unmarshaling for FlexInt
func (f *FlexInt) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as int first
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		f.Int = i
		f.Valid = true
		return nil
	}

	// Try to unmarshal as string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s == "" {
			f.Valid = false
			return nil
		}
		if val, err := strconv.Atoi(s); err == nil {
			f.Int = val
			f.Valid = true
			return nil
		}
	}

	f.Valid = false
	return nil
}

const baseURL = "https://api.sleeper.app/v1"

type Client struct{}

func New() *Client {
	return &Client{}
}

// get performs a GET request to the Sleeper API and unmarshals the response into the provided type
func (c *Client) get(endpoint string, v any) error {
	url := fmt.Sprintf("%s/%s", baseURL, endpoint)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch from %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d for %s", resp.StatusCode, endpoint)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return nil
}

type League struct {
	TotalRosters int    `json:"total_rosters"`
	Sport        string `json:"sport"`
	LeagueID     string `json:"league_id"`
}

func (c *Client) GetLeague(leagueID uint64) (League, error) {
	var league League
	endpoint := fmt.Sprintf("league/%d", leagueID)
	if err := c.get(endpoint, &league); err != nil {
		return League{}, err
	}
	return league, nil
}

type User struct {
	UserID         string         `json:"user_id"`
	Username       string         `json:"username"`
	DisplayName    string         `json:"display_name"`
	Avatar         string         `json:"avatar"`
	Metadata       map[string]any `json:"metadata"`
	IsOwner        bool           `json:"is_owner"`
	IsCommissioner bool           `json:"is_commissioner"`
}

// Transaction represents any transaction from the Sleeper API (trade, waiver, free_agent)
// This type is strongly typed and handles all transaction types
type Transaction struct {
	// Core fields (always present)
	Type          string `json:"type"`           // "trade", "free_agent", "waiver"
	TransactionID string `json:"transaction_id"` // Unique transaction ID
	StatusUpdated int64  `json:"status_updated"` // Timestamp in milliseconds
	Status        string `json:"status"`         // "complete", "pending", etc.
	RosterIDs     []int  `json:"roster_ids"`     // Roster IDs involved
	Leg           int    `json:"leg"`            // Week number
	Creator       string `json:"creator"`        // User ID who initiated
	Created       int64  `json:"created"`        // Creation timestamp in milliseconds
	ConsenterIDs  []int  `json:"consenter_ids"`  // Roster IDs who agreed

	// Optional fields (nil when not present/null)
	Settings     *TransactionSettings `json:"settings"`      // Used for waivers (FAAB bid amount)
	Metadata     map[string]any       `json:"metadata"`      // Additional metadata, can contain notes
	Adds         map[string]int       `json:"adds"`          // player_id -> roster_id (who added the player)
	Drops        map[string]int       `json:"drops"`         // player_id -> roster_id (who dropped the player)
	DraftPicks   []DraftPick          `json:"draft_picks"`   // Draft picks involved (trades only)
	WaiverBudget []WaiverBudgetItem   `json:"waiver_budget"` // FAAB transfers in trade
}

// TransactionSettings contains optional settings for transactions (primarily waivers)
type TransactionSettings struct {
	WaiverBid *int `json:"waiver_bid,omitempty"` // FAAB amount bid for waiver claims
}

// DraftPick represents a draft pick involved in a trade
type DraftPick struct {
	Season          string `json:"season"`            // Year of the draft pick (e.g., "2019")
	Round           int    `json:"round"`             // Round number
	RosterID        int    `json:"roster_id"`         // Original owner's roster ID
	PreviousOwnerID int    `json:"previous_owner_id"` // Previous owner in this trade
	OwnerID         int    `json:"owner_id"`          // New owner after trade
}

// WaiverBudgetItem represents FAAB dollars transferred in a trade
type WaiverBudgetItem struct {
	Sender   int `json:"sender"`   // Roster ID sending FAAB
	Receiver int `json:"receiver"` // Roster ID receiving FAAB
	Amount   int `json:"amount"`   // Amount of FAAB transferred
}

// Helper methods for type checking
func (t *Transaction) IsTrade() bool {
	return t.Type == "trade"
}

func (t *Transaction) IsWaiver() bool {
	return t.Type == "waiver"
}

func (t *Transaction) IsFreeAgent() bool {
	return t.Type == "free_agent"
}

func (c *Client) GetUsersInLeague(leagueID string) ([]User, error) {
	var users []User
	endpoint := fmt.Sprintf("league/%s/users", leagueID)
	if err := c.get(endpoint, &users); err != nil {
		return nil, err
	}
	return users, nil
}

func (c *Client) GetAllLeaguesForUser(userID string, sport, season string) ([]League, error) {
	var leagues []League
	endpoint := fmt.Sprintf("user/%s/leagues/%s/%s", userID, sport, season)
	if err := c.get(endpoint, &leagues); err != nil {
		return nil, err
	}
	return leagues, nil
}

func (c *Client) GetTransactions(leagueID string, round int) ([]Transaction, error) {
	var transactions []Transaction
	endpoint := fmt.Sprintf("league/%s/transactions/%d", leagueID, round)
	if err := c.get(endpoint, &transactions); err != nil {
		return nil, err
	}
	return transactions, nil
}

// Player represents a player from the Sleeper API
type Player struct {
	PlayerID              string   `json:"player_id"`
	FirstName             string   `json:"first_name"`
	LastName              string   `json:"last_name"`
	FullName              string   `json:"full_name,omitempty"`
	Position              string   `json:"position"`
	Team                  *string  `json:"team"`
	Number                *int     `json:"number"`
	Status                string   `json:"status"`
	Sport                 string   `json:"sport"`
	FantasyPositions      []string `json:"fantasy_positions"`
	Age                   *int     `json:"age"`
	Height                *string  `json:"height"`
	Weight                *string  `json:"weight"`
	College               *string  `json:"college"`
	YearsExp              *int     `json:"years_exp"`
	BirthCountry          *string  `json:"birth_country"`
	Hashtag               *string  `json:"hashtag"`
	DepthChartPosition    *FlexInt `json:"depth_chart_position"`
	DepthChartOrder       *FlexInt `json:"depth_chart_order"`
	SearchRank            *int     `json:"search_rank"`
	SearchFirstName       string   `json:"search_first_name"`
	SearchLastName        string   `json:"search_last_name"`
	SearchFullName        string   `json:"search_full_name"`
	InjuryStatus          *string  `json:"injury_status"`
	InjuryStartDate       *string  `json:"injury_start_date"`
	PracticeParticipation *string  `json:"practice_participation"`
	FantasyDataID         *int     `json:"fantasy_data_id"`
	EspnID                *int     `json:"espn_id"`
	YahooID               *int     `json:"yahoo_id"`
	RotoworldID           *int     `json:"rotoworld_id"`
	RotowireID            *int     `json:"rotowire_id"`
	SportsradarID         *string  `json:"sportradar_id"`
	StatsID               *int     `json:"stats_id"`
}

// ToDBPlayer converts a Sleeper API Player to a database Player model
func (p *Player) ToDBPlayer() (*models.Player, error) {
	dbPlayer := &models.Player{
		SleeperID: p.PlayerID,
		FirstName: p.FirstName,
		LastName:  p.LastName,
		Position:  p.Position,
		Status:    p.Status,
		Sport:     p.Sport,
	}

	// Set full name (use FullName if available, otherwise combine First + Last)
	if p.FullName != "" {
		dbPlayer.Name = p.FullName
	} else {
		dbPlayer.Name = p.FirstName + " " + p.LastName
	}

	// Handle nullable fields with pointers
	if p.Team != nil {
		dbPlayer.Team = *p.Team
	}

	if p.Number != nil {
		dbPlayer.Number = *p.Number
	}

	if p.Hashtag != nil {
		dbPlayer.Hashtag = *p.Hashtag
	}

	if p.Age != nil {
		dbPlayer.Age = *p.Age
	}

	if p.Height != nil {
		dbPlayer.Height = *p.Height
	}

	if p.Weight != nil {
		dbPlayer.Weight = *p.Weight
	}

	if p.College != nil {
		dbPlayer.College = *p.College
	}

	if p.YearsExp != nil {
		dbPlayer.YearsExp = *p.YearsExp
	}

	if p.BirthCountry != nil {
		dbPlayer.BirthCountry = *p.BirthCountry
	}

	if p.DepthChartPosition != nil && p.DepthChartPosition.Valid {
		dbPlayer.DepthChartPosition = p.DepthChartPosition.Int
	}

	if p.DepthChartOrder != nil && p.DepthChartOrder.Valid {
		dbPlayer.DepthChartOrder = p.DepthChartOrder.Int
	}

	if p.SearchRank != nil {
		dbPlayer.SearchRank = *p.SearchRank
	}

	if p.InjuryStatus != nil {
		dbPlayer.InjuryStatus = *p.InjuryStatus
	}

	if p.InjuryStartDate != nil {
		dbPlayer.InjuryStartDate = *p.InjuryStartDate
	}

	if p.PracticeParticipation != nil {
		dbPlayer.PracticeParticipation = *p.PracticeParticipation
	}

	// Handle external IDs
	if p.EspnID != nil {
		dbPlayer.ESPNID = int64(*p.EspnID)
	}

	if p.YahooID != nil {
		dbPlayer.YahooID = strconv.Itoa(*p.YahooID)
	}

	if p.FantasyDataID != nil {
		dbPlayer.FantasyDataID = *p.FantasyDataID
	}

	if p.RotoworldID != nil {
		dbPlayer.RotoworldID = *p.RotoworldID
	}

	if p.RotowireID != nil {
		dbPlayer.RotowireID = strconv.Itoa(*p.RotowireID)
	}

	if p.SportsradarID != nil {
		dbPlayer.SportsradarID = *p.SportsradarID
	}

	if p.StatsID != nil {
		dbPlayer.StatsID = strconv.Itoa(*p.StatsID)
	}

	// Convert FantasyPositions slice to JSON string
	if len(p.FantasyPositions) > 0 {
		fantasyPosJSON, err := json.Marshal(p.FantasyPositions)
		if err != nil {
			return nil, err
		}
		dbPlayer.FantasyPositions = string(fantasyPosJSON)
	}

	return dbPlayer, nil
}

// GetAllPlayers fetches all NFL players from the Sleeper API
// This endpoint returns ~5MB of data and should be called sparingly (max once per day)
// Returns a map of player_id -> Player
func (c *Client) GetAllPlayers() (map[string]Player, error) {
	players := make(map[string]Player)
	endpoint := "players/nfl"
	if err := c.get(endpoint, &players); err != nil {
		return nil, err
	}
	return players, nil
}
