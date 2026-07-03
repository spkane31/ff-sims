from src.db import rows_to_adp, rows_to_scores


def test_rows_to_adp():
    rows = [("p1", "Josh Allen", "QB", 1.8), ("p2", "Bijan Robinson", "RB", 2.4)]
    adp = rows_to_adp(rows)
    assert adp[0].player_id == "p1"
    assert adp[0].player_name == "Josh Allen"
    assert adp[0].position == "QB"
    assert adp[0].adp == 1.8


def test_rows_to_scores():
    rows = [(1, "p1", "QB", 31.5), (1, "p2", "RB", 0.0)]
    scores = rows_to_scores(rows)
    assert scores[0].week == 1
    assert scores[0].points == 31.5
    assert scores[1].points == 0.0
