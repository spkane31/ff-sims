from datetime import date, datetime, timedelta

import pandas as pd
import pytest

from src.config import SEASONS
from src.models import Trade
from src.runner import (
    build_events,
    replay,
    take_events_before,
    utc_batches,
    validate_step,
)
from src.valuation import Valuator

S2025 = SEASONS["2025"]
DAY = timedelta(days=1)
REPL = {"QB": 24, "RB": 30, "WR": 36, "TE": 12}


def _trade(ts: datetime, a: str = "p1", b: str = "p2") -> Trade:
    return Trade(
        trade_id=f"{a}-{b}-{ts.isoformat()}", ts=ts, side_a=[a], side_b=[b],
        created_ms=0,
    )


def _adp() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {"player_id": "p1", "player_name": "A", "position": "QB", "adp": 1.0},
            {"player_id": "p2", "player_name": "B", "position": "RB", "adp": 10.0},
        ]
    )


def _valuator(start: date) -> Valuator:
    v = Valuator(
        start_ts=datetime.combine(start, datetime.min.time()), repl_rank_by_pos=REPL
    )
    v.seed_from_adp(_adp())
    return v


# ------------------------------------------------------------- boundaries --


def test_utc_batches_are_contiguous_and_end_exactly_at_end():
    batches = list(utc_batches(date(2025, 9, 1), date(2025, 9, 4), DAY))
    assert batches == [
        (datetime(2025, 9, 1), datetime(2025, 9, 2)),
        (datetime(2025, 9, 2), datetime(2025, 9, 3)),
        (datetime(2025, 9, 3), datetime(2025, 9, 4)),
    ]


def test_validate_step_rejects_partial_days_and_non_dividing_steps():
    with pytest.raises(ValueError, match="whole number of days"):
        validate_step(date(2025, 9, 1), date(2025, 9, 4), timedelta(hours=12))
    with pytest.raises(ValueError, match="does not evenly divide"):
        validate_step(date(2025, 9, 1), date(2025, 9, 4), timedelta(days=2))
    with pytest.raises(ValueError, match="must be after start"):
        validate_step(date(2025, 9, 4), date(2025, 9, 4), DAY)
    # the scheduled configuration is valid
    validate_step(date(2025, 8, 25), date(2026, 7, 28), DAY)


def test_take_events_before_is_exclusive_and_advances_the_cursor():
    events = [{"ts": datetime(2025, 9, 1, 12)}, {"ts": datetime(2025, 9, 2)}]
    batch, cursor = take_events_before(events, 0, datetime(2025, 9, 2))
    assert [e["ts"] for e in batch] == [datetime(2025, 9, 1, 12)]
    assert cursor == 1
    batch, cursor = take_events_before(events, cursor, datetime(2025, 9, 3))
    assert [e["ts"] for e in batch] == [datetime(2025, 9, 2)]
    assert cursor == 2


# ----------------------------------------------------------------- replay --


def test_replay_emits_consecutive_dates_including_quiet_days():
    start, end = date(2025, 9, 1), date(2025, 9, 6)
    v = _valuator(start)
    events = build_events([_trade(datetime(2025, 9, 3, 14))], [], S2025)

    snaps: list[tuple[date, int]] = []
    stats = replay(v, events, start, end, DAY, lambda d, df: snaps.append((d, len(df))))

    # one snapshot per step boundary: start+1d .. end, no gaps on quiet days
    assert [d for d, _ in snaps] == [
        date(2025, 9, 2), date(2025, 9, 3), date(2025, 9, 4),
        date(2025, 9, 5), date(2025, 9, 6),
    ]
    assert stats.snapshots == 5
    assert all(n == 2 for _, n in snaps)


def test_replay_snapshot_is_state_at_start_of_that_day():
    """A trade at 14:00 on Sep 3 is in the Sep 4 snapshot, not the Sep 3 one."""
    start, end = date(2025, 9, 1), date(2025, 9, 6)
    v = _valuator(start)
    events = build_events([_trade(datetime(2025, 9, 3, 14))], [], S2025)

    values: dict[date, float] = {}
    replay(
        v, events, start, end, DAY,
        lambda d, df: values.__setitem__(
            d, float(df.set_index("player_id").loc["p1", "value"])
        ),
    )
    assert values[date(2025, 9, 2)] == values[date(2025, 9, 3)]
    assert values[date(2025, 9, 4)] != values[date(2025, 9, 3)]
    assert values[date(2025, 9, 5)] == values[date(2025, 9, 4)]


def test_replay_quiet_days_still_widen_uncertainty():
    start, end = date(2025, 9, 1), date(2025, 9, 5)
    v = _valuator(start)
    sds: list[float] = []
    replay(
        v, [], start, end, DAY,
        lambda d, df: sds.append(float(df.set_index("player_id").loc["p1", "sd"])),
    )
    assert sds == sorted(sds) and sds[0] < sds[-1]


def test_replay_consumes_every_in_window_event_exactly_once():
    start, end = date(2025, 9, 1), date(2025, 9, 5)
    v = _valuator(start)
    seen: list[str] = []
    original_advance = v.advance

    def spy(events):
        seen.extend(e.get("trade_id", e["ts"].isoformat()) for e in events)
        original_advance(events)

    v.advance = spy  # type: ignore[method-assign]
    trades = [
        _trade(datetime(2025, 9, 1, 9)),
        _trade(datetime(2025, 9, 1, 21)),  # same day
        _trade(datetime(2025, 9, 3, 3)),
    ]
    events = build_events(trades, [], S2025)
    stats = replay(v, events, start, end, DAY, lambda d, df: None)

    assert stats.events_applied == 3
    assert len(seen) == 3 and len(set(seen)) == 3


def test_replay_drops_events_outside_the_window_and_reports_them():
    start, end = date(2025, 9, 2), date(2025, 9, 4)
    v = _valuator(start)
    trades = [
        _trade(datetime(2025, 9, 1, 12)),  # before start
        _trade(datetime(2025, 9, 2, 12)),  # in window
        _trade(datetime(2025, 9, 4, 0)),   # exactly at the exclusive end
    ]
    events = build_events(trades, [], S2025)
    stats = replay(v, events, start, end, DAY, lambda d, df: None)

    assert stats.events_applied == 1
    assert stats.dropped_before_start == 1
    assert stats.dropped_at_or_after_end == 1
    # the model clock stops at the exclusive end, never past it
    assert v.last_ts == datetime(2025, 9, 4)


def test_replay_rejects_a_step_that_does_not_divide_the_range():
    start, end = date(2025, 9, 1), date(2025, 9, 4)
    with pytest.raises(ValueError):
        replay(_valuator(start), [], start, end, timedelta(days=2), lambda d, df: None)
