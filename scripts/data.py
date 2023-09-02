import json
import os
import logging
import sys

from dotenv import find_dotenv, load_dotenv
from espn_api.football import League, BoxPlayer
from prettytable import PrettyTable
from scipy import stats

from models.roster import Roster
from utils import write_to_file, mean, std_dev, flatten_extend


load_dotenv(find_dotenv())

SWID = os.environ.get("SWID")
ESPN_S2 = os.environ.get("ESPN_S2")

YEARS = [2022]


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


# TODO seankane: Update this to take the full data table, meaning it'll have to calculate everything frest.
def calc_team_overperformance(data: dict[str, list[float]]) -> None:
    # Print out averages for ESPN diff
    pt = PrettyTable()
    pt.field_names = ["#", "Team", "Total Difference"]

    rows = []
    for team in data:
        rows.append([team, round(sum(data[team]), 2)])

    rows = sorted(rows, key=lambda row: row[1], reverse=True)

    sum_out_perform = []
    for idx, row in enumerate(rows):
        pt.add_row([idx + 1, row[0], row[1]])
        sum_out_perform.append(row[1])
    pt.add_row(["", "", ""])
    pt.add_row(["", "Average", sum(sum_out_perform) / len(sum_out_perform)])

    print("ESPN Accuracy by Team")
    print(pt)


def add_positional_diffs(diffs_per_position: dict[str, float], lineup: dict[str, any]) -> None:
    for player in lineup:
        diff = player["actual"] - player["projection"]
        try:
            diffs_per_position[player["position"]].append(diff)
        except KeyError:
            diffs_per_position[player["position"]] = [diff]
    return


# TODO seankane: Update this to take the full data table, meaning it'll have to calculate everything frest.
# Get the rosters per week and look at projection vs actual by position
def calc_position_performances(data: dict[str, list[float]]) -> None:
    diff_per_position = {}

    matchup_data = data["matchup_data"]
    for _, year_data in matchup_data.items():
        for _, matchups in year_data.items():
            for matchup in matchups:
                add_positional_diffs(diff_per_position, matchup["home_lineup"])
                add_positional_diffs(diff_per_position, matchup["away_lineup"])

    pt = PrettyTable()
    pt.field_names = ["Position", "Average Difference", "Std. Dev.", "P-Value"]

    all_diffs = []
    rows = []
    idx = 1
    for pos, items in diff_per_position.items():
        normal_test = stats.normaltest(items)
        rows.append([round(mean(items), 2), round(std_dev(items), 2), normal_test.pvalue])
        all_diffs = flatten_extend([all_diffs, items])
        idx += 1

    rows = sorted(rows, key=lambda row: row[1], reverse=True)

    for idx, row in enumerate(rows):
        pt.add_row(flatten_extend([[idx], row]))

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

    print("ESPN Accuracy by Position")
    print(pt)


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


def scrape_matchups(file_name: str = "history.json", years=YEARS) -> dict[str, any]:
    """Scrape all matchup data from 2017 to 2020"""

    all_data = {}

    if os.path.isfile(file_name):
        # Read this file and return the data
        logging.info(f"found existing data, remove {file_name} to regen")
        f = open(file_name)
        try:
            return json.load(f), League(league_id=345674, year=years[0], swid=SWID, espn_s2=ESPN_S2, debug=False)
        except json.decoder.JSONDecodeError:
            pass

    PRINT_STR = "Year: {}\tWeek: {}"

    for year in years:
        matchup_data = {}

        matchup_data[year] = {}
        league = League(league_id=345674, year=year, swid=SWID, espn_s2=ESPN_S2, debug=False)

        diffs = {}
        positional_diffs = {}

        for week in range(1, 15):
            matchup_data[year][week] = []
            print(PRINT_STR.format(year, week))
            for box_score in league.box_scores(week):
                home_owner = box_score.home_team.owner.rstrip(" ")
                away_owner = box_score.away_team.owner.rstrip(" ")
                matchup_data[year][week].append(
                    {
                        "home_team": home_owner,
                        "away_team": away_owner,
                        "home_team_score": box_score.home_score,
                        "home_team_projected_score": box_score.home_projected,
                        "away_team_score": box_score.away_score,
                        "away_team_projected_score": box_score.away_projected,
                        "home_lineup": get_lineup_dict(box_score.home_lineup),
                        "away_lineup": get_lineup_dict(box_score.away_lineup),
                    }
                )
                if box_score.home_score > 0 and box_score.away_score > 0:
                    try:
                        diffs[home_owner].append(box_score.home_score - box_score.home_projected)
                    except KeyError:
                        diffs[home_owner] = [box_score.home_score - box_score.home_projected]

                    try:
                        diffs[away_owner].append(box_score.away_score - box_score.away_projected)
                    except KeyError:
                        diffs[away_owner] = [box_score.away_score - box_score.away_projected]

                for player in get_lineup_dict(box_score.home_lineup):
                    try:
                        positional_diffs[player["position"]].append(player["diff"])
                    except KeyError:
                        positional_diffs[player["position"]] = [player["diff"]]

                for player in get_lineup_dict(box_score.away_lineup):
                    try:
                        positional_diffs[player["position"]].append(player["diff"])
                    except KeyError:
                        positional_diffs[player["position"]] = [player["diff"]]

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

        output_data = {
            "matchup_data": matchup_data,
            "draft_data": draft_data,
        }

        all_data[year] = matchup_data

    return output_data, league


def perform_roster_analysis(data: dict[str, any]) -> None:
    matchup_data = data["matchup_data"]
    points_left_on_bench = {}

    for year in YEARS:
        for week, matchups in matchup_data[str(year)].items():
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


# TODO list:
#  * Fix up current todos
#  * Add a season simulator
#    * Probably need to migrate to using a poisson distribution
#    * Add a best lineup picker
#    * Add a random point generator per position based on ESPN accuracy
#    * Last place chances
#    * Playoff odds
#  * Add a trade analyzer
#  * Add a waiver wire pickup analyzer
#  * Rank value of upcoming games. 538 does this, but can't find anything about the methodology.
#  * Weekly roster analysis - Done
#    * What players should have been played - Done
#    * How many perfect rosters were chosen - Done

# Hope I remember later: A graph that shows who you should have picked at each spot, so for 1 - ?? it would be mahomes, then the next highest scorer
#  Should probably do this w/ and without QBs.

if __name__ == "__main__":
    logging.info("Scraping fantasy football data from ESPN")
    data, league = scrape_matchups()

    try:
        logging.info("calculating stats about the draft")

        perform_draft_analytics(data, league)

        perform_roster_analysis(data)

        logging.info("calculating basic statistics for positional data")
        calc_position_performances(data)

        raise Exception("early quit")

        logging.info("calculating overperformance by team")
        calc_team_overperformance(data)

    finally:
        write_to_file(data)
