"""Orchestration: turn DB rows into model events and drive the Valuator.

No DB access here — main.py wires data in and snapshots out.
"""

from collections.abc import Callable
from datetime import date, datetime
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
    set of days the value series can move."""
    events = sorted(events, key=lambda e: e["ts"])
    for day, day_events in groupby(events, key=lambda e: e["ts"].date()):
        valuator.advance(list(day_events))
        on_snapshot(day, valuator.rankings())
