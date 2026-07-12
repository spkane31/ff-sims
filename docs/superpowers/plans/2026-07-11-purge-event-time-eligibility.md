# Purge Event-Time Eligibility (T14) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix a real defect discovered while operating T9 in production — T6's purge (`PurgeTransactionsBatch`/`PurgeDraftsBatch`) currently decides eligibility by `created_at` (when *our* system inserted the row), not by when the underlying Sleeper event actually happened. In production, ~40 million transactions were all inserted within the last 30 days (ongoing league discovery/backfill), so **zero** of them are purge-eligible under the current rule despite most being years old by Sleeper's own timestamp. Purge should filter transactions by `created_at_sleeper` and drafts by `season`, not insert time.

**Architecture:** `PurgeTransactionsBatch`'s candidate query switches its `WHERE`/`ORDER BY` from `created_at` to `created_at_sleeper` (epoch-ms int64 → compare against a computed cutoff in the same units). `PurgeDraftsBatch`'s candidate query switches from `created_at` to `season` (string year), reusing `currentSleeperSeason()` — already defined in `data_fetch.go`, same package — rather than inventing a second definition of "current." Both activities keep returning `created_at` (insert time) as `purgeCandidate.CreatedAt` for the stalled-replication alarm specifically — that check is about "how long has the system known about this row without successfully purging it," which is correctly anchored to insert time regardless of what decides *eligibility*. Replicate (T5) is untouched: its cursor genuinely does need insert-time's monotonic-arrival guarantee, and nothing here changes that. A new migration adds the index this now requires (`sleeper_transactions.created_at_sleeper` and `sleeper_drafts.season` — the only existing `created_at_sleeper` index is a partial one scoped to `type='trade' AND status='complete'`, useless for a general purge scan across 40M+ rows).

**Tech Stack:** Go, GORM, goose migration (`CONCURRENTLY`, since these are large already-populated production tables).

## Global Constraints

- **This only touches purge (T6), not replicate (T5).** Replicate's cursor must keep using insert-time (`created_at`/`updated_at`/`last_fetched_at`) for its keyset-pagination correctness — that's a different requirement than purge's, and conflating them was the actual mistake in the original design (see chat context: insert-time was defended as necessary for both, but purge doesn't maintain a resumable cursor the way replicate does — it re-queries "current oldest candidates" fresh every batch, so deleted rows just naturally stop appearing in future scans).
- **The alarm-staleness field (`purgeCandidate.CreatedAt`, used by `checkUnverifiedAlarm`) stays insert-time**, even though the eligibility `WHERE` clause changes. The alarm answers "has replication stalled" (anchored to when the system became aware of the row), not "how old is the underlying event" — these are different questions and should stay on different clocks.
- For drafts, `RetentionDays` no longer drives *eligibility* (there's no day-granularity concept for a year-bucketed `season` string) — it's still passed through to `checkUnverifiedAlarm` for the staleness threshold. Document this explicitly in the updated doc comment so it's not mistaken for an oversight.
- Reuse `currentSleeperSeason()` from `data_fetch.go` (same package, `internal/activities`) — don't redefine "current season" a second time.
- Test fixtures that previously encoded "old enough to purge" via `CreatedAt: <40 days ago>` need updating to encode it via `CreatedAtSleeper` (transactions) or `Season` (drafts) instead — and computed dynamically (`time.Now().Year()`-based), not hardcoded to a specific year, so they don't silently go stale the same way this bug did.
- New index migration must be numbered `023` (`022` is the last existing cloud migration) and use `CREATE INDEX CONCURRENTLY` / repeated `-- +goose NO TRANSACTION` under both `Up` and `Down`, matching `021`/`022`'s established convention exactly.

---

## File Structure

| File | Responsibility |
|---|---|
| `backend/internal/activities/scavenger.go` (modify) | `PurgeTransactionsBatch` and `PurgeDraftsBatch` candidate queries switch to event-time criteria |
| `backend/internal/activities/scavenger_test.go` (modify) | Update existing purge tests' fixtures to event-time framing; add 2 new tests proving the fix directly |
| `backend/migrations/023_purge_event_time_indexes.sql` (new) | `idx_sleeper_transactions_created_at_sleeper`, `idx_sleeper_drafts_season` |
| `backend/internal/dbmigrate/dbmigrate_test.go` (modify) | Add the two new indexes to the existing migration-index assertion list |
| `docs/archive-purge.md` (modify) | Correct the monitoring SQL and preconditions prose to reflect event-time eligibility |
| `docs/superpowers/plans/2026-07-07-two-database-archive.md` (modify) | Correct Risk #5 (it documented the bug as an accepted tradeoff — it wasn't one), update the Scavenger design bullet, add T14's row |

---

### Task 1: Fix `PurgeTransactionsBatch` — event-time eligibility

**Files:**
- Modify: `backend/internal/activities/scavenger.go`
- Modify: `backend/internal/activities/scavenger_test.go`

**Interfaces:** no signature changes — `PurgeTransactionsBatch(ctx, PurgeBatchParams) (PurgeBatchResult, error)` stays identical; only its internal candidate-selection query changes.

- [x] **Step 1: Update the existing tests' fixtures to event-time framing**

In `scavenger_test.go`, the 5 existing `TestPurgeTransactionsBatch_*` tests currently encode "old enough to purge" via `CreatedAt: old` (insert time) and never set `CreatedAtSleeper` at all (leaving it at its zero value, epoch 0 — which would trivially satisfy *any* event-time cutoff, masking whether the fix actually works). Replace all 5 test bodies with:

```go
func TestPurgeTransactionsBatch_DeletesVerifiedOldRows(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	old := time.Now().UTC().AddDate(0, 0, -400) // event happened well over a year ago
	recentInsert := time.Now().UTC()            // but only just inserted — the exact scenario this fixes
	for i, id := range []string{"t1", "t2"} {
		if err := cloud.Create(&models.SleeperTransaction{
			SleeperTransactionID: id, SleeperLeagueID: "lg1",
			CreatedAtSleeper: old.Add(time.Duration(i) * time.Second).UnixMilli(),
			CreatedAt:        recentInsert,
		}).Error; err != nil {
			t.Fatalf("seed cloud txn %s: %v", id, err)
		}
		if err := archive.Create(&models.ArchiveSleeperTransaction{
			SleeperTransactionID: id, SleeperLeagueID: "lg1",
			CreatedAtSleeper: old.Add(time.Duration(i) * time.Second).UnixMilli(),
			CreatedAt:        recentInsert,
		}).Error; err != nil {
			t.Fatalf("seed archive txn %s: %v", id, err)
		}
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeTransactionsBatch: %v", err)
	}
	if res.Purged != 2 || res.Unverified != 0 || !res.Drained {
		t.Errorf("res = %+v, want {Purged: 2, Unverified: 0, Drained: true}", res)
	}
	var count int64
	cloud.Model(&models.SleeperTransaction{}).Count(&count)
	if count != 0 {
		t.Errorf("expected cloud transactions purged, got %d remaining", count)
	}
}

func TestPurgeTransactionsBatch_SkipsUnverifiedRows(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	old := time.Now().UTC().AddDate(0, 0, -40)
	if err := cloud.Create(&models.SleeperTransaction{
		SleeperTransactionID: "t1", CreatedAtSleeper: old.UnixMilli(), CreatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Not replicated to archive.

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeTransactionsBatch: %v", err)
	}
	if res.Purged != 0 || res.Unverified != 1 {
		t.Errorf("res = %+v, want {Purged: 0, Unverified: 1}", res)
	}
	var count int64
	cloud.Model(&models.SleeperTransaction{}).Count(&count)
	if count != 1 {
		t.Errorf("expected the unverified row to remain in cloud, got %d rows", count)
	}
}

func TestPurgeTransactionsBatch_IgnoresRowsWithinRetention(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	recent := time.Now().UTC().AddDate(0, 0, -5) // event happened within the 30-day retention window
	if err := cloud.Create(&models.SleeperTransaction{
		SleeperTransactionID: "t1", CreatedAtSleeper: recent.UnixMilli(), CreatedAt: recent,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeTransactionsBatch: %v", err)
	}
	if res.Purged != 0 || res.Unverified != 0 || !res.Drained {
		t.Errorf("res = %+v, want no candidates found (event is within retention)", res)
	}
}

func TestPurgeTransactionsBatch_ErrorsWhenOldestUnverifiedPastAlarmThreshold(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	waaayOld := time.Now().UTC().AddDate(0, 0, -46) // 30d retention + 15d alarm + 1, by INSERT time — the alarm clock
	if err := cloud.Create(&models.SleeperTransaction{
		SleeperTransactionID: "t1", CreatedAtSleeper: waaayOld.UnixMilli(), CreatedAt: waaayOld,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Not replicated to archive — stalled.

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	_, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err == nil {
		t.Fatal("expected an error once the oldest unverified row exceeds retention+15d (by insert time), got nil")
	}
}

func TestPurgeTransactionsBatch_DrainedWhenFewerThanBatchSize(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	old := time.Now().UTC().AddDate(0, 0, -400)
	for i, id := range []string{"t1", "t2", "t3"} {
		ts := old.Add(time.Duration(i) * time.Second)
		if err := cloud.Create(&models.SleeperTransaction{
			SleeperTransactionID: id, CreatedAtSleeper: ts.UnixMilli(), CreatedAt: time.Now().UTC(),
		}).Error; err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
		if err := archive.Create(&models.ArchiveSleeperTransaction{
			SleeperTransactionID: id, CreatedAtSleeper: ts.UnixMilli(), CreatedAt: time.Now().UTC(),
		}).Error; err != nil {
			t.Fatalf("seed archive %s: %v", id, err)
		}
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 2, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeTransactionsBatch: %v", err)
	}
	if res.Purged != 2 || res.Drained {
		t.Errorf("expected a full, non-drained batch of 2, got %+v", res)
	}
}
```

Also add a new test directly encoding the bug this fixes:

```go
func TestPurgeTransactionsBatch_EligibleByEventTimeDespiteRecentInsertTime(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	eventTime := time.Now().UTC().AddDate(-1, 0, 0) // a year-old Sleeper transaction
	insertTime := time.Now().UTC()                  // freshly inserted today (e.g. a new league's backfill)
	if err := cloud.Create(&models.SleeperTransaction{
		SleeperTransactionID: "t1", CreatedAtSleeper: eventTime.UnixMilli(), CreatedAt: insertTime,
	}).Error; err != nil {
		t.Fatalf("seed cloud: %v", err)
	}
	if err := archive.Create(&models.ArchiveSleeperTransaction{
		SleeperTransactionID: "t1", CreatedAtSleeper: eventTime.UnixMilli(), CreatedAt: insertTime,
	}).Error; err != nil {
		t.Fatalf("seed archive: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeTransactionsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeTransactionsBatch: %v", err)
	}
	if res.Purged != 1 {
		t.Errorf("expected the row to be purge-eligible despite being inserted today, because the underlying event is a year old; got %+v", res)
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run (with a disposable Postgres — see prior plans' Task setup): `cd backend && go test ./internal/activities/... -run "TestPurgeTransactionsBatch_EligibleByEventTimeDespiteRecentInsertTime|TestPurgeTransactionsBatch_DeletesVerifiedOldRows" -v`
Expected: `TestPurgeTransactionsBatch_EligibleByEventTimeDespiteRecentInsertTime` FAILs (`res.Purged` is `0`, not `1` — the current `created_at`-based query sees `CreatedAt: insertTime` = today, well within retention, so it's never even a candidate). `TestPurgeTransactionsBatch_DeletesVerifiedOldRows` also FAILs for the same reason (both rows have recent `CreatedAt`).

- [x] **Step 3: Implement**

In `scavenger.go`, replace `selectPurgeTransactionCandidatesSQL`:

```go
const selectPurgeTransactionCandidatesSQL = `
SELECT sleeper_transaction_id AS id, created_at
FROM sleeper_transactions
WHERE created_at_sleeper < ?
ORDER BY created_at_sleeper, sleeper_transaction_id
LIMIT ?`
```

Update `PurgeTransactionsBatch`'s doc comment and cutoff computation:

```go
// PurgeTransactionsBatch deletes up to BatchSize of the oldest cloud
// transactions — oldest by created_at_sleeper (Sleeper's own event
// timestamp), not by created_at (when we happened to insert the row) — that
// are older than RetentionDays and verified present in the archive. Using
// event time means a freshly-backfilled old transaction (e.g. a newly
// discovered league's history) is purge-eligible as soon as it's verified,
// not 30 days after whenever it happened to be inserted. Unverified rows
// (not yet replicated) are left in place — the next batch/run naturally
// retries them since only verified rows are ever deleted. Returns an error
// (see checkUnverifiedAlarm) if the oldest unverified row's insert time is
// past retention+15d — that alarm intentionally stays on the insert-time
// clock (it's tracking replication lag, not event age).
func (a *ScavengerActivities) PurgeTransactionsBatch(ctx context.Context, params PurgeBatchParams) (PurgeBatchResult, error) {
	cutoffMs := time.Now().UTC().AddDate(0, 0, -params.RetentionDays).UnixMilli()

	var candidates []purgeCandidate
	if err := a.Cloud.WithContext(ctx).Raw(selectPurgeTransactionCandidatesSQL, cutoffMs, params.BatchSize).
		Scan(&candidates).Error; err != nil {
		return PurgeBatchResult{}, err
	}
```

(The rest of the function — verification, `splitVerifiedCandidates`, `deleteInChunks`, `checkUnverifiedAlarm`, the returned `PurgeBatchResult` — is unchanged; `purgeCandidate.CreatedAt` is still populated from the SELECT's `created_at` column, still insert time, still what the alarm checks.)

- [x] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/activities/... -run TestPurgeTransactionsBatch -v`
Expected: all 6 PASS (5 updated + 1 new).

- [x] **Step 5: Commit**

```bash
git add internal/activities/scavenger.go internal/activities/scavenger_test.go
git commit -m "fix: purge transactions by event time (created_at_sleeper), not insert time"
```

---

### Task 2: Fix `PurgeDraftsBatch` — season-based eligibility

**Files:**
- Modify: `backend/internal/activities/scavenger.go`
- Modify: `backend/internal/activities/scavenger_test.go`

**Interfaces:** no signature changes.

- [x] **Step 1: Update the existing tests' fixtures to season-based framing**

In `scavenger_test.go`, add `"strconv"` to the import block. All 4 existing `TestPurgeDraftsBatch_*` tests hardcode `Season: "2026"` and rely on `CreatedAt` for age — replace all 4 with:

```go
func TestPurgeDraftsBatch_DeletesVerifiedDraftAndPicks(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	fetchedAt := time.Now().UTC()
	oldSeason := strconv.Itoa(time.Now().Year() - 1)
	if err := cloud.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg1", Season: oldSeason, Status: "complete", LastDraftsFetchedAt: &fetchedAt,
	}).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: oldSeason,
		LastFetchedAt: &fetchedAt, CreatedAt: time.Now().UTC(), // inserted today, but last season — the exact scenario this fixes
	}).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	if err := cloud.Create(&models.SleeperDraftPick{SleeperDraftID: "d1", Round: 1, PickNo: 1, SleeperPlayerID: "p1"}).Error; err != nil {
		t.Fatalf("seed pick: %v", err)
	}
	if err := archive.Create(&models.ArchiveSleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: oldSeason,
		LastFetchedAt: &fetchedAt, CreatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed archive draft: %v", err)
	}
	if err := archive.Create(&models.ArchiveSleeperDraftPick{SleeperDraftID: "d1", Round: 1, PickNo: 1, SleeperPlayerID: "p1"}).Error; err != nil {
		t.Fatalf("seed archive pick: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeDraftsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeDraftsBatch: %v", err)
	}
	if res.Purged != 1 || res.Unverified != 0 || !res.Drained {
		t.Errorf("res = %+v, want {Purged: 1, Unverified: 0, Drained: true}", res)
	}
	var draftCount, pickCount int64
	cloud.Model(&models.SleeperDraft{}).Count(&draftCount)
	cloud.Model(&models.SleeperDraftPick{}).Count(&pickCount)
	if draftCount != 0 || pickCount != 0 {
		t.Errorf("expected draft and picks purged from cloud, got draftCount=%d pickCount=%d", draftCount, pickCount)
	}
}

func TestPurgeDraftsBatch_SkipsLeagueStillInSyncPool(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	oldSeason := strconv.Itoa(time.Now().Year() - 1)
	if err := cloud.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg1", Season: oldSeason, Status: "pre_draft", // not yet excluded from the claim pool
	}).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
	fetchedAt := time.Now().UTC()
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: oldSeason,
		LastFetchedAt: &fetchedAt, CreatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	if err := archive.Create(&models.ArchiveSleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: oldSeason,
		LastFetchedAt: &fetchedAt, CreatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed archive draft: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeDraftsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeDraftsBatch: %v", err)
	}
	if res.Purged != 0 || !res.Drained {
		t.Errorf("expected the draft to be excluded from purge candidates entirely (league still claimable), got %+v", res)
	}
	var draftCount int64
	cloud.Model(&models.SleeperDraft{}).Count(&draftCount)
	if draftCount != 1 {
		t.Errorf("expected the draft to remain in cloud, got %d", draftCount)
	}
}

func TestPurgeDraftsBatch_SkipsPickCountMismatch(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	fetchedAt := time.Now().UTC()
	oldSeason := strconv.Itoa(time.Now().Year() - 1)
	if err := cloud.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg1", Season: oldSeason, Status: "complete", LastDraftsFetchedAt: &fetchedAt,
	}).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: oldSeason,
		LastFetchedAt: &fetchedAt, CreatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	for _, pickNo := range []int{1, 2} {
		if err := cloud.Create(&models.SleeperDraftPick{SleeperDraftID: "d1", Round: 1, PickNo: pickNo}).Error; err != nil {
			t.Fatalf("seed pick %d: %v", pickNo, err)
		}
	}
	if err := archive.Create(&models.ArchiveSleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: oldSeason,
		LastFetchedAt: &fetchedAt, CreatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed archive draft: %v", err)
	}
	// Only 1 of 2 picks made it to archive — parity mismatch.
	if err := archive.Create(&models.ArchiveSleeperDraftPick{SleeperDraftID: "d1", Round: 1, PickNo: 1}).Error; err != nil {
		t.Fatalf("seed archive pick: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeDraftsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeDraftsBatch: %v", err)
	}
	if res.Purged != 0 || res.Unverified != 1 {
		t.Errorf("res = %+v, want {Purged: 0, Unverified: 1} (pick count mismatch)", res)
	}
	var draftCount int64
	cloud.Model(&models.SleeperDraft{}).Count(&draftCount)
	if draftCount != 1 {
		t.Errorf("expected the draft to remain in cloud, got %d", draftCount)
	}
}

func TestPurgeDraftsBatch_IgnoresRecentDrafts(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	fetchedAt := time.Now().UTC()
	currentSeason := strconv.Itoa(time.Now().Year())
	if err := cloud.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg1", Season: currentSeason, Status: "complete", LastDraftsFetchedAt: &fetchedAt,
	}).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: currentSeason,
		LastFetchedAt: &fetchedAt, CreatedAt: time.Now().UTC().AddDate(0, 0, -40), // inserted a while ago, but still the current season
	}).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeDraftsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeDraftsBatch: %v", err)
	}
	if res.Purged != 0 || !res.Drained {
		t.Errorf("expected no candidates (draft is the current season), got %+v", res)
	}
}
```

Also add:

```go
func TestPurgeDraftsBatch_EligibleBySeasonDespiteRecentInsertTime(t *testing.T) {
	cloud, archive := newScavengerTestDBs(t)
	fetchedAt := time.Now().UTC()
	oldSeason := strconv.Itoa(time.Now().Year() - 3) // several seasons old
	if err := cloud.Create(&models.SleeperLeague{
		SleeperLeagueID: "lg1", Season: oldSeason, Status: "complete", LastDraftsFetchedAt: &fetchedAt,
	}).Error; err != nil {
		t.Fatalf("seed league: %v", err)
	}
	if err := cloud.Create(&models.SleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: oldSeason,
		LastFetchedAt: &fetchedAt, CreatedAt: time.Now().UTC(), // freshly inserted today — e.g. a new league's backfill
	}).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	if err := archive.Create(&models.ArchiveSleeperDraft{
		SleeperDraftID: "d1", SleeperLeagueID: "lg1", Status: "complete", Season: oldSeason,
		LastFetchedAt: &fetchedAt, CreatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed archive draft: %v", err)
	}

	a := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
	res, err := a.PurgeDraftsBatch(context.Background(), activities.PurgeBatchParams{BatchSize: 10, RetentionDays: 30})
	if err != nil {
		t.Fatalf("PurgeDraftsBatch: %v", err)
	}
	if res.Purged != 1 {
		t.Errorf("expected the draft to be purge-eligible despite being inserted today, because its season is 3 years old; got %+v", res)
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/activities/... -run "TestPurgeDraftsBatch_EligibleBySeasonDespiteRecentInsertTime|TestPurgeDraftsBatch_DeletesVerifiedDraftAndPicks" -v`
Expected: both FAIL (current `created_at`-based query sees `CreatedAt: time.Now()` = today, well within retention).

- [x] **Step 3: Implement**

In `scavenger.go`, replace `selectPurgeDraftCandidatesSQL`:

```go
const selectPurgeDraftCandidatesSQL = `
SELECT d.sleeper_draft_id AS id, d.created_at
FROM sleeper_drafts d
JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id
WHERE d.season < ?
  AND l.status IN ('in_season', 'complete')
  AND l.last_drafts_fetched_at IS NOT NULL
ORDER BY d.season, d.sleeper_draft_id
LIMIT ?`
```

Update `PurgeDraftsBatch`'s doc comment and cutoff computation:

```go
// PurgeDraftsBatch deletes up to BatchSize of the oldest cloud drafts (and
// their picks) whose season is before the current season (see
// currentSleeperSeason in data_fetch.go) and whose owning league satisfies
// the claim-pool-exclusion predicate — status IN ('in_season','complete')
// AND last_drafts_fetched_at IS NOT NULL, the same condition that
// permanently excludes a league from ClaimLeaguesForDrafts
// (data_fetch.go:43-54). Purging a draft whose league could still be
// re-claimed would let syncOneLeagueDrafts recreate the header with
// last_fetched_at = NULL and trigger a full pick-refetch loop.
//
// Eligibility is season-based, not insert-time-based: drafts have no
// per-row date, so "season" is the age proxy (same convention T13's ingest
// routing uses). RetentionDays no longer affects eligibility here — a
// season only ends once a year, so there's no day-granularity retention
// concept to apply — but it's still passed to checkUnverifiedAlarm for the
// stalled-replication threshold, which stays anchored to insert time.
//
// A draft is verified only when its header is present in the archive AND
// its cloud and archive pick counts match exactly. Unverified drafts are
// left in place — the next batch/run retries them. Picks are deleted before
// the draft header (FK, no ON DELETE CASCADE in the cloud schema).
func (a *ScavengerActivities) PurgeDraftsBatch(ctx context.Context, params PurgeBatchParams) (PurgeBatchResult, error) {
	cutoffSeason := currentSleeperSeason()

	var candidates []purgeCandidate
	if err := a.Cloud.WithContext(ctx).Raw(selectPurgeDraftCandidatesSQL, cutoffSeason, params.BatchSize).
		Scan(&candidates).Error; err != nil {
		return PurgeBatchResult{}, err
	}
```

(The rest of the function is unchanged.)

- [x] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/activities/... -run TestPurgeDraftsBatch -v`
Expected: all 5 PASS (4 updated + 1 new).

- [x] **Step 5: Run the full activities package for regressions**

Run: `cd backend && go test ./internal/activities/... -v 2>&1 | grep -E "^(--- |FAIL|PASS|ok)"`
Expected: everything PASSes, including every other pre-existing test untouched by this plan.

- [x] **Step 6: Commit**

```bash
git add internal/activities/scavenger.go internal/activities/scavenger_test.go
git commit -m "fix: purge drafts by season, not insert time"
```

---

### Task 3: Index migration for the new eligibility queries

**Files:**
- Create: `backend/migrations/023_purge_event_time_indexes.sql`
- Modify: `backend/internal/dbmigrate/dbmigrate_test.go`

**Interfaces:** none — pure SQL/test addition.

- [x] **Step 1: Write the migration**

```sql
-- backend/migrations/023_purge_event_time_indexes.sql
-- +goose Up
-- +goose NO TRANSACTION

-- Supports PurgeTransactionsBatch's WHERE created_at_sleeper < ? ORDER BY
-- created_at_sleeper, sleeper_transaction_id. The only existing index on
-- created_at_sleeper (012_sleeper_indexes.sql) is partial — type='trade' AND
-- status='complete' only — and doesn't cover this general scan across every
-- transaction.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_transactions_created_at_sleeper
    ON sleeper_transactions (created_at_sleeper);

-- Supports PurgeDraftsBatch's WHERE season < ? ORDER BY season, sleeper_draft_id.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sleeper_drafts_season
    ON sleeper_drafts (season);

-- +goose Down
-- +goose NO TRANSACTION

DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_drafts_season;
DROP INDEX CONCURRENTLY IF EXISTS idx_sleeper_transactions_created_at_sleeper;
```

- [x] **Step 2: Extend the migration-index assertion test**

In `dbmigrate_test.go`, change `TestRun_CloudMigrations_ApplyCleanlyAndCreateScavengerIndexes`'s index list:

```go
	for _, idx := range []string{
		"idx_sleeper_transactions_created_at", "idx_sleeper_drafts_last_fetched_at", "idx_sleeper_drafts_created_at",
		"idx_sleeper_transactions_created_at_sleeper", "idx_sleeper_drafts_season",
	} {
```

- [x] **Step 3: Run the test**

Run: `cd backend && go test ./internal/dbmigrate/... -v`
Expected: PASS — migration 023 applies cleanly and both new indexes exist.

- [x] **Step 4: Commit**

```bash
git add migrations/023_purge_event_time_indexes.sql internal/dbmigrate/dbmigrate_test.go
git commit -m "feat: add indexes for event-time purge eligibility (created_at_sleeper, season)"
```

---

### Task 4: Update docs

**Files:**
- Modify: `docs/archive-purge.md`
- Modify: `docs/superpowers/plans/2026-07-07-two-database-archive.md`

**Interfaces:** none — documentation only.

- [x] **Step 1: Fix the monitoring SQL and preconditions prose in `docs/archive-purge.md`**

Replace the "direct SQL check of the remaining backlog" block in Step 2 (Monitor the drain):

```markdown
Direct SQL check of the remaining backlog, run against the **cloud** DB —
note this is by Sleeper's own event time (`created_at_sleeper`, epoch
milliseconds), not insert time; a row can be purge-eligible the moment
it's replicated even if it was only just inserted (e.g. during a new
league's backfill):

```sql
SELECT count(*) FROM sleeper_transactions
WHERE created_at_sleeper < extract(epoch from now() - interval '30 days') * 1000;

SELECT count(*) FROM sleeper_drafts WHERE season < to_char(now(), 'YYYY');
```
```

Add a short callout right after the "Preconditions" numbered list explaining the fix (since this file predates it and an operator reading it fresh should know this isn't insert-time-based):

```markdown
**Purge eligibility is based on when the data actually happened, not when
it was inserted.** Transactions use `created_at_sleeper` (Sleeper's own
event timestamp); drafts use `season`. This matters if you're backfilling
history for newly-discovered leagues — that data can be purge-eligible
immediately once verified in archive, not 30 days after whenever it
happened to be synced.
```

- [x] **Step 2: Fetch latest `main` and correct the master plan doc**

This file has been edited by multiple concurrent short-lived doc PRs recently — fetch and merge `origin/main` into this branch before editing, so the diff is additive, not a conflicting rewrite (same lesson as the T9 runbook PR).

```bash
git fetch origin main
git merge origin/main --no-edit
```

Then, replace Risk #5 (currently reads "Insert-time retention — freshly ingested old-season rows stay hot 30 days; accepted, self-correcting.") with:

```markdown
5. ~~**Insert-time retention**~~ — **fixed (T14, 2026-07-11)**: purge originally filtered by `created_at` (insert time), meaning freshly-backfilled old data stayed hot for a full 30 days regardless of how old the underlying event actually was — this wasn't a deliberate tradeoff, it was a defect, discovered in production when ~40M backfilled transactions were all insert-time-recent and therefore zero were purge-eligible. Purge now filters transactions by `created_at_sleeper` (event time) and drafts by `season`. Replicate (T5) is unaffected — its cursor correctly still needs insert-time's monotonic-arrival guarantee.
```

Update the Scavenger design section's purge bullet (currently: "3. Purge (only if enabled and caught up): select cloud IDs past 30d → verify in archive → chunked deletes in short transactions...") to clarify the field:

```markdown
3. Purge (only if enabled and caught up): select cloud IDs whose *event* age (transactions: `created_at_sleeper`; drafts: `season`) is past 30d/the current season → verify in archive → chunked deletes in short transactions. Drafts additionally require the claim-pool-exclusion predicate and pick-count parity. Unverified rows are skipped and counted; oldest unverified (by insert time) > retention+15d ⇒ activity error ⇒ **red run in Temporal UI = replication-stalled alarm**.
```

Add T14's row to the task table, right after T13:

```markdown
| T14 | Fix purge eligibility to use event time (`created_at_sleeper`/`season`), not insert time — see Risk #5 | S/M | T6 | Done — PR #<fill in> |
```

- [x] **Step 3: Commit**

```bash
git add ../docs/archive-purge.md ../docs/superpowers/plans/2026-07-07-two-database-archive.md
git commit -m "docs: correct purge eligibility docs to event-time (created_at_sleeper/season)"
```

(Paths are relative to `backend/` — adjust if running from the repo root.)

---

### Task 5: Full verification

- [x] **Step 1: Full build, vet, and test suite**

Run: `cd backend && go build ./... && go vet ./...`
Expected: clean.

Run (with `TEST_DATABASE_URL` set): `cd backend && go test ./... -v 2>&1 | tail -100`
Expected: every test PASSes — the 11 updated/new purge tests, the new index-migration assertions, and every pre-existing test untouched by this plan (replicate tests, T7/T8/T13 tests, etc.).

- [x] **Step 2: Sanity-check the SQL directly against a real Postgres**

The unit tests already run against real Postgres via `newScavengerTestDBs` (not SQLite — this package's tests are PG-only, per its existing convention), so the raw SQL is already exercised for correctness. No additional manual step needed beyond Step 1 — unlike T5/T7/T8/T13, this task adds no new DB handle wiring or worker startup path, so there's nothing new to smoke-test at the `cmd/worker` boot level.

---

## Verification

- [x] `cd backend && go build ./...` and `go vet ./...` clean.
- [x] `cd backend && go test ./...` — full suite passes.
- [x] All 11 purge tests (6 transaction + 5 draft) pass, including the 2 new ones that directly encode "eligible despite recent insert time."
- [x] `dbmigrate` tests confirm migration 023 applies cleanly and both new indexes exist.
- [x] `docs/archive-purge.md`'s monitoring SQL and preconditions prose reflect event-time eligibility.
- [x] The master plan doc's Risk #5, Scavenger design bullet, and task table (new T14 row) are corrected/updated.

## Self-Review

**Spec coverage:** the user's stated requirement — "if the transaction/draft is > 30d old it should go to archive... It doesn't matter what order it came in" — maps directly to Tasks 1 (transactions) and 2 (drafts). The user's explicit follow-up ("It will also require updating docs/archive-purge.md") is Task 4.

**Placeholder scan:** no TBD/TODO markers; every step has literal code. Task 4's PR-number placeholder (`PR #<fill in>`) is the one intentional exception — it's filled in after the PR actually exists, same pattern as every prior status-table update in this project.

**Type consistency:** `PurgeTransactionsBatch`/`PurgeDraftsBatch` signatures are unchanged throughout — only their internal SQL and cutoff-computation logic changes, so nothing downstream (the `ScavengerDispatcher` workflow, its call sites) needs touching. `purgeCandidate{ID, CreatedAt}` stays the same shape across both fixed functions — `CreatedAt` consistently means "insert time, for the alarm" in both, never repurposed to mean something else between the two.
