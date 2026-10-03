from datetime import datetime
from unittest.mock import MagicMock, call, patch

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
    mock_league.year = 2026
    mock_league.current_week = 1
    mock_league.settings.reg_season_count = 14
    mock_league.scoreboard.return_value = []
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
    mock_league.settings.reg_season_count = 14
    mock_league.scoreboard.return_value = []
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


@pytest.mark.parametrize("regular_season_weeks", [12, 14, 16])
def test_schedule_sync_imports_real_future_opponents(regular_season_weeks):
    mock_league = MagicMock()
    mock_league.year = 2026
    mock_league.current_week = 4
    mock_league.settings.reg_season_count = regular_season_weeks
    mock_league.box_scores.return_value = [_mock_box_score(1, 2)]

    def future_scoreboard(week):
        # Real opponents change by week; scoreboard entries have no lineups.
        bs = _mock_box_score(1, 3 if week % 2 else 4, 0, 0)
        bs.matchup_type = "NONE"
        del bs.home_lineup
        del bs.away_lineup
        return [bs]

    mock_league.scoreboard.side_effect = future_scoreboard
    conn = MagicMock()
    cur = conn.cursor.return_value.__enter__.return_value
    cur.fetchall.return_value = [(1, 101), (2, 102), (3, 103), (4, 104)]
    cur.fetchone.side_effect = lambda: (
        (123,) if cur.execute.call_args.args[0].startswith("INSERT") else None
    )
    params = ESPNLeagueSyncParams(
        espn_league_id="8004", year=2026, espn_s2="s2", swid="swid"
    )

    with (
        patch("activities.schedule.League", return_value=mock_league),
        patch("activities.schedule.get_connection") as get_connection,
        patch("activities.schedule.resolve_league_id", return_value=1),
        patch("activities.schedule.activity.heartbeat"),
        patch("activities.schedule.datetime") as clock,
    ):
        get_connection.return_value.__enter__.return_value = conn
        clock.now.return_value = datetime(2026, 10, 1)
        fetch_and_upsert_schedule(params)

    assert mock_league.box_scores.call_args_list == [call(week=w) for w in range(1, 5)]
    assert mock_league.scoreboard.call_args_list == [
        call(week=w) for w in range(5, regular_season_weeks + 1)
    ]
    inserted = [
        c.args[1]
        for c in cur.execute.call_args_list
        if c.args[0].startswith("INSERT INTO matchups")
    ]
    future = [row for row in inserted if row[1] > 4]
    assert len(future) == regular_season_weeks - 4
    assert [(row[1], row[4]) for row in future] == [
        (week, 103 if week % 2 else 104) for week in range(5, regular_season_weeks + 1)
    ]
    assert all(
        row[5:7] == (0, 0) and row[9] is False and row[11] == "NONE"
        for row in future
    )


def test_in_progress_week_is_not_completed():
    # ESPN publishes live scores all week and leaves the winner UNDECIDED until
    # the scoring period is final, so mid-week scores must not count as final.
    mock_league = MagicMock()
    mock_league.year = 2026
    mock_league.current_week = 4
    mock_league.settings.reg_season_count = 14
    mock_league.teams = [
        MagicMock(team_id=1, outcomes=["W", "L", "W", "U"]),
        MagicMock(team_id=2, outcomes=["L", "W", "L", "U"]),
    ]
    mock_league.box_scores.side_effect = lambda week: [
        _mock_box_score(1, 2, 11.94, 21.0)
        if week == 4
        else _mock_box_score(1, 2, 110.0, 95.0)
    ]
    mock_league.scoreboard.side_effect = lambda week: []
    conn = MagicMock()
    cur = conn.cursor.return_value.__enter__.return_value
    cur.fetchall.return_value = [(1, 101), (2, 102)]
    cur.fetchone.side_effect = lambda: (
        (123,) if cur.execute.call_args.args[0].startswith("INSERT") else None
    )
    params = ESPNLeagueSyncParams(
        espn_league_id="8005", year=2026, espn_s2="s2", swid="swid"
    )

    with (
        patch("activities.schedule.League", return_value=mock_league),
        patch("activities.schedule.get_connection") as get_connection,
        patch("activities.schedule.resolve_league_id", return_value=1),
        patch("activities.schedule.activity.heartbeat"),
        patch("activities.schedule.datetime") as clock,
    ):
        get_connection.return_value.__enter__.return_value = conn
        clock.now.return_value = datetime(2026, 10, 3)
        fetch_and_upsert_schedule(params)

    completed_by_week = {
        c.args[1][1]: c.args[1][9]
        for c in cur.execute.call_args_list
        if c.args[0].startswith("INSERT INTO matchups")
    }
    assert completed_by_week[4] is False
    assert [completed_by_week[week] for week in (1, 2, 3)] == [True, True, True]

