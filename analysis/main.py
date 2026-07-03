"""
Player valuation CLI.

The model lives in src/valuation.py (recursive-belief estimator over ADP,
trades, and weekly scores). Data comes from the Sleeper tables in Postgres
(see src/db.py; DATABASE_URL in analysis/.env).

RUN
---
    python main.py --demo                    # synthetic data, no DB
    python main.py --season 2025 --backtest  # full replay, rewrites snapshots
    python main.py --season 2025             # incremental run (default mode)
"""

from __future__ import annotations

import argparse
import sys
from datetime import date, datetime, timedelta

import numpy as np
import pandas as pd

from src import db
from src.config import PPR_SF_12, SEASONS, week_ts
from src.models import RunState
from src.runner import adp_frame, build_events, filter_stale, run_backtest
from src.valuation import RHO, V_TOP, Valuator, curve

SEASON_2025 = SEASONS["2025"]


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
# RUN MODES
# ----------------------------------------------------------------------------- #


def _print_rankings(v: Valuator, top: int, source: str) -> None:
    print(f"\nPlayer valuations  ({source})")
    print(f"ρ (replacement) = {RHO:.0f}   |   top of curve = {V_TOP:.0f}\n")
    print(v.rankings().head(top).to_string())
    print(
        "\nvalue = current belief (additive scale) | vorp = value - ρ"
        " | sd = uncertainty band\n"
    )


def run_demo(top: int) -> None:
    adp, events = make_demo()
    v = Valuator(
        start_ts=datetime.combine(SEASON_2025.draft_date, datetime.min.time())
    )
    v.seed_from_adp(adp)
    v.advance(events)
    _print_rankings(v, top, "built-in demo data")


def run_db(season: str, backtest: bool, top: int) -> None:
    segment = PPR_SF_12
    season_dates = SEASONS[season]
    conn = db.get_connection()
    try:
        run = db.get_run(conn, segment.key, season)
        state = db.load_state(conn, segment.key)
        bootstrap = backtest or run is None or not state

        if bootstrap:
            print(f"[{segment.key}/{season}] full backtest replay")
            adp = db.get_adp(conn, segment, season)
            trades = db.get_trades(conn, segment, season, since_created=0)
            scores = db.get_weekly_scores(conn, season, after_week=0)
            print(
                f"  inputs: {len(adp)} ADP players, {len(trades)} trades,"
                f" {len(scores)} weekly score rows"
            )
            if not adp:
                sys.exit("no ADP data for this segment/season — nothing to seed")

            v = Valuator(
                start_ts=datetime.combine(season_dates.draft_date, datetime.min.time()),
                repl_rank_by_pos=segment.repl_rank_by_pos,
            )
            v.seed_from_adp(adp_frame(adp))
            events = build_events(trades, scores, season_dates)

            # rewrite the season's snapshot range (rerunnable as backlog lands)
            db.delete_snapshots(
                conn, segment.key,
                season_dates.draft_date,
                season_dates.draft_date + timedelta(days=365),
            )
            snap_count = 0

            def on_snapshot(day: date, rankings) -> None:
                nonlocal snap_count
                db.write_snapshot(conn, segment.key, day, rankings)
                snap_count += 1

            run_backtest(v, events, on_snapshot)
            print(f"  wrote {snap_count} daily snapshots")
        else:
            print(f"[{segment.key}/{season}] incremental run")
            adp = db.get_adp(conn, segment, season)
            trades = db.get_trades(
                conn, segment, season, since_created=run.last_transaction_created
            )
            scores = db.get_weekly_scores(
                conn, season, after_week=run.last_week_processed
            )
            v = Valuator.from_state(
                state,
                last_ts=run.last_event_ts,
                repl_rank_by_pos=segment.repl_rank_by_pos,
            )
            v.seed_from_adp(adp_frame(adp))  # late-arriving draftees only
            events = build_events(trades, scores, season_dates)
            fresh, skipped = filter_stale(events, run.last_event_ts)
            if skipped:
                print(
                    f"  WARNING: skipped {skipped} events at/before model clock"
                    f" {run.last_event_ts} (out-of-order arrivals)"
                )
            print(f"  applying {len(fresh)} new events")
            v.advance(fresh)
            db.write_snapshot(conn, segment.key, date.today(), v.rankings())

        max_created = max(
            [t.created_ms for t in trades],
            default=(0 if bootstrap else run.last_transaction_created),
        )
        max_week = max(
            [s.week for s in scores],
            default=(0 if bootstrap else run.last_week_processed),
        )
        db.save_state(conn, segment.key, v.to_state())
        db.save_run(
            conn,
            RunState(
                segment=segment.key, season=season, last_event_ts=v.last_ts,
                last_transaction_created=max_created, last_week_processed=max_week,
            ),
        )
        conn.commit()
        _print_rankings(v, top, f"{segment.key} season {season} (database)")
    except Exception:
        conn.rollback()
        raise
    finally:
        conn.close()


def main() -> None:
    ap = argparse.ArgumentParser(description="Single-segment player valuation.")
    ap.add_argument("--demo", action="store_true", help="run on synthetic demo data")
    ap.add_argument("--backtest", action="store_true",
                    help="full season replay, rewriting all dated snapshots")
    ap.add_argument("--season", default="2025", choices=sorted(SEASONS))
    ap.add_argument("--top", type=int, default=30, help="how many players to print")
    args = ap.parse_args()

    if args.demo:
        run_demo(args.top)
    else:
        run_db(args.season, args.backtest, args.top)


if __name__ == "__main__":
    main()
