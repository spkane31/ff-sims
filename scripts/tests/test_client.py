import pytest

from src.sleeper import SleeperClient

TEST_LEAGUE_ID = "1312466892979466240"


def test_sleeper_client_initialization():
    client = SleeperClient()
    assert client is not None


def test_get_league():
    client = SleeperClient()
    league = client.get_league(TEST_LEAGUE_ID)

    # Test basic fields
    assert league.league_id == TEST_LEAGUE_ID
    assert league.name == "Temporal League 2"
    assert league.sport == "nfl"
    assert league.status == "pre_draft"
    assert league.season == "2026"
    assert league.total_rosters == 12

    # Test nested metadata
    assert league.metadata is not None
    assert league.metadata.auto_continue == "on"

    # Test nested settings
    assert league.settings is not None
    assert league.settings.num_teams == 12
    assert league.settings.playoff_teams == 6
    assert league.settings.draft_rounds == 3

    # Test nested scoring settings
    assert league.scoring_settings is not None
    assert league.scoring_settings.rec == 1.0
    assert league.scoring_settings.pass_td == 4.0
    assert league.scoring_settings.rush_yd == 0.1

    # Test roster positions
    assert len(league.roster_positions) > 0
    assert "QB" in league.roster_positions
    assert "RB" in league.roster_positions


def test_get_users_in_league():
    client = SleeperClient()
    users = client.get_users_in_leauge(TEST_LEAGUE_ID)

    assert isinstance(users, list)
    assert len(users) > 0

    # Test fields of the first user
    for user in users:
        assert isinstance(user, object)
        assert hasattr(user, "user_id")
        assert hasattr(user, "display_name")
        assert hasattr(user, "avatar")
        assert hasattr(user, "metadata")
        assert isinstance(user.user_id, str)
        assert isinstance(user.display_name, str)
        assert isinstance(user.metadata, dict)
        assert isinstance(user.is_owner, bool)


def test_get_all_drafts_for_user():
    client = SleeperClient()
    users = client.get_users_in_leauge(TEST_LEAGUE_ID)
    user_id = users[0].user_id
    drafts = client.get_all_drafts_for_user(user_id, season="2025", sport="nfl")

    assert isinstance(drafts, list)

    for draft in drafts:
        assert isinstance(draft, object)
        assert hasattr(draft, "draft_id")
        assert hasattr(draft, "league_id")
        assert hasattr(draft, "status")
        assert hasattr(draft, "type")
        assert hasattr(draft, "rounds")
        assert hasattr(draft, "pick_time")
        assert hasattr(draft, "created")
        assert isinstance(draft.draft_id, str)
        assert isinstance(draft.league_id, str)
        assert isinstance(draft.status, str)
        assert isinstance(draft.type, str)
        assert isinstance(draft.rounds, int)
        assert isinstance(draft.pick_time, int)
        assert isinstance(draft.created, int)
