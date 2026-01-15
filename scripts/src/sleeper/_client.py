import requests
from dataclasses import dataclass
from typing import Optional


@dataclass
class LeagueMetadata:
    auto_continue: str
    keeper_deadline: str


@dataclass
class LeagueSettings:
    best_ball: int
    waiver_budget: int
    disable_adds: int
    capacity_override: int
    waiver_bid_min: int
    taxi_deadline: int
    draft_rounds: int
    reserve_allow_na: int
    start_week: int
    playoff_seed_type: int
    playoff_teams: int
    veto_votes_needed: int
    num_teams: int
    daily_waivers_hour: int
    playoff_type: int
    taxi_slots: int
    sub_start_time_eligibility: int
    daily_waivers_days: int
    sub_lock_if_starter_active: int
    playoff_week_start: int
    waiver_clear_days: int
    reserve_allow_doubtful: int
    commissioner_direct_invite: int
    veto_auto_poll: int
    reserve_allow_dnr: int
    taxi_allow_vets: int
    waiver_day_of_week: int
    playoff_round_type: int
    reserve_allow_out: int
    reserve_allow_sus: int
    veto_show_votes: int
    trade_deadline: int
    taxi_years: int
    daily_waivers: int
    faab_suggestions: int
    disable_trades: int
    pick_trading: int
    type: int
    max_keepers: int
    waiver_type: int
    max_subs: int
    league_average_match: int
    trade_review_days: int
    bench_lock: int
    offseason_adds: int
    leg: int
    reserve_slots: int
    reserve_allow_cov: int


@dataclass
class ScoringSettings:
    sack: Optional[float] = None
    fgm_40_49: Optional[float] = None
    pass_int: Optional[float] = None
    pts_allow_0: Optional[float] = None
    pass_2pt: Optional[float] = None
    st_td: Optional[float] = None
    rec_td: Optional[float] = None
    fgm_30_39: Optional[float] = None
    fgm_50_59: Optional[float] = None
    xpmiss: Optional[float] = None
    rush_td: Optional[float] = None
    rec_2pt: Optional[float] = None
    st_fum_rec: Optional[float] = None
    fgmiss: Optional[float] = None
    ff: Optional[float] = None
    rec: Optional[float] = None
    pts_allow_14_20: Optional[float] = None
    fgm_0_19: Optional[float] = None
    int: Optional[float] = None
    def_st_fum_rec: Optional[float] = None
    fum_lost: Optional[float] = None
    pts_allow_1_6: Optional[float] = None
    fgm_60p: Optional[float] = None
    fgm_20_29: Optional[float] = None
    pts_allow_21_27: Optional[float] = None
    xpm: Optional[float] = None
    rush_2pt: Optional[float] = None
    fum_rec: Optional[float] = None
    def_st_td: Optional[float] = None
    def_td: Optional[float] = None
    safe: Optional[float] = None
    pass_yd: Optional[float] = None
    blk_kick: Optional[float] = None
    pass_td: Optional[float] = None
    rush_yd: Optional[float] = None
    fum: Optional[float] = None
    pts_allow_28_34: Optional[float] = None
    pts_allow_35p: Optional[float] = None
    fum_rec_td: Optional[float] = None
    rec_yd: Optional[float] = None
    def_st_ff: Optional[float] = None
    pts_allow_7_13: Optional[float] = None
    st_ff: Optional[float] = None


@dataclass
class League:
    name: str
    status: str
    metadata: LeagueMetadata
    settings: LeagueSettings
    avatar: Optional[str]
    company_id: Optional[str]
    shard: int
    season: str
    season_type: str
    sport: str
    scoring_settings: ScoringSettings
    last_message_id: str
    last_author_avatar: Optional[str]
    last_author_display_name: Optional[str]
    last_author_id: Optional[str]
    last_author_is_bot: Optional[bool]
    last_message_attachment: Optional[str]
    last_message_text_map: Optional[str]
    last_message_time: int
    last_pinned_message_id: Optional[str]
    draft_id: str
    last_read_id: Optional[str]
    league_id: str
    previous_league_id: str
    bracket_id: Optional[str]
    bracket_overrides_id: Optional[str]
    group_id: Optional[str]
    loser_bracket_id: Optional[str]
    loser_bracket_overrides_id: Optional[str]
    roster_positions: list[str]
    total_rosters: int


class SleeperClient:
    def __init__(self):
        self.base_url = "https://api.sleeper.app/v1"

    def get_league(self, league_id: str) -> League:
        # curl "https://api.sleeper.app/v1/league/<league_id>"
        response = requests.get(f"{self.base_url}/league/{league_id}")
        response.raise_for_status()
        data = response.json()

        # Parse nested objects
        metadata = LeagueMetadata(**data["metadata"])
        settings = LeagueSettings(**data["settings"])
        scoring_settings = ScoringSettings(**data["scoring_settings"])

        # Create League object with parsed nested objects
        return League(
            name=data["name"],
            status=data["status"],
            metadata=metadata,
            settings=settings,
            avatar=data.get("avatar"),
            company_id=data.get("company_id"),
            shard=data["shard"],
            season=data["season"],
            season_type=data["season_type"],
            sport=data["sport"],
            scoring_settings=scoring_settings,
            last_message_id=data["last_message_id"],
            last_author_avatar=data.get("last_author_avatar"),
            last_author_display_name=data.get("last_author_display_name"),
            last_author_id=data.get("last_author_id"),
            last_author_is_bot=data.get("last_author_is_bot"),
            last_message_attachment=data.get("last_message_attachment"),
            last_message_text_map=data.get("last_message_text_map"),
            last_message_time=data["last_message_time"],
            last_pinned_message_id=data.get("last_pinned_message_id"),
            draft_id=data["draft_id"],
            last_read_id=data.get("last_read_id"),
            league_id=data["league_id"],
            previous_league_id=data["previous_league_id"],
            bracket_id=data.get("bracket_id"),
            bracket_overrides_id=data.get("bracket_overrides_id"),
            group_id=data.get("group_id"),
            loser_bracket_id=data.get("loser_bracket_id"),
            loser_bracket_overrides_id=data.get("loser_bracket_overrides_id"),
            roster_positions=data["roster_positions"],
            total_rosters=data["total_rosters"],
        )

