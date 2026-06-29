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
