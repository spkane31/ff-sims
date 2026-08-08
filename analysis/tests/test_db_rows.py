from src.db import rows_to_scores


def test_rows_to_scores():
    rows = [(1, "p1", "QB", 31.5), (1, "p2", "RB", 0.0)]
    scores = rows_to_scores(rows)
    assert scores[0].week == 1
    assert scores[0].points == 31.5
    assert scores[1].points == 0.0
