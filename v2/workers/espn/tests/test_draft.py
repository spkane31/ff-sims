from unittest.mock import MagicMock, patch

import psycopg
import pytest

from activities.draft import fetch_and_upsert_draft, mark_draft_fetched
from activities.teams import ESPNLeagueSyncParams


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


def test_fetch_and_upsert_draft_creates_selection(db_conn):
    league_id = _seed_league(db_conn, "7001")
    _seed_team(db_conn, league_id, 1)
    _seed_credentials(db_conn, "7001")

    pick = MagicMock()
    pick.playerId = 12345
    pick.playerName = "Patrick Mahomes"
    pick.round_num = 1
    pick.round_pick = 1
    pick.team = MagicMock(team_id=1)

    mock_league = MagicMock()
    mock_league.draft = [pick]
    mock_league.year = 2025
    mock_league.player_info.return_value = MagicMock(position="QB")

    params = ESPNLeagueSyncParams(espn_league_id="7001", year=2025, espn_s2="s2", swid="swid")

    with patch("activities.draft.League", return_value=mock_league):
        fetch_and_upsert_draft(params)

    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT player_name, round, pick FROM draft_selections WHERE league_id = %s",
            (league_id,),
        )
        rows = cur.fetchall()
    assert len(rows) == 1
    assert rows[0] == ("Patrick Mahomes", 1, 1)


def test_fetch_and_upsert_draft_is_idempotent(db_conn):
    league_id = _seed_league(db_conn, "7002")
    _seed_team(db_conn, league_id, 1)
    _seed_credentials(db_conn, "7002")

    pick = MagicMock()
    pick.playerId = 12346
    pick.playerName = "Josh Allen"
    pick.round_num = 1
    pick.round_pick = 2
    pick.team = MagicMock(team_id=1)

    mock_league = MagicMock(draft=[pick], year=2025)
    mock_league.player_info.return_value = MagicMock(position="QB")

    params = ESPNLeagueSyncParams(espn_league_id="7002", year=2025, espn_s2="s2", swid="swid")

    with patch("activities.draft.League", return_value=mock_league):
        fetch_and_upsert_draft(params)
        fetch_and_upsert_draft(params)

    with db_conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) FROM draft_selections WHERE league_id = %s", (league_id,))
        assert cur.fetchone()[0] == 1


def test_mark_draft_fetched_sets_timestamp(db_conn):
    _seed_credentials(db_conn, "7003")
    mark_draft_fetched("7003")
    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT last_draft_fetched_at FROM espn_league_credentials WHERE espn_league_id = '7003'"
        )
        assert cur.fetchone()[0] is not None
