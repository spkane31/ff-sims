from datetime import datetime

import pandas as pd

from src.config import PPR_SF_12
from src.models import PlayerBeliefState
from src.valuation import Valuator

START = datetime(2025, 8, 25)
REPL = PPR_SF_12.repl_rank_by_pos


def _adp():
    return pd.DataFrame(
        [
            {"player_id": "p1", "player_name": "QB One", "position": "QB", "adp": 1.0},
            {"player_id": "p2", "player_name": "RB Two", "position": "RB", "adp": 5.0},
        ]
    )


def test_state_round_trip_preserves_rankings():
    v = Valuator(start_ts=START, repl_rank_by_pos=REPL)
    v.seed_from_adp(_adp())
    v.apply_trade(["p1"], ["p2"])

    restored = Valuator.from_state(v.to_state(), last_ts=v.last_ts, repl_rank_by_pos=REPL)
    assert restored.last_ts == v.last_ts
    orig = v.rankings()
    back = restored.rankings()
    pd.testing.assert_frame_equal(orig, back)


def test_seed_from_adp_skips_existing_players():
    v = Valuator(start_ts=START, repl_rank_by_pos=REPL)
    v.seed_from_adp(_adp())
    before = v.beliefs["p1"].guess
    v.apply_trade(["p1"], ["p2"])  # moves p1 off its seed
    v.seed_from_adp(_adp())  # re-seeding must NOT reset p1
    assert v.beliefs["p1"].guess != before or v.beliefs["p1"].var != 1_500_000.0
    # and a brand-new player does get seeded
    new = _adp().assign(player_id=["p1", "p9"])
    v.seed_from_adp(new)
    assert "p9" in v.beliefs


def test_start_ts_sets_model_clock():
    v = Valuator(start_ts=START, repl_rank_by_pos=REPL)
    assert v.last_ts == START


def test_rankings_include_position_rank():
    v = Valuator(start_ts=START, repl_rank_by_pos=REPL)
    v.seed_from_adp(
        pd.DataFrame(
            [
                {"player_id": "q1", "player_name": "QB A", "position": "QB", "adp": 1.0},
                {"player_id": "r1", "player_name": "RB A", "position": "RB", "adp": 2.0},
                {"player_id": "q2", "player_name": "QB B", "position": "QB", "adp": 3.0},
                {"player_id": "r2", "player_name": "RB B", "position": "RB", "adp": 4.0},
            ]
        )
    )
    df = v.rankings()
    by_pid = df.set_index("player_id")
    # overall order is q1, r1, q2, r2; position ranks restart per position
    assert by_pid.loc["q1", "pos_rank"] == 1
    assert by_pid.loc["q2", "pos_rank"] == 2
    assert by_pid.loc["r1", "pos_rank"] == 1
    assert by_pid.loc["r2", "pos_rank"] == 2


def test_valuator_accepts_segment_replacement_ranks():
    # replacement rank 1 means nearly everyone scores below replacement
    custom = {"QB": 1, "RB": 1, "WR": 1, "TE": 1, "DEF": 1, "K": 1}
    v = Valuator(start_ts=START, repl_rank_by_pos=custom)
    assert v.repl_rank_by_pos == custom
    restored = Valuator.from_state(
        v.to_state(), last_ts=v.last_ts, repl_rank_by_pos=custom
    )
    assert restored.repl_rank_by_pos == custom


def test_belief_state_fields():
    s = PlayerBeliefState(
        player_id="p1", guess=100.0, var=5.0, games=2.0, cum_par=3.0,
        position="QB", name="QB One",
    )
    assert s.player_id == "p1"
