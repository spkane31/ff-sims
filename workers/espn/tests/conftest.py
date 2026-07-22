import os
from pathlib import Path
import pytest
import psycopg
from dotenv import load_dotenv

# Load from v2/backend/.env (two levels up from tests/)
_env_path = Path(__file__).parent.parent.parent.parent / "backend" / ".env"
if _env_path.exists():
    load_dotenv(_env_path, override=False)
else:
    load_dotenv(override=False)


@pytest.fixture
def db_conn():
    # Only ever read TEST_DATABASE_URL, never fall back to DATABASE_URL — these
    # tests commit real writes (seed helpers and the activities under test both
    # call conn.commit() directly, so the teardown rollback() below can't undo
    # them). A previous fallback-to-DATABASE_URL here let a local run with only
    # DATABASE_URL set in the shell silently commit test data — including an
    # unscoped `UPDATE players SET status = 'inactive'` from
    # test_player_status.py — straight to production. Mirrors how the Go
    # suite's Postgres-backed tests (e.g. discoverycron/claim_pg_test.go) only
    # ever read TEST_DATABASE_URL and skip otherwise.
    url = os.environ.get("TEST_DATABASE_URL")
    if not url:
        pytest.skip("TEST_DATABASE_URL not set; these tests commit real writes and must never run against DATABASE_URL")
    conn = psycopg.connect(url)
    yield conn
    conn.rollback()
    conn.close()
