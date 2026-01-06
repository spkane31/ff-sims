package sleeper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

var ErrNotImplemented = fmt.Errorf("not implemented")

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
