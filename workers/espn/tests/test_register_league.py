from unittest.mock import MagicMock, patch, AsyncMock

import pytest
import sys

import register_league

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

    try:
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
    finally:
        _clear_league(db_conn, "5001")


def test_upsert_updates_existing_league_and_credentials(db_conn):
    _clear_league(db_conn, "5002")

    try:
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
    finally:
        _clear_league(db_conn, "5002")


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


async def test_start_sync_workflow_calls_temporal_with_expected_id():
    mock_client = MagicMock()
    mock_client.start_workflow = AsyncMock(return_value=None)

    async def fake_create_client():
        return mock_client

    with patch("register_league.create_client", fake_create_client):
        workflow_id = await register_league.start_sync_workflow("345674", 2025)

    assert workflow_id == "espn-league-345674-2025"

    # Verify client.start_workflow was called with correct arguments
    mock_client.start_workflow.assert_called_once()
    args, kwargs = mock_client.start_workflow.call_args

    # Check positional args
    assert args[0] == register_league.LeagueESPNSyncWorkflow.run
    assert args[1].espn_league_id == "345674"
    assert args[1].year == 2025

    # Check keyword args
    assert kwargs["id"] == "espn-league-345674-2025"
    assert kwargs["task_queue"] == "espn-sync"


def test_main_no_sync_registers_league_without_starting_workflow(db_conn, monkeypatch, capsys):
    _clear_league(db_conn, "5004")

    try:
        monkeypatch.setattr(
            sys, "argv",
            ["register-league", "--league-id", "5004", "--espn-s2", "s2v", "--swid", "swidv", "--no-sync"],
        )

        with patch("register_league.League", return_value=_mock_league("No Sync League")):
            register_league.main()

        captured = capsys.readouterr()
        assert "Registered new league" in captured.out
        assert "Started sync workflow" not in captured.out

        with db_conn.cursor() as cur:
            cur.execute("SELECT name FROM leagues WHERE external_id = %s", ("5004",))
            assert cur.fetchone()[0] == "No Sync League"
    finally:
        _clear_league(db_conn, "5004")


def test_main_with_sync_starts_workflow(db_conn, monkeypatch, capsys):
    _clear_league(db_conn, "5005")

    try:
        monkeypatch.setattr(
            sys, "argv",
            ["register-league", "--league-id", "5005", "--espn-s2", "s2v", "--swid", "swidv"],
        )

        async def fake_start_sync_workflow(league_id, year):
            return f"espn-league-{league_id}-{year}"

        with patch("register_league.League", return_value=_mock_league("Synced League")), \
             patch("register_league.start_sync_workflow", fake_start_sync_workflow):
            register_league.main()

        captured = capsys.readouterr()
        assert "Registered new league" in captured.out
        assert "Started sync workflow: espn-league-5005-" in captured.out
    finally:
        _clear_league(db_conn, "5005")


def test_main_warns_but_does_not_crash_when_workflow_start_fails(db_conn, monkeypatch, capsys):
    _clear_league(db_conn, "5006")

    try:
        monkeypatch.setattr(
            sys, "argv",
            ["register-league", "--league-id", "5006", "--espn-s2", "s2v", "--swid", "swidv"],
        )

        async def failing_start_sync_workflow(league_id, year):
            raise RuntimeError("workflow already started")

        with patch("register_league.League", return_value=_mock_league("Warn League")), \
             patch("register_league.start_sync_workflow", failing_start_sync_workflow):
            register_league.main()  # must not raise

        captured = capsys.readouterr()
        assert "Registered new league" in captured.out
        assert "Warning: could not start sync workflow" in captured.err

        # Verify the league was persisted to DB despite workflow start failure
        with db_conn.cursor() as cur:
            cur.execute("SELECT name FROM leagues WHERE external_id = %s", ("5006",))
            assert cur.fetchone()[0] == "Warn League"
    finally:
        _clear_league(db_conn, "5006")
