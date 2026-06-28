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
