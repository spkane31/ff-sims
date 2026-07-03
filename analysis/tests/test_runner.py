from datetime import date, datetime

import pandas as pd

from src.config import SEASONS
from src.models import Trade, WeeklyScore
from src.runner import build_events, filter_stale, run_backtest
from src.valuation import Valuator

S2025 = SEASONS["2025"]


def _trade(ts: datetime, a: str, b: str) -> Trade:
    return Trade(
        trade_id=f"{a}-{b}", ts=ts, side_a=[a], side_b=[b],
        created_ms=int(ts.timestamp() * 1000),
    )


def _scores(week: int) -> list[WeeklyScore]:
    return [
        WeeklyScore(week=week, player_id="p1", position="QB", points=25.0),
        WeeklyScore(week=week, player_id="p2", position="RB", points=12.0),
    ]


def test_build_events_shapes_and_order():
    trades = [_trade(datetime(2025, 9, 20), "p1", "p2")]
    events = build_events(trades, _scores(1) + _scores(2), S2025)
    kinds = [e["kind"] for e in events]
    # week 1 lands Sep 8, week 2 lands Sep 15, trade Sep 20 -> sorted by ts
    assert kinds == ["week", "week", "trade"]
    assert events[0]["ts"] == datetime(2025, 9, 8)
    assert list(events[0]["scores"].columns) == ["player_id", "position", "points"]
    assert events[2]["side_a"] == ["p1"]


def test_filter_stale():
    trades = [
        _trade(datetime(2025, 9, 10), "p1", "p2"),
        _trade(datetime(2025, 9, 30), "p1", "p2"),
    ]
    events = build_events(trades, [], S2025)
    fresh, skipped = filter_stale(events, datetime(2025, 9, 15))
    assert skipped == 1
    assert len(fresh) == 1 and fresh[0]["ts"] == datetime(2025, 9, 30)
    # None watermark keeps everything
    fresh, skipped = filter_stale(events, None)
    assert (len(fresh), skipped) == (2, 0)


def test_run_backtest_snapshots_per_event_day():
    adp = pd.DataFrame(
        [
            {"player_id": "p1", "player_name": "A", "position": "QB", "adp": 1.0},
            {"player_id": "p2", "player_name": "B", "position": "RB", "adp": 10.0},
        ]
    )
    v = Valuator(
        start_ts=datetime(2025, 8, 25),
        repl_rank_by_pos={"QB": 24, "RB": 30, "WR": 36, "TE": 12},
    )
    v.seed_from_adp(adp)
    trades = [
        _trade(datetime(2025, 9, 10, 14, 0), "p1", "p2"),
        _trade(datetime(2025, 9, 10, 18, 0), "p1", "p2"),  # same day
        _trade(datetime(2025, 9, 12, 9, 0), "p1", "p2"),
    ]
    events = build_events(trades, _scores(1), S2025)

    snaps: list = []
    run_backtest(v, events, on_snapshot=lambda d, df: snaps.append((d, len(df))))

    # 3 distinct event days: Sep 8 (week 1), Sep 10 (two trades), Sep 12
    assert [d for d, _ in snaps] == [
        date(2025, 9, 8), date(2025, 9, 10), date(2025, 9, 12),
    ]
    assert all(n >= 2 for _, n in snaps)
