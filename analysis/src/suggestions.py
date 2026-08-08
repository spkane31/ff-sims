"""Roster-aware trade suggestions: fairness and improvement kept separate.

Equal package market values only establish that a trade is FAIR. Whether a
team IMPROVES is a property of its roster: the starter displaced, the flex
freed up, the bench player dropped to make room. So every candidate package
is applied to both rosters — drops included — and each team's optimized
starting lineup is scored before and after. A suggestion carries both axes:

* `fairness_delta`: difference in package market value, where each side's
  outlay includes the actual players it must drop to fit the trade.
* `utility_delta_a` / `utility_delta_b`: change in optimized starting-lineup
  projected points for each team.

Pareto-improving trades (both teams gain) rank first; market-fair trades
that help one side at the other's expense are still reported, labeled
"one-sided", never silently mixed in.
"""

from __future__ import annotations

from dataclasses import dataclass
from itertools import combinations

FLEX_SLOT = "FLEX"
SUPERFLEX_SLOT = "SUPERFLEX"


@dataclass(frozen=True)
class LineupSettings:
    """Starting slots and roster cap for the league being suggested for."""

    slots: dict[str, int]  # e.g. {"QB": 1, "RB": 2, "WR": 2, "TE": 1, "FLEX": 1}
    roster_size: int
    flex_positions: tuple[str, ...] = ("RB", "WR", "TE")
    superflex_positions: tuple[str, ...] = ("QB", "RB", "WR", "TE")


@dataclass(frozen=True)
class TradeSuggestion:
    give: tuple[str, ...]  # leaves team A
    receive: tuple[str, ...]  # leaves team B
    drops_a: tuple[str, ...]  # cut by A to fit the roster cap
    drops_b: tuple[str, ...]
    fairness_delta: float  # A's outlay minus B's outlay, market value
    utility_delta_a: float  # optimized lineup points, after minus before
    utility_delta_b: float
    lineup_a: tuple[str, ...]  # post-trade optimized starters
    lineup_b: tuple[str, ...]
    entered_a: tuple[str, ...]  # lineup changes, both teams, both directions
    exited_a: tuple[str, ...]
    entered_b: tuple[str, ...]
    exited_b: tuple[str, ...]
    label: str  # "pareto" (both improve) or "one-sided" (fair, but not both)


def optimal_lineup(
    roster: list[str],
    projections: dict[str, float],
    positions: dict[str, str],
    settings: LineupSettings,
) -> tuple[tuple[str, ...], float]:
    """Best legal starting lineup by projected points, deterministically.

    Dedicated slots fill first from each position's best; FLEX then
    SUPERFLEX take the best remaining eligible players.
    """
    def best(candidates: list[str], count: int) -> list[str]:
        ordered = sorted(candidates, key=lambda p: (-projections.get(p, 0.0), p))
        return ordered[:count]

    remaining = set(roster)
    starters: list[str] = []
    for slot, count in sorted(settings.slots.items()):
        if slot in (FLEX_SLOT, SUPERFLEX_SLOT):
            continue
        picks = best([p for p in remaining if positions.get(p) == slot], count)
        starters.extend(picks)
        remaining.difference_update(picks)
    for slot, eligible in (
        (FLEX_SLOT, settings.flex_positions),
        (SUPERFLEX_SLOT, settings.superflex_positions),
    ):
        count = settings.slots.get(slot, 0)
        if not count:
            continue
        picks = best(
            [p for p in remaining if positions.get(p) in eligible], count
        )
        starters.extend(picks)
        remaining.difference_update(picks)

    points = sum(projections.get(p, 0.0) for p in starters)
    return tuple(sorted(starters)), points


def _apply_package(
    roster: list[str],
    out_players: tuple[str, ...],
    in_players: tuple[str, ...],
    market_values: dict[str, float],
    projections: dict[str, float],
    positions: dict[str, str],
    settings: LineupSettings,
) -> tuple[list[str], tuple[str, ...], tuple[str, ...], float] | None:
    """Apply a package to one roster: swap, then drop the cheapest bench
    players until the roster cap holds. Returns (roster, drops, lineup,
    points), or None if the cap cannot be satisfied without cutting a
    starter — that trade is not actually executable as valued."""
    new_roster = [p for p in roster if p not in out_players] + list(in_players)
    drops: list[str] = []
    while len(new_roster) > settings.roster_size:
        lineup, _ = optimal_lineup(new_roster, projections, positions, settings)
        bench = [p for p in new_roster if p not in lineup]
        if not bench:
            return None
        cut = min(bench, key=lambda p: (market_values.get(p, 0.0), p))
        new_roster.remove(cut)
        drops.append(cut)
    lineup, points = optimal_lineup(new_roster, projections, positions, settings)
    return new_roster, tuple(drops), lineup, points


def suggest_trades(
    roster_a: list[str],
    roster_b: list[str],
    market_values: dict[str, float],
    projections: dict[str, float],
    positions: dict[str, str],
    settings: LineupSettings,
    fairness_tolerance: float = 0.10,
    max_package: int = 2,
) -> list[TradeSuggestion]:
    """Enumerate 1-for-1 through 2-for-2 packages between two rosters.

    A candidate survives if it is market-fair within `fairness_tolerance`
    (drop costs included) and improves at least one team's optimized lineup.
    Pareto improvements come first, ordered by combined utility gain.
    """
    lineup_a0, points_a0 = optimal_lineup(roster_a, projections, positions, settings)
    lineup_b0, points_b0 = optimal_lineup(roster_b, projections, positions, settings)

    def packages(roster: list[str]):
        for size in range(1, max_package + 1):
            yield from combinations(sorted(roster), size)

    results: list[TradeSuggestion] = []
    for give in packages(roster_a):
        for receive in packages(roster_b):
            applied_a = _apply_package(
                roster_a, give, receive, market_values, projections,
                positions, settings,
            )
            applied_b = _apply_package(
                roster_b, receive, give, market_values, projections,
                positions, settings,
            )
            if applied_a is None or applied_b is None:
                continue
            _, drops_a, lineup_a, points_a = applied_a
            _, drops_b, lineup_b, points_b = applied_b

            outlay_a = sum(market_values.get(p, 0.0) for p in (*give, *drops_a))
            outlay_b = sum(market_values.get(p, 0.0) for p in (*receive, *drops_b))
            fairness_delta = outlay_a - outlay_b
            biggest = max(outlay_a, outlay_b)
            if biggest <= 0 or abs(fairness_delta) > fairness_tolerance * biggest:
                continue

            delta_a = points_a - points_a0
            delta_b = points_b - points_b0
            if max(delta_a, delta_b) <= 0:
                continue  # fair but pointless: nobody improves

            results.append(
                TradeSuggestion(
                    give=give,
                    receive=receive,
                    drops_a=drops_a,
                    drops_b=drops_b,
                    fairness_delta=fairness_delta,
                    utility_delta_a=delta_a,
                    utility_delta_b=delta_b,
                    lineup_a=lineup_a,
                    lineup_b=lineup_b,
                    entered_a=tuple(sorted(set(lineup_a) - set(lineup_a0))),
                    exited_a=tuple(sorted(set(lineup_a0) - set(lineup_a))),
                    entered_b=tuple(sorted(set(lineup_b) - set(lineup_b0))),
                    exited_b=tuple(sorted(set(lineup_b0) - set(lineup_b))),
                    label=(
                        "pareto" if delta_a > 0 and delta_b > 0 else "one-sided"
                    ),
                )
            )

    results.sort(
        key=lambda s: (
            0 if s.label == "pareto" else 1,
            -(s.utility_delta_a + s.utility_delta_b),
            s.give,
            s.receive,
        )
    )
    return results
