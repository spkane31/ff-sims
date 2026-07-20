package cronpool_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"backend/internal/cronpool"
)

// fakeQueue is a simple in-memory claimable queue for testing RunPool
// without a database.
type fakeQueue struct {
	mu    sync.Mutex
	ids   []string
	claim int32 // number of claim() calls, for busy-loop assertions
}

func newFakeQueue(n int) *fakeQueue {
	q := &fakeQueue{}
	for i := 0; i < n; i++ {
		q.ids = append(q.ids, fmt.Sprintf("item%d", i))
	}
	return q
}

func (q *fakeQueue) claimFn(ctx context.Context, n int) ([]string, error) {
	atomic.AddInt32(&q.claim, 1)
	q.mu.Lock()
	defer q.mu.Unlock()
	if n > len(q.ids) {
		n = len(q.ids)
	}
	got := q.ids[:n]
	q.ids = q.ids[n:]
	return got, nil
}

func TestRunPool_ProcessesAllItemsAndReportsCounts(t *testing.T) {
	q := newFakeQueue(10)
	var processed sync.Map
	process := func(ctx context.Context, id string) error {
		processed.Store(id, true)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res := cronpool.RunPool(ctx, cronpool.PoolConfig{Size: 3, RefillBatch: 1, PollInterval: 5 * time.Millisecond},
		q.claimFn, process, func(string, error, time.Duration) {})

	if res.Processed != 10 || res.Failed != 0 {
		t.Fatalf("expected 10 processed / 0 failed, got %+v", res)
	}
	count := 0
	processed.Range(func(k, v any) bool { count++; return true })
	if count != 10 {
		t.Errorf("expected 10 distinct items processed, got %d", count)
	}
}

func TestRunPool_RefillOnlyTriggersAtThreshold(t *testing.T) {
	q := newFakeQueue(6)
	block := make(chan struct{})
	var startedCount int32
	process := func(ctx context.Context, id string) error {
		atomic.AddInt32(&startedCount, 1)
		<-block // hold every item open until the test releases them
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan cronpool.PoolResult, 1)
	go func() {
		done <- cronpool.RunPool(ctx, cronpool.PoolConfig{Size: 4, RefillBatch: 4, PollInterval: 5 * time.Millisecond},
			q.claimFn, process, func(string, error, time.Duration) {})
	}()

	// RefillBatch=4 with pool size 4: the very first claim should ask for up
	// to 4 (all slots free), then no further claim should happen until 4
	// slots free up again — never a partial refill of e.g. 1 or 2.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && atomic.LoadInt32(&startedCount) < 4 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&startedCount); got != 4 {
		t.Fatalf("expected exactly 4 items claimed before any slot freed, got %d", got)
	}

	close(block)
	cancel()
	<-done
}

func TestRunPool_EmptyClaimDoesNotBusyLoop(t *testing.T) {
	q := newFakeQueue(0)
	process := func(ctx context.Context, id string) error { return nil }

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	res := cronpool.RunPool(ctx, cronpool.PoolConfig{Size: 3, RefillBatch: 1, PollInterval: 20 * time.Millisecond},
		q.claimFn, process, func(string, error, time.Duration) {})

	// 100ms / 20ms poll interval should yield roughly 5 claim attempts, not
	// hundreds — proves the loop sleeps between empty claims instead of
	// spinning.
	if got := atomic.LoadInt32(&q.claim); got > 15 {
		t.Errorf("expected a bounded number of claim attempts on an empty queue, got %d", got)
	}
	// An empty-but-no-error claim is a legitimate outcome (nothing to do
	// right now), distinct from a claim error — it must not be counted as
	// one.
	if res.ClaimErrors != 0 {
		t.Errorf("expected ClaimErrors == 0 for an empty queue with no error, got %d", res.ClaimErrors)
	}
}

func TestRunPool_ClaimErrorIncrementsClaimErrorsAndDoesNotBusyLoop(t *testing.T) {
	var claimCount int32
	claimErr := errors.New("db unreachable")
	claim := func(ctx context.Context, n int) ([]string, error) {
		atomic.AddInt32(&claimCount, 1)
		return nil, claimErr
	}
	process := func(ctx context.Context, id string) error { return nil }

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	res := cronpool.RunPool(ctx, cronpool.PoolConfig{Size: 3, RefillBatch: 1, PollInterval: 20 * time.Millisecond},
		claim, process, func(string, error, time.Duration) {})

	if res.ClaimErrors == 0 {
		t.Error("expected ClaimErrors to be incremented when claim() returns an error")
	}
	if got := atomic.LoadInt32(&claimCount); int(got) != res.ClaimErrors {
		t.Errorf("expected ClaimErrors (%d) to equal the number of claim attempts (%d)", res.ClaimErrors, got)
	}
	// Same bound as the empty-queue busy-loop test: a claim error must still
	// sleep pollInterval between attempts, not spin.
	if got := atomic.LoadInt32(&claimCount); got > 15 {
		t.Errorf("expected a bounded number of claim attempts on a persistent claim error, got %d", got)
	}
	if res.Processed != 0 || res.Failed != 0 {
		t.Errorf("expected no items processed when every claim errors, got %+v", res)
	}
}

func TestRunPool_DrainsInFlightWorkOnDeadline(t *testing.T) {
	q := newFakeQueue(1)
	started := make(chan struct{})
	finished := make(chan struct{})
	process := func(ctx context.Context, id string) error {
		close(started)
		<-ctx.Done() // simulate work that respects the shared ctx
		close(finished)
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan cronpool.PoolResult, 1)
	go func() {
		done <- cronpool.RunPool(ctx, cronpool.PoolConfig{Size: 2, RefillBatch: 1, PollInterval: 5 * time.Millisecond},
			q.claimFn, process, func(string, error, time.Duration) {})
	}()

	<-started
	cancel()

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("expected in-flight work to be allowed to finish after ctx cancellation")
	}
	res := <-done
	if res.Failed != 1 {
		t.Fatalf("expected the cancelled item to count as failed, got %+v", res)
	}
}
