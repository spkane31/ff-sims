from .player import Player
from espn_api.football import Team

from utils import sample_normal_distribution


def _get_positions_by_actual(players: list[Player], pos: list[str]) -> list[Player]:
    # sorted by actual points scored
    players = [p for p in players if p.position in pos]
    return sorted(players, key=lambda p: p.actual, reverse=True)


def _get_positions_by_projected(players: list[Player], pos: list[str]) -> list[Player]:
    # sorted by projected points scored
    players = [p for p in players if p.position in pos]
    return sorted(players, key=lambda p: p.projection, reverse=True)


class Roster:
    def __init__(self, data: list[dict[str, any]]):
        self.players = [Player(d) for d in data]

    @classmethod
    def from_matchup(cls, team: Team):
        # print(team.roster[0].name)
        # print(team.roster[0].stats)
        return Roster(
            [
                {
                    "name": player.name,
                    "actual": player.stats[0]["points"],
                    "projection": -1,
                    "position": player.position,
                    "status": player.position,
                }
                for player in team.roster
            ]
        )

    def __str__(self):
        return f"Roster({[str(p) for p in self.players]})"

    def get_position(
        self,
        pos: list[str],
    ) -> list[Player]:
        # sorted by actual points provided
        players = [p for p in self.players if p.position in pos]
        return sorted(players, key=lambda p: p.actual, reverse=True)

    def get_top_n_by_position(self, pos: str, n: int, sorting_func=_get_positions_by_actual) -> list[Player]:
        available = sorting_func(self.players, [pos])
        if len(available) < n:
            return available
        return available[:n]

    def points_scored(self) -> float:
        return sum([p.actual for p in self.players if not p.on_bench()])

    def maximum_points(self) -> float:
        roster = self._max_points_by_sorting_func(_get_positions_by_actual)
        return sum([player.actual for player in roster])

    def _max_points_by_sorting_func(self, sorting_func) -> float:
        # Top 2 QBs on roster
        qbs = self.get_top_n_by_position("QB", 2, sorting_func=sorting_func)

        # Top 2 WRs
        wrs = self.get_top_n_by_position("WR", 2, sorting_func=sorting_func)

        # Top 2 RBs
        rbs = self.get_top_n_by_position("RB", 2, sorting_func=sorting_func)

        # Top TE
        te = self.get_top_n_by_position("TE", 2, sorting_func=sorting_func)

        # Top D/ST
        dst = self.get_top_n_by_position("D/ST", 2, sorting_func=sorting_func)

        # Top K
        k = self.get_top_n_by_position("K", 2, sorting_func=sorting_func)

        # Top 1 of remaining WR/RB/TE for flex
        flex = _get_positions_by_actual(self.players, ["WR", "RB", "TE"])

        # Have to remove the wrs, rbs, and te from the flex list to get the top 1
        self._trim_flexes(flex, wrs=wrs, rbs=rbs, te=te)

        return [
            qbs[0],
            qbs[1],
            rbs[0],
            rbs[1],
            wrs[0],
            wrs[1],
            te[0],
            dst[0],
            k[0],
            flex[0],
        ]

    def projected_points(self) -> float:
        """Projected score of best possible roster"""
        roster = self.best_projected_lineup()
        return sum([player.projection for player in roster])

    def best_projected_lineup(self) -> list[Player]:
        return self._max_points_by_sorting_func(_get_positions_by_projected)

    def projected_score(self) -> float:
        """Projected score of actual roster"""
        return sum([p.projection for p in self.players if p.on_bench()])

    def _trim_flexes(
        self,
        potential: list[Player],
        wrs: list[Player] = None,
        rbs: list[Player] = None,
        te: list[Player] = None,
    ):
        if wrs:
            [potential.remove(wr) for wr in wrs]
        if rbs:
            [potential.remove(rb) for rb in rbs]
        if te:
            [potential.remove(te) for te in te]

    def simulate_projected_score(self, positional_stats: dict[str, tuple[float, float]]) -> float:
        projected_roster = self.best_projected_lineup()

        total_points = 0
        for player in projected_roster:
            (mean, std_dev) = positional_stats[player.position]
            rng = sample_normal_distribution(mean, std_dev)
            total_points += (player.projection) * (1 + rng)

        return total_points
