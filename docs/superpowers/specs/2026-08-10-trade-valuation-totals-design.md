# Trade Valuation Totals: Persist Per-Side Values at Sync Time

**Date:** 2026-08-10
**Status:** Approved

**Update (2026-08-10, plan phase):** `analysis/main.py`'s `default_end()` is date-only
(`utc_today()`, no time-of-day) — its own docstring says the newest snapshot a run writes
"covers every event through the end of yesterday," deliberately never a same-day/partial
snapshot. Two runs on the same calendar day therefore compute an identical window and
produce byte-identical output: **a cadence bump alone does not improve trade-valuation
freshness today**, regardless of which two times of day are chosen. The timer change ships
anyway (at `06:00`/`18:00 UTC` per the plan) as inert prep for a later change to make the
replay's end boundary time-aware; the freshness gate and reconcile-sweep backfill described
below don't depend on it and work the same either way.

**Update (2026-08-11, post-merge bug fix):** The design below (and the plan built from it)
had a real bug, caught in post-merge review: `ComputeTradeValues` wrote a non-NULL
`trade_values` the moment **any one side** of a trade resolved, but `ReconcileTradeValues`
only ever retried rows where `trade_values IS NULL`. A trade with one resolved side and one
still-pending side therefore got permanently stuck with a partial total — the pending side
could never be filled in later, even after its player got a valuation, because the row had
already dropped out of the reconcile query's candidate set. Fixed by adding
`sleeper_transactions.trade_values_complete` (migration `033_trade_values_complete.sql`), a
column tracking per-trade completeness independently of whether `trade_values` itself is
null. `ReconcileTradeValues` now gates on `trade_values_complete = false` instead of
`trade_values IS NULL`; every write path (insert-time `attachTradeValues` and the reconcile
sweep) sets both columns together from `valuation.ComputeTradeValues`'s new second return
value (`complete bool`, replacing the old "resolved anything at all" `bool`). Every mention
of "trade_values IS NULL" below describes the original (buggy) gate — the actual gate is
`trade_values_complete = false`.

## Problem

`/trades` on the frontend shows a per-side total valuation for some trades and leaves it
blank for others, with no visible pattern to the user. The actual cause: `GetSleeperTrades`
(`backend/internal/api/handlers/sleeper.go:400-563`) computes valuation totals live, on every
request, by joining each trade's players against `player_valuations` as of the trade's
timestamp (`segmentKeyForLeague`, `loadValuationHistory`, `valueAsOf`, `applySideValues`).
Nothing is ever persisted per trade. Two things make this look broken:

- `player_valuations` is only ever written by `analysis/main.py`, invoked by
  `ff-sims-player-valuations.service` (`deploy/worker-host/ff-sims-player-valuations.service`),
  currently on a **daily** `00:00 UTC` timer
  (`deploy/worker-host/ff-sims-player-valuations.timer`) and scoped to exactly one segment,
  `ppr-sf-10` (10-man, PPR, superflex, redraft) — `--segment ppr-sf-10 --season 2025`. Trades
  in any other league format never get a value; that's expected and out of scope here.
- `applySideValues` (`sleeper.go:206-225`) shows a side's total as soon as *any* one of its
  players has a valuation, silently omitting the rest — a side with one valued veteran and
  one unvalued rookie shows a total that understates that side's real value without
  indicating anything is missing.

## Goals

- Every trade in a `ppr-sf-10` league gets a persisted, per-side valuation total attached at
  sync time, not computed on read.
- A side's total is only shown when **every** player on that side has a valuation — no silent
  understatement.
- No new cron job/systemd unit for the linking work itself — it lives inside the existing
  transaction-sync job (`transactioncron`, `ff-sims-transactions.service`, runs every 10
  minutes).
- The mechanism that fills in missing values also serves as the backfill for trades that
  already exist in the database — no separate one-time backfill script.
- None of this may add blocking latency to `ff-sims-transactions.service`'s fetch → flush
  cycle (Sleeper API polling / new-trade ingestion), which already runs inside an 8-minute
  ceiling every 10 minutes.
- Player valuations refresh twice a day instead of once, tightening the gap between a trade
  happening and its players having a valuation recent enough to use.

## Non-goals

- Valuing draft picks. No pick-valuation model exists; picks are ignored when deciding
  whether a side is "fully valued" and don't contribute to the total.
- Extending valuation coverage to `ppr-sf-8` / `ppr-sf-12` or any other league format. The
  segment gate (`segmentKeyForLeague`, moving to `internal/valuation`) already supports them
  in principle, but only `ppr-sf-10` has a systemd job actually producing snapshots today.
- Changing how `player_valuations` snapshots are computed (`analysis/src/market_value.py`)
  or how the replay itself works — only its cadence.
- Changing scavenger/purge behavior for `sleeper_transactions` or the archive DB.
- A general-purpose async task queue. The concurrency need here is narrow (one bounded
  background sweep per tick) and doesn't warrant one.

## Design

### Architecture

A new `backend/internal/valuation` package holds everything currently private to
`GetSleeperTrades` that computes values: `segmentKeyForLeague`/`knownValuationSegments`
(unchanged logic, exported), `loadValuationHistory`, `valueAsOf` (unchanged), and a new
`ComputeTradeValues` that replaces `applySideValues`'s "any player valued" rule with "every
player valued." This package has two importers:

- `transactioncron`, at two points in its existing 10-minute run:
  1. **Insert time** — `FlushLeagueTransactions` computes `trade_values` for newly-synced
     trade rows before writing them.
  2. **Reconcile sweep** — a bounded pass that retries existing `trade_values IS NULL` rows,
     launched **concurrently with** (not after) fetch → flush, both awaited before the
     process exits.
- `GetSleeperTrades`, which now just reads the persisted column — no more per-request
  `player_valuations` joins.

`sleeper_transactions` gains a `trade_values JSONB` column: `{"<roster_id>": <float>, ...}`,
present only for roster IDs where every player has a fresh valuation. A missing key means
"not computable yet," not an error — the reconcile sweep will keep retrying it.

`ff-sims-player-valuations.timer`'s `OnCalendar` moves from `00:00 UTC` daily to `00:00` and
`12:00 UTC`.

### Data model change

New migration `032_trade_values.sql`:

```sql
ALTER TABLE sleeper_transactions ADD COLUMN trade_values JSONB;
CREATE INDEX CONCURRENTLY idx_sleeper_transactions_trade_values_null
  ON sleeper_transactions (created_at_sleeper)
  WHERE type = 'trade' AND status = 'complete' AND trade_values IS NULL;
```

The partial index keeps the reconcile sweep's `SELECT ... WHERE trade_values IS NULL` cheap
regardless of how large `sleeper_transactions` grows — it shrinks as the backlog is worked
off, the same pattern already used by `idx_sleeper_transactions_trade_complete` for the
count-fast-path in `GetSleeperTrades` (`sleeper.go:434-446`).

Scoped to the **cloud** `sleeper_transactions` table only. The archive DB copy
(`ArchiveSleeperTransaction`, `upsertArchiveTransactions` in `fetch.go:256-269`) is never
read by `/trades` — it exists purely for historical retention — so it doesn't need this
column.

GORM model change: add `TradeValues json.RawMessage \`gorm:"column:trade_values;type:jsonb"\``
to `models.SleeperTransaction` (`backend/internal/models/sleeper.go:98-112`).

### Write path

**`valuation.ComputeTradeValues(sides map[int][]playerID, segment string, tradeTime time.Time, history map[string][]ValuationSnap) (json.RawMessage, bool)`**

For each roster ID's set of player IDs (built the same way `buildTradeSides` groups `adds` by
roster today, minus picks): look up each player's latest snapshot with
`valuation_date <= tradeTime` via `ValueAsOf` (unchanged 24h-window semantics come from the
caller only loading snapshots within 24h of `tradeTime` — see below), and require **all**
players on that roster to resolve before including that roster's total in the result map. A
roster with zero players (picks-only trade side) or any unvalued player is simply omitted
from the map. Returns `(nil, false)` when the whole map is empty, so the DB column stays SQL
`NULL` rather than storing `{}`.

Freshness: the caller passes `ValueAsOf` a snapshot set already filtered to
`valuation_date` within 24h of `tradeTime` — a value more than a day stale is treated the
same as no value. Given the replay's daily-granularity snapshots
(`--step 24h`), this is normally a no-op (the newest snapshot is always same-day) and only
matters as a safety net if the replay job is delayed or fails outright — better to show
nothing than a day-old-plus number.

**Insert-time hook** — in `FlushLeagueTransactions` (`fetch.go:208-252`), before building
`cloudRows`: load `(sleeper_league_id, ppr, is_superflex, total_rosters, league_type)` for the
batch's `leagueIDs` (one indexed query, `leagueIDs` is already collected at line 210-215),
resolve each trade row's segment, batch-load `player_valuations` for the segment's players in
this batch (mirroring `loadValuationHistory`'s batching, one query per segment present —
practically always just `ppr-sf-10`), and set `r.TradeValues` via `ComputeTradeValues` before
each row is appended to `cloudRows`. A lookup failure (e.g. a DB error loading valuations) is
logged and leaves `TradeValues` nil — it must never fail the trade insert itself. Trade
ingestion (`fetch → flush`) is unchanged in every other respect.

**`ReconcileTradeValues(ctx context.Context, db *gorm.DB, limit int) error`** — new function,
called from the top of the tick's run body concurrently with (not sequenced after)
fetch → flush:

```
go func() { errCh <- reconcile(reconcileCtx, db, 200) }()
fetchFlushErr := runFetchFlush(ctx, ...)
reconcileErr := <-errCh
```

- `reconcileCtx` is `context.WithTimeout(ctx, 5*time.Second)` — independent of the run's
  overall deadline, so a slow sweep can never extend the tick past its own small budget.
- Query: join `sleeper_transactions`/`sleeper_leagues` the same way `GetSleeperTrades` does
  today, `WHERE type='trade' AND status='complete' AND trade_values IS NULL`, `ORDER BY
  created_at_sleeper DESC LIMIT 200` (newest first — matches `/trades`' own default order, so
  the trades users are actually looking at get reconciled first; also means a large initial
  backlog drains newest-to-oldest rather than uniformly).
- For each candidate, recompute via `ComputeTradeValues` using whatever's fresh *now*
  (unlike insert time, `tradeTime` here is in the past, but freshness is still evaluated
  against the trade's own timestamp, not "now" — a trade from two weeks ago either has a
  same-day snapshot from the replay's historical window or it doesn't).
- Non-nil results get `UPDATE sleeper_transactions SET trade_values = ? WHERE
  sleeper_transaction_id = ? AND trade_values IS NULL` (guard against clobbering, though
  single-writer makes a race here unlikely in practice).
- No row limit exemption by age: this sweep is what backfills pre-existing trades, gradually,
  200 rows at a time per tick, for as long as any exist.

Because reconciliation runs concurrently with (not after) fetch → flush, its 5-second budget
overlaps the real wall-clock time fetch → flush already spends on Sleeper API calls in the
common case, adding effectively zero to the tick's total duration. Worst case — reconcile is
still running when fetch → flush finishes — it adds up to its own 5-second ceiling, well
inside the 8-minute service ceiling.

### Read path

`GetSleeperTrades` (`sleeper.go:400-563`): add `TradeValues json.RawMessage` to the
`tradeRow` struct and `SELECT`. Delete `loadValuationHistory`, `valueAsOf`, `applySideValues`,
`segmentKeyForLeague`, `knownValuationSegments`, `valuationSnap`, and the
`historyBySegment`/`playersBySegment`/`maxCreated` batching block (lines 496-526) from
`sleeper.go` — moved to `internal/valuation`. After `buildTradeSides`, unmarshal
`r.TradeValues` into `map[string]float64` and set `sides[i].TotalValue` by looking up
`strconv.Itoa(sides[i].RosterID)`. `formatScoring`/`formatLeagueSize` stay in the handler —
pure display formatting, unrelated to valuation.

### Error handling

- Insert-time valuation lookup errors: logged, `trade_values` left null, trade insert
  proceeds unaffected.
- Reconcile sweep errors (query or update failure, or the 5s timeout firing): logged via
  `slog.Error`, goroutine returns; never fails the tick or blocks fetch → flush's own error
  handling.
- Both paths are pure best-effort on top of ingestion, which is the load-bearing guarantee —
  trade sync correctness never depends on valuation succeeding.

### Testing

- `internal/valuation`: table-driven tests for `ComputeTradeValues` — full coverage produces
  a value; one unvalued player nulls that side only; a >24h-stale snapshot is treated as
  absent; a 3-way trade computes each roster independently; a picks-only side (no players)
  doesn't block or fabricate a total. Existing `segmentKeyForLeague`/`valueAsOf` tests move
  here from `sleeper_test.go` unchanged.
- `transactioncron`: fixture-DB test asserting `FlushLeagueTransactions` populates
  `trade_values` for a trade whose players already have fresh valuations at insert time;
  separate test seeds a null-valued row, adds a matching valuation snapshot, runs
  `ReconcileTradeValues`, and asserts the row is updated and the `limit` is respected against
  a larger backlog.
- `handlers`: `sleeper_test.go` updated so `GetSleeperTrades`'s trade-values assertions read
  from a seeded `trade_values` column instead of a live `player_valuations` fixture.

### Deployment

1. Apply migration `032_trade_values.sql` (`CREATE INDEX CONCURRENTLY`, safe live).
2. Ship the `internal/valuation` package + `transactioncron`/handler changes together —
   `ff-sims-transactions.service` and the API server both need this deploy.
3. Update `ff-sims-player-valuations.timer`'s `OnCalendar` and the stale "daily"
   wording in both `.timer` and `.service` files' comments/`Description`. Ships via the
   existing `git update` systemd redeploy path already in place — no new unit files.
4. No backfill step to run — the reconcile sweep drains existing null rows on its own over
   subsequent ticks once the deploy lands.
