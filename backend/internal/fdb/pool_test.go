package fdb_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/fdb"
)

// newTestDB opens an in-memory SQLite *gorm.DB — RunPool needs a real db to
// open each flush's transaction on, but these tests' flush closures never
// touch a real table, so no schema/AutoMigrate is needed.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

// fakeQueue is a simple in-memory claimable queue for testing RunPool
// without a real claim query.
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

func (q *fakeQueue) claimFn(ctx context.Context, db *gorm.DB, n int) ([]string, error) {
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

// noopFetch returns id unchanged as its own result, for tests that only
// care about claim/flush behavior.
func noopFetch(ctx context.Context, db *gorm.DB, id string) (string, error) { return id, nil }

// noopFlush never errors and doesn't inspect the batch.
func noopFlush(ctx context.Context, tx *gorm.DB, batch []string) error { return nil }

func TestRunPool_ProcessesAllItemsAndReportsCounts(t *testing.T) {
	db := newTestDB(t)
	q := newFakeQueue(10)
	var fetchedIDs sync.Map
	fetch := func(ctx context.Context, db *gorm.DB, id string) (string, error) {
		fetchedIDs.Store(id, true)
		return id, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Huge BatchSize/BatchFlushInterval so neither trigger fires mid-run —
	// this proves the guaranteed shutdown flush accounts for everything.
	res := fdb.RunPool(ctx, db, fdb.Config{Size: 3, RefillBatch: 1, PollInterval: 5 * time.Millisecond, BatchSize: 1_000_000, BatchFlushInterval: time.Hour},
		q.claimFn, fetch, noopFlush, func(string, error, time.Duration) {})

	if res.Processed != 10 || res.Failed != 0 {
		t.Fatalf("expected 10 processed / 0 failed, got %+v", res)
	}
	count := 0
	fetchedIDs.Range(func(k, v any) bool { count++; return true })
	if count != 10 {
		t.Errorf("expected 10 distinct items fetched, got %d", count)
	}
}

func TestRunPool_RefillOnlyTriggersAtThreshold(t *testing.T) {
	db := newTestDB(t)
	q := newFakeQueue(6)
	block := make(chan struct{})
	var startedCount int32
	fetch := func(ctx context.Context, db *gorm.DB, id string) (string, error) {
		atomic.AddInt32(&startedCount, 1)
		<-block // hold every item open until the test releases them
		return id, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan fdb.Result, 1)
	go func() {
		done <- fdb.RunPool(ctx, db, fdb.Config{Size: 4, RefillBatch: 4, PollInterval: 5 * time.Millisecond, BatchSize: 1_000_000, BatchFlushInterval: time.Hour},
			q.claimFn, fetch, noopFlush, func(string, error, time.Duration) {})
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
	db := newTestDB(t)
	q := newFakeQueue(0)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	res := fdb.RunPool(ctx, db, fdb.Config{Size: 3, RefillBatch: 1, PollInterval: 20 * time.Millisecond, BatchSize: 1_000_000, BatchFlushInterval: time.Hour},
		q.claimFn, noopFetch, noopFlush, func(string, error, time.Duration) {})

	// 100ms / 20ms poll interval should yield roughly 5 claim attempts, not
	// hundreds — proves the loop sleeps between empty claims instead of
	// spinning.
	if got := atomic.LoadInt32(&q.claim); got > 15 {
		t.Errorf("expected a bounded number of claim attempts on an empty queue, got %d", got)
	}
	if res.ClaimErrors != 0 {
		t.Errorf("expected ClaimErrors == 0 for an empty queue with no error, got %d", res.ClaimErrors)
	}
}

func TestRunPool_ClaimErrorIncrementsClaimErrorsAndDoesNotBusyLoop(t *testing.T) {
	db := newTestDB(t)
	var claimCount int32
	claimErr := errors.New("db unreachable")
	claim := func(ctx context.Context, db *gorm.DB, n int) ([]string, error) {
		atomic.AddInt32(&claimCount, 1)
		return nil, claimErr
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	res := fdb.RunPool(ctx, db, fdb.Config{Size: 3, RefillBatch: 1, PollInterval: 20 * time.Millisecond, BatchSize: 1_000_000, BatchFlushInterval: time.Hour},
		claim, noopFetch, noopFlush, func(string, error, time.Duration) {})

	if res.ClaimErrors == 0 {
		t.Error("expected ClaimErrors to be incremented when claim() returns an error")
	}
	if got := atomic.LoadInt32(&claimCount); int(got) != res.ClaimErrors {
		t.Errorf("expected ClaimErrors (%d) to equal the number of claim attempts (%d)", res.ClaimErrors, got)
	}
	if got := atomic.LoadInt32(&claimCount); got > 15 {
		t.Errorf("expected a bounded number of claim attempts on a persistent claim error, got %d", got)
	}
	if res.Processed != 0 || res.Failed != 0 {
		t.Errorf("expected no items processed when every claim errors, got %+v", res)
	}
}

func TestRunPool_DeadlineCanceledClaimIsNotAClaimError(t *testing.T) {
	db := newTestDB(t)
	claim := func(ctx context.Context, db *gorm.DB, n int) ([]string, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	res := fdb.RunPool(ctx, db, fdb.Config{Size: 1, RefillBatch: 1, PollInterval: 5 * time.Millisecond, BatchSize: 1, BatchFlushInterval: time.Hour},
		claim, noopFetch, noopFlush, func(string, error, time.Duration) {})

	if res.ClaimErrors != 0 {
		t.Fatalf("expected the pool's own deadline cancellation not to count as a claim error, got %+v", res)
	}
}

func TestRunPool_ShutdownGraceDrainsBeforeDeadline(t *testing.T) {
	db := newTestDB(t)
	var nextID int32
	claim := func(ctx context.Context, db *gorm.DB, n int) ([]string, error) {
		items := make([]string, n)
		for i := range items {
			items[i] = fmt.Sprintf("item%d", atomic.AddInt32(&nextID, 1))
		}
		return items, nil
	}
	fetch := func(ctx context.Context, db *gorm.DB, id string) (string, error) {
		select {
		case <-time.After(35 * time.Millisecond):
			return id, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	res := fdb.RunPool(ctx, db, fdb.Config{
		Size: 2, RefillBatch: 2, PollInterval: 5 * time.Millisecond,
		BatchSize: 1_000_000, BatchFlushInterval: time.Hour,
		ShutdownGracePeriod: 150 * time.Millisecond,
	}, claim, fetch, noopFlush, func(string, error, time.Duration) {})
	elapsed := time.Since(started)

	if res.Processed == 0 || res.Failed != 0 || res.FlushDropped != 0 {
		t.Fatalf("expected admitted work to drain and flush cleanly, got %+v", res)
	}
	if ctx.Err() != nil {
		t.Fatalf("expected the pool to return before the hard deadline, got %v after %s", ctx.Err(), elapsed)
	}
	if elapsed < 150*time.Millisecond {
		t.Fatalf("expected the pool to accept work until the grace window, returned after %s", elapsed)
	}
}

func TestRunPool_DrainsInFlightWorkOnDeadline(t *testing.T) {
	db := newTestDB(t)
	q := newFakeQueue(1)
	started := make(chan struct{})
	finished := make(chan struct{})
	fetch := func(ctx context.Context, db *gorm.DB, id string) (string, error) {
		close(started)
		<-ctx.Done() // simulate work that respects the shared ctx
		close(finished)
		return "", ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan fdb.Result, 1)
	go func() {
		done <- fdb.RunPool(ctx, db, fdb.Config{Size: 2, RefillBatch: 1, PollInterval: 5 * time.Millisecond, BatchSize: 1_000_000, BatchFlushInterval: time.Hour},
			q.claimFn, fetch, noopFlush, func(string, error, time.Duration) {})
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

func TestRunPool_FlushTriggersOnBatchSize(t *testing.T) {
	db := newTestDB(t)
	q := newFakeQueue(3)
	var mu sync.Mutex
	var flushSizes []int
	flush := func(ctx context.Context, tx *gorm.DB, batch []string) error {
		mu.Lock()
		flushSizes = append(flushSizes, len(batch))
		mu.Unlock()
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	res := fdb.RunPool(ctx, db, fdb.Config{Size: 3, RefillBatch: 1, PollInterval: 5 * time.Millisecond, BatchSize: 3, BatchFlushInterval: time.Hour},
		q.claimFn, noopFetch, flush, func(string, error, time.Duration) {})

	mu.Lock()
	defer mu.Unlock()
	if len(flushSizes) == 0 || flushSizes[0] != 3 {
		t.Fatalf("expected a flush call carrying exactly 3 items (BatchSize) before ctx ended, got %+v", flushSizes)
	}
	if res.Processed != 3 {
		t.Errorf("expected 3 processed, got %+v", res)
	}
}

func TestRunPool_FlushTriggersOnInterval(t *testing.T) {
	db := newTestDB(t)
	q := newFakeQueue(2)
	flushed := make(chan int, 5)
	flush := func(ctx context.Context, tx *gorm.DB, batch []string) error {
		flushed <- len(batch)
		return nil
	}

	// BatchSize is unreachable; only the interval trigger can flush these 2
	// items before ctx's deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	done := make(chan fdb.Result, 1)
	go func() {
		done <- fdb.RunPool(ctx, db, fdb.Config{Size: 2, RefillBatch: 1, PollInterval: 5 * time.Millisecond, BatchSize: 1_000_000, BatchFlushInterval: 50 * time.Millisecond},
			q.claimFn, noopFetch, flush, func(string, error, time.Duration) {})
	}()

	select {
	case n := <-flushed:
		if n != 2 {
			t.Fatalf("expected the interval flush to carry both pending items, got %d", n)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected a time-triggered flush well before ctx's 500ms deadline")
	}
	cancel()
	<-done
}

func TestRunPool_ShutdownFlushesPendingItems(t *testing.T) {
	db := newTestDB(t)
	q := newFakeQueue(2)

	// BatchSize and BatchFlushInterval are both unreachable within ctx's
	// short lifetime — only the guaranteed shutdown flush can account for
	// these items.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	res := fdb.RunPool(ctx, db, fdb.Config{Size: 2, RefillBatch: 1, PollInterval: 5 * time.Millisecond, BatchSize: 1_000_000, BatchFlushInterval: time.Hour},
		q.claimFn, noopFetch, noopFlush, func(string, error, time.Duration) {})

	if res.Processed != 2 {
		t.Fatalf("expected both pending items to be flushed at shutdown despite never hitting BatchSize/interval, got %+v", res)
	}
}

func TestRunPool_FailingFlushCountsDroppedNotProcessed(t *testing.T) {
	db := newTestDB(t)
	q := newFakeQueue(3)
	flushErr := errors.New("db unreachable")
	flush := func(ctx context.Context, tx *gorm.DB, batch []string) error { return flushErr }

	var mu sync.Mutex
	var gotErrs []error
	onResult := func(id string, err error, d time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		gotErrs = append(gotErrs, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	res := fdb.RunPool(ctx, db, fdb.Config{Size: 3, RefillBatch: 1, PollInterval: 5 * time.Millisecond, BatchSize: 3, BatchFlushInterval: time.Hour},
		q.claimFn, noopFetch, flush, onResult)

	if res.FlushErrors != 1 {
		t.Errorf("expected 1 flush error, got %d", res.FlushErrors)
	}
	if res.FlushDropped != 3 {
		t.Errorf("expected 3 dropped items, got %d", res.FlushDropped)
	}
	if res.Processed != 0 {
		t.Errorf("expected 0 processed when the only flush fails, got %d", res.Processed)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(gotErrs) != 3 {
		t.Fatalf("expected onResult called for all 3 items, got %d", len(gotErrs))
	}
	for _, err := range gotErrs {
		if !errors.Is(err, fdb.ErrBatchFlushFailed) {
			t.Errorf("expected errors.Is(err, fdb.ErrBatchFlushFailed), got %v", err)
		}
	}
}

func TestRunPool_FetchFailureNeverEntersABatch(t *testing.T) {
	db := newTestDB(t)
	q := newFakeQueue(1)
	fetchErr := errors.New("sleeper 500")
	fetch := func(ctx context.Context, db *gorm.DB, id string) (string, error) { return "", fetchErr }
	var flushCalled int32
	flush := func(ctx context.Context, tx *gorm.DB, batch []string) error {
		atomic.AddInt32(&flushCalled, 1)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	res := fdb.RunPool(ctx, db, fdb.Config{Size: 1, RefillBatch: 1, PollInterval: 5 * time.Millisecond, BatchSize: 1, BatchFlushInterval: time.Hour},
		q.claimFn, fetch, flush, func(string, error, time.Duration) {})

	if res.Failed == 0 {
		t.Error("expected the failed fetch to be counted immediately")
	}
	if atomic.LoadInt32(&flushCalled) != 0 {
		t.Error("expected flush to never be called when every fetch fails")
	}
}
