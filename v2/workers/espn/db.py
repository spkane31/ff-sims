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
