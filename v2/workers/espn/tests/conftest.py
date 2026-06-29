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
    url = os.environ.get("TEST_DATABASE_URL", os.environ["DATABASE_URL"])
    conn = psycopg.connect(url)
    yield conn
    conn.rollback()
    conn.close()
