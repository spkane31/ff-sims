import json
import logging
import os
import sys
import time

from dotenv import find_dotenv, load_dotenv
from espn_api.football import League, BoxPlayer
from prettytable import PrettyTable
from scipy import stats

from models.roster import Roster
from simulation import SeasonSimulation
from utils import write_to_file, mean, std_dev, flatten_extend


load_dotenv(find_dotenv())

SWID = os.environ.get("SWID")
ESPN_S2 = os.environ.get("ESPN_S2")

PRINT_STR = "Year: {}\tWeek: {}"


if os.environ.get("DEBUG_LEVEL") != "" and False:
    root = logging.getLogger()
    root.setLevel(logging.DEBUG)

    handler = logging.StreamHandler(sys.stdout)
    handler.setLevel(logging.DEBUG)
    formatter = logging.Formatter("%(asctime)s - %(name)s - %(levelname)s - %(message)s")
    handler.setFormatter(formatter)
    root.addHandler(handler)


def get_lineup_dict(box_score: list[BoxPlayer]) -> list[dict[str, any]]:
    return [
        {
            "name": player.name,
            "projection": player.projected_points,
            "actual": player.points,
            "diff": player.points - player.projected_points,
            "position": player.position,
            "status": player.slot_position,
        }
        for player in box_score
    ]


def calc_team_overperformance(data: dict[str, list[float]], current_week: int) -> None:
    """calc_team_overperformances calculates how much a teams starters outperform the ESPN projections"""

    differences_by_team = {}

    matchup_data = data["matchup_data"]
    for week, matchups in matchup_data.items():
        if int(week) > current_week:
            break
        for match_result in matchups:
            home_roster = Roster(match_result["home_lineup"])
            away_roster = Roster(match_result["away_lineup"])
            try:
                differences_by_team[match_result["home_team"]].append(
                    home_roster.points_scored() - home_roster.projected_score()
                )
            except KeyError:
                differences_by_team[match_result["home_team"]] = [
                    home_roster.points_scored() - home_roster.projected_score()
                ]

            try:
                differences_by_team[match_result["away_team"]].append(
                    away_roster.points_scored() - away_roster.projected_score()
                )
            except KeyError:
                differences_by_team[match_result["away_team"]] = [
                    away_roster.points_scored() - away_roster.projected_score()
                ]

    pt = PrettyTable()
    pt.field_names = ["#", "Team", "Total Difference", "Avg. Difference"]

    rows = []
    for team, data in differences_by_team.items():
        rows.append(
            [
                team,
                round(sum(data), 2),
                round(mean(data), 2),
            ]
        )

    rows = sorted(rows, key=lambda row: row[1], reverse=True)

    for idx, row in enumerate(rows):
        pt.add_row(flatten_extend([[idx], row]))

    pt.title = "ESPN Accuracy by Team (Actual - Projected score)"
    print(pt)


def add_positional_diffs(diffs_per_position: dict[str, float], lineup: dict[str, any]) -> None:
    for player in lineup:
        diff = player["actual"] - player["projection"]
        if player["projection"] != 0:
            diff = diff / player["projection"]
        try:
            diffs_per_position[player["position"]].append(diff)
        except KeyError:
            diffs_per_position[player["position"]] = [diff]
    return


# Get the rosters per week and look at projection vs actual by position
# Returns a dictionary mapping position to a (mean, std_dev) tuple
def calc_position_performances(data: dict[str, list[float]]) -> None:
    positional_data = {}
    diff_per_position = {}

    matchup_data = data["matchup_data"]
    for _, matchups in matchup_data.items():
        for matchup in matchups:
            add_positional_diffs(diff_per_position, matchup["home_lineup"])
            add_positional_diffs(diff_per_position, matchup["away_lineup"])

    pt = PrettyTable()
    pt.field_names = ["Position", "Average Difference (%)", "Std. Dev.", "P-Value"]

    pct_string = "%"  # Stupid python hack because you cannot put a '%' in an f-string
    all_diffs = []
    rows = []
    idx = 1
    for pos, items in diff_per_position.items():
        normal_test = stats.normaltest(items)
        rows.append([pos, f"{round(100 * mean(items), 2)} {pct_string}", round(std_dev(items), 2), normal_test.pvalue])
        all_diffs = flatten_extend([all_diffs, items])
        idx += 1
        positional_data[pos] = (mean(items), std_dev(items))

    rows = sorted(rows, key=lambda row: row[1], reverse=True)

    for idx, row in enumerate(rows):
        pt.add_row(row)

    pt.add_row(
        [
            "",
            "",
            "",
            "",
        ]
    )

    normal_test = stats.normaltest(all_diffs)
    pt.add_row(["Average", round(mean(all_diffs), 2), round(std_dev(all_diffs), 2), normal_test.pvalue])

    pt.title = "ESPN Accuracy by Position"
    print(pt)

    return positional_data


# Performs two calculations on draft data:
# Which drafted team scored the most points
# Which player was the best pick in each round
def perform_draft_analytics(data: dict[str, any], league: League):
    points_per_team = {}
    best_pick_per_round = {}

    for pick in data["draft_data"]:

        round_number = pick["round_number"]
        round_pick = pick["round_pick"]
        player_name = pick["player_name"]
        team_name = pick["team_name"]

        try:
            player_points = pick["total_points"]
        except KeyError:
            # Get total points and add to dict
            player = league.player_info(playerId=pick["player_id"])
            player_points = player.total_points
            pick["total_points"] = player_points

            print(
                f"Draft position: {((round_number - 1) * 10) + round_pick}, player: {player_name}, total points: {player_points}"
            )

        # Points per team
        try:
            points_per_team[team_name] += player_points
        except KeyError:
            points_per_team[team_name] = player_points

        # Best pick per round
        try:
            player = best_pick_per_round[round_number]
            if player[3] < player_points:
                best_pick_per_round[round_number] = [round_number, team_name, player_name, player_points]
        except KeyError:
            best_pick_per_round[round_number] = [round_number, team_name, player_name, player_points]

    # Sort and print total points per draft
    sortable_list = []
    for team_name in points_per_team:
        sortable_list.append([team_name, points_per_team[team_name]])
    sortable_list = sorted(sortable_list, key=lambda row: row[1], reverse=True)

    pt = PrettyTable()
    pt.field_names = ["#", "Team", "Total Drafted Points"]
    pt.title = "Draft Performance"

    for idx, row in enumerate(sortable_list):
        pt.add_row([idx + 1, row[0], round(row[1], 2)])

    print(pt)

    sortable_list = []
    for round_number in best_pick_per_round:
        sortable_list.append(best_pick_per_round[round_number])
    sortable_list = sorted(sortable_list, key=lambda row: row[0], reverse=False)

    pt = PrettyTable()
    pt.field_names = ["Round #", "Team Name", "Player", "Points"]
    pt.title = "Best Pick per Round"

    for idx, row in enumerate(sortable_list):
        pt.add_row([idx + 1, row[1], row[2], row[3]])

    print(pt)

    return


def scrape_matchups(file_name: str = "history.json", year=2023, debug=False) -> dict[str, any]:
    """Scrape all matchup data from 2017 to 2020"""

    if os.path.isfile(file_name):
        # Read this file and return the data
        logging.info(f"found existing data, remove {file_name} to regen")
        f = open(file_name)
        try:
            return json.load(f), League(league_id=345674, year=year, swid=SWID, espn_s2=ESPN_S2, debug=debug)
        except json.decoder.JSONDecodeError:
            pass

    matchup_data = {}

    league = League(league_id=345674, year=year, swid=SWID, espn_s2=ESPN_S2, debug=debug)

    for week in range(1, 15):
        matchup_data[week] = []
        print(PRINT_STR.format(year, week))
        # NOTE seankane: This might not work for current leagues, only for past leagues in which case will have to simulate in a different way.
        # If that is the case, I will be very sad
        for box_score in league.box_scores(week):
            home_owner = box_score.home_team.owner.rstrip(" ")
            away_owner = box_score.away_team.owner.rstrip(" ")
            matchup_data[week].append(
                {
                    "home_team": home_owner,
                    "home_team_id": box_score.home_team.team_id,
                    "away_team": away_owner,
                    "away_team_id": box_score.away_team.team_id,
                    "home_team_score": box_score.home_score,
                    "home_team_projected_score": box_score.home_projected,
                    "away_team_score": box_score.away_score,
                    "away_team_projected_score": box_score.away_projected,
                    "home_lineup": get_lineup_dict(box_score.home_lineup),
                    "away_lineup": get_lineup_dict(box_score.away_lineup),
                }
            )

    # draft stuff
    draft_data = []
    for pick in league.draft:
        draft_data.append(
            {
                "player_name": pick.playerName,
                "player_id": pick.playerId,
                "team": pick.team.team_id,
                "team_name": pick.team.team_name,
                "round_number": pick.round_num,
                "round_pick": pick.round_pick,
            }
        )

    activities = []
    # Waiver wire and draft activity
    # for offset in [0, 25, 50, 75]:
    #     recent_activity = league.recent_activity(25, offset=offset)
    #     for activity in recent_activity:
    #         print(activity)
    #         activities.append(
    #             {
    #                 "date": activity.date,
    #                 "actions": [
    #                     {
    #                         "team": action[0].team_name,
    #                         "action": action[1],
    #                         "player": {"name": action[2].name, "player_id": action[2].playerId},
    #                     }
    #                     for action in activity.actions
    #                 ],
    #             }
    #         )

    output_data = {
        "matchup_data": matchup_data,
        "draft_data": draft_data,
        "activity_data": activities,
    }

    return output_data, league


def perform_roster_analysis(data: dict[str, any], current_week: int) -> None:
    matchup_data = data["matchup_data"]
    points_left_on_bench = {}

    print("Perfect Rosters:")
    for week, matchups in matchup_data.items():
        if int(week) >= current_week:
            break

        for matchup in matchups:
            home_roster = Roster(matchup["home_lineup"])
            away_roster = Roster(matchup["away_lineup"])

            home_diff = home_roster.maximum_points() - home_roster.points_scored()
            try:
                points_left_on_bench[matchup["home_team"]] += home_diff
            except KeyError:
                points_left_on_bench[matchup["home_team"]] = home_diff

            away_diff = away_roster.maximum_points() - away_roster.points_scored()
            try:
                points_left_on_bench[matchup["away_team"]] += away_diff
            except KeyError:
                points_left_on_bench[matchup["away_team"]] = away_diff

            if home_diff == 0.0:
                print(f"Perfect roster by {matchup['home_team']} in week {week}")
            if away_diff == 0.0:
                print(f"Perfect roster by {matchup['away_team']} in week {week}")

    print()
    pt = PrettyTable()
    pt.field_names = ["", "Team Name", "Points Left on Bench"]
    pt.title = "Points left on Bench"

    sortable_list = []
    for team_name, pts in points_left_on_bench.items():
        sortable_list.append([team_name, pts])
    sortable_list = sorted(sortable_list, key=lambda p: p[1], reverse=True)

    idx = 1
    for sl in sortable_list:
        pt.add_row([idx, sl[0], round(sl[1], 2)])
        idx += 1

    print(pt)

    return None


def run_monte_carlo_simulation_from_week(
    league: League,
    data: dict[str, any],
    positional_data: dict[str, tuple[float, float]],
    week: int = None,
    n: int = 10000,
) -> tuple[dict, dict]:
    if not week:
        week = league.current_week
    season_data = data["matchup_data"]

    season_simulation = SeasonSimulation(season_data, positional_data, league, starting_week=week)
    reg, playoff = season_simulation.run(100)
    # season_simulation.print_regular_season_projected_win_losses()
    season_simulation.expected_wins()
    # season_simulation.print_regular_season_predictions()
    # season_simulation.print_playoff_predictions()

    return reg, playoff


# TODO list:
#  * Add a season simulator
#    * Last place chances
#    * Playoff odds
#  * Save data to JSON .
#  * Add a trade analyzer
#  * Add a waiver wire pickup analyzer
#  * Rank value of upcoming games. 538 does this, but can't find anything about the methodology.

# Hope I remember later: A graph that shows who you should have picked at each spot, so for 1 - ?? it would be mahomes, then the next highest scorer
#  Should probably do this w/ and without QBs.

if __name__ == "__main__":
    start = time.time()
    logging.info("Scraping fantasy football data from ESPN")
    data, league = scrape_matchups()

    try:
        logging.info("calculating stats about the draft")

        perform_draft_analytics(data, league)

        perform_roster_analysis(data, league.current_week)

        logging.info("calculating overperformance by team")
        calc_team_overperformance(data, league.current_week)

        logging.info("calculating basic statistics for positional data")
        position_data = calc_position_performances(data)
        data["position_data"] = position_data

        regular_season_results, playoff_results = run_monte_carlo_simulation_from_week(league, data, position_data, n=1)
        data["regular_season_results"] = regular_season_results
        data["playoff_results"] = playoff_results

    finally:
        write_to_file(data)
        print(f"Completed in {round(time.time() - start,2)} seconds")
