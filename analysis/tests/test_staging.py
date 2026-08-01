import json
import os
import time
from datetime import date, datetime, timedelta, timezone

import pyarrow.parquet as pq
import pytest

from src import staging
from src.models import AverageDraftPosition, PlayerProfile, Trade, WeeklyScore

ADP = [
    AverageDraftPosition(player_id="p1", player_name="A", position="QB", adp=1.5),
    AverageDraftPosition(player_id="p2", player_name="B", position="RB", adp=12.25),
]
TRADES = [
    Trade(
        trade_id="t1", ts=datetime(2025, 9, 3, 14, 30),
        side_a=["p1"], side_b=["p2", "p3"], created_ms=1756909800000,
        league_id="lgA",
    )
]
SCORES = [
    WeeklyScore(week=1, player_id="p1", position="QB", points=31.5),
    WeeklyScore(week=1, player_id="p2", position="RB", points=0.0),
]
# p3 is trade-only: it appears in TRADES but never in ADP or SCORES, so the
# bundle is the only place a replay can learn its name and position.
PLAYERS = {
    "p1": PlayerProfile(player_id="p1", name="A", position="QB"),
    "p2": PlayerProfile(player_id="p2", name="B", position="RB"),
    "p3": PlayerProfile(player_id="p3", name="C", position="WR"),
}


def _write(tmp_path):
    run_dir = staging.new_run_dir(tmp_path, run_id="run-a")
    manifest = staging.write_bundle(
        run_dir, adp=ADP, trades=TRADES, scores=SCORES, players=PLAYERS,
        manifest_extra=staging.manifest_args(
            "ppr-sf-10", "2025", date(2025, 8, 25), date(2026, 7, 28),
            timedelta(hours=24),
        ),
    )
    return run_dir, manifest


def test_bundle_round_trips_without_database_access(tmp_path):
    run_dir, _ = _write(tmp_path)
    got = staging.read_bundle(run_dir)
    assert got.adp == ADP
    assert got.trades == TRADES
    assert got.scores == SCORES
    assert got.players == PLAYERS


def test_bundle_schemas_are_stable_and_explicit(tmp_path):
    run_dir, _ = _write(tmp_path)
    assert pq.read_schema(run_dir / staging.ADP_FILE).equals(staging.ADP_SCHEMA)
    assert pq.read_schema(run_dir / staging.TRADES_FILE).equals(staging.TRADES_SCHEMA)
    assert pq.read_schema(run_dir / staging.SCORES_FILE).equals(staging.SCORES_SCHEMA)
    assert pq.read_schema(run_dir / staging.PLAYERS_FILE).equals(staging.PLAYERS_SCHEMA)


def test_empty_inputs_keep_the_same_schemas(tmp_path):
    """Without an explicit schema, pyarrow would infer null-typed columns here."""
    run_dir = staging.new_run_dir(tmp_path, run_id="empty")
    staging.write_bundle(run_dir, adp=[], trades=[], scores=[], players={})
    assert pq.read_schema(run_dir / staging.TRADES_FILE).equals(staging.TRADES_SCHEMA)
    got = staging.read_bundle(run_dir)
    assert (got.adp, got.trades, got.scores, got.players) == ([], [], [], {})


def test_manifest_records_arguments_counts_and_checksums(tmp_path):
    run_dir, manifest = _write(tmp_path)
    on_disk = json.loads((run_dir / staging.MANIFEST_FILE).read_text())
    assert on_disk == manifest
    assert on_disk["schema_version"] == staging.SCHEMA_VERSION
    assert on_disk["args"] == {
        "segment": "ppr-sf-10", "season": "2025",
        "start": "2025-08-25", "end": "2026-07-28", "step_hours": 24.0,
    }
    assert on_disk["counts"] == {
        "adp": 2, "trades": 1, "weekly_scores": 2, "players": 3,
    }
    assert set(on_disk["checksums"]) == {
        staging.ADP_FILE, staging.TRADES_FILE, staging.SCORES_FILE,
        staging.PLAYERS_FILE,
    }
    assert all(len(c) == 64 for c in on_disk["checksums"].values())


def test_corrupted_file_fails_the_checksum_check(tmp_path):
    run_dir, _ = _write(tmp_path)
    (run_dir / staging.ADP_FILE).write_bytes(b"not parquet")
    with pytest.raises(staging.BundleError, match="checksum mismatch"):
        staging.read_bundle(run_dir)


def test_incomplete_bundle_has_no_manifest(tmp_path):
    run_dir = staging.new_run_dir(tmp_path, run_id="partial")
    with pytest.raises(staging.BundleError, match="incomplete"):
        staging.read_bundle(run_dir)


def test_trades_round_trip_league_id(tmp_path):
    run_dir, _ = _write(tmp_path)
    got = staging.read_bundle(run_dir)
    assert [t.league_id for t in got.trades] == ["lgA"]


def test_v2_bundle_reads_with_unknown_league(tmp_path):
    """Bundles staged before league_id existed (schema v2) must stay
    replayable; their trades come back with league_id ''. League-blocked
    evaluation is simply unavailable for them."""
    import pyarrow as pa
    import pyarrow.parquet as pq

    run_dir = staging.new_run_dir(tmp_path, run_id="v2-era")
    v2_trades_schema = pa.schema(
        [
            pa.field("trade_id", pa.string(), nullable=False),
            pa.field("ts", pa.timestamp("us"), nullable=False),
            pa.field("side_a", pa.list_(pa.string()), nullable=False),
            pa.field("side_b", pa.list_(pa.string()), nullable=False),
            pa.field("created_ms", pa.int64(), nullable=False),
        ]
    )
    # the other three files use the current writers (their schemas are
    # unchanged between v2 and v3); the trades file is then replaced with a
    # v2-era one
    staging.write_bundle(run_dir, adp=ADP, trades=[], scores=SCORES, players=PLAYERS)
    t = TRADES[0]
    pq.write_table(
        pa.table(
            {
                "trade_id": [t.trade_id],
                "ts": [t.ts],
                "side_a": [list(t.side_a)],
                "side_b": [list(t.side_b)],
                "created_ms": [t.created_ms],
            },
            schema=v2_trades_schema,
        ),
        run_dir / staging.TRADES_FILE,
    )
    manifest = json.loads((run_dir / staging.MANIFEST_FILE).read_text())
    manifest["schema_version"] = 2
    manifest["checksums"].pop(staging.TRADES_FILE, None)
    (run_dir / staging.MANIFEST_FILE).write_text(json.dumps(manifest))

    got = staging.read_bundle(run_dir)
    assert [t.league_id for t in got.trades] == [""]
    assert got.trades[0].side_b == ["p2", "p3"]


def test_unknown_schema_version_is_refused(tmp_path):
    run_dir, manifest = _write(tmp_path)
    manifest["schema_version"] = staging.SCHEMA_VERSION + 1
    (run_dir / staging.MANIFEST_FILE).write_text(json.dumps(manifest))
    with pytest.raises(staging.BundleError, match="schema_version"):
        staging.read_bundle(run_dir)


# -------------------------------------------------------------- retention --


def _age(path, days: float) -> None:
    old = time.time() - days * 86400
    os.utime(path, (old, old))


def test_prune_removes_old_runs_and_never_the_active_one(tmp_path):
    active = staging.new_run_dir(tmp_path, run_id="active")
    stale_ok = staging.new_run_dir(tmp_path, run_id="stale-complete")
    staging.write_bundle(stale_ok, adp=ADP, trades=[], scores=[], players=PLAYERS)
    stale_failed = staging.new_run_dir(tmp_path, run_id="stale-failed")  # no manifest
    recent = staging.new_run_dir(tmp_path, run_id="recent")

    for d in (active, stale_ok, stale_failed):
        _age(d, 20)
    _age(recent, 3)

    logged: list[str] = []
    removed = staging.prune_run_dirs(
        tmp_path, keep=active, days=14, log=logged.append
    )

    assert sorted(p.name for p in removed) == ["stale-complete", "stale-failed"]
    assert active.exists() and recent.exists()
    assert not stale_ok.exists() and not stale_failed.exists()
    assert len(logged) == 2


def test_retention_and_root_come_from_the_environment(monkeypatch, tmp_path):
    monkeypatch.setenv(staging.STAGING_DIR_ENV, str(tmp_path / "custom"))
    monkeypatch.setenv(staging.RETENTION_DAYS_ENV, "3")
    assert staging.staging_root() == tmp_path / "custom"
    assert staging.retention_days() == 3

    monkeypatch.delenv(staging.RETENTION_DAYS_ENV)
    assert staging.retention_days() == staging.DEFAULT_RETENTION_DAYS

    monkeypatch.setenv(staging.RETENTION_DAYS_ENV, "0")
    with pytest.raises(ValueError, match="must be >= 1"):
        staging.retention_days()


def test_prune_uses_the_configured_retention_window(tmp_path):
    old = staging.new_run_dir(tmp_path, run_id="four-days")
    _age(old, 4)
    now = datetime.now(timezone.utc)
    assert staging.prune_run_dirs(tmp_path, days=14, now=now, log=lambda m: None) == []
    assert staging.prune_run_dirs(tmp_path, days=3, now=now, log=lambda m: None) == [old]
