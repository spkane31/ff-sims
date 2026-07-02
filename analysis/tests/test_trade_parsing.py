from datetime import datetime

from src.parsing import ms_to_dt, parse_trade


def test_two_sided_trade_parses():
    # adds: player_id -> receiving roster_id
    t = parse_trade("t1", 1728000000000, {"pA": 3, "pB": 3, "pC": 7}, [], [])
    assert t is not None
    assert t.trade_id == "t1"
    # sides ordered by roster_id for determinism
    assert sorted(t.side_a) == ["pA", "pB"]
    assert t.side_b == ["pC"]
    assert t.ts == datetime(2024, 10, 4, 0, 0)  # 1728000000000 ms UTC
    assert t.created_ms == 1728000000000


def test_three_team_trade_skipped():
    assert parse_trade("t2", 0, {"pA": 1, "pB": 2, "pC": 3}, [], []) is None


def test_one_sided_skipped():
    assert parse_trade("t3", 0, {"pA": 1}, [], []) is None


def test_draft_picks_skipped():
    picks = [{"round": 1, "season": "2026"}]
    assert parse_trade("t4", 0, {"pA": 1, "pB": 2}, picks, []) is None


def test_faab_skipped():
    faab = [{"sender": 1, "receiver": 2, "amount": 20}]
    assert parse_trade("t5", 0, {"pA": 1, "pB": 2}, [], faab) is None


def test_null_adds_skipped():
    # jsonb null comes back as Python None
    assert parse_trade("t6", 0, None, [], []) is None
    assert parse_trade("t7", 0, {}, [], []) is None


def test_ms_to_dt_is_naive_utc():
    dt = ms_to_dt(1728000000000)
    assert dt == datetime(2024, 10, 4, 0, 0)
    assert dt.tzinfo is None
