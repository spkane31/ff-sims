# Player Valuation on Real Sleeper Data

**Date:** 2026-07-02
**Status:** Approved

## Goal

Wire the Bayesian player-valuation model in `analysis/main.py` to the real Sleeper
data scraped by the Temporal workers, so it can run incrementally at set times,
persist per-player valuations over time, and be ready for the 2026 season while
validating against a full 2025-season replay.

## Background / Current State

- `analysis/main.py` contains a working recursive-belief valuation model
  (ADP seed → trade constraints → weekly points-above-replacement), currently
  fed by demo data or CSVs. `src/db.py` has two stub helpers (`get_adp`,
  `get_trades`) and `src/models.py` has two empty dataclasses.
- Trade timestamps **already exist**: `sleeper_transactions.created_at_sleeper`
  (unix ms, Sleeper's `created` field). Trade sides are reconstructable from the
  `adds` JSONB (player_id → receiving roster_id).
- League fetch tracking **already exists**: `sleeper_leagues.last_fetched_at`,
  `last_transactions_fetched_at`, `last_transaction_leg_fetched`.
- Weekly player scores have **no data source at all** — the largest gap.
- The existing `player_valuations` table (migration 005) has columns from an
  older design (`raw_trade_value`, `recency_factor`, `age_curve_factor`,
  `adjusted_value`), is empty, and has no writers.
- Segment distribution (fetched leagues): full-PPR / superflex / 12-team is
  dominant with 21,434 leagues (4x the next segment).

## Decisions

| Decision | Choice |
|---|---|
| Weekly scores source | Sleeper season-week stats API (one call per week, all NFL players, precomputed `pts_ppr`) |
| Stats fetcher | Go Temporal activity/workflow in `backend/`, matching existing scraper patterns |
| Run model | Incremental with persisted beliefs + watermarks (a separate backlog item ensures transactions are fetched fast enough to arrive in order) |
| Valuation storage | Migrate `player_valuations` to match the model's outputs, keyed with a segment label |
| Segment (v1) | `ppr-sf-12`: full PPR, superflex, 12-team, redraft; ADP from snake drafts only |
| Trades with draft picks | Skip in v1; valuing picks off the curve is a planned future extension |
| Season handling | `--season` CLI parameter with per-season date config; bootstrap = full 2025 replay |
| Backtesting | `--backtest` replays a season as if live, writing dated snapshots along the way; rerunnable as backlog data lands; doubles as the bootstrap |

## Components

### 1. Go: weekly NFL stats scraper

> **Spun out to [#118](https://github.com/spkane31/ff-sims/issues/118)** — built
> in parallel by a separate agent. The valuation work below treats it as an
> external dependency: migration `013` and the `sleeper_player_week_stats` /
> `sleeper_week_stat_fetches` tables land via that issue; this branch reserves
> migration `014`. The 2025 backtest validation is blocked until the 2025
> backfill from #118 completes.

**Sleeper client** (`backend/internal/sleeper/`):
- `GetWeekStats(ctx, season, week)` → `GET https://api.sleeper.app/v1/stats/nfl/regular/{season}/{week}`.
  Returns a map of player_id → stat fields including `pts_ppr`, `pts_half_ppr`, `pts_std`.

**Migration `013_sleeper_week_stats.sql`:**

```sql
CREATE TABLE sleeper_player_week_stats (
    season             TEXT NOT NULL,
    week               INT  NOT NULL,
    sleeper_player_id  TEXT NOT NULL,
    pts_ppr            FLOAT,
    pts_half_ppr       FLOAT,
    pts_std            FLOAT,
    stats              JSONB,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (season, week, sleeper_player_id)
);

CREATE TABLE sleeper_week_stat_fetches (
    season           TEXT NOT NULL,
    week             INT  NOT NULL,
    last_fetched_at  TIMESTAMPTZ,
    finalized        BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (season, week)
);
```

Rows are filtered to fantasy positions (QB/RB/WR/TE/K/DEF) before insert to skip
IDP noise. `sleeper_week_stat_fetches` is the per-week fetch tracking: during the
season the current week is refetched until marked final; finalized weeks are
skipped. (This is the new-data-type analogue of the existing league fetch
tracking columns.)

**Activity + workflow** (`backend/internal/activities/`, `backend/internal/workflows/`):
- `FetchWeekStats(season, week)` — fetch, filter, upsert, update the fetch row.
- `SyncWeekStats(season)` workflow — loop weeks 1–18, fetch non-finalized weeks.
  Registered in `backend/schedules/register.go` like existing syncs.
- 2025 backfill = one run of the workflow with season "2025" (~18 API calls).

### 2. Migration: valuation storage + belief state

**Migration `014_player_valuation_model.sql`** — drop and rebuild
`player_valuations` (empty, no writers), add state tables:

```sql
DROP TABLE player_valuations;

CREATE TABLE player_valuations (
    segment            TEXT NOT NULL,
    sleeper_player_id  TEXT NOT NULL REFERENCES sleeper_players(sleeper_player_id),
    valuation_date     DATE NOT NULL,
    rank               INT,
    value              FLOAT,
    vorp               FLOAT,
    sd                 FLOAT,
    games              FLOAT,
    position           TEXT,
    PRIMARY KEY (segment, sleeper_player_id, valuation_date)
);

CREATE TABLE valuation_state (
    segment            TEXT NOT NULL,
    sleeper_player_id  TEXT NOT NULL,
    guess              FLOAT NOT NULL,
    var                FLOAT NOT NULL,
    games              FLOAT NOT NULL DEFAULT 0,
    cum_par            FLOAT NOT NULL DEFAULT 0,
    position           TEXT,
    name               TEXT,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (segment, sleeper_player_id)
);

CREATE TABLE valuation_runs (
    segment                   TEXT NOT NULL,
    season                    TEXT NOT NULL,
    last_event_ts             TIMESTAMPTZ,
    last_transaction_created  BIGINT,
    last_week_processed       INT,
    last_run_at               TIMESTAMPTZ,
    PRIMARY KEY (segment, season)
);
```

- `player_valuations`: one snapshot row per player per run date — the
  over-time series a frontend can chart later. Re-running the same day upserts.
- `valuation_state`: persisted beliefs for incremental runs.
- `valuation_runs`: watermarks. Trades advance by `created_at_sleeper`; scores
  by week number; `last_event_ts` is the model clock used for aging.

Migrations live in `backend/migrations/` (goose) as the single schema source of
truth, even though Python consumes these tables.

### 3. Python: `analysis/` wiring

**Segment constant.** `ppr-sf-12` with filters
`ppr = 1.0 AND is_superflex AND total_rosters = 12 AND league_type = 'redraft'`;
ADP additionally requires `draft_type = 'snake'` (auction `pick_no` is not a
draft position).

**`src/models.py`** — real dataclasses:
- `AverageDraftPosition(player_id, player_name, position, adp)`
- `Trade(trade_id, ts, side_a, side_b)` — replaces the empty `TradeSide`; a
  whole trade is the unit the model consumes.
- `WeeklyScore(week, player_id, position, points)`

**`src/db.py`** — `psycopg` connection + `.env` discovery following
`workers/espn/db.py`; add `psycopg` and `python-dotenv` to `analysis/pyproject.toml`.
- `get_adp(conn, segment, season)` — avg `pick_no` per player across segment
  snake drafts, joined to `sleeper_players` for name/position.
- `get_trades(conn, segment, season, since_created)` — transactions with
  `type = 'trade' AND status = 'complete'` in segment leagues above the
  watermark. Sides grouped from `adds` by roster_id. Skip trades that are not
  clean two-sided player-for-player (3-team trades, non-empty `draft_picks`).
- `get_weekly_scores(conn, season, after_week)` — `pts_ppr` from
  `sleeper_player_week_stats` for finalized weeks above the watermark.
- State I/O: `load_state` / `save_state` (valuation_state),
  `get_run` / `save_run` (valuation_runs), `write_snapshot` (player_valuations).

**`main.py`** — DB mode becomes the default (`--demo` keeps the synthetic path):
1. Load watermarks + beliefs for (segment, season). Empty state → run the
   backtest replay as the bootstrap (see below).
2. Every run also seeds any ADP player not yet in state (late-arriving
   backlog drafts).
3. Build events: new trades at real `created_at_sleeper` timestamps; new
   finalized weeks at `week_to_date(week)`. `advance()` unchanged. Events older
   than `last_event_ts` are skipped with a warning (guard while the
   fast-transaction-fetch backlog item is outstanding).
4. Save beliefs, watermarks, and the dated snapshot.

**Backtest mode (`--backtest`).** Replays a season as if live: seed from ADP at
the draft date, then walk the event stream in time order, writing a
`player_valuations` snapshot at each day boundary where beliefs changed (event
days — trades and score landings; aging alone changes only `sd`, not `value`).
Because events are processed in timestamp order anyway, this is a single pass,
not N independent re-runs. Re-running backtest after more backlogged 2025
trades land deletes and rewrites the segment/season's snapshot range — the
"completely rewrite as more trade data arrives" behavior. Backtest finishes by
saving `valuation_state` and `valuation_runs`, so it doubles as the bootstrap
that live incremental runs continue from.

Season becomes `--season` (default 2025 for now) with a small per-season config
mapping season → draft date and week-1 kickoff, replacing the hardcoded 2025
dates. Config tunables get superflex-12-team starting values (QB weekly
replacement rank ~24, `RHO_RANK` deeper than 130) — marked as guesses to tune
later against held-out trades.

## Testing

- **Go:** activity tests for the stats fetch/upsert and week tracking, in the
  style of `data_fetch_test.go`.
- **Python:** add pytest to `analysis/`. Unit tests for trade-side parsing from
  `adds` JSONB (two-sided, three-team skip, draft-picks skip), watermark/event
  building, and season date math.
- **Validation:** 2025 backtest; sanity-check the top 30 at season end against
  known 2025 outcomes (QBs near the top, given superflex), and spot-check that
  the valuation time series moves sensibly around known events (breakouts,
  injuries, big trades).

## Out of Scope (v1)

- Valuing traded draft picks off the curve (planned future extension).
- Running the valuation inside Temporal (manual CLI for now; designed to slot in later).
- Multiple segments (schema supports it via the `segment` key; only `ppr-sf-12` runs).
- Frontend display of valuation history.
- Estimating replacement value ρ from unbalanced trades.
- Speeding up transaction fetching to eliminate out-of-order arrivals (separate backlog item; v1 guards by skipping stale events with a warning).
