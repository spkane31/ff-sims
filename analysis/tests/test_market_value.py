"""Work Package 2: the robust anchored market estimator and published curve.

Each test here pins a property the old recursive updater measurably lacked
(see analysis/PLAYER_VALUATION_MODEL_REVIEW.md): volume flattened its curve,
its result depended on row order, old evidence outweighed new, one league
could manufacture consensus, and outliers increased its confidence.
"""

from datetime import datetime, timedelta

import pytest

from src import evaluation, market_value
from src.models import Trade

T0 = datetime(2025, 9, 1)
EPOCH = datetime(1970, 1, 1)


def _trade(i, side_a, side_b, ts, league="lg1"):
    return Trade(
        trade_id=f"t{i:05d}",
        ts=ts,
        side_a=list(side_a),
        side_b=list(side_b),
        created_ms=int((ts - EPOCH).total_seconds() * 1000),
        league_id=league,
    )


def _curve_prior(players: list[str]) -> dict[str, float]:
    """Players in true-rank order -> prior at their curve value."""
    return {
        p: market_value.published_value(rank + 1) for rank, p in enumerate(players)
    }


# ------------------------------------------------------ published curve --


def test_published_curve_has_required_anchors():
    assert round(market_value.published_value(1)) == 10000
    assert round(market_value.published_value(5)) == 8566
    assert round(market_value.published_value(20)) == 4836
    assert round(market_value.published_value(50)) == 1666
    assert round(market_value.published_value(100)) == 485


def test_trade_volume_cannot_flatten_published_curve():
    """5,000 deterministic noisy trades: the published anchors do not move.

    The retired recursive updater collapsed on this kind of stream — every
    equality nudged the whole distribution toward a common level, so more
    noise meant a flatter curve (the review's probe measured a top of ~2,300
    from the 10,000 seed; see PLAYER_VALUATION_MODEL_REVIEW.md). Rank-through-
    the-curve publication makes volume structurally unable to do that."""
    players = [f"p{i:03d}" for i in range(100)]
    prior = _curve_prior(players)
    trades = []
    i = 0
    while len(trades) < 5000:
        # arbitrary package equalities — the review's negative control: each
        # asserts two unrelated packages are equal, which is pure noise
        ids = [(i * 17) % 100, (i * 53 + 11) % 100,
               (i * 29 + 37) % 100, (i * 71 + 5) % 100]
        i += 1
        if len(set(ids)) < 4:
            continue
        ts = T0 + timedelta(hours=len(trades) * 2)
        trades.append(
            _trade(
                len(trades),
                [players[ids[0]], players[ids[1]]],
                [players[ids[2]], players[ids[3]]],
                ts,
            )
        )

    asof = trades[-1].ts + timedelta(days=1)
    fit = market_value.fit_snapshot(trades, asof=asof, adp_prior=prior)
    ordered = sorted(fit.values.values(), reverse=True)
    assert round(ordered[0]) == 10000
    assert round(ordered[4]) == 8566
    assert round(ordered[19]) == 4836


# ------------------------------------------------------- fit properties --


def test_snapshot_fit_is_order_invariant():
    market = evaluation.synthetic_market(n_players=60, seed=11)
    prior = market_value.adp_prior(market.adp)

    forward = market_value.fit_snapshot(
        market.trades, asof=market.end, adp_prior=prior
    )
    backward = market_value.fit_snapshot(
        list(reversed(market.trades)), asof=market.end, adp_prior=prior
    )
    assert forward.scores.keys() == backward.scores.keys()
    worst = max(
        abs(forward.scores[p] - backward.scores[p]) for p in forward.scores
    )
    assert worst <= 1e-6
    assert forward.values == backward.values


def test_recent_market_wins():
    """Equal sample counts: 60-day-old trades imply 4,000, fresh trades imply
    8,000. The current estimate must land closer to 8,000 and above 6,500.
    The old updater did the opposite — early evidence shrank variance, so the
    newest trades moved the belief least."""
    asof = T0 + timedelta(days=60)
    prior = {}
    trades = []
    for i in range(4):  # old regime: X for a 4,000 player, 60 days ago
        anchor = f"a4k_{i}"
        prior[anchor] = 4000.0
        trades.append(_trade(i, ["X"], [anchor], T0, league=f"lgA{i}"))
    for i in range(4):  # new regime: X for an 8,000 player, today
        anchor = f"a8k_{i}"
        prior[anchor] = 8000.0
        trades.append(
            _trade(10 + i, ["X"], [anchor], asof - timedelta(hours=1),
                   league=f"lgB{i}")
        )

    fit = market_value.fit_snapshot(trades, asof=asof, adp_prior=prior)
    x = fit.scores["X"]
    assert x > 6500
    assert abs(x - 8000) < abs(x - 4000)


def test_one_league_cannot_duplicate_an_outlier_into_consensus():
    market = evaluation.synthetic_market(n_players=60, seed=11)
    prior = market_value.adp_prior(market.adp)
    base = market_value.fit_snapshot(market.trades, asof=market.end, adp_prior=prior)

    # one league repeats a nonsense trade 100 times: the worst player for the
    # best, straight up
    spam = [
        _trade(
            9000 + i, ["p59"], ["p00"],
            market.end - timedelta(hours=2, minutes=i), league="synlg0",
        )
        for i in range(100)
    ]
    spammed = market_value.fit_snapshot(
        market.trades + spam, asof=market.end, adp_prior=prior
    )

    for p, before in base.values.items():
        after = spammed.values[p]
        assert abs(after - before) / before < 0.05, (
            f"{p}: {before:,.0f} -> {after:,.0f}"
        )


def test_outlier_does_not_increase_confidence():
    """A |z| > 3 trade is excluded (or fully downweighted) by the two-pass
    fit, and must not shrink the reported market dispersion of anyone in it."""
    market = evaluation.synthetic_market(n_players=60, seed=11)
    prior = market_value.adp_prior(market.adp)
    base = market_value.fit_snapshot(market.trades, asof=market.end, adp_prior=prior)

    crazy = _trade(
        9999, ["p50"], ["p00", "p01"],  # a bench body for the two best players
        market.end - timedelta(hours=1), league="synlg9",
    )
    with_crazy = market_value.fit_snapshot(
        market.trades + [crazy], asof=market.end, adp_prior=prior
    )

    assert "t09999" in with_crazy.outlier_trade_ids
    for p in ("p50", "p00", "p01"):
        assert with_crazy.dispersion[p] >= base.dispersion[p] - 1e-9
    # and the excluded trade cannot have dragged the values themselves
    assert abs(with_crazy.scores["p50"] - base.scores["p50"]) < 0.05 * max(
        base.scores["p50"], 1.0
    )


# ---------------------------------------------------------- diagnostics --


def test_fit_diagnostics_summarize_the_fit():
    """A production run's log must say how healthy the fit is: evidence
    volume, outlier pressure, residual spread, league concentration, and how
    far the market moved players off their ADP prior."""
    market = evaluation.synthetic_market(n_players=60, seed=11)
    prior = market_value.adp_prior(market.adp)
    fit = market_value.fit_snapshot(market.trades, asof=market.end, adp_prior=prior)

    diag = market_value.fit_diagnostics(
        fit, market.trades, prior, asof=market.end
    )
    assert diag["players_fit"] == 60
    assert diag["players_with_prior"] == 60
    assert diag["trade_only_players"] == 0
    assert diag["trades_used"] == fit.n_trades_used
    assert diag["outliers_removed"] == len(fit.outlier_trade_ids)
    assert 0 < diag["inlier_gap_p50"] <= diag["inlier_gap_p90"] <= diag["inlier_gap_max"]
    assert 0 < diag["inlier_package_pct_error"] < 0.5
    assert diag["leagues"] == 8
    assert 0.0 < diag["top_league_weight_share"] < 1.0
    assert diag["prior_spearman"] is not None and diag["prior_spearman"] > 0.8
    assert len(diag["risers"]) <= 3 and len(diag["fallers"]) <= 3
    for pid, delta in diag["risers"]:
        assert pid in fit.ranks and delta > 0
    assert diag["dispersion_p50"] > 0
    assert 0 <= diag["dispersion_at_floor"] <= 60


def test_fit_diagnostics_survive_an_empty_trade_window():
    """The first snapshots of a season have a prior and nothing else; the
    log must degrade rather than crash."""
    prior = {"p1": 10000.0, "p2": 5000.0, "p3": 1000.0}
    fit = market_value.fit_snapshot([], asof=T0, adp_prior=prior)
    diag = market_value.fit_diagnostics(fit, [], prior, asof=T0)
    assert diag["players_fit"] == 3
    assert diag["trades_used"] == 0
    assert diag["leagues"] == 0
    assert diag["inlier_gap_p50"] is None
    assert diag["prior_spearman"] == pytest.approx(1.0)


# ------------------------------------------------------- synthetic e2e --


def test_synthetic_market_end_to_end():
    """The recovery gate: 60 players with known curve values, ~400 fair noisy
    trades. Final published order must track the truth."""
    market = evaluation.synthetic_market(n_players=60, seed=11)
    prior = market_value.adp_prior(market.adp)

    fit = market_value.fit_snapshot(market.trades, asof=market.end, adp_prior=prior)

    rho = evaluation.spearman(fit.values, market.true_values)
    assert rho is not None and rho >= 0.90

    ordered = sorted(fit.values.values(), reverse=True)
    assert all(v >= 8566 - 0.5 for v in ordered[:5])
    assert min(fit.values.values()) >= 300.0 - 1e-9


def test_warm_start_changes_speed_not_the_answer():
    market = evaluation.synthetic_market(n_players=60, seed=11)
    prior = market_value.adp_prior(market.adp)
    cold = market_value.fit_snapshot(market.trades, asof=market.end, adp_prior=prior)
    warm = market_value.fit_snapshot(
        market.trades, asof=market.end, adp_prior=prior, warm_start=cold.scores
    )
    worst = max(abs(cold.scores[p] - warm.scores[p]) for p in cold.scores)
    assert worst <= 1e-3


def test_adp_prior_reads_off_the_published_curve():
    from src.models import AverageDraftPosition

    adp = [
        AverageDraftPosition(player_id="p1", player_name="A", position="QB", adp=1.0),
        AverageDraftPosition(player_id="p2", player_name="B", position="RB", adp=20.0),
    ]
    prior = market_value.adp_prior(adp)
    assert prior["p1"] == pytest.approx(10000.0)
    assert prior["p2"] == pytest.approx(market_value.published_value(20.0))
