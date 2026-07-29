"""Orchestration: turn DB rows into model events and drive the Valuator.

No DB access here — main.py wires data in and snapshots out.
"""

from collections.abc import Callable, Iterator
from dataclasses import dataclass
from datetime import date, datetime, timedelta
from itertools import groupby

import pandas as pd

from .config import SeasonDates, week_ts
from .models import AverageDraftPosition, Trade, WeeklyScore
from .valuation import Valuator

ADP_COLUMNS = ["player_id", "player_name", "position", "adp"]


def adp_frame(adp: list[AverageDraftPosition]) -> pd.DataFrame:
    if not adp:
        return pd.DataFrame(columns=ADP_COLUMNS)
    return pd.DataFrame(
        [(a.player_id, a.player_name, a.position, a.adp) for a in adp],
        columns=ADP_COLUMNS,
    )


def build_events(
    trades: list[Trade], scores: list[WeeklyScore], season: SeasonDates
) -> list[dict]:
    """Valuator.advance() event dicts, sorted by timestamp."""
    events: list[dict] = [
        {"ts": t.ts, "kind": "trade", "side_a": t.side_a, "side_b": t.side_b}
        for t in trades
    ]
    by_week: dict[int, list[WeeklyScore]] = {}
    for s in scores:
        by_week.setdefault(s.week, []).append(s)
    for week, wk_scores in by_week.items():
        events.append(
            {
                "ts": week_ts(season, week),
                "kind": "week",
                "scores": pd.DataFrame(
                    [(s.player_id, s.position, s.points) for s in wk_scores],
                    columns=["player_id", "position", "points"],
                ),
            }
        )
    events.sort(key=lambda e: e["ts"])
    return events


def filter_stale(
    events: list[dict], last_event_ts: datetime | None
) -> tuple[list[dict], int]:
    """Drop events at or before the model clock (out-of-order arrivals)."""
    if last_event_ts is None:
        return events, 0
    fresh = [e for e in events if e["ts"] > last_event_ts]
    return fresh, len(events) - len(fresh)


def run_backtest(
    valuator: Valuator,
    events: list[dict],
    on_snapshot: Callable[[date, pd.DataFrame], None],
) -> None:
    """Replay a season as if live: advance one event-day at a time and emit a
    valuation snapshot after each day that had events. Aging between days
    changes only uncertainty (sd), not value, so event days are the complete
    set of days the value series can move.

    Kept for interactive/experimental use only. The scheduled job uses
    replay() below, which also emits on quiet days so uncertainty drift shows
    up in the published series."""
    events = sorted(events, key=lambda e: e["ts"])
    for day, day_events in groupby(events, key=lambda e: e["ts"].date()):
        valuator.advance(list(day_events))
        on_snapshot(day, valuator.rankings())


# ------------------------------------------------------------ fixed replay --


@dataclass
class ReplayStats:
    snapshots: int = 0
    events_applied: int = 0
    dropped_before_start: int = 0
    dropped_at_or_after_end: int = 0


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


def take_events_before(
    events: list[dict], cursor: int, boundary: datetime
) -> tuple[list[dict], int]:
    """Events from `cursor` onward whose ts is strictly before `boundary`.

    `events` must be sorted by ts. Returns the slice plus the new cursor, so
    each event is consumed exactly once across the whole replay.
    """
    end = cursor
    while end < len(events) and events[end]["ts"] < boundary:
        end += 1
    return events[cursor:end], end


def replay(
    valuator: Valuator,
    events: list[dict],
    start: date,
    end: date,
    step: timedelta,
    on_snapshot: Callable[[date, pd.DataFrame], None],
) -> ReplayStats:
    """Replay [start, end) in fixed UTC steps, snapshotting at every boundary.

    A snapshot dated D is the model state at the start of UTC day D, after all
    events strictly before that boundary. Quiet batches still emit: the
    valuator is aged to the boundary first, so uncertainty drifts through the
    off-season the same way it does mid-season.
    """
    validate_step(start, end, step)
    window_start = datetime.combine(start, datetime.min.time())
    window_end = datetime.combine(end, datetime.min.time())

    ordered = sorted(events, key=lambda e: e["ts"])
    in_window = [e for e in ordered if window_start <= e["ts"] < window_end]
    stats = ReplayStats(
        dropped_before_start=sum(1 for e in ordered if e["ts"] < window_start),
        dropped_at_or_after_end=sum(1 for e in ordered if e["ts"] >= window_end),
    )

    cursor = 0
    for _, batch_end in utc_batches(start, end, step):
        batch, cursor = take_events_before(in_window, cursor, batch_end)
        valuator.advance(batch)
        valuator.age_to(batch_end)  # quiet batches still drift
        stats.events_applied += len(batch)
        stats.snapshots += 1
        on_snapshot(batch_end.date(), valuator.rankings())

    return stats
