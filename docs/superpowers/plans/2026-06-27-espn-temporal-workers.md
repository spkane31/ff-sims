# ESPN Temporal Workers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate `scripts/main.py` ESPN data fetching into Python Temporal workers at `v2/workers/espn/`, writing directly to PostgreSQL and running inside the `v2/` Dockerfile alongside the existing Go Sleeper workers.

**Architecture:** A Python Temporal worker registers on the `espn-sync` task queue against the same Temporal Cloud namespace as the Go workers. `ESPNSyncDispatcher` fires on a weekly schedule (Tuesday 13:00 UTC / 8 AM EST) and spawns: one global `ESPNPlayerStatusSyncWorkflow` (runs once per cycle) plus one `LeagueESPNSyncWorkflow` per ESPN league (which fans out four parallel child workflows: teams, schedule, drafts, transactions). All activities write directly to PostgreSQL via psycopg — no intermediate JSON files.

**Tech Stack:** Python 3.12, `temporalio>=1.9.0`, `espn-api>=0.33.0`, `psycopg[binary]>=3.1.0`, `python-dotenv>=1.0.0`, `pytest`, `uv`

## Global Constraints

- Python >= 3.12; all activity functions are **sync** — `worker.py` must pass `activity_executor=ThreadPoolExecutor()`
- No DB mocks in tests — use `TEST_DATABASE_URL` (falls back to `DATABASE_URL`) pointing at a real test database
- Workflow files must import activities with `workflow.unsafe.imports_passed_through()` — never import directly at module level in a workflow file
- All DB writes use **check-before-act** for activity retry idempotency (query first, insert/update only if needed)
- Internal ID resolution: leagues via `(external_id=<espn_league_id>, platform='ESPN')`, teams via `(espn_id, league_id)`
- Task queue: `espn-sync` | Schedule ID: `espn-sync-schedule`
- **Manual historical backfill:** When adding a new league, next Tuesday's run syncs the current year automatically. For prior seasons, trigger manually:
  ```bash
  temporal workflow start \
    --type ESPNSyncDispatcher \
    --task-queue espn-sync \
    --workflow-id "espn-backfill-2023" \
    --input '{"year": 2023}'
  # Or for a single league:
  temporal workflow start \
    --type LeagueESPNSyncWorkflow \
    --task-queue espn-sync \
    --workflow-id "espn-league-backfill-345674-2022" \
    --input '{"espn_league_id": "345674", "year": 2022}'
  ```

---

### Task 1: Database Migration

**Files:**
- Create: `v2/backend/migrations/011_add_espn_league_credentials.sql`

**Interfaces:**
- Produces: `espn_league_credentials` table — columns `espn_league_id TEXT PK`, `espn_s2 TEXT`, `swid TEXT`, five `last_*_fetched_at TIMESTAMPTZ` columns, `created_at`, `updated_at`

- [ ] **Step 1: Write the migration file**

```sql
-- v2/backend/migrations/011_add_espn_league_credentials.sql
-- +goose Up

CREATE TABLE IF NOT EXISTS espn_league_credentials (
    espn_league_id               TEXT PRIMARY KEY,
    espn_s2                      TEXT NOT NULL,
    swid                         TEXT NOT NULL,
    last_teams_fetched_at        TIMESTAMPTZ,
    last_schedule_fetched_at     TIMESTAMPTZ,
    last_draft_fetched_at        TIMESTAMPTZ,
    last_transactions_fetched_at TIMESTAMPTZ,
    last_players_updated_at      TIMESTAMPTZ,
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS espn_league_credentials;
```

- [ ] **Step 2: Run the migration**

```bash
# From v2/backend/
go run ./cmd/migrate
```
Expected: `Applied 011_add_espn_league_credentials.sql`

- [ ] **Step 3: Verify the table**

```bash
psql $DATABASE_URL -c "\d espn_league_credentials"
```
Expected: all nine columns listed.

- [ ] **Step 4: Commit**

```bash
git add v2/backend/migrations/011_add_espn_league_credentials.sql
git commit -m "feat(espn): add espn_league_credentials migration"
```

---

### Task 2: Python Project Scaffolding

**Files:**
- Create: `v2/workers/espn/pyproject.toml`
- Create: `v2/workers/espn/db.py`
- Create: `v2/workers/espn/workflows/__init__.py`
- Create: `v2/workers/espn/activities/__init__.py`
- Create: `v2/workers/espn/tests/__init__.py`
- Create: `v2/workers/espn/tests/conftest.py`

**Interfaces:**
- Produces: `get_connection() -> psycopg.Connection` and `resolve_league_id(conn, espn_league_id: str) -> int` from `db.py`
- Produces: `db_conn` fixture from `tests/conftest.py`

- [ ] **Step 1: Create pyproject.toml**

```toml
[project]
name = "espn-temporal-worker"
version = "0.1.0"
requires-python = ">=3.12"
dependencies = [
    "temporalio>=1.9.0",
    "espn-api>=0.33.0",
    "psycopg[binary]>=3.1.0",
    "python-dotenv>=1.0.0",
]

[dependency-groups]
dev = [
    "pytest>=8.0.0",
    "pytest-asyncio>=0.23.0",
]

[tool.pytest.ini_options]
asyncio_mode = "auto"
testpaths = ["tests"]
```

- [ ] **Step 2: Install dependencies and lock**

```bash
cd v2/workers/espn
uv sync
```
Expected: `uv.lock` created, `.venv/` created.

- [ ] **Step 3: Create directory structure**

```bash
mkdir -p v2/workers/espn/workflows v2/workers/espn/activities v2/workers/espn/tests
touch v2/workers/espn/workflows/__init__.py
touch v2/workers/espn/activities/__init__.py
touch v2/workers/espn/tests/__init__.py
```

- [ ] **Step 4: Write db.py**

```python
# v2/workers/espn/db.py
import os
import psycopg
from dotenv import load_dotenv

load_dotenv()


def get_connection() -> psycopg.Connection:
    return psycopg.connect(os.environ["DATABASE_URL"])


def resolve_league_id(conn: psycopg.Connection, espn_league_id: str) -> int:
    """Return the internal leagues.id for an ESPN league's external_id."""
    with conn.cursor() as cur:
        cur.execute(
            "SELECT id FROM leagues WHERE external_id = %s AND platform = 'ESPN'",
            (espn_league_id,),
        )
        row = cur.fetchone()
    if row is None:
        raise ValueError(f"No ESPN league found with external_id={espn_league_id}")
    return row[0]


def resolve_team_id(cur: psycopg.Cursor, espn_team_id: int, league_id: int) -> int | None:
    """Return the internal teams.id for an ESPN team within a league, or None if not found."""
    cur.execute(
        "SELECT id FROM teams WHERE espn_id = %s AND league_id = %s",
        (espn_team_id, league_id),
    )
    row = cur.fetchone()
    return row[0] if row else None
```

- [ ] **Step 5: Write tests/conftest.py**

```python
# v2/workers/espn/tests/conftest.py
import os
import pytest
import psycopg
from dotenv import load_dotenv

load_dotenv()


@pytest.fixture
def db_conn():
    url = os.environ.get("TEST_DATABASE_URL", os.environ["DATABASE_URL"])
    conn = psycopg.connect(url)
    yield conn
    conn.rollback()
    conn.close()
```

- [ ] **Step 6: Verify imports work**

```bash
cd v2/workers/espn
uv run python -c "import psycopg; import temporalio; from espn_api.football import League; print('OK')"
```
Expected: `OK`

- [ ] **Step 7: Commit**

```bash
git add v2/workers/espn/
git commit -m "feat(espn): scaffold Python Temporal worker project"
```

---

### Task 3: Credentials Activities

**Files:**
- Create: `v2/workers/espn/activities/credentials.py`
- Create: `v2/workers/espn/tests/test_credentials.py`

**Interfaces:**
- Produces:
  - `@dataclass ESPNCredentials(espn_s2: str, swid: str)`
  - `@dataclass AnyESPNCredentials(espn_league_id: str, espn_s2: str, swid: str)`
  - `@activity.defn get_espn_leagues(year: int) -> list[str]`
  - `@activity.defn get_espn_credentials(espn_league_id: str) -> ESPNCredentials`
  - `@activity.defn get_any_espn_credentials() -> AnyESPNCredentials`

- [ ] **Step 1: Write the failing tests**

```python
# v2/workers/espn/tests/test_credentials.py
import psycopg
import pytest
from activities.credentials import (
    AnyESPNCredentials,
    ESPNCredentials,
    get_any_espn_credentials,
    get_espn_credentials,
    get_espn_leagues,
)


def _seed_credentials(conn: psycopg.Connection, espn_league_id: str, espn_s2: str = "s2val", swid: str = "swidval") -> None:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO espn_league_credentials (espn_league_id, espn_s2, swid) "
            "VALUES (%s, %s, %s) ON CONFLICT (espn_league_id) DO UPDATE SET espn_s2 = EXCLUDED.espn_s2, swid = EXCLUDED.swid",
            (espn_league_id, espn_s2, swid),
        )
    conn.commit()


def test_get_espn_leagues_returns_all_rows(db_conn):
    _seed_credentials(db_conn, "111")
    _seed_credentials(db_conn, "222")
    result = get_espn_leagues.__wrapped__(2025)
    assert "111" in result
    assert "222" in result


def test_get_espn_credentials_returns_matching_row(db_conn):
    _seed_credentials(db_conn, "333", espn_s2="myS2", swid="mySWID")
    creds = get_espn_credentials.__wrapped__("333")
    assert isinstance(creds, ESPNCredentials)
    assert creds.espn_s2 == "myS2"
    assert creds.swid == "mySWID"


def test_get_espn_credentials_raises_for_missing(db_conn):
    with pytest.raises(ValueError, match="No credentials found"):
        get_espn_credentials.__wrapped__("nonexistent-99")


def test_get_any_espn_credentials_returns_a_row(db_conn):
    _seed_credentials(db_conn, "444", espn_s2="anyS2", swid="anySWID")
    creds = get_any_espn_credentials.__wrapped__()
    assert isinstance(creds, AnyESPNCredentials)
    assert creds.espn_league_id is not None
    assert creds.espn_s2 is not None
    assert creds.swid is not None
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd v2/workers/espn
uv run pytest tests/test_credentials.py -v
```
Expected: `ImportError` — `activities.credentials` not defined yet.

- [ ] **Step 3: Write activities/credentials.py**

```python
# v2/workers/espn/activities/credentials.py
from dataclasses import dataclass
from temporalio import activity
from db import get_connection


@dataclass
class ESPNCredentials:
    espn_s2: str
    swid: str


@dataclass
class AnyESPNCredentials:
    espn_league_id: str
    espn_s2: str
    swid: str


@activity.defn
def get_espn_leagues(year: int) -> list[str]:
    """Return all ESPN league IDs that have credentials registered."""
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute("SELECT espn_league_id FROM espn_league_credentials ORDER BY espn_league_id")
            return [row[0] for row in cur.fetchall()]


@activity.defn
def get_espn_credentials(espn_league_id: str) -> ESPNCredentials:
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(
                "SELECT espn_s2, swid FROM espn_league_credentials WHERE espn_league_id = %s",
                (espn_league_id,),
            )
            row = cur.fetchone()
    if row is None:
        raise ValueError(f"No credentials found for ESPN league {espn_league_id}")
    return ESPNCredentials(espn_s2=row[0], swid=row[1])


@activity.defn
def get_any_espn_credentials() -> AnyESPNCredentials:
    """Return credentials from any registered ESPN league (used for global player status updates)."""
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute("SELECT espn_league_id, espn_s2, swid FROM espn_league_credentials LIMIT 1")
            row = cur.fetchone()
    if row is None:
        raise ValueError("No ESPN credentials found in the database")
    return AnyESPNCredentials(espn_league_id=row[0], espn_s2=row[1], swid=row[2])
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd v2/workers/espn
uv run pytest tests/test_credentials.py -v
```
Expected: 4 tests pass. Note: `@activity.defn` wraps the function; call the underlying function in tests via `.__wrapped__`.

- [ ] **Step 5: Commit**

```bash
git add v2/workers/espn/activities/credentials.py v2/workers/espn/tests/test_credentials.py
git commit -m "feat(espn): add credentials activities"
```

---

### Task 4: Teams Workflow

**Files:**
- Create: `v2/workers/espn/activities/teams.py`
- Create: `v2/workers/espn/workflows/teams.py`
- Create: `v2/workers/espn/tests/test_teams.py`

**Interfaces:**
- Produces:
  - `@dataclass ESPNLeagueSyncParams(espn_league_id: str, year: int, espn_s2: str, swid: str)`
  - `@activity.defn fetch_and_upsert_teams(params: ESPNLeagueSyncParams) -> None`
  - `@activity.defn mark_teams_fetched(espn_league_id: str) -> None`
  - `@workflow.defn class ESPNTeamSyncWorkflow` with `run(self, params: ESPNLeagueSyncParams) -> None`

- [ ] **Step 1: Write the failing tests**

```python
# v2/workers/espn/tests/test_teams.py
from unittest.mock import MagicMock, patch

import psycopg
import pytest

from activities.teams import ESPNLeagueSyncParams, fetch_and_upsert_teams, mark_teams_fetched


def _seed_league(conn: psycopg.Connection, external_id: str) -> int:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO leagues (name, platform, external_id) VALUES ('Test League', 'ESPN', %s) "
            "ON CONFLICT (platform, external_id) WHERE platform != '' AND external_id != '' "
            "DO UPDATE SET name = EXCLUDED.name RETURNING id",
            (external_id,),
        )
        conn.commit()
        return cur.fetchone()[0]


def _seed_credentials(conn: psycopg.Connection, espn_league_id: str) -> None:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO espn_league_credentials (espn_league_id, espn_s2, swid) "
            "VALUES (%s, 's2', 'swid') ON CONFLICT DO NOTHING",
            (espn_league_id,),
        )
    conn.commit()


def _mock_team(team_id: int, first: str, last: str, name: str) -> MagicMock:
    t = MagicMock()
    t.team_id = team_id
    t.owners = [{"firstName": first, "lastName": last}]
    t.team_name = name
    return t


def test_fetch_and_upsert_teams_creates_teams(db_conn):
    league_id = _seed_league(db_conn, "9001")
    _seed_credentials(db_conn, "9001")

    mock_league = MagicMock()
    mock_league.teams = [
        _mock_team(1, "Alice", "Smith", "Team A"),
        _mock_team(2, "Bob", "Jones", "Team B"),
    ]
    params = ESPNLeagueSyncParams(espn_league_id="9001", year=2025, espn_s2="s2", swid="swid")

    with patch("activities.teams.League", return_value=mock_league):
        fetch_and_upsert_teams.__wrapped__(params)

    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT espn_id, name, owner FROM teams WHERE league_id = %s ORDER BY espn_id",
            (league_id,),
        )
        rows = cur.fetchall()

    assert len(rows) == 2
    assert rows[0] == (1, "Team A", "Alice Smith")
    assert rows[1] == (2, "Team B", "Bob Jones")


def test_fetch_and_upsert_teams_is_idempotent(db_conn):
    league_id = _seed_league(db_conn, "9002")
    _seed_credentials(db_conn, "9002")

    mock_league = MagicMock()
    mock_league.teams = [_mock_team(1, "Alice", "Smith", "Team A")]
    params = ESPNLeagueSyncParams(espn_league_id="9002", year=2025, espn_s2="s2", swid="swid")

    with patch("activities.teams.League", return_value=mock_league):
        fetch_and_upsert_teams.__wrapped__(params)
        fetch_and_upsert_teams.__wrapped__(params)  # second call must not raise or duplicate

    with db_conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) FROM teams WHERE league_id = %s", (league_id,))
        assert cur.fetchone()[0] == 1


def test_mark_teams_fetched_sets_timestamp(db_conn):
    _seed_credentials(db_conn, "9003")
    mark_teams_fetched.__wrapped__("9003")
    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT last_teams_fetched_at FROM espn_league_credentials WHERE espn_league_id = '9003'"
        )
        assert cur.fetchone()[0] is not None
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd v2/workers/espn
uv run pytest tests/test_teams.py -v
```
Expected: `ImportError` — `activities.teams` not defined.

- [ ] **Step 3: Write activities/teams.py**

```python
# v2/workers/espn/activities/teams.py
from dataclasses import dataclass
from temporalio import activity
from espn_api.football import League
from db import get_connection, resolve_league_id


@dataclass
class ESPNLeagueSyncParams:
    espn_league_id: str
    year: int
    espn_s2: str
    swid: str


@activity.defn
def fetch_and_upsert_teams(params: ESPNLeagueSyncParams) -> None:
    league = League(
        league_id=int(params.espn_league_id),
        year=params.year,
        espn_s2=params.espn_s2,
        swid=params.swid,
        debug=False,
    )
    with get_connection() as conn:
        league_id = resolve_league_id(conn, params.espn_league_id)
        with conn.cursor() as cur:
            for team in league.teams:
                owner = f"{team.owners[0]['firstName']} {team.owners[0]['lastName']}"
                cur.execute(
                    "SELECT id FROM teams WHERE espn_id = %s AND league_id = %s",
                    (team.team_id, league_id),
                )
                if cur.fetchone() is None:
                    cur.execute(
                        "INSERT INTO teams (espn_id, league_id, name, owner, year, created_at, updated_at) "
                        "VALUES (%s, %s, %s, %s, %s, NOW(), NOW())",
                        (team.team_id, league_id, team.team_name, owner, params.year),
                    )
                else:
                    cur.execute(
                        "UPDATE teams SET name = %s, owner = %s, updated_at = NOW() "
                        "WHERE espn_id = %s AND league_id = %s",
                        (team.team_name, owner, team.team_id, league_id),
                    )
        conn.commit()


@activity.defn
def mark_teams_fetched(espn_league_id: str) -> None:
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(
                "UPDATE espn_league_credentials SET last_teams_fetched_at = NOW() "
                "WHERE espn_league_id = %s",
                (espn_league_id,),
            )
        conn.commit()
```

- [ ] **Step 4: Write workflows/teams.py**

```python
# v2/workers/espn/workflows/teams.py
from datetime import timedelta
from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities.teams import ESPNLeagueSyncParams, fetch_and_upsert_teams, mark_teams_fetched

_LONG = dict(start_to_close_timeout=timedelta(minutes=15))
_SHORT = dict(start_to_close_timeout=timedelta(seconds=30))


@workflow.defn
class ESPNTeamSyncWorkflow:
    @workflow.run
    async def run(self, params: ESPNLeagueSyncParams) -> None:
        await workflow.execute_activity(fetch_and_upsert_teams, params, **_LONG)
        await workflow.execute_activity(mark_teams_fetched, params.espn_league_id, **_SHORT)
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd v2/workers/espn
uv run pytest tests/test_teams.py -v
```
Expected: 3 tests pass.

- [ ] **Step 6: Commit**

```bash
git add v2/workers/espn/activities/teams.py v2/workers/espn/workflows/teams.py v2/workers/espn/tests/test_teams.py
git commit -m "feat(espn): add teams sync workflow"
```

---

### Task 5: Schedule Workflow

**Files:**
- Create: `v2/workers/espn/activities/schedule.py`
- Create: `v2/workers/espn/workflows/schedule.py`
- Create: `v2/workers/espn/tests/test_schedule.py`

**Interfaces:**
- Consumes: `ESPNLeagueSyncParams` from `activities/teams.py`; `resolve_league_id`, `resolve_team_id` from `db.py`
- Produces:
  - `@activity.defn fetch_and_upsert_schedule(params: ESPNLeagueSyncParams) -> None` — writes matchups and box scores
  - `@activity.defn mark_schedule_fetched(espn_league_id: str) -> None`
  - `@workflow.defn class ESPNScheduleSyncWorkflow`

Note on schema: `box_scores` has columns `matchup_id, player_id, team_id, slot_position, actual_points, projected_points, started_flag` (week/year/game_date were removed in migrations 002 and 004). `matchups` has `league_id, week, year, home_team_id, away_team_id, game_type, home_team_final_score, away_team_final_score, home_team_espn_projected_score, away_team_espn_projected_score, completed, is_playoff`.

- [ ] **Step 1: Write the failing tests**

```python
# v2/workers/espn/tests/test_schedule.py
from unittest.mock import MagicMock, patch

import psycopg
import pytest

from activities.schedule import fetch_and_upsert_schedule, mark_schedule_fetched
from activities.teams import ESPNLeagueSyncParams


def _seed_league(conn: psycopg.Connection, external_id: str) -> int:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO leagues (name, platform, external_id) VALUES ('Test', 'ESPN', %s) "
            "ON CONFLICT (platform, external_id) WHERE platform != '' AND external_id != '' "
            "DO UPDATE SET name = EXCLUDED.name RETURNING id",
            (external_id,),
        )
        conn.commit()
        return cur.fetchone()[0]


def _seed_team(conn: psycopg.Connection, league_id: int, espn_id: int) -> int:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO teams (espn_id, league_id, name, owner, year, created_at, updated_at) "
            "VALUES (%s, %s, 'Team', 'Owner', 2025, NOW(), NOW()) "
            "ON CONFLICT (espn_id, league_id) DO UPDATE SET updated_at = NOW() RETURNING id",
            (espn_id, league_id),
        )
        conn.commit()
        return cur.fetchone()[0]


def _seed_credentials(conn: psycopg.Connection, espn_league_id: str) -> None:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO espn_league_credentials (espn_league_id, espn_s2, swid) "
            "VALUES (%s, 's2', 'swid') ON CONFLICT DO NOTHING",
            (espn_league_id,),
        )
    conn.commit()


def _mock_box_score(home_espn_id: int, away_espn_id: int, home_score: float = 110.0, away_score: float = 95.0) -> MagicMock:
    bs = MagicMock()
    bs.home_team = MagicMock(team_id=home_espn_id)
    bs.away_team = MagicMock(team_id=away_espn_id)
    bs.home_score = home_score
    bs.away_score = away_score
    bs.home_projected = 108.0
    bs.away_projected = 97.0
    bs.matchup_type = "REGULAR"
    bs.is_playoff = False
    bs.home_lineup = []
    bs.away_lineup = []
    return bs


def test_fetch_and_upsert_schedule_creates_matchup(db_conn):
    league_id = _seed_league(db_conn, "8001")
    _seed_team(db_conn, league_id, 1)
    _seed_team(db_conn, league_id, 2)
    _seed_credentials(db_conn, "8001")

    mock_league = MagicMock()
    mock_league.year = 2025
    mock_league.current_week = 1
    mock_league.box_scores.return_value = [_mock_box_score(1, 2)]

    params = ESPNLeagueSyncParams(espn_league_id="8001", year=2025, espn_s2="s2", swid="swid")

    with patch("activities.schedule.League", return_value=mock_league):
        fetch_and_upsert_schedule.__wrapped__(params)

    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT week, year, completed FROM matchups WHERE league_id = %s",
            (league_id,),
        )
        rows = cur.fetchall()
    assert len(rows) == 1
    assert rows[0][:2] == (1, 2025)


def test_fetch_and_upsert_schedule_is_idempotent(db_conn):
    league_id = _seed_league(db_conn, "8002")
    _seed_team(db_conn, league_id, 1)
    _seed_team(db_conn, league_id, 2)
    _seed_credentials(db_conn, "8002")

    mock_league = MagicMock()
    mock_league.year = 2025
    mock_league.current_week = 1
    mock_league.box_scores.return_value = [_mock_box_score(1, 2)]

    params = ESPNLeagueSyncParams(espn_league_id="8002", year=2025, espn_s2="s2", swid="swid")

    with patch("activities.schedule.League", return_value=mock_league):
        fetch_and_upsert_schedule.__wrapped__(params)
        fetch_and_upsert_schedule.__wrapped__(params)

    with db_conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) FROM matchups WHERE league_id = %s", (league_id,))
        assert cur.fetchone()[0] == 1


def test_mark_schedule_fetched_sets_timestamp(db_conn):
    _seed_credentials(db_conn, "8003")
    mark_schedule_fetched.__wrapped__("8003")
    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT last_schedule_fetched_at FROM espn_league_credentials WHERE espn_league_id = '8003'"
        )
        assert cur.fetchone()[0] is not None
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd v2/workers/espn
uv run pytest tests/test_schedule.py -v
```
Expected: `ImportError`.

- [ ] **Step 3: Write activities/schedule.py**

```python
# v2/workers/espn/activities/schedule.py
import logging
from datetime import datetime
from temporalio import activity
from espn_api.football import League
from db import get_connection, resolve_league_id, resolve_team_id
from activities.teams import ESPNLeagueSyncParams

logger = logging.getLogger(__name__)


def _upsert_player(cur, espn_id: int, name: str, position: str) -> int:
    cur.execute("SELECT id FROM players WHERE espn_id = %s", (espn_id,))
    row = cur.fetchone()
    if row is None:
        cur.execute(
            "INSERT INTO players (espn_id, name, position, status, created_at, updated_at) "
            "VALUES (%s, %s, %s, 'active', NOW(), NOW()) RETURNING id",
            (espn_id, name, position),
        )
        return cur.fetchone()[0]
    return row[0]


@activity.defn
def fetch_and_upsert_schedule(params: ESPNLeagueSyncParams) -> None:
    league = League(
        league_id=int(params.espn_league_id),
        year=params.year,
        espn_s2=params.espn_s2,
        swid=params.swid,
        debug=False,
    )

    with get_connection() as conn:
        league_id = resolve_league_id(conn, params.espn_league_id)
        with conn.cursor() as cur:
            cur.execute("SELECT espn_id, id FROM teams WHERE league_id = %s", (league_id,))
            team_map = {row[0]: row[1] for row in cur.fetchall()}

        with conn.cursor() as cur:
            for week in range(1, 18):
                if week > league.current_week and datetime.now().year == league.year:
                    break

                if league.year < 2019:
                    entries = league.scoreboard(week=week)
                else:
                    entries = league.box_scores(week=week)

                for bs in entries:
                    if not hasattr(bs, "home_team") or not hasattr(bs, "away_team"):
                        continue
                    if bs.home_team == 0 or bs.away_team == 0:
                        continue

                    home_db_id = team_map.get(bs.home_team.team_id)
                    away_db_id = team_map.get(bs.away_team.team_id)
                    if home_db_id is None or away_db_id is None:
                        logger.warning("Skipping matchup week %d — team not found in DB", week)
                        continue

                    home_score = getattr(bs, "home_score", 0)
                    away_score = getattr(bs, "away_score", 0)
                    home_proj = getattr(bs, "home_projected", -1)
                    away_proj = getattr(bs, "away_projected", -1)
                    completed = (
                        league.year < 2019
                        or (league.current_week >= week and home_score > 0 and away_score > 0)
                    )

                    cur.execute(
                        "SELECT id FROM matchups WHERE league_id = %s AND week = %s AND year = %s "
                        "AND home_team_id = %s AND away_team_id = %s",
                        (league_id, week, league.year, home_db_id, away_db_id),
                    )
                    existing = cur.fetchone()

                    if existing is None:
                        cur.execute(
                            "INSERT INTO matchups (league_id, week, year, home_team_id, away_team_id, "
                            "home_team_final_score, away_team_final_score, "
                            "home_team_espn_projected_score, away_team_espn_projected_score, "
                            "completed, is_playoff, game_type, created_at, updated_at) "
                            "VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,NOW(),NOW()) RETURNING id",
                            (league_id, week, league.year, home_db_id, away_db_id,
                             home_score, away_score, home_proj, away_proj,
                             completed, bs.is_playoff, getattr(bs, "matchup_type", "REGULAR")),
                        )
                        matchup_id = cur.fetchone()[0]
                    else:
                        matchup_id = existing[0]
                        cur.execute(
                            "UPDATE matchups SET home_team_final_score = %s, away_team_final_score = %s, "
                            "home_team_espn_projected_score = %s, away_team_espn_projected_score = %s, "
                            "completed = %s, updated_at = NOW() WHERE id = %s",
                            (home_score, away_score, home_proj, away_proj, completed, matchup_id),
                        )

                    # Write box scores only for completed weeks with lineup data
                    if completed and hasattr(bs, "home_lineup") and hasattr(bs, "away_lineup"):
                        for player, team_db_id in (
                            [(p, home_db_id) for p in bs.home_lineup]
                            + [(p, away_db_id) for p in bs.away_lineup]
                        ):
                            player_db_id = _upsert_player(
                                cur, player.playerId, player.name,
                                getattr(player, "position", "Unknown"),
                            )
                            cur.execute(
                                "SELECT id FROM box_scores WHERE matchup_id = %s AND player_id = %s AND team_id = %s",
                                (matchup_id, player_db_id, team_db_id),
                            )
                            if cur.fetchone() is None:
                                started = player.slot_position not in ("BE", "IR")
                                cur.execute(
                                    "INSERT INTO box_scores (matchup_id, player_id, team_id, slot_position, "
                                    "actual_points, projected_points, started_flag, created_at, updated_at) "
                                    "VALUES (%s,%s,%s,%s,%s,%s,%s,NOW(),NOW())",
                                    (matchup_id, player_db_id, team_db_id, player.slot_position,
                                     player.points, player.projected_points, started),
                                )
        conn.commit()


@activity.defn
def mark_schedule_fetched(espn_league_id: str) -> None:
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(
                "UPDATE espn_league_credentials SET last_schedule_fetched_at = NOW() "
                "WHERE espn_league_id = %s",
                (espn_league_id,),
            )
        conn.commit()
```

- [ ] **Step 4: Write workflows/schedule.py**

```python
# v2/workers/espn/workflows/schedule.py
from datetime import timedelta
from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities.schedule import fetch_and_upsert_schedule, mark_schedule_fetched
    from activities.teams import ESPNLeagueSyncParams

_LONG = dict(start_to_close_timeout=timedelta(minutes=30))
_SHORT = dict(start_to_close_timeout=timedelta(seconds=30))


@workflow.defn
class ESPNScheduleSyncWorkflow:
    @workflow.run
    async def run(self, params: ESPNLeagueSyncParams) -> None:
        await workflow.execute_activity(fetch_and_upsert_schedule, params, **_LONG)
        await workflow.execute_activity(mark_schedule_fetched, params.espn_league_id, **_SHORT)
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd v2/workers/espn
uv run pytest tests/test_schedule.py -v
```
Expected: 3 tests pass.

- [ ] **Step 6: Commit**

```bash
git add v2/workers/espn/activities/schedule.py v2/workers/espn/workflows/schedule.py v2/workers/espn/tests/test_schedule.py
git commit -m "feat(espn): add schedule sync workflow"
```

---

### Task 6: Draft Workflow

**Files:**
- Create: `v2/workers/espn/activities/draft.py`
- Create: `v2/workers/espn/workflows/draft.py`
- Create: `v2/workers/espn/tests/test_draft.py`

**Interfaces:**
- Consumes: `ESPNLeagueSyncParams` from `activities/teams.py`; `resolve_league_id` from `db.py`
- Produces:
  - `@activity.defn fetch_and_upsert_draft(params: ESPNLeagueSyncParams) -> None`
  - `@activity.defn mark_draft_fetched(espn_league_id: str) -> None`
  - `@workflow.defn class ESPNDraftSyncWorkflow`

- [ ] **Step 1: Write the failing tests**

```python
# v2/workers/espn/tests/test_draft.py
from unittest.mock import MagicMock, patch

import psycopg
import pytest

from activities.draft import fetch_and_upsert_draft, mark_draft_fetched
from activities.teams import ESPNLeagueSyncParams


def _seed_league(conn: psycopg.Connection, external_id: str) -> int:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO leagues (name, platform, external_id) VALUES ('Test', 'ESPN', %s) "
            "ON CONFLICT (platform, external_id) WHERE platform != '' AND external_id != '' "
            "DO UPDATE SET name = EXCLUDED.name RETURNING id",
            (external_id,),
        )
        conn.commit()
        return cur.fetchone()[0]


def _seed_team(conn: psycopg.Connection, league_id: int, espn_id: int) -> int:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO teams (espn_id, league_id, name, owner, year, created_at, updated_at) "
            "VALUES (%s, %s, 'Team', 'Owner', 2025, NOW(), NOW()) "
            "ON CONFLICT (espn_id, league_id) DO UPDATE SET updated_at = NOW() RETURNING id",
            (espn_id, league_id),
        )
        conn.commit()
        return cur.fetchone()[0]


def _seed_credentials(conn: psycopg.Connection, espn_league_id: str) -> None:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO espn_league_credentials (espn_league_id, espn_s2, swid) "
            "VALUES (%s, 's2', 'swid') ON CONFLICT DO NOTHING",
            (espn_league_id,),
        )
    conn.commit()


def test_fetch_and_upsert_draft_creates_selection(db_conn):
    league_id = _seed_league(db_conn, "7001")
    _seed_team(db_conn, league_id, 1)
    _seed_credentials(db_conn, "7001")

    pick = MagicMock()
    pick.playerId = 12345
    pick.playerName = "Patrick Mahomes"
    pick.round_num = 1
    pick.round_pick = 1
    pick.team = MagicMock(team_id=1)

    player_info = MagicMock(position="QB")

    mock_league = MagicMock()
    mock_league.draft = [pick]
    mock_league.year = 2025
    mock_league.player_info.return_value = player_info

    params = ESPNLeagueSyncParams(espn_league_id="7001", year=2025, espn_s2="s2", swid="swid")

    with patch("activities.draft.League", return_value=mock_league):
        fetch_and_upsert_draft.__wrapped__(params)

    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT player_name, round, pick FROM draft_selections WHERE league_id = %s",
            (league_id,),
        )
        rows = cur.fetchall()
    assert len(rows) == 1
    assert rows[0] == ("Patrick Mahomes", 1, 1)


def test_fetch_and_upsert_draft_is_idempotent(db_conn):
    league_id = _seed_league(db_conn, "7002")
    _seed_team(db_conn, league_id, 1)
    _seed_credentials(db_conn, "7002")

    pick = MagicMock()
    pick.playerId = 12346
    pick.playerName = "Josh Allen"
    pick.round_num = 1
    pick.round_pick = 2
    pick.team = MagicMock(team_id=1)

    mock_league = MagicMock(draft=[pick], year=2025)
    mock_league.player_info.return_value = MagicMock(position="QB")

    params = ESPNLeagueSyncParams(espn_league_id="7002", year=2025, espn_s2="s2", swid="swid")

    with patch("activities.draft.League", return_value=mock_league):
        fetch_and_upsert_draft.__wrapped__(params)
        fetch_and_upsert_draft.__wrapped__(params)

    with db_conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) FROM draft_selections WHERE league_id = %s", (league_id,))
        assert cur.fetchone()[0] == 1


def test_mark_draft_fetched_sets_timestamp(db_conn):
    _seed_credentials(db_conn, "7003")
    mark_draft_fetched.__wrapped__("7003")
    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT last_draft_fetched_at FROM espn_league_credentials WHERE espn_league_id = '7003'"
        )
        assert cur.fetchone()[0] is not None
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd v2/workers/espn
uv run pytest tests/test_draft.py -v
```
Expected: `ImportError`.

- [ ] **Step 3: Write activities/draft.py**

```python
# v2/workers/espn/activities/draft.py
import logging
import time
from temporalio import activity
from espn_api.football import League
from db import get_connection, resolve_league_id
from activities.teams import ESPNLeagueSyncParams

logger = logging.getLogger(__name__)


@activity.defn
def fetch_and_upsert_draft(params: ESPNLeagueSyncParams) -> None:
    league = League(
        league_id=int(params.espn_league_id),
        year=params.year,
        espn_s2=params.espn_s2,
        swid=params.swid,
        debug=False,
    )

    with get_connection() as conn:
        league_id = resolve_league_id(conn, params.espn_league_id)

        with conn.cursor() as cur:
            cur.execute("SELECT espn_id, id FROM teams WHERE league_id = %s", (league_id,))
            team_map = {row[0]: row[1] for row in cur.fetchall()}

        with conn.cursor() as cur:
            for pick in league.draft:
                try:
                    info = league.player_info(playerId=pick.playerId)
                    position = info.position if info else "Unknown"
                except Exception as exc:
                    logger.warning("player_info failed for %s: %s", pick.playerName, exc)
                    position = "Unknown"

                # Upsert player
                cur.execute("SELECT id FROM players WHERE espn_id = %s", (pick.playerId,))
                row = cur.fetchone()
                if row is None:
                    cur.execute(
                        "INSERT INTO players (espn_id, name, position, status, created_at, updated_at) "
                        "VALUES (%s, %s, %s, 'active', NOW(), NOW()) RETURNING id",
                        (pick.playerId, pick.playerName, position),
                    )
                    player_db_id = cur.fetchone()[0]
                else:
                    player_db_id = row[0]

                team_db_id = team_map.get(pick.team.team_id)
                if team_db_id is None:
                    logger.warning("No team found for ESPN team_id=%s, skipping pick", pick.team.team_id)
                    continue

                cur.execute(
                    "SELECT id FROM draft_selections "
                    "WHERE player_id = %s AND team_id = %s AND year = %s AND league_id = %s",
                    (player_db_id, team_db_id, league.year, league_id),
                )
                if cur.fetchone() is None:
                    cur.execute(
                        "INSERT INTO draft_selections "
                        "(player_id, player_name, player_position, team_id, round, pick, year, league_id, created_at, updated_at) "
                        "VALUES (%s,%s,%s,%s,%s,%s,%s,%s,NOW(),NOW())",
                        (player_db_id, pick.playerName, position,
                         team_db_id, pick.round_num, pick.round_pick, league.year, league_id),
                    )
                time.sleep(0.1)  # avoid ESPN API rate limit when calling player_info per pick

        conn.commit()


@activity.defn
def mark_draft_fetched(espn_league_id: str) -> None:
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(
                "UPDATE espn_league_credentials SET last_draft_fetched_at = NOW() "
                "WHERE espn_league_id = %s",
                (espn_league_id,),
            )
        conn.commit()
```

- [ ] **Step 4: Write workflows/draft.py**

```python
# v2/workers/espn/workflows/draft.py
from datetime import timedelta
from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities.draft import fetch_and_upsert_draft, mark_draft_fetched
    from activities.teams import ESPNLeagueSyncParams

_LONG = dict(start_to_close_timeout=timedelta(minutes=30))
_SHORT = dict(start_to_close_timeout=timedelta(seconds=30))


@workflow.defn
class ESPNDraftSyncWorkflow:
    @workflow.run
    async def run(self, params: ESPNLeagueSyncParams) -> None:
        await workflow.execute_activity(fetch_and_upsert_draft, params, **_LONG)
        await workflow.execute_activity(mark_draft_fetched, params.espn_league_id, **_SHORT)
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd v2/workers/espn
uv run pytest tests/test_draft.py -v
```
Expected: 3 tests pass.

- [ ] **Step 6: Commit**

```bash
git add v2/workers/espn/activities/draft.py v2/workers/espn/workflows/draft.py v2/workers/espn/tests/test_draft.py
git commit -m "feat(espn): add draft sync workflow"
```

---

### Task 7: Transactions Workflow

**Files:**
- Create: `v2/workers/espn/activities/transactions.py`
- Create: `v2/workers/espn/workflows/transactions.py`
- Create: `v2/workers/espn/tests/test_transactions.py`

**Interfaces:**
- Consumes: `ESPNLeagueSyncParams` from `activities/teams.py`; `resolve_league_id` from `db.py`
- Produces:
  - `@activity.defn fetch_and_upsert_transactions(params: ESPNLeagueSyncParams) -> None`
  - `@activity.defn mark_transactions_fetched(espn_league_id: str) -> None`
  - `@workflow.defn class ESPNTransactionSyncWorkflow`

- [ ] **Step 1: Write the failing tests**

```python
# v2/workers/espn/tests/test_transactions.py
from datetime import datetime
from unittest.mock import MagicMock, patch

import psycopg
import pytest

from activities.transactions import fetch_and_upsert_transactions, mark_transactions_fetched
from activities.teams import ESPNLeagueSyncParams


def _seed_league(conn: psycopg.Connection, external_id: str) -> int:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO leagues (name, platform, external_id) VALUES ('Test', 'ESPN', %s) "
            "ON CONFLICT (platform, external_id) WHERE platform != '' AND external_id != '' "
            "DO UPDATE SET name = EXCLUDED.name RETURNING id",
            (external_id,),
        )
        conn.commit()
        return cur.fetchone()[0]


def _seed_team(conn: psycopg.Connection, league_id: int, espn_id: int) -> int:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO teams (espn_id, league_id, name, owner, year, created_at, updated_at) "
            "VALUES (%s, %s, 'Team', 'Owner', 2025, NOW(), NOW()) "
            "ON CONFLICT (espn_id, league_id) DO UPDATE SET updated_at = NOW() RETURNING id",
            (espn_id, league_id),
        )
        conn.commit()
        return cur.fetchone()[0]


def _seed_credentials(conn: psycopg.Connection, espn_league_id: str) -> None:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO espn_league_credentials (espn_league_id, espn_s2, swid) "
            "VALUES (%s, 's2', 'swid') ON CONFLICT DO NOTHING",
            (espn_league_id,),
        )
    conn.commit()


def test_fetch_and_upsert_transactions_creates_record(db_conn):
    league_id = _seed_league(db_conn, "6001")
    _seed_team(db_conn, league_id, 1)
    _seed_credentials(db_conn, "6001")

    player = MagicMock(playerId=99001, name="Justin Jefferson", position="WR")
    team = MagicMock(team_id=1)
    tx = MagicMock(date=1700000000000, actions=[(team, "ADDED", player, 0)])

    mock_league = MagicMock(year=2025)
    mock_league.recent_activity.side_effect = [[tx], []]

    params = ESPNLeagueSyncParams(espn_league_id="6001", year=2025, espn_s2="s2", swid="swid")

    with patch("activities.transactions.League", return_value=mock_league):
        fetch_and_upsert_transactions.__wrapped__(params)

    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT transaction_type, player_name FROM transactions WHERE league_id = %s",
            (league_id,),
        )
        rows = cur.fetchall()
    assert len(rows) == 1
    assert rows[0] == ("ADDED", "Justin Jefferson")


def test_fetch_and_upsert_transactions_skips_pre2024(db_conn):
    _seed_credentials(db_conn, "6002")
    mock_league = MagicMock(year=2023)
    params = ESPNLeagueSyncParams(espn_league_id="6002", year=2023, espn_s2="s2", swid="swid")

    with patch("activities.transactions.League", return_value=mock_league):
        fetch_and_upsert_transactions.__wrapped__(params)

    mock_league.recent_activity.assert_not_called()


def test_mark_transactions_fetched_sets_timestamp(db_conn):
    _seed_credentials(db_conn, "6003")
    mark_transactions_fetched.__wrapped__("6003")
    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT last_transactions_fetched_at FROM espn_league_credentials WHERE espn_league_id = '6003'"
        )
        assert cur.fetchone()[0] is not None
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd v2/workers/espn
uv run pytest tests/test_transactions.py -v
```
Expected: `ImportError`.

- [ ] **Step 3: Write activities/transactions.py**

```python
# v2/workers/espn/activities/transactions.py
import logging
from datetime import datetime
from temporalio import activity
from espn_api.football import League
from db import get_connection, resolve_league_id
from activities.teams import ESPNLeagueSyncParams

logger = logging.getLogger(__name__)


@activity.defn
def fetch_and_upsert_transactions(params: ESPNLeagueSyncParams) -> None:
    if params.year < 2024:
        logger.info("Transactions not available before 2024 — skipping year %d", params.year)
        return

    league = League(
        league_id=int(params.espn_league_id),
        year=params.year,
        espn_s2=params.espn_s2,
        swid=params.swid,
        debug=False,
    )

    with get_connection() as conn:
        league_id = resolve_league_id(conn, params.espn_league_id)

        with conn.cursor() as cur:
            cur.execute("SELECT espn_id, id FROM teams WHERE league_id = %s", (league_id,))
            team_map = {row[0]: row[1] for row in cur.fetchall()}

        offset = 0
        with conn.cursor() as cur:
            while True:
                try:
                    txns = league.recent_activity(offset=offset)
                    if not txns:
                        break
                    for tx in txns:
                        tx_date = datetime.fromtimestamp(tx.date / 1000)
                        for team, tx_type, player, bid_amount in tx.actions:
                            team_db_id = team_map.get(team.team_id)
                            if team_db_id is None:
                                continue

                            cur.execute("SELECT id FROM players WHERE espn_id = %s", (player.playerId,))
                            row = cur.fetchone()
                            if row is None:
                                cur.execute(
                                    "INSERT INTO players (espn_id, name, position, status, created_at, updated_at) "
                                    "VALUES (%s, %s, %s, 'active', NOW(), NOW()) RETURNING id",
                                    (player.playerId, player.name, player.position),
                                )
                                player_db_id = cur.fetchone()[0]
                            else:
                                player_db_id = row[0]

                            cur.execute(
                                "SELECT id FROM transactions "
                                "WHERE team_id = %s AND player_id = %s AND date = %s "
                                "AND transaction_type = %s AND league_id = %s",
                                (team_db_id, player_db_id, tx_date, tx_type, league_id),
                            )
                            if cur.fetchone() is None:
                                cur.execute(
                                    "INSERT INTO transactions "
                                    "(team_id, player_id, transaction_type, player_name, "
                                    "bid_amount, date, year, league_id, created_at, updated_at) "
                                    "VALUES (%s,%s,%s,%s,%s,%s,%s,%s,NOW(),NOW())",
                                    (team_db_id, player_db_id, tx_type, player.name,
                                     int(bid_amount), tx_date, league.year, league_id),
                                )
                    offset += 25
                except Exception as exc:
                    logger.error("Transaction fetch error at offset %d: %s", offset, exc)
                    break
        conn.commit()


@activity.defn
def mark_transactions_fetched(espn_league_id: str) -> None:
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(
                "UPDATE espn_league_credentials SET last_transactions_fetched_at = NOW() "
                "WHERE espn_league_id = %s",
                (espn_league_id,),
            )
        conn.commit()
```

- [ ] **Step 4: Write workflows/transactions.py**

```python
# v2/workers/espn/workflows/transactions.py
from datetime import timedelta
from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities.transactions import fetch_and_upsert_transactions, mark_transactions_fetched
    from activities.teams import ESPNLeagueSyncParams

_LONG = dict(start_to_close_timeout=timedelta(minutes=30))
_SHORT = dict(start_to_close_timeout=timedelta(seconds=30))


@workflow.defn
class ESPNTransactionSyncWorkflow:
    @workflow.run
    async def run(self, params: ESPNLeagueSyncParams) -> None:
        await workflow.execute_activity(fetch_and_upsert_transactions, params, **_LONG)
        await workflow.execute_activity(mark_transactions_fetched, params.espn_league_id, **_SHORT)
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd v2/workers/espn
uv run pytest tests/test_transactions.py -v
```
Expected: 3 tests pass.

- [ ] **Step 6: Commit**

```bash
git add v2/workers/espn/activities/transactions.py v2/workers/espn/workflows/transactions.py v2/workers/espn/tests/test_transactions.py
git commit -m "feat(espn): add transactions sync workflow"
```

---

### Task 8: Player Status Workflow

**Files:**
- Create: `v2/workers/espn/activities/player_status.py`
- Create: `v2/workers/espn/workflows/player_status.py`
- Create: `v2/workers/espn/tests/test_player_status.py`

**Interfaces:**
- Consumes: `AnyESPNCredentials` from `activities/credentials.py`; `get_any_espn_credentials` activity
- Produces:
  - `@dataclass PlayerStatusParams(espn_league_id: str, espn_s2: str, swid: str, year: int)`
  - `@activity.defn update_active_players(params: PlayerStatusParams) -> None`
  - `@activity.defn mark_players_updated() -> None` — updates ALL credential rows
  - `@workflow.defn class ESPNPlayerStatusSyncWorkflow` with `run(self, year: int) -> None`

- [ ] **Step 1: Write the failing tests**

```python
# v2/workers/espn/tests/test_player_status.py
from unittest.mock import MagicMock, patch

import psycopg
import pytest

from activities.player_status import PlayerStatusParams, mark_players_updated, update_active_players


def _seed_credentials(conn: psycopg.Connection, espn_league_id: str) -> None:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO espn_league_credentials (espn_league_id, espn_s2, swid) "
            "VALUES (%s, 's2', 'swid') ON CONFLICT DO NOTHING",
            (espn_league_id,),
        )
    conn.commit()


def _seed_player(conn: psycopg.Connection, espn_id: int, position: str = "WR", status: str = "active") -> int:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO players (espn_id, name, position, status, created_at, updated_at) "
            "VALUES (%s, 'Test Player', %s, %s, NOW(), NOW()) "
            "ON CONFLICT (espn_id) DO UPDATE SET position = EXCLUDED.position, status = EXCLUDED.status "
            "RETURNING id",
            (espn_id, position, status),
        )
        conn.commit()
        return cur.fetchone()[0]


def test_update_active_players_marks_missing_as_inactive(db_conn):
    _seed_player(db_conn, 55001)
    mock_league = MagicMock()
    mock_league.player_info.return_value = None  # ESPN returns None = no longer exists
    params = PlayerStatusParams(espn_league_id="5500", espn_s2="s2", swid="swid", year=2025)

    with patch("activities.player_status.League", return_value=mock_league):
        update_active_players.__wrapped__(params)

    with db_conn.cursor() as cur:
        cur.execute("SELECT status FROM players WHERE espn_id = 55001")
        assert cur.fetchone()[0] == "inactive"


def test_update_active_players_updates_changed_position(db_conn):
    _seed_player(db_conn, 55002, position="RB")
    updated = MagicMock(position="WR")
    mock_league = MagicMock()
    mock_league.player_info.return_value = updated
    params = PlayerStatusParams(espn_league_id="5501", espn_s2="s2", swid="swid", year=2025)

    with patch("activities.player_status.League", return_value=mock_league):
        update_active_players.__wrapped__(params)

    with db_conn.cursor() as cur:
        cur.execute("SELECT position FROM players WHERE espn_id = 55002")
        assert cur.fetchone()[0] == "WR"


def test_mark_players_updated_stamps_all_credential_rows(db_conn):
    _seed_credentials(db_conn, "5502")
    _seed_credentials(db_conn, "5503")
    mark_players_updated.__wrapped__()
    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT COUNT(*) FROM espn_league_credentials "
            "WHERE espn_league_id IN ('5502','5503') AND last_players_updated_at IS NOT NULL"
        )
        assert cur.fetchone()[0] == 2
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd v2/workers/espn
uv run pytest tests/test_player_status.py -v
```
Expected: `ImportError`.

- [ ] **Step 3: Write activities/player_status.py**

```python
# v2/workers/espn/activities/player_status.py
import logging
from dataclasses import dataclass
from temporalio import activity
from espn_api.football import League
from db import get_connection

logger = logging.getLogger(__name__)


@dataclass
class PlayerStatusParams:
    espn_league_id: str
    espn_s2: str
    swid: str
    year: int


@activity.defn
def update_active_players(params: PlayerStatusParams) -> None:
    """Check all active players against ESPN and mark inactive or update positions."""
    league = League(
        league_id=int(params.espn_league_id),
        year=params.year,
        espn_s2=params.espn_s2,
        swid=params.swid,
        debug=False,
    )

    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute("SELECT espn_id, position FROM players WHERE status != 'inactive'")
            all_players = cur.fetchall()

        logger.info("Checking %d active players against ESPN", len(all_players))

        with conn.cursor() as cur:
            for espn_id, position in all_players:
                p = league.player_info(playerId=espn_id)
                if p is None:
                    logger.info("Marking player espn_id=%s as inactive", espn_id)
                    cur.execute(
                        "UPDATE players SET status = 'inactive', updated_at = NOW() WHERE espn_id = %s",
                        (espn_id,),
                    )
                elif p.position != position:
                    logger.info("Updating espn_id=%s position %s → %s", espn_id, position, p.position)
                    cur.execute(
                        "UPDATE players SET position = %s, updated_at = NOW() WHERE espn_id = %s",
                        (p.position, espn_id),
                    )
        conn.commit()


@activity.defn
def mark_players_updated() -> None:
    """Stamp last_players_updated_at on all credential rows (update is global, not per-league)."""
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute("UPDATE espn_league_credentials SET last_players_updated_at = NOW()")
        conn.commit()
```

- [ ] **Step 4: Write workflows/player_status.py**

```python
# v2/workers/espn/workflows/player_status.py
from datetime import timedelta
from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities.credentials import get_any_espn_credentials
    from activities.player_status import PlayerStatusParams, mark_players_updated, update_active_players

_LONG = dict(start_to_close_timeout=timedelta(minutes=30))
_SHORT = dict(start_to_close_timeout=timedelta(seconds=30))


@workflow.defn
class ESPNPlayerStatusSyncWorkflow:
    @workflow.run
    async def run(self, year: int) -> None:
        creds = await workflow.execute_activity(get_any_espn_credentials, **_SHORT)
        params = PlayerStatusParams(
            espn_league_id=creds.espn_league_id,
            espn_s2=creds.espn_s2,
            swid=creds.swid,
            year=year,
        )
        await workflow.execute_activity(update_active_players, params, **_LONG)
        await workflow.execute_activity(mark_players_updated, **_SHORT)
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd v2/workers/espn
uv run pytest tests/test_player_status.py -v
```
Expected: 3 tests pass.

- [ ] **Step 6: Commit**

```bash
git add v2/workers/espn/activities/player_status.py v2/workers/espn/workflows/player_status.py v2/workers/espn/tests/test_player_status.py
git commit -m "feat(espn): add player status sync workflow"
```

---

### Task 9: LeagueESPNSyncWorkflow

**Files:**
- Create: `v2/workers/espn/workflows/league_sync.py`
- Create: `v2/workers/espn/tests/test_league_sync.py`

**Interfaces:**
- Consumes: `get_espn_credentials` from `activities/credentials.py`; `ESPNLeagueSyncParams` from `activities/teams.py`; `ESPNTeamSyncWorkflow`, `ESPNScheduleSyncWorkflow`, `ESPNDraftSyncWorkflow`, `ESPNTransactionSyncWorkflow`
- Produces:
  - `@dataclass LeagueDispatchParams(espn_league_id: str, year: int)`
  - `@workflow.defn class LeagueESPNSyncWorkflow` with `run(self, params: LeagueDispatchParams) -> None`

- [ ] **Step 1: Write the failing tests**

```python
# v2/workers/espn/tests/test_league_sync.py
import pytest
from temporalio.testing import WorkflowEnvironment
from temporalio.worker import Worker

from activities.credentials import ESPNCredentials
from workflows.league_sync import LeagueDispatchParams, LeagueESPNSyncWorkflow
from workflows.teams import ESPNTeamSyncWorkflow
from workflows.schedule import ESPNScheduleSyncWorkflow
from workflows.draft import ESPNDraftSyncWorkflow
from workflows.transactions import ESPNTransactionSyncWorkflow
from activities.teams import ESPNLeagueSyncParams


@pytest.mark.asyncio
async def test_league_sync_completes_without_error():
    """Verify LeagueESPNSyncWorkflow completes when child workflows are no-ops."""
    async with await WorkflowEnvironment.start_time_skipping() as env:

        def mock_get_credentials(espn_league_id: str) -> ESPNCredentials:
            return ESPNCredentials(espn_s2="s2", swid="swid")

        async def noop_team_run(params: ESPNLeagueSyncParams) -> None:
            pass

        async def noop_schedule_run(params: ESPNLeagueSyncParams) -> None:
            pass

        async def noop_draft_run(params: ESPNLeagueSyncParams) -> None:
            pass

        async def noop_tx_run(params: ESPNLeagueSyncParams) -> None:
            pass

        # Patch child workflow run methods with no-ops
        original_teams = ESPNTeamSyncWorkflow.run
        original_schedule = ESPNScheduleSyncWorkflow.run
        original_draft = ESPNDraftSyncWorkflow.run
        original_tx = ESPNTransactionSyncWorkflow.run

        ESPNTeamSyncWorkflow.run = noop_team_run
        ESPNScheduleSyncWorkflow.run = noop_schedule_run
        ESPNDraftSyncWorkflow.run = noop_draft_run
        ESPNTransactionSyncWorkflow.run = noop_tx_run

        try:
            async with Worker(
                env.client,
                task_queue="test-espn-sync",
                workflows=[
                    LeagueESPNSyncWorkflow,
                    ESPNTeamSyncWorkflow,
                    ESPNScheduleSyncWorkflow,
                    ESPNDraftSyncWorkflow,
                    ESPNTransactionSyncWorkflow,
                ],
                activities=[mock_get_credentials],
            ):
                await env.client.execute_workflow(
                    LeagueESPNSyncWorkflow.run,
                    LeagueDispatchParams(espn_league_id="123", year=2025),
                    id="test-league-sync-123",
                    task_queue="test-espn-sync",
                )
        finally:
            ESPNTeamSyncWorkflow.run = original_teams
            ESPNScheduleSyncWorkflow.run = original_schedule
            ESPNDraftSyncWorkflow.run = original_draft
            ESPNTransactionSyncWorkflow.run = original_tx
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd v2/workers/espn
uv run pytest tests/test_league_sync.py -v
```
Expected: `ImportError` — `workflows.league_sync` not defined.

- [ ] **Step 3: Write workflows/league_sync.py**

```python
# v2/workers/espn/workflows/league_sync.py
from dataclasses import dataclass
from datetime import timedelta
from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities.credentials import get_espn_credentials
    from activities.teams import ESPNLeagueSyncParams
    from workflows.draft import ESPNDraftSyncWorkflow
    from workflows.schedule import ESPNScheduleSyncWorkflow
    from workflows.teams import ESPNTeamSyncWorkflow
    from workflows.transactions import ESPNTransactionSyncWorkflow

TASK_QUEUE = "espn-sync"
_SHORT = dict(start_to_close_timeout=timedelta(seconds=30))


@dataclass
class LeagueDispatchParams:
    espn_league_id: str
    year: int


@workflow.defn
class LeagueESPNSyncWorkflow:
    @workflow.run
    async def run(self, params: LeagueDispatchParams) -> None:
        creds = await workflow.execute_activity(
            get_espn_credentials, params.espn_league_id, **_SHORT
        )
        sync_params = ESPNLeagueSyncParams(
            espn_league_id=params.espn_league_id,
            year=params.year,
            espn_s2=creds.espn_s2,
            swid=creds.swid,
        )

        # Start four child workflows in parallel (fire-and-forget with ABANDON policy).
        # await workflow.start_child_workflow() waits only for the child to be registered,
        # not for it to complete. With ABANDON, children continue if this workflow exits.
        child_configs = [
            (ESPNTeamSyncWorkflow, "teams"),
            (ESPNScheduleSyncWorkflow, "schedule"),
            (ESPNDraftSyncWorkflow, "draft"),
            (ESPNTransactionSyncWorkflow, "transactions"),
        ]
        for child_cls, suffix in child_configs:
            try:
                await workflow.start_child_workflow(
                    child_cls.run,
                    sync_params,
                    id=f"espn-{suffix}-{params.espn_league_id}-{params.year}",
                    task_queue=TASK_QUEUE,
                    parent_close_policy=workflow.ParentClosePolicy.ABANDON,
                )
            except Exception as exc:
                workflow.logger.warning(
                    "Failed to start %s child for league %s: %s",
                    suffix, params.espn_league_id, exc,
                )
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd v2/workers/espn
uv run pytest tests/test_league_sync.py -v
```
Expected: 1 test passes.

- [ ] **Step 5: Commit**

```bash
git add v2/workers/espn/workflows/league_sync.py v2/workers/espn/tests/test_league_sync.py
git commit -m "feat(espn): add LeagueESPNSyncWorkflow"
```

---

### Task 10: ESPNSyncDispatcher

**Files:**
- Create: `v2/workers/espn/workflows/dispatcher.py`
- Create: `v2/workers/espn/tests/test_dispatcher.py`

**Interfaces:**
- Consumes: `get_espn_leagues` from `activities/credentials.py`; `LeagueDispatchParams` and `LeagueESPNSyncWorkflow` from `workflows/league_sync.py`; `ESPNPlayerStatusSyncWorkflow` from `workflows/player_status.py`
- Produces:
  - `@workflow.defn class ESPNSyncDispatcher` with `run(self, year: int | None = None) -> None`

- [ ] **Step 1: Write the failing tests**

```python
# v2/workers/espn/tests/test_dispatcher.py
import pytest
from temporalio.testing import WorkflowEnvironment
from temporalio.worker import Worker

from workflows.dispatcher import ESPNSyncDispatcher
from workflows.league_sync import LeagueDispatchParams, LeagueESPNSyncWorkflow
from workflows.player_status import ESPNPlayerStatusSyncWorkflow


@pytest.mark.asyncio
async def test_dispatcher_completes_with_no_leagues():
    """Dispatcher should complete cleanly when there are no ESPN leagues registered."""
    async with await WorkflowEnvironment.start_time_skipping() as env:

        def mock_get_espn_leagues(year: int) -> list[str]:
            return []

        async def noop_league_run(params: LeagueDispatchParams) -> None:
            pass

        async def noop_player_run(year: int) -> None:
            pass

        original_league = LeagueESPNSyncWorkflow.run
        original_player = ESPNPlayerStatusSyncWorkflow.run
        LeagueESPNSyncWorkflow.run = noop_league_run
        ESPNPlayerStatusSyncWorkflow.run = noop_player_run

        try:
            async with Worker(
                env.client,
                task_queue="test-espn-sync",
                workflows=[ESPNSyncDispatcher, LeagueESPNSyncWorkflow, ESPNPlayerStatusSyncWorkflow],
                activities=[mock_get_espn_leagues],
            ):
                await env.client.execute_workflow(
                    ESPNSyncDispatcher.run,
                    id="test-dispatcher-empty",
                    task_queue="test-espn-sync",
                )
        finally:
            LeagueESPNSyncWorkflow.run = original_league
            ESPNPlayerStatusSyncWorkflow.run = original_player
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd v2/workers/espn
uv run pytest tests/test_dispatcher.py -v
```
Expected: `ImportError`.

- [ ] **Step 3: Write workflows/dispatcher.py**

```python
# v2/workers/espn/workflows/dispatcher.py
from datetime import timedelta
from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities.credentials import get_espn_leagues
    from workflows.league_sync import LeagueDispatchParams, LeagueESPNSyncWorkflow
    from workflows.player_status import ESPNPlayerStatusSyncWorkflow

TASK_QUEUE = "espn-sync"
_SHORT = dict(start_to_close_timeout=timedelta(seconds=30))


@workflow.defn
class ESPNSyncDispatcher:
    @workflow.run
    async def run(self, year: int | None = None) -> None:
        # workflow.now() is deterministic — safe to use in workflow code
        effective_year = year if year is not None else workflow.now().year

        league_ids: list[str] = await workflow.execute_activity(
            get_espn_leagues, effective_year, **_SHORT
        )

        # Spawn the global player status workflow once per dispatch cycle
        try:
            await workflow.start_child_workflow(
                ESPNPlayerStatusSyncWorkflow.run,
                effective_year,
                id=f"espn-player-status-{effective_year}",
                task_queue=TASK_QUEUE,
                parent_close_policy=workflow.ParentClosePolicy.ABANDON,
            )
        except Exception as exc:
            workflow.logger.warning("Failed to start ESPNPlayerStatusSyncWorkflow: %s", exc)

        # Spawn one LeagueESPNSyncWorkflow per registered ESPN league
        for league_id in league_ids:
            try:
                await workflow.start_child_workflow(
                    LeagueESPNSyncWorkflow.run,
                    LeagueDispatchParams(espn_league_id=league_id, year=effective_year),
                    id=f"espn-league-{league_id}-{effective_year}",
                    task_queue=TASK_QUEUE,
                    parent_close_policy=workflow.ParentClosePolicy.ABANDON,
                )
            except Exception as exc:
                workflow.logger.warning(
                    "Failed to start LeagueESPNSyncWorkflow for %s: %s", league_id, exc
                )
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd v2/workers/espn
uv run pytest tests/test_dispatcher.py -v
```
Expected: 1 test passes.

- [ ] **Step 5: Run the full test suite**

```bash
cd v2/workers/espn
uv run pytest -v
```
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add v2/workers/espn/workflows/dispatcher.py v2/workers/espn/tests/test_dispatcher.py
git commit -m "feat(espn): add ESPNSyncDispatcher"
```

---

### Task 11: Worker Entry Point

**Files:**
- Create: `v2/workers/espn/worker.py`

**Interfaces:**
- Consumes: all workflows and activities defined in Tasks 3–10
- Produces: runnable Python Temporal worker process with schedule auto-registration

- [ ] **Step 1: Write worker.py**

```python
# v2/workers/espn/worker.py
"""
ESPN Temporal worker entry point.

Connects to Temporal Cloud (or local dev server), registers all ESPN workflows
and activities on the 'espn-sync' task queue, creates the weekly Tuesday schedule
(idempotent), then polls indefinitely.

Environment variables (Temporal Cloud):
  TEMPORAL_NAMESPACE_ENDPOINT  e.g. ff-sims.b3i2g.tmprl-test.cloud:7233
  TEMPORAL_NAMESPACE           e.g. ff-sims.b3i2g
  TEMPORAL_API_KEY             API key for authentication

Environment variables (local dev server fallback):
  TEMPORAL_HOST                default localhost:7233
  TEMPORAL_NAMESPACE           default "default"

Database:
  DATABASE_URL                 PostgreSQL connection string
"""
import asyncio
import logging
import os
from concurrent.futures import ThreadPoolExecutor
from datetime import timedelta

from dotenv import load_dotenv
from temporalio.client import (
    Client,
    Schedule,
    ScheduleActionStartWorkflow,
    ScheduleAlreadyRunningError,
    ScheduleCalendarSpec,
    ScheduleRange,
    ScheduleSpec,
)
from temporalio.worker import Worker

from activities.credentials import get_any_espn_credentials, get_espn_credentials, get_espn_leagues
from activities.draft import fetch_and_upsert_draft, mark_draft_fetched
from activities.player_status import mark_players_updated, update_active_players
from activities.schedule import fetch_and_upsert_schedule, mark_schedule_fetched
from activities.teams import fetch_and_upsert_teams, mark_teams_fetched
from activities.transactions import fetch_and_upsert_transactions, mark_transactions_fetched
from workflows.dispatcher import ESPNSyncDispatcher
from workflows.draft import ESPNDraftSyncWorkflow
from workflows.league_sync import LeagueESPNSyncWorkflow
from workflows.player_status import ESPNPlayerStatusSyncWorkflow
from workflows.schedule import ESPNScheduleSyncWorkflow
from workflows.teams import ESPNTeamSyncWorkflow
from workflows.transactions import ESPNTransactionSyncWorkflow

load_dotenv()
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

TASK_QUEUE = "espn-sync"
SCHEDULE_ID = "espn-sync-schedule"


async def create_client() -> Client:
    if endpoint := os.getenv("TEMPORAL_NAMESPACE_ENDPOINT"):
        return await Client.connect(
            endpoint,
            namespace=os.environ["TEMPORAL_NAMESPACE"],
            tls=True,
            api_key=os.getenv("TEMPORAL_API_KEY"),
        )
    return await Client.connect(
        os.getenv("TEMPORAL_HOST", "localhost:7233"),
        namespace=os.getenv("TEMPORAL_NAMESPACE", "default"),
    )


async def register_schedule(client: Client) -> None:
    """Create the weekly ESPN sync schedule (idempotent — skips if already exists)."""
    try:
        await client.create_schedule(
            SCHEDULE_ID,
            Schedule(
                action=ScheduleActionStartWorkflow(
                    ESPNSyncDispatcher.run,
                    id="espn-sync-dispatcher",
                    task_queue=TASK_QUEUE,
                ),
                spec=ScheduleSpec(
                    calendars=[
                        ScheduleCalendarSpec(
                            # Tuesday = 2 (0=Sunday, 1=Monday, 2=Tuesday)
                            day_of_week=[ScheduleRange(2)],
                            hour=[ScheduleRange(13)],   # 13:00 UTC = 8:00 AM EST
                            minute=[ScheduleRange(0)],
                        )
                    ]
                ),
            ),
        )
        logger.info("Registered schedule %s", SCHEDULE_ID)
    except Exception as exc:
        # Schedule already exists — leave it unchanged
        logger.info("Schedule %s already exists, skipping: %s", SCHEDULE_ID, exc)


async def main() -> None:
    client = await create_client()
    await register_schedule(client)

    all_activities = [
        get_espn_leagues,
        get_espn_credentials,
        get_any_espn_credentials,
        fetch_and_upsert_teams,
        mark_teams_fetched,
        fetch_and_upsert_schedule,
        mark_schedule_fetched,
        fetch_and_upsert_draft,
        mark_draft_fetched,
        fetch_and_upsert_transactions,
        mark_transactions_fetched,
        update_active_players,
        mark_players_updated,
    ]

    all_workflows = [
        ESPNSyncDispatcher,
        LeagueESPNSyncWorkflow,
        ESPNTeamSyncWorkflow,
        ESPNScheduleSyncWorkflow,
        ESPNDraftSyncWorkflow,
        ESPNTransactionSyncWorkflow,
        ESPNPlayerStatusSyncWorkflow,
    ]

    with ThreadPoolExecutor(max_workers=20) as executor:
        worker = Worker(
            client,
            task_queue=TASK_QUEUE,
            workflows=all_workflows,
            activities=all_activities,
            activity_executor=executor,
        )
        logger.info("ESPN Temporal worker started on task queue '%s'", TASK_QUEUE)
        await worker.run()


if __name__ == "__main__":
    asyncio.run(main())
```

- [ ] **Step 2: Verify worker starts (dry run against local Temporal dev server)**

Start a local dev server if not already running:
```bash
temporal server start-dev &
```

Then:
```bash
cd v2/workers/espn
DATABASE_URL=postgresql://postgres@localhost:5432/ffsims uv run python worker.py
```
Expected: `ESPN Temporal worker started on task queue 'espn-sync'` — no crash. Ctrl-C to stop.

- [ ] **Step 3: Commit**

```bash
git add v2/workers/espn/worker.py
git commit -m "feat(espn): add worker entry point with schedule registration"
```

---

### Task 12: Dockerfile Integration

**Files:**
- Modify: `v2/Dockerfile`

**Interfaces:**
- Consumes: `v2/workers/espn/` Python project from Task 2–11
- Produces: single Docker image running Go HTTP server + Go Sleeper worker + Python ESPN worker

- [ ] **Step 1: Read the current Dockerfile**

Read `v2/Dockerfile` in full before editing to understand current stage names and COPY paths.

- [ ] **Step 2: Add the Python build stage and update the final image**

Add a new `espn-worker-builder` stage after the existing `backend-builder` stage, and extend the final `FROM alpine:latest` stage. The diff is:

```dockerfile
# Insert this new stage after the "backend-builder" stage and before "FROM alpine:latest":

# Stage 3: Build Python ESPN worker
FROM python:3.12-slim AS espn-worker-builder
WORKDIR /app/workers/espn
RUN pip install --no-cache-dir uv
COPY workers/espn/pyproject.toml workers/espn/uv.lock ./
RUN uv sync --frozen --no-dev
COPY workers/espn/ ./
```

In the final `FROM alpine:latest` stage, add after the existing `COPY --from=backend-builder` lines:

```dockerfile
# Copy Python ESPN worker and its virtualenv
COPY --from=espn-worker-builder /app/workers/espn /app/workers/espn
# Copy Python runtime from the builder (avoids installing Python in alpine separately)
COPY --from=espn-worker-builder /usr/local/lib/python3.12 /usr/local/lib/python3.12
COPY --from=espn-worker-builder /usr/local/bin/python3.12 /usr/local/bin/python3.12
COPY --from=espn-worker-builder /usr/local/bin/uv /usr/local/bin/uv
RUN ln -sf /usr/local/bin/python3.12 /usr/local/bin/python3
```

Replace the existing `RUN printf ... /entrypoint.sh` line with:

```dockerfile
# Entrypoint: Sleeper Go worker + ESPN Python worker + HTTP server
RUN printf '#!/bin/sh\n\
/app/backend/worker &\n\
cd /app/workers/espn && uv run python worker.py &\n\
exec /app/backend/main\n' > /entrypoint.sh && chmod +x /entrypoint.sh
```

- [ ] **Step 3: Build and verify the image**

```bash
cd v2
docker build -t ff-sims-v2:test .
```
Expected: build completes without error. All three stages complete.

- [ ] **Step 4: Smoke-test the container starts**

```bash
docker run --rm \
  -e DATABASE_URL=postgresql://postgres@host.docker.internal:5432/ffsims \
  -e TEMPORAL_HOST=host.docker.internal:7233 \
  -p 8080:8080 \
  ff-sims-v2:test
```
Expected: container starts, logs show Go HTTP server on 8080, Go Sleeper worker, and Python ESPN worker all running. Ctrl-C to stop.

- [ ] **Step 5: Commit**

```bash
git add v2/Dockerfile
git commit -m "feat(espn): add Python ESPN worker to Dockerfile"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Task covering it |
|---|---|
| `espn_league_credentials` table | Task 1 |
| Python project at `v2/workers/espn/` | Task 2 |
| Credentials activities | Task 3 |
| Teams child workflow | Task 4 |
| Schedule/box score child workflow | Task 5 |
| Draft child workflow | Task 6 |
| Transactions child workflow | Task 7 |
| Player status — global, once per cycle | Task 8 |
| `LeagueESPNSyncWorkflow` fanning out 4 children | Task 9 |
| `ESPNSyncDispatcher` + weekly schedule | Task 10 |
| Worker entry point + schedule registration | Task 11 |
| Dockerfile multi-language container | Task 12 |
| Manual backfill documented | Global Constraints header |
| Direct DB writes (no JSON files) | All activity tasks |
| Idempotency via check-before-act | Tasks 4–8 (all fetch activities) |
| `year` defaults to current year | Task 10 (`workflow.now().year`) |

**Placeholder scan:** None found — all steps contain concrete code.

**Type consistency:**
- `ESPNLeagueSyncParams` defined in Task 4, consumed by Tasks 5, 6, 7, 9 — consistent.
- `ESPNCredentials` defined in Task 3, consumed by Task 9 — consistent.
- `AnyESPNCredentials` defined in Task 3, consumed by Task 8 — consistent.
- `LeagueDispatchParams` defined in Task 9, consumed by Task 10 — consistent.
- `PlayerStatusParams` defined in Task 8, consumed internally — consistent.
- `mark_players_updated` takes no arguments (Task 8) — consistent with Task 11 registration.
