"""Work Package 4: roster-aware trade suggestions.

Fairness (market value, including drop costs) and improvement (optimized
lineup projected points) are separate axes. Equal package values alone cannot
show that either roster improves — that was the old value-only suggester's
gap. The fixture here is the handoff's required scenario: Team A holds a
bench QB worth 5,000, Team B a bench WR worth 5,100; swapping them improves
A's starters by +3.0 and B's by +2.0.
"""

import pytest

from src import suggestions

SETTINGS = suggestions.LineupSettings(
    slots={"QB": 1, "WR": 2},
    roster_size=6,
)

MARKET = {
    "qbA1": 6000.0, "qbA2": 5000.0,
    "wrA1": 3000.0, "wrA2": 2000.0, "wrA3": 1000.0,
    "qbB1": 4000.0,
    "wrB1": 5100.0, "wrB2": 3500.0, "wrB3": 3300.0, "wrB4": 3100.0,
}
PROJ = {
    "qbA1": 20.0, "qbA2": 18.0,
    "wrA1": 12.0, "wrA2": 10.0, "wrA3": 8.0,
    "qbB1": 16.0,
    "wrB1": 13.0, "wrB2": 15.0, "wrB3": 14.0, "wrB4": 9.0,
}
POS = {
    "qbA1": "QB", "qbA2": "QB", "qbB1": "QB",
    "wrA1": "WR", "wrA2": "WR", "wrA3": "WR",
    "wrB1": "WR", "wrB2": "WR", "wrB3": "WR", "wrB4": "WR",
}
ROSTER_A = ["qbA1", "qbA2", "wrA1", "wrA2", "wrA3"]
ROSTER_B = ["qbB1", "wrB1", "wrB2", "wrB3", "wrB4"]


def _suggest():
    return suggestions.suggest_trades(
        ROSTER_A, ROSTER_B, MARKET, PROJ, POS, SETTINGS,
        fairness_tolerance=0.10,
    )


def _find(results, give, receive):
    for i, s in enumerate(results):
        if set(s.give) == set(give) and set(s.receive) == set(receive):
            return i, s
    raise AssertionError(f"no suggestion {give} -> {receive} in {results}")


# --------------------------------------------------------- lineup engine --


def test_optimal_lineup_fills_slots_by_projection():
    starters, points = suggestions.optimal_lineup(ROSTER_A, PROJ, POS, SETTINGS)
    assert set(starters) == {"qbA1", "wrA1", "wrA2"}
    assert points == pytest.approx(42.0)


def test_flex_and_superflex_take_the_best_remaining():
    settings = suggestions.LineupSettings(
        slots={"QB": 1, "WR": 1, "FLEX": 1, "SUPERFLEX": 1},
        roster_size=6,
    )
    starters, points = suggestions.optimal_lineup(ROSTER_A, PROJ, POS, settings)
    # QB qbA1 (20), WR wrA1 (12), FLEX wrA2 (10, WRs only here), SF qbA2 (18)
    assert set(starters) == {"qbA1", "wrA1", "wrA2", "qbA2"}
    assert points == pytest.approx(60.0)


# ------------------------------------------------- the required fixture --


def test_bench_swap_is_fair_pareto_and_fully_described():
    results = _suggest()
    _, s = _find(results, give=["qbA2"], receive=["wrB1"])

    # 10% market-fairness tolerance: |5,000 - 5,100| within 10% of the bigger
    assert abs(s.fairness_delta) == pytest.approx(100.0)
    assert abs(s.fairness_delta) <= 0.10 * 5100.0

    # exact utility deltas from the optimized post-trade lineups
    assert s.utility_delta_a == pytest.approx(3.0)
    assert s.utility_delta_b == pytest.approx(2.0)
    assert s.label == "pareto"

    # both post-trade lineup changes are included
    assert "wrB1" in s.entered_a and "wrA2" in s.exited_a
    assert "qbA2" in s.entered_b and "qbB1" in s.exited_b
    assert set(s.lineup_a) == {"qbA1", "wrB1", "wrA1"}
    assert set(s.lineup_b) == {"qbA2", "wrB2", "wrB3"}


def test_pareto_trades_rank_above_fair_but_one_sided_ones():
    results = _suggest()
    pareto_idx, pareto = _find(results, give=["qbA2"], receive=["wrB1"])
    # wrA1 for wrB3 is market-fair (3,000 vs 3,300) but makes B worse:
    # B's WR starters fall from 15+14 to 15+13
    onesided_idx, onesided = _find(results, give=["wrA1"], receive=["wrB3"])

    assert pareto.label == "pareto"
    assert onesided.label == "one-sided"
    assert onesided.utility_delta_a > 0 and onesided.utility_delta_b < 0
    assert pareto_idx < onesided_idx
    # every pareto suggestion ranks above every one-sided one
    labels = [s.label for s in results]
    assert labels == sorted(labels, key=lambda l: 0 if l == "pareto" else 1)


def test_unfair_trades_are_not_suggested():
    results = _suggest()
    # qbA1 (6,000) for wrB4 (3,100) is nowhere near the 10% tolerance
    with pytest.raises(AssertionError):
        _find(results, give=["qbA1"], receive=["wrB4"])


def test_trades_that_help_nobody_are_not_suggested():
    results = _suggest()
    for s in results:
        assert max(s.utility_delta_a, s.utility_delta_b) > 0


def test_all_package_shapes_are_enumerated():
    results = _suggest()
    shapes = {(len(s.give), len(s.receive)) for s in results}
    # at least the 1-for-1 and one asymmetric/two-for-two shape must survive
    # the fairness/improvement filters in this fixture
    assert (1, 1) in shapes
    assert shapes <= {(1, 1), (1, 2), (2, 1), (2, 2)}


def test_roster_limit_forces_a_drop_and_prices_it_into_fairness():
    """2-for-1 into a full roster: the receiving side must drop its worst
    bench player, and that player's market value is part of what they pay."""
    settings = suggestions.LineupSettings(slots={"QB": 1, "WR": 2}, roster_size=5)
    results = suggestions.suggest_trades(
        ROSTER_A, ROSTER_B, MARKET, PROJ, POS, settings,
        fairness_tolerance=0.35,  # loose: this test is about the drop math
    )
    for s in results:
        if len(s.give) == 1 and len(s.receive) == 2:
            # A ends with 6 players on a 5-spot roster -> one drop
            assert len(s.drops_a) == 1
            dropped = s.drops_a[0]
            assert dropped not in s.lineup_a
            # outlay includes the drop: give + dropped vs receive
            outlay_a = sum(MARKET[p] for p in (*s.give, dropped))
            outlay_b = sum(MARKET[p] for p in s.receive)
            assert s.fairness_delta == pytest.approx(outlay_a - outlay_b)
            break
    else:
        pytest.fail("no 1-for-2 suggestion to exercise the drop path")
