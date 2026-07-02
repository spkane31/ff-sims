from datetime import datetime

import pandas as pd

from src.models import PlayerBeliefState
from src.valuation import Valuator

START = datetime(2025, 8, 25)


def _adp():
    return pd.DataFrame(
        [
            {"player_id": "p1", "player_name": "QB One", "position": "QB", "adp": 1.0},
            {"player_id": "p2", "player_name": "RB Two", "position": "RB", "adp": 5.0},
        ]
    )


def test_state_round_trip_preserves_rankings():
    v = Valuator(start_ts=START)
    v.seed_from_adp(_adp())
    v.apply_trade(["p1"], ["p2"])

    restored = Valuator.from_state(v.to_state(), last_ts=v.last_ts)
    assert restored.last_ts == v.last_ts
    orig = v.rankings()
    back = restored.rankings()
    pd.testing.assert_frame_equal(orig, back)


def test_seed_from_adp_skips_existing_players():
    v = Valuator(start_ts=START)
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
    v = Valuator(start_ts=START)
    assert v.last_ts == START


def test_belief_state_fields():
    s = PlayerBeliefState(
        player_id="p1", guess=100.0, var=5.0, games=2.0, cum_par=3.0,
        position="QB", name="QB One",
    )
    assert s.player_id == "p1"
