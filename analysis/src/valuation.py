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
ADP_VAR = 1_500_000.0           # prior uncertainty from the ADP seed  (sd ~1225)
TRADE_VAR = 3_000_000.0         # noise in a single trade              (sd ~1730)
WEEK_VAR_BASE = 4_000_000.0     # noise in a performance reading, divided by games seen
MAX_VAR = 9_000_000.0           # cap so long-inactive players don't blow up
UNSEEN_VAR = ADP_VAR * 3        # a player who appears with no ADP (UDFA, call-up)

# Residual size, in units of its own expected spread, past which a trade is
# counted as an outlier for diagnostics. 2 is the conventional gate for a
# robust (Huber) update; nothing acts on it yet.
OUTLIER_Z = 2.0

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

def curve(rank: float) -> float:
    """ADP/performance rank -> value on the additive 0..V_TOP scale."""
    return V_TOP * math.exp(-LAMBDA_ADP * (rank - 1.0))


def curve_rank(value: float, lam: float = LAMBDA_ADP) -> float:
    """Inverse of curve(): what draft rank a value corresponds to.

    Reported alongside the top value so "4561" reads as "the model values its
    best player like the ~20th pick", which is the form the number is
    actually interpretable in. Takes the steepness so it inverts the curve the
    run actually used, not the seed default.
    """
    if value <= 0:
        return float("inf")
    return 1.0 - math.log(value / V_TOP) / lam


def _percentile(ordered: list[float], pct: float) -> float:
    """Nearest-rank percentile over an already-sorted list."""
    if not ordered:
        return 0.0
    idx = min(len(ordered) - 1, int(round(pct / 100.0 * (len(ordered) - 1))))
    return ordered[idx]


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
    # Raw count of trades this player appeared in. Unlike `games` this is not
    # decayed: it answers "how much market evidence is this belief built on",
    # which is a question about the whole run, not about recent form.
    trades: int = 0


class Valuator:
    """Holds all player beliefs and advances them through a stream of events."""

    def __init__(
        self,
        start_ts: datetime,
        repl_rank_by_pos: dict[str, int],
        identities: dict[str, tuple[str, str]] | None = None,
        rho: float | None = None,
        lam: float | None = None,
    ) -> None:
        """repl_rank_by_pos: weekly replacement rank per position for the
        league combo being valued (each Segment in src/config.py defines its
        own — the Nth-best scorer at a position is "replacement").

        identities: player_id -> (name, position) for every player the run can
        touch, not just the ADP-seeded ones. Trades carry bare player IDs, so
        without this a player who only ever appears inside a trade gets a
        nameless DEFAULT belief: wrong drift rate, a bogus DEFAULT position
        group for pos_rank, and a DEFAULT row published to player_valuations.
        """
        self.beliefs: dict[str, Belief] = {}
        self.last_ts: datetime = start_ts
        self.repl_rank_by_pos = dict(repl_rank_by_pos)
        self.identities = dict(identities or {})
        # Trade fit: how far each trade was from what the model already
        # believed, accumulated. Falling mean |gap| is the market and the
        # model converging; a flat one means trades keep saying something the
        # values never absorb. Cumulative so a caller can difference it over
        # any window it likes.
        self.trades_applied = 0
        self.trade_abs_gap = 0.0
        # Curve steepness. Settable so a run can try a different shape, but
        # NOT fitted from trades: a flat curve (lam -> 0, rho -> the common
        # value) satisfies every trade constraint exactly, so trade fairness
        # has a global optimum at "everybody is worth the same" and cannot
        # identify the shape. See the NOTES at the bottom.
        self.lam = LAMBDA_ADP if lam is None else lam
        self.rho = RHO if rho is None else rho
        # Sufficient statistics for that fit. The trade rule only ever uses ρ
        # as ρ·(|B|−|A|), so a balanced trade says nothing about it: n = 0
        # contributes nothing to either sum, which is exactly why only
        # unbalanced trades identify ρ.
        self.rho_dn = 0.0  # Σ d·n, d = value gap, n = size gap
        self.rho_nn = 0.0  # Σ n²
        self.unbalanced_trades = 0
        # Per-trade residuals, kept whole rather than summarised on the fly:
        # the question is the shape of the tail, and a mean cannot answer it.
        # ~40k floats per run, rebuilt per fitting pass, so nothing accumulates.
        self.trade_gaps: list[float] = []
        self.trade_zs: list[float] = []
        self.trade_move_total = 0.0  # total value movement caused by trades
        self.trade_move_outlier = 0.0  # ...of which, from |z| > OUTLIER_Z
        self.outlier_trades = 0

    def _curve(self, rank: float) -> float:
        """This run's rank -> value curve. Same shape as the module-level
        curve(), but at the steepness this run is using."""
        return V_TOP * math.exp(-self.lam * (rank - 1.0))

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
        if not position or not name:
            known_name, known_pos = self.identities.get(pid, ("", ""))
            position = position or known_pos
            name = name or known_name
        b = self.beliefs.get(pid)
        if b is None:
            b = Belief(
                guess=self._curve(90),
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
                guess=self._curve(row.adp),
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

    def age_to(self, now: datetime) -> None:
        """Advance the model clock with no evidence: uncertainty grows, values
        do not move. A fixed-step replay calls this at every batch boundary so
        a quiet day's snapshot still shows drift."""
        self._age(now)

    # -- 2. UPDATE from a trade (additive constraint across several players) ------
    def apply_trade(self, side_a: list[str], side_b: list[str]) -> None:
        a = [self._ensure(p) for p in side_a]
        b = [self._ensure(p) for p in side_b]
        if not a or not b:
            return

        pred_a = sum(x.guess for x in a)
        pred_b = sum(x.guess for x in b)
        # fair-trade rule:  value(A) = value(B) - ρ * (|B| - |A|)
        # Equivalently, total VORP is conserved across the sides: each roster
        # spot a side gives up is worth at least replacement.
        size_gap = len(b) - len(a)
        value_gap = pred_b - pred_a
        target_a = pred_b - self.rho * size_gap
        gap = target_a - pred_a  # how wrong our values are

        total_var = sum(x.var for x in a + b)
        if total_var <= 0:
            return
        self.trades_applied += 1
        self.trade_abs_gap += abs(gap)
        if size_gap:
            # Least-squares accumulators for ρ̂ = Σ(d·n)/Σ(n²): the ρ that best
            # explains why the sides of unbalanced trades differ in value.
            self.rho_dn += value_gap * size_gap
            self.rho_nn += float(size_gap * size_gap)
            self.unbalanced_trades += 1
        for x in a + b:
            x.trades += 1

        k = total_var / (total_var + TRADE_VAR)  # gain on the summed constraint

        # -- outlier diagnostics ------------------------------------------------
        # The model treats every trade as fair. Real ones include dumps, panic
        # moves and rebuilds, and those enter as an equality constraint between
        # sides that are not equal. z is the residual in units of its own
        # expected spread, so it self-calibrates: early on, when variance is
        # high, a big gap is information; once the model is confident the same
        # gap is a bad trade. Recorded, not acted on — this measures how much
        # of the value movement comes from trades a robust update would reject.
        z = abs(gap) / math.sqrt(total_var + TRADE_VAR)
        # Shares sum to 1 across both sides, so k*|gap| is the total absolute
        # value movement this trade causes.
        move = k * abs(gap)
        self.trade_gaps.append(abs(gap))
        self.trade_zs.append(z)
        self.trade_move_total += move
        if z > OUTLIER_Z:
            self.outlier_trades += 1
            self.trade_move_outlier += move

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
            n = self.repl_rank_by_pos.get(pos, 24)
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
            obs = self._curve(perf_rank[pid])
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

    # -- run diagnostics ---------------------------------------------------------
    def diagnostics(self) -> dict:
        """Model-health numbers for the end of a run log.

        These answer the questions a finished replay actually raises: did the
        trade stream converge, how much of the belief set rests on real
        performance evidence, and how wide is the remaining uncertainty. All
        derived from current state — no extra bookkeeping during the replay.
        """
        beliefs = list(self.beliefs.values())
        values = sorted(b.guess for b in beliefs)
        sds = sorted(math.sqrt(b.var) for b in beliefs)
        by_position: dict[str, int] = {}
        for b in beliefs:
            by_position[b.position] = by_position.get(b.position, 0) + 1
        scored = sum(1 for b in beliefs if b.games > 0)
        return {
            "beliefs": len(beliefs),
            "scored": scored,
            "never_scored": len(beliefs) - scored,
            "value_top": values[-1] if values else 0.0,
            "value_p50": _percentile(values, 50),
            "sd_p50": _percentile(sds, 50),
            "sd_p90": _percentile(sds, 90),
            "rho": self.rho,
            "lam": self.lam,
            "unbalanced_trades": self.unbalanced_trades,
            "trades_applied": self.trades_applied,
            "gap_p50": _percentile(sorted(self.trade_gaps), 50),
            "gap_p90": _percentile(sorted(self.trade_gaps), 90),
            "gap_p99": _percentile(sorted(self.trade_gaps), 99),
            "gap_max": max(self.trade_gaps, default=0.0),
            "z_p50": _percentile(sorted(self.trade_zs), 50),
            "z_p99": _percentile(sorted(self.trade_zs), 99),
            "outlier_trades": self.outlier_trades,
            "outlier_share": (
                self.outlier_trades / self.trades_applied
                if self.trades_applied
                else 0.0
            ),
            # The decisive number: if a small share of trades carries a large
            # share of the movement, rejecting them changes the answer.
            "outlier_move_share": (
                self.trade_move_outlier / self.trade_move_total
                if self.trade_move_total > 0
                else 0.0
            ),
            "trade_mean_abs_gap": (
                self.trade_abs_gap / self.trades_applied
                if self.trades_applied
                else 0.0
            ),
            "by_position": by_position,
            "last_ts": self.last_ts,
        }

    # -- persistence: round-trip beliefs through valuation_state ------------------
    def to_state(self) -> list[PlayerBeliefState]:
        return [
            PlayerBeliefState(
                player_id=pid, guess=b.guess, var=b.var, games=b.games,
                cum_par=b.cum_par, position=b.position, name=b.name,
                trades=b.trades,
            )
            for pid, b in self.beliefs.items()
        ]

    @classmethod
    def from_state(
        cls,
        states: list[PlayerBeliefState],
        last_ts: datetime,
        repl_rank_by_pos: dict[str, int],
    ) -> "Valuator":
        v = cls(start_ts=last_ts, repl_rank_by_pos=repl_rank_by_pos)
        for s in states:
            v.beliefs[s.player_id] = Belief(
                guess=s.guess, var=s.var, position=s.position or "DEFAULT",
                name=s.name or "", games=s.games, cum_par=s.cum_par,
                trades=s.trades,
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
                    "vorp": round(max(0.0, b.guess - self.rho)),
                    "sd": round(math.sqrt(b.var)),  # uncertainty band half-width
                    "games": round(b.games, 1),
                    "trades": b.trades,
                }
            )
        df = (
            pd.DataFrame(rows)
            .sort_values("value", ascending=False)
            .reset_index(drop=True)
        )
        df["pos_rank"] = df.groupby("pos").cumcount() + 1
        df = df[
            [
                "player_id", "player", "pos", "pos_rank", "value", "vorp", "sd",
                "games", "trades",
            ]
        ]
        df.index += 1
        df.index.name = "rank"
        return df


# ----------------------------------------------------------------------------- #
# NOTES / KNOWN SIMPLIFICATIONS (honest list of what to improve)
# ----------------------------------------------------------------------------- #
# 1. ρ is now fitted from unbalanced trades (runner.fit_rho) rather than read
#    off the curve. On real data that cut mean |trade gap| by 38% (618 -> 385).
#    LAMBDA_ADP is NOT fitted, and cannot be from trades alone: see 7.
# 7. THE TRADE CONSTRAINT CANNOT IDENTIFY THE CURVE'S SHAPE, and it actively
#    pulls values together. Give every player the same value V and set ρ = V:
#    a 1-for-1 reads V == V, a 1-for-2 reads V == V + V - ρ, a 1-for-3 reads
#    V == 3V - 2ρ. All satisfied exactly. So "everybody is worth the same" is a
#    global optimum of trade fairness, and fitting λ by minimizing trade
#    residuals just walks to λ -> 0 (measured: the objective falls monotonically
#    all the way to the search floor).
#    This is also why the published top value sits near ADP rank 20 rather than
#    at V_TOP. Every trade nudges its two sides toward satisfying the sum rule,
#    which contracts the whole value distribution toward a common level around
#    ρ; the only things resisting are the ADP seed and the weekly PAR readings,
#    and with ~600 trades per top player against 18 weeks of scores, the trades
#    win. Raising ρ raises the level everything is pulled toward, which is why
#    fitting ρ moved the median up (117 -> 135) but barely moved the peak.
#    Fixing the compression means changing the trade update itself — e.g.
#    projecting out the common-mode component so a trade re-weights players
#    relative to each other without moving the overall scale — not fitting more
#    parameters to a system that is indifferent to scale. Until then, treat
#    published values as ordinal-plus-ratio, and set the scale from the seed.
# 2. Trade updates ignore CORRELATION between players. When A is traded for B, a
#    true Kalman filter records that their errors are now linked (in a covariance
#    matrix). The variance-share split here is a clean approximation that treats
#    each player independently — fine for point values, not exact for the trade
#    calculator's uncertainty bands.
# 3. Performance fuses the CUMULATIVE (decayed) PAR-rank each week. Combined with
#    weekly aging this tracks form reasonably, but it lightly re-uses information.
#    A cleaner design tracks a separate performance state with its own precision.
# 4. Positional replacement ranks and the points->value mapping are approximate.
#    PAR keeps performance position-aware; tune each Segment's repl_rank_by_pos
#    (src/config.py) to its league combo.
# 5. No injury shocks. To handle a season-ending injury, spike that player's var
#    (e.g. b.var = MAX_VAR) so the next evidence moves them hard and fast.
# 6. All variance/decay constants are starting guesses. The right way to set them
#    is to hold out ~20% of trades and tune the knobs to minimize
#    |sum(value side A) - sum(value side B)| on the held-out set.
