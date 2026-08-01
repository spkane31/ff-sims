"""Evaluation seam: holdouts, metrics, and the flat negative control.

The old model's health metric (held-out trade MAE) is minimized by a constant
value for everyone, so these tests pin the properties an evaluation must have
before it can select between models: league/time-blocked splits, and a
validity gate a flat model cannot pass no matter how low its package error is.
"""

from datetime import datetime, timedelta

import pytest

from src import evaluation
from src.models import Trade

T0 = datetime(2025, 9, 1)


def _trade(i: int, league: str, ts: datetime, side_a, side_b) -> Trade:
    return Trade(
        trade_id=f"t{i}",
        ts=ts,
        side_a=list(side_a),
        side_b=list(side_b),
        created_ms=int((ts - datetime(1970, 1, 1)).total_seconds() * 1000),
        league_id=league,
    )


def _many_trades(n: int = 200, leagues: int = 7) -> list[Trade]:
    return [
        _trade(i, f"lg{i % leagues}", T0 + timedelta(hours=i), ["a"], ["b"])
        for i in range(n)
    ]


# ------------------------------------------------------------- splits --


def test_league_blocked_split_never_puts_a_league_on_both_sides():
    train, test = evaluation.league_blocked_split(_many_trades(), test_fraction=0.4)
    assert train and test
    assert {t.league_id for t in train}.isdisjoint({t.league_id for t in test})


def test_league_blocked_split_is_deterministic():
    trades = _many_trades()
    first = evaluation.league_blocked_split(trades, test_fraction=0.4)
    second = evaluation.league_blocked_split(trades, test_fraction=0.4)
    assert [t.trade_id for t in first[0]] == [t.trade_id for t in second[0]]
    assert [t.trade_id for t in first[1]] == [t.trade_id for t in second[1]]


def test_unknown_league_trades_stay_in_train():
    """v2-era bundles have no league_id; those trades cannot be blocked out,
    so they must never leak into the test side."""
    trades = _many_trades() + [
        _trade(900 + i, "", T0 + timedelta(days=1), ["a"], ["b"]) for i in range(5)
    ]
    train, test = evaluation.league_blocked_split(trades, test_fraction=0.4)
    assert all(t.league_id != "" for t in test)


def test_time_blocked_split_cuts_at_the_boundary():
    trades = _many_trades(48)
    cutoff = T0 + timedelta(hours=24)
    train, test = evaluation.time_blocked_split(trades, cutoff)
    assert train and test
    assert all(t.ts < cutoff for t in train)
    assert all(t.ts >= cutoff for t in test)
    assert len(train) + len(test) == len(trades)


# ------------------------------------------------------------- metrics --


def test_package_error_metrics():
    values = {"a": 100.0, "b": 60.0, "c": 40.0}
    trades = [
        _trade(1, "lg1", T0, ["a"], ["b", "c"]),  # 100 vs 100 -> gap 0
        _trade(2, "lg1", T0, ["a"], ["b"]),  # 100 vs 60 -> gap 40
    ]
    report = evaluation.evaluate(values, trades)
    assert report.n_trades == 2
    assert report.package_mae == pytest.approx(20.0)
    # pct error is gap over the mean package size: 0 and 40/80
    assert report.package_pct_error == pytest.approx(0.25)


def test_trades_touching_unvalued_players_are_skipped_and_counted():
    values = {"a": 100.0, "b": 60.0}
    trades = [
        _trade(1, "lg1", T0, ["a"], ["b"]),
        _trade(2, "lg1", T0, ["a"], ["mystery"]),
    ]
    report = evaluation.evaluate(values, trades)
    assert report.n_trades == 1
    assert report.skipped_trades == 1


def test_flat_model_is_invalid_despite_perfect_package_error():
    """The negative control: a constant-value model satisfies every balanced
    trade exactly. Lower package MAE must not make it selectable."""
    players = [f"p{i:02d}" for i in range(60)]
    trades = [
        _trade(i, f"lg{i % 5}", T0 + timedelta(hours=i), [players[i % 60]],
               [players[(i * 7 + 3) % 60]])
        for i in range(100)
    ]

    flat = dict.fromkeys(players, 1445.0)  # value == rho, the old attractor
    flat_report = evaluation.evaluate(flat, trades)
    assert flat_report.package_mae == pytest.approx(0.0)
    assert not flat_report.curve_valid
    assert not flat_report.valid

    curved = {
        p: evaluation.published_value(rank + 1) for rank, p in enumerate(players)
    }
    curved_report = evaluation.evaluate(curved, trades)
    assert curved_report.package_mae > flat_report.package_mae
    assert curved_report.curve_valid
    assert curved_report.valid


def test_flat_control_report_is_built_from_the_model_universe():
    players = ["a", "b", "c"]
    trades = [_trade(1, "lg1", T0, ["a"], ["b"])]
    control = evaluation.flat_control_report(players, trades)
    assert control.package_mae == pytest.approx(0.0)
    assert not control.valid


# ------------------------------------------- synthetic market recovery --


def test_synthetic_market_true_values_recover_ranks():
    """A model handed the exact true values of the synthetic market must
    score essentially perfect rank recovery — the metric's own sanity gate."""
    market = evaluation.synthetic_market(n_players=60, seed=11)
    assert len(market.true_values) == 60
    assert len(market.trades) >= 200

    report = evaluation.evaluate(
        market.true_values, market.trades, reference=market.true_values
    )
    assert report.reference_spearman is not None
    assert report.reference_spearman >= 0.95


def test_synthetic_market_is_deterministic():
    a = evaluation.synthetic_market(n_players=60, seed=11)
    b = evaluation.synthetic_market(n_players=60, seed=11)
    assert a.true_values == b.true_values
    assert [t.trade_id for t in a.trades] == [t.trade_id for t in b.trades]


def test_spearman_against_an_external_benchmark():
    values = {"a": 10.0, "b": 8.0, "c": 5.0, "d": 1.0}
    aligned = {"a": 9000.0, "b": 7000.0, "c": 4000.0, "d": 300.0}
    reversed_bench = {"a": 1.0, "b": 2.0, "c": 3.0, "d": 4.0}
    assert evaluation.spearman(values, aligned) == pytest.approx(1.0)
    assert evaluation.spearman(values, reversed_bench) == pytest.approx(-1.0)


def test_published_curve_anchor_values():
    assert evaluation.published_value(1) == pytest.approx(10000.0)
    assert round(evaluation.published_value(5)) == 8566
    assert round(evaluation.published_value(20)) == 4836
    assert round(evaluation.published_value(50)) == 1666
    assert round(evaluation.published_value(100)) == 485
