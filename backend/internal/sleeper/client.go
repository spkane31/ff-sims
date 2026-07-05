package sleeper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

const (
	maxAttempts = 6
	backoffBase = 500 * time.Millisecond
	backoffCap  = 30 * time.Second

	// defaultRPM is the SLEEPER_RPM fallback: requests/minute budget per
	// process. Each fleet (DigitalOcean, Raspberry Pi) has its own IP, so
	// per-process is per-IP. Start high, tune down.
	defaultRPM = 2000
)

const defaultBaseURL = "https://api.sleeper.app"

type Client struct {
	http    *http.Client
	baseURL string
	limiter *rate.Limiter
}

func New() *Client {
	return NewWithBaseURL(defaultBaseURL)
}

func NewWithBaseURL(baseURL string) *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		baseURL: baseURL,
		limiter: newLimiter(),
	}
}

// newLimiter builds the client-wide request limiter from SLEEPER_RPM
// (requests/minute, default 2000). Burst of one second's worth of tokens keeps
// short spikes smooth without letting the minute budget be spent all at once.
func newLimiter() *rate.Limiter {
	rpm := defaultRPM
	if v := os.Getenv("SLEEPER_RPM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			rpm = n
		}
	}
	perSecond := float64(rpm) / 60.0
	burst := int(perSecond)
	if burst < 1 {
		burst = 1
	}
	return rate.NewLimiter(rate.Limit(perSecond), burst)
}

func (c *Client) get(ctx context.Context, path string, out interface{}) error {
	url := c.baseURL + path

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Every attempt (including retries) consumes a rate-limit token so the
		// SLEEPER_RPM budget bounds actual requests on the wire.
		if err := c.limiter.Wait(ctx); err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = err
			if !c.waitBeforeRetry(ctx, fullJitterBackoff(attempt), attempt) {
				return ctx.Err()
			}
			continue
		}

		switch {
		case resp.StatusCode == http.StatusNotFound:
			closeBody(resp)
			return &NotFoundError{URL: url}
		case resp.StatusCode == http.StatusTooManyRequests:
			wait := retryAfterOrBackoff(resp, attempt)
			lastErr = fmt.Errorf("sleeper: rate limited (429) for %s", url)
			drainAndClose(resp)
			if !c.waitBeforeRetry(ctx, wait, attempt) {
				return ctx.Err()
			}
			continue
		case resp.StatusCode >= 500 && resp.StatusCode <= 599:
			lastErr = fmt.Errorf("sleeper: unexpected status %d for %s", resp.StatusCode, url)
			drainAndClose(resp)
			if !c.waitBeforeRetry(ctx, fullJitterBackoff(attempt), attempt) {
				return ctx.Err()
			}
			continue
		case resp.StatusCode == http.StatusOK:
			defer resp.Body.Close()
			return json.NewDecoder(resp.Body).Decode(out)
		default:
			closeBody(resp)
			return fmt.Errorf("sleeper: unexpected status %d for %s", resp.StatusCode, url)
		}
	}
	return fmt.Errorf("sleeper: exhausted retries for %s: %w", url, lastErr)
}

// waitBeforeRetry sleeps for d unless this is the last available attempt, in
// which case it returns immediately so the caller can report lastErr. It
// returns false if the context is done while waiting.
func (c *Client) waitBeforeRetry(ctx context.Context, d time.Duration, attempt int) bool {
	if attempt >= maxAttempts-1 {
		return true
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// fullJitterBackoff returns a random duration in [0, backoff) where backoff
// grows exponentially with attempt, capped at backoffCap.
func fullJitterBackoff(attempt int) time.Duration {
	// Cap the shift so the multiplication can't overflow before the min below
	// clamps it to backoffCap (attempt is always < maxAttempts in practice).
	grown := backoffBase * time.Duration(uint64(1)<<uint(min(attempt, 32)))
	backoff := min(grown, backoffCap)
	return time.Duration(rand.Float64() * float64(backoff))
}

// retryAfterOrBackoff honors a 429 response's Retry-After header (seconds or
// HTTP-date) when present and parsable, falling back to computed backoff.
func retryAfterOrBackoff(resp *http.Response, attempt int) time.Duration {
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return time.Duration(secs) * time.Second
		}
		if t, err := http.ParseTime(v); err == nil {
			if d := time.Until(t); d > 0 {
				return d
			}
			return 0
		}
	}
	return fullJitterBackoff(attempt)
}

// drainAndClose reads any remaining response body so the underlying TCP
// connection can be reused, then closes it.
func drainAndClose(resp *http.Response) {
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func closeBody(resp *http.Response) {
	resp.Body.Close()
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

// GetWeekStats fetches per-player weekly stats for season/week. The map key is the
// sleeper_player_id; each value is the raw stat object (includes pts_ppr, pts_half_ppr,
// pts_std among many other fields, decoded further by callers).
func (c *Client) GetWeekStats(ctx context.Context, season string, week int) (map[string]json.RawMessage, error) {
	var stats map[string]json.RawMessage
	path := fmt.Sprintf("/v1/stats/nfl/regular/%s/%d", season, week)
	if err := c.get(ctx, path, &stats); err != nil {
		return nil, err
	}
	return stats, nil
}

// GetNFLState fetches the current NFL season/week/season_type.
func (c *Client) GetNFLState(ctx context.Context) (*NFLState, error) {
	var s NFLState
	if err := c.get(ctx, "/v1/state/nfl", &s); err != nil {
		return nil, err
	}
	return &s, nil
}
