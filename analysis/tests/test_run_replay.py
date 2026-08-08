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
    # market outputs, and only market outputs: no belief state, no watermarks
    for col in ("market_score", "market_dispersion", "projected_par",
                "projection_uncertainty"):
        assert col in inserts[0][1]
    assert not cloud.ran("INSERT INTO valuation_state")
    assert not cloud.ran("INSERT INTO valuation_runs")
    # the lock is the write transaction's first statement, so it is taken
    # after staging and released by the commit that ends that transaction
    assert cloud.order_of("pg_try_advisory_xact_lock") < cloud.order_of(
        "DELETE FROM player_valuations"
    )
    assert not cloud.ran("pg_advisory_unlock")  # nothing to unlock by hand
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
    # the rollback is what releases the lock; there is no unlock to forget
    assert "rollback" in after_delete


def test_lock_contention_exits_without_touching_output(monkeypatch, staging_dir):
    """The loser stages its inputs — the lock is not taken until the write
    transaction opens — but writes nothing, and rolls back, which is what
    releases the lock the winner is holding."""
    def responder(sql, params):
        if "pg_try_advisory_xact_lock" in sql:
            return [(False,)]
        return _cloud_responder()(sql, params)

    cloud = FakeConnection("cloud", responder)
    _install(monkeypatch, cloud)

    with pytest.raises(SystemExit) as exc:
        main.run_replay(*ARGS)

    assert exc.value.code == main.EXIT_LOCKED
    assert not cloud.ran("DELETE FROM player_valuations")
    assert not cloud.ran("INSERT INTO player_valuations")
    # SystemExit still has to unwind through the rollback, or the aborted
    # write transaction would keep the lock until the process exits
    assert cloud.calls[-1][0] == "rollback"


def test_missing_identity_aborts_before_the_output_transaction(
    monkeypatch, staging_dir
):
    cloud = FakeConnection("cloud", _cloud_responder(players=[("p1", "QB One", "QB")]))
    _install(monkeypatch, cloud)

    with pytest.raises(db.MissingPlayerIdentities):
        main.run_replay(*ARGS)

    assert not cloud.ran("DELETE FROM player_valuations")
    assert not cloud.ran("INSERT INTO player_valuations")
    # identity resolution fails in the read phase, before the lock is taken
    assert not cloud.ran("pg_try_advisory_xact_lock")
    # the run directory is kept for inspection even though the run failed
    assert len(list(staging_dir.iterdir())) == 1


def test_start_must_be_the_season_draft_date(monkeypatch, staging_dir):
    """A later --start silently drops every event before it.

    --start is both the input window's lower bound and the recency clock's
    origin, so the run would fit from a partial trade window and publish
    snapshots that omit every trade before it. load_inputs already windows
    its queries to [start, end), so replay_market's dropped_before_start
    counter sees nothing to report.
    """
    cloud = FakeConnection("cloud", _cloud_responder())
    _install(monkeypatch, cloud)

    late = date(2025, 11, 1)  # mid-season, well after the 2025-08-25 draft
    with pytest.raises(SystemExit) as exc:
        main.run_replay("ppr-sf-10", "2025", late, date(2025, 11, 4), timedelta(days=1), 5)

    assert "must be the 2025 draft date (2025-08-25)" in str(exc.value)
    # nothing was read, staged, or written
    assert not cloud.ran("DELETE FROM player_valuations")
    assert not cloud.ran("INSERT INTO player_valuations")
    assert not cloud.ran("pg_try_advisory_xact_lock")
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


# ------------------------------------------------- trade-only identities --

# A trade between a drafted player and one who never cleared the ADP
# threshold. w7 is the whole point: the archive knows only its ID, so its name
# and position have to survive the trip through cloud resolution, the staged
# bundle, and the Valuator.
_TRADE_ONLY_ADP = [("p1", 3.0), ("p2", 14.5)]
_TRADE_ONLY_TXNS = [("t9", 1756180000000, {"p1": 1, "w7": 2}, None, None, "lgA")]
_TRADE_ONLY_PLAYERS = [
    ("p1", "QB One", "QB"),
    ("p2", "RB Two", "RB"),
    ("w7", "WR Seven", "WR"),
]


def _trade_only_archive(sql, params):
    if "sleeper_draft_picks" in sql:
        return _TRADE_ONLY_ADP
    if "sleeper_transactions" in sql:
        return _TRADE_ONLY_TXNS
    return []


def _published(cloud: FakeConnection) -> list[tuple]:
    return [
        row
        for op, sql, rows in cloud.calls
        if op == "executemany" and sql and "INSERT INTO player_valuations" in sql
        for row in rows
    ]


def test_a_trade_only_player_is_published_with_its_real_position(
    monkeypatch, staging_dir
):
    cloud = FakeConnection("cloud", _cloud_responder(players=_TRADE_ONLY_PLAYERS, scores=[]))
    archive = FakeConnection("archive", _trade_only_archive)
    _install(monkeypatch, cloud, archive)

    main.run_replay(*ARGS)

    # write_snapshot rows are (segment, player_id, date, rank, pos_rank,
    # value, market_score, market_dispersion, projected_par,
    # projection_uncertainty, games, position, trades)
    w7 = [r for r in _published(cloud) if r[1] == "w7"]
    assert w7, "the trade-only player was never published"
    assert {r[11] for r in w7} == {"WR"}
    assert "DEFAULT" not in {r[11] for r in _published(cloud)}


def test_the_staged_bundle_reproduces_trade_only_identities(monkeypatch, staging_dir):
    """--from-bundle must resolve identities the same way the database run
    did, or the reproducibility check quietly compares different models."""
    cloud = FakeConnection("cloud", _cloud_responder(players=_TRADE_ONLY_PLAYERS, scores=[]))
    _install(monkeypatch, cloud, FakeConnection("archive", _trade_only_archive))

    main.run_replay(*ARGS)

    run_dir = next(iter(staging_dir.iterdir()))
    staged = staging.read_bundle(run_dir)
    assert staged.players["w7"].name == "WR Seven"
    assert staged.players["w7"].position == "WR"
    assert "w7" not in {a.player_id for a in staged.adp}  # trade-only, as intended


def test_the_replay_reports_progress_as_it_steps(monkeypatch, staging_dir, capsys):
    """A full rebuild is hundreds of snapshots over many minutes; without this
    the journal shows nothing between the input summary and the final table,
    so a slow run is indistinguishable from a hung one."""
    cloud = FakeConnection("cloud", _cloud_responder())
    _install(monkeypatch, cloud)

    main.run_replay(*ARGS)

    out = capsys.readouterr().out
    assert "3 snapshots" in out  # the header states the total up front
    # ARGS spans three days, so only the final line clears the interval
    assert "3/3 snapshots" in out
    assert "2025-08-28" in out
    assert "100%" in out


def test_the_run_logs_market_diagnostics(monkeypatch, staging_dir, capsys):
    """The rankings table alone cannot say whether the fit is healthy — how
    much trade evidence backs it, what the outlier pressure was, how
    concentrated the leagues are, and how far the market moved players off
    their ADP prior. A production run's log is where that has to live."""
    cloud = FakeConnection(
        "cloud", _cloud_responder(players=_TRADE_ONLY_PLAYERS, scores=[])
    )
    _install(monkeypatch, cloud, FakeConnection("archive", _trade_only_archive))

    main.run_replay(*ARGS)

    out = capsys.readouterr().out
    assert "market diagnostics:" in out
    assert "trades used 1" in out  # t9 lands Aug 26, inside the window
    assert "outliers removed" in out
    assert "|residual|" in out
    assert "league" in out
    assert "prior agreement" in out
    assert "dispersion" in out
    assert "scored 0 of" in out  # no weekly scores in this fixture


def test_diagnostics_degrade_when_the_window_has_no_trades(
    monkeypatch, staging_dir, capsys
):
    # the default responder's only trade is Sep 3, outside the Aug 25-28 window
    cloud = FakeConnection("cloud", _cloud_responder())
    _install(monkeypatch, cloud)

    main.run_replay(*ARGS)

    out = capsys.readouterr().out
    assert "market diagnostics:" in out
    assert "trades used 0" in out
    assert "no trades in window" in out


def test_a_trade_touching_an_unscoreable_position_is_skipped_whole(
    monkeypatch, staging_dir, capsys
):
    """The weekly-score query only returns fantasy positions, so an IDP or FB
    that entered through a trade could never receive performance evidence and
    would keep whatever the trade stream implied forever. The whole trade goes
    — dropping just that player would leave the sum constraint asserting an
    equality nobody offered."""
    idp = _TRADE_ONLY_PLAYERS[:2] + [("w7", "Edge Rusher", "DE")]
    cloud = FakeConnection("cloud", _cloud_responder(players=idp, scores=[]))
    _install(monkeypatch, cloud, FakeConnection("archive", _trade_only_archive))

    main.run_replay(*ARGS)

    out = capsys.readouterr().out
    assert "skipped 1 trades involving a position" in out
    assert "0 trades" in out  # the fantasy side of it is gone too, deliberately

    # neither the IDP nor its trade partner's trade count reaches the output
    published = _published(cloud)
    assert "w7" not in {r[1] for r in published}
    assert all(r[12] == 0 for r in published)
    assert "never scoreable" not in out  # nothing unscoreable is left to census


def test_the_staged_bundle_omits_players_no_surviving_input_references(
    monkeypatch, staging_dir
):
    idp = _TRADE_ONLY_PLAYERS[:2] + [("w7", "Edge Rusher", "DE")]
    cloud = FakeConnection("cloud", _cloud_responder(players=idp, scores=[]))
    _install(monkeypatch, cloud, FakeConnection("archive", _trade_only_archive))

    main.run_replay(*ARGS)

    staged = staging.read_bundle(next(iter(staging_dir.iterdir())))
    assert "w7" not in staged.players
    assert staged.trades == []


def test_the_trade_count_is_published_with_each_snapshot(monkeypatch, staging_dir):
    # _trade_only_archive's trade lands 2025-08-26, inside the ARGS window
    cloud = FakeConnection("cloud", _cloud_responder(players=_TRADE_ONLY_PLAYERS, scores=[]))
    _install(monkeypatch, cloud, FakeConnection("archive", _trade_only_archive))

    main.run_replay(*ARGS)

    insert = next(
        sql for op, sql, _ in cloud.calls
        if op == "executemany" and sql and "INSERT INTO player_valuations" in sql
    )
    assert "trades" in insert
    assert "trades = EXCLUDED.trades" in insert  # a re-run must overwrite it

    # p1 and w7 are the two sides of that trade; p2 was drafted, never traded
    final_day = max(r[2] for r in _published(cloud))
    rows = {r[1]: r[12] for r in _published(cloud) if r[2] == final_day}
    assert rows["p1"] == 1 and rows["w7"] == 1
    assert rows["p2"] == 0


def test_migrations_reconcile_the_legacy_market_model_version_29():
    """A database that recorded the old market migration as 029 can advance.

    That old migration has already added the market columns and removed the
    obsolete state. Migration 030 must therefore be safe to apply again, and
    031 restores the trade count skipped when Goose treated old 029 as current
    029_valuation_trade_counts.sql.
    """
    sql = (
        Path(__file__).resolve().parents[2]
        / "backend/migrations/030_market_valuation_model.sql"
    ).read_text()
    for needle in (
        "ADD COLUMN IF NOT EXISTS market_score FLOAT",
        "ADD COLUMN IF NOT EXISTS market_dispersion FLOAT",
        "ADD COLUMN IF NOT EXISTS projected_par FLOAT",
        "ADD COLUMN IF NOT EXISTS projection_uncertainty FLOAT",
        "DROP COLUMN IF EXISTS vorp",
        "DROP COLUMN IF EXISTS sd",
        "DROP TABLE IF EXISTS valuation_state",
        "DROP TABLE IF EXISTS valuation_runs",
    ):
        assert needle in sql

    reconciliation = (
        Path(__file__).resolve().parents[2]
        / "backend/migrations/031_reconcile_legacy_valuation_trade_counts.sql"
    ).read_text()
    assert "ADD COLUMN IF NOT EXISTS trades INT NOT NULL DEFAULT 0" in reconciliation
