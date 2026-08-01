"""Work Package 3: magnitude-preserving performance, universal decay,
projected PAR, named uncertainties, and query-time positional replacement.

The old model converted PAR magnitude to an overall rank and fused the same
cumulative evidence weekly: a breakout followed by six zero-point weeks rose
from 3,752 to 9,032 in published value, and absent players never decayed.
"""

from datetime import timedelta

import pytest

from src import evaluation, market_value, performance
from src.models import WeeklyScore

REPL = {"QB": 2, "RB": 2, "WR": 2, "TE": 1}


def _week(rows):
    return [
        WeeklyScore(week=w, player_id=p, position=pos, points=pts)
        for (w, p, pos, pts) in rows
    ]


def _filler(week, n=3, pos="RB", pts=10.0):
    """Enough scorers that the replacement rank exists every week."""
    return [
        (week, f"fill{i}", pos, pts + i) for i in range(n)
    ]


# ------------------------------------------------- decay every boundary --


def test_five_missed_weeks_decay_par_through_every_boundary():
    tracker = performance.PerformanceTracker(REPL)
    # week 1: the player outscores replacement by exactly 20
    tracker.apply_week(_week([(1, "star", "RB", 32.0)] + _filler(1, pts=10.0)))
    assert tracker.state("star").cum_par == pytest.approx(20.0)

    for week in range(2, 7):  # five boundaries with no row for the player
        tracker.apply_week(_week(_filler(week, pts=10.0)))

    assert tracker.state("star").cum_par == pytest.approx(
        20.0 * 0.85**5, abs=1e-4
    )
    assert tracker.state("star").cum_par == pytest.approx(8.8741, abs=1e-3)


def test_breakout_then_six_zero_weeks_decreases_projected_par():
    """The old probe rose from 3,752 to 9,032 published value across the six
    zero weeks. The projected signal must fall instead."""
    tracker = performance.PerformanceTracker(REPL, preseason_par={"star": ("RB", 2.0)})
    tracker.apply_week(_week([(1, "star", "RB", 32.0)] + _filler(1, pts=12.0)))
    after_breakout = tracker.projected_par("star")
    assert after_breakout > 2.0  # the breakout pulled projection up

    trajectory = [after_breakout]
    for week in range(2, 8):  # six weeks of zero points, playing
        tracker.apply_week(
            _week([(week, "star", "RB", 0.0)] + _filler(week, pts=12.0))
        )
        trajectory.append(tracker.projected_par("star"))

    assert trajectory[-1] < after_breakout
    # and the fall is monotone: no week of zeros may push projection up
    assert all(b <= a + 1e-9 for a, b in zip(trajectory, trajectory[1:]))


def test_projected_par_blends_preseason_with_observed_form():
    tracker = performance.PerformanceTracker(REPL, preseason_par={"p": ("RB", 8.0)})
    assert tracker.projected_par("p") == pytest.approx(8.0)  # nothing observed

    tracker.apply_week(_week([(1, "p", "RB", 22.0)] + _filler(1, pts=10.0)))
    # one game of PAR 10: (4*8 + 1*10) / (4+1)
    assert tracker.projected_par("p") == pytest.approx((4 * 8.0 + 10.0) / 5.0)


# ---------------------------------------------- separation from market --


def test_weekly_scores_change_projected_par_but_not_market_value():
    market = evaluation.synthetic_market(n_players=40, seed=5, n_trades=150)
    prior = market_value.adp_prior(market.adp)
    before = market_value.fit_snapshot(market.trades, asof=market.end, adp_prior=prior)

    tracker = performance.PerformanceTracker(REPL, preseason_par={"p00": ("QB", 4.0)})
    base_projection = tracker.projected_par("p00")
    tracker.apply_week(_week([(1, "p00", "QB", 40.0)] + _filler(1, pos="QB", pts=15.0)))

    assert tracker.projected_par("p00") != pytest.approx(base_projection)
    after = market_value.fit_snapshot(market.trades, asof=market.end, adp_prior=prior)
    assert after.values == before.values
    assert after.scores == before.scores


# ------------------------------------------------- named uncertainties --


def test_projection_uncertainty_narrows_with_consistent_games():
    tracker = performance.PerformanceTracker(REPL, preseason_par={"p": ("RB", 5.0)})
    unseen = tracker.projection_uncertainty("p")
    for week in range(1, 7):
        tracker.apply_week(_week([(week, "p", "RB", 15.0)] + _filler(week, pts=10.0)))
    seasoned = tracker.projection_uncertainty("p")
    assert seasoned < unseen


def test_repeating_a_correlated_trade_does_not_shrink_market_dispersion():
    """The old sd shrank with every duplicate. market_dispersion measures
    across distinct leagues, so one league repeating itself is one opinion."""
    market = evaluation.synthetic_market(n_players=40, seed=5, n_trades=150)
    prior = market_value.adp_prior(market.adp)

    dup = [t for t in market.trades if len(t.side_a) == 1 and len(t.side_b) == 1][0]
    clones = [
        type(dup)(
            trade_id=f"dup{i:03d}",
            ts=dup.ts + timedelta(minutes=i),
            side_a=dup.side_a,
            side_b=dup.side_b,
            created_ms=dup.created_ms + i,
            league_id=dup.league_id,
        )
        for i in range(50)
    ]
    base = market_value.fit_snapshot(market.trades, asof=market.end, adp_prior=prior)
    spammed = market_value.fit_snapshot(
        market.trades + clones, asof=market.end, adp_prior=prior
    )
    pid = dup.side_a[0]
    assert spammed.dispersion[pid] >= min(
        base.dispersion[pid],
        market_value.MIN_DISPERSION,
    )
    # and never anywhere near zero
    assert spammed.dispersion[pid] >= market_value.MIN_DISPERSION


# ------------------------------------- query-time positional replacement --


def test_replacement_is_positional_and_vorp_uses_it():
    """QB replacement 2,000 and RB replacement 800: a 3,000-value QB has
    VORP 1,000 while a 3,000-value RB has VORP 2,200. The old model
    subtracted one global rho from every position."""
    values = {
        "qb1": 3000.0, "qb2": 2000.0,
        "rb1": 3000.0, "rb2": 800.0, "rb3": 500.0,
        "wr1": 1200.0,
    }
    positions = {
        "qb1": "QB", "qb2": "QB",
        "rb1": "RB", "rb2": "RB", "rb3": "RB",
        "wr1": "WR",
    }
    rostered = {"qb1", "rb1"}  # qb2/rb2 are the best available at their spots

    repl = performance.replacement_by_position(values, positions, rostered)
    assert repl["QB"] == pytest.approx(2000.0)
    assert repl["RB"] == pytest.approx(800.0)

    assert performance.vorp(3000.0, "QB", repl) == pytest.approx(1000.0)
    assert performance.vorp(3000.0, "RB", repl) == pytest.approx(2200.0)


def test_replacement_defaults_to_zero_when_nothing_is_available():
    repl = performance.replacement_by_position(
        {"a": 100.0}, {"a": "QB"}, rostered={"a"}
    )
    assert repl["QB"] == 0.0
    assert performance.vorp(100.0, "QB", repl) == pytest.approx(100.0)
