import datetime
import math

from espn_api.football import League
from prettytable import PrettyTable


def get_waiver_wire_activity(league: League) -> list[dict[str, any]]:
    activities = []
    # Waiver wire and draft activity
    for offset in [0, 25, 50, 75]:
        recent_activity = league.recent_activity(25, offset=offset)
        for activity in recent_activity:
            a = {
                "date": activity.date,
                "week": _convert_date_to_nfl_week(activity.date),
                "actions": [
                    {
                        "team": action[0].team_name,
                        "action": action[1],
                        "player_name": action[2].name,
                        "player_id": action[2].playerId,
                        "player_weekly_points": _get_weekly_points_from_activity(
                            action[2], league
                        ),
                    }
                    for action in activity.actions
                ],
            }

            activities.append(a)

    return activities


def perform_waiver_analysis(waivers: list) -> None:
    team_net_points = {}
    for activity in waivers:
        week = activity["week"]
        if week < 0:
            continue
        net = _net_points_from_action(activity["actions"], week)
        for action in activity["actions"]:
            if len(action["player_weekly_points"]) < week:
                # Nothing to be done here
                continue

            if action["action"] == "DROPPED":
                # Subtract points from team_net_points
                try:
                    team_net_points[action["team"]] -= sum(
                        action["player_weekly_points"][week - 1 :]
                    )
                except KeyError:
                    team_net_points[action["team"]] = -sum(
                        action["player_weekly_points"][week - 1 :]
                    )

            if action["action"] == "FA ADDED" or action["action"] == "WAIVER ADDED":
                # Subtract points from team_net_points
                try:
                    team_net_points[action["team"]] += sum(
                        action["player_weekly_points"][week - 1 :]
                    )
                except KeyError:
                    team_net_points[action["team"]] = sum(
                        action["player_weekly_points"][week - 1 :]
                    )

    sortable_list = [
        [team, f"{round(net_points, 2)}"]
        for team, net_points in team_net_points.items()
    ]
    sortable_list = sorted(sortable_list, key=lambda x: float(x[1]), reverse=True)

    pt = PrettyTable()
    pt.field_names = ["Team", "Net Points"]
    pt.add_rows(sortable_list)
    pt.title = "Net Points from Waiver Wire Activity"
    print(pt)


def _net_points_from_action(actions: list[dict[str, any]], week: int) -> float:
    ret = 0

    for action in actions:
        if len(action["player_weekly_points"]) < week:
            # Nothing to be done here
            return ret

        if action["action"] == "DROPPED":
            # Subtract points from team_net_points
            ret -= sum(action["player_weekly_points"][week - 1 :])

        if action["action"] == "FA ADDED" or action["action"] == "WAIVER ADDED":
            # Subtract points from team_net_points
            ret += sum(action["player_weekly_points"][week - 1 :])

    return ret


def _convert_date_to_nfl_week(epoch: int) -> int:
    dt = datetime.datetime.fromtimestamp(epoch / 1000).date()
    week1 = datetime.date(2023, 9, 5)
    delta = dt - week1
    return max(math.floor((delta.days / 7) + 1), 1)


def _get_weekly_points_from_activity(player, league) -> list[float]:
    player = league.player_info(playerId=player.playerId)
    ret = []
    for i in range(1, 15):
        try:
            ret.append(player.stats[i]["points"])
        except KeyError:
            continue

    return ret
