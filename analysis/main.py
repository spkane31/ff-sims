"""
Player valuation CLI — full replays only.

The model lives in src/valuation.py (recursive-belief estimator over ADP,
trades, and weekly scores). Inputs come from the archive database
(ARCHIVE_DATABASE_URL: complete Sleeper history) plus the cloud database
(DATABASE_URL: player identities and finalized scoring); every valuation write
lands in the cloud. Both URLs are read from analysis/.env.

Each database run rebuilds the requested window from scratch and republishes
it. Incremental/checkpoint processing is deliberately NOT in this program —
that belongs in a future scheduled worker.

RUN
---
    python main.py --demo                          # synthetic data, no DB
    python main.py --segment ppr-sf-10 --season 2025 \
        --start 2025-08-25 --step 24h [--end YYYY-MM-DD]
    python main.py --from-bundle /tmp/ff-sims-player-valuations/<run-id>
"""

from __future__ import annotations

import argparse
import re
import sys
import time
from datetime import date, datetime, timedelta, timezone
from pathlib import Path

import numpy as np
import pandas as pd

from src import db, progress, staging, valuation
from src.config import DEFAULT_SEGMENT_KEY, SEASONS, SEGMENTS, week_ts
from src.models import RunState
from src.runner import adp_frame, build_events, fit_rho, replay, validate_step
from src.valuation import LAMBDA_ADP, RHO, V_TOP, Valuator, curve, curve_rank

SEASON_2025 = SEASONS["2025"]

EXIT_LOCKED = 2  # another replay holds the advisory lock


# ----------------------------------------------------------------------------- #
# DEMO generator (synthetic league, no DB needed)
# ----------------------------------------------------------------------------- #


def make_demo(seed: int = 7) -> tuple[pd.DataFrame, list[dict]]:
    rng = np.random.default_rng(seed)
    counts = {"QB": 12, "RB": 24, "WR": 28, "TE": 8}
    players = []
    pid = 0
    for pos, n in counts.items():
        for _ in range(n):
            players.append(
                {
                    "player_id": f"p{pid:03d}",
                    "player_name": f"{pos}_{pid:03d}",
                    "position": pos,
                }
            )
            pid += 1
    pdf = pd.DataFrame(players)
    order = rng.permutation(len(pdf))  # true talent order
    pdf["true_rank"] = np.argsort(order) + 1
    pdf["true_value"] = pdf["true_rank"].map(curve)

    # ADP = true rank observed with noise across 4 synthetic drafts
    adp = pdf["true_rank"].to_numpy()[:, None] + rng.normal(0, 3, size=(len(pdf), 4))
    adp = np.clip(adp.mean(axis=1), 1, None)
    adp_df = pdf[["player_id", "player_name", "position"]].copy()
    adp_df["adp"] = adp

    events: list[dict] = []

    # weekly scores, weeks 1..14
    pos_scale = {"QB": 1.6, "RB": 1.0, "WR": 1.05, "TE": 0.7}
    for week in range(1, 15):
        mean = pdf.apply(
            lambda r: pos_scale[r.position] * (26 * r.true_value / V_TOP + 4), axis=1
        )
        pts = np.maximum(0.0, rng.normal(mean.to_numpy(), (mean * 0.4).to_numpy()))
        wk = pdf[["player_id", "position"]].copy()
        wk["points"] = np.round(pts, 1)
        events.append({"ts": week_ts(SEASON_2025, week), "kind": "week", "scores": wk})

    # ~15 roughly-fair trades scattered across the season
    ids = pdf["player_id"].to_numpy()
    for _ in range(15):
        anchor = rng.choice(ids)
        partners = rng.choice(ids, size=2, replace=False)
        # keep it plausible: anchor for two players of similar combined value
        events.append(
            {
                "ts": datetime.combine(
                    SEASON_2025.season_start + timedelta(days=int(rng.integers(5, 95))),
                    datetime.min.time(),
                ),
                "kind": "trade",
                "side_a": [str(anchor)],
                "side_b": [str(p) for p in partners],
            }
        )

    return adp_df, events


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
    ap.add_argument(
        "--rho",
        type=float,
        metavar="VALUE",
        help="pin replacement value instead of fitting it from unbalanced"
        " trades (for comparing a fitted run against a fixed one)",
    )
    ap.add_argument(
        "--lam",
        type=float,
        metavar="VALUE",
        help="curve steepness (default 0.04). Never fitted: trade fairness has"
        " a global optimum at a flat curve, so it cannot identify a shape —"
        " see note 7 in src/valuation.py. This is a knob for trying one by hand",
    )
    return ap


# ----------------------------------------------------------------------------- #
# RUN MODES
# ----------------------------------------------------------------------------- #


def _print_rankings(v: Valuator, top: int, source: str) -> None:
    print(f"\nPlayer valuations  ({source})")
    print(f"ρ (replacement) = {v.rho:.0f}   |   top of curve = {V_TOP:.0f}\n")
    print(v.rankings().head(top).to_string())
    print(
        "\nvalue = current belief (additive scale) | vorp = value - ρ"
        " | sd = uncertainty band"
        "\ngames = decayed effective games scored | trades = raw count of"
        " trades the player appeared in\n"
    )


def run_demo(top: int) -> None:
    adp, events = make_demo()
    v = Valuator(
        start_ts=datetime.combine(SEASON_2025.draft_date, datetime.min.time()),
        repl_rank_by_pos=SEGMENTS[DEFAULT_SEGMENT_KEY].repl_rank_by_pos,
    )
    v.seed_from_adp(adp)
    v.advance(events)
    _print_rankings(v, top, "built-in demo data")


def _trade_fit_note(v: Valuator):
    """Per-interval trade volume and fit, for the progress line.

    Differences the valuator's cumulative counters, so each line describes the
    interval it closes rather than the run so far — a run-so-far average would
    flatten out and stop showing whether trades are still moving values.
    """
    seen = {"trades": 0, "gap": 0.0}

    def note() -> str:
        trades = v.trades_applied - seen["trades"]
        gap = v.trade_abs_gap - seen["gap"]
        seen["trades"], seen["gap"] = v.trades_applied, v.trade_abs_gap
        if not trades:
            return "no trades"
        return f"{trades} trades, mean |gap| {gap / trades:,.0f}"

    return note


def _log_diagnostics(v: Valuator) -> None:
    """One block at the end describing what the model actually ended up with."""
    d = v.diagnostics()
    print("  model diagnostics:", flush=True)
    print(
        f"    beliefs {d['beliefs']} · with score evidence {d['scored']}"
        f" · never scored {d['never_scored']}",
        flush=True,
    )
    print(
        f"    value: top {d['value_top']:,.0f}"
        f" (≈ADP rank {curve_rank(d['value_top'], d['lam']):.0f} on its curve)"
        f" · median {d['value_p50']:,.0f}",
        flush=True,
    )
    print(
        f"    curve: ρ {d['rho']:,.0f} · λ {d['lam']:.4f}"
        f" (e-fold every {1 / d['lam']:.0f} picks)",
        flush=True,
    )
    print(
        f"    sd: median {d['sd_p50']:,.0f} · p90 {d['sd_p90']:,.0f}"
        f" · last evidence {d['last_ts'].date()}",
        flush=True,
    )
    print(
        f"    trade fit: {d['trades_applied']:,} trades"
        f" ({d['unbalanced_trades']:,} unbalanced, the only ones that identify"
        f" ρ), mean |gap| {d['trade_mean_abs_gap']:,.0f}",
        flush=True,
    )
    print(
        f"    |gap| spread: p50 {d['gap_p50']:,.0f} · p90 {d['gap_p90']:,.0f}"
        f" · p99 {d['gap_p99']:,.0f} · max {d['gap_max']:,.0f}",
        flush=True,
    )
    # The model treats every trade as fair. These two numbers say how much of
    # the value movement comes from trades that plainly are not: the share of
    # trades whose residual exceeds its own expected spread, and the share of
    # total movement they cause. A small count carrying a large share of the
    # movement is the case where rejecting outliers changes the answer.
    print(
        f"    outliers (|z| > {valuation.OUTLIER_Z:g}):"
        f" {d['outlier_trades']:,} trades = {100 * d['outlier_share']:.1f}%"
        f" of trades, causing {100 * d['outlier_move_share']:.1f}% of all"
        f" value movement · z p50 {d['z_p50']:.2f} p99 {d['z_p99']:.2f}",
        flush=True,
    )

    # Positions the weekly-score query cannot return (it filters to the
    # fantasy set), so these beliefs can never receive performance evidence:
    # they enter through trades and keep whatever the trade stream implies.
    unscoreable = {
        pos: n
        for pos, n in sorted(d["by_position"].items())
        if pos not in db.FANTASY_POSITIONS
    }
    if unscoreable:
        census = ", ".join(f"{pos} {n}" for pos, n in unscoreable.items())
        print(
            f"    never scoreable ({sum(unscoreable.values())} beliefs"
            f" outside the fantasy positions): {census}",
            flush=True,
        )


def _resolve_curve(
    segment_key: str,
    inputs,
    events: list[dict],
    start: date,
    end: date,
    step: timedelta,
    rho_pin: float | None,
    lam_pin: float | None,
) -> tuple[float, float]:
    """Settle the two curve parameters for this run.

    λ is taken as given — from --lam, else the seed default. It is deliberately
    not fitted: trade fairness has a global optimum at a flat curve (see note 7
    in src/valuation.py), so minimizing trade residuals over λ just walks to
    zero. ρ is fitted, because unbalanced trades do identify it.

    Reported in full — the iteration path, the identifying sample size, and the
    raw pre-clamp value — because a ρ fitted from a thin sample deserves less
    trust than the seed it replaced, and the log is the only place that
    difference is visible.
    """
    lam = LAMBDA_ADP if lam_pin is None else lam_pin
    if lam_pin is not None:
        print(f"  λ pinned at {lam:.4f} (never fitted; see note 7)", flush=True)

    if rho_pin is not None:
        print(f"  ρ pinned at {rho_pin:,.0f} (not fitted)", flush=True)
        return rho_pin, lam

    fit = fit_rho(
        lambda r: _seeded_valuator(
            segment_key, inputs.adp, start, inputs.players, r, lam
        ),
        events,
        start,
        end,
        step,
        seed=RHO,
    )
    if not fit.unbalanced:
        print(
            "  ρ not identifiable — no unbalanced trades in window;"
            f" keeping the seed {fit.rho:,.0f}",
            flush=True,
        )
        return fit.rho, lam

    path = " -> ".join(f"{r:,.0f}" for r in fit.path)
    print(
        f"  ρ fitted to {fit.rho:,.0f} from {fit.unbalanced:,} unbalanced"
        f" trades (seed {RHO:,.0f}; {path};"
        f" {'converged' if fit.converged else 'did NOT converge'})",
        flush=True,
    )
    if fit.raw < 0:
        print(
            f"    note: the raw estimate was {fit.raw:,.0f} — the data asked"
            " for a negative replacement value, which was clamped to 0",
            flush=True,
        )
    return fit.rho, lam


def _seeded_valuator(
    segment_key: str,
    adp,
    start: date,
    players=None,
    rho: float | None = None,
    lam: float | None = None,
) -> Valuator:
    """players: the full resolved identity map, so a player who only ever shows
    up inside a trade still gets their real name and position."""
    v = Valuator(
        start_ts=datetime.combine(start, datetime.min.time()),
        repl_rank_by_pos=SEGMENTS[segment_key].repl_rank_by_pos,
        identities={
            pid: (p.name, p.position) for pid, p in (players or {}).items()
        },
        rho=rho,
        lam=lam,
    )
    v.seed_from_adp(adp_frame(adp))
    return v


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

    events = build_events(inputs.trades, inputs.scores, SEASONS[season])
    resolved_rho, resolved_lam = _resolve_curve(
        segment_key, inputs, events, start, end, step,
        manifest.get("rho"), manifest.get("lam"),
    )
    v = _seeded_valuator(
        segment_key, inputs.adp, start, inputs.players, resolved_rho, resolved_lam
    )
    reporter = progress.ProgressReporter(
        progress.total_batches(start, end, step), extra=_trade_fit_note(v)
    )
    stats = replay(
        v, events, start, end, step, on_snapshot=lambda d, df: reporter.tick(d)
    )
    print(f"  {stats.snapshots} snapshots, {stats.events_applied} events applied")
    _log_diagnostics(v)
    _print_rankings(v, top, f"{segment_key} season {season} (staged bundle)")


def run_replay(
    segment_key: str,
    season: str,
    start: date,
    end: date,
    step: timedelta,
    top: int,
    rho: float | None = None,
    lam: float | None = None,
) -> None:
    segment = SEGMENTS[segment_key]
    season_dates = SEASONS[season]
    validate_step(start, end, step)

    # --start is both the input window's lower bound and the model clock's
    # origin, so a later start would seed from ADP and then silently skip every
    # trade and score before it — publishing snapshots, and replacing
    # valuation_state, with a model that never saw that evidence. (replay()'s
    # dropped_before_start counter cannot catch this: load_inputs already
    # windows its queries to [start, end), so there is nothing left to drop.)
    # Supporting it properly means replaying from the draft date and only
    # publishing the requested window, which is the incremental behavior this
    # CLI deliberately does not have.
    if start != season_dates.draft_date:
        sys.exit(
            f"--start must be the {season} draft date"
            f" ({season_dates.draft_date}), got {start}. This CLI replays a"
            " season in full; a later start would omit every event before it"
            " while still rewriting the model state."
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

            events = build_events(inputs.trades, inputs.scores, season_dates)
            resolved_rho, resolved_lam = _resolve_curve(
                segment.key, inputs, events, start, end, step, rho, lam
            )

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
                    "rho": resolved_rho,
                    "lam": resolved_lam,
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

            v = _seeded_valuator(
                segment.key, inputs.adp, start, inputs.players,
                resolved_rho, resolved_lam,
            )

            db.delete_snapshots(sources.cloud, segment.key, start, end)

            reporter = progress.ProgressReporter(total, extra=_trade_fit_note(v))

            def on_snapshot(day: date, rankings: pd.DataFrame) -> None:
                db.write_snapshot(sources.cloud, segment.key, day, rankings)
                reporter.tick(day)

            stats = replay(v, events, start, end, step, on_snapshot)

            db.save_state(sources.cloud, segment.key, v.to_state())
            db.save_run(
                sources.cloud,
                RunState(
                    segment=segment.key,
                    season=season,
                    last_event_ts=v.last_ts,
                    last_transaction_created=max(
                        (t.created_ms for t in inputs.trades), default=0
                    ),
                    last_week_processed=max(
                        (s.week for s in inputs.scores), default=0
                    ),
                ),
            )
            sources.cloud.commit()
        except BaseException:
            # BaseException, not Exception: SystemExit (the lock-contention
            # path above) must roll back too, which is also what releases the
            # transaction-scoped lock.
            sources.cloud.rollback()
            raise

    rows = stats.snapshots * len(v.beliefs)
    print(
        f"  wrote {stats.snapshots} daily snapshots (~{rows} rows),"
        f" {stats.events_applied} events applied"
    )
    if stats.dropped_before_start or stats.dropped_at_or_after_end:
        print(
            f"  ignored {stats.dropped_before_start} events before {start} and"
            f" {stats.dropped_at_or_after_end} at/after {end}"
        )
    print(f"  elapsed {time.monotonic() - started:.1f}s")
    _log_diagnostics(v)
    _print_rankings(v, top, f"{segment.key} season {season} (database)")


def main(argv: list[str] | None = None) -> None:
    ap = build_parser()
    args = ap.parse_args(argv)

    if args.demo:
        run_demo(args.top)
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
    run_replay(
        args.segment, args.season, args.start, end, args.step, args.top,
        args.rho, args.lam,
    )


if __name__ == "__main__":
    main()
