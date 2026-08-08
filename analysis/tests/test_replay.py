"""Replay boundary semantics: fixed UTC steps, snapshots dated start-of-day."""

from datetime import date, datetime, timedelta

import pytest

from src.models import AverageDraftPosition, PlayerProfile, Trade, WeeklyScore
from src.config import SEASONS
from src.runner import replay_market, utc_batches, validate_step

S2025 = SEASONS["2025"]
DAY = timedelta(days=1)
REPL = {"QB": 24, "RB": 30, "WR": 36, "TE": 12}

ADP = [
    AverageDraftPosition(player_id="p1", player_name="A", position="QB", adp=1.0),
    AverageDraftPosition(player_id="p2", player_name="B", position="RB", adp=10.0),
]
PLAYERS = {
    "p1": PlayerProfile(player_id="p1", name="A", position="QB"),
    "p2": PlayerProfile(player_id="p2", name="B", position="RB"),
}


def _trade(ts: datetime, a: str = "p1", b: str = "p2") -> Trade:
    return Trade(
        trade_id=f"{a}-{b}-{ts.isoformat()}", ts=ts, side_a=[a], side_b=[b],
        created_ms=int((ts - datetime(1970, 1, 1)).total_seconds() * 1000),
        league_id="lg1",
    )


def _replay(trades, start, end, scores=None, on_snapshot=None):
    return replay_market(
        trades, scores or [], ADP, PLAYERS, S2025, start, end, DAY, REPL,
        on_snapshot,
    )


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


# ----------------------------------------------------------------- replay --


def test_replay_emits_consecutive_dates_including_quiet_days():
    start, end = date(2025, 9, 1), date(2025, 9, 6)
    snaps: list[tuple[date, int]] = []
    stats = _replay(
        [_trade(datetime(2025, 9, 3, 14))], start, end,
        on_snapshot=lambda d, df: snaps.append((d, len(df))),
    )

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

    scores: dict[date, float] = {}
    _replay(
        [_trade(datetime(2025, 9, 3, 14))], start, end,
        on_snapshot=lambda d, df: scores.__setitem__(
            d, float(df.set_index("player_id").loc["p1", "market_score"])
        ),
    )
    assert scores[date(2025, 9, 2)] == scores[date(2025, 9, 3)]
    assert scores[date(2025, 9, 4)] != scores[date(2025, 9, 3)]


def test_replay_counts_every_in_window_event_exactly_once():
    start, end = date(2025, 9, 1), date(2025, 9, 5)
    trades = [
        _trade(datetime(2025, 9, 1, 9)),
        _trade(datetime(2025, 9, 1, 21)),  # same day
        _trade(datetime(2025, 9, 3, 3)),
    ]
    counts: dict[date, int] = {}
    stats = _replay(
        trades, start, end,
        on_snapshot=lambda d, df: counts.__setitem__(
            d, int(df.set_index("player_id").loc["p1", "trades"])
        ),
    )
    assert stats.events_applied == 3
    # the trades column reports how many window trades back each snapshot
    assert counts[date(2025, 9, 2)] == 2
    assert counts[date(2025, 9, 5)] == 3


def test_replay_drops_events_outside_the_window_and_reports_them():
    start, end = date(2025, 9, 2), date(2025, 9, 4)
    trades = [
        _trade(datetime(2025, 9, 1, 12)),  # before start
        _trade(datetime(2025, 9, 2, 12)),  # in window
        _trade(datetime(2025, 9, 4, 0)),   # exactly at the exclusive end
    ]
    stats = _replay(trades, start, end)

    assert stats.events_applied == 1
    assert stats.dropped_before_start == 1
    assert stats.dropped_at_or_after_end == 1


def test_replay_rejects_a_step_that_does_not_divide_the_range():
    with pytest.raises(ValueError):
        replay_market(
            [], [], ADP, PLAYERS, S2025,
            date(2025, 9, 1), date(2025, 9, 4), timedelta(days=2), REPL, None,
        )


def test_week_boundaries_decay_performance_between_snapshots():
    """Weekly scores land at their derived model timestamp and every later
    boundary shows the decayed state — including for players with no row."""
    start, end = date(2025, 9, 1), date(2025, 10, 1)
    week1 = [
        WeeklyScore(week=1, player_id="p1", position="QB", points=30.0),
        WeeklyScore(week=1, player_id="p2", position="RB", points=10.0),
    ]
    projections: dict[date, float] = {}
    _replay(
        [], start, end, scores=week1,
        on_snapshot=lambda d, df: projections.__setitem__(
            d, float(df.set_index("player_id").loc["p1", "projected_par"])
        ),
    )
    # week 1 lands Sep 8; before that the projection is the preseason prior
    assert projections[date(2025, 9, 8)] == projections[date(2025, 9, 2)]
    assert projections[date(2025, 9, 9)] != projections[date(2025, 9, 8)]


def test_a_trade_only_player_gets_its_real_identity():
    """Trades carry bare player IDs; the resolved identity map is what names
    and positions a player who was never drafted."""
    start, end = date(2025, 9, 1), date(2025, 9, 3)
    players = dict(PLAYERS)
    players["w7"] = PlayerProfile(player_id="w7", name="WR Seven", position="WR")

    frames = {}
    replay_market(
        [_trade(datetime(2025, 9, 1, 12), a="p1", b="w7")], [], ADP, players,
        S2025, start, end, DAY, REPL,
        lambda d, df: frames.__setitem__(d, df),
    )
    ranked = frames[date(2025, 9, 3)].set_index("player_id")
    assert ranked.loc["w7", "player"] == "WR Seven"
    assert ranked.loc["w7", "pos"] == "WR"
    assert "DEFAULT" not in set(ranked["pos"])
