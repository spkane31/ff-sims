from unittest.mock import MagicMock, patch

import psycopg
import pytest

from activities.schedule import fetch_and_upsert_schedule, mark_schedule_fetched
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
            "VALUES (%s, %s, 'Team', 'Owner', 2026, NOW(), NOW()) "
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


def _mock_box_score(home_espn_id: int, away_espn_id: int, home_score: float = 110.0, away_score: float = 95.0) -> MagicMock:
    bs = MagicMock()
    bs.home_team = MagicMock(team_id=home_espn_id)
    bs.away_team = MagicMock(team_id=away_espn_id)
    bs.home_score = home_score
    bs.away_score = away_score
    bs.home_projected = 108.0
    bs.away_projected = 97.0
    bs.matchup_type = "REGULAR"
    bs.is_playoff = False
    bs.home_lineup = []
    bs.away_lineup = []
    return bs


def test_fetch_and_upsert_schedule_creates_matchup(db_conn):
    league_id = _seed_league(db_conn, "8001")
    _seed_team(db_conn, league_id, 1)
    _seed_team(db_conn, league_id, 2)
    _seed_credentials(db_conn, "8001")

    mock_league = MagicMock()
    mock_league.year = 2026  # matches current year so break condition fires at week > current_week
    mock_league.current_week = 1
    mock_league.box_scores.return_value = [_mock_box_score(1, 2)]

    params = ESPNLeagueSyncParams(espn_league_id="8001", year=2026, espn_s2="s2", swid="swid")

    with patch("activities.schedule.League", return_value=mock_league):
        fetch_and_upsert_schedule(params)

    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT week, year, completed FROM matchups WHERE league_id = %s",
            (league_id,),
        )
        rows = cur.fetchall()
    assert len(rows) == 1
    assert rows[0][:2] == (1, 2026)


def test_fetch_and_upsert_schedule_is_idempotent(db_conn):
    league_id = _seed_league(db_conn, "8002")
    _seed_team(db_conn, league_id, 1)
    _seed_team(db_conn, league_id, 2)
    _seed_credentials(db_conn, "8002")

    mock_league = MagicMock()
    mock_league.year = 2026
    mock_league.current_week = 1
    mock_league.box_scores.return_value = [_mock_box_score(1, 2)]

    params = ESPNLeagueSyncParams(espn_league_id="8002", year=2026, espn_s2="s2", swid="swid")

    with patch("activities.schedule.League", return_value=mock_league):
        fetch_and_upsert_schedule(params)
        fetch_and_upsert_schedule(params)

    with db_conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) FROM matchups WHERE league_id = %s", (league_id,))
        assert cur.fetchone()[0] == 1


def test_mark_schedule_fetched_sets_timestamp(db_conn):
    _seed_credentials(db_conn, "8003")
    mark_schedule_fetched("8003")
    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT last_schedule_fetched_at FROM espn_league_credentials WHERE espn_league_id = '8003'"
        )
        assert cur.fetchone()[0] is not None
