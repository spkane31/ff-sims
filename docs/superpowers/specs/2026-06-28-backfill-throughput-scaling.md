# Spec: Backfill Throughput — Observability Queries and Scaling Plan

**Date:** 2026-06-28  
**Status:** Reference

## Context

With 118,735 Sleeper leagues in the database, ~90% have never had transactions or drafts fetched.
Understanding the exact queue depth and the math behind current throughput is a prerequisite to
deciding how aggressively to scale the Temporal dispatchers.

## Observability Queries

### 1. Headline summary

```sql
SELECT
    COUNT(*)                                                          AS total_leagues,
    COUNT(*) FILTER (WHERE last_fetched_at IS NULL)                  AS never_fetched,
    COUNT(*) FILTER (WHERE last_fetched_at IS NOT NULL)              AS fetched,
    COUNT(*) FILTER (WHERE last_transactions_fetched_at IS NULL)     AS no_transactions_fetched,
    COUNT(*) FILTER (WHERE last_drafts_fetched_at IS NULL)           AS no_drafts_fetched,
    COUNT(*) FILTER (WHERE skipped_at IS NOT NULL)                   AS skipped
FROM sleeper_leagues;
```

**What it shows:** Four independent queue depths in one pass.

- `never_fetched` — leagues discovered from a user's history but not yet processed by
  `UserDiscoveryWorkflow`. These have no metadata at all; drafts and transactions cannot be
  fetched until this column is non-null. The `GetStaleLeaguesForDrafts` and
  `GetStaleLeaguesForTransactions` queries both guard on `last_fetched_at IS NOT NULL`, so
  these leagues are invisible to the sync dispatchers.
- `no_transactions_fetched` / `no_drafts_fetched` — leagues whose metadata exists but whose
  child-record fetch has never been attempted. These are the primary backlog for the
  `TransactionSyncDispatcher` and `DraftSyncDispatcher`.
- `skipped` — leagues permanently excluded (e.g., HTTP 404 from Sleeper). Not in any queue.

**Results as of 2026-06-28:**

| total_leagues | never_fetched | fetched | no_transactions_fetched | no_drafts_fetched | skipped |
|---------------|---------------|---------|-------------------------|-------------------|---------|
| 118,735       | 1             | 118,734 | 107,352                 | 107,318           | 0       |

Only 1 league is stuck at the discovery stage. The 107k backlog is entirely in the sync stage.

---

### 2. State breakdown (most useful for prioritising workflows)

```sql
SELECT
    CASE
        WHEN last_fetched_at IS NULL               THEN 'never_fetched'
        WHEN last_transactions_fetched_at IS NULL  THEN 'fetched_no_transactions'
        WHEN last_drafts_fetched_at IS NULL        THEN 'fetched_no_drafts'
        ELSE                                            'fully_fetched'
    END                          AS state,
    COUNT(*)                     AS league_count
FROM sleeper_leagues
WHERE skipped_at IS NULL
GROUP BY 1
ORDER BY 2 DESC;
```

**What it shows:** Mutually exclusive states in priority order. A league is counted in the
first bucket it falls into, so "fetched_no_transactions" and "fetched_no_drafts" are not
double-counted. Useful for a quick dashboard card.

---

### 3. Fetched but returned no records (detect silent empty results)

```sql
SELECT
    COUNT(*) FILTER (
        WHERE last_transactions_fetched_at IS NOT NULL
          AND NOT EXISTS (
              SELECT 1 FROM sleeper_transactions t
              WHERE t.sleeper_league_id = l.sleeper_league_id
          )
    ) AS fetched_but_no_transactions,
    COUNT(*) FILTER (
        WHERE last_drafts_fetched_at IS NOT NULL
          AND NOT EXISTS (
              SELECT 1 FROM sleeper_drafts d
              WHERE d.sleeper_league_id = l.sleeper_league_id
          )
    ) AS fetched_but_no_drafts
FROM sleeper_leagues l;
```

**What it shows:** Leagues where the fetch completed and set the timestamp but Sleeper returned
zero records. This is normal for leagues that never made trades or had very small rosters, but
a large number here could indicate a bug in the mark-fetched logic (e.g., stamping even on
network errors). Run this periodically as a sanity check.

---

### 4. Full per-league breakdown (debugging / ad hoc)

```sql
SELECT
    l.sleeper_league_id,
    l.last_fetched_at,
    l.last_transactions_fetched_at,
    l.last_drafts_fetched_at,
    COUNT(DISTINCT t.sleeper_transaction_id) AS transaction_count,
    COUNT(DISTINCT d.sleeper_draft_id)       AS draft_count
FROM sleeper_leagues l
LEFT JOIN sleeper_transactions t ON t.sleeper_league_id = l.sleeper_league_id
LEFT JOIN sleeper_drafts       d ON d.sleeper_league_id = l.sleeper_league_id
GROUP BY l.sleeper_league_id, l.last_fetched_at, l.last_transactions_fetched_at, l.last_drafts_fetched_at;
```

**What it shows:** Raw counts per league with all three timestamps. Best used in a `WHERE`
clause targeting a specific league or date range, not run over the full 118k set without a
filter.

---

## Throughput Analysis

### Current configuration

| Parameter | Value | Location |
|-----------|-------|----------|
| `BatchSize` | 25 leagues/dispatch | `internal/workflows/helpers.go` |
| Dispatch interval (drafts) | 10 minutes | `schedules/register.go` |
| Dispatch interval (transactions) | 10 minutes | `schedules/register.go` |
| Worker concurrency | default (100 workflow, 100 activity) | `cmd/worker/main.go` |

**Dispatchers are fire-and-forget.** Each `DraftSyncDispatcher` and `TransactionSyncDispatcher`
picks a batch, spawns child workflows with `PARENT_CLOSE_POLICY_ABANDON`, waits only for the
child *start* acknowledgement, then exits. Children run in parallel on the task queue. The
dispatcher itself completes in seconds; the per-league work happens concurrently.

### Throughput math

```
leagues_per_hour = BatchSize × (60 min / interval_min)
                 = 25 × 6
                 = 150 leagues/hour
```

**API call cost per league:**
- Drafts: 1 call (`GetLeagueDrafts`) + N calls for completed drafts' picks
- Transactions: up to 18 calls (legs 1–18, one `GetTransactions` each)

At 25 leagues per dispatch with transactions, worst case is 25 × 18 = 450 Sleeper API calls
per 10-minute window, or ~45 calls/minute.

### Time to clear backlog at current rate

| Queue | Backlog | Hours to clear | Days to clear |
|-------|---------|----------------|---------------|
| Transactions | 107,352 | 716 | **30 days** |
| Drafts | 107,318 | 715 | **30 days** |

---

## Scaling Options

The two independent levers are `BatchSize` (leagues per dispatch) and the schedule interval.
Sleeper API rate limits are undocumented; the primary risk of scaling too fast is HTTP 429
responses triggering retries that slow effective throughput.

| Option | BatchSize | Interval | leagues/hour | Transactions backlog | Sleeper calls/min (txn worst case) |
|--------|-----------|----------|--------------|----------------------|------------------------------------|
| Current | 25 | 10 min | 150 | ~30 days | 45 |
| Conservative | 100 | 5 min | 1,200 | ~3.7 days | 360 |
| **Moderate (recommended)** | **200** | **2 min** | **6,000** | **~18 hours** | **1,800** |
| Aggressive | 500 | 1 min | 30,000 | ~3.6 hours | 9,000 |

**Recommendation: start with the moderate option.** 1,800 calls/minute from a single client is
likely within Sleeper's tolerance since the API is designed for public access. If 429s appear
in activity logs, double the interval or halve the BatchSize and re-measure.

### Code changes (implemented)

**`internal/workflows/helpers.go`** — keep `BatchSize=25` for discovery, add `SyncBatchSize=200`
for draft and transaction dispatchers:

```go
BatchSize     = 25   // discovery — unchanged
SyncBatchSize = 200  // draft and transaction sync dispatchers
```

Dispatch intervals remain at 10 minutes for all three schedulers (no change to Temporal
action count, which matters on free/limited accounts).

**`cmd/worker/main.go`** — raise activity concurrency on the two sync workers so the task
queue doesn't become the bottleneck when 200 children arrive at once:

```go
draftsw := worker.New(c, workflows.TaskQueueDrafts, worker.Options{
    MaxConcurrentActivityExecutionSize: 100,
    MaxConcurrentWorkflowTaskPollers:   10,
})

transactionsw := worker.New(c, workflows.TaskQueueTransactions, worker.Options{
    MaxConcurrentActivityExecutionSize: 100,
    MaxConcurrentWorkflowTaskPollers:   10,
})
```

### Revised throughput at 10-minute interval, SyncBatchSize=200

```
leagues_per_hour = 200 × 6 = 1,200/hour
```

| Queue | Backlog | Hours to clear | Days to clear |
|-------|---------|----------------|---------------|
| Transactions | 107,352 | ~90 | **~3.7 days** |
| Drafts | 107,318 | ~90 | **~3.7 days** |

### Monitoring during ramp-up

Run query 1 (headline summary) every hour. Once `no_transactions_fetched` stops decreasing,
check Temporal workflow failure rates for `LeagueTransactionSyncWorkflow` — 429 errors from
Sleeper will surface as activity failures with the `temporalio.activity.ApplicationError` type.

After the initial backlog clears, tune `SyncBatchSize` back down to whatever sustains the
steady-state incoming rate (new leagues from discovery + periodic re-syncs for active leagues).
