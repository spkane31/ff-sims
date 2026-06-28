from dataclasses import dataclass

from espn_api.football import League
from temporalio import activity

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
    )
    with get_connection() as conn:
        league_id = resolve_league_id(conn, params.espn_league_id)
        with conn.cursor() as cur:
            for team in league.teams:
                owner = " ".join([
                    team.owners[0]["firstName"],
                    team.owners[0]["lastName"],
                ])
                cur.execute(
                    """
                    INSERT INTO teams (espn_id, league_id, name, owner, created_at, updated_at)
                    VALUES (%s, %s, %s, %s, NOW(), NOW())
                    ON CONFLICT (espn_id, league_id)
                    DO UPDATE SET name = EXCLUDED.name, owner = EXCLUDED.owner, updated_at = NOW()
                    """,
                    (team.team_id, league_id, team.team_name, owner),
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
