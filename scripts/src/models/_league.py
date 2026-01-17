import logging
import time
from dataclasses import asdict, dataclass
from datetime import datetime
from typing import Optional

import yaml
from espn_api.football import BoxPlayer
from espn_api.football import League as ESPNLeague

from src.sleeper import SleeperClient


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
    game_type: str
    is_playoff: bool
    home_score: float = 0.0
    away_score: float = 0.0
    home_projected_score: float = 0.0
    away_projected_score: float = 0.0


@dataclass
class PlayerBoxscore(Player):
    position: str
    team: str
    points: float
    projected_points: float
    on_bye: bool = False
    pro_opponent: str = None
    pro_pos_rank: int = None
    game_played: int = None
    game_date: str = None
    active_status: str = None
    eligible_slots: list[str] = None
    on_team_id: int = None
    injured: bool = False
    injury_status: str = None
    percent_owned: float = None
    percent_started: float = None
    stats: dict = None

    @classmethod
    def from_espn_player_boxscore(cls, espn_player_boxscore: BoxPlayer) -> "PlayerBoxscore":
        # Handle game_date formatting with conditional check
        game_date = None
        if hasattr(espn_player_boxscore, "game_date") and espn_player_boxscore.game_date:
            game_date = espn_player_boxscore.game_date.strftime("%Y-%m-%d %H:%M:%S")

        return PlayerBoxscore(
            espn_id=espn_player_boxscore.playerId,
            name=espn_player_boxscore.name,
            position=espn_player_boxscore.slot_position,
            team=espn_player_boxscore.proTeam,
            points=espn_player_boxscore.points,
            projected_points=espn_player_boxscore.projected_points,
            on_bye=espn_player_boxscore.on_bye_week,
            pro_opponent=espn_player_boxscore.pro_opponent,
            pro_pos_rank=espn_player_boxscore.pro_pos_rank,
            game_played=espn_player_boxscore.game_played,
            game_date=game_date,
            active_status=espn_player_boxscore.active_status,
            eligible_slots=espn_player_boxscore.eligibleSlots,
            on_team_id=espn_player_boxscore.onTeamId,
            injured=espn_player_boxscore.injured,
            injury_status=espn_player_boxscore.injuryStatus,
            percent_owned=espn_player_boxscore.percent_owned,
            percent_started=espn_player_boxscore.percent_started,
            stats=espn_player_boxscore.stats,
        )


@dataclass
class Boxscore(Matchup):
    home_roster: list[PlayerBoxscore] = None
    away_roster: list[PlayerBoxscore] = None
    completed: bool = False

    def __post_init__(self):
        if self.home_roster is None:
            self.home_roster = []
        if self.away_roster is None:
            self.away_roster = []


@dataclass
class BoxScorePlayerData:
    player_name: str
    player_id: int
    projected_points: float
    actual_points: float
    player_position: str
    status: str
    week: int
    year: int
    owner_espn_id: int


@dataclass
class Schedule:
    matchups: list[Matchup]
    boxscores: list[Boxscore]
    box_score_players: list[BoxScorePlayerData]

    def __init__(
        self,
        matchups: list[Matchup] = None,
        boxscores: list[Boxscore] = None,
        box_score_players: list[BoxScorePlayerData] = None,
    ):
        self.matchups = matchups if matchups is not None else []
        self.boxscores = boxscores if boxscores is not None else []
        self.box_score_players = box_score_players if box_score_players is not None else []

    @classmethod
    def from_espn_league(cls, espn_league: ESPNLeague, years: list[int] = None) -> "Schedule":
        if years is None:
            # NFL season starts in August, so before August use previous year
            now = datetime.now()
            years = [now.year if now.month >= 8 else now.year - 1]

        matchups_by_week: list[list[Matchup]] = []
        boxscores: list[Boxscore] = []
        box_score_players: list[BoxScorePlayerData] = []

        # Handle pre-2019 leagues differently (ESPN API changed in 2019)
        if espn_league.year < 2019:
            logging.info(f"Processing pre-2019 league data for year {espn_league.year}")
            # For pre-2019, scoreboard provides both matchups and scores
            for week in range(1, 18):
                logging.debug(f"Processing week {week}")
                # Break if we've gone past current week for current year
                if week > espn_league.current_week and datetime.now().year == espn_league.year:
                    break

                for scoreboard_matchup in espn_league.scoreboard(week=week):
                    # Check for valid teams (some playoff weeks may have None)
                    if not hasattr(scoreboard_matchup, "away_team") or not hasattr(scoreboard_matchup, "home_team"):
                        break

                    # Create matchup
                    matchup = Matchup(
                        year=espn_league.year,
                        week=week,
                        home_team_id=scoreboard_matchup.home_team.team_id,
                        away_team_id=scoreboard_matchup.away_team.team_id,
                        game_type=scoreboard_matchup.matchup_type,
                        is_playoff=scoreboard_matchup.is_playoff,
                        home_score=scoreboard_matchup.home_score,
                        away_score=scoreboard_matchup.away_score,
                        home_projected_score=-1,  # Projected scores not available pre-2019
                        away_projected_score=-1,
                    )
                    matchups_by_week.append([matchup])

                    # Create boxscore (no detailed lineup data available pre-2019)
                    boxscores.append(
                        Boxscore(
                            year=espn_league.year,
                            week=week,
                            home_team_id=scoreboard_matchup.home_team.team_id,
                            away_team_id=scoreboard_matchup.away_team.team_id,
                            game_type=scoreboard_matchup.matchup_type,
                            is_playoff=scoreboard_matchup.is_playoff,
                            home_score=scoreboard_matchup.home_score,
                            away_score=scoreboard_matchup.away_score,
                            home_projected_score=-1,  # Projected scores not available pre-2019
                            away_projected_score=-1,
                            home_roster=[],  # Lineup data not available pre-2019
                            away_roster=[],
                            completed=True,  # All historical games are completed
                        )
                    )

        else:
            # Post-2019 logic: Build matchups from scoreboard, then get detailed box scores
            for week in range(1, 18):
                logging.debug(f"Processing schedule for week: {week}")
                week_matchups: list[Matchup] = []
                for scoreboard_matchup in espn_league.scoreboard(week=week):
                    # Check for valid teams (some playoff weeks may have None)
                    if not hasattr(scoreboard_matchup, "away_team") or not hasattr(scoreboard_matchup, "home_team"):
                        continue

                    week_matchups.append(
                        Matchup(
                            year=espn_league.year,
                            week=week,
                            home_team_id=scoreboard_matchup.home_team.team_id,
                            away_team_id=scoreboard_matchup.away_team.team_id,
                            game_type=scoreboard_matchup.matchup_type,
                            is_playoff=scoreboard_matchup.is_playoff,
                            home_score=scoreboard_matchup.home_score,
                            away_score=scoreboard_matchup.away_score,
                            home_projected_score=0.0,  # Will be populated from box_scores if available
                            away_projected_score=0.0,
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

                # Fetch box scores for all weeks up to current week (regardless of matchup changes)
                if week <= espn_league.current_week:
                    weeks_boxscores = espn_league.box_scores(week=week)
                    logging.debug(f"Retrieved {len(weeks_boxscores)} boxscores for week: {week}")

                    for boxscore in weeks_boxscores:
                        # Skip invalid matchups (can happen in playoff weeks)
                        if boxscore.away_team == 0 or boxscore.home_team == 0:
                            continue

                        boxscores.append(
                            Boxscore(
                                year=espn_league.year,
                                week=week,
                                home_team_id=boxscore.home_team.team_id,
                                away_team_id=boxscore.away_team.team_id,
                                game_type=boxscore.matchup_type,
                                is_playoff=boxscore.is_playoff,
                                completed=boxscore.home_score > 0 and boxscore.away_score > 0,
                                home_score=boxscore.home_score,
                                away_score=boxscore.away_score,
                                home_projected_score=boxscore.home_projected,
                                away_projected_score=boxscore.away_projected,
                                home_roster=[PlayerBoxscore.from_espn_player_boxscore(p) for p in boxscore.home_lineup],
                                away_roster=[PlayerBoxscore.from_espn_player_boxscore(p) for p in boxscore.away_lineup],
                            )
                        )

                        # Collect box score player data (only for current year, past weeks)
                        if espn_league.year == datetime.now().year and week < espn_league.current_week:
                            home_team_id = boxscore.home_team.team_id
                            away_team_id = boxscore.away_team.team_id

                            # Collect home team player data
                            for player in boxscore.home_lineup:
                                box_score_players.append(
                                    BoxScorePlayerData(
                                        player_name=player.name,
                                        player_id=player.playerId,
                                        projected_points=player.projected_points,
                                        actual_points=player.points,
                                        player_position=player.position,
                                        status=player.slot_position,
                                        week=week,
                                        year=espn_league.year,
                                        owner_espn_id=home_team_id,
                                    )
                                )

                            # Collect away team player data
                            for player in boxscore.away_lineup:
                                box_score_players.append(
                                    BoxScorePlayerData(
                                        player_name=player.name,
                                        player_id=player.playerId,
                                        projected_points=player.projected_points,
                                        actual_points=player.points,
                                        player_position=player.position,
                                        status=player.slot_position,
                                        week=week,
                                        year=espn_league.year,
                                        owner_espn_id=away_team_id,
                                    )
                                )

        # Flatten matchups into a single list
        matchups = [matchup for week_matchups in matchups_by_week for matchup in week_matchups]

        return Schedule(matchups=matchups, boxscores=boxscores, box_score_players=box_score_players)


@dataclass
class DraftPick:
    team_id: int
    round: int
    pick: int
    keeper: bool
    player_id: int
    player_name: str
    player_position: str = "Unknown"

    @classmethod
    def from_espn_draft_pick(cls, espn_draft_pick, player_position: str = "Unknown") -> "DraftPick":
        return DraftPick(
            team_id=espn_draft_pick.team.team_id,
            round=espn_draft_pick.round_num,
            pick=espn_draft_pick.round_pick,
            keeper=espn_draft_pick.keeper_status,
            player_id=espn_draft_pick.playerId,
            player_name=espn_draft_pick.playerName,
            player_position=player_position,
        )


@dataclass
class Draft:
    year: int
    selections: list[DraftPick]

    @classmethod
    def from_espn_league(cls, espn_league: ESPNLeague) -> "Draft":
        selections = []
        for pick in espn_league.draft:
            try:
                logging.debug(f"Processing draft pick: {pick.playerName} (ID: {pick.playerId})")

                # Fetch player info to get position
                player_info = espn_league.player_info(playerId=pick.playerId)
                player_position = player_info.position if player_info else "Unknown"

                selections.append(DraftPick.from_espn_draft_pick(pick, player_position=player_position))

                # Avoid rate limiting when fetching player info
                time.sleep(0.1)

            except Exception as e:
                logging.error(f"Error processing draft pick {pick.playerName}: {e}")
                # Add the pick without position info
                selections.append(DraftPick.from_espn_draft_pick(pick))
                continue

        return Draft(
            year=espn_league.year,
            selections=selections,
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
    player_name: str
    player_position: str
    bid_amount: int = 0

    @classmethod
    def from_espn_transaction_action(cls, espn_transaction_action: list[tuple]) -> "Action":
        player = espn_transaction_action[2]
        return Action(
            team_id=espn_transaction_action[0].team_id,
            type=TransactionType.from_espn_transaction_action_type(espn_transaction_action[1]),
            player_id=player.playerId,
            player_name=player.name,
            player_position=player.position,
            bid_amount=espn_transaction_action[3],
        )


@dataclass
class Transaction:
    actions: list[Action]
    date: str
    year: int

    @classmethod
    def from_espn_league(cls, espn_league: ESPNLeague) -> list["Transaction"]:
        # Year validation: Transactions only available for 2024+
        if espn_league.year < 2024:
            logging.warning(f"Transactions are not available for years before 2024 (requested: {espn_league.year})")
            return []

        transactions = []
        offset = 0
        page_size = 25

        while True:
            espn_transactions = espn_league.recent_activity(offset=offset)
            if not espn_transactions:
                break

            for espn_transaction in espn_transactions:
                # Format date from timestamp to string
                tx_date = datetime.fromtimestamp(espn_transaction.date / 1000)
                formatted_date = tx_date.strftime("%Y-%m-%d %H:%M:%S")

                transactions.append(
                    Transaction(
                        date=formatted_date,
                        year=espn_league.year,
                        actions=[Action.from_espn_transaction_action(action) for action in espn_transaction.actions],
                    )
                )

            offset += page_size

            # If we got fewer results than the page size, we've reached the end
            if len(espn_transactions) < page_size:
                break

        # Group transactions by date to merge trade actions
        transactions_by_date: dict[str, Transaction] = {}
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
class LeagueSource:
    ESPN: str = "ESPN"
    SLEEPER: str = "SLEEPER"


@dataclass
class League:
    id: int
    year: int
    teams: list[Team]
    schedule: Schedule
    players: list[Player]
    transactions: list[Transaction]
    draft: Draft
    league_source: LeagueSource

    def __init__(
        self,
        id: int,
        year: int,
        teams: list[Team],
        schedule: Schedule,
        players: list[Player],
        transactions: list[Transaction],
        draft: Draft,
        league_source: LeagueSource = LeagueSource.ESPN,
    ):
        self.id = id
        self.year = year
        self.teams = teams if teams is not None else []
        self.schedule = schedule if schedule is not None else Schedule()
        self.players = players
        self.transactions = transactions
        self.draft = draft
        self.league_source = league_source

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
            league_source=LeagueSource.ESPN,
        )

    @classmethod
    def from_sleeper_league(cls, id: int) -> "League":
        raise NotImplementedError("Sleeper league integration is not yet implemented.")

    def to_yaml(self, file_name: str) -> None:
        with open(file=file_name, mode="w") as f:
            f.write(yaml.dump(asdict(self), indent=2))
        return None


def get_all_years(
    espn_league: Optional[ESPNLeague] = None, sleeper_league: Optional[SleeperClient] = None
) -> list[League]:
    if espn_league is not None:
        return _get_all_years_espn()
    elif sleeper_league is not None:
        return _get_all_years_sleeper()
    else:
        return NotImplementedError("Function to get all years of leagues is not yet implemented.")


def _get_all_years_espn() -> list[League]:
    return NotImplementedError("Function to get all years of ESPN leagues is not yet implemented.")


def _get_all_years_sleeper() -> list[League]:
    return NotImplementedError("Function to get all years of Sleeper leagues is not yet implemented.")
