import logging

from espn_api.football import League
from temporalio import activity

from activities.teams import ESPNLeagueSyncParams
from db import get_connection, resolve_league_id

logger = logging.getLogger(__name__)


@activity.defn
def fetch_and_upsert_draft(params: ESPNLeagueSyncParams) -> None:
    league = League(
        league_id=int(params.espn_league_id),
        year=params.year,
        espn_s2=params.espn_s2,
        swid=params.swid,
    )

    with get_connection() as conn:
        league_id = resolve_league_id(conn, params.espn_league_id)

        with conn.cursor() as cur:
            cur.execute("SELECT espn_id, id FROM teams WHERE league_id = %s", (league_id,))
            team_map = {row[0]: row[1] for row in cur.fetchall()}

        with conn.cursor() as cur:
            for pick in league.draft:
                try:
                    info = league.player_info(playerId=pick.playerId)
                    position = info.position if info else "Unknown"
                except Exception as exc:
                    logger.warning("player_info failed for %s: %s", pick.playerName, exc)
                    position = "Unknown"

                cur.execute("SELECT id FROM players WHERE espn_id = %s", (pick.playerId,))
                row = cur.fetchone()
                if row is None:
                    cur.execute(
                        "INSERT INTO players (espn_id, name, position, status, created_at, updated_at) "
                        "VALUES (%s, %s, %s, 'active', NOW(), NOW()) RETURNING id",
                        (pick.playerId, pick.playerName, position),
                    )
                    player_db_id = cur.fetchone()[0]
                else:
                    player_db_id = row[0]

                team_db_id = team_map.get(pick.team.team_id)
                if team_db_id is None:
                    logger.warning("No team found for ESPN team_id=%s, skipping pick", pick.team.team_id)
                    continue

                cur.execute(
                    "SELECT id FROM draft_selections "
                    "WHERE player_id = %s AND team_id = %s AND year = %s AND league_id = %s",
                    (player_db_id, team_db_id, league.year, league_id),
                )
                if cur.fetchone() is None:
                    cur.execute(
                        "INSERT INTO draft_selections "
                        "(player_id, player_name, player_position, team_id, round, pick, year, league_id, created_at, updated_at) "
                        "VALUES (%s,%s,%s,%s,%s,%s,%s,%s,NOW(),NOW())",
                        (player_db_id, pick.playerName, position,
                         team_db_id, pick.round_num, pick.round_pick, league.year, league_id),
                    )

        conn.commit()


@activity.defn
def mark_draft_fetched(espn_league_id: str) -> None:
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(
                "UPDATE espn_league_credentials SET last_draft_fetched_at = NOW() "
                "WHERE espn_league_id = %s",
                (espn_league_id,),
            )
        conn.commit()
