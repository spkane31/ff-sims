"""
Player valuation CLI — full replays only.

The model is a robust anchored market estimator (src/market_value.py): at
every snapshot boundary all player market scores are refitted jointly from
the recency-weighted trade window, anchored by an ADP prior, and published
through one explicit calibration curve. A separate magnitude-preserving
performance signal (src/performance.py) rides along as projected_par.

Inputs come from the archive database (ARCHIVE_DATABASE_URL: complete Sleeper
history) plus the cloud database (DATABASE_URL: player identities and
finalized scoring); every valuation write lands in the cloud. Both URLs are
read from analysis/.env.

Each database run rebuilds the requested window from scratch and republishes
it. Incremental/checkpoint processing is deliberately NOT in this program —
that belongs in a future scheduled worker.

RUN
---
    python main.py --demo                          # synthetic data, no DB
    python main.py --segment ppr-sf-10 --season 2025 \
        --start 2025-08-25 --step 24h [--end YYYY-MM-DD]
    python main.py --from-bundle /tmp/ff-sims-player-valuations/<run-id>
    python main.py --evaluate-bundle /tmp/ff-sims-player-valuations/<run-id>
"""

from __future__ import annotations

import argparse
import re
import sys
import time
from datetime import date, datetime, timedelta, timezone
from pathlib import Path

import pandas as pd

from src import db, evaluation, market_value, progress, staging
from src.config import DEFAULT_SEGMENT_KEY, SEASONS, SEGMENTS
from src.models import PlayerProfile
from src.runner import replay_market, validate_step

EXIT_LOCKED = 2  # another replay holds the advisory lock


# ----------------------------------------------------------------------------- #
# ARGUMENT PARSING
# ----------------------------------------------------------------------------- #


def parse_step(text: str) -> timedelta:
    """`--step 24h` / `2d` -> timedelta. Whole-day steps only (see
    runner.validate_step for why)."""
    m = re.fullmatch(r"(\d+)\s*([hd])", text.strip().lower())
    if not m:
        raise argparse.ArgumentTypeError(
            f"invalid step {text!r} — use e.g. 24h or 1d"
        )
    amount, unit = int(m.group(1)), m.group(2)
    step = timedelta(hours=amount) if unit == "h" else timedelta(days=amount)
    if step <= timedelta(0):
        raise argparse.ArgumentTypeError("step must be positive")
    return step


def parse_day(text: str) -> date:
    try:
        return datetime.strptime(text.strip(), "%Y-%m-%d").date()
    except ValueError:
        raise argparse.ArgumentTypeError(
            f"invalid date {text!r} — use YYYY-MM-DD"
        ) from None


def utc_today() -> date:
    return datetime.now(timezone.utc).date()


def default_end() -> date:
    """Exclusive end boundary: today.

    The last snapshot a run writes is dated `end`, and a row dated D is the
    state at the *start* of UTC day D. Today is therefore the newest date this
    can publish honestly: it covers every event through the end of yesterday.

    Defaulting to tomorrow instead would publish a future-dated row built from
    an input window that has not happened yet — provably incomplete the moment
    anything lands today, and stale until the next run overwrites it.
    """
    return utc_today()


def build_parser() -> argparse.ArgumentParser:
    ap = argparse.ArgumentParser(
        description="Single-segment player valuation (full replay)."
    )
    ap.add_argument("--demo", action="store_true", help="run on synthetic demo data")
    ap.add_argument(
        "--from-bundle",
        type=Path,
        metavar="DIR",
        help="replay a staged Parquet run directory; no database access, no writes",
    )
    ap.add_argument(
        "--evaluate-bundle",
        type=Path,
        metavar="DIR",
        help="score the model against a staged bundle's held-out trades"
        " (league-blocked and time-blocked, plus the flat negative control);"
        " never connects to either database",
    )
    ap.add_argument("--season", default="2025", choices=sorted(SEASONS))
    ap.add_argument(
        "--segment",
        default=DEFAULT_SEGMENT_KEY,
        choices=sorted(SEGMENTS),
        help="league segment to value (see SEGMENTS in src/config.py)",
    )
    ap.add_argument(
        "--start", type=parse_day, metavar="YYYY-MM-DD",
        help="first included UTC day (required for a database run)",
    )
    ap.add_argument(
        "--end", type=parse_day, metavar="YYYY-MM-DD",
        help="exclusive end boundary (default: the current UTC date)",
    )
    ap.add_argument(
        "--step", type=parse_step, metavar="DURATION",
        help="replay batch/snapshot cadence, e.g. 24h (required for a database run)",
    )
    ap.add_argument("--top", type=int, default=30, help="how many players to print")
    return ap


# ----------------------------------------------------------------------------- #
# RUN MODES
# ----------------------------------------------------------------------------- #


def _name_of(players: dict[str, PlayerProfile], pid: str) -> str:
    profile = players.get(pid)
    return (profile.name if profile else "") or pid


def _log_market_diagnostics(
    diagnostics: dict | None,
    players: dict[str, PlayerProfile],
    frame: pd.DataFrame,
) -> None:
    """One block at the end describing how healthy the final fit is."""
    if not diagnostics:
        return
    d = diagnostics
    print("  market diagnostics:", flush=True)
    print(
        f"    players {d['players_fit']}"
        f" (prior {d['players_with_prior']},"
        f" trade-only {d['trade_only_players']})"
        f" · trades used {d['trades_used']}"
        f" · outliers removed {d['outliers_removed']}"
        f" ({100 * d['outlier_share']:.1f}% of window)",
        flush=True,
    )
    if d["inlier_gap_p50"] is not None:
        print(
            f"    inlier |residual|: mean {d['inlier_gap_mean']:,.0f}"
            f" · p50 {d['inlier_gap_p50']:,.0f}"
            f" · p90 {d['inlier_gap_p90']:,.0f}"
            f" · p99 {d['inlier_gap_p99']:,.0f}"
            f" · max {d['inlier_gap_max']:,.0f}"
            f" ({100 * d['inlier_package_pct_error']:.1f}% of package)",
            flush=True,
        )
        share = d["top_league_weight_share"]
        print(
            f"    leagues {d['leagues']}"
            + (
                f" · heaviest league carries {100 * share:.1f}% of trade weight"
                if share is not None
                else ""
            ),
            flush=True,
        )
    else:
        print("    no trades in window — values are the ADP prior", flush=True)
    movers = ""
    if d["risers"]:
        names = ", ".join(
            f"{_name_of(players, p)} +{delta}" for p, delta in d["risers"]
        )
        movers += f" · risers: {names}"
    if d["fallers"]:
        names = ", ".join(
            f"{_name_of(players, p)} {delta}" for p, delta in d["fallers"]
        )
        movers += f" · fallers: {names}"
    spearman = (
        f"{d['prior_spearman']:.3f}"
        if d["prior_spearman"] is not None
        else "n/a (fewer than 3 prior players)"
    )
    print(f"    prior agreement: Spearman {spearman} vs ADP{movers}", flush=True)
    if d["dispersion_p50"] is not None:
        print(
            f"    dispersion: p50 {d['dispersion_p50']:,.0f}"
            f" · p90 {d['dispersion_p90']:,.0f}"
            f" · at floor {d['dispersion_at_floor']}/{d['players_fit']}",
            flush=True,
        )
    scored = int((frame["games"] > 0).sum())
    print(
        f"    performance: scored {scored} of {len(frame)} published players",
        flush=True,
    )


def _print_rankings(frame: pd.DataFrame, top: int, source: str) -> None:
    print(f"\nPlayer valuations  ({source})")
    print(
        f"top of curve = {market_value.PUBLISHED_TOP:.0f}"
        f"   |   floor = {market_value.PUBLISHED_FLOOR:.0f}\n"
    )
    print(frame.head(top).to_string())
    print(
        "\nvalue = published curve at market rank | market_score = fitted"
        " trade-market score"
        "\nmarket_dispersion = spread of recent implied trade values |"
        " projected_par = rest-of-season weekly PAR proxy\n"
    )


def run_demo(top: int) -> None:
    """The synthetic market with known true values, replayed like a real run."""
    market = evaluation.synthetic_market()
    players = {
        a.player_id: PlayerProfile(
            player_id=a.player_id, name=a.player_name, position=a.position
        )
        for a in market.adp
    }
    start, end = market.start.date(), market.end.date() + timedelta(days=1)

    last_frame: dict[str, pd.DataFrame] = {}
    stats = replay_market(
        market.trades, [], market.adp, players, SEASONS["2025"],
        start, end, timedelta(days=1),
        SEGMENTS[DEFAULT_SEGMENT_KEY].repl_rank_by_pos,
        lambda d, frame: last_frame.__setitem__("frame", frame),
    )
    print(f"  {stats.snapshots} snapshots, {stats.events_applied} events applied")
    frame = last_frame["frame"]
    truth_rank = {p: i + 1 for i, p in enumerate(market.players)}
    rho = evaluation.spearman(
        dict(zip(frame["player_id"], frame["market_score"])),
        {p: -r for p, r in truth_rank.items()},
    )
    print(f"  rank recovery vs planted truth: Spearman {rho:.3f}")
    _log_market_diagnostics(stats.diagnostics, players, frame)
    _print_rankings(frame, top, "built-in demo data")


def run_evaluate_bundle(bundle_dir: Path, segment_key: str, season: str) -> None:
    """Score the model against a staged bundle's held-out trades.

    League-blocked and time-blocked holdouts, each shown beside the flat
    negative control. Package error alone never selects a model — the
    curve_valid gate is what a flat solution cannot pass. This mode never
    connects to a database; the bundle is the entire input.
    """
    manifest = staging.read_manifest(bundle_dir)
    args = manifest.get("args", {})
    end = date.fromisoformat(args["end"])
    segment_key = args.get("segment", segment_key)
    season = args.get("season", season)

    inputs = staging.read_bundle(bundle_dir)
    print(f"[{segment_key}/{season}] evaluating staged bundle {bundle_dir}")
    print(
        f"  inputs: {len(inputs.adp)} ADP players, {len(inputs.trades)} trades,"
        f" {len(inputs.scores)} weekly score rows"
    )

    prior = market_value.adp_prior(inputs.adp)

    def report_block(label: str, train, test, asof: datetime) -> None:
        if not test:
            print(f"  {label}: unavailable (no held-out trades)")
            return
        print(f"  {label}: {len(train)} train / {len(test)} test trades")
        fit = market_value.fit_snapshot(train, asof=asof, adp_prior=prior)
        print(
            evaluation.format_report("model", evaluation.evaluate(fit.values, test))
        )
        print(
            evaluation.format_report(
                "flat control",
                evaluation.flat_control_report(sorted(fit.values), test),
            )
        )

    window_end = datetime.combine(end, datetime.min.time())
    with_league = [t for t in inputs.trades if t.league_id]
    if with_league:
        train, test = evaluation.league_blocked_split(inputs.trades)
        report_block("league-blocked holdout", train, test, window_end)
    else:
        print(
            "  league-blocked holdout: unavailable — bundle has no league ids"
            " (staged before schema v3)"
        )

    if inputs.trades:
        cutoff = evaluation.time_cutoff(inputs.trades)
        train, test = evaluation.time_blocked_split(inputs.trades, cutoff)
        report_block(
            f"time-blocked holdout (cutoff {cutoff.date()})", train, test, cutoff
        )
    else:
        print("  time-blocked holdout: unavailable — bundle has no trades")


def run_from_bundle(
    bundle_dir: Path, segment_key: str, season: str, top: int
) -> None:
    """Replay a staged bundle with no database access — the reproducibility
    check for a production run's artifact."""
    manifest = staging.read_manifest(bundle_dir)
    args = manifest.get("args", {})
    start = date.fromisoformat(args["start"])
    end = date.fromisoformat(args["end"])
    step = timedelta(hours=float(args["step_hours"]))
    segment_key = args.get("segment", segment_key)
    season = args.get("season", season)

    inputs = staging.read_bundle(bundle_dir)
    print(f"[{segment_key}/{season}] replaying staged bundle {bundle_dir}")
    print(
        f"  inputs: {len(inputs.adp)} ADP players, {len(inputs.trades)} trades,"
        f" {len(inputs.scores)} weekly score rows"
    )

    reporter = progress.ProgressReporter(progress.total_batches(start, end, step))
    last_frame: dict[str, pd.DataFrame] = {}

    def on_snapshot(day: date, frame: pd.DataFrame) -> None:
        last_frame["frame"] = frame
        reporter.tick(day)

    stats = replay_market(
        inputs.trades, inputs.scores, inputs.adp, inputs.players,
        SEASONS[season], start, end, step,
        SEGMENTS[segment_key].repl_rank_by_pos, on_snapshot,
    )
    print(f"  {stats.snapshots} snapshots, {stats.events_applied} events applied")
    _log_market_diagnostics(stats.diagnostics, inputs.players, last_frame["frame"])
    _print_rankings(
        last_frame["frame"], top, f"{segment_key} season {season} (staged bundle)"
    )


def run_replay(
    segment_key: str,
    season: str,
    start: date,
    end: date,
    step: timedelta,
    top: int,
) -> None:
    segment = SEGMENTS[segment_key]
    season_dates = SEASONS[season]
    validate_step(start, end, step)

    # --start is both the input window's lower bound and the model clock's
    # origin, so a later start would fit from a partial trade window and
    # publish snapshots that silently omit every trade before it. Supporting
    # it properly means loading from the draft date and only publishing the
    # requested window, which is the incremental behavior this CLI
    # deliberately does not have.
    if start != season_dates.draft_date:
        sys.exit(
            f"--start must be the {season} draft date"
            f" ({season_dates.draft_date}), got {start}. This CLI replays a"
            " season in full; a later start would omit every event before it"
            " while still rewriting the published range."
        )

    started = time.monotonic()

    total = progress.total_batches(start, end, step)
    print(
        f"[{segment.key}/{season}] full replay {start} -> {end} (exclusive),"
        f" step {int(step.total_seconds() // 3600)}h, {total} snapshots",
        flush=True,
    )

    # Prune before staging so a long-failing job can't fill the disk, and so
    # the active directory (created next) is never a pruning candidate.
    root = staging.staging_root()
    root.mkdir(parents=True, exist_ok=True)
    run_dir = staging.new_run_dir(root)
    print(f"  staging directory: {run_dir}")
    staging.prune_run_dirs(root, keep=run_dir)

    window_start = datetime.combine(start, datetime.min.time())
    window_end = datetime.combine(end, datetime.min.time())

    with db.open_sources() as sources:
        try:
            inputs = db.load_inputs(
                sources, segment, season, season_dates, window_start, window_end
            )
            print(
                f"  inputs: {len(inputs.adp)} ADP players, {len(inputs.trades)}"
                f" trades, {len(inputs.scores)} weekly score rows"
            )
            if inputs.skipped_trades:
                print(
                    f"  skipped {inputs.skipped_trades} trade rows the model"
                    " cannot value (draft picks / FAAB / not two-sided)",
                    flush=True,
                )
            if inputs.skipped_nonfantasy:
                print(
                    f"  skipped {inputs.skipped_nonfantasy} trades involving a"
                    " position weekly scores never cover (IDP / FB), which the"
                    " model could never correct",
                    flush=True,
                )
            if not inputs.adp:
                sys.exit("no ADP data for this segment/season — nothing to seed")

            manifest = staging.write_bundle(
                run_dir,
                adp=inputs.adp,
                trades=inputs.trades,
                scores=inputs.scores,
                players=inputs.players,
                manifest_extra={
                    **staging.manifest_args(segment.key, season, start, end, step),
                    "skipped_trades": inputs.skipped_trades,
                    "skipped_nonfantasy": inputs.skipped_nonfantasy,
                },
            )
            print(f"  staged bundle checksums: {manifest['checksums']}")

            latest = db.latest_snapshot_date(sources.cloud, segment.key)
            if latest is not None and latest > end:
                sys.exit(
                    f"refusing to run: {segment.key} already has snapshots through"
                    f" {latest}, which is later than --end {end}. Rows after the"
                    " replayed range would be left stale."
                )
            sources.cloud.commit()  # end the read phase before the write phase

            # Everything below is one cloud transaction: the delete and every
            # snapshot land together or not at all. The segment lock is its
            # first statement and lives exactly as long as it does — the
            # database releases it at commit or rollback no matter how this
            # process dies, so it cannot be left behind for the next run.
            # Contention is therefore detected after staging rather than
            # before it, which costs the loser a read pass and buys a lock
            # that cannot strand itself (see db.try_advisory_xact_lock).
            if not db.try_advisory_xact_lock(sources.cloud, segment.key):
                print(
                    f"another replay holds the {segment.key} lock — exiting"
                    " without touching cloud output",
                    file=sys.stderr,
                    flush=True,
                )
                sys.exit(EXIT_LOCKED)

            db.delete_snapshots(sources.cloud, segment.key, start, end)

            reporter = progress.ProgressReporter(total)
            last_frame: dict[str, pd.DataFrame] = {}

            def on_snapshot(day: date, frame: pd.DataFrame) -> None:
                db.write_snapshot(sources.cloud, segment.key, day, frame)
                last_frame["frame"] = frame
                reporter.tick(day)

            stats = replay_market(
                inputs.trades, inputs.scores, inputs.adp, inputs.players,
                season_dates, start, end, step,
                SEGMENTS[segment.key].repl_rank_by_pos, on_snapshot,
            )
            sources.cloud.commit()
        except BaseException:
            # BaseException, not Exception: SystemExit (the lock-contention
            # path above) must roll back too, which is also what releases the
            # transaction-scoped lock.
            sources.cloud.rollback()
            raise

    print(
        f"  wrote {stats.snapshots} daily snapshots,"
        f" {stats.events_applied} events applied"
    )
    if stats.dropped_before_start or stats.dropped_at_or_after_end:
        print(
            f"  ignored {stats.dropped_before_start} events before {start} and"
            f" {stats.dropped_at_or_after_end} at/after {end}"
        )
    print(f"  elapsed {time.monotonic() - started:.1f}s")
    _log_market_diagnostics(stats.diagnostics, inputs.players, last_frame["frame"])
    _print_rankings(
        last_frame["frame"], top, f"{segment.key} season {season} (database)"
    )


def main(argv: list[str] | None = None) -> None:
    ap = build_parser()
    args = ap.parse_args(argv)

    if args.demo:
        run_demo(args.top)
        return
    if args.evaluate_bundle:
        run_evaluate_bundle(args.evaluate_bundle, args.segment, args.season)
        return
    if args.from_bundle:
        run_from_bundle(args.from_bundle, args.segment, args.season, args.top)
        return

    # A database run is always a full replay: --step is what selects it, so a
    # caller can't accidentally get incremental behavior out of this program.
    if args.step is None:
        ap.error(
            "--step is required for a database run (e.g. --step 24h);"
            " this CLI does full replays only"
        )
    if args.start is None:
        ap.error("--start is required for a database run (e.g. --start 2025-08-25)")

    end = args.end or default_end()
    run_replay(args.segment, args.season, args.start, end, args.step, args.top)


if __name__ == "__main__":
    main()
