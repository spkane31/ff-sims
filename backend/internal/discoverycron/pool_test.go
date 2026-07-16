package discoverycron_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"backend/internal/discoverycron"
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
	res := discoverycron.RunPool(ctx, discoverycron.PoolConfig{Size: 3, RefillBatch: 1, PollInterval: 5 * time.Millisecond},
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
	done := make(chan discoverycron.PoolResult, 1)
	go func() {
		done <- discoverycron.RunPool(ctx, discoverycron.PoolConfig{Size: 4, RefillBatch: 4, PollInterval: 5 * time.Millisecond},
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
	discoverycron.RunPool(ctx, discoverycron.PoolConfig{Size: 3, RefillBatch: 1, PollInterval: 20 * time.Millisecond},
		q.claimFn, process, func(string, error, time.Duration) {})

	// 100ms / 20ms poll interval should yield roughly 5 claim attempts, not
	// hundreds — proves the loop sleeps between empty claims instead of
	// spinning.
	if got := atomic.LoadInt32(&q.claim); got > 15 {
		t.Errorf("expected a bounded number of claim attempts on an empty queue, got %d", got)
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
	done := make(chan discoverycron.PoolResult, 1)
	go func() {
		done <- discoverycron.RunPool(ctx, discoverycron.PoolConfig{Size: 2, RefillBatch: 1, PollInterval: 5 * time.Millisecond},
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
