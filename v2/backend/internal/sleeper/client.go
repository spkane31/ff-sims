package sleeper

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"
)

const defaultBaseURL = "https://api.sleeper.app"

type Client struct {
	http    *http.Client
	baseURL string
}

func New() *Client {
	return NewWithBaseURL(defaultBaseURL)
}

func NewWithBaseURL(baseURL string) *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		baseURL: baseURL,
	}
}

func (c *Client) get(ctx context.Context, path string, out interface{}) error {
	url := c.baseURL + path
	backoff := 500 * time.Millisecond
	for attempt := 0; attempt < 5; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusNotFound:
			return &NotFoundError{URL: url}
		case http.StatusTooManyRequests:
			wait := time.Duration(math.Pow(2, float64(attempt))) * backoff
			if wait > 60*time.Second {
				wait = 60 * time.Second
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			continue
		case http.StatusOK:
			return json.NewDecoder(resp.Body).Decode(out)
		default:
			return fmt.Errorf("sleeper: unexpected status %d for %s", resp.StatusCode, url)
		}
	}
	return fmt.Errorf("sleeper: exhausted retries for %s", url)
}

func (c *Client) GetUser(ctx context.Context, usernameOrID string) (*User, error) {
	var u User
	if err := c.get(ctx, "/v1/user/"+usernameOrID, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (c *Client) GetUserLeagues(ctx context.Context, userID, sport, season string) ([]League, error) {
	var leagues []League
	path := fmt.Sprintf("/v1/user/%s/leagues/%s/%s", userID, sport, season)
	if err := c.get(ctx, path, &leagues); err != nil {
		return nil, err
	}
	return leagues, nil
}

func (c *Client) GetLeague(ctx context.Context, leagueID string) (*League, error) {
	var l League
	if err := c.get(ctx, "/v1/league/"+leagueID, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

func (c *Client) GetLeagueUsers(ctx context.Context, leagueID string) ([]LeagueUser, error) {
	var users []LeagueUser
	if err := c.get(ctx, "/v1/league/"+leagueID+"/users", &users); err != nil {
		return nil, err
	}
	return users, nil
}

func (c *Client) GetLeagueDrafts(ctx context.Context, leagueID string) ([]Draft, error) {
	var drafts []Draft
	if err := c.get(ctx, "/v1/league/"+leagueID+"/drafts", &drafts); err != nil {
		return nil, err
	}
	return drafts, nil
}

func (c *Client) GetDraftPicks(ctx context.Context, draftID string) ([]DraftPick, error) {
	var picks []DraftPick
	if err := c.get(ctx, "/v1/draft/"+draftID+"/picks", &picks); err != nil {
		return nil, err
	}
	return picks, nil
}

func (c *Client) GetTransactions(ctx context.Context, leagueID string, round int) ([]Transaction, error) {
	var txns []Transaction
	path := fmt.Sprintf("/v1/league/%s/transactions/%d", leagueID, round)
	if err := c.get(ctx, path, &txns); err != nil {
		return nil, err
	}
	return txns, nil
}

func (c *Client) GetAllPlayers(ctx context.Context, sport string) (map[string]Player, error) {
	var players map[string]Player
	if err := c.get(ctx, "/v1/players/"+sport, &players); err != nil {
		return nil, err
	}
	return players, nil
}
