package sleeper_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

// TestGetUser_RateLimitLogsEachOccurrence is the regression test for the
// "we should have logging for 429s" ask: with no proactive rate/concurrency
// limiting left in the client, the 429/Retry-After backoff loop is the only
// defense against Sleeper-side throttling, so every occurrence must be
// visible in the process log — not just the terminal "exhausted retries"
// error, which only fires if a request never recovers.
func TestGetUser_RateLimitLogsEachOccurrence(t *testing.T) {
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

	var logBuf bytes.Buffer
	prevOutput := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevOutput)
		log.SetFlags(prevFlags)
	}()

	c := sleeper.NewWithBaseURL(srv.URL)
	if _, err := c.GetUser(context.Background(), "someone"); err != nil {
		t.Fatalf("GetUser error after retries: %v", err)
	}

	logged := logBuf.String()
	occurrences := strings.Count(logged, "429")
	if occurrences != 2 {
		t.Errorf("expected one log line per 429 response (2), got %d occurrences in log output: %q", occurrences, logged)
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

func TestGet_TransportErrorThenSuccess(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("server doesn't support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			conn.Close()
			return
		}
		json.NewEncoder(w).Encode(sleeper.User{UserID: "789"})
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	u, err := c.GetUser(context.Background(), "flaky")
	if err != nil {
		t.Fatalf("GetUser error: %v", err)
	}
	if u.UserID != "789" {
		t.Errorf("got UserID %q, want %q", u.UserID, "789")
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Errorf("expected 2 attempts, got %d", got)
	}
}

func TestGet_RetryAfterHeader_WaitsAtLeastSpecifiedDuration(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(sleeper.User{UserID: "111"})
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	start := time.Now()
	u, err := c.GetUser(context.Background(), "ratelimited")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("GetUser error: %v", err)
	}
	if u.UserID != "111" {
		t.Errorf("got UserID %q, want %q", u.UserID, "111")
	}
	if elapsed < 1*time.Second {
		t.Errorf("expected wait of at least 1s honoring Retry-After, got %v", elapsed)
	}
}

func TestGet_500ThenSuccess(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(sleeper.User{UserID: "222"})
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	u, err := c.GetUser(context.Background(), "flaky500")
	if err != nil {
		t.Fatalf("GetUser error: %v", err)
	}
	if u.UserID != "222" {
		t.Errorf("got UserID %q, want %q", u.UserID, "222")
	}
}

func TestGet_500ExhaustsRetries(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	_, err := c.GetUser(context.Background(), "always500")
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if got := atomic.LoadInt32(&attempts); got != 6 {
		t.Errorf("expected 6 attempts, got %d", got)
	}
}

func TestGet_NotFound_SingleRequest(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	_, err := c.GetUser(context.Background(), "ghost2")
	var nfe *sleeper.NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("expected exactly 1 request, got %d", got)
	}
}

func TestGet_BadRequest_SingleRequestNoRetry(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	_, err := c.GetUser(context.Background(), "bad")
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("expected exactly 1 request, got %d", got)
	}
}

func TestGet_CanceledContext_ReturnsPromptlyWithoutRetry(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := sleeper.NewWithBaseURL(srv.URL)
	start := time.Now()
	_, err := c.GetUser(ctx, "canceled")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("expected prompt return, took %v", elapsed)
	}
}

// TestGet_ReusesConnectionAcrossRetries verifies response bodies on failed
// attempts are drained and closed (not just closed), since Go's transport
// only returns a connection to the keep-alive pool when the body is fully
// read before Close. An unread body forces a new TCP connection per retry.
func TestGet_ReusesConnectionAcrossRetries(t *testing.T) {
	var attempts int32
	var newConns int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("server error body"))
			return
		}
		json.NewEncoder(w).Encode(sleeper.User{UserID: "333"})
	}))
	srv.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		if state == http.StateNew {
			atomic.AddInt32(&newConns, 1)
		}
	}
	srv.Start()
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	_, err := c.GetUser(context.Background(), "trackclose")
	if err != nil {
		t.Fatalf("GetUser error: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
	if got := atomic.LoadInt32(&newConns); got != 1 {
		t.Errorf("expected connection reuse across retries (1 new conn), got %d new conns", got)
	}
}
