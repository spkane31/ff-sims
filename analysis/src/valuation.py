"""
Player valuation model — a recursive-belief estimator.

Every player carries a *belief*: a best guess plus an uncertainty (variance).
Three kinds of evidence update that belief, and one operation ("fuse") does all
the updating:

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

See the NOTES at the bottom for known simplifications and how to extend.
"""

from __future__ import annotations

import math
from dataclasses import dataclass
from datetime import datetime

import pandas as pd

from .models import PlayerBeliefState

# ----------------------------------------------------------------------------- #
# CONFIG  — tune all of this per segment/league. Comments give the intuition.
# ----------------------------------------------------------------------------- #

V_TOP = 10_000.0  # value of the #1 player (top of the curve)
LAMBDA_ADP = 0.04  # curve steepness: value drops ~e-fold every 1/λ ≈ 25 picks

# Replacement value ρ: the floor a roster spot is worth. Used in the trade math
# (unbalanced trades) and to compute VORP = value - ρ. Here we read it off the
# curve at a "replacement rank". Estimating ρ jointly from trades is a good
# extension (see NOTES).
# 12-team superflex starting guesses — tune against held-out trades later.
RHO_RANK = 160
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

# Weekly positional replacement ranks (12-team superflex starting guesses).
# The Nth-best scorer at a position that week is "replacement"; PAR = points
# minus that score.
REPL_RANK_BY_POS = {"QB": 24, "RB": 30, "WR": 36, "TE": 12, "DEF": 12, "K": 12}


def curve(rank: float) -> float:
    """ADP/performance rank -> value on the additive 0..V_TOP scale."""
    return V_TOP * math.exp(-LAMBDA_ADP * (rank - 1.0))


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

    def __init__(self, start_ts: datetime) -> None:
        self.beliefs: dict[str, Belief] = {}
        self.last_ts: datetime = start_ts

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

    # -- 1. SEED from ADP -----------------------------------------------------
    def seed_from_adp(self, adp: pd.DataFrame) -> None:
        """adp columns: player_id, player_name, position, adp.
        Idempotent: players already tracked keep their current belief."""
        for row in adp.itertuples(index=False):
            if row.player_id in self.beliefs:
                continue
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

    # -- persistence: round-trip beliefs through valuation_state ------------------
    def to_state(self) -> list[PlayerBeliefState]:
        return [
            PlayerBeliefState(
                player_id=pid, guess=b.guess, var=b.var, games=b.games,
                cum_par=b.cum_par, position=b.position, name=b.name,
            )
            for pid, b in self.beliefs.items()
        ]

    @classmethod
    def from_state(
        cls, states: list[PlayerBeliefState], last_ts: datetime
    ) -> "Valuator":
        v = cls(start_ts=last_ts)
        for s in states:
            v.beliefs[s.player_id] = Belief(
                guess=s.guess, var=s.var, position=s.position or "DEFAULT",
                name=s.name or "", games=s.games, cum_par=s.cum_par,
            )
        return v

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
