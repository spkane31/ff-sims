"""Parquet staging for a replay run.

A run materializes its normalized model inputs to a unique directory before
touching cloud output, so a production run is inspectable and reproducible
without re-querying either database. The directory survives both success and
failure; old ones are pruned by age on the next run.

Layout of a run directory:

    <root>/<run-id>/adp.parquet
    <root>/<run-id>/trades.parquet
    <root>/<run-id>/weekly_scores.parquet
    <root>/<run-id>/players.parquet
    <root>/<run-id>/manifest.json     <- written last, after every file is flushed

The manifest's presence is therefore the marker of a complete bundle.
"""

from __future__ import annotations

import hashlib
import json
import os
import shutil
import tempfile
import uuid
from collections.abc import Callable, Iterable
from dataclasses import dataclass
from datetime import date, datetime, timedelta, timezone
from pathlib import Path

import pyarrow as pa
import pyarrow.parquet as pq

from .models import AverageDraftPosition, PlayerProfile, Trade, WeeklyScore

# Bump when a staged column changes meaning, is added, or is removed. A reader
# that does not recognize the version refuses the bundle rather than guessing.
# v2 added players.parquet: without it a bundle replay cannot give a
# trade-only player its name and position, so it would not reproduce the
# database run it was staged from.
# v3 added league_id to trades.parquet, which league-blocked evaluation splits
# on. v2 bundles are still readable: their trades come back with league_id "".
SCHEMA_VERSION = 3
READABLE_SCHEMA_VERSIONS = (2, SCHEMA_VERSION)

ADP_FILE = "adp.parquet"
TRADES_FILE = "trades.parquet"
SCORES_FILE = "weekly_scores.parquet"
PLAYERS_FILE = "players.parquet"
MANIFEST_FILE = "manifest.json"

STAGING_DIR_ENV = "FF_SIMS_VALUATION_STAGING_DIR"
RETENTION_DAYS_ENV = "FF_SIMS_VALUATION_RETENTION_DAYS"
DEFAULT_RETENTION_DAYS = 14
DEFAULT_STAGING_SUBDIR = "ff-sims-player-valuations"

# Explicit, stable schemas: pyarrow would otherwise infer types from the data,
# so an empty trade list and a populated one could disagree on column types.
ADP_SCHEMA = pa.schema(
    [
        pa.field("player_id", pa.string(), nullable=False),
        pa.field("player_name", pa.string(), nullable=False),
        pa.field("position", pa.string(), nullable=False),
        pa.field("adp", pa.float64(), nullable=False),
    ]
)

TRADES_SCHEMA = pa.schema(
    [
        pa.field("trade_id", pa.string(), nullable=False),
        # naive UTC, matching the model's end-to-end convention
        pa.field("ts", pa.timestamp("us"), nullable=False),
        pa.field("side_a", pa.list_(pa.string()), nullable=False),
        pa.field("side_b", pa.list_(pa.string()), nullable=False),
        pa.field("created_ms", pa.int64(), nullable=False),
        pa.field("league_id", pa.string(), nullable=False),
    ]
)

SCORES_SCHEMA = pa.schema(
    [
        pa.field("week", pa.int32(), nullable=False),
        pa.field("player_id", pa.string(), nullable=False),
        pa.field("position", pa.string(), nullable=False),
        pa.field("points", pa.float64(), nullable=False),
    ]
)


PLAYERS_SCHEMA = pa.schema(
    [
        pa.field("player_id", pa.string(), nullable=False),
        pa.field("name", pa.string(), nullable=False),
        pa.field("position", pa.string(), nullable=False),
    ]
)


class BundleError(RuntimeError):
    """A staged bundle is missing, incomplete, or of an unknown schema version."""


@dataclass(frozen=True)
class StagedInputs:
    adp: list[AverageDraftPosition]
    trades: list[Trade]
    scores: list[WeeklyScore]
    players: dict[str, PlayerProfile]


def staging_root() -> Path:
    """Root directory holding every run directory."""
    override = os.environ.get(STAGING_DIR_ENV)
    if override:
        return Path(override)
    return Path(tempfile.gettempdir()) / DEFAULT_STAGING_SUBDIR


def retention_days() -> int:
    raw = os.environ.get(RETENTION_DAYS_ENV)
    if not raw:
        return DEFAULT_RETENTION_DAYS
    days = int(raw)
    if days < 1:
        raise ValueError(f"{RETENTION_DAYS_ENV} must be >= 1, got {days}")
    return days


def new_run_dir(root: Path | None = None, run_id: str | None = None) -> Path:
    """Create (and return) this run's staging directory."""
    root = root if root is not None else staging_root()
    run_id = run_id or (
        datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
        + "-"
        + uuid.uuid4().hex[:8]
    )
    run_dir = root / run_id
    run_dir.mkdir(parents=True, exist_ok=False)
    return run_dir


def prune_run_dirs(
    root: Path,
    keep: Path | None = None,
    days: int | None = None,
    now: datetime | None = None,
    log: Callable[[str], None] = print,
) -> list[Path]:
    """Delete run directories older than the retention window.

    `keep` (the active run) is never removed regardless of age. Failed runs
    have no manifest, so age comes from directory mtime — that covers both
    completed and failed runs with one rule.
    """
    if not root.exists():
        return []
    days = days if days is not None else retention_days()
    now = now or datetime.now(timezone.utc)
    cutoff = now - timedelta(days=days)

    removed: list[Path] = []
    for child in sorted(root.iterdir()):
        if not child.is_dir():
            continue
        if keep is not None and child.resolve() == keep.resolve():
            continue
        mtime = datetime.fromtimestamp(child.stat().st_mtime, tz=timezone.utc)
        if mtime >= cutoff:
            continue
        shutil.rmtree(child)
        removed.append(child)
        log(f"  pruned staged run {child} (last modified {mtime.date()})")
    return removed


# ------------------------------------------------------------------ write --


def _adp_table(adp: Iterable[AverageDraftPosition]) -> pa.Table:
    rows = list(adp)
    return pa.table(
        {
            "player_id": [r.player_id for r in rows],
            "player_name": [r.player_name for r in rows],
            "position": [r.position for r in rows],
            "adp": [float(r.adp) for r in rows],
        },
        schema=ADP_SCHEMA,
    )


def _trades_table(trades: Iterable[Trade]) -> pa.Table:
    rows = list(trades)
    return pa.table(
        {
            "trade_id": [r.trade_id for r in rows],
            "ts": [r.ts for r in rows],
            "side_a": [list(r.side_a) for r in rows],
            "side_b": [list(r.side_b) for r in rows],
            "created_ms": [int(r.created_ms) for r in rows],
            "league_id": [r.league_id for r in rows],
        },
        schema=TRADES_SCHEMA,
    )


def _scores_table(scores: Iterable[WeeklyScore]) -> pa.Table:
    rows = list(scores)
    return pa.table(
        {
            "week": [int(r.week) for r in rows],
            "player_id": [r.player_id for r in rows],
            "position": [r.position for r in rows],
            "points": [float(r.points) for r in rows],
        },
        schema=SCORES_SCHEMA,
    )


def _players_table(players: dict[str, PlayerProfile]) -> pa.Table:
    # Sorted by id so the file — and therefore its checksum — depends only on
    # the resolved set, not on dict insertion order.
    rows = [players[pid] for pid in sorted(players)]
    return pa.table(
        {
            "player_id": [r.player_id for r in rows],
            "name": [r.name for r in rows],
            "position": [r.position for r in rows],
        },
        schema=PLAYERS_SCHEMA,
    )


def _sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def write_bundle(
    run_dir: Path,
    adp: list[AverageDraftPosition],
    trades: list[Trade],
    scores: list[WeeklyScore],
    players: dict[str, PlayerProfile],
    manifest_extra: dict | None = None,
) -> dict:
    """Write the four input files, then the manifest. Returns the manifest."""
    files = {
        ADP_FILE: _adp_table(adp),
        TRADES_FILE: _trades_table(trades),
        SCORES_FILE: _scores_table(scores),
        PLAYERS_FILE: _players_table(players),
    }
    for name, table in files.items():
        pq.write_table(table, run_dir / name)

    manifest = {
        "schema_version": SCHEMA_VERSION,
        "run_id": run_dir.name,
        "created_at": datetime.now(timezone.utc).isoformat(),
        "counts": {
            "adp": len(adp),
            "trades": len(trades),
            "weekly_scores": len(scores),
            "players": len(players),
        },
        "checksums": {name: _sha256(run_dir / name) for name in files},
    }
    manifest.update(manifest_extra or {})

    # Written last: a directory with a manifest is a complete bundle.
    (run_dir / MANIFEST_FILE).write_text(json.dumps(manifest, indent=2, default=str))
    return manifest


# ------------------------------------------------------------------- read --


def read_manifest(run_dir: Path) -> dict:
    path = run_dir / MANIFEST_FILE
    if not path.exists():
        raise BundleError(f"no {MANIFEST_FILE} in {run_dir} — bundle is incomplete")
    manifest = json.loads(path.read_text())
    version = manifest.get("schema_version")
    if version not in READABLE_SCHEMA_VERSIONS:
        readable = ", ".join(str(v) for v in READABLE_SCHEMA_VERSIONS)
        raise BundleError(
            f"{run_dir} has schema_version {version}, this build reads {readable}"
        )
    return manifest


def read_bundle(run_dir: Path, verify_checksums: bool = True) -> StagedInputs:
    """Load staged inputs back into model objects. No database access."""
    manifest = read_manifest(run_dir)
    if verify_checksums:
        for name, expected in manifest.get("checksums", {}).items():
            actual = _sha256(run_dir / name)
            if actual != expected:
                raise BundleError(
                    f"{run_dir / name} checksum mismatch:"
                    f" manifest {expected}, file {actual}"
                )

    adp_t = pq.read_table(run_dir / ADP_FILE)
    trades_t = pq.read_table(run_dir / TRADES_FILE)
    scores_t = pq.read_table(run_dir / SCORES_FILE)
    players_t = pq.read_table(run_dir / PLAYERS_FILE)

    adp = [
        AverageDraftPosition(
            player_id=r["player_id"],
            player_name=r["player_name"],
            position=r["position"],
            adp=float(r["adp"]),
        )
        for r in adp_t.to_pylist()
    ]
    trades = [
        Trade(
            trade_id=r["trade_id"],
            ts=r["ts"],
            side_a=list(r["side_a"]),
            side_b=list(r["side_b"]),
            created_ms=int(r["created_ms"]),
            # absent in v2-era bundles: league unknown
            league_id=r.get("league_id", "") or "",
        )
        for r in trades_t.to_pylist()
    ]
    scores = [
        WeeklyScore(
            week=int(r["week"]),
            player_id=r["player_id"],
            position=r["position"],
            points=float(r["points"]),
        )
        for r in scores_t.to_pylist()
    ]
    players = {
        r["player_id"]: PlayerProfile(
            player_id=r["player_id"], name=r["name"], position=r["position"]
        )
        for r in players_t.to_pylist()
    }
    return StagedInputs(adp=adp, trades=trades, scores=scores, players=players)


def manifest_args(
    segment_key: str,
    season: str,
    start: date,
    end: date,
    step: timedelta,
) -> dict:
    """The argument block every run records in its manifest."""
    return {
        "args": {
            "segment": segment_key,
            "season": season,
            "start": start.isoformat(),
            "end": end.isoformat(),
            "step_hours": step.total_seconds() / 3600.0,
        }
    }
