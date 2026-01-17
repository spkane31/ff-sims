from dataclasses import dataclass
from typing import Any, Optional

import requests


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


@dataclass
class User:
    user_id: str
    username: str
    display_name: str
    avatar: Optional[str]
    metadata: dict
    is_owner: bool


@dataclass
class DraftSettings:
    alpha_sort: int
    autopause_enabled: int
    autopause_end_time: int
    autopause_start_time: int
    autostart: int
    cpu_autopick: int
    enforce_position_limits: int
    nomination_timer: int
    pick_timer: int
    player_type: int
    reversal_round: int
    rounds: int
    teams: int
    slots_bn: Optional[int] = None
    slots_def: Optional[int] = None
    slots_flex: Optional[int] = None
    slots_k: Optional[int] = None
    slots_qb: Optional[int] = None
    slots_rb: Optional[int] = None
    slots_te: Optional[int] = None
    slots_wr: Optional[int] = None
    slots_super_flex: Optional[int] = None


@dataclass
class DraftMetadata:
    scoring_type: str
    name: str
    description: str
    show_team_names: str
    elapsed_pick_timer: Optional[str] = None
    is_autopaused: Optional[str] = None


@dataclass
class Draft:
    type: str
    status: str
    start_time: int
    sport: str
    settings: DraftSettings
    season_type: str
    season: str
    metadata: DraftMetadata
    league_id: str
    last_picked: int
    last_message_time: int
    last_message_id: str
    draft_order: Optional[list[str]]
    draft_id: str
    creators: Optional[list[str]]
    created: int

    @property
    def rounds(self) -> int:
        """Expose rounds from settings for backward compatibility."""
        return self.settings.rounds

    @property
    def pick_time(self) -> int:
        """Expose pick_timer from settings as pick_time for backward compatibility."""
        return self.settings.pick_timer


class SleeperClient:
    def __init__(self):
        self._base_url = "https://api.sleeper.app/v1"

    def _get(self, endpoint: str, retry_count: int = 0) -> Any:
        response = requests.get(f"{self._base_url}{endpoint}")
        response.raise_for_status()
        return response.json()

    def get_league(self, league_id: str) -> League:
        data = self._get(f"/league/{league_id}")

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

    def get_users_in_leauge(self, league_id: str) -> list[User]:
        data = self._get(f"/league/{league_id}/users")

        return [
            User(
                user_id=user_data.get("user_id"),
                username=user_data.get("username"),
                display_name=user_data.get("display_name"),
                avatar=user_data.get("avatar"),
                metadata=user_data.get("metadata", {}),
                is_owner=user_data["is_owner"],
            )
            for user_data in data
        ]

    # DRAFTS endpoints

    def get_all_drafts_for_user(self, user_id: str, season: str, sport: str = "nfl") -> list[Draft]:
        data = self._get(f"/user/{user_id}/drafts/{sport}/{season}")
        return [
            Draft(
                type=draft_data.get("type"),
                status=draft_data.get("status"),
                start_time=draft_data.get("start_time"),
                sport=draft_data.get("sport"),
                settings=DraftSettings(**draft_data.get("settings")),
                season_type=draft_data.get("season_type"),
                season=draft_data.get("season"),
                metadata=DraftMetadata(**draft_data.get("metadata")),
                league_id=draft_data.get("league_id"),
                last_picked=draft_data.get("last_picked"),
                last_message_time=draft_data.get("last_message_time"),
                last_message_id=draft_data.get("last_message_id"),
                draft_order=draft_data.get("draft_order"),
                draft_id=draft_data.get("draft_id"),
                creators=draft_data.get("creators"),
                created=draft_data.get("created"),
            )
            for draft_data in data
        ]
