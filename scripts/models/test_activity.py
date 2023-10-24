from .activity import _net_points_from_action


def test_net_points_from_action():
    net = _net_points_from_action(
        [
            {
                "team": "Von Miller High Life",
                "action": "DROPPED",
                "player_name": "Ravens D/ST",
                "player_id": -16033,
                "player_weekly_points": [12.0, 5.0, 9.0, 17.0],
            },
            {
                "team": "Von Miller High Life",
                "action": "WAIVER ADDED",
                "player_name": "Packers D/ST",
                "player_id": -16009,
                "player_weekly_points": [14.0, 0.0, 7.0, 0.0],
            },
        ],
        2,
    )
    assert net == -24.0

    net = _net_points_from_action(
        [
            {
                "team": "The glass legs",
                "action": "WAIVER ADDED",
                "player_name": "Tyler Allgeier",
                "player_id": 4373626,
                "player_weekly_points": [24.4, 4.8, 4.9, 2.2],
            }
        ],
        2,
    )
    assert round(net, 1) == 11.9

    net = _net_points_from_action(
        [
            {
                "team": "Daddy Doepker",
                "action": "DROPPED",
                "player_name": "Adam Thielen",
                "player_id": 16460,
                "player_weekly_points": [3.2, 20.4, 31.5, 15.2],
            }
        ],
        2,
    )
    assert round(net, 1) == -67.1
