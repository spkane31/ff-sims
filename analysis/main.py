"""
Player valuation CLI.

The model lives in src/valuation.py (recursive-belief estimator over ADP,
trades, and weekly scores). This file loads data (demo or CSVs) and runs it.

RUN
---
    python main.py                 # runs on built-in demo data
    python main.py --data ./mydata # runs on your CSVs (schemas below)

INPUT CSV SCHEMAS (put these in the --data folder)
--------------------------------------------------
    draft_picks.csv : player_id, player_name, position, pick_no [, draft_id]
                      ADP is computed as the mean pick_no per player across drafts.
    trades.csv      : trade_id, timestamp, side, player_id
                      `side` groups the two baskets (e.g. A/B, or two roster ids).
                      `timestamp` is ISO ("2025-10-03") or unix seconds.
    scores.csv      : week, player_id, points
                      `points` already in THIS segment's scoring (e.g. PPR).
    players.csv     : player_id, player_name, position   (optional metadata)
"""

from __future__ import annotations

import argparse
from datetime import datetime, timedelta
from pathlib import Path

import numpy as np
import pandas as pd

from src.config import SEASONS, week_ts
from src.valuation import RHO, V_TOP, Valuator, curve

SEASON_2025 = SEASONS["2025"]


# ----------------------------------------------------------------------------- #
# DATA LOADING  (real CSVs) and a self-contained DEMO generator
# ----------------------------------------------------------------------------- #


def load_real(data_dir: Path) -> tuple[pd.DataFrame, list[dict]]:
    picks = pd.read_csv(data_dir / "draft_picks.csv")
    adp = (
        picks.groupby(["player_id", "player_name", "position"], as_index=False)[
            "pick_no"
        ]
        .mean()
        .rename(columns={"pick_no": "adp"})
    )

    pos_map = dict(zip(picks["player_id"], picks["position"]))

    events: list[dict] = []

    trades = pd.read_csv(data_dir / "trades.csv")
    trades["ts"] = pd.to_datetime(trades["timestamp"], errors="coerce", unit=None)
    # unix-seconds fallback
    mask = trades["ts"].isna()
    if mask.any():
        trades.loc[mask, "ts"] = pd.to_datetime(trades.loc[mask, "timestamp"], unit="s")
    for tid, grp in trades.groupby("trade_id"):
        sides = sorted(grp["side"].astype(str).unique())
        if len(sides) != 2:
            continue  # skip anything that isn't a clean two-sided trade
        a = grp.loc[grp["side"].astype(str) == sides[0], "player_id"].tolist()
        b = grp.loc[grp["side"].astype(str) == sides[1], "player_id"].tolist()
        events.append(
            {
                "ts": grp["ts"].max().to_pydatetime(),
                "kind": "trade",
                "side_a": a,
                "side_b": b,
            }
        )

    scores = pd.read_csv(data_dir / "scores.csv")
    scores["position"] = scores["player_id"].map(pos_map).fillna("DEFAULT")
    for week, grp in scores.groupby("week"):
        events.append(
            {
                "ts": week_ts(SEASON_2025, int(week)),
                "kind": "week",
                "scores": grp[["player_id", "position", "points"]].copy(),
            }
        )

    return adp, events


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
# MAIN
# ----------------------------------------------------------------------------- #


def main() -> None:
    ap = argparse.ArgumentParser(description="Single-segment player valuation.")
    ap.add_argument(
        "--data",
        type=str,
        default=None,
        help="folder with draft_picks.csv, trades.csv, scores.csv",
    )
    ap.add_argument("--top", type=int, default=30, help="how many players to print")
    args = ap.parse_args()

    if args.data:
        adp, events = load_real(Path(args.data))
        source = f"real data in {args.data}"
    else:
        adp, events = make_demo()
        source = "built-in demo data"

    v = Valuator(
        start_ts=datetime.combine(SEASON_2025.draft_date, datetime.min.time())
    )
    v.seed_from_adp(adp)  # the exp curve, applied once
    v.advance(events)  # trades + weekly scores, in time order, with aging

    print(f"\nPlayer valuations  ({source})")
    print(f"ρ (replacement) = {RHO:.0f}   |   top of curve = {V_TOP:.0f}\n")
    print(v.rankings().head(args.top).to_string())
    print(
        "\nvalue = current belief (additive scale) | vorp = value - ρ | sd = uncertainty band\n"
    )


if __name__ == "__main__":
    main()
