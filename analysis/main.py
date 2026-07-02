"""
Player valuation — single-segment starting point (e.g. PPR / 10-team / redraft).

WHAT THIS IS
------------
A recursive-belief valuation model. Every player carries a *belief*: a best
guess plus an uncertainty (variance). Three kinds of evidence update that
belief, and one operation ("fuse") does all the updating:

    1. DRAFT (ADP)      -> the initial guess, via an exponential rank curve.
    2. TRADES           -> a constraint that two baskets are roughly equal.
    3. WEEKLY SCORES    -> points-above-replacement, ranked and read off the curve.

Between updates, uncertainty grows ("age"), so beliefs go stale on their own and
new evidence moves stale beliefs more than fresh ones. There are NO hand-tuned
calendar blend weights: draft dominates early only because uncertainty is high
early, and fades on its own as trades/games accumulate.

Everything lives on ONE additive scale where the #1 player ~= 10,000. That is the
scale trades sum on. We never apply a second curve on top of it (exp(a)+exp(b)
!= exp(a+b)); the curving happens once, at the ADP->value seed.

RUN
---
    python valuation.py            # runs on built-in demo data
    python valuation.py --data ./mydata   # runs on your CSVs (schemas below)

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

Tune the CONFIG block for your league. This is a starting point, not a final model
— see the NOTES at the bottom for the known simplifications and how to extend.
"""

from __future__ import annotations

import argparse
import math
from dataclasses import dataclass, field
from datetime import date, datetime, timedelta
from pathlib import Path

import numpy as np
import pandas as pd

from src.db import get_adp, get_trades

# ----------------------------------------------------------------------------- #
# CONFIG  — tune all of this per segment/league. Comments give the intuition.
# ----------------------------------------------------------------------------- #

V_TOP = 10_000.0  # value of the #1 player (top of the curve)
LAMBDA_ADP = 0.04  # curve steepness: value drops ~e-fold every 1/λ ≈ 25 picks

# Replacement value ρ: the floor a roster spot is worth. Used in the trade math
# (unbalanced trades) and to compute VORP = value - ρ. Here we read it off the
# curve at a "replacement rank". Estimating ρ jointly from trades is a good
# extension (see NOTES).
RHO_RANK = 130
RHO = V_TOP * math.exp(-LAMBDA_ADP * (RHO_RANK - 1))

# Uncertainty knobs, expressed as VARIANCES on the 0..10,000 scale.
# (A belief's "give or take" is sqrt(var). sd 1000 -> var 1_000_000.)
ADP_VAR = 1_500_000.0  # prior uncertainty from the ADP seed  (sd ~1225)
TRADE_VAR = 3_000_000.0  # noise in a single trade              (sd ~1730)
WEEK_VAR_BASE = 4_000_000.0  # noise in a performance reading, divided by games seen
MAX_VAR = 9_000_000.0  # cap so long-inactive players don't blow up
UNSEEN_VAR = ADP_VAR * 3  # a player who appears with no ADP (UDFA, call-up)

# Performance signal.
PERF_DECAY = 0.85  # recency weight: last week matters more than week 1
PERF_N_CAP = 6.0  # cap effective games so performance can't get overconfident
DRIFT_PER_DAY = {  # variance added per day since a player's last evidence
    "QB": 700,
    "RB": 1600,
    "WR": 1300,
    "TE": 1000,
    "DEF": 900,
    "K": 900,
    "DEFAULT": 1200,
}

# Weekly positional replacement ranks (approx for a 10-team PPR). The Nth-best
# scorer at a position that week is "replacement"; PAR = points - that score.
REPL_RANK_BY_POS = {"QB": 12, "RB": 28, "WR": 32, "TE": 12, "DEF": 10, "K": 10}

# Calendar anchors (only used to place weekly scores on the same timeline as trades).
DRAFT_DATE = date(2025, 8, 25)  # when the initial ADP belief is seeded
SEASON_START = date(2025, 9, 4)  # NFL 2025 Week 1 kickoff (configurable)
SCORE_LAG_DAYS = 4  # week W scores land ~this many days after kickoff


def curve(rank: float) -> float:
    """ADP/performance rank -> value on the additive 0..V_TOP scale."""
    return V_TOP * math.exp(-LAMBDA_ADP * (rank - 1.0))


def week_to_date(week: int) -> date:
    return SEASON_START + timedelta(days=(week - 1) * 7 + SCORE_LAG_DAYS)


# ----------------------------------------------------------------------------- #
# THE BELIEF + THE VALUATOR
# ----------------------------------------------------------------------------- #


@dataclass
class Belief:
    guess: float
    var: float
    position: str = "DEFAULT"
    name: str = ""
    games: float = 0.0  # effective (decayed) games observed
    cum_par: float = 0.0  # decayed cumulative points-above-replacement


class Valuator:
    """Holds all player beliefs and advances them through a stream of events."""

    def __init__(self) -> None:
        self.beliefs: dict[str, Belief] = {}
        self.last_ts: datetime = datetime.combine(DRAFT_DATE, datetime.min.time())

    # -- the single update primitive: trust-weighted blend of guess and evidence --
    @staticmethod
    def _fuse(
        guess: float, var: float, obs: float, obs_var: float
    ) -> tuple[float, float]:
        k = var / (var + obs_var)  # gain in [0,1]: how far to move toward obs
        new_guess = guess + k * (obs - guess)
        new_var = (1.0 - k) * var  # evidence always shrinks uncertainty
        return max(0.0, new_guess), new_var

    def _drift(self, pos: str) -> float:
        return DRIFT_PER_DAY.get(pos, DRIFT_PER_DAY["DEFAULT"])

    def _ensure(self, pid: str, position: str | None = None, name: str = "") -> Belief:
        """Fetch a belief, creating a wide-uncertainty one for players we've never seen."""
        b = self.beliefs.get(pid)
        if b is None:
            b = Belief(
                guess=curve(90),
                var=UNSEEN_VAR,  # seed low, very unsure
                position=(position or "DEFAULT"),
                name=name,
            )
            self.beliefs[pid] = b
        else:
            if position and b.position == "DEFAULT":
                b.position = position
            if name and not b.name:
                b.name = name
        return b

    # -- 1. SEED from ADP (runs once) --------------------------------------------
    def seed_from_adp(self, adp: pd.DataFrame) -> None:
        """adp columns: player_id, player_name, position, adp"""
        for row in adp.itertuples(index=False):
            self.beliefs[row.player_id] = Belief(
                guess=curve(row.adp),
                var=ADP_VAR,
                position=row.position,
                name=row.player_name,
            )

    # -- PREDICT: age every belief forward by dt days ----------------------------
    def _age(self, now: datetime) -> None:
        dt_days = max(0.0, (now - self.last_ts).total_seconds() / 86400.0)
        if dt_days == 0.0:
            return
        for b in self.beliefs.values():
            b.var = min(MAX_VAR, b.var + self._drift(b.position) * dt_days)
        self.last_ts = now

    # -- 2. UPDATE from a trade (additive constraint across several players) ------
    def apply_trade(self, side_a: list[str], side_b: list[str]) -> None:
        a = [self._ensure(p) for p in side_a]
        b = [self._ensure(p) for p in side_b]
        if not a or not b:
            return

        pred_a = sum(x.guess for x in a)
        pred_b = sum(x.guess for x in b)
        # fair-trade rule:  value(A) = value(B) - ρ * (|B| - |A|)
        target_a = pred_b - RHO * (len(b) - len(a))
        gap = target_a - pred_a  # how wrong our values are

        total_var = sum(x.var for x in a + b)
        if total_var <= 0:
            return
        k = total_var / (total_var + TRADE_VAR)  # gain on the summed constraint

        # spread the correction across players, weighted by how unsure we were:
        # the uncertain players absorb most of the fix, the confident ones barely move.
        for x in a:
            share = x.var / total_var
            x.guess = max(0.0, x.guess + k * gap * share)
            x.var *= 1.0 - k * share
        for x in b:
            share = x.var / total_var
            x.guess = max(0.0, x.guess - k * gap * share)
            x.var *= 1.0 - k * share

    # -- 3. UPDATE from a week of scores (points -> PAR -> rank -> value) ---------
    def apply_week(self, week_scores: pd.DataFrame) -> None:
        """week_scores columns: player_id, position, points"""
        if week_scores.empty:
            return

        # positional replacement level = Nth-best score at each position this week
        repl: dict[str, float] = {}
        for pos, grp in week_scores.groupby("position"):
            pts = grp["points"].sort_values(ascending=False).to_numpy()
            n = REPL_RANK_BY_POS.get(pos, 24)
            repl[pos] = (
                float(pts[n - 1])
                if len(pts) >= n
                else float(pts.min() if len(pts) else 0.0)
            )

        # update each playing player's decayed PAR and effective game count
        played: list[str] = []
        for row in week_scores.itertuples(index=False):
            b = self._ensure(row.player_id, position=row.position)
            par = float(row.points) - repl.get(row.position, 0.0)
            b.cum_par = b.cum_par * PERF_DECAY + par
            b.games = b.games * PERF_DECAY + 1.0
            played.append(row.player_id)

        # rank ALL players who have performance data by decayed PAR -> value on the curve
        perf = [(pid, bel) for pid, bel in self.beliefs.items() if bel.games > 0]
        perf.sort(key=lambda t: t[1].cum_par, reverse=True)
        perf_rank = {pid: i + 1 for i, (pid, _) in enumerate(perf)}

        # fuse the performance-implied value into the players who played this week
        for pid in played:
            b = self.beliefs[pid]
            obs = curve(perf_rank[pid])
            obs_var = WEEK_VAR_BASE / min(b.games, PERF_N_CAP)  # more games -> tighter
            b.guess, b.var = self._fuse(b.guess, b.var, obs, obs_var)

    # -- the batch cadence: advance through a time-ordered list of events ---------
    def advance(self, events: list[dict]) -> None:
        """Each event: {'ts': datetime, 'kind': 'trade'|'week', ...payload}.
        Re-runnable: call with only the new events since the last tick."""
        for ev in sorted(events, key=lambda e: e["ts"]):
            self._age(ev["ts"])
            if ev["kind"] == "trade":
                self.apply_trade(ev["side_a"], ev["side_b"])
            elif ev["kind"] == "week":
                self.apply_week(ev["scores"])

    # -- read out current valuations at any time ---------------------------------
    def rankings(self) -> pd.DataFrame:
        rows = []
        for pid, b in self.beliefs.items():
            rows.append(
                {
                    "player_id": pid,
                    "player": b.name or pid,
                    "pos": b.position,
                    "value": round(b.guess),
                    "vorp": round(max(0.0, b.guess - RHO)),
                    "sd": round(math.sqrt(b.var)),  # uncertainty band half-width
                    "games": round(b.games, 1),
                }
            )
        df = (
            pd.DataFrame(rows)
            .sort_values("value", ascending=False)
            .reset_index(drop=True)
        )
        df.index += 1
        df.index.name = "rank"
        return df


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
        ts = datetime.combine(week_to_date(int(week)), datetime.min.time())
        events.append(
            {
                "ts": ts,
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
        ts = datetime.combine(week_to_date(week), datetime.min.time())
        events.append({"ts": ts, "kind": "week", "scores": wk})

    # ~15 roughly-fair trades scattered across the season
    ids = pdf["player_id"].to_numpy()
    tv = dict(zip(pdf["player_id"], pdf["true_value"]))
    for _ in range(15):
        anchor = rng.choice(ids)
        partners = rng.choice(ids, size=2, replace=False)
        # keep it plausible: anchor for two players of similar combined value
        events.append(
            {
                "ts": datetime.combine(
                    SEASON_START + timedelta(days=int(rng.integers(5, 95))),
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

    v = Valuator()
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


# ----------------------------------------------------------------------------- #
# NOTES / KNOWN SIMPLIFICATIONS (honest list of what to improve)
# ----------------------------------------------------------------------------- #
# 1. ρ is a fixed parameter here (read off the curve). The principled version
#    ESTIMATES ρ from unbalanced trades — those are what pin the replacement floor.
#    That needs a small joint least-squares pass; this recursive version treats ρ
#    as given. Start by tuning RHO_RANK, then upgrade.
# 2. Trade updates ignore CORRELATION between players. When A is traded for B, a
#    true Kalman filter records that their errors are now linked (in a covariance
#    matrix). The variance-share split here is a clean approximation that treats
#    each player independently — fine for point values, not exact for the trade
#    calculator's uncertainty bands.
# 3. Performance fuses the CUMULATIVE (decayed) PAR-rank each week. Combined with
#    weekly aging this tracks form reasonably, but it lightly re-uses information.
#    A cleaner design tracks a separate performance state with its own precision.
# 4. Positional replacement ranks and the points->value mapping are approximate.
#    PAR keeps performance position-aware; tune REPL_RANK_BY_POS to your league.
# 5. No injury shocks. To handle a season-ending injury, spike that player's var
#    (e.g. b.var = MAX_VAR) so the next evidence moves them hard and fast.
# 6. All variance/decay constants are starting guesses. The right way to set them
#    is to hold out ~20% of trades and tune the knobs to minimize
#    |sum(value side A) - sum(value side B)| on the held-out set.
