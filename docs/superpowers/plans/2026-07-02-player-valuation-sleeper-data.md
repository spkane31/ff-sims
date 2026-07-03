# Player Valuation on Real Sleeper Data — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the Bayesian valuation model in `analysis/main.py` to real Sleeper data in Postgres, with a persisted-belief incremental mode, a 2025 season backtest that writes dated valuation snapshots, and per-player valuations stored over time.

**Architecture:** The model code moves out of `main.py` into `analysis/src/` modules (valuation model, config, DB I/O, runner orchestration) so each piece is unit-testable. Migration `014` (Go/goose, in `backend/migrations/`) rebuilds `player_valuations` and adds `valuation_state` + `valuation_runs`. `main.py` becomes a thin CLI: `--backtest` replays a season in one time-ordered pass writing a snapshot per event day; the default incremental mode loads persisted beliefs, applies only events past the watermarks, and writes today's snapshot.

**Tech Stack:** Python 3.10+ managed by `uv` (`psycopg[binary]`, `python-dotenv`, `pandas`, `numpy`, `pytest`), goose SQL migrations in the Go backend.

**Spec:** `docs/superpowers/specs/2026-07-02-player-valuation-sleeper-data-design.md`. Component 1 (stats scraper) already landed via issue #118 / PR #120: `sleeper_player_week_stats` and `sleeper_week_stat_fetches` exist and hold finalized 2025 data.

## Global Constraints

- Segment v1 is exactly: `ppr = 1.0 AND is_superflex AND total_rosters = 12 AND league_type = 'redraft'`; ADP additionally requires draft `type = 'snake'`. Segment key string: `ppr-sf-12`.
- Migration number `014` (013 is taken by the stats scraper on main).
- All Python datetimes are **naive UTC**. Sleeper `created_at_sleeper` is unix **milliseconds**.
- Trades that include draft picks or FAAB (`draft_picks`/`waiver_budget` non-empty), or that don't resolve to exactly two roster sides, are skipped.
- Skip-with-warning any event at or before `valuation_runs.last_event_ts` (out-of-order guard).
- Python commands run from `analysis/` via `uv run …`. Go commands run from `backend/`.
- Model math in `Valuator` (`_fuse`, `apply_trade`, `apply_week`, `_age`) is moved, not modified.

---

### Task 1: Merge main; migration 014

**Files:**
- Create: `backend/migrations/014_player_valuation_model.sql`

**Interfaces:**
- Produces: tables `player_valuations(segment, sleeper_player_id, valuation_date, rank, value, vorp, sd, games, position)`, `valuation_state(segment, sleeper_player_id, guess, var, games, cum_par, position, name, updated_at)`, `valuation_runs(segment, season, last_event_ts, last_transaction_created, last_week_processed, last_run_at)` — consumed by Task 5's SQL.

- [ ] **Step 1: Merge latest main into this branch** (brings migration 013 and the stats scraper)

```bash
git merge origin/main -m "Merge main: sleeper week stats scraper (#120)"
```

Expected: clean merge (this branch only adds `analysis/` + docs files).

- [ ] **Step 2: Write the migration**

Create `backend/migrations/014_player_valuation_model.sql`:

```sql
-- +goose Up

-- Rebuild player_valuations: the old columns (raw_trade_value, recency_factor,
-- age_curve_factor, adjusted_value) were from an earlier valuation design that
-- was never implemented; the table is empty and has no writers.
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

CREATE INDEX idx_player_valuations_segment_date
    ON player_valuations (segment, valuation_date);

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
    last_transaction_created  BIGINT NOT NULL DEFAULT 0,
    last_week_processed       INT NOT NULL DEFAULT 0,
    last_run_at               TIMESTAMPTZ,
    PRIMARY KEY (segment, season)
);

-- +goose Down

DROP TABLE IF EXISTS valuation_runs;
DROP TABLE IF EXISTS valuation_state;
DROP TABLE IF EXISTS player_valuations;

CREATE TABLE player_valuations (
    sleeper_player_id  TEXT  REFERENCES sleeper_players(sleeper_player_id),
    valuation_date     DATE,
    raw_trade_value    FLOAT,
    recency_factor     FLOAT,
    age_curve_factor   FLOAT,
    adjusted_value     FLOAT,
    PRIMARY KEY (sleeper_player_id, valuation_date)
);
```

- [ ] **Step 3: Verify the embed still builds**

```bash
cd backend && go build ./...
```

Expected: no output, exit 0 (`migrations/fs.go` embeds `*.sql`, so a bad filename would break nothing here — this just confirms the module builds).

- [ ] **Step 4: Apply to the database**

This drops an **empty, writer-less** table and creates three new ones — safe, but it does touch the live DB.

```bash
cd backend && export $(grep -E '^DATABASE_URL=' .env | head -1) && go run ./cmd/migrate up
```

Expected: goose output ending in `OK   014_player_valuation_model.sql` (it may apply 013 first if the DB hasn't already).

- [ ] **Step 5: Verify tables exist**

```bash
cd backend && psql "$DATABASE_URL" -c "\d valuation_runs" -c "SELECT count(*) FROM sleeper_player_week_stats WHERE season='2025';"
```

Expected: `valuation_runs` columns as in Step 2; a non-zero 2025 stats count (populated by #118).

- [ ] **Step 6: Commit**

```bash
git add backend/migrations/014_player_valuation_model.sql
git commit -m "feat: migration 014 - valuation snapshots, belief state, run watermarks"
```

---

### Task 2: Python scaffolding + season/segment config

**Files:**
- Modify: `analysis/pyproject.toml`
- Create: `analysis/src/config.py`
- Create: `analysis/tests/__init__.py` (empty)
- Test: `analysis/tests/test_config.py`

**Interfaces:**
- Produces: `Segment` (fields `key, ppr, is_superflex, total_rosters, league_type, draft_type`), constant `PPR_SF_12`; `SeasonDates` (fields `draft_date, season_start, score_lag_days`), dict `SEASONS: dict[str, SeasonDates]`; `week_ts(season_dates, week) -> datetime` (naive midnight of the week's score-landing date). Consumed by Tasks 5–7.

- [ ] **Step 1: Add dependencies**

In `analysis/pyproject.toml`, extend `dependencies` and add a dev group:

```toml
dependencies = [
    "numpy>=2.2.6",
    "pandas>=2.3.3",
    "psycopg[binary]>=3.2",
    "python-dotenv>=1.0",
]

[dependency-groups]
dev = ["pytest>=8.0"]
```

Run: `cd analysis && uv sync`
Expected: resolves and installs psycopg, python-dotenv, pytest.

- [ ] **Step 2: Write failing tests**

`analysis/tests/test_config.py`:

```python
from datetime import datetime

from src.config import PPR_SF_12, SEASONS, week_ts


def test_segment_ppr_sf_12():
    assert PPR_SF_12.key == "ppr-sf-12"
    assert PPR_SF_12.ppr == 1.0
    assert PPR_SF_12.is_superflex is True
    assert PPR_SF_12.total_rosters == 12
    assert PPR_SF_12.league_type == "redraft"
    assert PPR_SF_12.draft_type == "snake"


def test_week_ts_2025():
    s = SEASONS["2025"]
    # 2025 kickoff Thu Sep 4; week 1 scores land 4 days later, Mon Sep 8
    assert week_ts(s, 1) == datetime(2025, 9, 8)
    assert week_ts(s, 2) == datetime(2025, 9, 15)


def test_seasons_have_2026():
    assert "2026" in SEASONS
    assert SEASONS["2026"].draft_date < SEASONS["2026"].season_start
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd analysis && uv run pytest tests/test_config.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'src.config'`

- [ ] **Step 4: Implement `src/config.py`**

```python
"""Segment and season configuration for the valuation pipeline."""

from dataclasses import dataclass
from datetime import date, datetime, timedelta


@dataclass(frozen=True)
class Segment:
    """A league segment: one scoring/roster format the model runs on."""

    key: str
    ppr: float
    is_superflex: bool
    total_rosters: int
    league_type: str = "redraft"
    draft_type: str = "snake"  # ADP only; auction pick_no isn't a draft position


PPR_SF_12 = Segment(key="ppr-sf-12", ppr=1.0, is_superflex=True, total_rosters=12)


@dataclass(frozen=True)
class SeasonDates:
    draft_date: date  # when the ADP belief is seeded (model clock start)
    season_start: date  # NFL week 1 kickoff (Thursday)
    score_lag_days: int = 4  # week W scores land ~this many days after kickoff


SEASONS: dict[str, SeasonDates] = {
    "2025": SeasonDates(draft_date=date(2025, 8, 25), season_start=date(2025, 9, 4)),
    "2026": SeasonDates(draft_date=date(2026, 8, 24), season_start=date(2026, 9, 10)),
}


def week_to_date(season: SeasonDates, week: int) -> date:
    return season.season_start + timedelta(days=(week - 1) * 7 + season.score_lag_days)


def week_ts(season: SeasonDates, week: int) -> datetime:
    return datetime.combine(week_to_date(season, week), datetime.min.time())
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd analysis && uv run pytest tests/test_config.py -v`
Expected: 3 passed.

- [ ] **Step 6: Commit**

```bash
git add analysis/pyproject.toml analysis/uv.lock analysis/src/config.py analysis/tests/
git commit -m "feat(analysis): deps, segment/season config"
```

---

### Task 3: Extract model into `src/valuation.py` with persistable state

**Files:**
- Create: `analysis/src/valuation.py` (moved from `analysis/main.py`)
- Modify: `analysis/main.py` (delete moved code; import instead)
- Modify: `analysis/src/models.py` (add `PlayerBeliefState`)
- Test: `analysis/tests/test_valuation_state.py`

**Interfaces:**
- Consumes: nothing new.
- Produces: `src.valuation` module exporting `V_TOP, RHO, curve, Belief, Valuator` plus everything `main.py`'s demo needs. Key signatures for later tasks:
  - `Valuator(start_ts: datetime)` — replaces the module-level `DRAFT_DATE` clock.
  - `Valuator.seed_from_adp(adp: pd.DataFrame)` — now **skips players already in `self.beliefs`** (idempotent; used to seed late-arriving ADP players on every run).
  - `Valuator.to_state() -> list[PlayerBeliefState]`
  - `Valuator.from_state(states: list[PlayerBeliefState], last_ts: datetime) -> Valuator` (classmethod)
  - `Valuator.advance(events)`, `Valuator.rankings()` — unchanged behavior.
  - `models.PlayerBeliefState(player_id, guess, var, games, cum_par, position, name)` (dataclass).

- [ ] **Step 1: Write failing tests**

`analysis/tests/test_valuation_state.py`:

```python
from datetime import datetime

import pandas as pd

from src.models import PlayerBeliefState
from src.valuation import Valuator

START = datetime(2025, 8, 25)


def _adp():
    return pd.DataFrame(
        [
            {"player_id": "p1", "player_name": "QB One", "position": "QB", "adp": 1.0},
            {"player_id": "p2", "player_name": "RB Two", "position": "RB", "adp": 5.0},
        ]
    )


def test_state_round_trip_preserves_rankings():
    v = Valuator(start_ts=START)
    v.seed_from_adp(_adp())
    v.apply_trade(["p1"], ["p2"])

    restored = Valuator.from_state(v.to_state(), last_ts=v.last_ts)
    assert restored.last_ts == v.last_ts
    orig = v.rankings()
    back = restored.rankings()
    pd.testing.assert_frame_equal(orig, back)


def test_seed_from_adp_skips_existing_players():
    v = Valuator(start_ts=START)
    v.seed_from_adp(_adp())
    before = v.beliefs["p1"].guess
    v.apply_trade(["p1"], ["p2"])  # moves p1 off its seed
    v.seed_from_adp(_adp())  # re-seeding must NOT reset p1
    assert v.beliefs["p1"].guess != before or v.beliefs["p1"].var != 1_500_000.0
    # and a brand-new player does get seeded
    new = _adp().assign(player_id=["p1", "p9"])
    v.seed_from_adp(new)
    assert "p9" in v.beliefs


def test_start_ts_sets_model_clock():
    v = Valuator(start_ts=START)
    assert v.last_ts == START


def test_belief_state_fields():
    s = PlayerBeliefState(
        player_id="p1", guess=100.0, var=5.0, games=2.0, cum_par=3.0,
        position="QB", name="QB One",
    )
    assert s.player_id == "p1"
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd analysis && uv run pytest tests/test_valuation_state.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'src.valuation'`

- [ ] **Step 3: Create `src/valuation.py` and slim `main.py`**

Move from `main.py` into `src/valuation.py`, **unchanged**: the CONFIG block constants (`V_TOP`, `LAMBDA_ADP`, `RHO_RANK`, `RHO`, `ADP_VAR`, `TRADE_VAR`, `WEEK_VAR_BASE`, `MAX_VAR`, `UNSEEN_VAR`, `PERF_DECAY`, `PERF_N_CAP`, `DRIFT_PER_DAY`, `REPL_RANK_BY_POS`), `curve()`, `Belief`, and `Valuator` with all its methods (`_fuse`, `_drift`, `_ensure`, `_age`, `apply_trade`, `apply_week`, `advance`, `rankings`). Do NOT move `DRAFT_DATE`, `SEASON_START`, `SCORE_LAG_DAYS`, `week_to_date` (superseded by `src/config.py`).

Apply exactly these deltas while moving:

1. Superflex-12-team tunables (replace the old values, keep the comments' spirit):

```python
# 12-team superflex starting guesses — tune against held-out trades later.
RHO_RANK = 160
RHO = V_TOP * math.exp(-LAMBDA_ADP * (RHO_RANK - 1))
...
REPL_RANK_BY_POS = {"QB": 24, "RB": 30, "WR": 36, "TE": 12, "DEF": 12, "K": 12}
```

2. Constructor takes the clock:

```python
class Valuator:
    def __init__(self, start_ts: datetime) -> None:
        self.beliefs: dict[str, Belief] = {}
        self.last_ts: datetime = start_ts
```

3. `seed_from_adp` skips known players:

```python
    def seed_from_adp(self, adp: pd.DataFrame) -> None:
        """adp columns: player_id, player_name, position, adp.
        Idempotent: players already tracked keep their current belief."""
        for row in adp.itertuples(index=False):
            if row.player_id in self.beliefs:
                continue
            self.beliefs[row.player_id] = Belief(
                guess=curve(row.adp),
                var=ADP_VAR,
                position=row.position,
                name=row.player_name,
            )
```

4. State round-trip methods on `Valuator`:

```python
    def to_state(self) -> list[PlayerBeliefState]:
        return [
            PlayerBeliefState(
                player_id=pid, guess=b.guess, var=b.var, games=b.games,
                cum_par=b.cum_par, position=b.position, name=b.name,
            )
            for pid, b in self.beliefs.items()
        ]

    @classmethod
    def from_state(
        cls, states: list[PlayerBeliefState], last_ts: datetime
    ) -> "Valuator":
        v = cls(start_ts=last_ts)
        for s in states:
            v.beliefs[s.player_id] = Belief(
                guess=s.guess, var=s.var, position=s.position or "DEFAULT",
                name=s.name or "", games=s.games, cum_par=s.cum_par,
            )
        return v
```

Add to `src/models.py` (keep the existing empty `TradeSide`/`AverageDraftPosition` for now — Task 4 replaces them):

```python
@dataclass
class PlayerBeliefState:
    player_id: str
    guess: float
    var: float
    games: float
    cum_par: float
    position: str
    name: str
```

In `main.py`: delete the moved code, add `from src.valuation import V_TOP, RHO, Belief, Valuator, curve`, keep a local season shim for the demo (`SEASON_2025 = SEASONS["2025"]` from `src.config`, and use `week_ts(SEASON_2025, week)` where the demo previously called `week_to_date`), and construct `Valuator(start_ts=datetime.combine(SEASONS["2025"].draft_date, datetime.min.time()))` in `main()`. The stale `from src.db import get_adp, get_trades` import and the `load_real` CSV path stay untouched for now (Task 7 rewrites `main.py` properly).

- [ ] **Step 4: Run tests + demo to verify**

Run: `cd analysis && uv run pytest tests/ -v`
Expected: all pass.

Run: `cd analysis && uv run python main.py --top 5`
Expected: demo table prints exactly as before the refactor (same seed, same numbers).

- [ ] **Step 5: Commit**

```bash
git add analysis/src/valuation.py analysis/src/models.py analysis/main.py analysis/tests/test_valuation_state.py
git commit -m "refactor(analysis): extract Valuator to src/valuation.py with persistable state"
```

---

### Task 4: Data models + trade-side parsing

**Files:**
- Modify: `analysis/src/models.py` (replace stubs)
- Create: `analysis/src/parsing.py`
- Test: `analysis/tests/test_trade_parsing.py`

**Interfaces:**
- Consumes: nothing new.
- Produces:
  - `models.AverageDraftPosition(player_id: str, player_name: str, position: str, adp: float)`
  - `models.Trade(trade_id: str, ts: datetime, side_a: list[str], side_b: list[str], created_ms: int)` — replaces the empty `TradeSide` stub (delete it).
  - `models.WeeklyScore(week: int, player_id: str, position: str, points: float)`
  - `models.RunState(segment: str, season: str, last_event_ts: datetime | None, last_transaction_created: int, last_week_processed: int)`
  - `parsing.parse_trade(trade_id, created_ms, adds, draft_picks, waiver_budget) -> Trade | None`
  - `parsing.ms_to_dt(ms: int) -> datetime` (naive UTC)

- [ ] **Step 1: Write failing tests**

`analysis/tests/test_trade_parsing.py`:

```python
from datetime import datetime

from src.parsing import ms_to_dt, parse_trade


def test_two_sided_trade_parses():
    # adds: player_id -> receiving roster_id
    t = parse_trade("t1", 1728000000000, {"pA": 3, "pB": 3, "pC": 7}, [], [])
    assert t is not None
    assert t.trade_id == "t1"
    # sides ordered by roster_id for determinism
    assert sorted(t.side_a) == ["pA", "pB"]
    assert t.side_b == ["pC"]
    assert t.ts == datetime(2024, 10, 4, 0, 0)  # 1728000000000 ms UTC
    assert t.created_ms == 1728000000000


def test_three_team_trade_skipped():
    assert parse_trade("t2", 0, {"pA": 1, "pB": 2, "pC": 3}, [], []) is None


def test_one_sided_skipped():
    assert parse_trade("t3", 0, {"pA": 1}, [], []) is None


def test_draft_picks_skipped():
    picks = [{"round": 1, "season": "2026"}]
    assert parse_trade("t4", 0, {"pA": 1, "pB": 2}, picks, []) is None


def test_faab_skipped():
    faab = [{"sender": 1, "receiver": 2, "amount": 20}]
    assert parse_trade("t5", 0, {"pA": 1, "pB": 2}, [], faab) is None


def test_null_adds_skipped():
    # jsonb null comes back as Python None
    assert parse_trade("t6", 0, None, [], []) is None
    assert parse_trade("t7", 0, {}, [], []) is None


def test_ms_to_dt_is_naive_utc():
    dt = ms_to_dt(1728000000000)
    assert dt == datetime(2024, 10, 4, 0, 0)
    assert dt.tzinfo is None
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd analysis && uv run pytest tests/test_trade_parsing.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'src.parsing'`

- [ ] **Step 3: Implement**

Replace `analysis/src/models.py` entirely:

```python
from dataclasses import dataclass
from datetime import datetime


@dataclass(frozen=True)
class AverageDraftPosition:
    player_id: str
    player_name: str
    position: str
    adp: float


@dataclass(frozen=True)
class Trade:
    trade_id: str
    ts: datetime  # naive UTC
    side_a: list[str]
    side_b: list[str]
    created_ms: int  # Sleeper created_at_sleeper, unix ms — the trade watermark


@dataclass(frozen=True)
class WeeklyScore:
    week: int
    player_id: str
    position: str
    points: float


@dataclass
class PlayerBeliefState:
    player_id: str
    guess: float
    var: float
    games: float
    cum_par: float
    position: str
    name: str


@dataclass
class RunState:
    segment: str
    season: str
    last_event_ts: datetime | None
    last_transaction_created: int
    last_week_processed: int
```

Create `analysis/src/parsing.py`:

```python
"""Pure transforms from Sleeper DB rows to model inputs (no DB access)."""

from datetime import datetime, timezone

from .models import Trade


def ms_to_dt(ms: int) -> datetime:
    """Unix milliseconds -> naive UTC datetime (the model's timeline)."""
    return datetime.fromtimestamp(ms / 1000.0, tz=timezone.utc).replace(tzinfo=None)


def parse_trade(
    trade_id: str,
    created_ms: int,
    adds: dict[str, int] | None,
    draft_picks: list | None,
    waiver_budget: list | None,
) -> Trade | None:
    """Build a two-sided player trade from a sleeper_transactions row.

    `adds` maps player_id -> receiving roster_id, which fully determines the
    sides. Returns None for anything the model can't value cleanly: trades
    with draft picks or FAAB attached, and trades not between exactly two
    rosters.
    """
    if draft_picks or waiver_budget or not adds:
        return None
    sides: dict[int, list[str]] = {}
    for player_id, roster_id in adds.items():
        sides.setdefault(int(roster_id), []).append(player_id)
    if len(sides) != 2:
        return None
    (_, side_a), (_, side_b) = sorted(sides.items())
    return Trade(
        trade_id=trade_id,
        ts=ms_to_dt(created_ms),
        side_a=side_a,
        side_b=side_b,
        created_ms=created_ms,
    )
```

Update the import in `src/db.py` stubs to keep the package importable:

```python
from .models import AverageDraftPosition, Trade


def get_adp() -> list[AverageDraftPosition]:
    return []


def get_trades() -> list[Trade]:
    return []
```

- [ ] **Step 4: Run all tests**

Run: `cd analysis && uv run pytest tests/ -v`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add analysis/src/models.py analysis/src/parsing.py analysis/src/db.py analysis/tests/test_trade_parsing.py
git commit -m "feat(analysis): data models and pure trade-side parsing"
```

---

### Task 5: DB layer — queries, state I/O, snapshots

**Files:**
- Rewrite: `analysis/src/db.py`
- Test: `analysis/tests/test_db_rows.py`

**Interfaces:**
- Consumes: `Segment` (Task 2), models + `parse_trade` (Task 4), `PlayerBeliefState` (Task 3).
- Produces (all take a `psycopg.Connection`; **no function commits — callers commit**):
  - `get_connection() -> psycopg.Connection`
  - `get_adp(conn, segment, season) -> list[AverageDraftPosition]`
  - `get_trades(conn, segment, season, since_created: int) -> list[Trade]` (sorted by `created_ms`)
  - `get_weekly_scores(conn, season, after_week: int) -> list[WeeklyScore]` (finalized weeks only)
  - `load_state(conn, segment_key) -> list[PlayerBeliefState]`
  - `save_state(conn, segment_key, states)` (full replace: DELETE + batch INSERT)
  - `get_run(conn, segment_key, season) -> RunState | None`
  - `save_run(conn, run: RunState)` (upsert; also stamps `last_run_at = now()`)
  - `write_snapshot(conn, segment_key, valuation_date: date, rankings: pd.DataFrame)` — `rankings` is `Valuator.rankings()` output (index = rank; columns `player_id, player, pos, value, vorp, sd, games`); upserts on `(segment, sleeper_player_id, valuation_date)`
  - `delete_snapshots(conn, segment_key, start: date, end: date)`
  - `rows_to_adp(rows)`, `rows_to_scores(rows)` — pure row transforms (unit-tested without a DB)

- [ ] **Step 1: Write failing tests** (pure transforms only — SQL is exercised in Task 7's live run)

`analysis/tests/test_db_rows.py`:

```python
from src.db import rows_to_adp, rows_to_scores


def test_rows_to_adp():
    rows = [("p1", "Josh Allen", "QB", 1.8), ("p2", "Bijan Robinson", "RB", 2.4)]
    adp = rows_to_adp(rows)
    assert adp[0].player_id == "p1"
    assert adp[0].player_name == "Josh Allen"
    assert adp[0].position == "QB"
    assert adp[0].adp == 1.8


def test_rows_to_scores():
    rows = [(1, "p1", "QB", 31.5), (1, "p2", "RB", 0.0)]
    scores = rows_to_scores(rows)
    assert scores[0].week == 1
    assert scores[0].points == 31.5
    assert scores[1].points == 0.0
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd analysis && uv run pytest tests/test_db_rows.py -v`
Expected: FAIL — `ImportError: cannot import name 'rows_to_adp'`

- [ ] **Step 3: Rewrite `src/db.py`**

```python
"""Postgres access for the valuation pipeline.

Connection/.env discovery follows workers/espn/db.py. No function here
commits; callers own the transaction so a run is all-or-nothing.
"""

import os
from datetime import date
from pathlib import Path

import pandas as pd
import psycopg
from dotenv import load_dotenv

from .config import Segment
from .models import (
    AverageDraftPosition,
    PlayerBeliefState,
    RunState,
    Trade,
    WeeklyScore,
)
from .parsing import parse_trade

# Walk up from this file to find backend/.env (repo-root/backend/.env)
_here = Path(__file__).parent
for _p in [_here, _here.parent, _here.parent.parent, _here.parent.parent.parent]:
    _env = _p / "backend" / ".env"
    if _env.exists():
        load_dotenv(_env, override=False)
        break
else:
    load_dotenv(override=False)

FANTASY_POSITIONS = ("QB", "RB", "WR", "TE", "K", "DEF")


def get_connection() -> psycopg.Connection:
    conn = psycopg.connect(os.environ["DATABASE_URL"])
    with conn.cursor() as cur:
        cur.execute("SET TIME ZONE 'UTC'")  # naive-UTC convention end-to-end
    return conn


# ---------------------------------------------------------------- inputs --


def rows_to_adp(rows) -> list[AverageDraftPosition]:
    return [
        AverageDraftPosition(
            player_id=r[0], player_name=r[1], position=r[2], adp=float(r[3])
        )
        for r in rows
    ]


def rows_to_scores(rows) -> list[WeeklyScore]:
    return [
        WeeklyScore(week=r[0], player_id=r[1], position=r[2], points=float(r[3]))
        for r in rows
    ]


def get_adp(
    conn: psycopg.Connection, segment: Segment, season: str
) -> list[AverageDraftPosition]:
    """Mean pick_no per player across the segment's completed snake drafts."""
    sql = """
        SELECT dp.sleeper_player_id, p.full_name, p.position,
               AVG(dp.pick_no) AS adp
        FROM sleeper_draft_picks dp
        JOIN sleeper_drafts d   ON d.sleeper_draft_id = dp.sleeper_draft_id
        JOIN sleeper_leagues l  ON l.sleeper_league_id = d.sleeper_league_id
        JOIN sleeper_players p  ON p.sleeper_player_id = dp.sleeper_player_id
        WHERE l.ppr = %s AND l.is_superflex = %s AND l.total_rosters = %s
          AND l.league_type = %s
          AND d.type = %s AND d.status = 'complete' AND d.season = %s
          AND p.position = ANY(%s)
        GROUP BY dp.sleeper_player_id, p.full_name, p.position
    """
    with conn.cursor() as cur:
        cur.execute(
            sql,
            (
                segment.ppr,
                segment.is_superflex,
                segment.total_rosters,
                segment.league_type,
                segment.draft_type,
                season,
                list(FANTASY_POSITIONS),
            ),
        )
        return rows_to_adp(cur.fetchall())


def get_trades(
    conn: psycopg.Connection, segment: Segment, season: str, since_created: int
) -> list[Trade]:
    """Completed two-sided player trades in segment leagues, past the watermark."""
    sql = """
        SELECT t.sleeper_transaction_id, t.created_at_sleeper,
               t.adds, t.draft_picks, t.waiver_budget
        FROM sleeper_transactions t
        JOIN sleeper_leagues l ON l.sleeper_league_id = t.sleeper_league_id
        WHERE t.type = 'trade' AND t.status = 'complete'
          AND l.ppr = %s AND l.is_superflex = %s AND l.total_rosters = %s
          AND l.league_type = %s AND l.season = %s
          AND t.created_at_sleeper > %s
        ORDER BY t.created_at_sleeper
    """
    with conn.cursor() as cur:
        cur.execute(
            sql,
            (
                segment.ppr,
                segment.is_superflex,
                segment.total_rosters,
                segment.league_type,
                season,
                since_created,
            ),
        )
        rows = cur.fetchall()
    trades = [parse_trade(r[0], r[1], r[2], r[3], r[4]) for r in rows]
    return [t for t in trades if t is not None]


def get_weekly_scores(
    conn: psycopg.Connection, season: str, after_week: int
) -> list[WeeklyScore]:
    """PPR points for finalized weeks after the watermark (all NFL players)."""
    sql = """
        SELECT s.week, s.sleeper_player_id, p.position, s.pts_ppr
        FROM sleeper_player_week_stats s
        JOIN sleeper_week_stat_fetches f
             ON f.season = s.season AND f.week = s.week AND f.finalized
        JOIN sleeper_players p ON p.sleeper_player_id = s.sleeper_player_id
        WHERE s.season = %s AND s.week > %s AND s.pts_ppr IS NOT NULL
          AND p.position = ANY(%s)
        ORDER BY s.week
    """
    with conn.cursor() as cur:
        cur.execute(sql, (season, after_week, list(FANTASY_POSITIONS)))
        return rows_to_scores(cur.fetchall())


# --------------------------------------------------------- state + output --


def load_state(conn: psycopg.Connection, segment_key: str) -> list[PlayerBeliefState]:
    sql = """
        SELECT sleeper_player_id, guess, var, games, cum_par, position, name
        FROM valuation_state WHERE segment = %s
    """
    with conn.cursor() as cur:
        cur.execute(sql, (segment_key,))
        return [
            PlayerBeliefState(
                player_id=r[0], guess=r[1], var=r[2], games=r[3],
                cum_par=r[4], position=r[5] or "DEFAULT", name=r[6] or "",
            )
            for r in cur.fetchall()
        ]


def save_state(
    conn: psycopg.Connection, segment_key: str, states: list[PlayerBeliefState]
) -> None:
    """Full replace: the in-memory Valuator is the source of truth."""
    with conn.cursor() as cur:
        cur.execute("DELETE FROM valuation_state WHERE segment = %s", (segment_key,))
        cur.executemany(
            """
            INSERT INTO valuation_state
                (segment, sleeper_player_id, guess, var, games, cum_par,
                 position, name, updated_at)
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s, now())
            """,
            [
                (segment_key, s.player_id, s.guess, s.var, s.games, s.cum_par,
                 s.position, s.name)
                for s in states
            ],
        )


def get_run(
    conn: psycopg.Connection, segment_key: str, season: str
) -> RunState | None:
    sql = """
        SELECT last_event_ts, last_transaction_created, last_week_processed
        FROM valuation_runs WHERE segment = %s AND season = %s
    """
    with conn.cursor() as cur:
        cur.execute(sql, (segment_key, season))
        row = cur.fetchone()
    if row is None:
        return None
    last_event_ts = row[0].replace(tzinfo=None) if row[0] is not None else None
    return RunState(
        segment=segment_key, season=season, last_event_ts=last_event_ts,
        last_transaction_created=row[1], last_week_processed=row[2],
    )


def save_run(conn: psycopg.Connection, run: RunState) -> None:
    sql = """
        INSERT INTO valuation_runs
            (segment, season, last_event_ts, last_transaction_created,
             last_week_processed, last_run_at)
        VALUES (%s, %s, %s, %s, %s, now())
        ON CONFLICT (segment, season) DO UPDATE SET
            last_event_ts = EXCLUDED.last_event_ts,
            last_transaction_created = EXCLUDED.last_transaction_created,
            last_week_processed = EXCLUDED.last_week_processed,
            last_run_at = now()
    """
    with conn.cursor() as cur:
        cur.execute(
            sql,
            (run.segment, run.season, run.last_event_ts,
             run.last_transaction_created, run.last_week_processed),
        )


def write_snapshot(
    conn: psycopg.Connection,
    segment_key: str,
    valuation_date: date,
    rankings: pd.DataFrame,
) -> None:
    """rankings = Valuator.rankings(): index is rank, columns include
    player_id, pos, value, vorp, sd, games."""
    sql = """
        INSERT INTO player_valuations
            (segment, sleeper_player_id, valuation_date, rank, value, vorp,
             sd, games, position)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
        ON CONFLICT (segment, sleeper_player_id, valuation_date) DO UPDATE SET
            rank = EXCLUDED.rank, value = EXCLUDED.value,
            vorp = EXCLUDED.vorp, sd = EXCLUDED.sd,
            games = EXCLUDED.games, position = EXCLUDED.position
    """
    with conn.cursor() as cur:
        cur.executemany(
            sql,
            [
                (segment_key, row.player_id, valuation_date, rank,
                 float(row.value), float(row.vorp), float(row.sd),
                 float(row.games), row.pos)
                for rank, row in zip(rankings.index, rankings.itertuples(index=False))
            ],
        )


def delete_snapshots(
    conn: psycopg.Connection, segment_key: str, start: date, end: date
) -> None:
    with conn.cursor() as cur:
        cur.execute(
            """
            DELETE FROM player_valuations
            WHERE segment = %s AND valuation_date BETWEEN %s AND %s
            """,
            (segment_key, start, end),
        )
```

Note: seeded ADP players are never dropped from `rankings()`, and every player in a snapshot was either drafted, traded, or scored — all of which exist in `sleeper_players` — so the `player_valuations.sleeper_player_id` FK holds.

- [ ] **Step 4: Run all tests**

Run: `cd analysis && uv run pytest tests/ -v`
Expected: all pass (new file imports cleanly; transforms tested).

- [ ] **Step 5: Commit**

```bash
git add analysis/src/db.py analysis/tests/test_db_rows.py
git commit -m "feat(analysis): DB queries, belief-state I/O, valuation snapshots"
```

---

### Task 6: Runner — event building, backtest, incremental advance

**Files:**
- Create: `analysis/src/runner.py`
- Test: `analysis/tests/test_runner.py`

**Interfaces:**
- Consumes: `Valuator` (Task 3), models (Task 4), `SeasonDates`/`week_ts` (Task 2).
- Produces:
  - `adp_frame(adp: list[AverageDraftPosition]) -> pd.DataFrame` (columns `player_id, player_name, position, adp`; empty-safe)
  - `build_events(trades: list[Trade], scores: list[WeeklyScore], season: SeasonDates) -> list[dict]` — the `Valuator.advance` event dicts, sorted by `ts`
  - `run_backtest(valuator, events, on_snapshot: Callable[[date, pd.DataFrame], None]) -> None` — advances day-by-day, calling `on_snapshot(day, rankings)` after each event day
  - `filter_stale(events, last_event_ts: datetime | None) -> tuple[list[dict], int]` — (fresh events, skipped count)

No DB access in this module — `main.py` wires data in and snapshots out, which is what makes this testable.

- [ ] **Step 1: Write failing tests**

`analysis/tests/test_runner.py`:

```python
from datetime import datetime

from src.config import SEASONS
from src.models import Trade, WeeklyScore
from src.runner import build_events, filter_stale, run_backtest
from src.valuation import Valuator

S2025 = SEASONS["2025"]


def _trade(ts: datetime, a: str, b: str) -> Trade:
    return Trade(
        trade_id=f"{a}-{b}", ts=ts, side_a=[a], side_b=[b],
        created_ms=int(ts.timestamp() * 1000),
    )


def _scores(week: int) -> list[WeeklyScore]:
    return [
        WeeklyScore(week=week, player_id="p1", position="QB", points=25.0),
        WeeklyScore(week=week, player_id="p2", position="RB", points=12.0),
    ]


def test_build_events_shapes_and_order():
    trades = [_trade(datetime(2025, 9, 20), "p1", "p2")]
    events = build_events(trades, _scores(1) + _scores(2), S2025)
    kinds = [e["kind"] for e in events]
    # week 1 lands Sep 8, trade Sep 20, week 2 lands Sep 15 -> sorted by ts
    assert kinds == ["week", "week", "trade"]
    assert events[0]["ts"] == datetime(2025, 9, 8)
    assert list(events[0]["scores"].columns) == ["player_id", "position", "points"]
    assert events[2]["side_a"] == ["p1"]


def test_filter_stale():
    trades = [
        _trade(datetime(2025, 9, 10), "p1", "p2"),
        _trade(datetime(2025, 9, 30), "p1", "p2"),
    ]
    events = build_events(trades, [], S2025)
    fresh, skipped = filter_stale(events, datetime(2025, 9, 15))
    assert skipped == 1
    assert len(fresh) == 1 and fresh[0]["ts"] == datetime(2025, 9, 30)
    # None watermark keeps everything
    fresh, skipped = filter_stale(events, None)
    assert (len(fresh), skipped) == (2, 0)


def test_run_backtest_snapshots_per_event_day():
    import pandas as pd

    adp = pd.DataFrame(
        [
            {"player_id": "p1", "player_name": "A", "position": "QB", "adp": 1.0},
            {"player_id": "p2", "player_name": "B", "position": "RB", "adp": 10.0},
        ]
    )
    v = Valuator(start_ts=datetime(2025, 8, 25))
    v.seed_from_adp(adp)
    trades = [
        _trade(datetime(2025, 9, 10, 14, 0), "p1", "p2"),
        _trade(datetime(2025, 9, 10, 18, 0), "p1", "p2"),  # same day
        _trade(datetime(2025, 9, 12, 9, 0), "p1", "p2"),
    ]
    events = build_events(trades, _scores(1), S2025)

    snaps: list = []
    run_backtest(v, events, on_snapshot=lambda d, df: snaps.append((d, len(df))))

    # 3 distinct event days: Sep 8 (week 1), Sep 10 (two trades), Sep 12
    from datetime import date

    assert [d for d, _ in snaps] == [
        date(2025, 9, 8), date(2025, 9, 10), date(2025, 9, 12),
    ]
    assert all(n >= 2 for _, n in snaps)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd analysis && uv run pytest tests/test_runner.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'src.runner'`

- [ ] **Step 3: Implement `src/runner.py`**

```python
"""Orchestration: turn DB rows into model events and drive the Valuator.

No DB access here — main.py wires data in and snapshots out.
"""

from collections.abc import Callable
from datetime import date, datetime
from itertools import groupby

import pandas as pd

from .config import SeasonDates, week_ts
from .models import AverageDraftPosition, Trade, WeeklyScore
from .valuation import Valuator

ADP_COLUMNS = ["player_id", "player_name", "position", "adp"]


def adp_frame(adp: list[AverageDraftPosition]) -> pd.DataFrame:
    if not adp:
        return pd.DataFrame(columns=ADP_COLUMNS)
    return pd.DataFrame(
        [(a.player_id, a.player_name, a.position, a.adp) for a in adp],
        columns=ADP_COLUMNS,
    )


def build_events(
    trades: list[Trade], scores: list[WeeklyScore], season: SeasonDates
) -> list[dict]:
    """Valuator.advance() event dicts, sorted by timestamp."""
    events: list[dict] = [
        {"ts": t.ts, "kind": "trade", "side_a": t.side_a, "side_b": t.side_b}
        for t in trades
    ]
    by_week: dict[int, list[WeeklyScore]] = {}
    for s in scores:
        by_week.setdefault(s.week, []).append(s)
    for week, wk_scores in by_week.items():
        events.append(
            {
                "ts": week_ts(season, week),
                "kind": "week",
                "scores": pd.DataFrame(
                    [(s.player_id, s.position, s.points) for s in wk_scores],
                    columns=["player_id", "position", "points"],
                ),
            }
        )
    events.sort(key=lambda e: e["ts"])
    return events


def filter_stale(
    events: list[dict], last_event_ts: datetime | None
) -> tuple[list[dict], int]:
    """Drop events at or before the model clock (out-of-order arrivals)."""
    if last_event_ts is None:
        return events, 0
    fresh = [e for e in events if e["ts"] > last_event_ts]
    return fresh, len(events) - len(fresh)


def run_backtest(
    valuator: Valuator,
    events: list[dict],
    on_snapshot: Callable[[date, pd.DataFrame], None],
) -> None:
    """Replay a season as if live: advance one event-day at a time and emit a
    valuation snapshot after each day that had events. Aging between days
    changes only uncertainty (sd), not value, so event days are the complete
    set of days the value series can move."""
    events = sorted(events, key=lambda e: e["ts"])
    for day, day_events in groupby(events, key=lambda e: e["ts"].date()):
        valuator.advance(list(day_events))
        on_snapshot(day, valuator.rankings())
```

- [ ] **Step 4: Run all tests**

Run: `cd analysis && uv run pytest tests/ -v`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add analysis/src/runner.py analysis/tests/test_runner.py
git commit -m "feat(analysis): event building, backtest replay, stale-event guard"
```

---

### Task 7: CLI wiring + live 2025 backtest validation

**Files:**
- Modify: `analysis/main.py` (replace `load_real` and `main()`; keep demo)
- Modify: `analysis/README.md` (usage)

**Interfaces:**
- Consumes: everything above. This is the glue + the real-data validation gate.

- [ ] **Step 1: Rewrite `main.py`'s entry points**

Delete `load_real()` (CSV mode is superseded by the DB) and the `from src.db import get_adp, get_trades` stale import. Keep `make_demo()` (updating its helpers to use `src.config`/`src.valuation` imports from Task 3). Replace `main()`:

```python
import argparse
import sys
from datetime import date, datetime, timedelta

from src import db
from src.config import PPR_SF_12, SEASONS
from src.models import RunState
from src.runner import adp_frame, build_events, filter_stale, run_backtest
from src.valuation import RHO, V_TOP, Valuator


def run_demo(top: int) -> None:
    adp, events = make_demo()
    v = Valuator(start_ts=datetime.combine(SEASONS["2025"].draft_date, datetime.min.time()))
    v.seed_from_adp(adp)
    v.advance(events)
    _print_rankings(v, top, "built-in demo data")


def _print_rankings(v: Valuator, top: int, source: str) -> None:
    print(f"\nPlayer valuations  ({source})")
    print(f"ρ (replacement) = {RHO:.0f}   |   top of curve = {V_TOP:.0f}\n")
    print(v.rankings().head(top).to_string())
    print(
        "\nvalue = current belief (additive scale) | vorp = value - ρ"
        " | sd = uncertainty band\n"
    )


def run_db(season: str, backtest: bool, top: int) -> None:
    segment = PPR_SF_12
    season_dates = SEASONS[season]
    conn = db.get_connection()
    try:
        run = db.get_run(conn, segment.key, season)
        state = db.load_state(conn, segment.key)
        bootstrap = backtest or run is None or not state

        if bootstrap:
            print(f"[{segment.key}/{season}] full backtest replay")
            adp = db.get_adp(conn, segment, season)
            trades = db.get_trades(conn, segment, season, since_created=0)
            scores = db.get_weekly_scores(conn, season, after_week=0)
            print(
                f"  inputs: {len(adp)} ADP players, {len(trades)} trades,"
                f" {len(scores)} weekly score rows"
            )
            if not adp:
                sys.exit("no ADP data for this segment/season — nothing to seed")

            v = Valuator(
                start_ts=datetime.combine(season_dates.draft_date, datetime.min.time())
            )
            v.seed_from_adp(adp_frame(adp))
            events = build_events(trades, scores, season_dates)

            # rewrite the season's snapshot range (rerunnable as backlog lands)
            db.delete_snapshots(
                conn, segment.key,
                season_dates.draft_date,
                season_dates.draft_date + timedelta(days=365),
            )
            snap_count = 0

            def on_snapshot(day: date, rankings) -> None:
                nonlocal snap_count
                db.write_snapshot(conn, segment.key, day, rankings)
                snap_count += 1

            run_backtest(v, events, on_snapshot)
            print(f"  wrote {snap_count} daily snapshots")
        else:
            print(f"[{segment.key}/{season}] incremental run")
            adp = db.get_adp(conn, segment, season)
            trades = db.get_trades(
                conn, segment, season, since_created=run.last_transaction_created
            )
            scores = db.get_weekly_scores(
                conn, season, after_week=run.last_week_processed
            )
            v = Valuator.from_state(state, last_ts=run.last_event_ts)
            v.seed_from_adp(adp_frame(adp))  # late-arriving draftees only
            events = build_events(trades, scores, season_dates)
            fresh, skipped = filter_stale(events, run.last_event_ts)
            if skipped:
                print(
                    f"  WARNING: skipped {skipped} events at/before model clock"
                    f" {run.last_event_ts} (out-of-order arrivals)"
                )
            print(f"  applying {len(fresh)} new events")
            v.advance(fresh)
            db.write_snapshot(conn, segment.key, date.today(), v.rankings())

        max_created = max(
            [t.created_ms for t in trades],
            default=(0 if bootstrap else run.last_transaction_created),
        )
        max_week = max(
            [s.week for s in scores],
            default=(0 if bootstrap else run.last_week_processed),
        )
        db.save_state(conn, segment.key, v.to_state())
        db.save_run(
            conn,
            RunState(
                segment=segment.key, season=season, last_event_ts=v.last_ts,
                last_transaction_created=max_created, last_week_processed=max_week,
            ),
        )
        conn.commit()
        _print_rankings(v, top, f"{segment.key} season {season} (database)")
    except Exception:
        conn.rollback()
        raise
    finally:
        conn.close()


def main() -> None:
    ap = argparse.ArgumentParser(description="Single-segment player valuation.")
    ap.add_argument("--demo", action="store_true", help="run on synthetic demo data")
    ap.add_argument("--backtest", action="store_true",
                    help="full season replay, rewriting all dated snapshots")
    ap.add_argument("--season", default="2025", choices=sorted(SEASONS))
    ap.add_argument("--top", type=int, default=30, help="how many players to print")
    args = ap.parse_args()

    if args.demo:
        run_demo(args.top)
    else:
        run_db(args.season, args.backtest, args.top)


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Verify demo + tests still pass**

Run: `cd analysis && uv run pytest tests/ -v && uv run python main.py --demo --top 5`
Expected: all tests pass; demo table prints.

- [ ] **Step 3: Run the real 2025 backtest**

Run: `cd analysis && uv run python main.py --season 2025 --backtest --top 30`
Expected: input counts print (thousands of trades, ~thousands of score rows), snapshot count in the hundreds (one per event day — nearly every calendar day in-season), and a top-30 table. **Sanity gates:** QBs cluster near the top (superflex); the top 30 contains recognizable 2025 studs; no player with `games == 0` outranks everyone.

- [ ] **Step 4: Verify persistence + incremental rerun**

```bash
cd backend && export $(grep -E '^DATABASE_URL=' .env | head -1) && psql "$DATABASE_URL" \
  -c "SELECT count(DISTINCT valuation_date) AS days, count(*) AS rows FROM player_valuations WHERE segment='ppr-sf-12';" \
  -c "SELECT valuation_date, rank, position, value FROM player_valuations WHERE segment='ppr-sf-12' AND rank <= 3 ORDER BY valuation_date DESC, rank LIMIT 9;" \
  -c "SELECT * FROM valuation_runs;"
```

Expected: `days` matches the printed snapshot count; recent top-3 look sane; `valuation_runs` has the watermarks.

Then: `cd analysis && uv run python main.py --season 2025 --top 5`
Expected: takes the **incremental** path, applies 0 new events (season over, watermarks current), writes today's snapshot, prints the same top players as the backtest.

- [ ] **Step 5: Spot-check the time series**

```bash
psql "$DATABASE_URL" -c "
SELECT valuation_date, rank, value, sd
FROM player_valuations
WHERE segment='ppr-sf-12'
  AND sleeper_player_id = (
    SELECT sleeper_player_id FROM player_valuations
    WHERE segment='ppr-sf-12' ORDER BY valuation_date DESC, rank LIMIT 1)
  AND EXTRACT(day FROM valuation_date) IN (1, 15)
ORDER BY valuation_date;"
```

Expected: the eventual #1 player's value **rises over the season** while `sd` shrinks as games accumulate — the model learning. If the curve is flat or erratic, flag it for tuning rather than "fixing" constants ad hoc.

- [ ] **Step 6: Update `analysis/README.md`**

Replace its contents with current usage:

````markdown
# analysis

Bayesian player valuation on real Sleeper data. See
`docs/superpowers/specs/2026-07-02-player-valuation-sleeper-data-design.md`.

```bash
uv sync
uv run pytest                                  # unit tests (no DB needed)
uv run python main.py --demo                   # synthetic data
uv run python main.py --season 2025 --backtest # full replay, rewrites snapshots
uv run python main.py --season 2025            # incremental run (default mode)
```

Reads `DATABASE_URL` from `backend/.env`. Segment v1 is `ppr-sf-12`
(full PPR, superflex, 12-team, redraft) — see `src/config.py`.
Outputs land in `player_valuations` (dated snapshots), `valuation_state`
(beliefs), `valuation_runs` (watermarks).
````

- [ ] **Step 7: Commit**

```bash
git add analysis/main.py analysis/README.md
git commit -m "feat(analysis): DB-backed CLI with backtest and incremental modes"
```
