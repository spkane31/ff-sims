import math
import json
import os

from dotenv import find_dotenv, load_dotenv
from espn_api.football import League, matchup

# import necessary packages
import matplotlib.pyplot as plt

load_dotenv(find_dotenv())

SWID = os.environ.get("SWID")
ESPN_S2 = os.environ.get("ESPN_S2")


def scrape_matchups():
    """Scrape all matchup data from 2017 to 2020"""
    all_data = {}
    years = [2022]

    PRINT_STR = "Year: {}\tWeek: {}"

    for year in years:
        matchup_data = {}

        matchup_data[year] = {}
        league = League(league_id=345674, year=year, swid=SWID, espn_s2=ESPN_S2)

        diffs = []

        for week in range(1, 15):
            matchup_data[year][week] = []
            print(PRINT_STR.format(year, week))
            for box_score in league.box_scores(week):
                matchup_data[year][week].append(
                    {
                        "home_team": box_score.home_team.owner.rstrip(" "),
                        "away_team": box_score.away_team.owner.rstrip(" "),
                        "home_team_score": box_score.home_score,
                        "home_team_projected_score": box_score.home_projected,
                        "away_team_score": box_score.away_score,
                        "away_team_projected_score": box_score.away_projected,
                    }
                )
                if box_score.home_score > 0 and box_score.away_score > 0:
                    diffs.append(box_score.home_score - box_score.home_projected)
                    diffs.append(box_score.away_score - box_score.away_projected)

        # Print out averages for ESPN diff
        print(f"Average difference in scores: {sum(diffs) / len(diffs)}")
        print(f"Std. Dev. in scores vs projected: {std_dev(diffs)}")

        # Enable later
        # plotting labelled histogram
        # plt.hist(diffs, density=True)
        # plt.xlabel('score differential')
        # plt.ylabel('Count')
        # plt.savefig("img.png")

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
