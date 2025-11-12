import logging
from dataclasses import asdict, dataclass

import yaml
from espn_api.football import BoxPlayer, League as ESPNLeague


@dataclass
class Player:
    espn_id: int
    name: str

    def __init__(self, espn_id: int, name: str):
        self.espn_id = espn_id
        self.name = name

    @classmethod
    def from_espn_league(cls, espn_league: ESPNLeague) -> list["Player"]:
        players = []
        for team in espn_league.teams:
            for player in team.roster:
                players.append(Player(espn_id=player.playerId, name=player.name))
        return players


@dataclass
class Team:
    espn_id: int
    name: str
    owners: list[str]


@dataclass
class Matchup:
    year: int
    week: int
    home_team_id: int
    away_team_id: int


@dataclass
class PlayerBoxscore(Player):
    position: str
    team: str
    points: float
    projected_points: float
    on_bye: bool = False

    @classmethod
    def from_espn_player_boxscore(cls, espn_player_boxscore: BoxPlayer) -> "PlayerBoxscore":
        return PlayerBoxscore(
            espn_id=espn_player_boxscore.playerId,
            name=espn_player_boxscore.name,
            position=espn_player_boxscore.slot_position,
            team=espn_player_boxscore.proTeam,
            points=espn_player_boxscore.points,
            projected_points=espn_player_boxscore.projected_points,
            on_bye=espn_player_boxscore.on_bye_week,
        )


@dataclass
class Boxscore(Matchup):
    home_score: float
    away_score: float
    home_projected_score: float
    away_projected_score: float
    home_roster: list[PlayerBoxscore]
    away_roster: list[PlayerBoxscore]
    completed: bool = False


@dataclass
class Schedule:
    matchups: list[Matchup]
    boxscores: list[Boxscore]

    def __init__(self, matchups: list[Matchup], boxscores: list[Boxscore]):
        self.matchups = matchups
        self.boxscores = boxscores

    @classmethod
    def from_espn_league(cls, espn_league: ESPNLeague) -> "Schedule":
        matchups_by_week: list[list[Matchup]] = []
        boxscores: list[Boxscore] = []

        for week in range(1, 18):
            logging.debug(f"Processing schedule for week: {week}")
            week_matchups: list[Matchup] = []
            for scoreboard_matchup in espn_league.scoreboard(week=week):
                week_matchups.append(
                    Matchup(
                        year=espn_league.year,
                        week=week,
                        home_team_id=scoreboard_matchup.home_team.team_id,
                        away_team_id=scoreboard_matchup.away_team.team_id,
                    )
                )

            # Only add week's matchups if different from previous week
            if not matchups_by_week:
                # First week, always add
                matchups_by_week.append(week_matchups)
            else:
                # Check if this week's matchups are different from previous week
                prev_week_matchups = matchups_by_week[-1]

                # Create sets of (home_team_id, away_team_id) tuples for comparison
                prev_teams = {(m.home_team_id, m.away_team_id) for m in prev_week_matchups}
                curr_teams = {(m.home_team_id, m.away_team_id) for m in week_matchups}

                if prev_teams != curr_teams:
                    matchups_by_week.append(week_matchups)

                    if espn_league.current_week <= week:
                        weeks_boxscores = espn_league.box_scores(week=week)
                        logging.debug(f"Retrieved {len(weeks_boxscores)} boxscores for week: {week}")

                        boxscores.extend(
                            Boxscore(
                                year=espn_league.year,
                                week=week,
                                home_team_id=boxscore.home_team.team_id,
                                away_team_id=boxscore.away_team.team_id,
                                completed=boxscore.home_score > 0 and boxscore.away_score > 0,
                                home_score=boxscore.home_score,
                                away_score=boxscore.away_score,
                                home_projected_score=boxscore.home_projected,
                                away_projected_score=boxscore.away_projected,
                                home_roster=[PlayerBoxscore.from_espn_player_boxscore(p) for p in boxscore.home_lineup],
                                away_roster=[PlayerBoxscore.from_espn_player_boxscore(p) for p in boxscore.away_lineup],
                            )
                            for boxscore in weeks_boxscores
                        )

        # Flatten matchups into a single list
        matchups = [matchup for week_matchups in matchups_by_week for matchup in week_matchups]

        return Schedule(matchups=matchups, boxscores=boxscores)


@dataclass
class DraftPick:
    team_id: int
    round: int
    pick: int
    keeper: bool
    player_id: int

    @classmethod
    def from_espn_draft_pick(cls, espn_draft_pick) -> "DraftPick":
        return DraftPick(
            team_id=espn_draft_pick.team.team_id,
            round=espn_draft_pick.round_num,
            pick=espn_draft_pick.round_pick,
            keeper=espn_draft_pick.keeper_status,
            player_id=espn_draft_pick.playerId,
        )


@dataclass
class Draft:
    year: int
    selections: list[DraftPick]

    @classmethod
    def from_espn_league(cls, espn_league: ESPNLeague) -> "Draft":
        return Draft(
            year=espn_league.year,
            selections=[DraftPick.from_espn_draft_pick(pick) for pick in espn_league.draft],
        )


class TransactionType:
    ADD = "ADD"
    DROP = "DROP"
    TRADE = "TRADE"
    FREE_AGENT_ADD = "FREE_AGENT_ADD"

    @classmethod
    def from_espn_transaction_action_type(cls, espn_transaction_action_type: str) -> str:
        if espn_transaction_action_type.upper() == "WAIVER ADDED":
            return cls.ADD
        elif espn_transaction_action_type.upper() == "FA ADDED":
            return cls.FREE_AGENT_ADD
        elif espn_transaction_action_type.upper() == "DROPPED":
            return cls.DROP
        elif espn_transaction_action_type.upper() == "TRADED":
            return cls.TRADE
        else:
            raise ValueError(f"Unknown transaction action type: {espn_transaction_action_type}")


@dataclass
class Action:
    team_id: int
    type: TransactionType
    player_id: int
    bid_amount: int = 0

    @classmethod
    def from_espn_transaction_action(cls, espn_transaction_action: list[tuple]) -> "Action":
        return Action(
            team_id=espn_transaction_action[0].team_id,
            type=TransactionType.from_espn_transaction_action_type(espn_transaction_action[1]),
            player_id=espn_transaction_action[2].playerId,
            bid_amount=espn_transaction_action[3],
        )


@dataclass
class Transaction:
    actions: list[Action]
    date: int

    @classmethod
    def from_espn_league(cls, espn_league: ESPNLeague) -> list["Transaction"]:
        transactions = []
        offset = 0
        page_size = 25

        while True:
            espn_transactions = espn_league.recent_activity(offset=offset)
            if not espn_transactions:
                break

            for espn_transaction in espn_transactions:
                transactions.append(
                    Transaction(
                        date=espn_transaction.date,
                        actions=[Action.from_espn_transaction_action(action) for action in espn_transaction.actions],
                    )
                )

            offset += page_size

            # If we got fewer results than the page size, we've reached the end
            if len(espn_transactions) < page_size:
                break

        # Group transactions by date to merge trade actions
        transactions_by_date: dict[int, Transaction] = {}
        for transaction in transactions:
            if transaction.date in transactions_by_date:
                # Merge actions for same date
                transactions_by_date[transaction.date].actions.extend(transaction.actions)
            else:
                transactions_by_date[transaction.date] = transaction

        # Convert back to list
        merged_transactions = list(transactions_by_date.values())

        return merged_transactions


@dataclass
class League:
    id: int
    year: int
    teams: list[Team]
    schedule: Schedule
    players: list[Player]
    transactions: list[Transaction]
    draft: Draft

    def __init__(
        self,
        id: int,
        year: int,
        teams: list[Team],
        schedule: Schedule,
        players: list[Player],
        transactions: list[Transaction],
        draft: Draft,
    ):
        self.id = id
        self.year = year
        self.teams = teams if teams is not None else []
        self.schedule = schedule if schedule is not None else Schedule()
        self.players = players
        self.transactions = transactions
        self.draft = draft

    @classmethod
    def from_espn_league(cls, espn_league: ESPNLeague) -> "League":
        teams = [
            Team(
                espn_id=team.team_id,
                name=team.team_name,
                owners=[f"{owner['firstName']} {owner['lastName']}".rstrip(" ").lstrip(" ") for owner in team.owners],
            )
            for team in espn_league.teams
        ]
        schedule = Schedule.from_espn_league(espn_league)
        players = Player.from_espn_league(espn_league)
        return League(
            id=espn_league.league_id,
            year=espn_league.year,
            teams=teams,
            schedule=schedule,
            players=players,
            transactions=Transaction.from_espn_league(espn_league),
            draft=Draft.from_espn_league(espn_league),
        )

    def to_yaml(self, file_name: str) -> None:
        with open(file=file_name, mode="w") as f:
            f.write(yaml.dump(asdict(self), indent=2))
        return None
