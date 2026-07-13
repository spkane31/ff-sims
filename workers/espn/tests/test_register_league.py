from unittest.mock import MagicMock, patch

import pytest

from register_league import upsert_league_and_credentials, validate_and_fetch_name


def _mock_league(name: str) -> MagicMock:
    league = MagicMock()
    league.settings.name = name
    return league


def _clear_league(conn, external_id: str) -> None:
    with conn.cursor() as cur:
        cur.execute("DELETE FROM espn_league_credentials WHERE espn_league_id = %s", (external_id,))
        cur.execute("DELETE FROM leagues WHERE external_id = %s", (external_id,))
    conn.commit()


def test_validate_and_fetch_name_returns_league_name():
    with patch("register_league.League", return_value=_mock_league("My League")):
        name = validate_and_fetch_name("5001", 2025, "s2", "swid")
    assert name == "My League"


def test_validate_and_fetch_name_raises_on_bad_credentials():
    with patch("register_league.League", side_effect=RuntimeError("401 Unauthorized")):
        with pytest.raises(RuntimeError):
            validate_and_fetch_name("5001", 2025, "bad-s2", "bad-swid")


def test_upsert_creates_new_league_and_credentials(db_conn):
    _clear_league(db_conn, "5001")

    internal_id, was_inserted = upsert_league_and_credentials(
        db_conn, "New League", "5001", "s2-value", "swid-value"
    )
    assert was_inserted is True

    with db_conn.cursor() as cur:
        cur.execute("SELECT name, platform, external_id FROM leagues WHERE id = %s", (internal_id,))
        assert cur.fetchone() == ("New League", "ESPN", "5001")

        cur.execute(
            "SELECT espn_s2, swid FROM espn_league_credentials WHERE espn_league_id = %s", ("5001",)
        )
        assert cur.fetchone() == ("s2-value", "swid-value")


def test_upsert_updates_existing_league_and_credentials(db_conn):
    _clear_league(db_conn, "5002")

    first_id, first_inserted = upsert_league_and_credentials(
        db_conn, "Original Name", "5002", "old-s2", "old-swid"
    )
    assert first_inserted is True

    second_id, second_inserted = upsert_league_and_credentials(
        db_conn, "Renamed League", "5002", "new-s2", "new-swid"
    )
    assert second_inserted is False
    assert second_id == first_id

    with db_conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) FROM leagues WHERE external_id = %s", ("5002",))
        assert cur.fetchone()[0] == 1

        cur.execute("SELECT name FROM leagues WHERE id = %s", (first_id,))
        assert cur.fetchone()[0] == "Renamed League"

        cur.execute(
            "SELECT espn_s2, swid FROM espn_league_credentials WHERE espn_league_id = %s", ("5002",)
        )
        assert cur.fetchone() == ("new-s2", "new-swid")


def test_validation_failure_writes_nothing(db_conn):
    _clear_league(db_conn, "5003")

    with patch("register_league.League", side_effect=RuntimeError("401 Unauthorized")):
        with pytest.raises(RuntimeError):
            validate_and_fetch_name("5003", 2025, "bad-s2", "bad-swid")

    with db_conn.cursor() as cur:
        cur.execute("SELECT COUNT(*) FROM leagues WHERE external_id = %s", ("5003",))
        assert cur.fetchone()[0] == 0
        cur.execute(
            "SELECT COUNT(*) FROM espn_league_credentials WHERE espn_league_id = %s", ("5003",)
        )
        assert cur.fetchone()[0] == 0
