"""--evaluate-bundle: score models on a staged bundle, never touching a DB."""

from datetime import date, timedelta

import pytest

import main
from src import db, evaluation, staging
from src.models import PlayerProfile


def _bundle(tmp_path):
    market = evaluation.synthetic_market(n_players=40, seed=3, n_trades=120)
    players = {
        a.player_id: PlayerProfile(
            player_id=a.player_id, name=a.player_name, position=a.position
        )
        for a in market.adp
    }
    run_dir = staging.new_run_dir(tmp_path, run_id="eval-me")
    staging.write_bundle(
        run_dir,
        adp=market.adp,
        trades=market.trades,
        scores=[],
        players=players,
        manifest_extra=staging.manifest_args(
            "ppr-sf-10", "2025", date(2025, 8, 25), date(2025, 11, 25),
            timedelta(hours=24),
        ),
    )
    return run_dir


def _forbid_db(monkeypatch):
    monkeypatch.setattr(
        db.psycopg, "connect",
        lambda *a, **k: pytest.fail("evaluation must never connect to a database"),
    )
    monkeypatch.setattr(
        db, "open_sources",
        lambda *a, **k: pytest.fail("evaluation must never open sources"),
    )


def test_evaluate_bundle_reports_blocked_holdouts_and_flat_control(
    monkeypatch, tmp_path, capsys
):
    run_dir = _bundle(tmp_path)
    _forbid_db(monkeypatch)

    main.main(["--evaluate-bundle", str(run_dir)])
    out = capsys.readouterr().out

    assert "league-blocked" in out
    assert "time-blocked" in out
    assert "flat control" in out
    assert "curve_valid" in out
    # the flat control must be reported invalid even with zero package error
    assert "-> valid False" in out


def test_evaluate_bundle_writes_nothing_anywhere(monkeypatch, tmp_path):
    """Beyond never connecting: the bundle directory itself stays untouched,
    so evaluating a production artifact cannot alter it."""
    run_dir = _bundle(tmp_path)
    _forbid_db(monkeypatch)
    before = {
        p.name: (p.stat().st_size, p.stat().st_mtime_ns)
        for p in run_dir.iterdir()
    }

    main.main(["--evaluate-bundle", str(run_dir)])

    after = {
        p.name: (p.stat().st_size, p.stat().st_mtime_ns)
        for p in run_dir.iterdir()
    }
    assert after == before


def test_evaluate_bundle_flag_routes_to_the_evaluator(monkeypatch):
    seen = {}
    monkeypatch.setattr(
        main, "run_evaluate_bundle",
        lambda bundle_dir, segment, season: seen.update(
            bundle=bundle_dir, segment=segment, season=season
        ),
    )
    main.main(["--evaluate-bundle", "/tmp/some-bundle"])
    assert str(seen["bundle"]) == "/tmp/some-bundle"
