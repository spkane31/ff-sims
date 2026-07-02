from dataclasses import dataclass

@dataclass
class TradeSide:
    pass

@dataclass
class AverageDraftPosition:
    pass


@dataclass
class PlayerBeliefState:
    player_id: str
    guess: float
    var: float
    games: float
    cum_par: float
    position: str
    name: str