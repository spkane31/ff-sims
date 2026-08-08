"""Performance signal v2: magnitude-preserving PAR, universal decay, and a
projected-performance proxy kept strictly separate from market value.

What changed from the old model (see PLAYER_VALUATION_MODEL_REVIEW.md):

* PAR keeps its magnitude. The old model converted cumulative PAR to an
  overall rank and read a curve value off it, so a shrinking lead still fused
  as "rank 1 = 10,000" week after week — a breakout followed by six zero
  weeks *rose* from 3,752 to 9,032.
* Every tracked player decays at every week boundary — byes, injuries,
  inactives, and missing score rows included. The old model only decayed
  players present in the current score frame, so absence froze form.
* The output is `projected_par`, a rest-of-season production proxy blending
  a preseason expectation with observed form. It never touches
  `market_value`; the two disagreeing is signal (a trade target), not error.
* `sd` is replaced by two named quantities: `market_dispersion` (in
  market_value.py — robust spread of recent implied trade values) and
  `projection_uncertainty` here — an error band for projected PAR.

Replacement is computed at query time, per position, from the league's own
rosters and available players. There is no global rho anywhere in this
module, and no VORP from `value - rho`.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .models import WeeklyScore

PERF_DECAY = 0.85  # week-over-week recency weight on observed PAR
PRESEASON_GAMES = 4.0  # preseason expectation counts as this many games
# Prior variance of a weekly PAR reading before any games are observed, in
# (points above replacement)^2. Wide: a preseason projection is a guess.
PRIOR_PAR_VAR = 64.0

DEFAULT_REPL_RANK = 24

# Weekly PAR the market's #1 asset is expected to produce; the preseason
# expectation scales linearly with the ADP-prior market value. A crude v1
# calibration knob — the projection, not the market price, absorbs it.
PRESEASON_PAR_TOP = 12.0


def preseason_par_from_market(prior_value: float, top_value: float) -> float:
    """Map an ADP-prior market value to an expected weekly PAR."""
    if top_value <= 0:
        return 0.0
    return PRESEASON_PAR_TOP * prior_value / top_value


@dataclass
class PlayerPerf:
    position: str
    preseason_par: float = 0.0
    cum_par: float = 0.0  # decayed sum of weekly PAR (magnitude preserved)
    cum_par_sq: float = 0.0  # decayed sum of squared weekly PAR
    games: float = 0.0  # decayed effective games


class PerformanceTracker:
    """Tracks every player's decayed PAR state through week boundaries."""

    def __init__(
        self,
        repl_rank_by_pos: dict[str, int],
        preseason_par: dict[str, tuple[str, float]] | None = None,
    ) -> None:
        """preseason_par: player_id -> (position, expected weekly PAR)."""
        self.repl_rank_by_pos = dict(repl_rank_by_pos)
        self.players: dict[str, PlayerPerf] = {
            pid: PlayerPerf(position=pos, preseason_par=par)
            for pid, (pos, par) in (preseason_par or {}).items()
        }

    def state(self, player_id: str) -> PlayerPerf:
        return self.players[player_id]

    # -- the week boundary -------------------------------------------------

    def apply_week(self, scores: list[WeeklyScore]) -> None:
        """Advance one week: decay EVERYONE, then credit those who played.

        Decay-first is the whole fix for the old absence bug: a bye, injury,
        inactive week, or missing score row decays a player's form exactly
        like a played week does — the player just adds nothing on top.
        """
        for p in self.players.values():
            p.cum_par *= PERF_DECAY
            p.cum_par_sq *= PERF_DECAY
            p.games *= PERF_DECAY

        if not scores:
            return

        # positional replacement level = Nth-best score at each position
        by_pos: dict[str, list[float]] = {}
        for s in scores:
            by_pos.setdefault(s.position, []).append(s.points)
        repl: dict[str, float] = {}
        for pos, pts in by_pos.items():
            pts.sort(reverse=True)
            n = self.repl_rank_by_pos.get(pos, DEFAULT_REPL_RANK)
            repl[pos] = pts[n - 1] if len(pts) >= n else pts[-1]

        for s in scores:
            p = self.players.get(s.player_id)
            if p is None:
                p = PlayerPerf(position=s.position)
                self.players[s.player_id] = p
            par = s.points - repl.get(s.position, 0.0)
            p.cum_par += par
            p.cum_par_sq += par * par
            p.games += 1.0

    # -- projections ---------------------------------------------------------

    def projected_par(self, player_id: str) -> float:
        """Rest-of-season weekly PAR proxy:

            (PRESEASON_GAMES * preseason + games * recent_mean)
            / (PRESEASON_GAMES + games)

        Stored separately from market value by construction — nothing in this
        module can reach a market score.
        """
        p = self.players[player_id]
        recent_mean = p.cum_par / p.games if p.games > 0 else 0.0
        return (PRESEASON_GAMES * p.preseason_par + p.games * recent_mean) / (
            PRESEASON_GAMES + p.games
        )

    def projection_uncertainty(self, player_id: str) -> float:
        """Error band for projected_par: observed weekly variance blended
        with the wide preseason prior, shrinking as effective games accrue.
        Decay caps effective games near 1/(1-PERF_DECAY), so this never
        pretends a season of correlated weeks is unlimited evidence."""
        p = self.players[player_id]
        if p.games > 0:
            mean = p.cum_par / p.games
            observed_var = max(0.0, p.cum_par_sq / p.games - mean * mean)
        else:
            observed_var = 0.0
        blended_var = (PRESEASON_GAMES * PRIOR_PAR_VAR + p.games * observed_var) / (
            PRESEASON_GAMES + p.games
        )
        return math.sqrt(blended_var / (1.0 + p.games))

    def all_projections(self) -> dict[str, float]:
        return {pid: self.projected_par(pid) for pid in self.players}


# ---------------------------------------- query-time replacement / VORP --


def replacement_by_position(
    values: dict[str, float],
    positions: dict[str, str],
    rostered: set[str],
    interest: tuple[str, ...] = ("QB", "RB", "WR", "TE"),
) -> dict[str, float]:
    """Per-position replacement value for one league, at query time.

    Replacement at a position is the best player still available on that
    league's waiver wire — computed from the league's own rosters and player
    pool, never fitted globally. A superflex league's scarce QBs and a
    TE-premium league's TEs come out different by construction.
    """
    best: dict[str, float] = {pos: 0.0 for pos in interest}
    for pid, value in values.items():
        if pid in rostered:
            continue
        pos = positions.get(pid)
        if pos is None:
            continue
        if value > best.get(pos, 0.0):
            best[pos] = value
    return best


def vorp(value: float, position: str, replacement: dict[str, float]) -> float:
    """Value over the *positional* replacement — the league-specific floor a
    roster spot at this position could be filled for."""
    return value - replacement.get(position, 0.0)
