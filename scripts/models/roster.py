class Player:
    def __init__(self, data: dict[str, any]):
        self.name = data["name"]
        self.projection = data["projection"]
        self.actual = data["actual"]
        self.diff = data["diff"]
        self.position = data["position"]
        self.status = data["status"]

    def on_bench(self) -> bool:
        return self.status == "BE"

    def __str__(self):
        return f"Player({self.name}, {self.position}, {self.status}, proj: {self.projection}, actual: {self.actual})"


class Roster:
    def __init__(self, data: list[dict[str, any]]):
        self.players = [Player(d) for d in data]

    def __str__(self):
        return f"Roster({[str(p) for p in self.players]})"

    def get_position(self, pos: list[str]) -> list[Player]:
        # sorted by actual points provided
        players = [p for p in self.players if p.position in pos]
        return sorted(players, key=lambda p: p.actual, reverse=True)

    def _get_positions(self, pos: list[str]) -> list[Player]:
        # sorted by actual points provided
        players = [p for p in self.players if p.position in pos]
        return sorted(players, key=lambda p: p.actual, reverse=True)

    def get_top_n_by_position(self, pos: str, n: int) -> list[Player]:
        available = self.get_position([pos])
        if len(available) < n:
            return available
        return available[:n]

    def points_scored(self) -> float:
        return sum([p.actual for p in self.players if p.status != "BE"])

    def maximum_points(self) -> float:
        # Top 2 QBs on roster
        qbs = self.get_top_n_by_position("QB", 2)

        # Top 2 WRs
        wrs = self.get_top_n_by_position("WR", 2)

        # Top 2 RBs
        rbs = self.get_top_n_by_position("RB", 2)

        # Top TE
        te = self.get_top_n_by_position("TE", 2)

        # Top D/ST
        dst = self.get_top_n_by_position("D/ST", 2)

        # Top K
        k = self.get_top_n_by_position("K", 2)

        # Top 1 of remaining WR/RB/TE for flex
        flex = self._get_positions(["WR", "RB", "TE"])

        # Have to remove the wrs, rbs, and te from the flex list to get the top 1
        self._trim_flexes(flex, wrs=wrs, rbs=rbs, te=te)

        ret = sum([qb.actual for qb in qbs])
        ret += sum([qb.actual for qb in wrs])
        ret += sum([qb.actual for qb in rbs])
        ret += sum([qb.actual for qb in te])
        ret += sum([qb.actual for qb in dst])
        ret += sum([qb.actual for qb in k])
        ret += flex[0].actual  # One flex player

        return ret

    def _trim_flexes(
        self, potential: list[Player], wrs: list[Player] = None, rbs: list[Player] = None, te: list[Player] = None
    ):
        if wrs:
            [potential.remove(wr) for wr in wrs]
        if rbs:
            [potential.remove(rb) for rb in rbs]
        if te:
            [potential.remove(te) for te in te]
