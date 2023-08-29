import math
import json
import os

from dotenv import find_dotenv, load_dotenv
from espn_api.football import League, BoxPlayer
from prettytable import PrettyTable

load_dotenv(find_dotenv())

SWID = os.environ.get("SWID")
ESPN_S2 = os.environ.get("ESPN_S2")


def flatten_extend(matrix):
    flat_list = []
    for row in matrix:
        flat_list.extend(row)
    return flat_list


def get_lineup_dict(box_score: list[BoxPlayer]):
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


def calc_position_performances(data: dict[str, list[float]]) -> None:
    pt = PrettyTable()
    pt.field_names = ["#", "Position", "Total Difference"]

    rows = []
    for pos in data:
        rows.append([pos, round(sum(data[pos]), 2)])

    rows = sorted(rows, key=lambda row: row[1], reverse=True)

    sum_out_perform = []
    for idx, row in enumerate(rows):
        pt.add_row([idx + 1, row[0], row[1]])
        sum_out_perform.append(row[1])

    pt.add_row(["", "", ""])
    pt.add_row(["", "Average", sum(sum_out_perform) / len(sum_out_perform)])

    print("ESPN Accuracy by Position")
    print(pt)


def scrape_matchups():
    """Scrape all matchup data from 2017 to 2020"""
    all_data = {}
    years = [2022]

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

        calc_team_overperformance(diffs)
        calc_position_performances(positional_diffs)

        # draft stuff
        draft_data = []
        for pick in league.draft:
            draft_data.append(
                {
                    "player_name": pick.playerName,
                    "player_id": pick.playerId,
                    "team": pick.team.team_id,
                    "round_number": pick.round_num,
                    "round_pick": pick.round_pick,
                }
            )

        output_data = {
            "matchup_data": matchup_data,
            "draft_data": draft_data,
        }

        all_data[year] = matchup_data

    with open("history.json", mode="w") as f:
        json.dump(output_data, f, indent=4)


def std_dev(arr: list[int | float]) -> int | float:
    if len(arr) == 0:
        return 0
    avg = sum(arr) / len(arr)
    sum_squares = sum([(x - avg) * (x - avg) for x in arr])
    return math.pow(sum_squares / len(arr), 0.5)


scrape_matchups()
