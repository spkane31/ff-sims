package sleeper_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/sleeper"
)

func TestGetUser_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/testuser" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(sleeper.User{
			UserID: "123", Username: "testuser", DisplayName: "Test User",
		})
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	u, err := c.GetUser(context.Background(), "testuser")
	if err != nil {
		t.Fatalf("GetUser error: %v", err)
	}
	if u.UserID != "123" {
		t.Errorf("got UserID %q, want %q", u.UserID, "123")
	}
}

func TestGetUser_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	_, err := c.GetUser(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	var nfe *sleeper.NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}

func TestGetUser_RateLimitRetries(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(sleeper.User{UserID: "456"})
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	u, err := c.GetUser(context.Background(), "someone")
	if err != nil {
		t.Fatalf("GetUser error after retries: %v", err)
	}
	if u.UserID != "456" {
		t.Errorf("got UserID %q, want %q", u.UserID, "456")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestGetWeekStats_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/stats/nfl/regular/2025/3" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"421":{"pts_ppr":24.06,"pts_half_ppr":20.56,"pts_std":17.06,"rec":5},"999":{"pts_ppr":0}}`))
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	stats, err := c.GetWeekStats(context.Background(), "2025", 3)
	if err != nil {
		t.Fatalf("GetWeekStats error: %v", err)
	}
	if len(stats) != 2 {
		t.Errorf("got %d players, want 2", len(stats))
	}
	var p421 struct {
		PtsPPR float64 `json:"pts_ppr"`
	}
	if err := json.Unmarshal(stats["421"], &p421); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p421.PtsPPR != 24.06 {
		t.Errorf("got PtsPPR %v, want 24.06", p421.PtsPPR)
	}
}

func TestGetWeekStats_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	_, err := c.GetWeekStats(context.Background(), "2025", 25)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	var nfe *sleeper.NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}

func TestGetNFLState_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/state/nfl" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(sleeper.NFLState{Season: "2025", SeasonType: "regular", Week: 5})
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	state, err := c.GetNFLState(context.Background())
	if err != nil {
		t.Fatalf("GetNFLState error: %v", err)
	}
	if state.Season != "2025" || state.Week != 5 {
		t.Errorf("got %+v, want season=2025 week=5", state)
	}
}
