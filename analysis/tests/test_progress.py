from datetime import date, timedelta

from src.progress import ProgressReporter, format_duration, total_batches


class FakeClock:
    """Hand-advanced monotonic clock, so cadence is asserted not slept for."""

    def __init__(self) -> None:
        self.t = 0.0

    def __call__(self) -> float:
        return self.t

    def advance(self, seconds: float) -> None:
        self.t += seconds


def _reporter(total, interval=30.0):
    clock = FakeClock()
    lines: list[str] = []
    return ProgressReporter(total, interval, lines.append, clock), lines, clock


def test_total_batches_matches_the_snapshots_a_replay_emits():
    # the production invocation: draft date through today, daily
    assert total_batches(date(2025, 8, 25), date(2026, 7, 30), timedelta(days=1)) == 339
    assert total_batches(date(2025, 8, 25), date(2025, 8, 28), timedelta(days=1)) == 3
    assert total_batches(date(2025, 8, 25), date(2025, 9, 8), timedelta(days=7)) == 2


def test_durations_stay_short_enough_to_scan():
    assert format_duration(0) == "0s"
    assert format_duration(45) == "45s"
    assert format_duration(125) == "2m05s"
    assert format_duration(3725) == "1h02m"
    assert format_duration(-5) == "0s"  # clock skew must not print nonsense


def test_reporting_is_throttled_by_the_clock_not_the_snapshot_count():
    reporter, lines, clock = _reporter(total=100, interval=30.0)

    for _ in range(10):
        clock.advance(1.0)
        reporter.tick(date(2025, 9, 1))
    assert lines == []  # 10 snapshots in 10s: nothing worth logging yet

    clock.advance(25.0)
    reporter.tick(date(2025, 9, 2))
    assert len(lines) == 1
    assert "11/100 snapshots" in lines[0]
    assert "2025-09-02" in lines[0]
    assert "11%" in lines[0]
    assert "35s elapsed" in lines[0]


def test_the_final_snapshot_always_reports():
    """A run shorter than one interval still has to say where it ended up."""
    reporter, lines, clock = _reporter(total=3, interval=30.0)

    for day in (date(2025, 8, 26), date(2025, 8, 27), date(2025, 8, 28)):
        clock.advance(1.0)
        reporter.tick(day)

    assert len(lines) == 1
    assert "3/3 snapshots" in lines[0]
    assert "100%" in lines[0]
    assert "left" not in lines[0]  # nothing remains to estimate


def test_the_estimate_tracks_the_recent_rate_not_the_run_average():
    """Per-snapshot cost grows with the belief set, so a run average would
    keep predicting a finish that then slips."""
    reporter, lines, clock = _reporter(total=200, interval=10.0)

    # 80 snapshots at 4/s (0.25 is exact in binary — no clock drift to chase)
    for _ in range(80):
        clock.advance(0.25)
        reporter.tick(date(2025, 9, 1))
    # ...then it slows to 1/s
    for _ in range(10):
        clock.advance(1.0)
        reporter.tick(date(2025, 9, 2))

    last = lines[-1]
    assert "90/200 snapshots" in last
    # 110 left at the recent 1/s. The whole-run average is 3/s, which would
    # have promised ~36s — the optimistic estimate this avoids.
    assert "~1m50s left" in last
