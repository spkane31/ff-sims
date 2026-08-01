"""Orchestration: turn DB rows into market snapshots.

No DB access here — main.py wires data in and snapshots out. Each replay
boundary refits every market score jointly from the trades seen so far and
advances the performance tracker through completed score weeks.
"""

import math
from collections.abc import Callable, Iterator
from dataclasses import dataclass
from datetime import date, datetime, timedelta

import pandas as pd

from . import market_value, performance
from .config import SeasonDates, week_ts
from .models import AverageDraftPosition, PlayerProfile, Trade, WeeklyScore

# ------------------------------------------------------------ fixed replay --


@dataclass
class ReplayStats:
    snapshots: int = 0
    events_applied: int = 0
    dropped_before_start: int = 0
    dropped_at_or_after_end: int = 0
    # market_value.fit_diagnostics of the final boundary's fit — the
    # fit-health block a run log prints (None until a replay completes)
    diagnostics: dict | None = None


def validate_step(start: date, end: date, step: timedelta) -> None:
    """--step must be a positive whole number of days that divides the range.

    Partial-day batching would put two snapshots on one calendar date (the
    output is keyed by date), so it is rejected rather than silently collapsed.
    """
    if end <= start:
        raise ValueError(f"end ({end}) must be after start ({start})")
    if step <= timedelta(0):
        raise ValueError(f"step must be positive, got {step}")
    if step % timedelta(days=1) != timedelta(0):
        raise ValueError(f"step must be a whole number of days, got {step}")
    span = end - start
    if span % step != timedelta(0):
        raise ValueError(
            f"step {step} does not evenly divide the range {start}..{end} ({span})"
        )


def utc_batches(
    start: date, end: date, step: timedelta
) -> Iterator[tuple[datetime, datetime]]:
    """UTC-aligned [batch_start, batch_end) pairs covering [start, end).

    The last pair ends exactly at `end`, so `end` always gets a snapshot.
    """
    validate_step(start, end, step)
    cursor = datetime.combine(start, datetime.min.time())
    stop = datetime.combine(end, datetime.min.time())
    while cursor < stop:
        yield cursor, cursor + step
        cursor += step


# ----------------------------------------------------------- market replay --


MARKET_FRAME_COLUMNS = [
    "player_id", "player", "pos", "pos_rank", "value", "market_score",
    "market_dispersion", "projected_par", "projection_uncertainty", "games",
    "trades",
]


def market_frame(
    fit: market_value.FitResult,
    tracker: performance.PerformanceTracker,
    players: dict[str, PlayerProfile],
    trade_counts: dict[str, int],
) -> pd.DataFrame:
    """One market-v2 snapshot as a rankings frame (index = market rank).

    Publishes the union of market-fitted players and performance-tracked
    ones: a scores-only streamer has no market evidence, so their score is 0
    and they rank at the bottom — honest, rather than invisible.
    """
    scores = dict(fit.scores)
    for pid in tracker.players:
        scores.setdefault(pid, 0.0)
    ordered = sorted(scores, key=lambda p: (-scores[p], p))

    prior_band = math.sqrt(performance.PRIOR_PAR_VAR)
    rows = []
    for rank, pid in enumerate(ordered, start=1):
        value = market_value.published_value(rank)
        profile = players.get(pid)
        perf = tracker.players.get(pid)
        rows.append(
            {
                "player_id": pid,
                "player": (profile.name if profile else "") or pid,
                "pos": (
                    (profile.position if profile else "")
                    or (perf.position if perf else "")
                    or "DEFAULT"
                ),
                "value": round(value),
                "market_score": round(scores[pid], 1),
                "market_dispersion": round(
                    fit.dispersion.get(
                        pid,
                        max(
                            market_value.MIN_DISPERSION,
                            market_value.DISPERSION_VALUE_FLOOR_FRAC * value,
                        ),
                    )
                ),
                "projected_par": round(
                    tracker.projected_par(pid) if perf else 0.0, 2
                ),
                "projection_uncertainty": round(
                    tracker.projection_uncertainty(pid) if perf else prior_band,
                    2,
                ),
                "games": round(perf.games, 1) if perf else 0.0,
                "trades": trade_counts.get(pid, 0),
            }
        )
    df = pd.DataFrame(rows, columns=MARKET_FRAME_COLUMNS[:3] + MARKET_FRAME_COLUMNS[4:])
    df["pos_rank"] = df.groupby("pos").cumcount() + 1
    df = df[MARKET_FRAME_COLUMNS]
    df.index += 1
    df.index.name = "rank"
    return df


def replay_market(
    trades: list[Trade],
    scores: list[WeeklyScore],
    adp: list[AverageDraftPosition],
    players: dict[str, PlayerProfile],
    season: SeasonDates,
    start: date,
    end: date,
    step: timedelta,
    repl_rank_by_pos: dict[str, int],
    on_snapshot: Callable[[date, pd.DataFrame], None] | None,
) -> ReplayStats:
    """Replay [start, end) in fixed UTC steps, snapshotting at every boundary.

    A snapshot dated D is the model state at the start of UTC day D, after
    all events strictly before that boundary. Each boundary refits every
    market score jointly from the trades seen so far (recency-weighted at the
    boundary, warm-started from the previous snapshot) and advances the
    performance tracker through any completed score weeks. There is no
    incremental belief state: the fit at a boundary is a pure function of the
    inputs before it.
    """
    validate_step(start, end, step)
    window_start = datetime.combine(start, datetime.min.time())
    window_end = datetime.combine(end, datetime.min.time())

    ordered_trades = sorted(trades, key=lambda t: (t.created_ms, t.trade_id))
    in_window = [
        t for t in ordered_trades if window_start <= t.ts < window_end
    ]

    by_week: dict[int, list[WeeklyScore]] = {}
    for s in scores:
        by_week.setdefault(s.week, []).append(s)
    weeks = sorted(
        ((week_ts(season, wk), rows) for wk, rows in by_week.items()),
        key=lambda pair: pair[0],
    )
    weeks_in_window = [
        (ts, rows) for ts, rows in weeks if window_start <= ts < window_end
    ]

    stats = ReplayStats(
        dropped_before_start=(
            sum(1 for t in ordered_trades if t.ts < window_start)
            + sum(1 for ts, _ in weeks if ts < window_start)
        ),
        dropped_at_or_after_end=(
            sum(1 for t in ordered_trades if t.ts >= window_end)
            + sum(1 for ts, _ in weeks if ts >= window_end)
        ),
    )

    prior = market_value.adp_prior(adp)
    preseason = {
        a.player_id: (
            a.position,
            performance.preseason_par_from_market(
                prior[a.player_id], market_value.PUBLISHED_TOP
            ),
        )
        for a in adp
    }
    tracker = performance.PerformanceTracker(
        repl_rank_by_pos, preseason_par=preseason
    )

    trade_cursor = 0
    week_cursor = 0
    warm: dict[str, float] | None = None
    fit: market_value.FitResult | None = None
    for _, batch_end in utc_batches(start, end, step):
        while (
            week_cursor < len(weeks_in_window)
            and weeks_in_window[week_cursor][0] < batch_end
        ):
            tracker.apply_week(weeks_in_window[week_cursor][1])
            week_cursor += 1
            stats.events_applied += 1
        while (
            trade_cursor < len(in_window)
            and in_window[trade_cursor].ts < batch_end
        ):
            trade_cursor += 1
            stats.events_applied += 1

        window_trades = in_window[:trade_cursor]
        fit = market_value.fit_snapshot(
            window_trades, asof=batch_end, adp_prior=prior, warm_start=warm
        )
        warm = fit.scores
        stats.snapshots += 1
        if on_snapshot is not None:
            counts: dict[str, int] = {}
            for t in window_trades:
                for p in (*t.side_a, *t.side_b):
                    counts[p] = counts.get(p, 0) + 1
            on_snapshot(
                batch_end.date(), market_frame(fit, tracker, players, counts)
            )

    if fit is not None:
        stats.diagnostics = market_value.fit_diagnostics(
            fit, in_window, prior, asof=window_end
        )
    return stats
