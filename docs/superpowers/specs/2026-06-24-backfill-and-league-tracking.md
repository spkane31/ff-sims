# Spec: Sleeper Data Backfill and Per-League Fetch Tracking

**Date:** 2026-06-24  
**Status:** Approved

## Context

The database contains 1,222 discovered `sleeper_leagues` and 2,933 `sleeper_users`, but the deployed web page shows 0 leagues, 0 trades, and 0 drafts. The root cause is that `GetSleeperStats` (`backend/internal/api/handlers/sleeper.go`) filters `WHERE last_fetched_at IS NOT NULL` — leagues discovered but not yet processed through `LeagueSyncWorkflow` have `NULL last_fetched_at`.

Additionally:
- User discovery only fetches leagues for seasons 2022–2025, leaving 2020–2021 unfetched.
- `LeagueSyncWorkflow` bundles draft and transaction fetching into a single workflow, preventing independent scaling. Drafts peak Aug–Sep; transactions peak Oct–Jan.

## Goals

1. Expand season range from 2022–2025 to **2020–2025**.
2. Split `LeagueSyncWorkflow` into **two independent workflows** — one for drafts, one for transactions — each with its own dispatcher and task queue so they can scale independently.
3. Move `FetchLeagueDetails` into `UserDiscoveryWorkflow` so league metadata is populated during discovery, not during sync.
4. Add `last_drafts_fetched_at` and `last_transactions_fetched_at` to `sleeper_leagues` as the primary queue cursor for each new dispatcher.
5. Trigger re-discovery for existing users to pick up 2020–2021 leagues.

## Architecture

### Before

```
DiscoveryBatchDispatcher (15 min, sleeper-discovery)
├─ UserDiscoveryWorkflow × 25  (sleeper-discovery)
└─ LeagueSyncWorkflow × 25     (sleeper-data)
   ├─ FetchLeagueDetails
   ├─ FetchLeagueDrafts + FetchDraftPicks
   ├─ FetchLeagueTransactions
   └─ MarkLeagueFetched
```

### After

```
DiscoveryBatchDispatcher (15 min, sleeper-discovery)        ← modified: no longer spawns LeagueSyncWorkflow
└─ UserDiscoveryWorkflow × 25  (sleeper-discovery)          ← modified: now calls FetchLeagueDetails
   ├─ FetchUserLeagues (2020–2025)                          ← expanded
   ├─ FetchLeagueMembers (per league)
   ├─ FetchLeagueDetails (per league)                       ← moved here from LeagueSyncWorkflow
   │   └─ stamps sleeper_leagues.last_fetched_at
   └─ MarkUserFetched

DraftSyncDispatcher (30 min, sleeper-drafts)                ← new
└─ LeagueDraftSyncWorkflow × 25  (sleeper-drafts)           ← new
   ├─ FetchLeagueDrafts
   ├─ FetchDraftPicks (per completed draft)
   └─ MarkLeagueDraftsFetched
       └─ stamps sleeper_leagues.last_drafts_fetched_at     ← new column

TransactionSyncDispatcher (30 min, sleeper-transactions)    ← new
└─ LeagueTransactionSyncWorkflow × 25  (sleeper-transactions) ← new
   ├─ FetchLeagueTransactions (rounds 1–18)
   └─ MarkLeagueTransactionsFetched
       └─ stamps sleeper_leagues.last_transactions_fetched_at ← new column

PlayerDatabaseSyncWorkflow (daily 03:00 UTC, sleeper-player-sync) ← unchanged
```

### Column Repurposing

`sleeper_leagues.last_fetched_at` is repurposed: it now means "league details last fetched" and is stamped when `FetchLeagueDetails` completes during `UserDiscoveryWorkflow`. This keeps `GetSleeperStats` (`WHERE last_fetched_at IS NOT NULL`) working correctly — leagues with populated metadata are counted.

### Queue Ordering

Each dispatcher uses its own tracking column, ordering `NULL FIRST` then oldest:

```sql
-- DraftSyncDispatcher
ORDER BY CASE WHEN last_drafts_fetched_at IS NULL THEN 0 ELSE 1 END, last_drafts_fetched_at ASC

-- TransactionSyncDispatcher
ORDER BY CASE WHEN last_transactions_fetched_at IS NULL THEN 0 ELSE 1 END, last_transactions_fetched_at ASC
```

## Changes

### 1. Expand Season Range

**File:** `v2/backend/internal/activities/discovery.go` line 17

```go
var Seasons = []string{"2020", "2021", "2022", "2023", "2024", "2025"}
```

### 2. DB Migration

**New file:** `v2/backend/migrations/006_league_fetch_tracking.sql`

```sql
-- +goose Up

ALTER TABLE sleeper_leagues
    ADD COLUMN IF NOT EXISTS last_drafts_fetched_at        TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_transactions_fetched_at  TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_sleeper_leagues_last_drafts_fetched
    ON sleeper_leagues (last_drafts_fetched_at ASC NULLS FIRST);

CREATE INDEX IF NOT EXISTS idx_sleeper_leagues_last_transactions_fetched
    ON sleeper_leagues (last_transactions_fetched_at ASC NULLS FIRST);

-- +goose Down

DROP INDEX IF EXISTS idx_sleeper_leagues_last_transactions_fetched;
DROP INDEX IF EXISTS idx_sleeper_leagues_last_drafts_fetched;

ALTER TABLE sleeper_leagues
    DROP COLUMN IF EXISTS last_transactions_fetched_at,
    DROP COLUMN IF EXISTS last_drafts_fetched_at;
```

### 3. Model Struct

**File:** `v2/backend/internal/models/sleeper.go` — add two fields to `SleeperLeague` after `LastFetchedAt`:

```go
LastFetchedAt                *time.Time      `gorm:"column:last_fetched_at"`
LastDraftsFetchedAt          *time.Time      `gorm:"column:last_drafts_fetched_at"`
LastTransactionsFetchedAt    *time.Time      `gorm:"column:last_transactions_fetched_at"`
SkippedAt                    *time.Time      `gorm:"column:skipped_at"`
```

### 4. Move `FetchLeagueDetails` to `DiscoveryActivities`

**File:** `v2/backend/internal/activities/data_fetch.go` — remove `FetchLeagueDetails`.

**File:** `v2/backend/internal/activities/discovery.go` — add `FetchLeagueDetails` (same implementation, same Sleeper API call). It stamps `last_fetched_at` on the league upon success:

```go
func (a *DiscoveryActivities) FetchLeagueDetails(ctx context.Context, params FetchLeagueDetailsParams) error {
    // ... same scoring settings logic ...
    updates["last_fetched_at"] = time.Now().UTC()
    return a.DB.WithContext(ctx).Model(&models.SleeperLeague{}).
        Where("sleeper_league_id = ?", params.LeagueID).
        Updates(updates).Error
}
```

**File:** `v2/backend/internal/activities/params.go` — `FetchLeagueDetailsParams` already exists, no change needed.

### 5. Update `UserDiscoveryWorkflow`

**File:** `v2/backend/internal/workflows/discovery.go` — add `FetchLeagueDetails` call in the league loop:

```go
for _, lid := range leagueIDs {
    if err := workflow.ExecuteActivity(actCtx, da.FetchLeagueMembers, ...).Get(ctx, nil); err != nil {
        workflow.GetLogger(ctx).Warn("FetchLeagueMembers failed, continuing", ...)
    }
    if err := workflow.ExecuteActivity(actCtx, da.FetchLeagueDetails, activities.FetchLeagueDetailsParams{LeagueID: lid}).Get(ctx, nil); err != nil {
        workflow.GetLogger(ctx).Warn("FetchLeagueDetails failed, continuing", ...)
    }
}
```

### 6. Update `DiscoveryBatchDispatcher`

**File:** `v2/backend/internal/workflows/dispatcher.go` — remove the `GetStaleLeagues` + `LeagueSyncWorkflow` block entirely. The dispatcher only spawns `UserDiscoveryWorkflow` children now. Remove the `dfa` and `dataActCtx` references.

### 7. New Task Queue Constants

**File:** `v2/backend/internal/workflows/helpers.go` — add two constants, remove `TaskQueueData`:

```go
const (
    TaskQueueDiscovery     = "sleeper-discovery"
    TaskQueueDrafts        = "sleeper-drafts"        // new
    TaskQueueTransactions  = "sleeper-transactions"  // new
    TaskQueuePlayerSync    = "sleeper-player-sync"
    BatchSize              = 25
)
```

### 8. New Activities

**File:** `v2/backend/internal/activities/data_fetch.go`

Add `GetStaleLeaguesForDrafts`, `GetStaleLeaguesForTransactions`, `MarkLeagueDraftsFetched`, `MarkLeagueTransactionsFetched`:

```go
func (a *DataFetchActivities) GetStaleLeaguesForDrafts(ctx context.Context, params GetStaleLeaguesParams) ([]string, error) {
    // WHERE skipped_at IS NULL ORDER BY CASE WHEN last_drafts_fetched_at IS NULL THEN 0 ELSE 1 END, last_drafts_fetched_at ASC
}

func (a *DataFetchActivities) GetStaleLeaguesForTransactions(ctx context.Context, params GetStaleLeaguesParams) ([]string, error) {
    // WHERE skipped_at IS NULL ORDER BY CASE WHEN last_transactions_fetched_at IS NULL THEN 0 ELSE 1 END, last_transactions_fetched_at ASC
}

func (a *DataFetchActivities) MarkLeagueDraftsFetched(ctx context.Context, params MarkLeagueFetchedParams) error {
    // UPDATE sleeper_leagues SET last_drafts_fetched_at = now() WHERE sleeper_league_id = ?
}

func (a *DataFetchActivities) MarkLeagueTransactionsFetched(ctx context.Context, params MarkLeagueFetchedParams) error {
    // UPDATE sleeper_leagues SET last_transactions_fetched_at = now() WHERE sleeper_league_id = ?
}
```

Remove `GetStaleLeagues` (replaced by the two above), and remove the `last_transactions_fetched_at` stamp from inside `FetchLeagueTransactions` (moved to `MarkLeagueTransactionsFetched`).

**File:** `v2/backend/internal/activities/params.go` — add `GetStaleLeaguesForDraftsParams` and `GetStaleLeaguesForTransactionsParams` (both identical to `GetStaleLeaguesParams{BatchSize int}`; can reuse the same type).

### 9. New Workflows

**New file:** `v2/backend/internal/workflows/draft_sync.go`

```go
func DraftSyncDispatcher(ctx workflow.Context) error {
    // GetStaleLeaguesForDrafts → spawn LeagueDraftSyncWorkflow × BatchSize (ABANDON)
}

func LeagueDraftSyncWorkflow(ctx workflow.Context, params LeagueSyncParams) error {
    // FetchLeagueDrafts → completedDraftIDs
    // for each: FetchDraftPicks (errors: warn+continue)
    // MarkLeagueDraftsFetched
}
```

**New file:** `v2/backend/internal/workflows/transaction_sync.go`

```go
func TransactionSyncDispatcher(ctx workflow.Context) error {
    // GetStaleLeaguesForTransactions → spawn LeagueTransactionSyncWorkflow × BatchSize (ABANDON)
}

func LeagueTransactionSyncWorkflow(ctx workflow.Context, params LeagueSyncParams) error {
    // FetchLeagueTransactions
    // MarkLeagueTransactionsFetched
    // 404 on league → MarkLeagueSkipped (same pattern as existing)
}
```

### 10. Remove `LeagueSyncWorkflow`

**File:** `v2/backend/internal/workflows/league_sync.go` — delete this file entirely once the two new workflows are in place.

### 11. Update Worker Registration

**File:** `v2/backend/cmd/worker/main.go` — replace the `dataw` worker with two new workers:

```go
// Remove:
dataw := worker.New(c, workflows.TaskQueueData, worker.Options{})
dataw.RegisterWorkflow(workflows.LeagueSyncWorkflow)
dataw.RegisterActivity(dfa)

// Add:
draftsw := worker.New(c, workflows.TaskQueueDrafts, worker.Options{})
draftsw.RegisterWorkflow(workflows.DraftSyncDispatcher)
draftsw.RegisterWorkflow(workflows.LeagueDraftSyncWorkflow)
draftsw.RegisterActivity(dfa)

transactionsw := worker.New(c, workflows.TaskQueueTransactions, worker.Options{})
transactionsw.RegisterWorkflow(workflows.TransactionSyncDispatcher)
transactionsw.RegisterWorkflow(workflows.LeagueTransactionSyncWorkflow)
transactionsw.RegisterActivity(dfa)
```

Discovery worker also needs `FetchLeagueDetails` registered (now on `DiscoveryActivities`):
```go
dw.RegisterActivity(da) // da already registered; FetchLeagueDetails is now on da
```

### 12. Update Schedules

**File:** `v2/backend/schedules/register.go` — add two new schedules, update discovery schedule to no longer include league dispatch:

```go
// Add:
upsert(ctx, c, client.ScheduleOptions{
    ID: "sleeper-draft-sync-schedule",
    Spec: client.ScheduleSpec{Intervals: []client.ScheduleIntervalSpec{{Every: 30 * time.Minute}}},
    Action: &client.ScheduleWorkflowAction{
        Workflow: workflows.DraftSyncDispatcher,
        TaskQueue: workflows.TaskQueueDrafts,
    },
})

upsert(ctx, c, client.ScheduleOptions{
    ID: "sleeper-transaction-sync-schedule",
    Spec: client.ScheduleSpec{Intervals: []client.ScheduleIntervalSpec{{Every: 30 * time.Minute}}},
    Action: &client.ScheduleWorkflowAction{
        Workflow: workflows.TransactionSyncDispatcher,
        TaskQueue: workflows.TaskQueueTransactions,
    },
})
```

## One-Time Backfill Trigger

After deploying the updated workers, reset all users to force re-discovery with the expanded season range:

```sql
UPDATE sleeper_users
SET last_fetched_at = NULL
WHERE skipped_at IS NULL;
```

Existing 2022–2025 leagues will re-upsert as `DoNothing`. New 2020–2021 leagues will be inserted with NULL tracking columns, placing them at the top of the draft and transaction queues.

## Verification Queries

```sql
-- Confirm migration applied
SELECT column_name FROM information_schema.columns
WHERE table_name = 'sleeper_leagues'
  AND column_name IN ('last_drafts_fetched_at', 'last_transactions_fetched_at');

-- Check 2020/2021 leagues arriving after user reset
SELECT season, COUNT(*) FROM sleeper_leagues
WHERE season IN ('2020', '2021') GROUP BY season;

-- Track each queue independently
SELECT
    COUNT(*) FILTER (WHERE last_fetched_at IS NULL)              AS details_pending,
    COUNT(*) FILTER (WHERE last_drafts_fetched_at IS NULL)       AS drafts_pending,
    COUNT(*) FILTER (WHERE last_transactions_fetched_at IS NULL) AS txns_pending,
    COUNT(*)                                                     AS total
FROM sleeper_leagues WHERE skipped_at IS NULL;

-- Confirm stats API returns non-zero (uses last_fetched_at = league details)
SELECT COUNT(*) FROM sleeper_leagues WHERE last_fetched_at IS NOT NULL;
```

## Files Modified

| File | Change |
|---|---|
| `v2/backend/internal/activities/discovery.go` | Expand `Seasons`; add `FetchLeagueDetails` (stamps `last_fetched_at`) |
| `v2/backend/internal/activities/data_fetch.go` | Remove `FetchLeagueDetails`, `GetStaleLeagues`; add `GetStaleLeaguesForDrafts`, `GetStaleLeaguesForTransactions`, `MarkLeagueDraftsFetched`, `MarkLeagueTransactionsFetched` |
| `v2/backend/internal/activities/params.go` | Add params for new activities |
| `v2/backend/internal/models/sleeper.go` | Add `LastDraftsFetchedAt`, `LastTransactionsFetchedAt` to `SleeperLeague` |
| `v2/backend/migrations/006_league_fetch_tracking.sql` | New: two columns + indexes |
| `v2/backend/internal/workflows/helpers.go` | Replace `TaskQueueData` with `TaskQueueDrafts` + `TaskQueueTransactions` |
| `v2/backend/internal/workflows/discovery.go` | Add `FetchLeagueDetails` call in league loop |
| `v2/backend/internal/workflows/dispatcher.go` | Remove league dispatch block |
| `v2/backend/internal/workflows/draft_sync.go` | New: `DraftSyncDispatcher` + `LeagueDraftSyncWorkflow` |
| `v2/backend/internal/workflows/transaction_sync.go` | New: `TransactionSyncDispatcher` + `LeagueTransactionSyncWorkflow` |
| `v2/backend/internal/workflows/league_sync.go` | Delete |
| `v2/backend/cmd/worker/main.go` | Replace `dataw` with `draftsw` + `transactionsw` |
| `v2/backend/schedules/register.go` | Add two new schedules |
| `v2/backend/internal/workflows/workflows_test.go` | Update/remove `LeagueSyncWorkflow` tests; add tests for new workflows |
| `v2/backend/internal/activities/data_fetch_test.go` | Update for removed/added activities |
