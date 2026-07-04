# Spec: Admin Backlog Fetch-Age Buckets

**Date:** 2026-07-04
**Status:** Draft

## Context

`/admin` shows a "Sync Backlog" stat row (`GetAdminBacklog`, `backend/internal/api/handlers/admin.go:191`)
with two numbers for the current season: how many leagues have never had transactions fetched, and
the oldest `last_transactions_fetched_at` among the ones that have. That's a coarse view — it
doesn't show how the rest of the (fetched) leagues are distributed by staleness, which matters for
sizing Temporal worker throughput.

This spec adds a bucketed breakdown of current-season league transaction-fetch staleness: a
"never fetched" bucket plus six 4-hour buckets covering 0–24h, plus a catch-all 24h+ bucket. It's
rendered as a new table placed inside the existing Discovery Frontier section of `/admin`, even
though it's sourced from the backlog endpoint/data, per explicit placement request. The existing
Discovery Frontier leagues-by-season table also gets an explanatory caption, since its
Total/Expanded/Pending/Skipped/% Pending columns aren't self-explanatory.

## Non-goals

- No change to bucket scope: current season only (`season = MAX(season)`), matching the existing
  Sync Backlog stat cards — not all seasons like Discovery Frontier's per-season table.
- No historical/trend view — point-in-time snapshot, matching every other admin endpoint.
- No SQL-side `NOW()`/`INTERVAL` bucketing — computed in Go instead, so it stays testable against
  the in-memory SQLite fake used by `admin_test.go` (same reasoning as the ADP 95% CI computation
  in PR #134, which was done in Go rather than SQL `PERCENTILE_CONT` for the same reason).
- No auth (matches existing app-wide posture).

## Design

### Backend

**Added to** `backend/internal/api/handlers/admin.go`:

```go
// AdminBacklogBucketRow is one fetch-age bucket for current-season leagues,
// ordered from "never fetched" through "24h+".
type AdminBacklogBucketRow struct {
	Label   string `json:"label"`
	Leagues int64  `json:"leagues"`
}
```

`AdminBacklogResponse` gains:

```go
Buckets []AdminBacklogBucketRow `json:"buckets"`
```

Bucket labels, in fixed order: `Never fetched`, `0h-3h59m`, `4h-7h59m`, `8h-11h59m`,
`12h-15h59m`, `16h-19h59m`, `20h-23h59m`, `24h+`.

`GetAdminBacklog` already scopes to `season = MAX(season) AND skipped_at IS NULL`. After computing
`TotalLeagues`/`NeverFetchedCount`/`OldestTransactionsFetchedAt`, pull just the timestamp column
for that same scope and bucket in Go:

```go
var timestamps []*time.Time
if err := database.DB.Model(&models.SleeperLeague{}).
	Where("season = ? AND skipped_at IS NULL", season).
	Pluck("last_transactions_fetched_at", &timestamps).Error; err != nil {
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	return
}
resp.Buckets = bucketBacklogAges(timestamps, time.Now())
```

`bucketBacklogAges` is a small package-level helper:

```go
func bucketBacklogAges(timestamps []*time.Time, now time.Time) []AdminBacklogBucketRow {
	labels := []string{
		"Never fetched", "0h-3h59m", "4h-7h59m", "8h-11h59m",
		"12h-15h59m", "16h-19h59m", "20h-23h59m", "24h+",
	}
	counts := make(map[string]int64, len(labels))

	for _, ts := range timestamps {
		if ts == nil {
			counts["Never fetched"]++
			continue
		}
		hours := now.Sub(*ts).Hours()
		switch {
		case hours < 4:
			counts["0h-3h59m"]++
		case hours < 8:
			counts["4h-7h59m"]++
		case hours < 12:
			counts["8h-11h59m"]++
		case hours < 16:
			counts["12h-15h59m"]++
		case hours < 20:
			counts["16h-19h59m"]++
		case hours < 24:
			counts["20h-23h59m"]++
		default:
			counts["24h+"]++
		}
	}

	rows := make([]AdminBacklogBucketRow, len(labels))
	for i, label := range labels {
		rows[i] = AdminBacklogBucketRow{Label: label, Leagues: counts[label]}
	}
	return rows
}
```

Buckets are always returned in the fixed order above, with zero counts included (not omitted), so
the frontend table always has all 8 rows.

**Response shape:**

```json
{
  "season": "2026",
  "total_leagues": 42,
  "never_fetched_count": 5,
  "oldest_transactions_fetched_at": "2026-07-03T02:00:00Z",
  "buckets": [
    { "label": "Never fetched", "leagues": 5 },
    { "label": "0h-3h59m", "leagues": 20 },
    { "label": "4h-7h59m", "leagues": 10 },
    { "label": "8h-11h59m", "leagues": 4 },
    { "label": "12h-15h59m", "leagues": 2 },
    { "label": "16h-19h59m", "leagues": 1 },
    { "label": "20h-23h59m", "leagues": 0 },
    { "label": "24h+", "leagues": 0 }
  ]
}
```

### Frontend

- `frontend/src/services/adminService.ts` — add `AdminBacklogBucketRow` interface, add `buckets:
  AdminBacklogBucketRow[]` to `AdminBacklogResponse`.
- `frontend/src/pages/admin/index.tsx`:
  - `DiscoveryFrontier` becomes `function DiscoveryFrontier({ backlog }: { backlog:
    AdminBacklogResponse | null })`, called from the page component as `<DiscoveryFrontier
    backlog={backlog} />` (the page already holds `backlog` from `useAdminBacklog()` at the top).
  - Inside `DiscoveryFrontier`, below the existing leagues-by-season table, add a caption
    paragraph explaining that table's columns: "Total is every league discovered that season;
    Expanded means the discovery workflow has fetched it (`last_fetched_at` set); Pending is
    discovered but not yet expanded — the frontier left to crawl; Skipped is permanently excluded
    and doesn't count toward pending."
  - Below that, a new table titled "Transaction Fetch Age (season {backlog.season})": columns
    Bucket | Leagues | % of Total, rendering `backlog.buckets` in the fixed server order (no
    client-side re-sorting). % of Total uses `backlog.total_leagues` as denominator, `"—"` when
    zero.
  - Below the new table, a caption: "How stale each current-season league's transaction sync is,
    bucketed in 4-hour increments, to help gauge how much to scale the Temporal workers."
  - If `backlog` is `null` (still loading, or errored) or `total_leagues` is 0, render the new
    table's body as a single "No leagues yet." row (same empty-state convention as
    `SegmentDistribution`), matching the section's own loading/error handling for its primary
    data — the section doesn't duplicate backlog's own loading/error UI since that's already
    shown at the top of the page.

### Error handling

- Backend: DB errors return `500` with `gin.H{"error": ...}`, matching every other admin handler.
- Frontend: no new fetch is introduced (reuses `backlog` already fetched by the page), so no new
  loading/error state — falls back to the empty-state row if `backlog` isn't available yet.

### Testing

- Backend, in `admin_test.go`:
  - `TestGetAdminBacklog_Buckets`: current-season leagues with `last_transactions_fetched_at` at
    fixed offsets from `time.Now()` covering each bucket (nil, -1h, -5h, -9h, -13h, -17h, -21h,
    -30h), assert each bucket's count is 1 and all 8 labels are present in fixed order.
  - `TestGetAdminBacklog_BucketsExcludeOtherSeasonsAndSkipped`: leagues in a different season and
    a skipped league in the current season, assert they're excluded from bucket counts.
  - Extend `TestGetAdminBacklog_EmptyTable` to also assert `Buckets` has all 8 labels present with
    zero counts (not an empty/nil slice).
- Frontend: no automated test (no page-level test infra in the repo, matching existing precedent
  for this page); verified manually in-browser against a real Postgres backend.
