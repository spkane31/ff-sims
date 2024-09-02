import json
import logging
import os
import random
import sys
import time

from dotenv import find_dotenv, load_dotenv
from espn_api.football import League, BoxPlayer
from prettytable import PrettyTable
from scipy import stats
import psycopg2

from models.roster import Roster
from models.activity import get_waiver_wire_activity, perform_waiver_analysis
from database.tables import initialize
from simulation import SeasonSimulation, SingleSeasonSimulationResults
from utils import write_to_file, mean, std_dev, flatten_extend


load_dotenv(find_dotenv())

SWID = os.environ.get("SWID")
ESPN_S2 = os.environ.get("ESPN_S2")


PRINT_STR = "Year: {}\tWeek: {}"

conn = psycopg2.connect(os.environ["COCKROACHDB_URL"])

with conn.cursor() as cur:
    cur.execute("SELECT now()")
    res = cur.fetchall()
    conn.commit()
    print(res)

    cur.execute("SELECT count(*) FROM teams")
    res = cur.fetchall()
    conn.commit()
    print(f"Number of teams: {res[0][0]}")

    cur.execute("SELECT count(*) FROM matchups")
    res = cur.fetchall()
    conn.commit()
    print(f"Number of matchups: {res[0][0]}")

    cur.close()


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

    # TODO seankane: not sure any of this is right
    return

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
                round(sum(data) / (len(data) - 1), 2),
            ]
        )

    rows = sorted(rows, key=lambda row: row[1], reverse=True)

    for idx, row in enumerate(rows):
        pt.add_row(flatten_extend([[idx + 1], row]))

    pt.title = "ESPN Accuracy by Team (Actual - Projected score)"
    print(pt)


def add_positional_diff_as_pct(diffs_per_position: dict[str, float], lineup: dict[str, any]) -> None:
    for player in lineup:
        diff = player["actual"] - player["projection"]
        if player["projection"] != 0:
            diff = diff / player["projection"]
        try:
            diffs_per_position[player["position"]].append(diff)
        except KeyError:
            diffs_per_position[player["position"]] = [diff]
    return


def add_positional_diff_as_raw(diffs_per_position: dict[str, float], lineup: dict[str, any]) -> None:
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
    raw_diff_per_position = {}

    matchup_data = data["matchup_data"]
    for _, matchups in matchup_data.items():
        for matchup in matchups:
            add_positional_diff_as_pct(diff_per_position, matchup["home_lineup"])
            add_positional_diff_as_pct(diff_per_position, matchup["away_lineup"])
            add_positional_diff_as_raw(raw_diff_per_position, matchup["home_lineup"])
            add_positional_diff_as_raw(raw_diff_per_position, matchup["away_lineup"])

    pt = PrettyTable()
    pt.field_names = [
        "Position",
        "Average Difference (%)",
        "Total Difference",
        "Std. Dev.",
        "P-Value",
    ]

    pct_string = "%"  # Stupid python hack because you cannot put a '%' in an f-string
    all_diffs = []
    rows = []
    idx = 1
    for pos, items in diff_per_position.items():
        normal_test = stats.normaltest(items)
        rows.append(
            [
                pos,
                f"{round(100 * mean(items), 2)} {pct_string}",
                f"{round(sum(raw_diff_per_position[pos]), 2)}",
                round(std_dev(items), 2),
                normal_test.pvalue,
            ]
        )
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
            "",
        ]
    )

    normal_test = stats.normaltest(all_diffs)
    pt.add_row(
        [
            "Average",
            round(mean(all_diffs), 2),
            f"{round(sum(all_diffs), 2)}",
            round(std_dev(all_diffs), 2),
            normal_test.pvalue,
        ]
    )

    pt.title = "ESPN Accuracy by Position"
    print(pt)

    # TODO seankane: add a graph of the distribution of the data
    # One plot for the projection, one for actual. little histogram action

    return positional_data


def perform_redraft(draft_data: list[dict[str, any]]) -> None:
    sorted_draft = sorted(draft_data, key=lambda p: p["total_points"], reverse=True)

    remove_for_injury = set()
    remove_for_injury.add("Aaron Rodgers")
    remove_for_injury.add("Nick Chubb")
    remove_for_injury.add("J.K. Dobbins")

    redraft_difference = []
    for idx, data in enumerate(sorted_draft):
        if data["player_name"] in remove_for_injury:
            continue
        redraft_difference.append(
            [
                data["player_name"],
                data["team_name"],
                data["total_points"],
                idx + 1,
                ((data["round_number"] - 1) * 10) + data["round_pick"],
                ((data["round_number"] - 1) * 10) + data["round_pick"] - (idx + 1),
            ]
        )

    sorted_redraft = sorted(redraft_difference, key=lambda p: p[-1], reverse=True)

    pt = PrettyTable()
    pt.title = "Best/Worst Picks By Value"
    pt.field_names = [
        "Player Name",
        "Team Name",
        "Total Points",
        "Redraft Position",
        "Draft Position",
        "Difference",
    ]

    top_n = 10
    sorted_redraft = flatten_extend([sorted_redraft[:top_n], sorted_redraft[-top_n:]])

    for idx, row in enumerate(sorted_redraft):
        if idx == top_n:
            pt.add_row(["-", "-", "-", "-", "-", "-"])
        pt.add_row(row)

    print(pt)

    # Sort by team
    sorted_redraft = sorted(
        sorted(redraft_difference, key=lambda p: p[-1], reverse=True),
        key=lambda p: p[1],
        reverse=False,
    )

    pt = PrettyTable()
    pt.title = "Draft Performance By Team"
    pt.field_names = [
        "Player Name",
        "Team Name",
        "Total Points",
        "Redraft Position",
        "Draft Position",
        "Difference",
    ]
    pt.add_rows(sorted_redraft)
    print(pt)

    return


# Performs two calculations on draft data:
# Which drafted team scored the most points
# Which player was the best pick in each round
def perform_draft_analytics(data: dict[str, any], league: League):
    points_per_team = {}
    best_pick_per_round = {}
    worst_pick_per_round = {}
    points_per_round = {}

    pt = PrettyTable()
    pt.title = "Draft Selections"
    pt.field_names = ["Pick #", "Player", "Total Points", "Drafting Team"]

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

            pt.add_row(
                [
                    ((round_number - 1) * 10) + round_pick,
                    player_name,
                    player_points,
                    team_name,
                ]
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
                best_pick_per_round[round_number] = [
                    round_number,
                    team_name,
                    player_name,
                    player_points,
                ]
        except KeyError:
            best_pick_per_round[round_number] = [
                round_number,
                team_name,
                player_name,
                player_points,
            ]

        # Worst pick per round
        try:
            player = worst_pick_per_round[round_number]
            if player[2] > player_points:
                worst_pick_per_round[round_number] = [
                    team_name,
                    player_name,
                    player_points,
                ]
        except KeyError:
            worst_pick_per_round[round_number] = [team_name, player_name, player_points]

        # Points per round
        try:
            points_per_round[round_number] += player_points
        except KeyError:
            points_per_round[round_number] = player_points

    print(pt)

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
        sortable_list.append(flatten_extend([best_pick_per_round[round_number], worst_pick_per_round[round_number]]))
    sortable_list = sorted(sortable_list, key=lambda row: row[0], reverse=False)

    pt = PrettyTable()
    pt.field_names = [
        "Round #",
        "Team Name (Best)",
        "Player (Best)",
        "Points (Best)",
        "Team Name (Worst)",
        "Player (Worst)",
        "Points (Worst)",
        "Total Points in Round",
    ]
    pt.title = "Best/Worst Pick per Round"

    for idx, row in enumerate(sortable_list):
        pt.add_row(
            [
                idx + 1,
                row[1],
                row[2],
                row[3],
                row[4],
                row[5],
                row[6],
                f"{round(points_per_round[row[0]], 2)}",
            ]
        )

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
    schedule = []

    league = League(league_id=345674, year=year, swid=SWID, espn_s2=ESPN_S2, debug=debug)

    for week in range(1, 15):
        matchup_data[week] = []
        print(PRINT_STR.format(year, week))
        # NOTE seankane: This might not work for current leagues, only for past leagues in which case will have to simulate in a different way.
        # If that is the case, I will be very sad
        for box_score in league.box_scores(week):
            home_owner = box_score.home_team.team_name.rstrip(" ")
            away_owner = box_score.away_team.team_name.rstrip(" ")
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

        week_schedule = []
        for matchup in league.scoreboard(week):
            week_schedule.append(
                {
                    "home_team": matchup.home_team.team_name.rstrip(" "),
                    "home_team_score": matchup.home_score,
                    "away_team": matchup.away_team.team_name.rstrip(" "),
                    "away_team_score": matchup.away_score,
                }
            )
        schedule.append(week_schedule)

    # draft stuff
    draft_data = []
    for pick in league.draft:
        draft_data.append(
            {
                "player_name": pick.playerName,
                "player_id": pick.playerId,
                "team": pick.team.team_id,
                "team_name": pick.team.team_name.rstrip(" "),
                "round_number": pick.round_num,
                "round_pick": pick.round_pick,
            }
        )

    activities = get_waiver_wire_activity(league)

    output_data = {
        "matchup_data": matchup_data,
        "draft_data": draft_data,
        "activity_data": activities,
        "schedule": schedule,
    }

    return output_data, league


def get_waiver_wire_activity(league: League) -> list[dict[str, any]]:
    activities = []
    # Waiver wire and draft activity
    for offset in [0, 25, 50, 75]:
        recent_activity = league.recent_activity(25, offset=offset)
        for activity in recent_activity:
            activities.append(
                {
                    "date": activity.date,
                    "actions": [
                        {
                            "team": action[0].team_name,
                            "action": action[1],
                            "player": {
                                "name": action[2].name,
                                "player_id": action[2].playerId,
                            },
                        }
                        for action in activity.actions
                    ],
                }
            )

    return activities


def perform_roster_analysis(data: dict[str, any], current_week: int) -> None:
    matchup_data = data["matchup_data"]
    points_left_on_bench = {}
    points_left_on_bench_per_week = []

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

            if home_roster.points_scored() > away_roster.points_scored():
                margin = home_roster.points_scored() - away_roster.points_scored()
                points_left_on_bench_per_week.append(
                    [
                        str(week),
                        matchup["home_team"],
                        home_diff,
                        "WIN",
                        f"{round(margin, 2)}",
                    ]
                )
                points_left_on_bench_per_week.append(
                    [
                        str(week),
                        matchup["away_team"],
                        away_diff,
                        "LOSS",
                        f"{round(-margin, 2)}",
                    ]
                )
            else:
                margin = home_roster.points_scored() - away_roster.points_scored()
                points_left_on_bench_per_week.append(
                    [
                        str(week),
                        matchup["home_team"],
                        home_diff,
                        "LOSS",
                        f"{round(margin, 2)}",
                    ]
                )
                points_left_on_bench_per_week.append(
                    [
                        str(week),
                        matchup["away_team"],
                        away_diff,
                        "WIN",
                        f"{round(-margin, 2)}",
                    ]
                )

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

    # The weekly points left on bench
    points_left_on_bench_per_week = sorted(points_left_on_bench_per_week, key=lambda p: p[2], reverse=True)
    pt = PrettyTable()
    pt.field_names = ["Week", "Team", "Points Left on Bench", "Result", "Margin"]
    pt.title = "Points left on Bench per week"

    for idx, sl in enumerate(points_left_on_bench_per_week):
        points_left_on_bench_per_week[idx][2] = f"{round(points_left_on_bench_per_week[idx][2], 2)}"

    pt.add_rows(points_left_on_bench_per_week)
    print(pt)

    return None


def rank_weekly_performances(data: dict[str, any]) -> None:
    pt = PrettyTable()
    pt.title = "Best/Worst Weekly Performances"
    pt.field_names = ["Week", "Team Name", "Performance Over Expected"]

    all_data = []

    for week, matchups in data["matchup_data"].items():
        for matchup in matchups:
            if matchup["home_team_score"] == 0.0:
                continue
            all_data.append(
                [
                    week,
                    matchup["home_team"],
                    round(
                        matchup["home_team_score"] - matchup["home_team_projected_score"],
                        2,
                    ),
                ]
            )
            all_data.append(
                [
                    week,
                    matchup["away_team"],
                    round(
                        matchup["away_team_score"] - matchup["away_team_projected_score"],
                        2,
                    ),
                ]
            )

    sorted_data = sorted(all_data, key=lambda p: p[2])

    pt.add_rows(sorted_data[:5])
    pt.add_row(["-", "-", "-"])
    pt.add_rows(sorted_data[-5:])

    print(pt)
    return


def random_scheduling(data: dict[str, any]) -> None:
    matchup_data = data["matchup_data"]

    all_teams = set()
    team_scores_by_week = {}
    weeks_completed = -1
    team_wins = {}

    for week, scoreboard in matchup_data.items():
        for matchup in scoreboard:
            if matchup["home_team_score"] == 0.0 and matchup["away_team_score"] == 0.0:
                continue

            all_teams.add(matchup["home_team"])
            all_teams.add(matchup["away_team"])

            home_score = matchup["home_team_score"]
            away_score = matchup["away_team_score"]
            try:
                team_scores_by_week[matchup["home_team"]].append(matchup["home_team_score"])
            except KeyError:
                team_scores_by_week[matchup["home_team"]] = [matchup["home_team_score"]]

            try:
                team_scores_by_week[matchup["away_team"]].append(matchup["away_team_score"])
            except KeyError:
                team_scores_by_week[matchup["away_team"]] = [matchup["away_team_score"]]

            if home_score > away_score:
                try:
                    team_wins[matchup["home_team"]] += 1
                except KeyError:
                    team_wins[matchup["home_team"]] = 1
            else:
                try:
                    team_wins[matchup["away_team"]] += 1
                except KeyError:
                    team_wins[matchup["away_team"]] = 1

            weeks_completed = int(week)

    team_results = {t: [0 for i in range(0, weeks_completed + 1)] for t in all_teams}

    number_of_sims = 250_000
    for i in range(0, number_of_sims):
        results = SingleSeasonSimulationResults([t for t in all_teams])
        for week in range(0, weeks_completed):
            schedule = _randomize_set(all_teams)

            for j in range(0, len(schedule), 2):
                first_score = team_scores_by_week[schedule[j]][week]
                second_score = team_scores_by_week[schedule[j + 1]][week]

                if first_score > second_score:
                    results.team_win(schedule[j], first_score, second_score)
                    results.team_loss(schedule[j + 1], second_score, first_score)
                else:
                    results.team_win(schedule[j + 1], second_score, first_score)
                    results.team_loss(schedule[j], first_score, second_score)

        for team in results.get_sorted_results():
            # This increments the win count for a team
            team_results[team[0]][team[1]] += 1

    team_results_as_list = [flatten_extend([[team_name], results]) for team_name, results in team_results.items()]
    team_results_as_list = sorted(team_results_as_list, key=lambda p: p[1:], reverse=True)

    # Add two columns for actual and expected # of wins
    for idx, _ in enumerate(team_results_as_list):
        team_results_as_list[idx].append(0)
        team_results_as_list[idx].append(0)

    for idx, team_results in enumerate(team_results_as_list):
        for i in range(1, len(team_results) - 2):
            if team_results[i] == 0:
                team_results[i] = " "
            else:
                team_results[-1] += team_results[i] * (i - 1) / number_of_sims
                team_results[i] = f"{round(100 * team_results[i] / number_of_sims, 1)} %"
        team_results_as_list[idx][-2] = team_wins[team_results[0]]
        team_results_as_list[idx][-1] = f"{round(team_results[-1], 1)}"

    pt = PrettyTable()
    field_names = flatten_extend([["Team Name"], [str(i) for i in range(0, weeks_completed + 1)]])
    pt.field_names = flatten_extend([field_names, ["Actual Wins", "Average Wins"]])
    pt.title = f"Odds of # of Wins ({number_of_sims} random schedules)"
    pt.add_rows(team_results_as_list)
    print(pt)

    return


def _randomize_set(teams: set[str]) -> list[str]:
    teams_list = [t for t in teams]
    random.shuffle(teams_list)
    return teams_list


def run_monte_carlo_simulation_from_week(
    league: League,
    data: dict[str, any],
    positional_data: dict[str, tuple[float, float]],
    schedule: list[list[dict[str, any]]],
    week: int = None,
    n: int = 10000,
) -> tuple[dict, dict]:
    if not week:
        week = league.current_week
    season_data = data["matchup_data"]

    season_simulation = SeasonSimulation(season_data, positional_data, league, schedule, starting_week=week)
    season_simulation.expected_wins()
    reg, playoff = season_simulation.run(50000)
    season_simulation.print_regular_season_projected_win_losses()
    season_simulation.print_regular_season_predictions()
    season_simulation.print_playoff_predictions()

    return reg, playoff


def get_historical_basic_stats() -> None:
    output = {}
    for year in range(2023, 2017, -1):
        league = League(league_id=345674, year=year, swid=SWID, espn_s2=ESPN_S2, debug=False)

        result = league.standings()

        # ret = {}
        for team in result:
            team = league.get_team_data(team.team_id)
            owner = f"{team.owners[0]['firstName']} {team.owners[0]['lastName']}".title()
            try:
                data = output[owner]
                data["wins"] += team.wins
                data["losses"] += team.losses
                data["total_points_for"] += team.points_for
                data["total_points_against"] += team.points_against
                data["record"] = data["wins"] / (data["wins"] + data["losses"])
            except KeyError:
                output[owner] = {
                    "owner": ""
                    if len(team.owners) == 0
                    else f"{team.owners[0]['firstName']} {team.owners[0]['lastName']}".title(),
                    "wins": team.wins,
                    "losses": team.losses,
                    "total_points_for": team.points_for,
                    "total_points_against": team.points_against,
                    "id": len(output) + 1,
                }

    # Remove the last two, sorry Knapp and Trev
    write_to_file(sorted([i for _, i in output.items()], key=lambda x: x["wins"], reverse=True)[:-2], "basic.json")

    return None


def get_schedule(league: League) -> None:
    schedule = []

    team_to_id = {}
    team_id_to_owner = {}

    # Get the owners names from each Team
    result = league.standings()
    for team in result:
        team = league.get_team_data(team.team_id)
        team_to_id[f"{team.owners[0]['firstName']} {team.owners[0]['lastName']}".title()] = team.team_id
        team_id_to_owner[team.team_id] = f"{team.owners[0]['firstName']} {team.owners[0]['lastName']}".title()

    for week in range(1, 15):
        # weekly_schedule = league.scoreboard(week=week)
        weekly_schedule = league.box_scores(week=week)
        week_matchups = []
        for matchup in weekly_schedule:
            home_team_id = team_to_id[team_id_to_owner[matchup.home_team.team_id]]
            away_team_id = team_to_id[team_id_to_owner[matchup.away_team.team_id]]
            home_team_owner = team_id_to_owner[home_team_id]
            away_team_owner = team_id_to_owner[away_team_id]
            week_matchups.append(
                {
                    "home_team_id": home_team_id,
                    "away_team_id": away_team_id,
                    "home_team_owner": home_team_owner,
                    "away_team_owner": away_team_owner,
                    "home_team_score": matchup.home_score,
                    "away_team_score": matchup.away_score,
                    "home_team_espn_projected_score": matchup.home_projected,
                    "away_team_espn_projected_score": matchup.away_projected,
                }
            )

        schedule.append(week_matchups)

    create_teams(team_to_id)

    write_to_file(schedule, "schedule.json")
    create_schedule(schedule, year=league.year)
    write_to_file(team_id_to_owner, "team_id_to_owner.json")
    write_to_file(team_to_id, "team_to_id.json")

    return None


def create_schedule(schedule: list[list[dict[str, any]]], year) -> None:
    conn = psycopg2.connect(os.environ["COCKROACHDB_URL"])

    with conn.cursor() as cur:
        for week, matchups in enumerate(schedule):
            for matchup in matchups:
                cur.execute(
                    "INSERT INTO matchups (week, year, home_team_espn_id, away_team_espn_id, home_team_final_score, away_team_final_score, home_team_espn_projected_score, away_team_espn_projected_score) SELECT %s, %s, %s, %s, %s, %s, %s, %s WHERE NOT EXISTS (SELECT 1 FROM matchups WHERE week = %s AND year = %s AND home_team_espn_id = %s AND away_team_espn_id = %s)",
                    (
                        week + 1,
                        year,
                        matchup["home_team_id"],
                        matchup["away_team_id"],
                        matchup["home_team_score"],
                        matchup["away_team_score"],
                        matchup["home_team_espn_projected_score"],
                        matchup["away_team_espn_projected_score"],
                        week + 1,
                        year,
                        matchup["home_team_id"],
                        matchup["away_team_id"],
                    ),
                )

        conn.commit()
        cur.close()

    return None


def create_teams(team_to_id: dict[str, int]) -> None:
    conn = psycopg2.connect(os.environ["COCKROACHDB_URL"])

    with conn.cursor() as cur:
        for owner, team_id in team_to_id.items():
            cur.execute(
                "INSERT INTO teams (owner, espn_id) SELECT %s, %s WHERE NOT EXISTS (SELECT 1 FROM teams WHERE owner = %s AND espn_id = %s)",
                (owner, team_id, owner, team_id),
            )

        conn.commit()
        cur.close()

    return None


def get_basic_stats(league: League) -> None:
    team_id_to_owner: dict[str, str] = {}
    scores: dict[str, list[float]] = {}
    all_scores: list[float] = []

    result = league.standings()
    for team in result:
        team = league.get_team_data(team.team_id)
        owner = f"{team.owners[0]['firstName']} {team.owners[0]['lastName']}".title()
        team_id_to_owner[team.team_id] = owner
        scores[owner] = []

    for week in range(1, 15):
        weekly_schedule = league.scoreboard(week=week)
        for matchup in weekly_schedule:
            if matchup.is_playoff:
                continue
            try:
                scores[team_id_to_owner[matchup.home_team.team_id]].append(matchup.home_score)
                scores[team_id_to_owner[matchup.away_team.team_id]].append(matchup.away_score)
                all_scores.append(matchup.home_score)
                all_scores.append(matchup.away_score)
            except KeyError:
                pass

    output = {}
    for owner, score in scores.items():
        output[owner] = {
            "average": round(mean(score), 3),
            "std_dev": round(std_dev(score), 3),
        }

    output["League"] = {
        "average": round(mean(all_scores), 3),
        "std_dev": round(std_dev(all_scores), 3),
    }

    write_to_file(output, "team_avgs.json")
    return None


def get_simple_draft(league: League) -> None:
    # draft stuff
    draft_data = []
    for pick in league.draft:
        draft_data.append(
            {
                "player_name": pick.playerName,
                "player_id": pick.playerId,
                "team_id": pick.team.team_id,
                "team_name": pick.team.team_name.rstrip(" "),
                "round_number": pick.round_num,
                "round_pick": pick.round_pick,
            }
        )

    write_to_file(draft_data, "draft_data.json")


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

    # get_historical_basic_stats()

    league = League(league_id=345674, year=2023, swid=SWID, espn_s2=ESPN_S2, debug=False)

    get_schedule(league)
    get_basic_stats(league)
    get_simple_draft(league)

    exit(1)

    data, league = scrape_matchups()

    # random_scheduling(data)

    rank_weekly_performances(data)

    try:
        logging.info("calculating stats about the draft")

        perform_draft_analytics(data, league)

        perform_redraft(data["draft_data"])

        perform_roster_analysis(data, league.current_week)

        # perform_waiver_analysis(data["activity_data"])

        rank_weekly_performances(data)

        logging.info("calculating overperformance by team")
        calc_team_overperformance(data, league.current_week)

        logging.info("calculating basic statistics for positional data")
        position_data = calc_position_performances(data)
        data["position_data"] = position_data
        schedule = data["schedule"]

        regular_season_results, playoff_results = run_monte_carlo_simulation_from_week(
            league, data, position_data, schedule, n=1
        )
        data["regular_season_results"] = regular_season_results
        data["playoff_results"] = playoff_results

    finally:
        write_to_file(data)
        print(f"Completed in {round(time.time() - start,2)} seconds")
