"""Estimating replacement value from unbalanced trades.

ρ enters the trade rule only as ρ·(|B|−|A|), so its whole identifying signal
is trades whose sides differ in size. These pin the accumulator arithmetic
exactly, then the iteration behaviour around it.
"""

from datetime import date, datetime, timedelta

import pandas as pd

from src.runner import fit_rho
from src.valuation import Valuator, curve

REPL = {"QB": 24, "RB": 30, "WR": 36, "TE": 12}
START, END, STEP = date(2025, 9, 1), date(2025, 9, 11), timedelta(days=1)
TS = datetime(2025, 9, 5)


def _adp(n: int = 40) -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "player_id": f"p{i:02d}",
                "player_name": f"P{i:02d}",
                "position": "RB",
                "adp": float(i + 1),
            }
            for i in range(n)
        ]
    )


def _valuator(rho: float = 0.0) -> Valuator:
    v = Valuator(start_ts=datetime(2025, 9, 1), repl_rank_by_pos=REPL, rho=rho)
    v.seed_from_adp(_adp())
    return v


def _trade(side_a, side_b, ts=TS):
    return {"ts": ts, "kind": "trade", "side_a": side_a, "side_b": side_b}


# ------------------------------------------------------- the accumulators --


def test_a_balanced_trade_says_nothing_about_rho():
    """n = 0, so it contributes to neither sum — a 1-for-1 is satisfied by any
    ρ at all, which is exactly why ρ is unidentified without unbalanced ones."""
    v = _valuator()
    v.apply_trade(["p00"], ["p05"])

    assert v.trades_applied == 1
    assert v.unbalanced_trades == 0
    assert (v.rho_dn, v.rho_nn) == (0.0, 0.0)


def test_an_unbalanced_trade_contributes_the_least_squares_terms():
    v = _valuator()
    # 1-for-2: n = |b| - |a| = 1, d = sum(b) - sum(a)
    expected_d = curve(6.0) + curve(7.0) - curve(1.0)

    v.apply_trade(["p00"], ["p05", "p06"])

    assert v.unbalanced_trades == 1
    assert v.rho_nn == 1.0
    assert v.rho_dn == expected_d


def test_a_three_for_one_weighs_more_than_a_two_for_one():
    """n = 2 contributes n² = 4, so the more lopsided trade carries more
    weight in the fit — it says more about what a roster spot is worth."""
    v = _valuator()
    v.apply_trade(["p00"], ["p05", "p06", "p07"])
    assert v.rho_nn == 4.0


# ------------------------------------------------------------ the fitting --


def test_rho_is_left_at_the_seed_when_nothing_identifies_it():
    events = [_trade(["p00"], ["p05"]), _trade(["p01"], ["p06"])]
    fit = fit_rho(_valuator, events, START, END, STEP, seed=17.0)

    assert fit.rho == 17.0
    assert fit.unbalanced == 0
    assert fit.converged  # nothing to converge toward; not a failure


def test_a_consolidation_premium_fits_rho_up():
    """Rearranged, the rule says sum(B) = sum(A) + ρ when B has one more
    player: the side giving up a roster spot has to receive more raw value to
    break even. So ρ > 0 is a market where packages sum to *more* than the
    single player they buy — the premium paid to consolidate."""
    events = [
        _trade([f"p{i:02d}"], [f"p{i + 2:02d}", f"p{i + 30:02d}"], TS)
        for i in range(8)
    ]
    fit = fit_rho(_valuator, events, START, END, STEP, seed=0.0)

    assert fit.unbalanced == 8
    assert fit.rho > 0


def test_a_negative_estimate_is_clamped_but_still_reported():
    """A negative replacement value is not a thing. Clamping silently would
    hide that the data asked for one — and this is not a contrived case: a
    market that trades one stud for two lesser players summing to less than
    him implies exactly this, and real trade data may well say so."""
    # the single player is worth far more than the pair, so d·n < 0
    events = [_trade([f"p{i:02d}"], ["p38", "p39"], TS) for i in range(10)]
    fit = fit_rho(_valuator, events, START, END, STEP, seed=17.0)

    assert fit.raw < 0
    assert fit.rho == 0.0


def test_fitting_passes_build_no_snapshots():
    """Each pass replays the whole event stream; if they built rankings the
    fit would cost more than the published run it precedes."""
    events = [_trade(["p00"], ["p05", "p06"])]
    calls: list[int] = []

    def make(rho):
        v = _valuator(rho)
        original = v.rankings
        v.rankings = lambda: (calls.append(1), original())[1]  # type: ignore[method-assign]
        return v

    fit_rho(make, events, START, END, STEP, seed=17.0, iterations=3)
    assert calls == []
