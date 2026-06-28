from unittest.mock import MagicMock, patch

import psycopg
import pytest

from activities.player_status import PlayerStatusParams, mark_players_updated, update_active_players


def _seed_credentials(conn: psycopg.Connection, espn_league_id: str) -> None:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO espn_league_credentials (espn_league_id, espn_s2, swid) "
            "VALUES (%s, 's2', 'swid') ON CONFLICT DO NOTHING",
            (espn_league_id,),
        )
    conn.commit()


def _seed_player(conn: psycopg.Connection, espn_id: int, position: str = "WR", status: str = "active") -> int:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO players (espn_id, name, position, status, created_at, updated_at) "
            "VALUES (%s, 'Test Player', %s, %s, NOW(), NOW()) "
            "ON CONFLICT (espn_id) DO UPDATE SET position = EXCLUDED.position, status = EXCLUDED.status "
            "RETURNING id",
            (espn_id, position, status),
        )
        conn.commit()
        return cur.fetchone()[0]


def test_update_active_players_marks_missing_as_inactive(db_conn):
    _seed_player(db_conn, 55001)
    mock_league = MagicMock()
    mock_league.player_info.return_value = None
    params = PlayerStatusParams(espn_league_id="5500", espn_s2="s2", swid="swid", year=2025)

    with patch("activities.player_status.League", return_value=mock_league):
        update_active_players(params)

    with db_conn.cursor() as cur:
        cur.execute("SELECT status FROM players WHERE espn_id = 55001")
        assert cur.fetchone()[0] == "inactive"


def test_update_active_players_updates_changed_position(db_conn):
    _seed_player(db_conn, 55002, position="RB")
    updated = MagicMock()
    updated.position = "WR"
    mock_league = MagicMock()
    mock_league.player_info.return_value = updated
    params = PlayerStatusParams(espn_league_id="5501", espn_s2="s2", swid="swid", year=2025)

    with patch("activities.player_status.League", return_value=mock_league):
        update_active_players(params)

    with db_conn.cursor() as cur:
        cur.execute("SELECT position FROM players WHERE espn_id = 55002")
        assert cur.fetchone()[0] == "WR"


def test_mark_players_updated_stamps_all_credential_rows(db_conn):
    _seed_credentials(db_conn, "5502")
    _seed_credentials(db_conn, "5503")
    mark_players_updated()
    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT COUNT(*) FROM espn_league_credentials "
            "WHERE espn_league_id IN ('5502','5503') AND last_players_updated_at IS NOT NULL"
        )
        assert cur.fetchone()[0] == 2
