"""Segment and season configuration for the valuation pipeline."""

from dataclasses import dataclass
from datetime import date, datetime, timedelta

# A player must appear in at least this many of a segment's drafts to get an
# ADP seed. Below this, a mean pick is a fluke (one autodraft accident at pick
# 17 seeded an Albany TE as the #1 TE); such players enter the model only via
# real evidence (games/trades), with wide unseen-player uncertainty.
MIN_ADP_DRAFTS = 5


@dataclass(frozen=True)
class Segment:
    """A league segment: one scoring/roster format the model runs on."""

    key: str
    ppr: float
    is_superflex: bool
    total_rosters: int
    # weekly replacement rank per position for THIS league combo: the Nth-best
    # scorer at a position is "replacement" (feeds PAR in the model)
    repl_rank_by_pos: dict[str, int]
    league_type: str = "redraft"
    draft_type: str = "snake"  # ADP only; auction pick_no isn't a draft position


PPR_SF_12 = Segment(
    key="ppr-sf-12",
    ppr=1.0,
    is_superflex=True,
    total_rosters=12,
    repl_rank_by_pos={
        "QB": 24,  # superflex: ~2 QB starters per team
        "RB": 30,
        "WR": 36,
        "TE": 12,
        "DEF": 12,
        "K": 12,
    },
)

PPR_SF_10 = Segment(
    key="ppr-sf-10",
    ppr=1.0,
    is_superflex=True,
    total_rosters=10,
    repl_rank_by_pos={  # same per-team ratios as ppr-sf-12, scaled to 10 teams
        "QB": 20,
        "RB": 25,
        "WR": 30,
        "TE": 10,
        "DEF": 10,
        "K": 10,
    },
)

PPR_SF_8 = Segment(
    key="ppr-sf-8",
    ppr=1.0,
    is_superflex=True,
    total_rosters=8,
    repl_rank_by_pos={  # same per-team ratios as ppr-sf-12, scaled to 8 teams
        "QB": 16,
        "RB": 20,
        "WR": 24,
        "TE": 8,
        "DEF": 8,
        "K": 8,
    },
)

# Master registry: every runnable segment, keyed by its segment key. Add new
# league combos here (e.g. a 1QB or half-PPR Segment) and they become valid
# --segment values everywhere.
SEGMENTS: dict[str, Segment] = {s.key: s for s in [PPR_SF_12, PPR_SF_10, PPR_SF_8]}

DEFAULT_SEGMENT_KEY = PPR_SF_12.key


@dataclass(frozen=True)
class SeasonDates:
    draft_date: date  # when the ADP belief is seeded (model clock start)
    season_start: date  # NFL week 1 kickoff (Thursday)
    score_lag_days: int = 4  # week W scores land ~this many days after kickoff


SEASONS: dict[str, SeasonDates] = {
    "2025": SeasonDates(draft_date=date(2025, 8, 25), season_start=date(2025, 9, 4)),
    "2026": SeasonDates(draft_date=date(2026, 8, 24), season_start=date(2026, 9, 10)),
}


def week_to_date(season: SeasonDates, week: int) -> date:
    return season.season_start + timedelta(days=(week - 1) * 7 + season.score_lag_days)


def week_ts(season: SeasonDates, week: int) -> datetime:
    return datetime.combine(week_to_date(season, week), datetime.min.time())
