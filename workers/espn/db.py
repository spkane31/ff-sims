import os
from pathlib import Path
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit

import psycopg
from dotenv import load_dotenv

# Walk up from this file to find the backend .env (v2/backend/.env)
_here = Path(__file__).parent
for _p in [_here, _here.parent, _here.parent.parent]:
    _env = _p / "backend" / ".env"
    if _env.exists():
        load_dotenv(_env, override=False)
        break
else:
    load_dotenv(override=False)


# pgx (the Go worker's driver) understands this param for routing through
# DigitalOcean's pgbouncer pool (see docs/transaction-sync-operations.md), but
# libpq/psycopg doesn't recognize it and aborts the connection entirely with
# "invalid URI query parameter" — even though both workers share the same
# DATABASE_URL from /etc/ff-sims-worker.env.
_PGX_ONLY_PARAMS = {"default_query_exec_mode"}


def _strip_pgx_only_params(url: str) -> str:
    parts = urlsplit(url)
    query = [(k, v) for k, v in parse_qsl(parts.query, keep_blank_values=True) if k not in _PGX_ONLY_PARAMS]
    return urlunsplit(parts._replace(query=urlencode(query)))


def get_connection() -> psycopg.Connection:
    # TEST_DATABASE_URL takes precedence so tests can never fall through to
    # production even if only DATABASE_URL is set in the environment —
    # matches tests/conftest.py's db_conn fixture precedence.
    url = os.environ.get("TEST_DATABASE_URL", os.environ["DATABASE_URL"])
    return psycopg.connect(_strip_pgx_only_params(url))


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
