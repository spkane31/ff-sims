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
