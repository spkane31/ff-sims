package sleeper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestGet_RateLimiterSpacesRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewWithBaseURL(srv.URL)
	// 120 RPM = one token every 500ms; burst 1 so the second call must wait.
	c.limiter = rate.NewLimiter(rate.Limit(120.0/60.0), 1)

	start := time.Now()
	var out map[string]any
	for i := 0; i < 2; i++ {
		if err := c.get(context.Background(), "/v1/state/nfl", &out); err != nil {
			t.Fatalf("get %d: %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed < 400*time.Millisecond {
		t.Errorf("expected second request to be rate-limited (>=400ms), took %v", elapsed)
	}
}

func TestNewLimiter_DefaultsAndEnvOverride(t *testing.T) {
	t.Setenv("SLEEPER_RPM", "")
	if l := newLimiter(); l.Limit() != rate.Limit(2000.0/60.0) {
		t.Errorf("default limiter = %v, want %v", l.Limit(), rate.Limit(2000.0/60.0))
	}
	t.Setenv("SLEEPER_RPM", "600")
	if l := newLimiter(); l.Limit() != rate.Limit(600.0/60.0) {
		t.Errorf("env limiter = %v, want %v", l.Limit(), rate.Limit(600.0/60.0))
	}
}
