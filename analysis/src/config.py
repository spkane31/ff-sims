"""Segment and season configuration for the valuation pipeline."""

from dataclasses import dataclass
from datetime import date, datetime, timedelta


@dataclass(frozen=True)
class Segment:
    """A league segment: one scoring/roster format the model runs on."""

    key: str
    ppr: float
    is_superflex: bool
    total_rosters: int
    league_type: str = "redraft"
    draft_type: str = "snake"  # ADP only; auction pick_no isn't a draft position


PPR_SF_12 = Segment(key="ppr-sf-12", ppr=1.0, is_superflex=True, total_rosters=12)


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
