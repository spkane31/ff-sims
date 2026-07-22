import logging
from datetime import datetime

from espn_api.football import League
from temporalio import activity

from activities.teams import ESPNLeagueSyncParams
from db import get_connection, resolve_league_id

logger = logging.getLogger(__name__)


@activity.defn
def fetch_and_upsert_transactions(params: ESPNLeagueSyncParams) -> None:
    if params.year < 2024:
        logger.info("Transactions not available before 2024 — skipping year %d", params.year)
        return

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

        offset = 0
        with conn.cursor() as cur:
            while True:
                try:
                    txns = league.recent_activity(offset=offset)
                    if not txns:
                        break
                    for tx in txns:
                        tx_date = datetime.fromtimestamp(tx.date / 1000)
                        for team, tx_type, player, bid_amount in tx.actions:
                            team_db_id = team_map.get(team.team_id)
                            if team_db_id is None:
                                continue

                            # Atomic upsert (not SELECT-then-INSERT) — see the
                            # comment on activities/schedule.py's
                            # _upsert_player for why: concurrent writers here
                            # deadlocked on idx_players_espn_id.
                            cur.execute(
                                "INSERT INTO players (espn_id, name, position, status, created_at, updated_at) "
                                "VALUES (%s, %s, %s, 'active', NOW(), NOW()) "
                                "ON CONFLICT (espn_id) DO UPDATE SET espn_id = players.espn_id "
                                "RETURNING id",
                                (player.playerId, player.name, player.position),
                            )
                            player_db_id = cur.fetchone()[0]

                            cur.execute(
                                "SELECT id FROM transactions "
                                "WHERE team_id = %s AND player_id = %s AND date = %s "
                                "AND transaction_type = %s AND league_id = %s",
                                (team_db_id, player_db_id, tx_date, tx_type, league_id),
                            )
                            if cur.fetchone() is None:
                                cur.execute(
                                    "INSERT INTO transactions "
                                    "(team_id, player_id, transaction_type, player_name, "
                                    "bid_amount, date, year, league_id, created_at, updated_at) "
                                    "VALUES (%s,%s,%s,%s,%s,%s,%s,%s,NOW(),NOW())",
                                    (team_db_id, player_db_id, tx_type, player.name,
                                     int(bid_amount), tx_date, league.year, league_id),
                                )
                    offset += 25
                except Exception as exc:
                    logger.error("Transaction fetch error at offset %d: %s", offset, exc)
                    break
        conn.commit()


@activity.defn
def mark_transactions_fetched(espn_league_id: str) -> None:
    with get_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(
                "UPDATE espn_league_credentials SET last_transactions_fetched_at = NOW() "
                "WHERE espn_league_id = %s",
                (espn_league_id,),
            )
        conn.commit()
