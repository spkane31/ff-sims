from datetime import datetime

from src.config import PPR_SF_12, SEASONS, week_ts


def test_segment_ppr_sf_12():
    assert PPR_SF_12.key == "ppr-sf-12"
    assert PPR_SF_12.ppr == 1.0
    assert PPR_SF_12.is_superflex is True
    assert PPR_SF_12.total_rosters == 12
    assert PPR_SF_12.league_type == "redraft"
    assert PPR_SF_12.draft_type == "snake"


def test_week_ts_2025():
    s = SEASONS["2025"]
    # 2025 kickoff Thu Sep 4; week 1 scores land 4 days later, Mon Sep 8
    assert week_ts(s, 1) == datetime(2025, 9, 8)
    assert week_ts(s, 2) == datetime(2025, 9, 15)


def test_seasons_have_2026():
    assert "2026" in SEASONS
    assert SEASONS["2026"].draft_date < SEASONS["2026"].season_start
