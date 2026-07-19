# Temporal Work-Done Response Types Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every scheduled Temporal workflow, and every activity that performs real upsert/replicate work, a typed return value reporting how much work was done, so a run is inspectable for productivity/tuning without reading worker logs.

**Architecture:** Extend the codebase's existing `Result`/`Report` convention (`SyncBatchResult`, `ReplicateBatchResult`, `PurgeBatchResult`, `ScavengerReport`). Activities gain `<Name>Result` structs in `backend/internal/activities/params.go`; workflows gain `<Name>Report` structs in `backend/internal/workflows/params.go`. Every workflow — including ones that wrap a single activity 1:1 — gets its own `Report` type for uniformity.

**Tech Stack:** Go, Temporal Go SDK (`go.temporal.io/sdk`), `testsuite.WorkflowTestSuite` for workflow tests, `testify/mock` and `testify/require`.

## Global Constraints

- All work happens on branch `worktree-temporal-work-reports` inside this worktree. Never touch `main`.
- Before every commit: `cd backend && gofmt -l . && go vet ./... && go build ./... && go test ./...` must show no gofmt diffs, no vet issues, and all tests passing.
- Follow the naming convention from the spec (`docs/superpowers/specs/2026-07-12-temporal-work-reports-design.md`) exactly: activities return `<Name>Result`, workflows return `<Name>Report`.
- Do not change retry policies, activity options, claim/batch SQL, or any error-handling/control-flow semantics beyond what's needed to thread the new return values through. This is additive plumbing only.
- Query-only activities/configs (`ListADPSeasons`, `ClaimLeaguesForDrafts`, `ClaimLeaguesForTransactions`, `ClaimStaleUsers`, `GetFinalizedWeeks`, `GetCurrentSeason`, all `Get*Config`) are unchanged.

---

## Task 1: Add all new Result/Report struct definitions

**Files:**
- Modify: `backend/internal/activities/params.go`
- Modify: `backend/internal/workflows/params.go`

**Interfaces:**
- Produces: `activities.PlayerSyncResult{PlayersUpserted int}`, `activities.WeekStatsResult{PlayersUpserted int, Finalized bool}`, `activities.ADPRollupResult{PlayersUpserted int}`, `workflows.DiscoveryReport{UsersProcessed, UsersFailed int}`, `workflows.DraftSyncReport{LeaguesProcessed, LeaguesFailed int}`, `workflows.TransactionSyncReport{LeaguesProcessed, LeaguesFailed int}`, `workflows.PlayerSyncReport{PlayersUpserted int}`, `workflows.WeekStatsReport{WeeksFetched, PlayersUpserted int}`, `workflows.ADPRollupDispatchReport{SegmentsScheduled int}`, `workflows.SegmentADPReport{PlayersUpserted int}`, `workflows.BackfillReport{LeaguesReplicated, TransactionsReplicated, DraftHeadersReplicated, DraftPicksReplicated int}` — every later task in this plan depends on these types existing.

- [ ] **Step 1: Append new result types to `backend/internal/activities/params.go`**

Add at the end of the file:

```go

// PlayerSyncResult reports how many players FetchAndUpsertAllPlayers upserted.
type PlayerSyncResult struct {
	PlayersUpserted int
}

// WeekStatsResult reports how many player rows FetchWeekStats upserted for one
// week, and whether Sleeper considers that week finalized.
type WeekStatsResult struct {
	PlayersUpserted int
	Finalized       bool
}

// ADPRollupResult reports how many player rows ComputeSegmentSeasonADP
// upserted for one (segment, season) pair.
type ADPRollupResult struct {
	PlayersUpserted int
}
```

- [ ] **Step 2: Append new report types to `backend/internal/workflows/params.go`**

Add at the end of the file:

```go

// DiscoveryReport summarizes one DiscoveryBatchDispatcher run.
type DiscoveryReport struct {
	UsersProcessed int
	UsersFailed    int
}

// DraftSyncReport summarizes one DraftSyncDispatcher run.
type DraftSyncReport struct {
	LeaguesProcessed int
	LeaguesFailed    int
}

// TransactionSyncReport summarizes one TransactionSyncDispatcher run.
type TransactionSyncReport struct {
	LeaguesProcessed int
	LeaguesFailed    int
}

// PlayerSyncReport summarizes one PlayerDatabaseSyncWorkflow run.
type PlayerSyncReport struct {
	PlayersUpserted int
}

// WeekStatsReport summarizes a SyncWeekStats (or WeekStatsSyncDispatcher) run.
type WeekStatsReport struct {
	WeeksFetched    int
	PlayersUpserted int
}

// ADPRollupDispatchReport summarizes one ADPRollupDispatcher run. Child
// workflows are fire-and-forget (ParentClosePolicy: ABANDON), so this counts
// segments scheduled, not completed.
type ADPRollupDispatchReport struct {
	SegmentsScheduled int
}

// SegmentADPReport summarizes one SegmentSeasonADPRollupWorkflow run.
type SegmentADPReport struct {
	PlayersUpserted int
}

// BackfillReport summarizes one ArchiveBackfillWorkflow execution (not the
// full backfill lifetime across ContinueAsNew hops).
type BackfillReport struct {
	LeaguesReplicated      int
	TransactionsReplicated int
	DraftHeadersReplicated int
	DraftPicksReplicated   int
}
```

- [ ] **Step 3: Verify it builds**

Run: `cd backend && go build ./...`
Expected: exits 0, no output (new struct types are unused so far, which is not a Go compile error).

- [ ] **Step 4: Commit**

```bash
git add backend/internal/activities/params.go backend/internal/workflows/params.go
git commit -m "Add Result/Report struct definitions for Temporal work-done reporting"
```

---

## Task 2: PlayerSync activity returns PlayerSyncResult

**Files:**
- Modify: `backend/internal/activities/player_sync.go`
- Modify: `backend/internal/activities/player_sync_test.go`

**Interfaces:**
- Consumes: `PlayerSyncResult` from Task 1.
- Produces: `(a *PlayerSyncActivities) FetchAndUpsertAllPlayers(ctx context.Context) (PlayerSyncResult, error)` — Task 3 (workflow) depends on this exact signature.

- [ ] **Step 1: Update the test file to expect a result**

Replace the four `if err := psa.FetchAndUpsertAllPlayers(...); err != nil` call sites in `backend/internal/activities/player_sync_test.go` as follows.

In `TestFetchAndUpsertAllPlayers_InsertsPlayers`, replace:
```go
	psa := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := psa.FetchAndUpsertAllPlayers(context.Background()); err != nil {
		t.Fatalf("FetchAndUpsertAllPlayers error: %v", err)
	}
```
with:
```go
	psa := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result, err := psa.FetchAndUpsertAllPlayers(context.Background())
	if err != nil {
		t.Fatalf("FetchAndUpsertAllPlayers error: %v", err)
	}
	if result.PlayersUpserted != 2 {
		t.Errorf("expected PlayersUpserted 2, got %d", result.PlayersUpserted)
	}
```

In `TestFetchAndUpsertAllPlayers_Idempotent`, replace:
```go
	psa := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := psa.FetchAndUpsertAllPlayers(context.Background()); err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if err := psa.FetchAndUpsertAllPlayers(context.Background()); err != nil {
		t.Fatalf("second run error: %v", err)
	}
```
with:
```go
	psa := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result1, err := psa.FetchAndUpsertAllPlayers(context.Background())
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if result1.PlayersUpserted != 1 {
		t.Errorf("expected first run PlayersUpserted 1, got %d", result1.PlayersUpserted)
	}
	result2, err := psa.FetchAndUpsertAllPlayers(context.Background())
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if result2.PlayersUpserted != 1 {
		t.Errorf("expected second run PlayersUpserted 1, got %d", result2.PlayersUpserted)
	}
```

In `TestFetchAndUpsertAllPlayers_NumericYahooAndEspnID`, replace:
```go
	psa := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := psa.FetchAndUpsertAllPlayers(context.Background()); err != nil {
		t.Fatalf("FetchAndUpsertAllPlayers error: %v", err)
	}
```
with:
```go
	psa := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result, err := psa.FetchAndUpsertAllPlayers(context.Background())
	if err != nil {
		t.Fatalf("FetchAndUpsertAllPlayers error: %v", err)
	}
	if result.PlayersUpserted != 1 {
		t.Errorf("expected PlayersUpserted 1, got %d", result.PlayersUpserted)
	}
```

In `TestFetchAndUpsertAllPlayers_UpdatesExisting`, replace:
```go
	psa1 := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv1.URL)}
	if err := psa1.FetchAndUpsertAllPlayers(context.Background()); err != nil {
		t.Fatalf("first run error: %v", err)
	}
```
with:
```go
	psa1 := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv1.URL)}
	if _, err := psa1.FetchAndUpsertAllPlayers(context.Background()); err != nil {
		t.Fatalf("first run error: %v", err)
	}
```
and replace:
```go
	psa2 := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv2.URL)}
	if err := psa2.FetchAndUpsertAllPlayers(context.Background()); err != nil {
		t.Fatalf("second run error: %v", err)
	}
```
with:
```go
	psa2 := &activities.PlayerSyncActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv2.URL)}
	result, err := psa2.FetchAndUpsertAllPlayers(context.Background())
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if result.PlayersUpserted != 1 {
		t.Errorf("expected PlayersUpserted 1, got %d", result.PlayersUpserted)
	}
```

- [ ] **Step 2: Run tests to verify they fail to compile**

Run: `cd backend && go test ./internal/activities/... -run TestFetchAndUpsertAllPlayers -v`
Expected: FAIL — compile error, `psa.FetchAndUpsertAllPlayers(...)` returns one value (`error`), not two.

- [ ] **Step 3: Update `backend/internal/activities/player_sync.go`**

Replace the whole `FetchAndUpsertAllPlayers` function body:

```go
func (a *PlayerSyncActivities) FetchAndUpsertAllPlayers(ctx context.Context) (PlayerSyncResult, error) {
	players, err := a.Sleeper.GetAllPlayers(ctx, "nfl")
	if err != nil {
		return PlayerSyncResult{}, err
	}

	now := time.Now().UTC()
	batch := make([]models.SleeperPlayer, 0, 100)
	processed := 0

	for id, p := range players {
		batch = append(batch, models.SleeperPlayer{
			SleeperPlayerID: id,
			EspnID:          string(p.EspnID),
			YahooID:         string(p.YahooID),
			FullName:        p.FullName,
			Position:        p.Position,
			NflTeam:         p.Team,
			Age:             p.Age,
			YearsExp:        p.YearsExp,
			LastFetchedAt:   &now,
		})
		if len(batch) >= 100 {
			if err := a.upsertBatch(ctx, batch); err != nil {
				return PlayerSyncResult{PlayersUpserted: processed}, err
			}
			processed += len(batch)
			activity.RecordHeartbeat(ctx, processed)
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		if err := a.upsertBatch(ctx, batch); err != nil {
			return PlayerSyncResult{PlayersUpserted: processed}, err
		}
		processed += len(batch)
	}
	return PlayerSyncResult{PlayersUpserted: processed}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/activities/... -run TestFetchAndUpsertAllPlayers -v`
Expected: PASS (all 4 subtests)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/activities/player_sync.go backend/internal/activities/player_sync_test.go
git commit -m "FetchAndUpsertAllPlayers: return PlayerSyncResult with upsert count"
```

---

## Task 3: PlayerDatabaseSyncWorkflow returns PlayerSyncReport

**Files:**
- Modify: `backend/internal/workflows/player_sync.go`
- Modify: `backend/internal/workflows/workflows_test.go` (function `TestPlayerSync_CallsFetchAndUpsert`)

**Interfaces:**
- Consumes: `activities.PlayerSyncResult` (Task 2), `workflows.PlayerSyncReport` (Task 1).
- Produces: `PlayerDatabaseSyncWorkflow(ctx workflow.Context) (PlayerSyncReport, error)`.

- [ ] **Step 1: Update `TestPlayerSync_CallsFetchAndUpsert` in `backend/internal/workflows/workflows_test.go`**

Replace:
```go
func TestPlayerSync_CallsFetchAndUpsert(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	psa := &activities.PlayerSyncActivities{}
	env.OnActivity(psa.FetchAndUpsertAllPlayers, mock.Anything).Return(nil)

	env.ExecuteWorkflow(workflows.PlayerDatabaseSyncWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}
```
with:
```go
func TestPlayerSync_CallsFetchAndUpsert(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	psa := &activities.PlayerSyncActivities{}
	env.OnActivity(psa.FetchAndUpsertAllPlayers, mock.Anything).Return(activities.PlayerSyncResult{PlayersUpserted: 42}, nil)

	env.ExecuteWorkflow(workflows.PlayerDatabaseSyncWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report workflows.PlayerSyncReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.PlayerSyncReport{PlayersUpserted: 42}, report)
	env.AssertExpectations(t)
}
```

- [ ] **Step 2: Run test to verify it fails to compile**

Run: `cd backend && go test ./internal/workflows/... -run TestPlayerSync_CallsFetchAndUpsert -v`
Expected: FAIL — `psa.FetchAndUpsertAllPlayers` mock return type mismatch (workflow still returns bare `error`).

- [ ] **Step 3: Update `backend/internal/workflows/player_sync.go`**

Replace the whole `PlayerDatabaseSyncWorkflow` function body:

```go
func PlayerDatabaseSyncWorkflow(ctx workflow.Context) (PlayerSyncReport, error) {
	psa := &activities.PlayerSyncActivities{}
	actCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    10 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		},
	})
	var res activities.PlayerSyncResult
	if err := workflow.ExecuteActivity(actCtx, psa.FetchAndUpsertAllPlayers).Get(ctx, &res); err != nil {
		return PlayerSyncReport{}, err
	}
	return PlayerSyncReport{PlayersUpserted: res.PlayersUpserted}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/workflows/... -run TestPlayerSync_CallsFetchAndUpsert -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/workflows/player_sync.go backend/internal/workflows/workflows_test.go
git commit -m "PlayerDatabaseSyncWorkflow: return PlayerSyncReport"
```

---

## Task 4: WeekStats activity FetchWeekStats returns WeekStatsResult

**Files:**
- Modify: `backend/internal/activities/week_stats.go`
- Modify: `backend/internal/activities/week_stats_test.go`

**Interfaces:**
- Consumes: `WeekStatsResult` from Task 1.
- Produces: `(a *WeekStatsActivities) FetchWeekStats(ctx context.Context, params FetchWeekStatsParams) (WeekStatsResult, error)` — Task 5 depends on this exact signature.

- [ ] **Step 1: Update `backend/internal/activities/week_stats_test.go` call sites**

In `TestFetchWeekStats_FiltersToFantasyPositionsAndUpserts`, replace:
```go
	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 3})
	if err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}
```
with:
```go
	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result, err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 3})
	if err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}
	if result.PlayersUpserted != 1 {
		t.Errorf("expected PlayersUpserted 1, got %d", result.PlayersUpserted)
	}
	if !result.Finalized {
		t.Errorf("expected Finalized true (week 3, current week 10), got false")
	}
```

In `TestFetchWeekStats_RefetchOverwrites`, replace:
```go
	srv1 := weekStatsServer(t, `{"421":{"pts_ppr":10}}`, 10, "2025")
	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv1.URL)}
	if err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 3}); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	srv1.Close()

	srv2 := weekStatsServer(t, `{"421":{"pts_ppr":15.5}}`, 10, "2025")
	defer srv2.Close()
	wsa2 := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv2.URL)}
	if err := wsa2.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 3}); err != nil {
		t.Fatalf("second fetch: %v", err)
	}
```
with:
```go
	srv1 := weekStatsServer(t, `{"421":{"pts_ppr":10}}`, 10, "2025")
	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv1.URL)}
	if _, err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 3}); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	srv1.Close()

	srv2 := weekStatsServer(t, `{"421":{"pts_ppr":15.5}}`, 10, "2025")
	defer srv2.Close()
	wsa2 := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv2.URL)}
	result, err := wsa2.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 3})
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if result.PlayersUpserted != 1 {
		t.Errorf("expected PlayersUpserted 1 on refetch, got %d", result.PlayersUpserted)
	}
```

In `TestFetchWeekStats_MarksFinalized_PastWeek`, replace:
```go
	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 3}); err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}
```
with:
```go
	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result, err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 3})
	if err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}
	if !result.Finalized {
		t.Errorf("expected result.Finalized true, got false")
	}
```

In `TestFetchWeekStats_NotFinalized_CurrentWeek`, replace:
```go
	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 10}); err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}
```
with:
```go
	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result, err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 10})
	if err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}
	if result.Finalized {
		t.Errorf("expected result.Finalized false, got true")
	}
```

In `TestFetchWeekStats_PastSeasonAlwaysFinalized`, replace:
```go
	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 18}); err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}
```
with:
```go
	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result, err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 18})
	if err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}
	if !result.Finalized {
		t.Errorf("expected result.Finalized true, got false")
	}
```

In `TestFetchWeekStats_EmptyWeek404_NoRowsButFetchStamped`, replace:
```go
	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	if err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 20}); err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}
```
with:
```go
	wsa := &activities.WeekStatsActivities{DB: db, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	result, err := wsa.FetchWeekStats(context.Background(), activities.FetchWeekStatsParams{Season: "2025", Week: 20})
	if err != nil {
		t.Fatalf("FetchWeekStats error: %v", err)
	}
	if result.PlayersUpserted != 0 {
		t.Errorf("expected PlayersUpserted 0 for 404 week, got %d", result.PlayersUpserted)
	}
```

- [ ] **Step 2: Run tests to verify they fail to compile**

Run: `cd backend && go test ./internal/activities/... -run TestFetchWeekStats -v`
Expected: FAIL — compile error, `wsa.FetchWeekStats(...)` still returns a single `error`.

- [ ] **Step 3: Update `backend/internal/activities/week_stats.go`**

Replace the whole `FetchWeekStats` function body:

```go
func (a *WeekStatsActivities) FetchWeekStats(ctx context.Context, params FetchWeekStatsParams) (WeekStatsResult, error) {
	raw, err := a.Sleeper.GetWeekStats(ctx, params.Season, params.Week)
	if err != nil {
		var nfe *sleeper.NotFoundError
		if !errors.As(err, &nfe) {
			return WeekStatsResult{}, err
		}
		raw = nil // no stats published for this week yet
	}

	upserted := 0
	if len(raw) > 0 {
		var players []models.SleeperPlayer
		if err := a.DB.WithContext(ctx).
			Where("position IN ?", fantasyPositions).
			Find(&players).Error; err != nil {
			return WeekStatsResult{}, err
		}
		fantasyIDs := make(map[string]struct{}, len(players))
		for _, p := range players {
			fantasyIDs[p.SleeperPlayerID] = struct{}{}
		}

		for id, statBytes := range raw {
			if _, ok := fantasyIDs[id]; !ok {
				continue
			}
			var pts weekStatPoints
			if err := json.Unmarshal(statBytes, &pts); err != nil {
				return WeekStatsResult{PlayersUpserted: upserted}, err
			}
			row := models.SleeperPlayerWeekStat{
				Season:          params.Season,
				Week:            params.Week,
				SleeperPlayerID: id,
				PtsPPR:          pts.PtsPPR,
				PtsHalfPPR:      pts.PtsHalfPPR,
				PtsStd:          pts.PtsStd,
				Stats:           json.RawMessage(statBytes),
			}
			if err := a.DB.WithContext(ctx).Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "season"}, {Name: "week"}, {Name: "sleeper_player_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"pts_ppr", "pts_half_ppr", "pts_std", "stats", "updated_at",
				}),
			}).Create(&row).Error; err != nil {
				return WeekStatsResult{PlayersUpserted: upserted}, err
			}
			upserted++
		}
	}

	state, err := a.Sleeper.GetNFLState(ctx)
	if err != nil {
		return WeekStatsResult{PlayersUpserted: upserted}, err
	}
	finalized := params.Season < state.Season || (params.Season == state.Season && params.Week < state.Week)

	now := time.Now().UTC()
	fetchRow := models.SleeperWeekStatFetch{
		Season:        params.Season,
		Week:          params.Week,
		LastFetchedAt: &now,
		Finalized:     finalized,
	}
	if err := a.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "season"}, {Name: "week"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_fetched_at", "finalized"}),
	}).Create(&fetchRow).Error; err != nil {
		return WeekStatsResult{PlayersUpserted: upserted}, err
	}

	return WeekStatsResult{PlayersUpserted: upserted, Finalized: finalized}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/activities/... -run TestFetchWeekStats -v`
Expected: PASS (all subtests)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/activities/week_stats.go backend/internal/activities/week_stats_test.go
git commit -m "FetchWeekStats: return WeekStatsResult with upsert count and finalized flag"
```

---

## Task 5: SyncWeekStats and WeekStatsSyncDispatcher return WeekStatsReport

**Files:**
- Modify: `backend/internal/workflows/week_stats_sync.go`
- Modify: `backend/internal/workflows/workflows_test.go` (functions `TestSyncWeekStats_SkipsFinalizedWeeks`, `TestSyncWeekStats_AllWeeksFinalized_NoFetchCalls`, `TestWeekStatsSyncDispatcher_ResolvesSeasonAndSyncs`)

**Interfaces:**
- Consumes: `activities.WeekStatsResult` (Task 4), `workflows.WeekStatsReport` (Task 1).
- Produces: `SyncWeekStats(ctx workflow.Context, params SyncWeekStatsParams) (WeekStatsReport, error)`, `WeekStatsSyncDispatcher(ctx workflow.Context) (WeekStatsReport, error)`.

- [ ] **Step 1: Update the three tests in `backend/internal/workflows/workflows_test.go`**

Replace `TestSyncWeekStats_SkipsFinalizedWeeks`:
```go
func TestSyncWeekStats_SkipsFinalizedWeeks(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	wsa := &activities.WeekStatsActivities{}
	// Weeks 1 and 2 already finalized — only weeks 3-18 should be fetched.
	env.OnActivity(wsa.GetFinalizedWeeks, mock.Anything, activities.GetFinalizedWeeksParams{Season: "2025"}).
		Return([]int{1, 2}, nil)
	for week := 3; week <= 18; week++ {
		env.OnActivity(wsa.FetchWeekStats, mock.Anything, activities.FetchWeekStatsParams{Season: "2025", Week: week}).
			Return(activities.WeekStatsResult{PlayersUpserted: 1}, nil)
	}

	env.ExecuteWorkflow(workflows.SyncWeekStats, workflows.SyncWeekStatsParams{Season: "2025"})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report workflows.WeekStatsReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.WeekStatsReport{WeeksFetched: 16, PlayersUpserted: 16}, report)
	env.AssertExpectations(t)
}
```

Replace `TestSyncWeekStats_AllWeeksFinalized_NoFetchCalls`:
```go
func TestSyncWeekStats_AllWeeksFinalized_NoFetchCalls(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	allWeeks := make([]int, 0, 18)
	for w := 1; w <= 18; w++ {
		allWeeks = append(allWeeks, w)
	}

	wsa := &activities.WeekStatsActivities{}
	env.OnActivity(wsa.GetFinalizedWeeks, mock.Anything, activities.GetFinalizedWeeksParams{Season: "2025"}).
		Return(allWeeks, nil)

	env.ExecuteWorkflow(workflows.SyncWeekStats, workflows.SyncWeekStatsParams{Season: "2025"})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report workflows.WeekStatsReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.WeekStatsReport{}, report)
	env.AssertExpectations(t)
}
```

Replace `TestWeekStatsSyncDispatcher_ResolvesSeasonAndSyncs`:
```go
func TestWeekStatsSyncDispatcher_ResolvesSeasonAndSyncs(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	wsa := &activities.WeekStatsActivities{}
	env.OnActivity(wsa.GetCurrentSeason, mock.Anything).Return("2025", nil)
	env.OnActivity(wsa.GetFinalizedWeeks, mock.Anything, activities.GetFinalizedWeeksParams{Season: "2025"}).
		Return([]int{}, nil)
	for week := 1; week <= 18; week++ {
		env.OnActivity(wsa.FetchWeekStats, mock.Anything, activities.FetchWeekStatsParams{Season: "2025", Week: week}).
			Return(activities.WeekStatsResult{PlayersUpserted: 1}, nil)
	}

	env.ExecuteWorkflow(workflows.WeekStatsSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report workflows.WeekStatsReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.WeekStatsReport{WeeksFetched: 18, PlayersUpserted: 18}, report)
	env.AssertExpectations(t)
}
```

- [ ] **Step 2: Run tests to verify they fail to compile**

Run: `cd backend && go test ./internal/workflows/... -run 'TestSyncWeekStats|TestWeekStatsSyncDispatcher' -v`
Expected: FAIL — compile error, mocked `FetchWeekStats` return type doesn't match the still-single-`error` workflow signature.

- [ ] **Step 3: Update `backend/internal/workflows/week_stats_sync.go`**

Replace both function bodies:

```go
func SyncWeekStats(ctx workflow.Context, params SyncWeekStatsParams) (WeekStatsReport, error) {
	wsa := &activities.WeekStatsActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var finalizedWeeks []int
	if err := workflow.ExecuteActivity(actCtx, wsa.GetFinalizedWeeks, activities.GetFinalizedWeeksParams{Season: params.Season}).Get(ctx, &finalizedWeeks); err != nil {
		return WeekStatsReport{}, err
	}
	finalized := make(map[int]bool, len(finalizedWeeks))
	for _, w := range finalizedWeeks {
		finalized[w] = true
	}

	var report WeekStatsReport
	for week := 1; week <= lastFantasyWeek; week++ {
		if finalized[week] {
			continue
		}
		var res activities.WeekStatsResult
		if err := workflow.ExecuteActivity(actCtx, wsa.FetchWeekStats, activities.FetchWeekStatsParams{Season: params.Season, Week: week}).Get(ctx, &res); err != nil {
			return report, err
		}
		report.WeeksFetched++
		report.PlayersUpserted += res.PlayersUpserted
	}
	return report, nil
}

// WeekStatsSyncDispatcher is the scheduled entry point: it resolves the current NFL
// season via Sleeper's state endpoint, then runs SyncWeekStats for it.
func WeekStatsSyncDispatcher(ctx workflow.Context) (WeekStatsReport, error) {
	wsa := &activities.WeekStatsActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var season string
	if err := workflow.ExecuteActivity(actCtx, wsa.GetCurrentSeason).Get(ctx, &season); err != nil {
		return WeekStatsReport{}, err
	}
	return SyncWeekStats(ctx, SyncWeekStatsParams{Season: season})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/workflows/... -run 'TestSyncWeekStats|TestWeekStatsSyncDispatcher' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/workflows/week_stats_sync.go backend/internal/workflows/workflows_test.go
git commit -m "SyncWeekStats/WeekStatsSyncDispatcher: return WeekStatsReport"
```

---

## Task 6: ADPRollup activity ComputeSegmentSeasonADP returns ADPRollupResult

**Files:**
- Modify: `backend/internal/activities/adp_rollup.go`
- Modify: `backend/internal/activities/adp_rollup_test.go`

**Interfaces:**
- Consumes: `ADPRollupResult` from Task 1.
- Produces: `(a *ADPRollupActivities) ComputeSegmentSeasonADP(ctx context.Context, params ComputeSegmentSeasonADPParams) (ADPRollupResult, error)` — Task 7 depends on this exact signature.

- [ ] **Step 1: Update call sites in `backend/internal/activities/adp_rollup_test.go`**

In `TestComputeSegmentSeasonADP_ComputesAverages`, replace:
```go
	a := &activities.ADPRollupActivities{Read: db, Write: db}
	if err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	}); err != nil {
		t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
	}
```
with:
```go
	a := &activities.ADPRollupActivities{Read: db, Write: db}
	result, err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	})
	if err != nil {
		t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
	}
	if result.PlayersUpserted != 2 {
		t.Errorf("expected PlayersUpserted 2, got %d", result.PlayersUpserted)
	}
```

In `TestComputeSegmentSeasonADP_CIFieldsAreZeroUnderSQLite`, replace:
```go
	a := &activities.ADPRollupActivities{Read: db, Write: db}
	if err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	}); err != nil {
		t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
	}
```
with:
```go
	a := &activities.ADPRollupActivities{Read: db, Write: db}
	result, err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	})
	if err != nil {
		t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
	}
	if result.PlayersUpserted != 1 {
		t.Errorf("expected PlayersUpserted 1, got %d", result.PlayersUpserted)
	}
```

In `TestComputeSegmentSeasonADP_ExcludesAuctionAndNonRedraft`, replace:
```go
	a := &activities.ADPRollupActivities{Read: db, Write: db}
	if err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	}); err != nil {
		t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
	}
```
with:
```go
	a := &activities.ADPRollupActivities{Read: db, Write: db}
	result, err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	})
	if err != nil {
		t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
	}
	if result.PlayersUpserted != 0 {
		t.Errorf("expected PlayersUpserted 0 (auction/dynasty excluded), got %d", result.PlayersUpserted)
	}
```

In `TestComputeSegmentSeasonADP_NoMinDraftsThresholdAtWriteTime`, replace:
```go
	a := &activities.ADPRollupActivities{Read: db, Write: db}
	if err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	}); err != nil {
		t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
	}
```
with:
```go
	a := &activities.ADPRollupActivities{Read: db, Write: db}
	result, err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	})
	if err != nil {
		t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
	}
	if result.PlayersUpserted != 1 {
		t.Errorf("expected PlayersUpserted 1, got %d", result.PlayersUpserted)
	}
```

In `TestComputeSegmentSeasonADP_UpsertOverwritesPreviousRun`, replace:
```go
	a := &activities.ADPRollupActivities{Read: db, Write: db}
	run := func() {
		if err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
			Segment: adpTestSegment,
			Season:  "2024",
		}); err != nil {
			t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
		}
	}
```
with:
```go
	a := &activities.ADPRollupActivities{Read: db, Write: db}
	run := func() {
		result, err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
			Segment: adpTestSegment,
			Season:  "2024",
		})
		if err != nil {
			t.Fatalf("ComputeSegmentSeasonADP error: %v", err)
		}
		if result.PlayersUpserted != 1 {
			t.Errorf("expected PlayersUpserted 1 (one distinct player), got %d", result.PlayersUpserted)
		}
	}
```

In `TestComputeSegmentSeasonADP_ReadsFromArchiveWritesToCloud`, replace:
```go
	a := &activities.ADPRollupActivities{Read: read, Write: write}
	if err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	}); err != nil {
		t.Fatalf("ComputeSegmentSeasonADP: %v", err)
	}
```
with:
```go
	a := &activities.ADPRollupActivities{Read: read, Write: write}
	result, err := a.ComputeSegmentSeasonADP(context.Background(), activities.ComputeSegmentSeasonADPParams{
		Segment: adpTestSegment,
		Season:  "2024",
	})
	if err != nil {
		t.Fatalf("ComputeSegmentSeasonADP: %v", err)
	}
	if result.PlayersUpserted != 2 {
		t.Errorf("expected PlayersUpserted 2, got %d", result.PlayersUpserted)
	}
```

- [ ] **Step 2: Run tests to verify they fail to compile**

Run: `cd backend && go test ./internal/activities/... -run TestComputeSegmentSeasonADP -v`
Expected: FAIL — compile error, `a.ComputeSegmentSeasonADP(...)` still returns a single `error`.

- [ ] **Step 3: Update `backend/internal/activities/adp_rollup.go`**

Replace the whole `ComputeSegmentSeasonADP` function body:

```go
func (a *ADPRollupActivities) ComputeSegmentSeasonADP(ctx context.Context, params ComputeSegmentSeasonADPParams) (ADPRollupResult, error) {
	db := a.Read.WithContext(ctx).
		Table("sleeper_draft_picks p").
		Select(adpSelectClause(a.Read.Dialector.Name())).
		Joins("JOIN sleeper_drafts d ON d.sleeper_draft_id = p.sleeper_draft_id").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id").
		Where("d.status = ? AND d.type IN ? AND l.league_type = ? AND d.season = ?",
			"complete", qualifyingDraftTypes, "redraft", params.Season).
		Where("p.sleeper_player_id != ''")
	db = applySegmentPredicate(db, params.Segment)

	var rows []adpRow
	if err := db.Group("p.sleeper_player_id").Scan(&rows).Error; err != nil {
		return ADPRollupResult{}, err
	}
	if len(rows) == 0 {
		return ADPRollupResult{}, nil
	}

	segmentKey := params.Segment.Key()
	records := make([]models.DraftADP, len(rows))
	for i, r := range rows {
		records[i] = models.DraftADP{
			Segment:         segmentKey,
			Season:          params.Season,
			SleeperPlayerID: r.SleeperPlayerID,
			AvgPickNo:       r.AvgPickNo,
			PickCount:       r.PickCount,
			MinPickNo:       r.MinPickNo,
			MaxPickNo:       r.MaxPickNo,
			CILowPickNo:     r.CILowPickNo,
			CIHighPickNo:    r.CIHighPickNo,
		}
	}

	// One batched upsert instead of one round-trip per player: with a large
	// qualifying draft pool (hundreds of distinct players), a per-row loop
	// could exceed the activity's StartToCloseTimeout partway through,
	// leaving only whichever players were reached — in whatever order
	// Postgres happened to return the GROUP BY in — upserted for that
	// segment/season, with no rollback. A single batched statement is both
	// atomic and one round trip instead of hundreds.
	if err := a.Write.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "segment"}, {Name: "season"}, {Name: "sleeper_player_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"avg_pick_no", "pick_count", "min_pick_no", "max_pick_no", "ci_low_pick_no", "ci_high_pick_no", "updated_at",
		}),
	}).CreateInBatches(&records, 500).Error; err != nil {
		return ADPRollupResult{}, err
	}

	return ADPRollupResult{PlayersUpserted: len(records)}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/activities/... -run TestComputeSegmentSeasonADP -v`
Expected: PASS (all subtests). Note: `TestListADPSeasons_ReadsFromArchiveOnly` and `TestComputeSegmentSeasonADP_ReadsFromArchiveWritesToCloud` require `TEST_DATABASE_URL` and will `t.Skip()` if unset — that's expected, not a failure.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/activities/adp_rollup.go backend/internal/activities/adp_rollup_test.go
git commit -m "ComputeSegmentSeasonADP: return ADPRollupResult with upsert count"
```

---

## Task 7: ADPRollupDispatcher and SegmentSeasonADPRollupWorkflow return reports

**Files:**
- Modify: `backend/internal/workflows/adp_rollup.go`
- Modify: `backend/internal/workflows/workflows_test.go` (functions `TestADPRollupDispatcher_SpawnsChildPerSeasonSegment`, `TestADPRollupDispatcher_ChildWorkflowIDIsDeterministic`, `TestADPRollupDispatcher_NoSeasons_NoChildren`, `TestSegmentSeasonADPRollupWorkflow_CallsComputeActivity`, `TestSegmentSeasonADPRollupWorkflow_ActivityFailure_WorkflowStillSucceeds`)

**Interfaces:**
- Consumes: `activities.ADPRollupResult` (Task 6), `workflows.ADPRollupDispatchReport` and `workflows.SegmentADPReport` (Task 1).
- Produces: `ADPRollupDispatcher(ctx workflow.Context) (ADPRollupDispatchReport, error)`, `SegmentSeasonADPRollupWorkflow(ctx workflow.Context, params SegmentSeasonADPParams) (SegmentADPReport, error)`.

- [ ] **Step 1: Update the five tests in `backend/internal/workflows/workflows_test.go`**

Replace `TestADPRollupDispatcher_SpawnsChildPerSeasonSegment`:
```go
func TestADPRollupDispatcher_SpawnsChildPerSeasonSegment(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	ara := &activities.ADPRollupActivities{}
	env.OnActivity(ara.ListADPSeasons, mock.Anything).Return([]string{"2024"}, nil)

	env.RegisterWorkflow(workflows.SegmentSeasonADPRollupWorkflow)
	segments := models.AllADPSegments()
	if len(segments) != 24 {
		t.Fatalf("expected 24 segments, got %d", len(segments))
	}
	for _, seg := range segments {
		env.OnWorkflow(workflows.SegmentSeasonADPRollupWorkflow, mock.Anything, workflows.SegmentSeasonADPParams{
			Segment: seg,
			Season:  "2024",
		}).Return(workflows.SegmentADPReport{}, nil)
	}

	env.ExecuteWorkflow(workflows.ADPRollupDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report workflows.ADPRollupDispatchReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.ADPRollupDispatchReport{SegmentsScheduled: 24}, report)
	env.AssertExpectations(t)
}
```

Replace `TestADPRollupDispatcher_ChildWorkflowIDIsDeterministic`:
```go
func TestADPRollupDispatcher_ChildWorkflowIDIsDeterministic(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	ara := &activities.ADPRollupActivities{}
	env.OnActivity(ara.ListADPSeasons, mock.Anything).Return([]string{"2024"}, nil)

	env.RegisterWorkflow(workflows.SegmentSeasonADPRollupWorkflow)

	seenIDs := make(map[string]bool)
	for _, seg := range models.AllADPSegments() {
		env.OnActivity(ara.ComputeSegmentSeasonADP, mock.MatchedBy(func(ctx context.Context) bool {
			seenIDs[activity.GetInfo(ctx).WorkflowExecution.ID] = true
			return true
		}), activities.ComputeSegmentSeasonADPParams{Segment: seg, Season: "2024"}).Return(activities.ADPRollupResult{}, nil)
	}

	env.ExecuteWorkflow(workflows.ADPRollupDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	for _, seg := range models.AllADPSegments() {
		wantID := "2024-" + seg.Key()
		require.True(t, seenIDs[wantID], "expected child workflow ID %q to have been used", wantID)
	}
}
```

Replace `TestADPRollupDispatcher_NoSeasons_NoChildren`:
```go
func TestADPRollupDispatcher_NoSeasons_NoChildren(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	ara := &activities.ADPRollupActivities{}
	env.OnActivity(ara.ListADPSeasons, mock.Anything).Return([]string{}, nil)

	env.ExecuteWorkflow(workflows.ADPRollupDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report workflows.ADPRollupDispatchReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.ADPRollupDispatchReport{}, report)
}
```

Replace `TestSegmentSeasonADPRollupWorkflow_CallsComputeActivity`:
```go
func TestSegmentSeasonADPRollupWorkflow_CallsComputeActivity(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	seg := models.ADPSegment{LeagueSize: "12", ScoringFormat: "ppr", Superflex: true}
	ara := &activities.ADPRollupActivities{}
	env.OnActivity(ara.ComputeSegmentSeasonADP, mock.Anything, activities.ComputeSegmentSeasonADPParams{
		Segment: seg,
		Season:  "2024",
	}).Return(activities.ADPRollupResult{PlayersUpserted: 5}, nil)

	env.ExecuteWorkflow(workflows.SegmentSeasonADPRollupWorkflow, workflows.SegmentSeasonADPParams{
		Segment: seg,
		Season:  "2024",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report workflows.SegmentADPReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.SegmentADPReport{PlayersUpserted: 5}, report)
	env.AssertExpectations(t)
}
```

Replace `TestSegmentSeasonADPRollupWorkflow_ActivityFailure_WorkflowStillSucceeds`:
```go
func TestSegmentSeasonADPRollupWorkflow_ActivityFailure_WorkflowStillSucceeds(t *testing.T) {
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	seg := models.ADPSegment{LeagueSize: "12", ScoringFormat: "ppr", Superflex: true}
	ara := &activities.ADPRollupActivities{}
	env.OnActivity(ara.ComputeSegmentSeasonADP, mock.Anything, activities.ComputeSegmentSeasonADPParams{
		Segment: seg,
		Season:  "2024",
	}).Return(activities.ADPRollupResult{}, temporal.NewApplicationError("db error", "DB_ERROR", nil))

	env.ExecuteWorkflow(workflows.SegmentSeasonADPRollupWorkflow, workflows.SegmentSeasonADPParams{
		Segment: seg,
		Season:  "2024",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError()) // logged and swallowed, not propagated
	var report workflows.SegmentADPReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.SegmentADPReport{}, report)
	env.AssertExpectations(t)
}
```

- [ ] **Step 2: Run tests to verify they fail to compile**

Run: `cd backend && go test ./internal/workflows/... -run 'TestADPRollupDispatcher|TestSegmentSeasonADPRollupWorkflow' -v`
Expected: FAIL — compile error, mocks now return two values but the workflows still return a single `error`.

- [ ] **Step 3: Update `backend/internal/workflows/adp_rollup.go`**

Replace both function bodies:

```go
func ADPRollupDispatcher(ctx workflow.Context) (ADPRollupDispatchReport, error) {
	ara := &activities.ADPRollupActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var seasons []string
	if err := workflow.ExecuteActivity(actCtx, ara.ListADPSeasons).Get(ctx, &seasons); err != nil {
		return ADPRollupDispatchReport{}, err
	}

	var report ADPRollupDispatchReport
	for _, season := range seasons {
		for _, seg := range models.AllADPSegments() {
			cwo := workflow.ChildWorkflowOptions{
				WorkflowID:        fmt.Sprintf("%s-%s", season, seg.Key()),
				TaskQueue:         TaskQueueADP,
				ParentClosePolicy: enumspb.PARENT_CLOSE_POLICY_ABANDON,
			}
			params := SegmentSeasonADPParams{Segment: seg, Season: season}
			f := workflow.ExecuteChildWorkflow(workflow.WithChildOptions(ctx, cwo), SegmentSeasonADPRollupWorkflow, params)
			if err := f.GetChildWorkflowExecution().Get(ctx, nil); err != nil {
				workflow.GetLogger(ctx).Warn("failed to start SegmentSeasonADPRollupWorkflow",
					"segment", seg.Key(), "season", season, "error", err)
				continue
			}
			report.SegmentsScheduled++
		}
	}
	return report, nil
}

// SegmentSeasonADPRollupWorkflow computes and upserts ADP for one
// (segment, season) pair. A compute failure is logged rather than returned,
// so one bad segment/season doesn't surface as a workflow failure.
func SegmentSeasonADPRollupWorkflow(ctx workflow.Context, params SegmentSeasonADPParams) (SegmentADPReport, error) {
	ara := &activities.ADPRollupActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)

	var res activities.ADPRollupResult
	if err := workflow.ExecuteActivity(actCtx, ara.ComputeSegmentSeasonADP, activities.ComputeSegmentSeasonADPParams{
		Segment: params.Segment,
		Season:  params.Season,
	}).Get(ctx, &res); err != nil {
		workflow.GetLogger(ctx).Warn("ComputeSegmentSeasonADP failed",
			"segment", params.Segment.Key(), "season", params.Season, "error", err)
		return SegmentADPReport{}, nil
	}
	return SegmentADPReport{PlayersUpserted: res.PlayersUpserted}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/workflows/... -run 'TestADPRollupDispatcher|TestSegmentSeasonADPRollupWorkflow' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/workflows/adp_rollup.go backend/internal/workflows/workflows_test.go
git commit -m "ADPRollupDispatcher/SegmentSeasonADPRollupWorkflow: return work-done reports"
```

---

## Task 8: DiscoveryBatchDispatcher returns DiscoveryReport

**Files:**
- Modify: `backend/internal/workflows/dispatcher.go`
- Modify: `backend/internal/workflows/workflows_test.go` (functions `TestDiscoveryDispatcher_DrainsUntilShortClaim`, `TestDiscoveryDispatcher_EmptyClaimStopsImmediately`, `TestDiscoveryDispatcher_BatchFailureDoesNotFailRun`)

**Interfaces:**
- Consumes: `activities.SyncBatchResult` (unchanged, pre-existing), `workflows.DiscoveryReport` (Task 1).
- Produces: `DiscoveryBatchDispatcher(ctx workflow.Context) (DiscoveryReport, error)`.

- [ ] **Step 1: Update the three tests in `backend/internal/workflows/workflows_test.go`**

In `TestDiscoveryDispatcher_DrainsUntilShortClaim`, after `env.ExecuteWorkflow(workflows.DiscoveryBatchDispatcher)` and before `env.AssertExpectations(t)`, insert:
```go

	var report workflows.DiscoveryReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.DiscoveryReport{UsersProcessed: 3}, report)
```
so the end of the test reads:
```go
	env.ExecuteWorkflow(workflows.DiscoveryBatchDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report workflows.DiscoveryReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.DiscoveryReport{UsersProcessed: 3}, report)
	env.AssertExpectations(t)
}
```

In `TestDiscoveryDispatcher_EmptyClaimStopsImmediately`, make the end read:
```go
	env.ExecuteWorkflow(workflows.DiscoveryBatchDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report workflows.DiscoveryReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.DiscoveryReport{}, report)
	env.AssertExpectations(t)
}
```

In `TestDiscoveryDispatcher_BatchFailureDoesNotFailRun`, make the end read:
```go
	env.ExecuteWorkflow(workflows.DiscoveryBatchDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	// Failed batches are logged; the users' claims expire and re-queue.
	require.NoError(t, env.GetWorkflowError())
	var report workflows.DiscoveryReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.DiscoveryReport{}, report)
	env.AssertExpectations(t)
}
```

- [ ] **Step 2: Run tests to verify they fail to compile**

Run: `cd backend && go test ./internal/workflows/... -run TestDiscoveryDispatcher -v`
Expected: FAIL — `env.GetWorkflowResult(&report)` errors because the workflow still returns bare `error` (no result to decode).

- [ ] **Step 3: Update `backend/internal/workflows/dispatcher.go`**

Replace the whole `DiscoveryBatchDispatcher` function body:

```go
func DiscoveryBatchDispatcher(ctx workflow.Context) (DiscoveryReport, error) {
	da := &activities.DiscoveryActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	batchCtx := workflow.WithActivityOptions(ctx, batchActivityOptions)
	logger := workflow.GetLogger(ctx)

	var cfg activities.DiscoveryConfig
	if err := workflow.ExecuteActivity(actCtx, da.GetDiscoveryConfig).Get(ctx, &cfg); err != nil {
		return DiscoveryReport{}, err
	}

	var report DiscoveryReport
	for iter := 0; iter < MaxDispatchIterations; iter++ {
		var futures []workflow.Future
		drained := false
		for k := 0; k < cfg.ParallelBatches; k++ {
			var userIDs []string
			err := workflow.ExecuteActivity(actCtx, da.ClaimStaleUsers, activities.ClaimStaleUsersParams{
				BatchSize: cfg.BatchSize,
			}).Get(ctx, &userIDs)
			if err != nil {
				logger.Error("user claim failed; stopping dispatch for this run", "error", err)
				drained = true
				break
			}
			if len(userIDs) == 0 {
				drained = true
				break
			}
			futures = append(futures, workflow.ExecuteActivity(batchCtx, da.DiscoverUsersBatch, activities.DiscoverUsersBatchParams{
				UserIDs:     userIDs,
				Concurrency: cfg.Concurrency,
			}))
			if len(userIDs) < cfg.BatchSize {
				drained = true
				break
			}
		}
		for _, f := range futures {
			var res activities.SyncBatchResult
			if err := f.Get(ctx, &res); err != nil {
				logger.Error("discovery batch failed; claims will expire and re-queue", "error", err)
				continue
			}
			logger.Info("discovery batch done", "processed", res.Processed, "failed", res.Failed)
			report.UsersProcessed += res.Processed
			report.UsersFailed += res.Failed
		}
		if drained {
			break
		}
	}
	return report, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/workflows/... -run TestDiscoveryDispatcher -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/workflows/dispatcher.go backend/internal/workflows/workflows_test.go
git commit -m "DiscoveryBatchDispatcher: return DiscoveryReport"
```

---

## Task 9: DraftSyncDispatcher returns DraftSyncReport

**Files:**
- Modify: `backend/internal/workflows/draft_sync.go`
- Modify: `backend/internal/workflows/workflows_test.go` (functions `TestDraftSyncDispatcher_DrainsUntilShortClaim`, `TestDraftSyncDispatcher_EmptyClaimStopsImmediately`, `TestDraftSyncDispatcher_BatchFailureDoesNotFailRun`)

**Interfaces:**
- Consumes: `activities.SyncBatchResult` (unchanged), `workflows.DraftSyncReport` (Task 1).
- Produces: `DraftSyncDispatcher(ctx workflow.Context) (DraftSyncReport, error)`.

- [ ] **Step 1: Update the three tests in `backend/internal/workflows/workflows_test.go`**

In `TestDraftSyncDispatcher_DrainsUntilShortClaim`, make the end read:
```go
	env.ExecuteWorkflow(workflows.DraftSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report workflows.DraftSyncReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.DraftSyncReport{LeaguesProcessed: 3}, report)
	env.AssertExpectations(t)
}
```

In `TestDraftSyncDispatcher_EmptyClaimStopsImmediately`, make the end read:
```go
	env.ExecuteWorkflow(workflows.DraftSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report workflows.DraftSyncReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.DraftSyncReport{}, report)
	env.AssertExpectations(t)
}
```

In `TestDraftSyncDispatcher_BatchFailureDoesNotFailRun`, make the end read:
```go
	env.ExecuteWorkflow(workflows.DraftSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	// Failed batches are logged; the leagues' claims expire and re-queue.
	require.NoError(t, env.GetWorkflowError())
	var report workflows.DraftSyncReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.DraftSyncReport{}, report)
	env.AssertExpectations(t)
}
```

- [ ] **Step 2: Run tests to verify they fail to compile**

Run: `cd backend && go test ./internal/workflows/... -run TestDraftSyncDispatcher -v`
Expected: FAIL — `env.GetWorkflowResult(&report)` errors, workflow still returns bare `error`.

- [ ] **Step 3: Update `backend/internal/workflows/draft_sync.go`**

Replace the whole `DraftSyncDispatcher` function body:

```go
func DraftSyncDispatcher(ctx workflow.Context) (DraftSyncReport, error) {
	dfa := &activities.DataFetchActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	batchCtx := workflow.WithActivityOptions(ctx, batchActivityOptions)
	logger := workflow.GetLogger(ctx)

	var cfg activities.DraftSyncConfig
	if err := workflow.ExecuteActivity(actCtx, dfa.GetDraftSyncConfig).Get(ctx, &cfg); err != nil {
		return DraftSyncReport{}, err
	}

	var report DraftSyncReport
	for iter := 0; iter < MaxDispatchIterations; iter++ {
		var futures []workflow.Future
		drained := false
		for k := 0; k < cfg.ParallelBatches; k++ {
			var leagueIDs []string
			err := workflow.ExecuteActivity(actCtx, dfa.ClaimLeaguesForDrafts, activities.ClaimLeaguesForDraftsParams{
				BatchSize: cfg.BatchSize,
			}).Get(ctx, &leagueIDs)
			if err != nil {
				logger.Error("draft claim failed; stopping dispatch for this run", "error", err)
				drained = true
				break
			}
			if len(leagueIDs) == 0 {
				drained = true
				break
			}
			futures = append(futures, workflow.ExecuteActivity(batchCtx, dfa.SyncLeagueDraftsBatch, activities.SyncLeagueDraftsBatchParams{
				LeagueIDs:   leagueIDs,
				Concurrency: cfg.Concurrency,
			}))
			if len(leagueIDs) < cfg.BatchSize {
				drained = true
				break
			}
		}
		for _, f := range futures {
			var res activities.SyncBatchResult
			if err := f.Get(ctx, &res); err != nil {
				logger.Error("draft batch failed; claims will expire and re-queue", "error", err)
				continue
			}
			logger.Info("draft batch done", "processed", res.Processed, "failed", res.Failed)
			report.LeaguesProcessed += res.Processed
			report.LeaguesFailed += res.Failed
		}
		if drained {
			break
		}
	}
	return report, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/workflows/... -run TestDraftSyncDispatcher -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/workflows/draft_sync.go backend/internal/workflows/workflows_test.go
git commit -m "DraftSyncDispatcher: return DraftSyncReport"
```

---

## Task 10: TransactionSyncDispatcher returns TransactionSyncReport

**Files:**
- Modify: `backend/internal/workflows/transaction_sync.go`
- Modify: `backend/internal/workflows/workflows_test.go` (functions `TestTransactionSyncDispatcher_DrainsUntilShortClaim`, `TestTransactionSyncDispatcher_EmptyClaimStopsImmediately`, `TestTransactionSyncDispatcher_BatchFailureDoesNotFailRun`)

**Interfaces:**
- Consumes: `activities.SyncBatchResult` (unchanged), `workflows.TransactionSyncReport` (Task 1).
- Produces: `TransactionSyncDispatcher(ctx workflow.Context) (TransactionSyncReport, error)`.

- [ ] **Step 1: Update the three tests in `backend/internal/workflows/workflows_test.go`**

In `TestTransactionSyncDispatcher_DrainsUntilShortClaim`, make the end read:
```go
	env.ExecuteWorkflow(workflows.TransactionSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report workflows.TransactionSyncReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.TransactionSyncReport{LeaguesProcessed: 3}, report)
	env.AssertExpectations(t)
}
```

In `TestTransactionSyncDispatcher_EmptyClaimStopsImmediately`, make the end read:
```go
	env.ExecuteWorkflow(workflows.TransactionSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report workflows.TransactionSyncReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.TransactionSyncReport{}, report)
	env.AssertExpectations(t)
}
```

In `TestTransactionSyncDispatcher_BatchFailureDoesNotFailRun`, make the end read:
```go
	env.ExecuteWorkflow(workflows.TransactionSyncDispatcher)

	require.True(t, env.IsWorkflowCompleted())
	// Failed batches are logged; the leagues' claims expire and re-queue.
	require.NoError(t, env.GetWorkflowError())
	var report workflows.TransactionSyncReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.TransactionSyncReport{}, report)
	env.AssertExpectations(t)
}
```

- [ ] **Step 2: Run tests to verify they fail to compile**

Run: `cd backend && go test ./internal/workflows/... -run TestTransactionSyncDispatcher -v`
Expected: FAIL — `env.GetWorkflowResult(&report)` errors, workflow still returns bare `error`.

- [ ] **Step 3: Update `backend/internal/workflows/transaction_sync.go`**

Replace the whole `TransactionSyncDispatcher` function body:

```go
func TransactionSyncDispatcher(ctx workflow.Context) (TransactionSyncReport, error) {
	dfa := &activities.DataFetchActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	batchCtx := workflow.WithActivityOptions(ctx, batchActivityOptions)
	logger := workflow.GetLogger(ctx)

	var cfg activities.TransactionSyncConfig
	if err := workflow.ExecuteActivity(actCtx, dfa.GetTransactionSyncConfig).Get(ctx, &cfg); err != nil {
		return TransactionSyncReport{}, err
	}

	var report TransactionSyncReport
	for iter := 0; iter < MaxDispatchIterations; iter++ {
		var futures []workflow.Future
		drained := false
		for k := 0; k < cfg.ParallelBatches; k++ {
			var leagues []activities.LeagueTransactionState
			err := workflow.ExecuteActivity(actCtx, dfa.ClaimLeaguesForTransactions, activities.ClaimLeaguesForTransactionsParams{
				BatchSize: cfg.BatchSize,
			}).Get(ctx, &leagues)
			if err != nil {
				logger.Error("claim failed; stopping dispatch for this run", "error", err)
				drained = true
				break
			}
			if len(leagues) == 0 {
				drained = true
				break
			}
			futures = append(futures, workflow.ExecuteActivity(batchCtx, dfa.SyncLeagueTransactionsBatch, activities.SyncLeagueTransactionsBatchParams{
				Leagues:     leagues,
				Concurrency: cfg.Concurrency,
			}))
			if len(leagues) < cfg.BatchSize {
				drained = true
				break
			}
		}
		for _, f := range futures {
			var res activities.SyncBatchResult
			if err := f.Get(ctx, &res); err != nil {
				logger.Error("transaction batch failed; claims will expire and re-queue", "error", err)
				continue
			}
			logger.Info("transaction batch done", "processed", res.Processed, "failed", res.Failed)
			report.LeaguesProcessed += res.Processed
			report.LeaguesFailed += res.Failed
		}
		if drained {
			break
		}
	}
	return report, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/workflows/... -run TestTransactionSyncDispatcher -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/workflows/transaction_sync.go backend/internal/workflows/workflows_test.go
git commit -m "TransactionSyncDispatcher: return TransactionSyncReport"
```

---

## Task 11: ArchiveBackfillWorkflow returns BackfillReport

**Files:**
- Modify: `backend/internal/workflows/backfill.go`
- Modify: `backend/internal/workflows/workflows_test.go` (function `TestArchiveBackfillWorkflow_CompletesWhenAllStreamsDrainWithinOneExecution`)

**Interfaces:**
- Consumes: `workflows.BackfillReport` (Task 1), `drainStream` (unchanged, from `helpers.go`).
- Produces: `ArchiveBackfillWorkflow(ctx workflow.Context) (BackfillReport, error)`.

- [ ] **Step 1: Update `TestArchiveBackfillWorkflow_CompletesWhenAllStreamsDrainWithinOneExecution` in `backend/internal/workflows/workflows_test.go`**

Make the end of the test read:
```go
	env.ExecuteWorkflow(workflows.ArchiveBackfillWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	var report workflows.BackfillReport
	require.NoError(t, env.GetWorkflowResult(&report))
	require.Equal(t, workflows.BackfillReport{
		LeaguesReplicated: 3, TransactionsReplicated: 10, DraftHeadersReplicated: 2, DraftPicksReplicated: 1,
	}, report)
	env.AssertExpectations(t)
}
```

(`TestArchiveBackfillWorkflow_ContinuesAsNewWhenAStreamHitsTheBatchCap` and `TestArchiveBackfillWorkflow_StreamFailureFailsTheExecution` only assert on `env.GetWorkflowError()` / `workflow.IsContinueAsNewError`, not on a decoded result, so they need no changes.)

- [ ] **Step 2: Run test to verify it fails to compile**

Run: `cd backend && go test ./internal/workflows/... -run TestArchiveBackfillWorkflow_CompletesWhenAllStreamsDrainWithinOneExecution -v`
Expected: FAIL — `env.GetWorkflowResult(&report)` errors, workflow still returns bare `error`.

- [ ] **Step 3: Update `backend/internal/workflows/backfill.go`**

Replace the whole `ArchiveBackfillWorkflow` function body:

```go
func ArchiveBackfillWorkflow(ctx workflow.Context) (BackfillReport, error) {
	sa := &activities.ScavengerActivities{}
	actCtx := workflow.WithActivityOptions(ctx, defaultActivityOptions)
	logger := workflow.GetLogger(ctx)

	var cfg activities.ScavengerConfig
	if err := workflow.ExecuteActivity(actCtx, sa.GetScavengerConfig).Get(ctx, &cfg); err != nil {
		return BackfillReport{}, err
	}

	allDrained := true

	leaguesReplicated, leaguesDrained, err := drainStream(ctx, actCtx, sa.ReplicateLeaguesBatch, cfg.LeagueBatchSize, backfillBatchesPerExecution)
	if err != nil {
		return BackfillReport{}, fmt.Errorf("replicate leagues: %w", err)
	}
	allDrained = allDrained && leaguesDrained

	txnReplicated, txnDrained, err := drainStream(ctx, actCtx, sa.ReplicateTransactionsBatch, cfg.TxnBatchSize, backfillBatchesPerExecution)
	if err != nil {
		return BackfillReport{}, fmt.Errorf("replicate transactions: %w", err)
	}
	allDrained = allDrained && txnDrained

	headersReplicated, headersDrained, err := drainStream(ctx, actCtx, sa.ReplicateDraftHeadersBatch, cfg.DraftBatchSize, backfillBatchesPerExecution)
	if err != nil {
		return BackfillReport{}, fmt.Errorf("replicate draft headers: %w", err)
	}
	allDrained = allDrained && headersDrained

	picksReplicated, picksDrained, err := drainStream(ctx, actCtx, sa.ReplicateDraftPicksBatch, cfg.DraftBatchSize, backfillBatchesPerExecution)
	if err != nil {
		return BackfillReport{}, fmt.Errorf("replicate draft picks: %w", err)
	}
	allDrained = allDrained && picksDrained

	report := BackfillReport{
		LeaguesReplicated:      leaguesReplicated,
		TransactionsReplicated: txnReplicated,
		DraftHeadersReplicated: headersReplicated,
		DraftPicksReplicated:   picksReplicated,
	}

	logger.Info("backfill execution complete", "leagues", leaguesReplicated, "transactions", txnReplicated,
		"draftHeaders", headersReplicated, "draftPicks", picksReplicated, "allDrained", allDrained)

	if !allDrained {
		return report, workflow.NewContinueAsNewError(ctx, ArchiveBackfillWorkflow)
	}
	logger.Info("archive backfill complete")
	return report, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/workflows/... -run TestArchiveBackfillWorkflow -v`
Expected: PASS (all three `TestArchiveBackfillWorkflow_*` tests)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/workflows/backfill.go backend/internal/workflows/workflows_test.go
git commit -m "ArchiveBackfillWorkflow: return BackfillReport"
```

---

## Task 12: Full verification pass

**Files:** none (verification only)

**Interfaces:**
- Consumes: everything from Tasks 1–11.
- Produces: a verified-green backend build/vet/format/test pass, ready for PR.

- [ ] **Step 1: Format check**

Run: `cd backend && gofmt -l .`
Expected: no output (empty = no files need formatting). If any files are listed, run `gofmt -w <files>` and re-check.

- [ ] **Step 2: Vet**

Run: `cd backend && go vet ./...`
Expected: no output, exit 0.

- [ ] **Step 3: Build**

Run: `cd backend && go build ./...`
Expected: exit 0, no output.

- [ ] **Step 4: Full test suite**

Run: `cd backend && go test ./... 2>&1 | tail -60`
Expected: every package reports `ok`; `internal/workflows` and `internal/activities` in particular must be all-green. Tests requiring `TEST_DATABASE_URL` will show as skipped, not failed — that's expected in this environment.

- [ ] **Step 5: Confirm no stray changes**

Run: `git status`
Expected: working tree clean (everything already committed in Tasks 1–11), still on branch `worktree-temporal-work-reports`, no changes outside `backend/internal/{activities,workflows}/` and the two spec/plan docs under `docs/superpowers/`.

- [ ] **Step 6: If everything above is green, this plan is complete.** No commit needed for this task (verification only) — hand back to the orchestrator to push the branch and open the PR.
