"""Periodic progress reporting for a long replay.

A full-season rebuild emits several hundred snapshots over many minutes with
no output between the input summary and the final table, which makes a slow
run indistinguishable from a hung one in `journalctl`. This prints one line
per interval showing how far the replay has stepped and what is left.
"""

from __future__ import annotations

import time
from collections.abc import Callable
from datetime import date, timedelta

# Long enough that an eleven-month rebuild produces a readable handful of
# journal lines rather than hundreds, short enough to tell a stalled run from
# a slow one well before the 2h unit timeout.
DEFAULT_INTERVAL_S = 30.0


def total_batches(start: date, end: date, step: timedelta) -> int:
    """How many snapshots a replay over [start, end) will emit.

    Mirrors utc_batches: the range is validated to divide evenly by step, so
    this is exact rather than an estimate.
    """
    return (end - start) // step


def format_duration(seconds: float) -> str:
    """Compact h/m/s, so a progress line stays scannable in the journal."""
    seconds = max(0, int(seconds))
    hours, rem = divmod(seconds, 3600)
    minutes, secs = divmod(rem, 60)
    if hours:
        return f"{hours}h{minutes:02d}m"
    if minutes:
        return f"{minutes}m{secs:02d}s"
    return f"{secs}s"


def _stdout(line: str) -> None:
    # flush: under systemd stdout is a pipe, so Python block-buffers it and
    # progress would arrive in one burst at exit — exactly when it is useless.
    print(line, flush=True)


class ProgressReporter:
    """Emits a progress line every `interval` seconds, plus a final one.

    Throttled on the wall clock rather than on a snapshot count: a three-day
    test run stays silent while a full rebuild reports at a steady cadence,
    without either needing to know how fast a step happens to be.
    """

    def __init__(
        self,
        total: int,
        interval: float = DEFAULT_INTERVAL_S,
        log: Callable[[str], None] | None = None,
        clock: Callable[[], float] = time.monotonic,
        extra: Callable[[], str] | None = None,
    ) -> None:
        """extra: called only when a line is emitted, for a caller-supplied
        suffix. Because it runs once per line and not once per snapshot, a
        caller can use it to difference its own counters over the interval."""
        self.total = total
        self.interval = interval
        self.log = log or _stdout
        self.clock = clock
        self.extra = extra
        self.done = 0
        self.started = clock()
        self._last_report_at = self.started
        self._last_report_done = 0

    def tick(self, day: date) -> None:
        """Record one completed snapshot, reporting if the interval has passed.

        The last snapshot always reports, so a run that finishes inside one
        interval still logs where it ended up.
        """
        self.done += 1
        now = self.clock()
        is_last = self.done >= self.total
        if not is_last and now - self._last_report_at < self.interval:
            return
        self.log(self._line(day, now))
        self._last_report_at = now
        self._last_report_done = self.done

    def _line(self, day: date, now: float) -> str:
        elapsed = now - self.started
        remaining = max(0, self.total - self.done)
        pct = 100.0 * self.done / self.total if self.total else 100.0

        parts = [
            f"  {self.done}/{self.total} snapshots",
            str(day),
            f"{pct:.0f}%",
            f"{format_duration(elapsed)} elapsed",
        ]
        if remaining:
            parts.append(self._eta(now, remaining))
        if self.extra is not None:
            note = self.extra()
            if note:
                parts.append(note)
        return " · ".join(parts)

    def _eta(self, now: float, remaining: int) -> str:
        """Estimate from the rate since the previous line, not the run average.

        Per-snapshot cost grows as beliefs accumulate — every snapshot ranks
        and writes every player seen so far — so a whole-run average keeps
        predicting a finish that then slips.
        """
        span = now - self._last_report_at
        recent = self.done - self._last_report_done
        if span <= 0 or recent <= 0:
            return "~? left"
        return f"~{format_duration(remaining * span / recent)} left"
