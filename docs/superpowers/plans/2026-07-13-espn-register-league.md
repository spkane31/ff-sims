# ESPN register-league CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `uv run register-league` CLI in `workers/espn/` that validates an ESPN league's credentials against the live ESPN API and writes the `leagues` row and `espn_league_credentials` row together in one transaction, so they can never end up split the way they did in a real production incident.

**Architecture:** A new `workers/espn/register_league.py` module with four small, independently-testable functions (ESPN validation, DB upsert, Temporal workflow kickoff, argparse `main()`), plus a `workers/espn/temporal_client.py` module extracted from `worker.py` so both share one Temporal-connection implementation. Ships as a `uv run register-league` entry point.

**Tech Stack:** Python 3.12, `espn_api`, `psycopg[binary]` 3.x, `temporalio`, stdlib `argparse`/`asyncio`, `pytest` + `pytest-asyncio` (already `asyncio_mode = "auto"`).

## Global Constraints

- Working directory for all commands in this plan: `/Users/seankane/github.com/ff-sims/.claude/worktrees/espn-register-league/workers/espn` (already on branch `worktree-espn-register-league`).
- Every test run in this plan **must** set both `DATABASE_URL` and `TEST_DATABASE_URL` to `postgresql://postgres@localhost:5432/ffsims` (a local Postgres instance already confirmed to have the full schema, including `leagues` and `espn_league_credentials`). Never let a test run resolve to production — this is precisely the class of incident this feature exists to prevent.
- `workers/espn/db.py`'s `get_connection()` already prefers `TEST_DATABASE_URL` over `DATABASE_URL` (fixed and committed in `018575f`, spec commit) — this is a completed prerequisite, verified in Task 1, not re-implemented.
- No new runtime dependency: use stdlib `argparse`, not `click`/`typer`.
- `leagues` upsert conflict target is the existing partial unique index `idx_leagues_platform_external_id` on `(platform, external_id) WHERE platform != '' AND external_id != ''` (migration `backend/migrations/003_add_league_platform_external_id.sql`) — any `ON CONFLICT` clause must match it exactly.
- `espn_league_credentials` upsert conflict target is its primary key, `espn_league_id` (migration `backend/migrations/011_add_espn_league_credentials.sql`).

---

### Task 1: Verify baseline (prerequisite fix already applied)

**Files:**
- Read only: `workers/espn/db.py`, `workers/espn/tests/conftest.py`

**Interfaces:**
- Consumes: nothing new.
- Produces: confirmation that `TEST_DATABASE_URL` isolation works end-to-end, which every later task's test runs depend on.

- [ ] **Step 1: Confirm `db.py`'s fix is present**

Run: `grep -n "TEST_DATABASE_URL" db.py`
Expected output includes:
```
    url = os.environ.get("TEST_DATABASE_URL", os.environ["DATABASE_URL"])
```
If this line is missing, stop and re-apply it before continuing — every later task's tests assume it's there.

- [ ] **Step 2: Run the full existing test suite against the local DB only**

Run:
```bash
DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" \
TEST_DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" \
uv run --no-sync python -m pytest -q
```
Expected: `6 failed, 31 passed` — the 6 failures are pre-existing (`RuntimeError: Not in activity context` from `activity.heartbeat()` calls outside a real Temporal activity context, in `test_expected_wins.py` and `test_schedule.py`), unrelated to this feature. If the failure count or failing test names differ from this, stop and investigate before continuing.

No commit for this task — it's verification only.

---

### Task 2: Extract shared Temporal client into `temporal_client.py`

**Files:**
- Create: `workers/espn/temporal_client.py`
- Modify: `workers/espn/worker.py`
- Test: `workers/espn/tests/test_temporal_client.py`

**Interfaces:**
- Produces: `temporal_client.create_client() -> Awaitable[Client]` — used by Task 4's `register_league.py`.

- [ ] **Step 1: Write the failing test**

Create `workers/espn/tests/test_temporal_client.py`:
```python
import temporal_client
import worker


def test_worker_reuses_shared_create_client():
    """worker.py must not define its own copy of create_client — both modules
    share the same implementation, or the TLS cert-chain handling for
    Temporal Cloud's custom-CA endpoints would drift between them."""
    assert worker.create_client is temporal_client.create_client
```

- [ ] **Step 2: Run test to verify it fails**

Run: `DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" TEST_DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" uv run --no-sync python -m pytest tests/test_temporal_client.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'temporal_client'`

- [ ] **Step 3: Create `temporal_client.py`**

Create `workers/espn/temporal_client.py`:
```python
"""
Shared Temporal Cloud / local-dev client factory for the ESPN worker package.

Used by both worker.py (the long-running Temporal worker) and
register_league.py (the one-shot CLI) so there's a single implementation of
the TLS cert-chain handling Temporal Cloud's custom-CA endpoints need.

Temporal Cloud env vars:
  TEMPORAL_NAMESPACE_ENDPOINT     e.g. ff-sims.b3i2g.tmprl-test.cloud:7233
  TEMPORAL_NAMESPACE              e.g. ff-sims.b3i2g
  TEMPORAL_API_KEY                API key

Local dev server fallback:
  TEMPORAL_HOST                default localhost:7233
  TEMPORAL_NAMESPACE           default "default"
"""
import os
import re
import subprocess

from temporalio.client import Client
from temporalio.service import TLSConfig


def _fetch_server_tls_config(endpoint: str) -> TLSConfig:
    """Trust whatever cert chain the server presents — equivalent to InsecureSkipVerify=true in Go.

    Uses `openssl s_client -showcerts` to capture every cert in the chain (leaf,
    intermediates, and root CA). Passing the full chain as server_root_ca_cert lets
    rustls build a valid path even when the CA is not in the system trust store, which
    is the common case for tmprl-test.cloud and other custom-CA Temporal environments.

    Python 3.12's ssl module only exposes the leaf cert via getpeercert(), which fails
    as a rustls trust anchor because its CA bit is false. This approach works on any
    Python version and requires openssl to be installed in the container.
    """
    host, port_str = endpoint.rsplit(":", 1)
    result = subprocess.run(
        ["openssl", "s_client", "-connect", f"{host}:{port_str}", "-showcerts"],
        input=b"",
        capture_output=True,
        timeout=10,
    )
    pem_certs = re.findall(
        rb"-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----",
        result.stdout,
        re.DOTALL,
    )
    if not pem_certs:
        raise RuntimeError(
            f"Could not retrieve TLS certificate chain from {endpoint} — "
            "is the endpoint reachable and is openssl installed in the container?"
        )
    return TLSConfig(server_root_ca_cert=b"\n".join(pem_certs))


async def create_client() -> Client:
    if endpoint := os.getenv("TEMPORAL_NAMESPACE_ENDPOINT"):
        return await Client.connect(
            endpoint,
            namespace=os.environ["TEMPORAL_NAMESPACE"],
            tls=_fetch_server_tls_config(endpoint),
            api_key=os.getenv("TEMPORAL_API_KEY"),
        )
    return await Client.connect(
        os.getenv("TEMPORAL_HOST", "localhost:7233"),
        namespace=os.getenv("TEMPORAL_NAMESPACE", "default"),
    )
```

- [ ] **Step 4: Update `worker.py` to import from the new module instead of defining it inline**

In `worker.py`, replace the import block (lines 20-37) — remove `import os`, `import re`, `import subprocess`, and `from temporalio.service import TLSConfig` (all now unused in this file), and add an import of the shared client. The full replacement:

```python
import asyncio
import logging
from concurrent.futures import ThreadPoolExecutor

from dotenv import load_dotenv
from temporalio.client import (
    Client,
    Schedule,
    ScheduleActionStartWorkflow,
    ScheduleCalendarSpec,
    ScheduleRange,
    ScheduleSpec,
)
from temporalio.worker import Worker

from activities.credentials import get_any_espn_credentials, get_espn_credentials, get_espn_leagues
from activities.draft import fetch_and_upsert_draft, mark_draft_fetched
from activities.expected_wins import calculate_and_store_expected_wins, get_matchup_years
from activities.player_status import mark_players_updated, update_active_players
from activities.schedule import fetch_and_upsert_schedule, mark_schedule_fetched
from activities.teams import fetch_and_upsert_teams, mark_teams_fetched
from activities.transactions import fetch_and_upsert_transactions, mark_transactions_fetched
from temporal_client import create_client
from workflows.dispatcher import ESPNSyncDispatcher
from workflows.draft import ESPNDraftSyncWorkflow
from workflows.expected_wins import ExpectedWinsBackfillWorkflow
from workflows.league_sync import LeagueESPNSyncWorkflow
from workflows.player_status import ESPNPlayerStatusSyncWorkflow
from workflows.schedule import ESPNScheduleSyncWorkflow
from workflows.teams import ESPNTeamSyncWorkflow
from workflows.transactions import ESPNTransactionSyncWorkflow
```

Then delete the now-duplicated `_fetch_server_tls_config` function definition and `create_client` function definition from `worker.py` entirely (they were at lines 63-105 in the original file, immediately before `async def register_schedule(client: Client) -> None:`) — `create_client` is now only imported, not redefined. Everything from `TASK_QUEUE = "espn-sync"` / `SCHEDULE_ID = "espn-sync-schedule"` through the rest of the file (`register_schedule`, `main`, the `if __name__ == "__main__":` block) stays exactly as-is.

- [ ] **Step 5: Run test to verify it passes**

Run: `DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" TEST_DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" uv run --no-sync python -m pytest tests/test_temporal_client.py -v`
Expected: PASS

- [ ] **Step 6: Run the full suite to confirm no regression**

Run: `DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" TEST_DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" uv run --no-sync python -m pytest -q`
Expected: `6 failed, 32 passed` (same 6 pre-existing failures as Task 1, plus the 1 new passing test)

- [ ] **Step 7: Commit**

```bash
git add workers/espn/temporal_client.py workers/espn/worker.py workers/espn/tests/test_temporal_client.py
git commit -m "$(cat <<'EOF'
Extract shared Temporal client factory into temporal_client.py

worker.py and the new register_league.py both need a Temporal Cloud
client with the same custom-CA TLS handling; extracting it avoids
duplicating that logic.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: `register_league.py` — ESPN validation + DB upsert

**Files:**
- Create: `workers/espn/register_league.py`
- Test: `workers/espn/tests/test_register_league.py`

**Interfaces:**
- Consumes: `db.get_connection() -> psycopg.Connection` (existing, from Task 1's verified `db.py`).
- Produces: `validate_and_fetch_name(league_id: str, year: int, espn_s2: str, swid: str) -> str`, `upsert_league_and_credentials(conn, name: str, league_id: str, espn_s2: str, swid: str) -> tuple[int, bool]` (returns `(internal_leagues_id, was_inserted)`) — both used by Task 4's `main()`.

- [ ] **Step 1: Write the failing tests**

Create `workers/espn/tests/test_register_league.py`:
```python
from unittest.mock import MagicMock, patch

import pytest

from register_league import upsert_league_and_credentials, validate_and_fetch_name


def _mock_league(name: str) -> MagicMock:
    league = MagicMock()
    league.settings.name = name
    return league


def _clear_league(conn, external_id: str) -> None:
    with conn.cursor() as cur:
        cur.execute("DELETE FROM espn_league_credentials WHERE espn_league_id = %s", (external_id,))
        cur.execute("DELETE FROM leagues WHERE external_id = %s", (external_id,))
    conn.commit()


def test_validate_and_fetch_name_returns_league_name():
    with patch("register_league.League", return_value=_mock_league("My League")):
        name = validate_and_fetch_name("5001", 2025, "s2", "swid")
    assert name == "My League"


def test_validate_and_fetch_name_raises_on_bad_credentials():
    with patch("register_league.League", side_effect=RuntimeError("401 Unauthorized")):
        with pytest.raises(RuntimeError):
            validate_and_fetch_name("5001", 2025, "bad-s2", "bad-swid")


def test_upsert_creates_new_league_and_credentials(db_conn):
    _clear_league(db_conn, "5001")

    internal_id, was_inserted = upsert_league_and_credentials(
        db_conn, "New League", "5001", "s2-value", "swid-value"
    )
    assert was_inserted is True

    with db_conn.cursor() as cur:
        cur.execute("SELECT name, platform, external_id FROM leagues WHERE id = %s", (internal_id,))
        assert cur.fetchone() == ("New League", "ESPN", "5001")

        cur.execute(
            "SELECT espn_s2, swid FROM espn_league_credentials WHERE espn_league_id = %s", ("5001",)
        )
        assert cur.fetchone() == ("s2-value", "swid-value")


def test_upsert_updates_existing_league_and_credentials(db_conn):
    _clear_league(db_conn, "5002")

    first_id, first_inserted = upsert_league_and_credentials(
        db_conn, "Original Name", "5002", "old-s2", "old-swid"
    )
    assert first_inserted is True

    second_id, second_inserted = upsert_league_and_credentials(
        db_conn, "Renamed League", "5002", "new-s2", "new-swid"
    )
    assert second_inserted is False
    assert second_id == first_id

    with db_conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) FROM leagues WHERE external_id = %s", ("5002",))
        assert cur.fetchone()[0] == 1

        cur.execute("SELECT name FROM leagues WHERE id = %s", (first_id,))
        assert cur.fetchone()[0] == "Renamed League"

        cur.execute(
            "SELECT espn_s2, swid FROM espn_league_credentials WHERE espn_league_id = %s", ("5002",)
        )
        assert cur.fetchone() == ("new-s2", "new-swid")


def test_validation_failure_writes_nothing(db_conn):
    _clear_league(db_conn, "5003")

    with patch("register_league.League", side_effect=RuntimeError("401 Unauthorized")):
        with pytest.raises(RuntimeError):
            validate_and_fetch_name("5003", 2025, "bad-s2", "bad-swid")

    with db_conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) FROM leagues WHERE external_id = %s", ("5003",))
        assert cur.fetchone()[0] == 0
        cur.execute(
            "SELECT COUNT(*) FROM espn_league_credentials WHERE espn_league_id = %s", ("5003",)
        )
        assert cur.fetchone()[0] == 0
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" TEST_DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" uv run --no-sync python -m pytest tests/test_register_league.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'register_league'`

- [ ] **Step 3: Write the minimal implementation**

Create `workers/espn/register_league.py`:
```python
"""
register-league — onboard or re-authenticate an ESPN league in one step.

Validates the given league ID + ESPN_S2/SWID cookies against the live ESPN
API, then writes both the `leagues` row (platform='ESPN') and the
`espn_league_credentials` row in a single transaction — so the two can never
end up split the way they did in the incident this tool was built to prevent
(credentials rotated, `leagues` row missing, sync silently broken forever).

Usage:
  uv run register-league --league-id 345674 --espn-s2 '...' --swid '{...}'
  uv run register-league --league-id 345674 --espn-s2 '...' --swid '{...}' --year 2025
  uv run register-league --league-id 345674 --espn-s2 '...' --swid '{...}' --no-sync
"""
from espn_api.football import League

from db import get_connection


def validate_and_fetch_name(league_id: str, year: int, espn_s2: str, swid: str) -> str:
    """Confirm the league is reachable with these credentials; return its real name."""
    league = League(league_id=int(league_id), year=year, espn_s2=espn_s2, swid=swid)
    return league.settings.name


def upsert_league_and_credentials(
    conn, name: str, league_id: str, espn_s2: str, swid: str
) -> tuple[int, bool]:
    """Upsert both the leagues row and the credentials row in one transaction.

    Returns (internal leagues.id, was_inserted) — was_inserted is True only
    when this call created a brand-new leagues row, False when it updated an
    existing one.
    """
    with conn.cursor() as cur:
        cur.execute(
            """
            INSERT INTO leagues (name, platform, external_id, created_at, updated_at)
            VALUES (%s, 'ESPN', %s, NOW(), NOW())
            ON CONFLICT (platform, external_id) WHERE platform != '' AND external_id != ''
            DO UPDATE SET name = EXCLUDED.name, updated_at = NOW()
            RETURNING id, (xmax = 0) AS inserted
            """,
            (name, league_id),
        )
        internal_id, was_inserted = cur.fetchone()

        cur.execute(
            """
            INSERT INTO espn_league_credentials (espn_league_id, espn_s2, swid)
            VALUES (%s, %s, %s)
            ON CONFLICT (espn_league_id) DO UPDATE
                SET espn_s2 = EXCLUDED.espn_s2, swid = EXCLUDED.swid, updated_at = NOW()
            """,
            (league_id, espn_s2, swid),
        )
    conn.commit()
    return internal_id, was_inserted
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" TEST_DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" uv run --no-sync python -m pytest tests/test_register_league.py -v`
Expected: PASS (5 passed)

- [ ] **Step 5: Run the full suite to confirm no regression**

Run: `DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" TEST_DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" uv run --no-sync python -m pytest -q`
Expected: `6 failed, 37 passed`

- [ ] **Step 6: Commit**

```bash
git add workers/espn/register_league.py workers/espn/tests/test_register_league.py
git commit -m "$(cat <<'EOF'
Add register_league validation + DB upsert core

Validates ESPN league credentials against the live API before writing
anything, then upserts the leagues row and espn_league_credentials row
in one transaction — the two can no longer end up split.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: `register_league.py` — Temporal kickoff + CLI wiring

**Files:**
- Modify: `workers/espn/register_league.py`
- Test: `workers/espn/tests/test_register_league.py`

**Interfaces:**
- Consumes: `temporal_client.create_client` (Task 2), `validate_and_fetch_name`/`upsert_league_and_credentials` (Task 3), `workflows.league_sync.LeagueDispatchParams`/`LeagueESPNSyncWorkflow` (existing).
- Produces: `start_sync_workflow(league_id: str, year: int) -> Awaitable[str]` (returns the workflow ID), `main() -> None` (the CLI entry point, parses `sys.argv`).

- [ ] **Step 1: Write the failing tests**

Append to `workers/espn/tests/test_register_league.py` (add these imports at the top alongside the existing ones, then add the new tests at the end of the file):
```python
import sys

import register_league
```

New tests, appended to the end of the file:
```python
async def test_start_sync_workflow_calls_temporal_with_expected_id():
    mock_client = MagicMock()

    async def fake_start_workflow(*args, **kwargs):
        return None

    mock_client.start_workflow = fake_start_workflow

    async def fake_create_client():
        return mock_client

    with patch("register_league.create_client", fake_create_client):
        workflow_id = await register_league.start_sync_workflow("345674", 2025)

    assert workflow_id == "espn-league-345674-2025"


def test_main_no_sync_registers_league_without_starting_workflow(db_conn, monkeypatch, capsys):
    _clear_league(db_conn, "5004")
    monkeypatch.setattr(
        sys, "argv",
        ["register-league", "--league-id", "5004", "--espn-s2", "s2v", "--swid", "swidv", "--no-sync"],
    )

    with patch("register_league.League", return_value=_mock_league("No Sync League")):
        register_league.main()

    captured = capsys.readouterr()
    assert "Registered new league" in captured.out
    assert "Started sync workflow" not in captured.out

    with db_conn.cursor() as cur:
        cur.execute("SELECT name FROM leagues WHERE external_id = %s", ("5004",))
        assert cur.fetchone()[0] == "No Sync League"


def test_main_with_sync_starts_workflow(db_conn, monkeypatch, capsys):
    _clear_league(db_conn, "5005")
    monkeypatch.setattr(
        sys, "argv",
        ["register-league", "--league-id", "5005", "--espn-s2", "s2v", "--swid", "swidv"],
    )

    async def fake_start_sync_workflow(league_id, year):
        return f"espn-league-{league_id}-{year}"

    with patch("register_league.League", return_value=_mock_league("Synced League")), \
         patch("register_league.start_sync_workflow", fake_start_sync_workflow):
        register_league.main()

    captured = capsys.readouterr()
    assert "Registered new league" in captured.out
    assert "Started sync workflow: espn-league-5005-" in captured.out


def test_main_warns_but_does_not_crash_when_workflow_start_fails(db_conn, monkeypatch, capsys):
    _clear_league(db_conn, "5006")
    monkeypatch.setattr(
        sys, "argv",
        ["register-league", "--league-id", "5006", "--espn-s2", "s2v", "--swid", "swidv"],
    )

    async def failing_start_sync_workflow(league_id, year):
        raise RuntimeError("workflow already started")

    with patch("register_league.League", return_value=_mock_league("Warn League")), \
         patch("register_league.start_sync_workflow", failing_start_sync_workflow):
        register_league.main()  # must not raise

    captured = capsys.readouterr()
    assert "Registered new league" in captured.out
    assert "Warning: could not start sync workflow" in captured.err
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" TEST_DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" uv run --no-sync python -m pytest tests/test_register_league.py -v`
Expected: FAIL — `AttributeError: module 'register_league' has no attribute 'start_sync_workflow'` (and `main`)

- [ ] **Step 3: Add the Temporal kickoff and `main()` to `register_league.py`**

Append to `workers/espn/register_league.py` (after the existing imports, add these three; after `upsert_league_and_credentials`, add the rest):
```python
import argparse
import asyncio
import datetime
import sys

from temporal_client import create_client
from workflows.league_sync import LeagueDispatchParams, LeagueESPNSyncWorkflow

TASK_QUEUE = "espn-sync"


async def start_sync_workflow(league_id: str, year: int) -> str:
    """Start LeagueESPNSyncWorkflow for this league/year; return the workflow ID."""
    client = await create_client()
    workflow_id = f"espn-league-{league_id}-{year}"
    await client.start_workflow(
        LeagueESPNSyncWorkflow.run,
        LeagueDispatchParams(espn_league_id=league_id, year=year),
        id=workflow_id,
        task_queue=TASK_QUEUE,
    )
    return workflow_id


def main() -> None:
    parser = argparse.ArgumentParser(description="Register or re-authenticate an ESPN league")
    parser.add_argument("--league-id", required=True, help="ESPN league ID")
    parser.add_argument("--espn-s2", required=True, help="ESPN_S2 cookie value")
    parser.add_argument("--swid", required=True, help="SWID cookie value")
    parser.add_argument(
        "--year", type=int, default=datetime.date.today().year,
        help="Season year to validate against and sync (default: current year)",
    )
    parser.add_argument(
        "--no-sync", action="store_true",
        help="Write the database rows but skip starting the Temporal sync workflow",
    )
    args = parser.parse_args()

    try:
        name = validate_and_fetch_name(args.league_id, args.year, args.espn_s2, args.swid)
    except Exception as exc:
        print(f"Could not reach league {args.league_id}: {exc}", file=sys.stderr)
        sys.exit(1)

    try:
        with get_connection() as conn:
            internal_id, was_inserted = upsert_league_and_credentials(
                conn, name, args.league_id, args.espn_s2, args.swid
            )
    except Exception as exc:
        print(f"Database error while registering league {args.league_id}: {exc}", file=sys.stderr)
        sys.exit(1)

    verb = "Registered new" if was_inserted else "Updated existing"
    print(f"{verb} league: {name!r} (internal id {internal_id}, ESPN id {args.league_id})")

    if args.no_sync:
        return

    try:
        workflow_id = asyncio.run(start_sync_workflow(args.league_id, args.year))
        print(f"Started sync workflow: {workflow_id}")
    except Exception as exc:
        print(f"Warning: could not start sync workflow: {exc}", file=sys.stderr)


if __name__ == "__main__":
    main()
```

The full file's import block (top of `register_league.py`) should now read:
```python
"""
register-league — onboard or re-authenticate an ESPN league in one step.

Validates the given league ID + ESPN_S2/SWID cookies against the live ESPN
API, then writes both the `leagues` row (platform='ESPN') and the
`espn_league_credentials` row in a single transaction — so the two can never
end up split the way they did in the incident this tool was built to prevent
(credentials rotated, `leagues` row missing, sync silently broken forever).

Usage:
  uv run register-league --league-id 345674 --espn-s2 '...' --swid '{...}'
  uv run register-league --league-id 345674 --espn-s2 '...' --swid '{...}' --year 2025
  uv run register-league --league-id 345674 --espn-s2 '...' --swid '{...}' --no-sync
"""
import argparse
import asyncio
import datetime
import sys

from espn_api.football import League

from db import get_connection
from temporal_client import create_client
from workflows.league_sync import LeagueDispatchParams, LeagueESPNSyncWorkflow

TASK_QUEUE = "espn-sync"
```
(i.e. move all imports to the top of the file in this order, rather than having a second import block partway down — `validate_and_fetch_name` and `upsert_league_and_credentials` from Task 3 stay exactly as they were, positioned between the imports and `start_sync_workflow`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" TEST_DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" uv run --no-sync python -m pytest tests/test_register_league.py -v`
Expected: PASS (9 passed)

- [ ] **Step 5: Run the full suite to confirm no regression**

Run: `DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" TEST_DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" uv run --no-sync python -m pytest -q`
Expected: `6 failed, 41 passed`

- [ ] **Step 6: Commit**

```bash
git add workers/espn/register_league.py workers/espn/tests/test_register_league.py
git commit -m "$(cat <<'EOF'
Add Temporal sync kickoff and CLI wiring to register_league

After writing the DB rows, starts LeagueESPNSyncWorkflow immediately
(same workflow ID convention the weekly dispatcher uses, so it's
idempotent with it) unless --no-sync is passed. A failed workflow
start only warns, since the DB rows are already correct by then.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Expose as a `uv run register-league` entry point

**Files:**
- Modify: `workers/espn/pyproject.toml`

**Interfaces:**
- Consumes: `register_league.main` (Task 4).
- Produces: the `register-league` console command.

- [ ] **Step 1: Add the entry point**

In `workers/espn/pyproject.toml`, add a `[project.scripts]` table. The full file should read:
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

[project.scripts]
register-league = "register_league:main"

[dependency-groups]
dev = [
    "pytest>=8.0.0",
    "pytest-asyncio>=0.23.0",
]

[tool.pytest.ini_options]
asyncio_mode = "auto"
testpaths = ["tests"]
```

- [ ] **Step 2: Re-sync and verify the entry point resolves**

Run: `uv sync 2>&1 | tail -5 && uv run --no-sync register-league --help`
Expected: argparse-generated help text listing `--league-id`, `--espn-s2`, `--swid`, `--year`, `--no-sync`, exit code 0. (No network/DB calls happen for `--help`.)

- [ ] **Step 3: Manual end-to-end smoke test against the local DB**

Run:
```bash
DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" \
TEST_DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" \
uv run --no-sync register-league --league-id 999999 --espn-s2 bad --swid bad --no-sync
```
Expected: exits non-zero, prints `Could not reach league 999999: ...` to stderr (999999 is not a real ESPN league, so the live API call fails — this confirms validation genuinely runs before any DB write, using the real `espn_api` library, not a mock). Then confirm nothing was written:
```bash
psql postgresql://postgres@localhost:5432/ffsims -c "SELECT COUNT(*) FROM leagues WHERE external_id = '999999'"
```
Expected: `0`

- [ ] **Step 4: Run the full suite one more time**

Run: `DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" TEST_DATABASE_URL="postgresql://postgres@localhost:5432/ffsims" uv run --no-sync python -m pytest -q`
Expected: `6 failed, 41 passed` (unchanged from Task 4 — this task only touches `pyproject.toml`)

- [ ] **Step 5: Commit**

```bash
git add workers/espn/pyproject.toml
git commit -m "$(cat <<'EOF'
Expose register_league as a uv run register-league entry point

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Push branch and open PR

**Files:** none (git/GitHub operations only)

**Interfaces:** none

- [ ] **Step 1: Confirm the branch's full diff and commit log look right**

Run: `git log --oneline main..HEAD` and `git diff main...HEAD --stat`
Expected: 4 commits from this plan (Tasks 2–5; Task 1 made no commit) on top of the spec commit already made during brainstorming, touching `workers/espn/temporal_client.py`, `workers/espn/worker.py`, `workers/espn/register_league.py`, `workers/espn/tests/test_temporal_client.py`, `workers/espn/tests/test_register_league.py`, `workers/espn/pyproject.toml`, `docs/superpowers/specs/2026-07-13-espn-register-league-design.md`, `docs/superpowers/plans/2026-07-13-espn-register-league.md`, `workers/espn/db.py`.

- [ ] **Step 2: Push the branch**

Run: `git push -u origin worktree-espn-register-league`

- [ ] **Step 3: Open the PR**

Run:
```bash
gh pr create --title "Add ESPN register-league CLI" --body "$(cat <<'EOF'
## Summary
- Adds `uv run register-league` to onboard or re-authenticate an ESPN league in one step: validates credentials against the live ESPN API, then upserts both the `leagues` row and `espn_league_credentials` row in a single transaction.
- Directly fixes the class of incident where a league's credentials were rotated but its `leagues` row didn't exist (or vice versa), silently breaking sync.
- Fixes `workers/espn/db.py`'s `get_connection()` to respect `TEST_DATABASE_URL` like the test fixture already does — without it, running this package's test suite could write real rows into whatever `DATABASE_URL` happens to be set to.
- Extracts the Temporal Cloud client factory (`create_client`/`_fetch_server_tls_config`) out of `worker.py` into a new `temporal_client.py`, shared by both the long-running worker and this new CLI.

## Test plan
- [x] `pytest -q` in `workers/espn` against a local Postgres DB (never production): 41 passed, 6 pre-existing unrelated failures.
- [x] Manual smoke test: `register-league --league-id 999999 --espn-s2 bad --swid bad --no-sync` fails validation against the real ESPN API and writes nothing to the DB.
- [x] `register-league --help` resolves via the new `uv run` entry point.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR created; report the PR URL to the user.

---

## Plan Self-Review

**Spec coverage:** `db.py` fix (Task 1, already done), `temporal_client.py` extraction (Task 2), ESPN validation + DB upsert (Task 3), Temporal kickoff + CLI + error-handling table from the spec (Task 4), `[project.scripts]` entry (Task 5), testing section — new-league/update/bad-credentials/no-DB-write-on-failure all covered (Task 3 + 4), commit+push+PR (Task 6). No spec section without a task.

**Placeholder scan:** no TBD/TODO; every step has literal code or literal commands with expected output.

**Type consistency:** `upsert_league_and_credentials` returns `tuple[int, bool]` in both its Task 3 definition and every Task 3/4 test's unpacking (`internal_id, was_inserted = ...`). `start_sync_workflow(league_id: str, year: int) -> str` matches its Task 4 definition and test usage. `create_client` is imported as `from temporal_client import create_client` in both `worker.py` (Task 2) and `register_league.py` (Task 4), and tests patch it at `register_league.create_client` (the name as imported into that module's namespace) — correct per `unittest.mock.patch` convention (patch where a name is *looked up*, not where it's defined).
