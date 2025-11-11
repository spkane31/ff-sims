from dataclasses import dataclass, asdict
import yaml
import os
from typing import Optional
from espn_api.football import League as ESPNLeague


@dataclass
class Team:
    id: int
    espn_id: int


@dataclass
class Matchup:
    home_team_id: int
    away_team_id: int


@dataclass
class Schedule:
    matchups: list[Matchup]

    def __init__(self, matchups: Optional[list[Matchup]] = None):
        self.matchups = matchups if matchups is not None else []


@dataclass
class League:
    id: int
    teams: list[Team]
    schedule: Schedule
    __league: ESPNLeague
    __espn_s2: str
    __espn_swid: str
    settings: dict

    def __init__(
        self,
        id: int,
        espn_s2: str = os.getenv("ESPN_S2", ""),
        espn_swid: str = os.getenv("SWID", ""),
        teams: Optional[list[Team]] = None,
        schedule: Optional[Schedule] = None,
    ):
        self.id = id
        self.__espn_s2 = espn_s2
        self.__espn_swid = espn_swid
        self.__league = ESPNLeague(
            league_id=id, espn_s2=espn_s2, swid=espn_swid, debug=False, year=2025
        )
        self.teams = teams if teams is not None else []
        self.schedule = schedule if schedule is not None else Schedule()

        self.settings = self.__league.settings

    def to_yaml(self, file_name: str) -> None:
        with open(file=file_name, mode="w") as f:
            f.write(yaml.dump(asdict(self)))
        return None

    def add_team(self, team: Team) -> None:
        # Check if a team w/ same id / espn_id already exists
        if (
            len(
                [t for t in self.teams if t.espn_id == team.espn_id and t.id == team.id]
            )
            > 0
        ):
            return

        self.teams.append(team)
