package discoverycron

import (
	"context"
	"time"
)

// defaultPollInterval is used when PoolConfig.PollInterval is unset. It
// bounds how often RunPool re-queries the claim function when the pool is
// below its refill threshold or the last claim came back empty — long
// enough not to hammer the database on an empty queue, short enough that a
// freshly-claimable item doesn't sit idle for long once the pool is behind.
const defaultPollInterval = 2 * time.Second

// PoolConfig sizes one RunPool call.
type PoolConfig struct {
	// Size is the maximum number of items processed concurrently.
	Size int
	// RefillBatch is how many pool slots must be free before RunPool claims
	// more work. Claiming in batches (rather than one-for-one as each slot
	// frees) keeps the number of claim queries bounded as Size scales up.
	RefillBatch int
	// PollInterval is how long RunPool waits before re-checking when it's
	// below RefillBatch free slots or the last claim was empty. Defaults to
	// defaultPollInterval if zero.
	PollInterval time.Duration
}

// PoolResult summarizes one RunPool call.
type PoolResult struct {
	Processed int
	Failed    int
	// ClaimErrors counts how many times claim(ctx, free) returned a non-nil
	// error (e.g. the DB is unreachable) — distinct from an empty-but-error-
	// free claim, which means "genuinely nothing to do right now" and isn't
	// counted here.
	ClaimErrors int
}

type itemResult struct {
	id       string
	err      error
	duration time.Duration
}

// RunPool claims and processes work items until ctx is done, then waits for
// any still-in-flight items to finish before returning. claim(ctx, n) should
// return up to n item IDs (fewer, or none, if the queue is short right now).
// process(ctx, id) handles one item; a non-nil return is recorded as a
// failure but does not stop the pool or retry the item here — the caller's
// claim mechanism (a DB claim with a TTL, in production use) is what makes a
// failed item eligible again later. onResult is called once per completed
// item (success or failure) for logging.
//
// No per-item timeout is imposed here — see
// docs/superpowers/specs/2026-07-15-discovery-cron-migration-design.md's
// Concurrency model section for why. process is expected to respect ctx
// itself (the Sleeper client's calls already do).
func RunPool(
	ctx context.Context,
	cfg PoolConfig,
	claim func(ctx context.Context, n int) ([]string, error),
	process func(ctx context.Context, id string) error,
	onResult func(id string, err error, duration time.Duration),
) PoolResult {
	size := max(1, cfg.Size)
	refillBatch := max(1, cfg.RefillBatch)
	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	var res PoolResult
	results := make(chan itemResult, size)
	inFlight := 0

	record := func(r itemResult) {
		inFlight--
		if r.err != nil {
			res.Failed++
		} else {
			res.Processed++
		}
		onResult(r.id, r.err, r.duration)
	}

	drainNonBlocking := func() {
		for {
			select {
			case r := <-results:
				record(r)
			default:
				return
			}
		}
	}

	for ctx.Err() == nil {
		drainNonBlocking()
		free := size - inFlight
		if free < refillBatch {
			select {
			case r := <-results:
				record(r)
			case <-time.After(pollInterval):
			case <-ctx.Done():
			}
			continue
		}

		ids, err := claim(ctx, free)
		if err != nil {
			res.ClaimErrors++
			select {
			case <-time.After(pollInterval):
			case <-ctx.Done():
			}
			continue
		}
		if len(ids) == 0 {
			select {
			case <-time.After(pollInterval):
			case <-ctx.Done():
			}
			continue
		}

		for _, id := range ids {
			inFlight++
			go func(id string) {
				start := time.Now()
				err := process(ctx, id)
				results <- itemResult{id: id, err: err, duration: time.Since(start)}
			}(id)
		}
	}

	for inFlight > 0 {
		record(<-results)
	}
	return res
}
