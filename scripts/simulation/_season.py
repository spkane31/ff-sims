from prettytable import PrettyTable as PT


from models import Roster

from utils import flatten_extend

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

        return playoff_teams

    def _top_team_in_division(self, sorted_teams: list[list], division: str) -> list:
        division_teams = _DIVISIONAL_BREAKDOWN[division]
        for team in sorted_teams:
            if team in division_teams:
                return team


class SeasonSimulation:
    def __init__(self, data: dict[str, any], position_data: dict[str, tuple[float, float]]) -> None:
        self.teams = _get_teams_from_annual_data(data)
        self.position_stats = position_data
        self.standings: dict[str, Standing] = {}
        self.simulation_results: dict[str, list[int]] = {}
        self.matchup_data = data

        for team in self.teams:
            self.standings[team] = Standing()
            self.simulation_results[team] = [
                0 for t in self.teams
            ]  # Counting of how many times a team is in each position.
        self._validate_teams()

    def _validate_teams(self) -> None:
        """This function ensures each team is in one of the divisions. I don't know of a way to get this data from the API, so this is a work around"""

        for team in self.teams:
            flag = False
            for teams in _DIVISIONAL_BREAKDOWN.values():
                if team in teams:
                    flag = True

            if not flag:
                raise ValueError(f"team `{team}` not in the stated divisions")

    def run(self, n: int) -> None:
        for i in range(n):
            if i % 25 == 0:
                print(f"Simulation #{i}")
            results = self._run_single_simulation()
            sorted_results = results.get_sorted_results()
            for idx, team in enumerate(sorted_results):
                team_name = team[0]
                self.simulation_results[team_name][idx] += 1.0 / n

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

        for team, result in self.simulation_results.items():
            sortable_list.append(flatten_extend([[team], [round(r * 100, 2) for r in result]]))
        sortable_list = sorted(sortable_list, key=lambda row: row[-1])  # Sort by last place chances
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

    def _run_single_simulation(self) -> SingleSeasonSimulationResults:
        results = SingleSeasonSimulationResults(self.teams)

        # Regular season simulation
        for week, scoreboard in self.matchup_data.items():
            for matchup in scoreboard:
                home_roster = Roster(matchup["home_lineup"])
                away_roster = Roster(matchup["away_lineup"])

                home_score = home_roster.simulate_projected_score(self.position_stats)
                away_score = away_roster.simulate_projected_score(self.position_stats)

                if home_score > away_score:
                    results.team_win(matchup["home_team"], home_score, away_score)
                    results.team_loss(matchup["away_team"], away_score, home_score)
                else:
                    results.team_win(matchup["away_team"], away_score, home_score)
                    results.team_loss(matchup["home_team"], home_score, away_score)

        return results
        # Playoff selection
        # Have to do the whole east and west thing
        playoff_teams = results.select_playoff_teams()

        # Get the most recent roster from each playoff team

        # Simulate 4th and 5th place game
        fourth = playoff_teams[3]
        fifth = playoff_teams[4]

        # Simulate 3rd and 6th place game

        # Simulate 1st and 4th/5th

        # Simulate 2nd and 3rd/6th

        # Simulate 3rd place game

        # Simulate Championship game

        return results


def _get_teams_from_annual_data(matchup_data: dict[str, any]) -> list[str]:
    teams = []
    for week, scoreboard in matchup_data.items():
        for matchup in scoreboard:
            teams.append(matchup["home_team"])
            teams.append(matchup["away_team"])
        return teams
