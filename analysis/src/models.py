from dataclasses import dataclass
from datetime import datetime


@dataclass(frozen=True)
class AverageDraftPosition:
    player_id: str
    player_name: str
    position: str
    adp: float


@dataclass(frozen=True)
class Trade:
    trade_id: str
    ts: datetime  # naive UTC
    side_a: list[str]
    side_b: list[str]
    created_ms: int  # Sleeper created_at_sleeper, unix ms — the trade watermark


@dataclass(frozen=True)
class PlayerProfile:
    """Cloud `sleeper_players` identity for one player.

    Lives here rather than in db.py because staging round-trips these: a
    bundle has to resolve identities the same way the database run did.
    """

    player_id: str
    name: str
    position: str


@dataclass(frozen=True)
class WeeklyScore:
    week: int
    player_id: str
    position: str
    points: float


@dataclass
class PlayerBeliefState:
    player_id: str
    guess: float
    var: float
    games: float
    cum_par: float
    position: str
    name: str


@dataclass
class RunState:
    segment: str
    season: str
    last_event_ts: datetime | None
    last_transaction_created: int
    last_week_processed: int
