"""Market-value model v2: a robust anchored snapshot fit over recent trades.

At each snapshot, ALL player market scores are fitted jointly from a rolling
trade window instead of mutating beliefs one trade at a time. The design
answers the failure modes documented in PLAYER_VALUATION_MODEL_REVIEW.md:

* Every trade is a soft equality between two package sums; a Huber loss keeps
  dumps and data errors from acting as exact constraints, and a second pass
  removes |z| > 3 residuals outright.
* Every drafted player keeps an ADP prior on every fit, and the total trade
  mass is normalized, so raw trade volume can never erase the prior — that is
  what removes the flat-value optimum from the objective.
* Trade weight decays with age (2^(-age/30)), so the recent market wins.
  Nothing shrinks with sample count: confidence comes from dispersion of
  recent implied values, not from a variance that only ever contracts.
* Weight is capped per league and per repeated identical package, so one
  hyperactive league cannot manufacture consensus.
* The published value is NOT the raw fitted score. Scores only order the
  players; presentation goes through one explicit calibration:

      market_value(rank) = 300 + 9700 * exp(-0.04 * (rank - 1))

  This v1 presentation curve is deliberately not fitted from trade residuals
  — trade fairness has a flat-value global optimum, so residuals cannot
  identify a scale or shape.
"""

from __future__ import annotations

import math
from dataclasses import dataclass
from datetime import datetime

import numpy as np
from scipy import sparse
from scipy.sparse.linalg import spsolve

from .models import AverageDraftPosition, Trade

PUBLISHED_TOP = 10_000.0
PUBLISHED_FLOOR = 300.0
PUBLISHED_LAM = 0.04

# --- fit knobs ---------------------------------------------------------------
HALF_LIFE_DAYS = 30.0  # trade weight halves every 30 days of age
PRIOR_WEIGHT = 1.0  # quadratic pull of each drafted player toward ADP
RIDGE_WEIGHT = 1e-6  # numeric floor for players with no prior at all
TRADE_MASS_PER_PLAYER = 4.0  # total trade weight is capped at this per player
LEAGUE_CAP_MULTIPLE = 3.0  # no league carries more than 3x the median league
DUPLICATE_CAP = 3.0  # identical package in one league counts at most 3x
OUTLIER_Z = 3.0  # standardized residual gate for the second pass
IRLS_ITERATIONS = 60
IRLS_TOL = 1e-10  # max relative score change that counts as converged

# --- dispersion knobs --------------------------------------------------------
MIN_DISPERSION_LEAGUES = 3  # fewer distinct leagues than this is no evidence
DEFAULT_DISPERSION_FRAC = 0.30  # fallback spread as a fraction of the score
MIN_DISPERSION = 150.0
# Reported uncertainty never drops below this fraction of the published
# value: market spread below that is beyond this data's resolution, and the
# floor keeps borderline inlier-set jitter from ever *tightening* a top
# player's band when an outlier is added and excluded.
DISPERSION_VALUE_FLOOR_FRAC = 0.05


def published_value(rank: float) -> float:
    """Market rank -> published value on the stable exponential 300..10,000
    scale. rank 1 -> 10,000; rank 5 -> 8,566; rank 20 -> 4,836; rank 50 ->
    1,666; rank 100 -> 485."""
    return PUBLISHED_FLOOR + (PUBLISHED_TOP - PUBLISHED_FLOOR) * math.exp(
        -PUBLISHED_LAM * (rank - 1.0)
    )


def adp_prior(adp: list[AverageDraftPosition]) -> dict[str, float]:
    """Every drafted player's prior score: the published curve read at ADP."""
    return {a.player_id: published_value(a.adp) for a in adp}


@dataclass(frozen=True)
class FitResult:
    scores: dict[str, float]  # fitted market score (the fit's own scale)
    ranks: dict[str, int]  # 1-based market rank by score
    values: dict[str, float]  # published_value(rank) — what consumers see
    dispersion: dict[str, float]  # robust spread of recent implied values
    n_trades_used: int  # inlier trades in the final fit
    outlier_trade_ids: tuple[str, ...]  # removed by the |z| > 3 second pass


# ------------------------------------------------------------- internals --


def _canonical_package(t: Trade) -> tuple:
    """Duplicate key: same league, same two packages, either orientation."""
    sides = sorted((tuple(sorted(t.side_a)), tuple(sorted(t.side_b))))
    return (t.league_id, sides[0], sides[1])


def _trade_weights(trades: list[Trade], asof: datetime) -> np.ndarray:
    """Recency weight, then the three caps: duplicate, league, global mass."""
    w = np.array(
        [
            2.0 ** (-max(0.0, (asof - t.ts).total_seconds() / 86400.0)
                    / HALF_LIFE_DAYS)
            for t in trades
        ]
    )

    # identical package repeated within one league: cap the group's combined
    # weight at DUPLICATE_CAP times its strongest single row
    groups: dict[tuple, list[int]] = {}
    for i, t in enumerate(trades):
        groups.setdefault(_canonical_package(t), []).append(i)
    for idx in groups.values():
        total = float(w[idx].sum())
        cap = DUPLICATE_CAP * float(w[idx].max())
        if total > cap:
            w[idx] *= cap / total

    # league cap: no league carries more than LEAGUE_CAP_MULTIPLE times the
    # median league's total weight (unknown-league trades form one bucket)
    league_total: dict[str, float] = {}
    for i, t in enumerate(trades):
        league_total[t.league_id] = league_total.get(t.league_id, 0.0) + float(w[i])
    if len(league_total) >= 2:
        cap = LEAGUE_CAP_MULTIPLE * float(np.median(list(league_total.values())))
        scale = {
            lg: (cap / total if total > cap else 1.0)
            for lg, total in league_total.items()
        }
        w *= np.array([scale[t.league_id] for t in trades])

    return w


def _normalize_mass(w: np.ndarray, n_players: int) -> np.ndarray:
    """Cap total trade weight so volume cannot erase the per-player priors."""
    total = float(w.sum())
    budget = TRADE_MASS_PER_PLAYER * n_players
    if total > budget > 0:
        w = w * (budget / total)
    return w


def _design_matrix(
    trades: list[Trade], index: dict[str, int]
) -> sparse.csr_matrix:
    """One row per trade: +1 for side A players, -1 for side B."""
    rows, cols, data = [], [], []
    for j, t in enumerate(trades):
        for p in t.side_a:
            rows.append(j)
            cols.append(index[p])
            data.append(1.0)
        for p in t.side_b:
            rows.append(j)
            cols.append(index[p])
            data.append(-1.0)
    return sparse.csr_matrix(
        (data, (rows, cols)), shape=(len(trades), len(index))
    )


def _robust_sigma(residuals: np.ndarray) -> float:
    if residuals.size == 0:
        return 0.0
    return 1.4826 * float(np.median(np.abs(residuals)))


def _irls(
    a: sparse.csr_matrix,
    w: np.ndarray,
    prior_vec: np.ndarray,
    prior_w: np.ndarray,
    x0: np.ndarray,
) -> np.ndarray:
    """Sparse Huber IRLS for  min  Σ w_j·Huber(a_j·x) + Σ p_i·(x_i - prior_i)²,
    with x >= 0 enforced by projection each step."""
    x = np.maximum(0.0, x0.copy())
    p_diag = sparse.diags(prior_w)
    rhs_prior = prior_w * prior_vec
    for _ in range(IRLS_ITERATIONS):
        r = a @ x
        sigma = _robust_sigma(r)
        if sigma > 0:
            delta = 1.345 * sigma
            hub = np.minimum(1.0, delta / np.maximum(np.abs(r), 1e-12))
        else:
            hub = np.ones_like(r)
        wh = w * hub
        lhs = (a.T @ sparse.diags(wh) @ a + p_diag).tocsc()
        new_x = spsolve(lhs, rhs_prior)
        new_x = np.maximum(0.0, np.asarray(new_x).ravel())
        change = float(np.max(np.abs(new_x - x))) / max(1.0, float(np.max(new_x)))
        x = new_x
        if change < IRLS_TOL:
            break
    return x


def _implied_dispersion(
    trades: list[Trade],
    scores: dict[str, float],
    values: dict[str, float],
) -> dict[str, float]:
    """Robust spread of each player's recent implied trade values.

    An implied value is what one trade says a player is worth given everyone
    else's fitted score. Grouped by league first: repeating one trade (or one
    league's conviction) many times is still ONE market opinion, so dispersion
    is measured across league medians. Fewer than MIN_DISPERSION_LEAGUES
    distinct leagues is not enough evidence to claim tightness, and falls back
    to a wide default — this is what keeps duplicated or correlated trades
    from manufacturing confidence.
    """
    by_player: dict[str, dict[str, list[float]]] = {}
    for t in trades:
        sum_a = sum(scores[p] for p in t.side_a)
        sum_b = sum(scores[p] for p in t.side_b)
        for p in t.side_a:
            implied = max(0.0, sum_b - (sum_a - scores[p]))
            by_player.setdefault(p, {}).setdefault(t.league_id, []).append(implied)
        for p in t.side_b:
            implied = max(0.0, sum_a - (sum_b - scores[p]))
            by_player.setdefault(p, {}).setdefault(t.league_id, []).append(implied)

    out: dict[str, float] = {}
    for pid, score in scores.items():
        floor = max(
            MIN_DISPERSION, DISPERSION_VALUE_FLOOR_FRAC * values.get(pid, 0.0)
        )
        leagues = by_player.get(pid)
        if not leagues or len(leagues) < MIN_DISPERSION_LEAGUES:
            out[pid] = max(floor, DEFAULT_DISPERSION_FRAC * score)
            continue
        medians = np.array([np.median(v) for v in leagues.values()])
        spread = 1.4826 * float(np.median(np.abs(medians - np.median(medians))))
        out[pid] = max(spread, floor)
    return out


# ------------------------------------------------------------ public fit --


def fit_snapshot(
    trades: list[Trade],
    asof: datetime,
    adp_prior: dict[str, float],
    warm_start: dict[str, float] | None = None,
) -> FitResult:
    """Fit all market scores jointly from the trades at/before `asof`.

    Deterministic and invariant to input row order: trades are canonically
    sorted before any weight or matrix is built. `warm_start` (the previous
    snapshot's scores) only speeds convergence; it must not change the answer.
    """
    window = sorted(
        (t for t in trades if t.ts <= asof),
        key=lambda t: (t.created_ms, t.trade_id),
    )

    players = sorted(
        set(adp_prior)
        | {p for t in window for p in (*t.side_a, *t.side_b)}
    )
    index = {p: i for i, p in enumerate(players)}
    n = len(players)

    prior_vec = np.array([adp_prior.get(p, 0.0) for p in players])
    prior_w = np.array(
        [PRIOR_WEIGHT if p in adp_prior else RIDGE_WEIGHT for p in players]
    )
    x0 = np.array(
        [
            (warm_start or {}).get(p, adp_prior.get(p, 0.0))
            for p in players
        ]
    )

    if not window:
        scores = {p: float(max(0.0, x)) for p, x in zip(players, prior_vec)}
        return _finish(scores, [], 0, ())

    a = _design_matrix(window, index)
    w = _normalize_mass(_trade_weights(window, asof), n)

    # pass 1: fit, then standardized residuals
    x = _irls(a, w, prior_vec, prior_w, x0)
    residuals = np.asarray(a @ x).ravel()
    sigma = _robust_sigma(residuals)
    if sigma > 0:
        inlier = np.abs(residuals) / sigma <= OUTLIER_Z
    else:
        inlier = np.ones(len(window), dtype=bool)
    outlier_ids = tuple(
        t.trade_id for t, keep in zip(window, inlier) if not keep
    )

    # pass 2: refit without the outliers
    if not np.all(inlier):
        kept = [t for t, keep in zip(window, inlier) if keep]
        a = _design_matrix(kept, index)
        w = _normalize_mass(_trade_weights(kept, asof), n)
        x = _irls(a, w, prior_vec, prior_w, x)
        window = kept

    scores = {p: float(x[index[p]]) for p in players}
    return _finish(scores, window, len(window), outlier_ids)


def _spearman(a: np.ndarray, b: np.ndarray) -> float | None:
    """Spearman rank correlation (no tie correction; ties are rare here)."""
    if len(a) < 3:
        return None
    ra = np.argsort(np.argsort(a))
    rb = np.argsort(np.argsort(b))
    if ra.std() == 0 or rb.std() == 0:
        return None
    return float(np.corrcoef(ra, rb)[0, 1])


def fit_diagnostics(
    fit: FitResult,
    trades: list[Trade],
    adp_prior: dict[str, float],
    asof: datetime,
) -> dict:
    """Fit-health numbers for the end of a run log.

    These answer the questions a finished replay actually raises: how much
    trade evidence backs the fit, how hard the outlier pass had to work, how
    spread the inlier residuals are, whether one league dominates the weight,
    and how far the market moved players off their ADP prior. All derived
    from the final state — no extra bookkeeping during the replay.
    """
    window = [t for t in trades if t.ts <= asof]
    inliers = [t for t in window if t.trade_id not in set(fit.outlier_trade_ids)]

    gaps: list[float] = []
    pcts: list[float] = []
    for t in inliers:
        sum_a = sum(fit.scores.get(p, 0.0) for p in t.side_a)
        sum_b = sum(fit.scores.get(p, 0.0) for p in t.side_b)
        gap = abs(sum_a - sum_b)
        gaps.append(gap)
        mean_package = (sum_a + sum_b) / 2.0
        if mean_package > 0:
            pcts.append(gap / mean_package)

    league_share = None
    leagues: set[str] = {t.league_id for t in inliers}
    if inliers:
        w = _normalize_mass(_trade_weights(inliers, asof), len(fit.scores))
        by_league: dict[str, float] = {}
        for weight, t in zip(w, inliers):
            by_league[t.league_id] = by_league.get(t.league_id, 0.0) + float(weight)
        total = sum(by_league.values())
        if total > 0:
            league_share = max(by_league.values()) / total

    prior_players = sorted(set(adp_prior) & set(fit.scores))
    prior_spearman = _spearman(
        np.array([adp_prior[p] for p in prior_players]),
        np.array([fit.scores[p] for p in prior_players]),
    )
    # rank movement off the prior: positive delta = the market ranks the
    # player better than ADP did
    prior_rank = {
        p: i + 1
        for i, p in enumerate(
            sorted(prior_players, key=lambda p: (-adp_prior[p], p))
        )
    }
    deltas = sorted(
        ((p, prior_rank[p] - fit.ranks[p]) for p in prior_players),
        key=lambda pair: pair[1],
    )
    fallers = [(p, d) for p, d in deltas[:3] if d < 0]
    risers = [(p, d) for p, d in reversed(deltas[-3:]) if d > 0]

    dispersions = np.array(sorted(fit.dispersion.values()))
    at_floor = sum(
        1
        for p, d in fit.dispersion.items()
        if abs(
            d
            - max(MIN_DISPERSION, DISPERSION_VALUE_FLOOR_FRAC * fit.values[p])
        )
        < 1e-9
    )

    def pct_of(values: list[float], q: float) -> float | None:
        return float(np.percentile(values, q)) if values else None

    return {
        "players_fit": len(fit.scores),
        "players_with_prior": len(prior_players),
        "trade_only_players": len(fit.scores) - len(prior_players),
        "trades_used": fit.n_trades_used,
        "outliers_removed": len(fit.outlier_trade_ids),
        "outlier_share": (
            len(fit.outlier_trade_ids) / len(window) if window else 0.0
        ),
        "inlier_gap_mean": float(np.mean(gaps)) if gaps else None,
        "inlier_gap_p50": pct_of(gaps, 50),
        "inlier_gap_p90": pct_of(gaps, 90),
        "inlier_gap_p99": pct_of(gaps, 99),
        "inlier_gap_max": max(gaps) if gaps else None,
        "inlier_package_pct_error": float(np.mean(pcts)) if pcts else None,
        "leagues": len(leagues),
        "top_league_weight_share": league_share,
        "prior_spearman": prior_spearman,
        "risers": risers,
        "fallers": fallers,
        "dispersion_p50": (
            float(np.median(dispersions)) if len(dispersions) else None
        ),
        "dispersion_p90": pct_of(list(dispersions), 90),
        "dispersion_at_floor": at_floor,
    }


def _finish(
    scores: dict[str, float],
    inlier_trades: list[Trade],
    n_used: int,
    outlier_ids: tuple[str, ...],
) -> FitResult:
    # rank by score, ties broken by id so output is deterministic
    ordered = sorted(scores, key=lambda p: (-scores[p], p))
    ranks = {p: i + 1 for i, p in enumerate(ordered)}
    values = {p: published_value(ranks[p]) for p in ordered}
    return FitResult(
        scores=scores,
        ranks=ranks,
        values=values,
        dispersion=_implied_dispersion(inlier_trades, scores, values),
        n_trades_used=n_used,
        outlier_trade_ids=outlier_ids,
    )
