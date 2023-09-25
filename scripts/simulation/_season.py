import os

from espn_api.football import League, Team, Player
from prettytable import PrettyTable as PT

from models import Roster
from utils import mean, sample_normal_distribution, std_dev

from utils import flatten_extend

# This has to be hard coded for now, can't find a way to get the information from the API
_DIVISIONAL_BREAKDOWN = {
    "EAST": ["nick toth", "Connor Brand", "mitch lichtinger", "Nick DeHaven", "Josh Doepker"],
    "WEST": ["Kyle Burns", "Ethan Moran", "jack aldridge", "Sean  Kane", "Kevin Dailey"],
}


class Standing:
    def __init__(self) -> None:
        self.wins = 0
        self.losses = 0
        self.points_scored = 0
        self.points_against = 0

    def add_win(self) -> None:
        self.wins += 1

    def add_loss(self) -> None:
        self.losses += 1

    def add_points_scored(self, points: float) -> None:
        self.points_scored += points

    def add_points_against(self, points: float) -> None:
        self.points_against += points


class SingleSeasonSimulationResults:
    def __init__(self, teams: list[str]) -> None:
        self.teams = teams
        self.standings: dict[str, Standing] = {}

        for team in self.teams:
            self.standings[team] = Standing()

        self.final_results: list[str] = []

    def team_win(self, team: str, points_scored: float, points_against: float) -> None:
        self.standings[team].add_win()
        self.standings[team].add_points_scored(points_scored)
        self.standings[team].add_points_against(points_against)

    def team_loss(self, team: str, points_scored: float, points_against: float) -> None:
        self.standings[team].add_loss()
        self.standings[team].add_points_scored(points_scored)
        self.standings[team].add_points_against(points_against)

    def get_sorted_results(self) -> list[list[any]]:
        sortable_list = []
        for team_name, standings in self.standings.items():
            sortable_list.append(
                [team_name, standings.wins, standings.losses, standings.points_scored, standings.points_against]
            )
        return sorted(sortable_list, key=lambda row: (row[1], row[3], row[4]), reverse=True)

    def print(self) -> None:
        """Pretty print a table of the season simulation, this is for debugging purposes primarily"""

        pt = PT()
        pt.title = "Final Standings"
        pt.field_names = ["Team", "Wins", "losses", "PF", "PA"]
        pt.add_rows(self.get_sorted_results())

        print(pt)

    def print_current_standings(self) -> None:
        pt = PT()
        pt.title = "Current Standings"
        pt.field_names = ["Team", "Wins", "losses", "PF", "PA"]
        pt.add_rows(self.get_sorted_results())

        print(pt)

    def select_playoff_teams(self) -> None:
        """Leage is set up to so top team in each division get a bye"""
        sorted_teams = self.get_sorted_results()
        playoff_teams = [sorted_teams[0]]
        top_east = self._top_team_in_division(sorted_teams, "EAST")
        top_west = self._top_team_in_division(sorted_teams, "WEST")

        if top_east == playoff_teams[0]:
            playoff_teams.append(top_west)
        if top_west == playoff_teams[0]:
            playoff_teams.append(top_east)

        try:
            sorted_teams.remove(top_west)
        except ValueError:
            raise ValueError(f"team '{top_west}' not in '{sorted_teams}")
        try:
            sorted_teams.remove(top_east)
        except ValueError:
            raise ValueError(f"team '{top_east}' not in '{sorted_teams}")

        playoff_teams.append(sorted_teams[0])
        playoff_teams.append(sorted_teams[1])
        playoff_teams.append(sorted_teams[2])
        playoff_teams.append(sorted_teams[3])

        return [p[0] for p in playoff_teams]

    def _top_team_in_division(self, sorted_teams: list[list], division: str) -> list:
        division_teams = _DIVISIONAL_BREAKDOWN[division]
        for team in sorted_teams:
            if team[0] in division_teams:
                return team
        raise ValueError(f"could not determine winner of division: {sorted_teams}")

    def set_playoff_results(self, results: list[str]) -> None:
        self.final_results = results


class SeasonSimulation:
    def __init__(
        self,
        data: dict[str, any],
        position_data: dict[str, tuple[float, float]],
        league: League = None,
        schedule: list[list[dict[str, any]]] = None,
        starting_week: int = 0,
    ) -> None:
        self.league: League = league
        self.schedule = schedule
        self.teams, self._team_to_id = _get_teams_from_annual_data(data)
        self._team_scores = _get_team_scores_from_annual_data(data)
        self.position_stats = position_data
        self.standings: dict[str, Standing] = {}
        self.regular_season_simulation_results: dict[str, list[int]] = {}
        self.playoff_simulation_results: dict[str, list[int]] = {}
        self.matchup_data = data
        self.starting_week = starting_week
        self.raw_results = []

        for team in self.teams:
            self.standings[team] = Standing()
            self.regular_season_simulation_results[team] = [
                0 for t in self.teams
            ]  # Counting of how many times a team is in each position.
            self.playoff_simulation_results[team] = [0 for t in self.teams]
        self._validate_teams()
        self._current_standings()

    def _current_standings(self) -> None:
        results = SingleSeasonSimulationResults(self.teams)
        for week, matchups in self.matchup_data.items():
            if int(week) >= self.starting_week:
                break

            for matchup in matchups:
                home_score = matchup["home_team_score"]
                away_score = matchup["away_team_score"]

                if home_score > away_score:
                    results.team_win(matchup["home_team"], home_score, away_score)
                    results.team_loss(matchup["away_team"], away_score, home_score)
                else:
                    results.team_win(matchup["away_team"], away_score, home_score)
                    results.team_loss(matchup["home_team"], home_score, away_score)

        results.print_current_standings()
        return

    def _validate_teams(self) -> None:
        """This function ensures each team is in one of the divisions. I don't know of a way to get this data from the API, so this is a work around"""

        for team in self.teams:
            flag = False
            for teams in _DIVISIONAL_BREAKDOWN.values():
                if team in teams:
                    flag = True

            if not flag:
                raise ValueError(f"team `{team}` not in the stated divisions")

    def expected_wins(self, n: int = 1000) -> None:
        sims_completed = 0
        team_expected = {t: 0.0 for t in self.teams}
        for idx, week in enumerate(self.schedule):
            if idx + 1 > self.league.current_week:
                break
            for matchup in week:
                if matchup["home_team_score"] == 0.0 or matchup["away_team_score"] == 0.0:
                    # This is a week that hasn't finished yet
                    continue

                for _ in range(0, n):
                    home_scores = self._team_scores[matchup["home_team"]]
                    home_score = sample_normal_distribution(mean(home_scores), std_dev(home_scores))

                    away_scores = self._team_scores[matchup["away_team"]]
                    away_score = sample_normal_distribution(mean(away_scores), std_dev(away_scores))

                    if home_score > away_score:
                        team_expected[matchup["home_team"]] += 1.0 / n
                    else:
                        team_expected[matchup["away_team"]] += 1.0 / n

                    sims_completed += 1

        team_actual = {t: 0 for t in self.teams}
        for idx, week in enumerate(self.schedule):
            if idx + 1 > self.league.current_week:
                break
            for matchup in week:
                if matchup["home_team_score"] == 0.0 or matchup["away_team_score"] == 0.0:
                    # This is a week that hasn't finished yet
                    continue

                if matchup["home_team_score"] > matchup["away_team_score"]:
                    team_actual[matchup["home_team"]] += 1
                else:
                    team_actual[matchup["away_team"]] += 1

        pt = PT()
        pt.title = "Expected Wins"
        pt.field_names = ["Team", "xWins", "Wins", "Over Performance"]
        sortable_list = [
            [
                team,
                round(team_expected[team], 2),
                round(team_actual[team], 2),
                round(team_actual[team] - team_expected[team], 2),
            ]
            for team in team_expected.keys()
        ]
        sortable_list = sorted(sortable_list, key=lambda row: row[3], reverse=True)
        pt.add_rows(sortable_list)
        print(pt)
        return

    def run(self, n: int) -> tuple[dict, dict]:
        for i in range(n):
            if i % 50 == 0:
                if False:
                    print(f"Simulation #{i}")
            results = self._run_single_simulation()
            self.raw_results.append(results)
            sorted_results = results.get_sorted_results()
            for idx, team in enumerate(sorted_results):
                team_name = team[0]
                self.regular_season_simulation_results[team_name][idx] += 1.0 / n

            for idx, team in enumerate(results.final_results):
                self.playoff_simulation_results[team][idx] += 1.0 / n

        return self.regular_season_simulation_results, self.playoff_simulation_results

    def print_regular_season_projected_win_losses(self) -> None:
        expected_results = {t: Standing() for t in self.teams}
        n = len(self.raw_results)
        for result in self.raw_results:
            for team, standing in result.standings.items():
                expected_results[team].wins += standing.wins / len(self.raw_results)
                expected_results[team].losses += standing.losses / len(self.raw_results)
                expected_results[team].points_against += standing.points_against / len(self.raw_results)
                expected_results[team].points_scored += standing.points_scored / len(self.raw_results)

        pt = PT()
        pt.title = "Projected Standings"
        pt.field_names = ["Team", "Wins", "Losses", "PF", "PA"]

        sortable_list = []
        for team, standing in expected_results.items():
            sortable_list.append(
                [
                    team,
                    round(standing.wins, 2),
                    round(standing.losses, 2),
                    round(standing.points_scored, 2),
                    round(standing.points_against, 2),
                ]
            )
        sortable_list = sorted(sortable_list, key=lambda row: row[1:], reverse=True)

        pt.add_rows(sortable_list)
        print(pt)

    def print_regular_season_predictions(self) -> None:
        pt = PT()
        pt.title = "Final Position Probability"
        pt.field_names = [
            "Team",
            "1st %",
            "2nd %",
            "3rd %",
            "4th %",
            "5th %",
            "6th %",
            "7th %",
            "8th %",
            "9th %",
            "10th %",
        ]

        sortable_list = []

        for team, result in self.regular_season_simulation_results.items():
            sortable_list.append(flatten_extend([[team], [round(r * 100, 2) for r in result]]))
        sortable_list = sorted(sortable_list, key=lambda row: row[1:])  # Sort by last place chances
        pt.add_rows(sortable_list)

        print(pt)

        # Playoff odds
        pt = PT()
        pt.title = "Playoff Odds"
        pt.field_names = ["Team", "Odds"]
        sortable_list = sorted(sortable_list, key=lambda row: sum(row[1:7]), reverse=True)  # Sort by playoff odds
        for row in sortable_list:
            pt.add_row([row[0], sum(row[1:7])])
        print(pt)

    def print_playoff_predictions(self) -> None:
        pt = PT()
        pt.title = "Playoff Results Probability"
        pt.field_names = [
            "Team",
            "1st %",
            "2nd %",
            "3rd %",
            "4th %",
            "5th %",
            "6th %",
        ]

        sortable_list = []

        for team, result in self.playoff_simulation_results.items():
            sortable_list.append(flatten_extend([[team], [round(r * 100, 2) for r in result[:6]]]))
        sortable_list = sorted(sortable_list, key=lambda row: row[1:], reverse=True)  # Sort by last place chances
        pt.add_rows(sortable_list)

        print(pt)

    def _matchup_is_completed(self, matchup: dict[str, any]) -> bool:
        return matchup["home_team_score"] != 0.0 and matchup["away_team_score"] != 0.0

    def _run_single_simulation(self) -> SingleSeasonSimulationResults:
        # TODO seankane: this needs to be reimplemented to predict scores in a better way
        results = SingleSeasonSimulationResults(self.teams)

        # Regular season simulation
        for week, scoreboard in self.matchup_data.items():
            for matchup in scoreboard:
                home_roster = Roster(matchup["home_lineup"])
                away_roster = Roster(matchup["away_lineup"])

                if int(week) > self.starting_week:
                    if matchup["home_team_score"] > matchup["away_team_score"]:
                        results.team_win(matchup["home_team"], matchup["home_team_score"], matchup["away_team_score"])
                        results.team_loss(matchup["away_team"], matchup["away_team_score"], matchup["home_team_score"])
                    else:
                        results.team_win(matchup["away_team"], matchup["away_team_score"], matchup["home_team_score"])
                        results.team_loss(matchup["home_team"], matchup["home_team_score"], matchup["away_team_score"])
                    continue

                home_score = home_roster.simulate_projected_score(self.position_stats)
                away_score = away_roster.simulate_projected_score(self.position_stats)

                if home_score > away_score:
                    results.team_win(matchup["home_team"], home_score, away_score)
                    results.team_loss(matchup["away_team"], away_score, home_score)
                else:
                    results.team_win(matchup["away_team"], away_score, home_score)
                    results.team_loss(matchup["home_team"], home_score, away_score)

        # return results
        # Playoff selection
        # Have to do the whole east and west thing
        playoff_teams = results.select_playoff_teams()

        # Get the most recent roster from each playoff team

        # Simulate 4th and 5th place game
        fourth = playoff_teams[3]
        fifth = playoff_teams[4]
        winner, fifth_place = self.simulate_game(fourth, fifth)

        # Simulate 3rd and 6th place game
        third = playoff_teams[2]
        sixth = playoff_teams[5]
        winner2, sixth_place = self.simulate_game(third, sixth)

        # Simulate 1st and 4th/5th
        first = playoff_teams[0]
        champ_game1, third_place_game1 = self.simulate_game(first, winner)

        # Simulate 2nd and 3rd/6th
        second = playoff_teams[1]
        champ_game2, third_place_game2 = self.simulate_game(second, winner2)

        # Simulate 3rd place game
        third_place, fourth_place = self.simulate_game(third_place_game1, third_place_game2)

        # Simulate Championship game
        winner, loser = self.simulate_game(champ_game1, champ_game2)

        results.set_playoff_results([winner, loser, third_place, fourth_place, fifth_place, sixth_place])

        return results

    def simulate_game(self, team1: str, team2: str) -> tuple[str, str]:
        """Simulate the game between two teams, returning a tuple of (winner, loser)"""
        team1_obj = self._get_team_object(team1)
        team2_obj = self._get_team_object(team2)

        team1_points = self._simulate_from_roster(team1_obj.roster)
        team2_points = self._simulate_from_roster(team2_obj.roster)

        if team1_points > team2_points:
            return team1, team2
        return team2, team1

    def _get_team_object(self, team1: str) -> Team:
        team_id = self._team_to_id[team1]
        for team in self.league.teams:
            if team.team_id == team_id:
                return team
        return None

    def _simulate_from_roster(self, roster: list[Player]) -> float:
        """For each player, find average projection and do a normal distribution sampling of that player, then pick the best roster"""
        sim_roster = []
        for player in roster:
            sim_roster.append(
                {
                    "name": player.name,
                    "position": player.position,
                    "status": player.position,
                    "projection": player.projected_total_points / 17,
                    "actual": 0.0,
                    "diff": 0.0,
                }
            )
        r = Roster(sim_roster)
        return r.simulate_projected_score(self.position_stats)


def _get_teams_from_annual_data(matchup_data: dict[str, any]) -> tuple[list[str], dict[str, int]]:
    """Returns a list of all teams and a mapping of team name to team id"""
    teams = []
    team_ids = {}
    for week, scoreboard in matchup_data.items():
        for matchup in scoreboard:
            teams.append(matchup["home_team"])
            teams.append(matchup["away_team"])
            team_ids[matchup["home_team"]] = matchup["home_team_id"]
            team_ids[matchup["away_team"]] = matchup["away_team_id"]
        return teams, team_ids


# TODO seankane: to improve how I predict a teams scores I'd like to use the individual players on the roster and see the odds they outperform
# their projection. For now, use the average score of a team and average score of the entire league.
def _get_team_scores_from_annual_data(matchup_data: dict[str, any]) -> dict[str, list[float]]:
    all_scores = []
    ret = {}
    for _, scoreboard in matchup_data.items():
        for matchup in scoreboard:
            if matchup["home_team_score"] == 0.0 or matchup["away_team_score"] == 0.0:
                continue
            try:
                ret[matchup["home_team"]].append(matchup["home_team_score"])
            except KeyError:
                ret[matchup["home_team"]] = [matchup["home_team_score"]]

            try:
                ret[matchup["away_team"]].append(matchup["away_team_score"])
            except KeyError:
                ret[matchup["away_team"]] = [matchup["away_team_score"]]

            all_scores.append(matchup["home_team_score"])
            all_scores.append(matchup["away_team_score"])

    ret["ALL_SCORES"] = all_scores
    return ret
