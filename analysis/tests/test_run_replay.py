"""End-to-end shape of a database replay, driven through fake connections.

Covers the two-stage boundary from the plan: stage inputs first, then one
cloud write transaction that either lands whole or not at all.
"""

import re
from contextlib import contextmanager
from datetime import date, timedelta
from pathlib import Path

import pytest

import main
from src import db, staging
from src.config import SEASONS
from tests.fakes import FakeConnection
from tests.test_db_sources import _archive_responder, _cloud_responder

# start must be the season's draft date — the model clock's origin (see
# test_start_must_be_the_season_draft_date).
START, END = SEASONS["2025"].draft_date, date(2025, 8, 28)
ARGS = ("ppr-sf-10", "2025", START, END, timedelta(days=1), 5)


@pytest.fixture(autouse=True)
def staging_dir(monkeypatch, tmp_path):
    monkeypatch.setenv(staging.STAGING_DIR_ENV, str(tmp_path / "staging"))
    return tmp_path / "staging"


def _install(monkeypatch, cloud: FakeConnection, archive: FakeConnection | None = None):
    archive = archive or FakeConnection("archive", _archive_responder)
    sources = db.DataSources(archive=archive, cloud=cloud)

    @contextmanager
    def fake_open_sources():
        yield sources

    monkeypatch.setattr(db, "open_sources", fake_open_sources)
    return sources


def test_successful_replay_stages_then_writes_one_transaction(monkeypatch, staging_dir):
    cloud = FakeConnection("cloud", _cloud_responder())
    sources = _install(monkeypatch, cloud)

    main.run_replay(*ARGS)

    # staged bundle exists and is complete (manifest written last)
    runs = list(staging_dir.iterdir())
    assert len(runs) == 1
    manifest = staging.read_manifest(runs[0])
    assert manifest["args"]["segment"] == "ppr-sf-10"
    assert manifest["args"]["start"] == "2025-08-25"
    assert manifest["args"]["step_hours"] == 24.0
    staging.read_bundle(runs[0])  # checksums verify

    # inputs were staged before anything was deleted
    assert cloud.order_of("DELETE FROM player_valuations") > 0
    assert cloud.order_of("DELETE FROM player_valuations") < cloud.order_of(
        "INSERT INTO player_valuations"
    )
    # one snapshot per boundary: Aug 26, 27, 28 (start + step .. end)
    inserts = [c for c in cloud.calls if c[1] and "INSERT INTO player_valuations" in c[1]]
    assert len(inserts) == 3
    assert [rows[0][2] for _, _, rows in inserts] == [
        date(2025, 8, 26), date(2025, 8, 27), date(2025, 8, 28)
    ]

    assert cloud.ran("INSERT INTO valuation_state")
    assert cloud.ran("INSERT INTO valuation_runs")
    assert cloud.ran("pg_advisory_unlock")
    assert cloud.rollbacks == 1  # only the read-only snapshot's rollback
    assert sources.archive.closed is False  # closed by the real context manager


def test_a_failed_snapshot_write_rolls_back_the_delete(monkeypatch, staging_dir):
    cloud = FakeConnection(
        "cloud", _cloud_responder(), fail_on="INSERT INTO player_valuations"
    )
    _install(monkeypatch, cloud)

    with pytest.raises(RuntimeError, match="simulated failure"):
        main.run_replay(*ARGS)

    assert cloud.ran("DELETE FROM player_valuations")
    deleted_at = cloud.order_of("DELETE FROM player_valuations")
    after_delete = [op for op, _, _ in cloud.calls[deleted_at:]]
    # nothing committed the delete; it was rolled back instead
    assert "commit" not in after_delete[: after_delete.index("rollback")]
    assert cloud.ran("pg_advisory_unlock")  # released even on failure
    assert not cloud.ran("INSERT INTO valuation_runs")


def test_lock_contention_exits_immediately_without_touching_output(
    monkeypatch, staging_dir
):
    def responder(sql, params):
        if "pg_try_advisory_lock" in sql:
            return [(False,)]
        return _cloud_responder()(sql, params)

    cloud = FakeConnection("cloud", responder)
    _install(monkeypatch, cloud)

    with pytest.raises(SystemExit) as exc:
        main.run_replay(*ARGS)

    assert exc.value.code == main.EXIT_LOCKED
    assert not cloud.ran("DELETE FROM player_valuations")
    assert not cloud.ran("INSERT INTO player_valuations")
    assert not cloud.ran("sleeper_draft_picks")


def test_missing_identity_aborts_before_the_output_transaction(
    monkeypatch, staging_dir
):
    cloud = FakeConnection("cloud", _cloud_responder(players=[("p1", "QB One", "QB")]))
    _install(monkeypatch, cloud)

    with pytest.raises(db.MissingPlayerIdentities):
        main.run_replay(*ARGS)

    assert not cloud.ran("DELETE FROM player_valuations")
    assert not cloud.ran("INSERT INTO player_valuations")
    assert cloud.ran("pg_advisory_unlock")
    # the run directory is kept for inspection even though the run failed
    assert len(list(staging_dir.iterdir())) == 1


def test_start_must_be_the_season_draft_date(monkeypatch, staging_dir):
    """A later --start silently drops every event before it.

    --start is both the input window's lower bound and the model clock's
    origin, so the run would re-seed from ADP, skip the intervening trades and
    scores entirely, and still overwrite valuation_state with that gap-ridden
    model. load_inputs already windows its queries to [start, end), so
    replay()'s dropped_before_start counter sees nothing to report.
    """
    cloud = FakeConnection("cloud", _cloud_responder())
    _install(monkeypatch, cloud)

    late = date(2025, 11, 1)  # mid-season, well after the 2025-08-25 draft
    with pytest.raises(SystemExit) as exc:
        main.run_replay("ppr-sf-10", "2025", late, date(2025, 11, 4), timedelta(days=1), 5)

    assert "must be the 2025 draft date (2025-08-25)" in str(exc.value)
    # nothing was read, staged, or written
    assert not cloud.ran("DELETE FROM player_valuations")
    assert not cloud.ran("INSERT INTO valuation_state")
    assert not cloud.ran("pg_try_advisory_lock")
    assert not staging_dir.exists() or list(staging_dir.iterdir()) == []


def test_a_start_before_the_draft_date_is_also_refused(monkeypatch, staging_dir):
    """Pre-draft snapshots would claim ADP knowledge nobody had yet."""
    cloud = FakeConnection("cloud", _cloud_responder())
    _install(monkeypatch, cloud)
    with pytest.raises(SystemExit, match="must be the 2025 draft date"):
        main.run_replay(
            "ppr-sf-10", "2025", date(2025, 8, 1), date(2025, 8, 4), timedelta(days=1), 5
        )


def test_the_scheduled_invocation_uses_the_draft_date():
    """The systemd unit's --start must satisfy the guard above."""
    unit = (
        Path(__file__).resolve().parents[2]
        / "deploy/worker-host/ff-sims-player-valuations.service"
    )
    exec_start = next(
        ln for ln in unit.read_text().splitlines() if ln.startswith("ExecStart=")
    )
    assert f"--start {SEASONS['2025'].draft_date}" in exec_start


def test_end_earlier_than_published_snapshots_is_refused(monkeypatch, staging_dir):
    def responder(sql, params):
        if "max(valuation_date)" in sql:
            return [(date(2026, 1, 1),)]
        return _cloud_responder()(sql, params)

    cloud = FakeConnection("cloud", responder)
    _install(monkeypatch, cloud)

    with pytest.raises(SystemExit) as exc:
        main.run_replay(*ARGS)

    assert "later than --end" in str(exc.value)
    assert not cloud.ran("DELETE FROM player_valuations")


def test_staged_bundle_replays_to_the_same_values_without_a_database(
    monkeypatch, staging_dir, capsys
):
    cloud = FakeConnection("cloud", _cloud_responder())
    _install(monkeypatch, cloud)
    main.run_replay(*ARGS)
    db_output = capsys.readouterr().out

    run_dir = next(iter(staging_dir.iterdir()))
    # any DB access from here on is a bug
    monkeypatch.setattr(
        db.psycopg, "connect", lambda *a, **k: pytest.fail("must not connect")
    )
    main.run_from_bundle(run_dir, "ppr-sf-10", "2025", 5)
    bundle_output = capsys.readouterr().out

    def ranking_rows(text: str) -> list[str]:
        """The printed rankings table: `<rank> <player_id> <name> ... <games>`."""
        return [ln for ln in text.splitlines() if re.match(r"^\d+\s+\w+\s+\S", ln)]

    rows = ranking_rows(db_output)
    assert len(rows) == 3
    assert ranking_rows(bundle_output) == rows


def test_stale_run_directories_are_pruned_on_the_next_run(monkeypatch, staging_dir):
    import os
    import time

    staging_dir.mkdir(parents=True)
    stale = staging.new_run_dir(staging_dir, run_id="ancient")
    old = time.time() - 30 * 86400
    os.utime(stale, (old, old))

    cloud = FakeConnection("cloud", _cloud_responder())
    _install(monkeypatch, cloud)
    main.run_replay(*ARGS)

    assert not stale.exists()
    assert len(list(staging_dir.iterdir())) == 1
