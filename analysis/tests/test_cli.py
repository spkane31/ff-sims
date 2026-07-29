from datetime import date, datetime, timedelta, timezone

import pytest

import main


def _parse(argv):
    return main.build_parser().parse_args(argv)


SCHEDULED = [
    "--segment", "ppr-sf-10", "--season", "2025",
    "--start", "2025-08-25", "--step", "24h",
]


def test_scheduled_arguments_select_only_ppr_sf_10_2025():
    args = _parse(SCHEDULED)
    assert args.segment == "ppr-sf-10"
    assert args.season == "2025"
    assert args.start == date(2025, 8, 25)
    assert args.step == timedelta(hours=24)
    assert args.end is None  # filled in by default_end()
    # the CLI default is unchanged: the segment must be passed explicitly
    assert _parse([]).segment != "ppr-sf-10"


def test_step_parsing():
    assert main.parse_step("24h") == timedelta(days=1)
    assert main.parse_step("1d") == timedelta(days=1)
    assert main.parse_step(" 48H ") == timedelta(days=2)
    for bad in ("24", "24m", "-1d", "abc", ""):
        with pytest.raises(Exception):
            main.parse_step(bad)


def test_day_parsing_requires_iso_dates():
    assert main.parse_day("2025-08-25") == date(2025, 8, 25)
    for bad in ("08/25/2025", "2025-08-25T00:00:00", "nope"):
        with pytest.raises(Exception):
            main.parse_day(bad)


def test_default_end_is_the_current_utc_date(monkeypatch):
    """The newest date a run can publish honestly.

    `end` is exclusive and the last snapshot is dated `end`, so defaulting to
    tomorrow would publish a future-dated row from an input window that hasn't
    happened yet — the 00:00 UTC timer fires at the *start* of the day.
    """
    assert main.default_end() == main.utc_today()

    class FrozenDatetime(datetime):
        @classmethod
        def now(cls, tz=None):
            return datetime(2026, 7, 28, 0, 0, tzinfo=tz or timezone.utc)

    monkeypatch.setattr(main, "datetime", FrozenDatetime)
    assert main.default_end() == date(2026, 7, 28)
    # the snapshot the scheduled run ends on is dated today, not tomorrow
    assert main.default_end() != date(2026, 7, 29)


def test_database_run_requires_start_and_step(capsys):
    """--step is what selects the full-replay path, so a database invocation
    can never fall through to incremental processing."""
    for argv in (
        ["--segment", "ppr-sf-10", "--season", "2025"],
        ["--segment", "ppr-sf-10", "--start", "2025-08-25"],
        ["--segment", "ppr-sf-10", "--step", "24h"],
    ):
        with pytest.raises(SystemExit):
            main.main(argv)
    err = capsys.readouterr().err
    assert "--step is required" in err or "--start is required" in err


def test_no_incremental_or_backtest_mode_remains():
    for flag in ("--backtest", "--incremental", "--rebuild-daily", "--as-of"):
        with pytest.raises(SystemExit):
            _parse([flag])


def test_demo_mode_needs_no_database(monkeypatch):
    called = {}
    monkeypatch.setattr(main, "run_demo", lambda top: called.setdefault("top", top))
    monkeypatch.setattr(
        main, "run_replay",
        lambda *a, **k: pytest.fail("demo must not touch the database"),
    )
    main.main(["--demo"])
    assert called == {"top": 30}


def test_replay_receives_the_defaulted_end(monkeypatch):
    seen = {}
    monkeypatch.setattr(main, "default_end", lambda: date(2026, 7, 28))
    monkeypatch.setattr(
        main, "run_replay",
        lambda segment, season, start, end, step, top: seen.update(
            segment=segment, season=season, start=start, end=end, step=step
        ),
    )
    main.main(SCHEDULED)
    assert seen == {
        "segment": "ppr-sf-10", "season": "2025",
        "start": date(2025, 8, 25), "end": date(2026, 7, 28),
        "step": timedelta(hours=24),
    }
