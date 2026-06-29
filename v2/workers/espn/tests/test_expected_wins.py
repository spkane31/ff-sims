"""Tests for the expected wins Monte Carlo simulation activity."""
import psycopg
import pytest

from activities.expected_wins import (
    ExpectedWinsParams,
    _calculate_actual_stats,
    _calculate_sos,
    _extract_team_weekly_scores,
    _run_monte_carlo,
    _weekly_expected_wins_for_week,
    calculate_and_store_expected_wins,
    get_matchup_years,
)


# ---------------------------------------------------------------------------
# Fixtures / helpers
# ---------------------------------------------------------------------------

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
            "VALUES (%s, %s, 'Team', 'Owner', 2024, NOW(), NOW()) "
            "ON CONFLICT (espn_id, league_id) DO UPDATE SET updated_at = NOW() RETURNING id",
            (espn_id, league_id),
        )
        conn.commit()
        return cur.fetchone()[0]


def _seed_matchup(
    conn: psycopg.Connection,
    league_id: int,
    week: int,
    year: int,
    home_id: int,
    away_id: int,
    home_score: float,
    away_score: float,
    completed: bool = True,
    game_type: str = "NONE",
) -> int:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO matchups "
            "(league_id, week, year, home_team_id, away_team_id, "
            "home_team_final_score, away_team_final_score, "
            "home_team_espn_projected_score, away_team_espn_projected_score, "
            "completed, is_playoff, game_type, created_at, updated_at) "
            "VALUES (%s,%s,%s,%s,%s,%s,%s,0,0,%s,false,%s,NOW(),NOW()) "
            "ON CONFLICT DO NOTHING RETURNING id",
            (league_id, week, year, home_id, away_id,
             home_score, away_score, completed, game_type),
        )
        row = cur.fetchone()
        conn.commit()
        return row[0] if row else 0


def _seed_credentials(conn: psycopg.Connection, espn_league_id: str) -> None:
    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO espn_league_credentials (espn_league_id, espn_s2, swid) "
            "VALUES (%s, 's2', 'swid') ON CONFLICT DO NOTHING",
            (espn_league_id,),
        )
    conn.commit()


# ---------------------------------------------------------------------------
# Pure-function unit tests
# ---------------------------------------------------------------------------

def _make_matchup(home_id, away_id, week, home_score, away_score, completed=True,
                  is_playoff=False, game_type="NONE"):
    return {
        "home_team_id": home_id,
        "away_team_id": away_id,
        "week": week,
        "home_team_final_score": home_score,
        "away_team_final_score": away_score,
        "completed": completed,
        "is_playoff": is_playoff,
        "game_type": game_type,
    }


def test_extract_team_weekly_scores_basic():
    matchups = [
        _make_matchup(1, 2, 1, 110.0, 95.0),
        _make_matchup(3, 4, 1, 120.0, 80.0),
        _make_matchup(1, 3, 2, 100.0, 105.0),
        _make_matchup(2, 4, 2, 90.0, 115.0),
    ]
    scores, weeks = _extract_team_weekly_scores(matchups)
    assert weeks == [1, 2]
    assert scores[1][1] == 110.0
    assert scores[2][1] == 95.0
    assert scores[1][2] == 100.0
    assert scores[3][2] == 105.0


def test_extract_team_weekly_scores_ignores_incomplete():
    matchups = [
        _make_matchup(1, 2, 1, 110.0, 95.0, completed=True),
        _make_matchup(1, 2, 2, 0.0, 0.0, completed=False),
    ]
    scores, weeks = _extract_team_weekly_scores(matchups)
    assert weeks == [1]
    assert 2 not in scores.get(1, {})


def test_extract_team_weekly_scores_ignores_playoffs():
    matchups = [
        _make_matchup(1, 2, 1, 110.0, 95.0, completed=True),
        _make_matchup(1, 2, 14, 120.0, 100.0, completed=True, is_playoff=True, game_type="WINNER_BRACKET"),
    ]
    scores, weeks = _extract_team_weekly_scores(matchups)
    assert weeks == [1]


def test_run_monte_carlo_returns_expected_structure():
    team_scores = {
        1: {1: 110.0, 2: 105.0},
        2: {1: 95.0, 2: 120.0},
        3: {1: 130.0, 2: 90.0},
        4: {1: 85.0, 2: 100.0},
    }
    result = _run_monte_carlo(team_scores, [1, 2], num_sims=1000)
    assert set(result.keys()) == {1, 2, 3, 4}
    # 4 teams → 2 matchups per week → 2 wins per week × 2 weeks × 1000 sims = 4000
    total = sum(result.values())
    assert abs(total - 4000) < 1


def test_run_monte_carlo_odd_team_count_returns_empty():
    team_scores = {1: {1: 100.0}, 2: {1: 90.0}, 3: {1: 80.0}}
    result = _run_monte_carlo(team_scores, [1], num_sims=100)
    assert result == {}


def test_run_monte_carlo_dominant_team_wins_more():
    # Team 1 always scores highest — should win most simulations
    team_scores = {
        1: {1: 200.0},
        2: {1: 100.0},
        3: {1: 90.0},
        4: {1: 80.0},
    }
    result = _run_monte_carlo(team_scores, [1], num_sims=2000)
    # Team 1 always wins; teams 2/3/4 each win only when not paired against team 1
    # With 4 teams, team 1 always wins 1 game per sim. Others share the remaining 1.
    assert result[1] == 2000
    assert result[2] + result[3] + result[4] == 2000


def test_calculate_actual_stats():
    matchups = [
        _make_matchup(1, 2, 1, 110.0, 95.0),   # team 1 wins
        _make_matchup(3, 4, 1, 80.0, 120.0),    # team 4 wins
        _make_matchup(1, 3, 2, 100.0, 105.0),   # team 3 wins
    ]
    stats = _calculate_actual_stats(matchups)
    assert stats[1] == (1, 1, 2)   # 1 win, 1 loss, 2 games
    assert stats[2] == (0, 1, 1)
    assert stats[3] == (1, 1, 2)
    assert stats[4] == (1, 0, 1)


def test_calculate_sos_basic():
    matchups = [
        _make_matchup(1, 2, 1, 110.0, 95.0),
        _make_matchup(3, 4, 1, 80.0, 120.0),
    ]
    actual_stats = _calculate_actual_stats(matchups)
    sos = _calculate_sos(matchups, actual_stats)
    # Team 1's opponent (team 2) won 0 games → SOS = 0.0
    assert sos[1] == 0.0
    # Team 2's opponent (team 1) won 1/1 = 1.0 → SOS = 1.0
    assert sos[2] == 1.0


def test_weekly_expected_wins_sums_to_one():
    # 4 teams, 1 week — total expected wins must equal 2 (N/2)
    matchups = [
        _make_matchup(1, 2, 1, 110.0, 95.0),
        _make_matchup(3, 4, 1, 120.0, 80.0),
    ]
    result = _weekly_expected_wins_for_week(matchups, target_week=1, num_sims=2000)
    total = sum(result.values())
    assert abs(total - 2.0) < 0.05  # should be 2.0 ± small rounding


def test_weekly_expected_wins_returns_empty_for_no_matchups():
    matchups = [_make_matchup(1, 2, 1, 100.0, 90.0)]
    result = _weekly_expected_wins_for_week(matchups, target_week=2, num_sims=100)
    assert result == {}


# ---------------------------------------------------------------------------
# Integration tests against real DB
# ---------------------------------------------------------------------------

def test_calculate_and_store_expected_wins_basic(db_conn):
    eid = "ew9001"
    league_id = _seed_league(db_conn, eid)
    t1 = _seed_team(db_conn, league_id, 1)
    t2 = _seed_team(db_conn, league_id, 2)
    t3 = _seed_team(db_conn, league_id, 3)
    t4 = _seed_team(db_conn, league_id, 4)

    # 2 weeks of matchups with all 4 teams
    _seed_matchup(db_conn, league_id, 1, 2024, t1, t2, 110.0, 95.0)
    _seed_matchup(db_conn, league_id, 1, 2024, t3, t4, 120.0, 80.0)
    _seed_matchup(db_conn, league_id, 2, 2024, t1, t3, 100.0, 105.0)
    _seed_matchup(db_conn, league_id, 2, 2024, t2, t4, 90.0, 115.0)

    calculate_and_store_expected_wins(ExpectedWinsParams(espn_league_id=eid, year=2024))

    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT COUNT(*) FROM weekly_expected_wins WHERE league_id = %s AND year = %s",
            (league_id, 2024),
        )
        count = cur.fetchone()[0]

    # 4 teams × 2 weeks = 8 records
    assert count == 8


def test_calculate_and_store_expected_wins_season_finalized(db_conn):
    eid = "ew9002"
    league_id = _seed_league(db_conn, eid)
    t1 = _seed_team(db_conn, league_id, 1)
    t2 = _seed_team(db_conn, league_id, 2)
    t3 = _seed_team(db_conn, league_id, 3)
    t4 = _seed_team(db_conn, league_id, 4)

    _seed_matchup(db_conn, league_id, 1, 2024, t1, t2, 110.0, 95.0)
    _seed_matchup(db_conn, league_id, 1, 2024, t3, t4, 120.0, 80.0)

    calculate_and_store_expected_wins(ExpectedWinsParams(espn_league_id=eid, year=2024))

    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT COUNT(*) FROM season_expected_wins WHERE league_id = %s AND year = %s",
            (league_id, 2024),
        )
        count = cur.fetchone()[0]

    assert count == 4  # one row per team


def test_calculate_and_store_expected_wins_is_idempotent(db_conn):
    eid = "ew9003"
    league_id = _seed_league(db_conn, eid)
    t1 = _seed_team(db_conn, league_id, 1)
    t2 = _seed_team(db_conn, league_id, 2)
    t3 = _seed_team(db_conn, league_id, 3)
    t4 = _seed_team(db_conn, league_id, 4)

    _seed_matchup(db_conn, league_id, 1, 2024, t1, t2, 110.0, 95.0)
    _seed_matchup(db_conn, league_id, 1, 2024, t3, t4, 120.0, 80.0)

    params = ExpectedWinsParams(espn_league_id=eid, year=2024)
    calculate_and_store_expected_wins(params)
    calculate_and_store_expected_wins(params)  # second run should not create duplicates

    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT COUNT(*) FROM weekly_expected_wins WHERE league_id = %s AND year = %s",
            (league_id, 2024),
        )
        assert cur.fetchone()[0] == 4  # 4 teams × 1 week, not 8


def test_calculate_and_store_expected_wins_no_matchups(db_conn):
    eid = "ew9004"
    _seed_league(db_conn, eid)
    # Should complete without error and write nothing
    calculate_and_store_expected_wins(ExpectedWinsParams(espn_league_id=eid, year=2024))


def test_calculate_and_store_expected_wins_cumulative_wins(db_conn):
    eid = "ew9005"
    league_id = _seed_league(db_conn, eid)
    t1 = _seed_team(db_conn, league_id, 1)
    t2 = _seed_team(db_conn, league_id, 2)
    t3 = _seed_team(db_conn, league_id, 3)
    t4 = _seed_team(db_conn, league_id, 4)

    # t1 wins week 1 and week 2
    _seed_matchup(db_conn, league_id, 1, 2024, t1, t2, 150.0, 50.0)
    _seed_matchup(db_conn, league_id, 1, 2024, t3, t4, 100.0, 90.0)
    _seed_matchup(db_conn, league_id, 2, 2024, t1, t3, 150.0, 50.0)
    _seed_matchup(db_conn, league_id, 2, 2024, t2, t4, 100.0, 90.0)

    calculate_and_store_expected_wins(ExpectedWinsParams(espn_league_id=eid, year=2024))

    with db_conn.cursor() as cur:
        cur.execute(
            "SELECT actual_wins, actual_losses FROM weekly_expected_wins "
            "WHERE team_id = %s AND year = 2024 AND week = 2",
            (t1,),
        )
        row = cur.fetchone()

    assert row is not None
    assert row[0] == 2  # 2 cumulative actual wins
    assert row[1] == 0  # 0 losses


def test_get_matchup_years(db_conn):
    eid = "ew9006"
    league_id = _seed_league(db_conn, eid)
    t1 = _seed_team(db_conn, league_id, 1)
    t2 = _seed_team(db_conn, league_id, 2)
    t3 = _seed_team(db_conn, league_id, 3)
    t4 = _seed_team(db_conn, league_id, 4)

    _seed_matchup(db_conn, league_id, 1, 2022, t1, t2, 100.0, 90.0)
    _seed_matchup(db_conn, league_id, 1, 2023, t1, t2, 100.0, 90.0)
    _seed_matchup(db_conn, league_id, 1, 2024, t1, t2, 100.0, 90.0)

    years = get_matchup_years(eid)
    assert years == [2022, 2023, 2024]
