# Age-Based Write Routing (T13) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement T13 from `docs/superpowers/plans/2026-07-07-two-database-archive.md` — sync activities route already-old transactions and drafts/picks straight to the archive DB at ingest time, skipping cloud entirely, instead of write-to-cloud-then-replicate-then-eventually-purge. This matters specifically for data that's old the moment it's first synced (a newly-discovered league's multi-season history, or catching up after downtime) — today that data takes three round trips (cloud write, scavenger copy, scavenger purge) to end up exactly where it always belonged; after this change it takes one.

**Architecture:** `DataFetchActivities` gains an `Archive *gorm.DB` field (nil when no archive DB is configured — sync must keep working without one, unlike T5/T7's ADP/scavenger workers which fully gate on archive availability). In `syncOneLeague` (transactions), each leg's fetched rows are partitioned by `CreatedAtSleeper` against a cutoff (`now - SCAVENGER_RETENTION_DAYS`, reusing T6's exact knob) — rows past the cutoff go straight to `models.ArchiveSleeperTransaction` inserts; everything else keeps going to cloud exactly as before. In `syncOneLeagueDrafts` (drafts + picks), each draft routes as a whole unit by its `Season` field: `Season < currentSeason` (calendar year, matching `Seasons()`'s existing convention) goes straight to archive (header + picks, once fetched); `Season == currentSeason` keeps using the existing cloud-only code path completely unchanged. When `Archive` is nil, everything falls back to cloud-only — the exact pre-T13 behavior — so this doesn't touch environments without an archive DB (and every existing test, which never sets `Archive`, keeps passing unmodified).

**Tech Stack:** Go, GORM — no new dependencies, no new migrations, no new raw SQL.

## Global Constraints

- **Scope boundary vs. the ongoing scavenger (T5/T6): this only optimizes the ingest-time case.** A draft/transaction that's *current* when first synced still goes to cloud and stays there — it only migrates to archive-only (and eventually gets purged from cloud) via the regular 6h scavenger once it ages out, exactly as today. T13 doesn't change that steady-state path at all; it only short-circuits the case where data is *already* outside the retention window the moment it's first discovered. Don't try to make T13 handle the aging-out case too — that's T5/T6's job, working correctly today.
- `Archive` is nil-safe everywhere: every routing decision starts with `if a.Archive != nil`. No panics, no behavior change, when no archive DB is configured — this is what keeps every existing test in `data_fetch_test.go` (none of which set `Archive`) passing unmodified.
- Reuse `SCAVENGER_RETENTION_DAYS` (T6, `internal/activities/scavenger.go`'s `GetScavengerConfig`) as the age threshold — read directly via `helpers.GetEnv` inside `data_fetch.go`, not by calling `ScavengerActivities.GetScavengerConfig` (that would be an odd cross-activity-struct dependency for one int). Both "too old for cloud to keep" (T6) and "too old to bother writing to cloud in the first place" (T13) are the same threshold, so they share the env var name, not a wrapper.
- "Current season" is the calendar year (`strconv.Itoa(time.Now().Year())`), matching the existing convention in `internal/activities/discovery.go`'s `Seasons()` — not NFL-state week boundaries. Season strings compare correctly as plain strings (`"2024" < "2026"`), same as the existing claim-query predicates (`season >= '2025'`).
- Transactions are partitioned per-row (a single leg's fetch can contain both old and recent transactions). Drafts route as a whole unit — a draft and all its picks go to the same database; there's no partial-draft split.
- All new tests are pure SQLite (two in-memory DBs — one simulating cloud, one archive), matching the existing `data_fetch_test.go` convention exactly. No PG-specific raw SQL is introduced here (unlike the scavenger's keyset-cursor queries), so there's no need for the `TEST_DATABASE_URL`-gated two-schema pattern this time.

---

## File Structure

| File | Responsibility |
|---|---|
| `backend/internal/activities/data_fetch.go` (modify) | `DataFetchActivities.Archive`; `archiveRoutingCutoff`/`currentSleeperSeason` helpers; age-based routing in `syncOneLeague` + `syncOneLeagueDrafts`; new `upsertArchiveTransactions`/`upsertArchiveDraftHeader`/`fetchArchiveDraftPicks` |
| `backend/internal/activities/data_fetch_test.go` (modify) | `newArchiveTestDB` helper; new tests for both routing paths (old→archive, current→cloud, nil-Archive fallback, fetch-once) |
| `backend/cmd/worker/main.go` (modify) | Wire `Archive: database.Archive` into `dfa`'s construction (unconditional — sync must work with or without an archive DB) |

---

### Task 1: `DataFetchActivities.Archive` field + routing helpers + wire `cmd/worker`

**Files:**
- Modify: `backend/internal/activities/data_fetch.go`
- Modify: `backend/cmd/worker/main.go`

**Interfaces:**
- Produces: `DataFetchActivities.Archive *gorm.DB`, `archiveRoutingCutoff() time.Time`, `currentSleeperSeason() string`. Consumed by Tasks 2 and 3.

No test for this task alone — it's pure plumbing with no observable behavior until Tasks 2/3 use it. `go build` is the verification.

- [ ] **Step 1: Add the field and helpers**

In `backend/internal/activities/data_fetch.go`, change:

```go
// DataFetchActivities holds dependencies for per-league data fetching activities.
type DataFetchActivities struct {
	DB      *gorm.DB
	Sleeper *sleeper.Client
}
```

to:

```go
// DataFetchActivities holds dependencies for per-league data fetching activities.
// Archive is nil unless ARCHIVE_DATABASE_URL is configured — every use of it
// is nil-checked, falling back to cloud-only (the pre-T13 behavior) when
// unset. Unlike ScavengerActivities/ADPRollupActivities, this struct's
// worker is never gated on archive availability: sync must keep working
// with or without one.
type DataFetchActivities struct {
	DB      *gorm.DB
	Archive *gorm.DB
	Sleeper *sleeper.Client
}

// archiveRoutingCutoff returns the age boundary for routing already-old data
// straight to archive at ingest time instead of cloud — see syncOneLeague
// and syncOneLeagueDrafts. Reuses SCAVENGER_RETENTION_DAYS (T6): "too old
// for cloud to keep" and "too old to bother writing to cloud in the first
// place" are the same threshold.
func archiveRoutingCutoff() time.Time {
	days := max(helpers.GetEnv("SCAVENGER_RETENTION_DAYS", 30), 1)
	return time.Now().UTC().AddDate(0, 0, -days)
}

// currentSleeperSeason anchors "current" the same way Seasons() does
// (discovery.go) — the calendar year, not NFL-state week boundaries.
func currentSleeperSeason() string {
	return strconv.Itoa(time.Now().Year())
}
```

Add `"strconv"` to the import block.

- [ ] **Step 2: Wire it into `cmd/worker/main.go`**

Change:

```go
	dfa := &activities.DataFetchActivities{DB: database.DB, Sleeper: sc}
```

to:

```go
	dfa := &activities.DataFetchActivities{DB: database.DB, Archive: database.Archive, Sleeper: sc}
```

`database.Archive` is nil when `ARCHIVE_DATABASE_URL` isn't set — no gating needed here, this assignment is safe unconditionally (see Global Constraints).

- [ ] **Step 3: Build**

Run: `cd backend && go build ./...`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
git add internal/activities/data_fetch.go cmd/worker/main.go
git commit -m "feat: add DataFetchActivities.Archive + age-based routing helpers (unused yet)"
```

---

### Task 2: Age-based routing for transactions

**Files:**
- Modify: `backend/internal/activities/data_fetch.go`
- Modify: `backend/internal/activities/data_fetch_test.go`

**Interfaces:**
- Produces: `(a *DataFetchActivities) upsertArchiveTransactions(ctx, rows []models.SleeperTransaction) error`.

- [ ] **Step 1: Add the SQLite archive test-DB helper**

Add to `backend/internal/activities/data_fetch_test.go` (near the top, after imports):

```go
// newArchiveTestDB opens an in-memory SQLite DB migrated with the archive
// models — a lightweight stand-in for the archive DB in tests that need to
// prove routing actually lands rows in a *different* database than cloud.
// No PG-specific SQL is involved in this routing logic, so SQLite suffices
// here (unlike the scavenger's keyset-cursor queries).
func newArchiveTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("unwrap sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&models.ArchiveSleeperLeague{}, &models.ArchiveSleeperTransaction{},
		&models.ArchiveSleeperDraft{}, &models.ArchiveSleeperDraftPick{},
	); err != nil {
		t.Fatalf("automigrate archive: %v", err)
	}
	return db
}
```

Add `"gorm.io/driver/sqlite"` and `"gorm.io/gorm/logger"` to the import block.

- [ ] **Step 2: Write the failing tests**

Append to `data_fetch_test.go`:

```go
func TestSyncBatch_RoutesOldTransactionsToArchiveOnly(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)
	claimedLeague(t, cloud, "lg1")

	now := time.Now().UTC()
	recentMs := now.Add(-1 * time.Hour).UnixMilli()
	oldMs := now.AddDate(0, 0, -60).UnixMilli() // 60 days ago, past the 30-day default retention

	srv := batchTestServer(t, 3, map[string][]sleeper.Transaction{
		"lg1/2": {
			{TransactionID: "tx-recent", Type: "waiver", Status: "complete", Leg: 2, Created: recentMs},
			{TransactionID: "tx-old", Type: "waiver", Status: "complete", Leg: 2, Created: oldMs},
		},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runBatch(t, dfa, activities.SyncLeagueTransactionsBatchParams{
		Leagues:     []activities.LeagueTransactionState{{LeagueID: "lg1", Season: "2026"}},
		Concurrency: 1,
	})
	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("expected 1 processed / 0 failed, got %+v", res)
	}

	var cloudIDs, archiveIDs []string
	cloud.Model(&models.SleeperTransaction{}).Pluck("sleeper_transaction_id", &cloudIDs)
	archive.Model(&models.ArchiveSleeperTransaction{}).Pluck("sleeper_transaction_id", &archiveIDs)
	if len(cloudIDs) != 1 || cloudIDs[0] != "tx-recent" {
		t.Errorf("expected only tx-recent in cloud, got %v", cloudIDs)
	}
	if len(archiveIDs) != 1 || archiveIDs[0] != "tx-old" {
		t.Errorf("expected only tx-old in archive, got %v", archiveIDs)
	}
}

func TestSyncBatch_AllTransactionsToCloudWhenArchiveNil(t *testing.T) {
	cloud := newTestDB(t)
	claimedLeague(t, cloud, "lg1")

	oldMs := time.Now().UTC().AddDate(0, 0, -60).UnixMilli()
	srv := batchTestServer(t, 3, map[string][]sleeper.Transaction{
		"lg1/2": {{TransactionID: "tx-old", Type: "waiver", Status: "complete", Leg: 2, Created: oldMs}},
	}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Sleeper: sleeper.NewWithBaseURL(srv.URL)} // Archive nil
	runBatch(t, dfa, activities.SyncLeagueTransactionsBatchParams{
		Leagues:     []activities.LeagueTransactionState{{LeagueID: "lg1", Season: "2026"}},
		Concurrency: 1,
	})

	var count int64
	cloud.Model(&models.SleeperTransaction{}).Count(&count)
	if count != 1 {
		t.Errorf("expected the old txn to fall back to cloud when Archive is nil, got %d rows", count)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

This compiles immediately (`Archive` and `models.ArchiveSleeperTransaction` already exist from Task 1 and T5) — there's no compile-error red step here; the test fails on its assertion instead, since the routing logic itself doesn't exist yet:

Run: `cd backend && go test ./internal/activities/... -run TestSyncBatch_RoutesOldTransactionsToArchiveOnly -v`
Expected: FAIL — both transactions land in cloud (routing not implemented yet), so `cloudIDs` has 2 entries and `archiveIDs` has 0.

- [ ] **Step 4: Implement**

In `syncOneLeague` (`data_fetch.go`), replace:

```go
		if err := a.DB.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(rows, 500).Error; err != nil {
			return fmt.Errorf("leg %d upsert: %w", leg, err)
		}
		if leg > maxSeen {
			maxSeen = leg
		}
```

with:

```go
		cloudRows := rows
		if a.Archive != nil {
			cutoff := archiveRoutingCutoff()
			var newRows, oldRows []models.SleeperTransaction
			for _, r := range rows {
				if time.UnixMilli(r.CreatedAtSleeper).UTC().Before(cutoff) {
					oldRows = append(oldRows, r)
				} else {
					newRows = append(newRows, r)
				}
			}
			if len(oldRows) > 0 {
				if err := a.upsertArchiveTransactions(ctx, oldRows); err != nil {
					return fmt.Errorf("leg %d archive upsert: %w", leg, err)
				}
			}
			cloudRows = newRows
		}
		if len(cloudRows) > 0 {
			if err := a.DB.WithContext(ctx).
				Clauses(clause.OnConflict{DoNothing: true}).
				CreateInBatches(cloudRows, 500).Error; err != nil {
				return fmt.Errorf("leg %d upsert: %w", leg, err)
			}
		}
		if leg > maxSeen {
			maxSeen = leg
		}
```

Add, after `syncOneLeague`:

```go
// upsertArchiveTransactions writes rows directly to the archive DB, skipping
// cloud — see syncOneLeague's age-based routing.
func (a *DataFetchActivities) upsertArchiveTransactions(ctx context.Context, rows []models.SleeperTransaction) error {
	archiveRows := make([]models.ArchiveSleeperTransaction, len(rows))
	for i, r := range rows {
		archiveRows[i] = models.ArchiveSleeperTransaction{
			SleeperTransactionID: r.SleeperTransactionID, SleeperLeagueID: r.SleeperLeagueID,
			Type: r.Type, Status: r.Status, CreatedAtSleeper: r.CreatedAtSleeper, Leg: r.Leg,
			Adds: r.Adds, Drops: r.Drops, DraftPicks: r.DraftPicks, WaiverBudget: r.WaiverBudget,
			CreatedAt: time.Now().UTC(),
		}
	}
	return a.Archive.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(archiveRows, 500).Error
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd backend && go test ./internal/activities/... -run "TestSyncBatch_RoutesOldTransactionsToArchiveOnly|TestSyncBatch_AllTransactionsToCloudWhenArchiveNil" -v`
Expected: both PASS.

- [ ] **Step 6: Run the full existing transaction-sync test suite for regressions**

Run: `cd backend && go test ./internal/activities/... -run TestSyncBatch -v`
Expected: every `TestSyncBatch_*` test PASSes unchanged — none of them set `Archive`, so they all exercise the nil-fallback path and behave exactly as before.

- [ ] **Step 7: Commit**

```bash
git add internal/activities/data_fetch.go internal/activities/data_fetch_test.go
git commit -m "feat: route already-old transactions straight to archive at ingest time"
```

---

### Task 3: Age-based routing for drafts + picks

**Files:**
- Modify: `backend/internal/activities/data_fetch.go`
- Modify: `backend/internal/activities/data_fetch_test.go`

**Interfaces:**
- Produces: `(a *DataFetchActivities) upsertArchiveDraftHeader(ctx, d sleeper.Draft, leagueID string) error`, `(a *DataFetchActivities) fetchArchiveDraftPicks(ctx, draftID string) error`.

- [ ] **Step 1: Write the failing tests**

Append to `data_fetch_test.go`:

```go
func TestSyncDraftsBatch_RoutesOldDraftToArchiveOnly(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)
	draftClaimedLeague(t, cloud, "lg1")

	srv := draftsTestServer(t,
		map[string][]sleeper.Draft{
			"lg1": {{DraftID: "d-old", Status: "complete", Type: "snake", Season: "2024"}},
		},
		map[string][]sleeper.DraftPick{
			"d-old": {{Round: 1, PickNo: 1, RosterID: 1, PlayerID: "p1"}},
		}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runDraftsBatch(t, dfa, activities.SyncLeagueDraftsBatchParams{LeagueIDs: []string{"lg1"}, Concurrency: 1})
	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("expected 1 processed / 0 failed, got %+v", res)
	}

	var cloudCount int64
	cloud.Model(&models.SleeperDraft{}).Count(&cloudCount)
	if cloudCount != 0 {
		t.Errorf("expected no draft rows in cloud, got %d", cloudCount)
	}
	var archiveDraft models.ArchiveSleeperDraft
	if err := archive.Where("sleeper_draft_id = ?", "d-old").First(&archiveDraft).Error; err != nil {
		t.Fatalf("expected d-old in archive: %v", err)
	}
	if archiveDraft.LastFetchedAt == nil {
		t.Error("expected archive draft's picks to be fetched (last_fetched_at set)")
	}
	var pickCount int64
	archive.Model(&models.ArchiveSleeperDraftPick{}).Where("sleeper_draft_id = ?", "d-old").Count(&pickCount)
	if pickCount != 1 {
		t.Errorf("expected 1 archived pick, got %d", pickCount)
	}
}

func TestSyncDraftsBatch_CurrentSeasonDraftStaysInCloud(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)
	draftClaimedLeague(t, cloud, "lg1")

	srv := draftsTestServer(t,
		map[string][]sleeper.Draft{"lg1": {{DraftID: "d-current", Status: "complete", Type: "snake", Season: "2026"}}},
		map[string][]sleeper.DraftPick{"d-current": {{Round: 1, PickNo: 1, RosterID: 1, PlayerID: "p1"}}}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	runDraftsBatch(t, dfa, activities.SyncLeagueDraftsBatchParams{LeagueIDs: []string{"lg1"}, Concurrency: 1})

	var cloudCount, archiveCount int64
	cloud.Model(&models.SleeperDraft{}).Where("sleeper_draft_id = ?", "d-current").Count(&cloudCount)
	archive.Model(&models.ArchiveSleeperDraft{}).Where("sleeper_draft_id = ?", "d-current").Count(&archiveCount)
	if cloudCount != 1 {
		t.Errorf("expected current-season draft in cloud, got %d", cloudCount)
	}
	if archiveCount != 0 {
		t.Errorf("expected current-season draft NOT in archive, got %d", archiveCount)
	}
}

func TestSyncDraftsBatch_AllDraftsToCloudWhenArchiveNil(t *testing.T) {
	cloud := newTestDB(t)
	draftClaimedLeague(t, cloud, "lg1")

	srv := draftsTestServer(t,
		map[string][]sleeper.Draft{"lg1": {{DraftID: "d-old", Status: "complete", Type: "snake", Season: "2024"}}},
		map[string][]sleeper.DraftPick{"d-old": {{Round: 1, PickNo: 1, RosterID: 1, PlayerID: "p1"}}}, nil)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Sleeper: sleeper.NewWithBaseURL(srv.URL)} // Archive nil
	runDraftsBatch(t, dfa, activities.SyncLeagueDraftsBatchParams{LeagueIDs: []string{"lg1"}, Concurrency: 1})

	var count int64
	cloud.Model(&models.SleeperDraft{}).Where("sleeper_draft_id = ?", "d-old").Count(&count)
	if count != 1 {
		t.Errorf("expected old draft to fall back to cloud when Archive is nil, got %d", count)
	}
}

func TestSyncDraftsBatch_OldDraftPicksFetchOnce(t *testing.T) {
	cloud := newTestDB(t)
	archive := newArchiveTestDB(t)
	draftClaimedLeague(t, cloud, "lg1")
	// Old draft already fetched by an earlier sweep — into archive, not cloud.
	fetched := time.Now().UTC()
	archive.Create(&models.ArchiveSleeperDraft{
		SleeperDraftID: "d-old", SleeperLeagueID: "lg1", Status: "complete", Season: "2024", LastFetchedAt: &fetched,
	})

	var calls atomic.Int64
	srv := draftsTestServer(t,
		map[string][]sleeper.Draft{"lg1": {{DraftID: "d-old", Status: "complete", Type: "snake", Season: "2024"}}},
		map[string][]sleeper.DraftPick{"d-old": {{Round: 1, PickNo: 1, RosterID: 1, PlayerID: "p1"}}}, &calls)
	defer srv.Close()

	dfa := &activities.DataFetchActivities{DB: cloud, Archive: archive, Sleeper: sleeper.NewWithBaseURL(srv.URL)}
	res := runDraftsBatch(t, dfa, activities.SyncLeagueDraftsBatchParams{LeagueIDs: []string{"lg1"}, Concurrency: 1})
	if res.Processed != 1 {
		t.Fatalf("expected 1 processed, got %+v", res)
	}
	// Only the /drafts call — no /picks call for the already-fetched archived draft.
	if got := calls.Load(); got != 1 {
		t.Errorf("expected 1 HTTP call (drafts only), got %d", got)
	}
	var pickCount int64
	archive.Model(&models.ArchiveSleeperDraftPick{}).Count(&pickCount)
	if pickCount != 0 {
		t.Errorf("expected no picks refetched, got %d", pickCount)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/activities/... -run TestSyncDraftsBatch_RoutesOldDraftToArchiveOnly -v`
Expected: FAIL — old draft currently still lands in cloud (routing not implemented yet).

- [ ] **Step 3: Implement**

Replace `syncOneLeagueDrafts` in `data_fetch.go` entirely with:

```go
// syncOneLeagueDrafts upserts a league's drafts, fetches picks for completed
// drafts that haven't been picked up yet (completed drafts are immutable, so
// picks are fetch-once), and stamps completion (clearing the claim) in one
// update. A 404 on the league marks it skipped; a 404 on a draft's picks
// skips that draft. Each draft routes as a whole unit (header + picks) to
// either cloud (current season, or no archive configured — the unchanged
// pre-T13 path) or straight to archive (past seasons, when Archive is set).
func (a *DataFetchActivities) syncOneLeagueDrafts(ctx context.Context, leagueID string) error {
	drafts, err := a.Sleeper.GetLeagueDrafts(ctx, leagueID)
	if err != nil {
		var nfe *sleeper.NotFoundError
		if errors.As(err, &nfe) {
			return a.DB.WithContext(ctx).
				Model(&models.SleeperLeague{}).
				Where("sleeper_league_id = ?", leagueID).
				Updates(map[string]interface{}{
					"skipped_at":        time.Now().UTC(),
					"drafts_claimed_at": nil,
				}).Error
		}
		return err
	}

	currentSeason := currentSleeperSeason()
	var cloudCompletedIDs, archiveCompletedIDs []string
	for _, d := range drafts {
		if a.Archive != nil && d.Season < currentSeason {
			if err := a.upsertArchiveDraftHeader(ctx, d, leagueID); err != nil {
				return err
			}
			if d.Status == "complete" {
				archiveCompletedIDs = append(archiveCompletedIDs, d.DraftID)
			}
			continue
		}
		row := models.SleeperDraft{
			SleeperDraftID:  d.DraftID,
			SleeperLeagueID: leagueID,
			Type:            d.Type,
			Status:          d.Status,
			Season:          d.Season,
		}
		if err := a.DB.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "sleeper_draft_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"status", "type", "season"}),
		}).Create(&row).Error; err != nil {
			return err
		}
		if d.Status == "complete" {
			cloudCompletedIDs = append(cloudCompletedIDs, d.DraftID)
		}
	}

	if len(cloudCompletedIDs) > 0 {
		var pending []string
		if err := a.DB.WithContext(ctx).Model(&models.SleeperDraft{}).
			Where("sleeper_draft_id IN ? AND last_fetched_at IS NULL", cloudCompletedIDs).
			Pluck("sleeper_draft_id", &pending).Error; err != nil {
			return err
		}
		for _, draftID := range pending {
			if err := a.fetchDraftPicks(ctx, draftID); err != nil {
				var nfe *sleeper.NotFoundError
				if errors.As(err, &nfe) {
					continue // draft gone on Sleeper's side; nothing to fetch
				}
				return fmt.Errorf("draft %s: %w", draftID, err)
			}
		}
	}

	if len(archiveCompletedIDs) > 0 {
		var pending []string
		if err := a.Archive.WithContext(ctx).Model(&models.ArchiveSleeperDraft{}).
			Where("sleeper_draft_id IN ? AND last_fetched_at IS NULL", archiveCompletedIDs).
			Pluck("sleeper_draft_id", &pending).Error; err != nil {
			return err
		}
		for _, draftID := range pending {
			if err := a.fetchArchiveDraftPicks(ctx, draftID); err != nil {
				var nfe *sleeper.NotFoundError
				if errors.As(err, &nfe) {
					continue
				}
				return fmt.Errorf("draft %s (archive): %w", draftID, err)
			}
		}
	}

	return a.DB.WithContext(ctx).
		Model(&models.SleeperLeague{}).
		Where("sleeper_league_id = ?", leagueID).
		Updates(map[string]interface{}{
			"last_drafts_fetched_at": time.Now().UTC(),
			"drafts_claimed_at":      nil,
		}).Error
}
```

Add, right after `fetchDraftPicks`:

```go
// upsertArchiveDraftHeader upserts d directly into the archive DB, skipping
// cloud — see syncOneLeagueDrafts's age-based routing.
func (a *DataFetchActivities) upsertArchiveDraftHeader(ctx context.Context, d sleeper.Draft, leagueID string) error {
	now := time.Now().UTC()
	row := models.ArchiveSleeperDraft{
		SleeperDraftID:  d.DraftID,
		SleeperLeagueID: leagueID,
		Type:            d.Type,
		Status:          d.Status,
		Season:          d.Season,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	return a.Archive.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "sleeper_draft_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "type", "season", "updated_at"}),
	}).Create(&row).Error
}

// fetchArchiveDraftPicks mirrors fetchDraftPicks but writes directly to the
// archive DB for an old (archive-routed) draft — see syncOneLeagueDrafts.
func (a *DataFetchActivities) fetchArchiveDraftPicks(ctx context.Context, draftID string) error {
	picks, err := a.Sleeper.GetDraftPicks(ctx, draftID)
	if err != nil {
		return err
	}
	if len(picks) > 0 {
		rows := make([]models.ArchiveSleeperDraftPick, len(picks))
		for i, p := range picks {
			metadata, _ := json.Marshal(p.Metadata)
			rows[i] = models.ArchiveSleeperDraftPick{
				SleeperDraftID:  draftID,
				Round:           p.Round,
				PickNo:          p.PickNo,
				RosterID:        p.RosterID,
				PickedByUserID:  p.PickedBy,
				SleeperPlayerID: p.PlayerID,
				Metadata:        metadata,
			}
		}
		if err := a.Archive.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(rows, 500).Error; err != nil {
			return err
		}
	}
	return a.Archive.WithContext(ctx).
		Model(&models.ArchiveSleeperDraft{}).
		Where("sleeper_draft_id = ?", draftID).
		Update("last_fetched_at", time.Now().UTC()).Error
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/activities/... -run "TestSyncDraftsBatch_RoutesOldDraftToArchiveOnly|TestSyncDraftsBatch_CurrentSeasonDraftStaysInCloud|TestSyncDraftsBatch_AllDraftsToCloudWhenArchiveNil|TestSyncDraftsBatch_OldDraftPicksFetchOnce" -v`
Expected: all 4 PASS.

- [ ] **Step 5: Run the full existing draft-sync test suite for regressions**

Run: `cd backend && go test ./internal/activities/... -run TestSyncDraftsBatch -v`
Expected: every pre-existing `TestSyncDraftsBatch_*` test PASSes unchanged.

- [ ] **Step 6: Commit**

```bash
git add internal/activities/data_fetch.go internal/activities/data_fetch_test.go
git commit -m "feat: route already-old drafts and picks straight to archive at ingest time"
```

---

### Task 4: Full verification

**Files:** none — verification only.

- [ ] **Step 1: Full build, vet, and test suite**

Run: `cd backend && go build ./... && go vet ./...`
Expected: clean.

Run: `cd backend && go test ./... -v 2>&1 | tail -100` (with `TEST_DATABASE_URL` set for the full suite)
Expected: every test PASSes, including all Task 2/3 additions and every pre-existing test unchanged.

- [ ] **Step 2: Manual worker-boot smoke test**

This task touches the hottest write path in the codebase (every league's transaction/draft sync), so beyond the thorough unit tests, do one real boot check confirming the wiring doesn't break startup — reuse the two-throwaway-database pattern from prior plans (T3/T5/T7/T8's verification steps):

```bash
# Disposable Postgres already running per prior plans' convention (initdb/pg_ctl on :5499).
psql "postgres://$(whoami)@localhost:5499/postgres?sslmode=disable" -c "DROP DATABASE IF EXISTS ffsims_cloud" -c "DROP DATABASE IF EXISTS ffsims_archive"
psql "postgres://$(whoami)@localhost:5499/postgres?sslmode=disable" -c "CREATE DATABASE ffsims_cloud" -c "CREATE DATABASE ffsims_archive"

cd backend
go build -o bin/migrate ./cmd/migrate
DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_cloud?sslmode=disable" ./bin/migrate up

go build -o bin/backend-worker ./cmd/worker
DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_cloud?sslmode=disable" \
  ARCHIVE_DATABASE_URL="postgres://$(whoami)@localhost:5499/ffsims_archive?sslmode=disable" \
  timeout 5 ./bin/backend-worker 2>&1 | grep -i "archive\|panic"

rm -f bin/migrate bin/backend-worker
psql "postgres://$(whoami)@localhost:5499/postgres?sslmode=disable" -c "DROP DATABASE IF EXISTS ffsims_cloud" -c "DROP DATABASE IF EXISTS ffsims_archive"
```
Expected: `Connected to archive database (...)`, no panic, no error before the harmless Temporal-dial-failure tail (no local Temporal server needed for this check — Task 1's `dfa` construction happens before the dial, so this alone confirms the wiring is sound).

- [ ] **Step 3: Update the master plan's status table**

In `docs/superpowers/plans/2026-07-07-two-database-archive.md`, change T13's row status from "Not started" to "Done — PR #<N>" once the PR is up (fill in the actual number).

---

## Verification

- [ ] `cd backend && go build ./...` and `go vet ./...` clean.
- [ ] `cd backend && go test ./...` — full suite passes, `TEST_DATABASE_URL` set or unset (this task's own tests need neither, but the suite as a whole still has PG-gated tests elsewhere).
- [ ] Every pre-existing `TestSyncBatch_*`/`TestSyncDraftsBatch_*` test passes unchanged — proof the nil-`Archive` fallback preserves exact pre-T13 behavior.
- [ ] Task 4 Step 2's manual boot check: `Archive` wiring doesn't break worker startup.

## Self-Review

**Spec coverage:** T13's stated mechanism — route old transactions/drafts/picks straight to archive at ingest, skip cloud, configurable age threshold — is Tasks 1–3. The master doc's open question ("share `SCAVENGER_RETENTION_DAYS` or give it its own knob?") is resolved in Global Constraints: shared, since both concepts are the same threshold by definition.

**Placeholder scan:** no TBD/TODO markers; every step has literal code.

**Type consistency:** `DataFetchActivities.Archive *gorm.DB` matches between Task 1's definition and Tasks 2/3's usage. `upsertArchiveTransactions(ctx, []models.SleeperTransaction) error`, `upsertArchiveDraftHeader(ctx, sleeper.Draft, string) error`, and `fetchArchiveDraftPicks(ctx, string) error` match between their definitions and call sites within the same task. `archiveRoutingCutoff() time.Time` and `currentSleeperSeason() string` (Task 1) match their usage in Tasks 2 and 3 respectively.
