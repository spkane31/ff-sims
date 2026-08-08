"""Evaluation seam for valuation models. No database access anywhere here.

Two rules shape this module, both learned from the old model:

* Splits are blocked, never random-by-row. Trades from one league share
  players, managers, and market quirks; letting a league straddle train and
  test leaks exactly the structure the model memorizes. Time blocks guard the
  other axis: a model fitted on the future of its own test set proves nothing.
* Low held-out package error is NOT sufficient evidence of a good model. A
  constant value for every player satisfies balanced trades exactly, so every
  report carries a `curve_valid` gate the flat solution cannot pass, and
  `flat_control_report` makes the negative control explicit.
"""

from __future__ import annotations

import hashlib
import math
from dataclasses import dataclass, field
from datetime import datetime, timedelta

import numpy as np
import pandas as pd

from .market_value import published_value
from .models import AverageDraftPosition, Trade

# Relative tolerance for the published-curve anchor check. Wide enough that a
# real fit with a shuffled tail passes, far too tight for a compressed or flat
# value distribution (the old model's top sat at ~47% of the curve top).
CURVE_ANCHOR_TOLERANCE = 0.10
CURVE_ANCHOR_RANKS = (1, 5, 20, 50, 100)


# ------------------------------------------------------------------ splits --


def _league_bucket(league_id: str) -> int:
    """Stable per-league hash, independent of Python's seeded str hash."""
    return int.from_bytes(
        hashlib.sha256(league_id.encode()).digest()[:8], "big"
    )


def league_blocked_split(
    trades: list[Trade], test_fraction: float = 0.25
) -> tuple[list[Trade], list[Trade]]:
    """Split trades so no league appears on both sides.

    Leagues are ordered by a stable hash and the last `test_fraction` of them
    become the test block, so the assignment is deterministic across runs and
    machines. Trades with an unknown league ("" — v2-era bundles) cannot be
    blocked and therefore always stay in train.
    """
    if not 0.0 < test_fraction < 1.0:
        raise ValueError(f"test_fraction must be in (0, 1), got {test_fraction}")
    leagues = sorted(
        {t.league_id for t in trades if t.league_id}, key=_league_bucket
    )
    n_test = math.ceil(len(leagues) * test_fraction) if leagues else 0
    test_leagues = set(leagues[len(leagues) - n_test:])
    train = [t for t in trades if t.league_id not in test_leagues or not t.league_id]
    test = [t for t in trades if t.league_id and t.league_id in test_leagues]
    return train, test


def time_blocked_split(
    trades: list[Trade], cutoff: datetime
) -> tuple[list[Trade], list[Trade]]:
    """Train strictly before the cutoff, evaluate at/after it."""
    train = [t for t in trades if t.ts < cutoff]
    test = [t for t in trades if t.ts >= cutoff]
    return train, test


def time_cutoff(trades: list[Trade], test_fraction: float = 0.25) -> datetime:
    """Cutoff timestamp leaving ~test_fraction of TRADES after it.

    A quantile of trade timestamps by count, deliberately not of the calendar
    span: redraft trading dies when the season ends, so most of the calendar
    is dead offseason and a span-based 3/4 point can leave a single trade in
    the test block (measured on real data: 39,234 train / 1 test).
    """
    if not trades:
        raise ValueError("no trades to split")
    if not 0.0 < test_fraction < 1.0:
        raise ValueError(f"test_fraction must be in (0, 1), got {test_fraction}")
    ordered = sorted(t.ts for t in trades)
    idx = min(len(ordered) - 1, int(len(ordered) * (1.0 - test_fraction)))
    return ordered[idx]


# ----------------------------------------------------------------- metrics --


def spearman(values: dict[str, float], reference: dict[str, float]) -> float | None:
    """Spearman rank correlation over the players both sides know."""
    common = sorted(set(values) & set(reference))
    if len(common) < 3:
        return None
    a = pd.Series([values[p] for p in common]).rank()
    b = pd.Series([reference[p] for p in common]).rank()
    return float(np.corrcoef(a, b)[0, 1])


@dataclass(frozen=True)
class EvaluationReport:
    n_trades: int
    skipped_trades: int
    package_mae: float
    package_pct_error: float
    curve_anchors: dict[int, float]  # rank -> model value at that rank
    curve_valid: bool
    value_spread: float  # top value / median value (flat model -> 1.0)
    reference_spearman: float | None
    valid: bool  # the selection gate: curve_valid, never package error alone
    notes: list[str] = field(default_factory=list)


def _curve_check(ordered_values: list[float]) -> tuple[dict[int, float], bool, list[str]]:
    anchors: dict[int, float] = {}
    notes: list[str] = []
    ok = bool(ordered_values)
    for rank in CURVE_ANCHOR_RANKS:
        if rank > len(ordered_values):
            continue
        got = ordered_values[rank - 1]
        want = published_value(rank)
        anchors[rank] = got
        if abs(got - want) > CURVE_ANCHOR_TOLERANCE * want:
            ok = False
            notes.append(
                f"anchor rank {rank}: value {got:,.0f} outside"
                f" {CURVE_ANCHOR_TOLERANCE:.0%} of curve {want:,.0f}"
            )
    return anchors, ok, notes


def evaluate(
    values: dict[str, float],
    trades: list[Trade],
    reference: dict[str, float] | None = None,
) -> EvaluationReport:
    """Score one model's values against a block of held-out trades.

    `reference` is an optional dated benchmark (synthetic truth, FantasyCalc
    pull, next-season ADP) for rank recovery; package error alone must never
    select a model, so `valid` comes from the curve gate.
    """
    gaps: list[float] = []
    pcts: list[float] = []
    skipped = 0
    for t in trades:
        if any(p not in values for p in (*t.side_a, *t.side_b)):
            skipped += 1
            continue
        sum_a = sum(values[p] for p in t.side_a)
        sum_b = sum(values[p] for p in t.side_b)
        gap = abs(sum_a - sum_b)
        gaps.append(gap)
        mean_package = (sum_a + sum_b) / 2.0
        pcts.append(gap / mean_package if mean_package > 0 else 0.0)

    ordered = sorted(values.values(), reverse=True)
    anchors, curve_valid, notes = _curve_check(ordered)
    median = ordered[len(ordered) // 2] if ordered else 0.0
    spread = (ordered[0] / median) if median > 0 else 0.0

    return EvaluationReport(
        n_trades=len(gaps),
        skipped_trades=skipped,
        package_mae=float(np.mean(gaps)) if gaps else 0.0,
        package_pct_error=float(np.mean(pcts)) if pcts else 0.0,
        curve_anchors=anchors,
        curve_valid=curve_valid,
        value_spread=spread,
        reference_spearman=(
            spearman(values, reference) if reference is not None else None
        ),
        valid=curve_valid,
        notes=notes,
    )


def flat_control_report(
    player_ids: list[str], trades: list[Trade], level: float = 1445.0
) -> EvaluationReport:
    """The negative control: every player worth the same constant.

    This model scores a perfect package error on balanced trades. Its report
    must come back invalid — if it ever doesn't, the evaluation itself is
    broken and no model comparison downstream of it can be trusted.
    """
    return evaluate(dict.fromkeys(player_ids, level), trades)


def format_report(name: str, report: EvaluationReport) -> str:
    lines = [
        f"  {name}:",
        f"    trades scored {report.n_trades} (skipped {report.skipped_trades})",
        f"    package MAE {report.package_mae:,.0f}"
        f" · pct error {report.package_pct_error:.1%}",
        f"    value spread (top/median) {report.value_spread:,.1f}",
        f"    curve anchors "
        + " ".join(
            f"r{rank}={value:,.0f}" for rank, value in sorted(report.curve_anchors.items())
        ),
        f"    curve_valid {report.curve_valid} -> valid {report.valid}",
    ]
    if report.reference_spearman is not None:
        lines.append(f"    reference Spearman {report.reference_spearman:.3f}")
    lines.extend(f"    ! {n}" for n in report.notes)
    return "\n".join(lines)


# ------------------------------------------------------- synthetic market --


@dataclass(frozen=True)
class SyntheticMarket:
    """A deterministic market with known true values, for recovery gates."""

    players: list[str]  # in true-rank order, best first
    true_values: dict[str, float]
    adp: list[AverageDraftPosition]
    trades: list[Trade]
    start: datetime
    end: datetime


def synthetic_market(
    n_players: int = 60,
    seed: int = 11,
    n_trades: int = 400,
    n_leagues: int = 8,
    start: datetime = datetime(2025, 9, 1),
    days: int = 60,
) -> SyntheticMarket:
    """Players on the published curve, traded in roughly-fair packages.

    Sides are matched on true value within a small tolerance plus noise, so
    the trade stream carries real (but imperfect) information about the true
    order — exactly what an estimator must recover.
    """
    rng = np.random.default_rng(seed)
    players = [f"p{i:02d}" for i in range(n_players)]
    true_values = {p: published_value(rank + 1) for rank, p in enumerate(players)}
    positions = ["QB", "RB", "WR", "TE"]

    adp = [
        AverageDraftPosition(
            player_id=p,
            player_name=p.upper(),
            position=positions[i % len(positions)],
            adp=float(max(1.0, i + 1 + rng.normal(0.0, 2.0))),
        )
        for i, p in enumerate(players)
    ]

    trades: list[Trade] = []
    ids = np.array(players)
    attempts = 0
    while len(trades) < n_trades and attempts < n_trades * 50:
        attempts += 1
        size_a, size_b = [(1, 1), (1, 2), (2, 1), (2, 2)][
            int(rng.integers(0, 4))
        ]
        chosen = rng.choice(len(ids), size=size_a + size_b, replace=False)
        side_a = [str(ids[i]) for i in chosen[:size_a]]
        side_b = [str(ids[i]) for i in chosen[size_a:]]
        sum_a = sum(true_values[p] for p in side_a)
        sum_b = sum(true_values[p] for p in side_b)
        mean_package = (sum_a + sum_b) / 2.0
        # accept only roughly-fair exchanges, with market noise
        if abs(sum_a - sum_b) > 0.15 * mean_package + abs(
            rng.normal(0.0, 0.05 * mean_package)
        ):
            continue
        i = len(trades)
        ts = start + timedelta(
            days=float(i) * days / n_trades, hours=float(rng.integers(0, 24))
        )
        trades.append(
            Trade(
                trade_id=f"syn{i:04d}",
                ts=ts,
                side_a=side_a,
                side_b=side_b,
                created_ms=int((ts - datetime(1970, 1, 1)).total_seconds() * 1000),
                league_id=f"synlg{i % n_leagues}",
            )
        )

    return SyntheticMarket(
        players=players,
        true_values=true_values,
        adp=adp,
        trades=trades,
        start=start,
        end=start + timedelta(days=days),
    )
