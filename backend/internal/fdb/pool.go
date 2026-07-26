// Package fdb ("fetch, dispatch, batch") generalizes internal/cronpool's
// claim/dispatch worker loop by splitting per-item work into two stages: a
// concurrent fetch stage that talks to an external source (e.g. Sleeper) and
// produces an in-memory result, and a batch stage that accumulates those
// results and flushes them via one bulk DB write instead of one write per
// item. fdb owns the single *gorm.DB instance used by a RunPool call and
// hands it to every stage that needs it (claim, fetch, and — as an open
// transaction — flush), so there is exactly one database connection path for
// a given RunPool call rather than each stage independently holding its own
// reference.
//
// A flush that never happens (ctx ends before BatchSize/interval trigger, or
// the flush write itself fails) is treated as acceptable, not a bug to work
// around: callers of RunPool are expected to build on a claim mechanism with
// a TTL, so an item whose batch never flushed simply keeps its claim and is
// reclaimed and retried on a later run.
package fdb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// defaultPollInterval mirrors cronpool's: how often RunPool re-checks when
// below its refill threshold or the last claim came back empty.
const defaultPollInterval = 2 * time.Second

// defaultBatchFlushInterval is used when Config.BatchFlushInterval is unset.
const defaultBatchFlushInterval = 5 * time.Second

// shutdownFlushTimeout bounds the final best-effort flush RunPool performs
// after its caller's ctx is already done. That ctx is, by construction,
// expired by the time this flush runs — using it directly would make
// db.WithContext(ctx) fail immediately, defeating the point of a shutdown
// flush — so this last call gets its own short-lived, independent context
// instead.
const shutdownFlushTimeout = 30 * time.Second

// Config sizes and paces one RunPool call.
type Config struct {
	// Size is the maximum number of items fetched concurrently.
	Size int
	// RefillBatch is how many fetch slots must be free before RunPool claims
	// more work. See cronpool.PoolConfig.RefillBatch for the same rationale.
	RefillBatch int
	// PollInterval is how long RunPool waits before re-checking when it's
	// below RefillBatch free slots or the last claim was empty. Defaults to
	// defaultPollInterval if zero.
	PollInterval time.Duration
	// BatchSize flushes the pending batch once it reaches this many fetched
	// results.
	BatchSize int
	// BatchFlushInterval flushes the pending batch at least this often, even
	// if BatchSize hasn't been reached, so results don't sit indefinitely.
	// Defaults to defaultBatchFlushInterval if zero.
	BatchFlushInterval time.Duration
}

// Result summarizes one RunPool call. Processed + Failed + FlushDropped
// always equals the number of items whose fetch completed by the time
// RunPool returns — guaranteed by RunPool's shutdown flush.
type Result struct {
	// Processed counts items whose fetch succeeded AND whose batch flushed
	// successfully.
	Processed int
	// Failed counts items whose fetch itself returned a non-nil error — they
	// never entered a batch.
	Failed int
	// FlushDropped counts items whose fetch succeeded but whose batch's
	// flush call failed. Their final DB state is unknown (the flush was
	// rolled back); they are dropped, not retried in-process — the caller's
	// claim mechanism is what makes them eligible again later.
	FlushDropped int
	// FlushErrors counts how many flush() calls returned a non-nil error —
	// batch-level, not item-level (usually much smaller than FlushDropped).
	FlushErrors int
	// ClaimErrors counts how many times claim(ctx, db, n) returned a non-nil
	// error, as opposed to a legitimately empty claim.
	ClaimErrors int
}

// ErrBatchFlushFailed wraps the error passed to onResult for every item in a
// batch whose flush() call failed, so callers can distinguish "this item's
// own fetch failed" from "this item fetched fine but its batch's write
// failed" via errors.Is(err, fdb.ErrBatchFlushFailed).
var ErrBatchFlushFailed = errors.New("fdb: batch flush failed")

type pendingItem[C any, R any] struct {
	claim      C
	value      R
	err        error
	fetchStart time.Time
}

// RunPool claims items of type C, fetches/transforms each concurrently into
// a result of type R (no DB writes during fetch), and periodically flushes
// accumulated R values via one bulk write executed in a transaction RunPool
// itself opens on db.
//
// claim(ctx, db, n) should return up to n item IDs (fewer, or none, if the
// queue is short right now) using db directly — no transaction, since a
// claim query is expected to be atomic on its own (e.g. an UPDATE ...
// RETURNING ... FOR UPDATE SKIP LOCKED). fetch(ctx, db, item) handles one
// item concurrently; a non-nil error is recorded as Failed immediately and
// the item never enters a batch. db is passed to fetch for the rare
// read-only check a fetch needs before deciding whether to do its external
// work at all — fetch itself must never write. flush(ctx, tx, batch) is
// called with the transaction RunPool opened for this batch; a non-nil
// return rolls that transaction back and every item in the batch is counted
// as FlushDropped. onResult is called once per item that leaves RunPool's
// bookkeeping, whether via Failed, a successful flush, or a failed flush.
//
// No per-item timeout is imposed here, matching cronpool.RunPool — fetch is
// expected to respect ctx itself.
func RunPool[C any, R any](
	ctx context.Context,
	db *gorm.DB,
	cfg Config,
	claim func(ctx context.Context, db *gorm.DB, n int) ([]C, error),
	fetch func(ctx context.Context, db *gorm.DB, item C) (R, error),
	flush func(ctx context.Context, tx *gorm.DB, batch []R) error,
	onResult func(item C, err error, duration time.Duration),
) Result {
	size := max(1, cfg.Size)
	refillBatch := max(1, cfg.RefillBatch)
	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}
	batchSize := max(1, cfg.BatchSize)
	batchFlushInterval := cfg.BatchFlushInterval
	if batchFlushInterval <= 0 {
		batchFlushInterval = defaultBatchFlushInterval
	}

	var res Result
	fetched := make(chan pendingItem[C, R], size)
	inFlight := 0
	var buf []pendingItem[C, R]

	ticker := time.NewTicker(batchFlushInterval)
	defer ticker.Stop()

	flushNow := func(flushCtx context.Context) {
		if len(buf) == 0 {
			return
		}
		batch := buf
		buf = nil
		values := make([]R, len(batch))
		for i, p := range batch {
			values[i] = p.value
		}

		err := db.WithContext(flushCtx).Transaction(func(tx *gorm.DB) error {
			return flush(flushCtx, tx, values)
		})
		if err != nil {
			res.FlushErrors++
			wrapped := fmt.Errorf("%w: %v", ErrBatchFlushFailed, err)
			for _, p := range batch {
				res.FlushDropped++
				onResult(p.claim, wrapped, time.Since(p.fetchStart))
			}
			return
		}
		for _, p := range batch {
			res.Processed++
			onResult(p.claim, nil, time.Since(p.fetchStart))
		}
	}

	recordFetch := func(p pendingItem[C, R]) {
		inFlight--
		if p.err != nil {
			res.Failed++
			onResult(p.claim, p.err, time.Since(p.fetchStart))
			return
		}
		buf = append(buf, p)
		if len(buf) >= batchSize {
			flushNow(ctx)
		}
	}

	drainNonBlocking := func() {
		for {
			select {
			case p := <-fetched:
				recordFetch(p)
			default:
				return
			}
		}
	}

	checkTicker := func() {
		select {
		case <-ticker.C:
			flushNow(ctx)
		default:
		}
	}

	for ctx.Err() == nil {
		drainNonBlocking()
		checkTicker()

		free := size - inFlight
		if free < refillBatch {
			select {
			case p := <-fetched:
				recordFetch(p)
			case <-ticker.C:
				flushNow(ctx)
			case <-time.After(pollInterval):
			case <-ctx.Done():
			}
			continue
		}

		items, err := claim(ctx, db, free)
		if err != nil {
			res.ClaimErrors++
			select {
			case <-ticker.C:
				flushNow(ctx)
			case <-time.After(pollInterval):
			case <-ctx.Done():
			}
			continue
		}
		if len(items) == 0 {
			select {
			case <-ticker.C:
				flushNow(ctx)
			case <-time.After(pollInterval):
			case <-ctx.Done():
			}
			continue
		}

		for _, item := range items {
			inFlight++
			go func(item C) {
				start := time.Now()
				v, ferr := fetch(ctx, db, item)
				fetched <- pendingItem[C, R]{claim: item, value: v, err: ferr, fetchStart: start}
			}(item)
		}
	}

	for inFlight > 0 {
		recordFetch(<-fetched)
	}

	// ctx is guaranteed already done here — give this last, best-effort
	// flush its own fresh deadline instead of one that would fail instantly.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownFlushTimeout)
	defer shutdownCancel()
	flushNow(shutdownCtx)
	return res
}
