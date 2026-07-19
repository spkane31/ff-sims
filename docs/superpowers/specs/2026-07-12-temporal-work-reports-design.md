# Temporal work-done response types

## Problem

Every scheduled Temporal workflow in `backend/internal/workflows/` currently returns bare `error`, and several activities they call do the same despite doing real upsert/replicate work (`FetchAndUpsertAllPlayers`, `FetchWeekStats`, `ComputeSegmentSeasonADP`). When inspecting a run in the Temporal UI or CLI (`temporal workflow describe`), there's no way to see how much work happened — you have to go read worker logs. The goal is to give every scheduled workflow, and every activity that performs real work, a typed result indicating the amount of work done (rows upserted, leagues processed, etc.), so runs are inspectable at a glance for tuning batch sizes/concurrency.

## Convention

The codebase already has this pattern half-built: `SyncBatchResult`, `ReplicateBatchResult`, `PurgeBatchResult` (activities), and `ScavengerReport` (workflow). This work extends that same convention rather than introducing a new one:

- **Activities** return `<Name>Result` structs, defined in `backend/internal/activities/params.go`.
- **Workflows** return `<Name>Report` structs, defined in `backend/internal/workflows/params.go`.
- Every workflow gets its own `Report` type, even ones that wrap a single activity call 1:1 — no passthrough of an activity's `Result` type as a workflow's return value, for uniformity across the package.
- Query-only activities that return data to act on rather than a record of work done (`ListADPSeasons`, `ClaimLeaguesForDrafts`, `ClaimLeaguesForTransactions`, `ClaimStaleUsers`, `GetFinalizedWeeks`, `GetCurrentSeason`, all `Get*Config` methods) are unchanged.

## Changes

### Activities (`backend/internal/activities/`)

| File | Activity | Signature change |
|---|---|---|
| `player_sync.go` | `PlayerSyncActivities.FetchAndUpsertAllPlayers` | `(ctx) error` → `(ctx) (PlayerSyncResult, error)`. `PlayerSyncResult{PlayersUpserted int}`, filled from the existing `processed` counter. |
| `week_stats.go` | `WeekStatsActivities.FetchWeekStats` | `(ctx, params) error` → `(ctx, params) (WeekStatsResult, error)`. `WeekStatsResult{PlayersUpserted int, Finalized bool}`. Needs a new counter incremented once per upserted row in the existing loop; `Finalized` is already computed as the local `finalized` var. |
| `adp_rollup.go` | `ADPRollupActivities.ComputeSegmentSeasonADP` | `(ctx, params) error` → `(ctx, params) (ADPRollupResult, error)`. `ADPRollupResult{PlayersUpserted int}`, filled from `len(records)` (0 on the existing early return when `len(rows) == 0`). |

All three new `Result` types are added to `backend/internal/activities/params.go`, alongside the existing result types.

### Workflows (`backend/internal/workflows/`)

All new `Report` types are added to `backend/internal/workflows/params.go`.

| File | Workflow | Signature change | Report contents |
|---|---|---|---|
| `dispatcher.go` | `DiscoveryBatchDispatcher` | `(ctx) error` → `(ctx) (DiscoveryReport, error)` | `DiscoveryReport{UsersProcessed, UsersFailed int}` — sum of `SyncBatchResult.Processed`/`.Failed` across every `DiscoverUsersBatch` future collected in the loop. |
| `draft_sync.go` | `DraftSyncDispatcher` | `(ctx) error` → `(ctx) (DraftSyncReport, error)` | `DraftSyncReport{LeaguesProcessed, LeaguesFailed int}` — same aggregation pattern over `SyncLeagueDraftsBatch` futures. |
| `transaction_sync.go` | `TransactionSyncDispatcher` | `(ctx) error` → `(ctx) (TransactionSyncReport, error)` | `TransactionSyncReport{LeaguesProcessed, LeaguesFailed int}` — same pattern over `SyncLeagueTransactionsBatch` futures. |
| `player_sync.go` | `PlayerDatabaseSyncWorkflow` | `(ctx) error` → `(ctx) (PlayerSyncReport, error)` | `PlayerSyncReport{PlayersUpserted int}` — wraps `activities.PlayerSyncResult` returned by `FetchAndUpsertAllPlayers`. |
| `week_stats_sync.go` | `SyncWeekStats` | `(ctx, params) error` → `(ctx, params) (WeekStatsReport, error)` | `WeekStatsReport{WeeksFetched, PlayersUpserted int}` — sums `WeekStatsResult` across every non-skipped week in the 1..18 loop; `WeeksFetched` counts weeks actually fetched (not already-finalized weeks that were skipped). |
| `week_stats_sync.go` | `WeekStatsSyncDispatcher` | `(ctx) error` → `(ctx) (WeekStatsReport, error)` | Passes through the `WeekStatsReport` returned by `SyncWeekStats`. |
| `adp_rollup.go` | `ADPRollupDispatcher` | `(ctx) error` → `(ctx) (ADPRollupDispatchReport, error)` | `ADPRollupDispatchReport{SegmentsScheduled int}` — count of child workflow starts that succeeded (`GetChildWorkflowExecution().Get()` returned no error). Stays fire-and-forget (`ParentClosePolicy: ABANDON`); does **not** wait on children, so this counts *scheduled* work, not completed work. |
| `adp_rollup.go` | `SegmentSeasonADPRollupWorkflow` | `(ctx, params) error` → `(ctx, params) (SegmentADPReport, error)` | `SegmentADPReport{PlayersUpserted int}` — wraps `activities.ADPRollupResult`. Zero value on the already-swallowed activity failure path (logged, not propagated — unchanged behavior). |
| `scavenger.go` | `ScavengerDispatcher` | *(unchanged)* | Already returns `activities.ScavengerReport`. |
| `backfill.go` | `ArchiveBackfillWorkflow` | `(ctx) error` → `(ctx) (BackfillReport, error)` | `BackfillReport{LeaguesReplicated, TransactionsReplicated, DraftHeadersReplicated, DraftPicksReplicated int}` — the same per-execution counts already computed and logged (`leaguesReplicated`, `txnReplicated`, `headersReplicated`, `picksReplicated`); reports only the current execution's work, not a lifetime total across `ContinueAsNew` hops (matches existing per-run logging semantics). Returned both on the `ContinueAsNewError` path and on final completion. |

`ScavengerDispatcher` and its `drainStream` helper in `helpers.go` are unchanged — they're already the reference implementation this design follows.

### Callers

`backend/schedules/register.go` registers workflows by function reference (`workflows.DiscoveryBatchDispatcher` etc.) and never inspects return values — no changes needed there. Any other caller of `workflows.SyncWeekStats` (e.g. manual `temporal workflow start --type SyncWeekStats`) is unaffected beyond the return type change, which is source-compatible for anyone not currently capturing the (previously absent) result.

### Tests

- `backend/internal/activities/player_sync_test.go`, `week_stats_test.go`, `adp_rollup_test.go`: every direct call to the three changed activities currently does `if err := a.Method(...); err != nil`. These become `result, err := a.Method(...)`, with new assertions on the `Result` counts where the test seeds a known number of rows.
- `backend/internal/workflows/workflows_test.go`: every `env.OnActivity(...).Return(nil)` for the three changed activities becomes `.Return(activities.XResult{...}, nil)`. Every `env.ExecuteWorkflow(...)` for a changed workflow gains a `env.GetWorkflowResult(&report)` assertion checking the aggregated counts, following the existing pattern already used for `TestScavengerDispatcher_*`. `env.OnWorkflow(...).Return(nil)` for `SegmentSeasonADPRollupWorkflow` in the `ADPRollupDispatcher` tests becomes `.Return(workflows.SegmentADPReport{}, nil)`.

## Out of scope

- No change to `drainStream` (`helpers.go`) or `ScavengerDispatcher` — already correct.
- No change to `DiscoveryActivities.FetchUserLeagues` / `FetchLeagueMembers` / `FetchLeagueDetails` — these are Go methods on an activity struct but are never invoked via `workflow.ExecuteActivity` (only called in-process from `discoverOneUser` inside `DiscoverUsersBatch`), so they aren't part of the Temporal work-visibility surface this change targets.
- No change to query-only activities/configs listed under Convention above.
- No behavior change to error handling, retry policies, activity options, or claim/batch logic anywhere — this is purely additive return-value plumbing.
