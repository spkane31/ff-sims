package sleeper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestGet_ConcurrencyLimitSerializesRequestsBeyondCapacity verifies the
// semaphore actually bounds simultaneous in-flight requests: with capacity 1
// against a server that holds each request open briefly, two concurrent
// callers must be serialized rather than both hitting the network at once.
func TestGet_ConcurrencyLimitSerializesRequestsBeyondCapacity(t *testing.T) {
	const holdOpen = 150 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(holdOpen)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewWithBaseURL(srv.URL)
	c.sem = make(chan struct{}, 1)

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out map[string]any
			if err := c.get(context.Background(), "/v1/state/nfl", &out); err != nil {
				t.Errorf("get: %v", err)
			}
		}()
	}
	wg.Wait()

	// Serialized (capacity 1): ~2*holdOpen. Concurrent (uncapped): ~holdOpen.
	if elapsed := time.Since(start); elapsed < 2*holdOpen {
		t.Errorf("expected the second request to wait for a concurrency slot (>= %v), took %v", 2*holdOpen, elapsed)
	}
}

func TestMaxConcurrentRequests_DefaultsAndEnvOverride(t *testing.T) {
	t.Setenv("SLEEPER_MAX_CONCURRENT_REQUESTS", "")
	if n := maxConcurrentRequests(); n != defaultMaxConcurrentRequests {
		t.Errorf("default = %d, want %d", n, defaultMaxConcurrentRequests)
	}
	t.Setenv("SLEEPER_MAX_CONCURRENT_REQUESTS", "10")
	if n := maxConcurrentRequests(); n != 10 {
		t.Errorf("env override = %d, want 10", n)
	}
}
