import logging
from dataclasses import dataclass

from espn_api.football import League
from temporalio import activity

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
    league = League(
        league_id=int(params.espn_league_id),
        year=params.year,
        espn_s2=params.espn_s2,
        swid=params.swid,
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
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute("UPDATE espn_league_credentials SET last_players_updated_at = NOW()")
        conn.commit()
