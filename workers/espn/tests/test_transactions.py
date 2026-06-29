from unittest.mock import MagicMock, patch

import psycopg
import pytest

from activities.teams import ESPNLeagueSyncParams
from activities.transactions import fetch_and_upsert_transactions, mark_transactions_fetched


def _seed_league(conn: psycopg.Connection, external_id: str) -> int:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO leagues (name, platform, external_id) VALUES ('Test', 'ESPN', %s) "
            "ON CONFLICT (platform, external_id) WHERE platform != '' AND external_id != '' "
            "DO UPDATE SET name = EXCLUDED.name RETURNING id",
            (external_id,),
        )
        conn.commit()
        return cur.fetchone()[0]


def _seed_team(conn: psycopg.Connection, league_id: int, espn_id: int) -> int:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO teams (espn_id, league_id, name, owner, year, created_at, updated_at) "
            "VALUES (%s, %s, 'Team', 'Owner', 2025, NOW(), NOW()) "
            "ON CONFLICT (espn_id, league_id) DO UPDATE SET updated_at = NOW() RETURNING id",
            (espn_id, league_id),
        )
        conn.commit()
        return cur.fetchone()[0]


def _seed_credentials(conn: psycopg.Connection, espn_league_id: str) -> None:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO espn_league_credentials (espn_league_id, espn_s2, swid) "
            "VALUES (%s, 's2', 'swid') ON CONFLICT DO NOTHING",
            (espn_league_id,),
        )
    conn.commit()


def test_fetch_and_upsert_transactions_creates_record(db_conn):
    league_id = _seed_league(db_conn, "6001")
    _seed_team(db_conn, league_id, 1)
    _seed_credentials(db_conn, "6001")

    player = MagicMock()
    player.playerId = 99001
    player.name = "Justin Jefferson"
    player.position = "WR"
    team = MagicMock(team_id=1)
    tx = MagicMock(date=1700000000000, actions=[(team, "ADDED", player, 0)])

    mock_league = MagicMock(year=2025)
    mock_league.recent_activity.side_effect = [[tx], []]

    params = ESPNLeagueSyncParams(espn_league_id="6001", year=2025, espn_s2="s2", swid="swid")

    with patch("activities.transactions.League", return_value=mock_league):
        fetch_and_upsert_transactions(params)

    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT transaction_type, player_name FROM transactions WHERE league_id = %s",
            (league_id,),
        )
        rows = cur.fetchall()
    assert len(rows) == 1
    assert rows[0] == ("ADDED", "Justin Jefferson")


def test_fetch_and_upsert_transactions_skips_pre2024(db_conn):
    _seed_credentials(db_conn, "6002")
    mock_league = MagicMock(year=2023)
    params = ESPNLeagueSyncParams(espn_league_id="6002", year=2023, espn_s2="s2", swid="swid")

    with patch("activities.transactions.League", return_value=mock_league):
        fetch_and_upsert_transactions(params)

    mock_league.recent_activity.assert_not_called()


def test_mark_transactions_fetched_sets_timestamp(db_conn):
    _seed_credentials(db_conn, "6003")
    mark_transactions_fetched("6003")
    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT last_transactions_fetched_at FROM espn_league_credentials WHERE espn_league_id = '6003'"
        )
        assert cur.fetchone()[0] is not None
