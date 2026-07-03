from datetime import datetime

from src.config import (
    DEFAULT_SEGMENT_KEY,
    PPR_SF_10,
    PPR_SF_12,
    SEASONS,
    SEGMENTS,
    week_ts,
)


def test_segments_registry():
    # master map: every segment is registered under its own key
    assert SEGMENTS["ppr-sf-12"] is PPR_SF_12
    assert SEGMENTS["ppr-sf-10"] is PPR_SF_10
    assert all(seg.key == key for key, seg in SEGMENTS.items())
    assert DEFAULT_SEGMENT_KEY == "ppr-sf-12"
    assert DEFAULT_SEGMENT_KEY in SEGMENTS


def test_ppr_sf_10_segment():
    assert PPR_SF_10.total_rosters == 10
    assert PPR_SF_10.is_superflex is True
    assert PPR_SF_10.ppr == 1.0
    # replacement ranks scale with league size (2 QB / 2.5 RB / 3 WR per team)
    assert PPR_SF_10.repl_rank_by_pos == {
        "QB": 20, "RB": 25, "WR": 30, "TE": 10, "DEF": 10, "K": 10,
    }


def test_segment_ppr_sf_12():
    assert PPR_SF_12.key == "ppr-sf-12"
    assert PPR_SF_12.ppr == 1.0
    assert PPR_SF_12.is_superflex is True
    assert PPR_SF_12.total_rosters == 12
    assert PPR_SF_12.league_type == "redraft"
    assert PPR_SF_12.draft_type == "snake"


def test_segment_carries_replacement_ranks():
    # superflex: QB replacement is deep (2 starters/team)
    assert PPR_SF_12.repl_rank_by_pos["QB"] == 24
    assert PPR_SF_12.repl_rank_by_pos["RB"] == 30
    assert set(PPR_SF_12.repl_rank_by_pos) == {"QB", "RB", "WR", "TE", "DEF", "K"}


def test_week_ts_2025():
    s = SEASONS["2025"]
    # 2025 kickoff Thu Sep 4; week 1 scores land 4 days later, Mon Sep 8
    assert week_ts(s, 1) == datetime(2025, 9, 8)
    assert week_ts(s, 2) == datetime(2025, 9, 15)


def test_seasons_have_2026():
    assert "2026" in SEASONS
    assert SEASONS["2026"].draft_date < SEASONS["2026"].season_start
