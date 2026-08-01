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


def test_a_trade_only_player_gets_its_real_identity_not_a_default_belief():
    """Trades carry bare player IDs. Without the resolved identity map, the
    only player in a trade who was never drafted becomes a nameless DEFAULT
    belief — wrong drift rate, and a DEFAULT row published to the API."""
    v = Valuator(
        start_ts=datetime(2025, 8, 25),
        repl_rank_by_pos=REPL,
        identities={"w7": ("WR Seven", "WR")},
    )
    v.seed_from_adp(_adp())

    v.apply_trade(["p1"], ["w7"])

    assert v.beliefs["w7"].position == "WR"
    assert v.beliefs["w7"].name == "WR Seven"
    ranked = v.rankings().set_index("player_id")
    assert ranked.loc["w7", "player"] == "WR Seven"
    assert "DEFAULT" not in set(ranked["pos"])


def test_trade_fit_accumulates_so_consensus_is_measurable():
    """Mean |gap| is the consensus signal: how far each trade landed from what
    the model already believed. Without it a run can only report that trades
    were applied, not whether they agreed with the values."""
    v = _valuator(date(2025, 8, 25))
    assert (v.trades_applied, v.trade_abs_gap) == (0, 0.0)

    v.apply_trade(["p1"], ["p2"])  # QB1 for RB10: a large gap
    first_gap = v.trade_abs_gap
    assert v.trades_applied == 1
    assert first_gap > 0

    # the same trade again moves the values closer together, so it now
    # surprises the model less than it did the first time
    v.apply_trade(["p1"], ["p2"])
    assert v.trades_applied == 2
    assert v.trade_abs_gap - first_gap < first_gap


def test_a_trade_the_model_cannot_use_is_not_counted_as_fit():
    v = _valuator(date(2025, 8, 25))
    v.apply_trade(["p1"], [])  # one-sided: no constraint to fit
    assert v.trades_applied == 0


def test_diagnostics_summarize_evidence_coverage_and_spread():
    v = _valuator(date(2025, 8, 25))
    v.apply_trade(["p1"], ["idp1"])  # pulls in a player with no ADP and no scores

    d = v.diagnostics()
    assert d["beliefs"] == 3
    assert d["scored"] == 0  # no weeks applied in this test
    assert d["never_scored"] == 3
    assert d["trades_applied"] == 1
    assert d["value_top"] > d["value_p50"]
    assert d["sd_p90"] >= d["sd_p50"]
    assert d["by_position"]["QB"] == 1
    assert d["last_ts"] == datetime(2025, 8, 25)


def test_curve_rank_inverts_the_seed_curve():
    """The top value is only interpretable next to the rank it implies."""
    from src.valuation import curve, curve_rank

    assert round(curve_rank(curve(1))) == 1
    assert round(curve_rank(curve(20))) == 20
    assert curve_rank(0) == float("inf")


def test_each_side_of_a_trade_counts_toward_its_players():
    """`games` measures performance evidence; this is the market counterpart,
    so a value backed by hundreds of trades is distinguishable from one backed
    by two."""
    v = _valuator(date(2025, 8, 25))
    assert v.beliefs["p1"].trades == 0

    v.apply_trade(["p1"], ["p2"])
    v.apply_trade(["p1"], ["p2"])
    v.apply_trade(["p2"], ["p1"])  # sides swapped: still one trade for each

    assert v.beliefs["p1"].trades == 3
    assert v.beliefs["p2"].trades == 3

    ranked = v.rankings().set_index("player_id")
    assert int(ranked.loc["p1", "trades"]) == 3


def test_a_trade_the_model_cannot_use_counts_for_nobody():
    v = _valuator(date(2025, 8, 25))
    v.apply_trade(["p1"], [])
    assert v.beliefs["p1"].trades == 0


def test_trade_counts_are_not_decayed_like_games():
    """Trade count answers how much evidence exists over the whole run, not
    how recent it is, so it must stay a raw count."""
    v = _valuator(date(2025, 8, 25))
    for _ in range(10):
        v.apply_trade(["p1"], ["p2"])
    assert v.beliefs["p1"].trades == 10  # not 6.2-style decayed


# ------------------------------------------------- trade residual spread --


def _wide_valuator() -> Valuator:
    """Beliefs far enough apart that a lopsided trade is a real outlier."""
    v = Valuator(start_ts=datetime(2025, 8, 25), repl_rank_by_pos=REPL)
    v.seed_from_adp(
        pd.DataFrame(
            [
                {"player_id": "stud", "player_name": "S", "position": "RB", "adp": 1.0},
                {"player_id": "mid", "player_name": "M", "position": "RB", "adp": 20.0},
                {"player_id": "scrub", "player_name": "C", "position": "RB", "adp": 150.0},
            ]
        )
    )
    for b in v.beliefs.values():
        b.var = 10_000.0  # confident, so a big gap is the trade's fault
    return v


def test_the_gap_distribution_is_recorded_not_just_its_mean():
    """A mean cannot distinguish "every trade is slightly off" from "most are
    fair and a few are dumps", which is the difference that decides whether
    rejecting outliers would change anything."""
    v = _wide_valuator()
    v.apply_trade(["stud"], ["mid"])  # mildly off
    v.apply_trade(["stud"], ["scrub"])  # a dump

    d = v.diagnostics()
    assert len(v.trade_gaps) == 2
    assert d["gap_max"] == max(v.trade_gaps)
    assert d["gap_p50"] <= d["gap_p99"] <= d["gap_max"]


def test_a_lopsided_trade_is_counted_as_an_outlier_and_a_fair_one_is_not():
    v = _wide_valuator()
    v.apply_trade(["mid"], ["mid"])  # identical sides: zero residual
    assert v.outlier_trades == 0

    v.apply_trade(["stud"], ["scrub"])
    assert v.outlier_trades == 1
    assert v.diagnostics()["outlier_share"] == 0.5


def test_the_share_of_movement_outliers_cause_is_reported():
    """The decisive number: a few trades carrying most of the value movement
    is the case where a robust update changes the answer."""
    v = _wide_valuator()
    v.apply_trade(["mid"], ["mid"])
    v.apply_trade(["stud"], ["scrub"])

    d = v.diagnostics()
    assert d["outlier_move_share"] > 0.99  # the fair trade moved nothing
    assert v.trade_move_outlier <= v.trade_move_total


def test_confidence_decides_what_counts_as_an_outlier_not_raw_size():
    """z is the residual over its own expected spread, so the same trade is
    information early and a dump later. Without that, every trade looks like an
    outlier in preseason when the model knows nothing."""
    # stud-for-mid, not stud-for-scrub: the latter is implausible at any
    # confidence, so it could not show the threshold moving.
    unsure = _wide_valuator()
    for b in unsure.beliefs.values():
        b.var = 4_000_000.0  # preseason: wide open
    unsure.apply_trade(["stud"], ["mid"])

    sure = _wide_valuator()  # var 10_000: confident
    sure.apply_trade(["stud"], ["mid"])

    assert unsure.trade_gaps[0] == sure.trade_gaps[0]  # same raw disagreement
    assert unsure.outlier_trades == 0
    assert sure.outlier_trades == 1
