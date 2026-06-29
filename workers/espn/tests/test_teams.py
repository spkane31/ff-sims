from unittest.mock import MagicMock, patch

import psycopg
import pytest

from activities.teams import ESPNLeagueSyncParams, fetch_and_upsert_teams, mark_teams_fetched


def _seed_league(conn: psycopg.Connection, external_id: str) -> int:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO leagues (name, platform, external_id) VALUES ('Test League', 'ESPN', %s) "
            "ON CONFLICT (platform, external_id) WHERE platform != '' AND external_id != '' "
            "DO UPDATE SET name = EXCLUDED.name RETURNING id",
            (external_id,),
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


def _mock_team(team_id: int, first: str, last: str, name: str) -> MagicMock:
    t = MagicMock()
    t.team_id = team_id
    t.owners = [{"firstName": first, "lastName": last}]
    t.team_name = name
    return t


def test_fetch_and_upsert_teams_creates_teams(db_conn):
    league_id = _seed_league(db_conn, "9001")
    _seed_credentials(db_conn, "9001")

    mock_league = MagicMock()
    mock_league.teams = [
        _mock_team(1, "Alice", "Smith", "Team A"),
        _mock_team(2, "Bob", "Jones", "Team B"),
    ]
    params = ESPNLeagueSyncParams(espn_league_id="9001", year=2025, espn_s2="s2", swid="swid")

    with patch("activities.teams.League", return_value=mock_league):
        fetch_and_upsert_teams(params)

    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT espn_id, name, owner FROM teams WHERE league_id = %s ORDER BY espn_id",
            (league_id,),
        )
        rows = cur.fetchall()

    assert len(rows) == 2
    assert rows[0] == (1, "Team A", "Alice Smith")
    assert rows[1] == (2, "Team B", "Bob Jones")


def test_fetch_and_upsert_teams_is_idempotent(db_conn):
    league_id = _seed_league(db_conn, "9002")
    _seed_credentials(db_conn, "9002")

    mock_league = MagicMock()
    mock_league.teams = [_mock_team(1, "Alice", "Smith", "Team A")]
    params = ESPNLeagueSyncParams(espn_league_id="9002", year=2025, espn_s2="s2", swid="swid")

    with patch("activities.teams.League", return_value=mock_league):
        fetch_and_upsert_teams(params)
        fetch_and_upsert_teams(params)

    with db_conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) FROM teams WHERE league_id = %s", (league_id,))
        assert cur.fetchone()[0] == 1


def test_mark_teams_fetched_sets_timestamp(db_conn):
    _seed_credentials(db_conn, "9003")
    mark_teams_fetched("9003")
    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT last_teams_fetched_at FROM espn_league_credentials WHERE espn_league_id = '9003'"
        )
        assert cur.fetchone()[0] is not None
